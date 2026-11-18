package storage_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"techpedia/internal/changeproposal"
	"techpedia/internal/document"
	"techpedia/internal/oauth"
	"techpedia/internal/storage"
	"techpedia/internal/techmark"
)

func TestPartialProposalMaterializesAndValidatesWholeSourceWithoutSavingDocument(t *testing.T) {
	db, _, g := proposalFixture(t)
	ctx := context.Background()
	doc, err := db.Save(ctx, document.Document{Locale: "ko", Title: "부분 수정", Slug: techmark.Slugify("부분 수정"), Source: "# 제목\n원문🙂\n그대로"})
	if err != nil {
		t.Fatal(err)
	}
	in := changeproposal.Input{Kind: "edit", Locale: doc.Locale, Name: doc.Title, BaseRevision: doc.Revision, Summary: "일부 변경", Edits: []changeproposal.TextEdit{{Before: "원문🙂", After: "수정🙂"}}}
	p := submitProposal(t, db, g, in)
	if p.Input.Source != "# 제목\n수정🙂\n그대로" || p.Before != doc.Source || len(p.Input.Edits) != 0 {
		t.Fatal(p)
	}
	current, _ := db.GetLatest(ctx, doc.Locale, doc.Slug)
	if current.Source != doc.Source || current.Revision != doc.Revision {
		t.Fatal("submitted directly applied")
	}
	in.Edits = nil
	in.Source = p.Input.Source
	if duplicate := submitProposal(t, db, g, in); duplicate.ID != p.ID {
		t.Fatal("full and partial duplicate not coalesced")
	}
	for _, after := range []string{"<wiki-home-grid>", "<wiki-recent-changes>"} {
		in.Source = ""
		in.Edits = []changeproposal.TextEdit{{Before: doc.Source, After: after}}
		if _, err := db.SubmitChangeProposal(ctx, in, g.ID, g.UserID, g.Username); !errors.Is(err, changeproposal.ErrInvalid) {
			t.Fatal("unsafe assembled source", err)
		}
	}
	in.Edits = []changeproposal.TextEdit{{Before: doc.Source, After: ""}}
	if _, err := db.SubmitChangeProposal(ctx, in, g.ID, g.UserID, g.Username); !errors.Is(err, changeproposal.ErrInvalid) {
		t.Fatal("empty assembled source", err)
	}
	in.BaseRevision = "r99"
	if _, err := db.SubmitChangeProposal(ctx, in, g.ID, g.UserID, g.Username); !errors.Is(err, changeproposal.ErrConflict) {
		t.Fatal("stale partial edit", err)
	}
}

func TestAmendmentCreatesImmutableReplacementWithSameExpiryAndBlocksOldApproval(t *testing.T) {
	db, _, g := proposalFixture(t)
	ctx := context.Background()
	p := submitProposal(t, db, g, changeproposal.Input{Kind: "create", Locale: "ko", Name: "새 문서", Source: "old\nkeep", Summary: "처음"})
	amend := changeproposal.Amendment{ProposalID: p.ID, Summary: "정정", Edits: []changeproposal.TextEdit{{Before: "old", After: "new"}}}
	next, err := db.AmendChangeProposal(ctx, amend, g.ID, g.UserID, g.Username)
	if err != nil {
		t.Fatal(err)
	}
	if next.ID == p.ID || next.Supersedes != p.ID || next.Input.Source != "new\nkeep" || next.Status != "pending" || !next.ExpiresAt.Equal(p.ExpiresAt) || !next.CreatedAt.Equal(p.CreatedAt) || next.AmendedAt.IsZero() {
		t.Fatal(next)
	}
	old, _ := db.GetChangeProposal(ctx, p.ID)
	if old.Status != "superseded" || old.SupersededBy != next.ID || old.Input.Source != p.Input.Source || old.Digest() == p.Digest() {
		t.Fatal(old)
	}
	if _, err := db.DecideChangeProposal(ctx, p.ID, p.Digest(), true, "reviewer", "html"); err == nil {
		t.Fatal("old review approved replacement")
	}
	if retry, err := db.AmendChangeProposal(ctx, amend, g.ID, g.UserID, g.Username); err != nil || retry.ID != next.ID {
		t.Fatal("retry changed ID", retry, err)
	}
	if _, err := db.GetLatest(ctx, "ko", techmark.Slugify("새 문서")); !errors.Is(err, storage.ErrDocumentNotFound) {
		t.Fatal("amendment applied", err)
	}
	if _, err := db.DecideChangeProposal(ctx, next.ID, next.Digest(), true, "reviewer", "html"); err != nil {
		t.Fatal(err)
	}
	current, err := db.GetLatest(ctx, "ko", techmark.Slugify("새 문서"))
	if err != nil || current.Source != next.Input.Source {
		t.Fatal(current, err)
	}
	amend.ProposalID = next.ID
	if _, err := db.AmendChangeProposal(ctx, amend, g.ID, g.UserID, g.Username); err == nil {
		t.Fatal("applied proposal amended")
	}
	if err := db.SetProposalPolicy(ctx, changeproposal.Policy{RetentionDays: 1}, "reviewer"); err != nil {
		t.Fatal(err)
	}
	short, _ := db.GetChangeProposal(ctx, next.ID)
	if short.ExpiresAt.After(p.CreatedAt.Add(24 * time.Hour)) {
		t.Fatal("amendment extended retention")
	}
}

func TestAmendmentOwnershipAndSnapshotsRemainProtected(t *testing.T) {
	db, _, g := proposalFixture(t)
	ctx := context.Background()
	doc, err := db.Save(ctx, document.Document{Locale: "ko", Title: "수정 대상", Slug: techmark.Slugify("수정 대상"), Source: "original"})
	if err != nil {
		t.Fatal(err)
	}
	p := submitProposal(t, db, g, changeproposal.Input{Kind: "edit", Locale: "ko", Name: doc.Title, BaseRevision: doc.Revision, Source: "proposed", Summary: "수정"})
	amend := changeproposal.Amendment{ProposalID: p.ID, Source: "corrected", Summary: "정정"}
	for _, who := range [][3]string{{"other", g.UserID, g.Username}, {g.ID, "other", g.Username}, {g.ID, g.UserID, "other"}} {
		if _, err := db.AmendChangeProposal(ctx, amend, who[0], who[1], who[2]); !errors.Is(err, changeproposal.ErrNotFound) {
			t.Fatal("foreign amendment", err)
		}
	}
	if _, err := db.Save(ctx, doc); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AmendChangeProposal(ctx, amend, g.ID, g.UserID, g.Username); !errors.Is(err, changeproposal.ErrConflict) {
		t.Fatal("silently rebased amendment", err)
	}
	old, _ := db.GetChangeProposal(ctx, p.ID)
	if old.Status != "pending" {
		t.Fatal("failed amendment changed old status")
	}
}

func TestApprovalAndAmendmentRaceCannotApplyUnreviewedSource(t *testing.T) {
	db, _, g := proposalFixture(t)
	ctx := context.Background()
	for i := 0; i < 8; i++ {
		p := submitProposal(t, db, g, changeproposal.Input{Kind: "create", Locale: "ko", Name: string(rune('A' + i)), Source: "reviewed", Summary: "create"})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = db.DecideChangeProposal(ctx, p.ID, p.Digest(), true, "reviewer", "html")
		}()
		go func() {
			defer wg.Done()
			_, _ = db.AmendChangeProposal(ctx, changeproposal.Amendment{ProposalID: p.ID, Source: "unreviewed", Summary: "amend"}, g.ID, g.UserID, g.Username)
		}()
		wg.Wait()
		doc, err := db.GetLatest(ctx, "ko", techmark.Slugify(p.Input.Name))
		if err == nil && doc.Source != "reviewed" {
			t.Fatal("unreviewed amendment committed", doc.Source)
		}
		if err != nil && !errors.Is(err, storage.ErrDocumentNotFound) {
			t.Fatal(err)
		}
	}
}

func TestRejectedRevokedAndInvalidAmendmentsDoNotReplacePendingRecords(t *testing.T) {
	for _, mode := range []string{"rejected", "revoked", "privileged", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			db, service, g := proposalFixture(t)
			ctx := context.Background()
			p := submitProposal(t, db, g, changeproposal.Input{Kind: "create", Locale: "ko", Name: "제안", Source: "original proposed", Summary: "create"})
			amend := changeproposal.Amendment{ProposalID: p.ID, Source: "corrected", Summary: "correct"}
			switch mode {
			case "rejected":
				if _, err := db.DecideChangeProposal(ctx, p.ID, p.Digest(), false, "reviewer", ""); err != nil {
					t.Fatal(err)
				}
			case "revoked":
				if err := service.Revoke(ctx, g.ID, oauth.Actor{ID: g.UserID, Username: g.Username}); err != nil {
					t.Fatal(err)
				}
			case "privileged":
				amend.Source = "<wiki-recent-changes>"
			case "duplicate":
				submitProposal(t, db, g, changeproposal.Input{Kind: "create", Locale: "ko", Name: "제안", Source: amend.Source, Summary: amend.Summary})
			}
			if _, err := db.AmendChangeProposal(ctx, amend, g.ID, g.UserID, g.Username); err == nil {
				t.Fatal("invalid amendment accepted")
			}
			stored, err := db.GetChangeProposal(ctx, p.ID)
			if err != nil || stored.Input.Source != p.Input.Source || stored.SupersededBy != "" {
				t.Fatal(stored, err)
			}
			values, err := db.ListChangeProposals(ctx)
			want := 1
			if mode == "duplicate" {
				want = 2
			}
			if err != nil || len(values) != want {
				t.Fatal("partial mutation", len(values), err)
			}
		})
	}
}
