package storage_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"techpedia/internal/changeproposal"
	"techpedia/internal/document"
	"techpedia/internal/edition"
	"techpedia/internal/oauth"
	"techpedia/internal/storage"
	"techpedia/internal/techdata"
	"techpedia/internal/techmark"
)

func TestProposalIntakeHasNoAccountOrRetainedQueueCaps(t *testing.T) {
	db, _, g := proposalFixture(t)
	ctx := context.Background()
	for i := range 150 {
		p := submitProposal(t, db, g, changeproposal.Input{Kind: "create", Locale: "ko", Name: fmt.Sprintf("문서 %d", i), Source: "source", Summary: "create"})
		// Leave more than the previous per-account/global pending limits waiting.
		if i >= 70 {
			if _, err := db.DecideChangeProposal(ctx, p.ID, p.Digest(), false, "reviewer", ""); err != nil {
				t.Fatal(err)
			}
		}
	}
	values, err := db.ListChangeProposals(ctx)
	if err != nil || len(values) != 150 {
		t.Fatal("retained queue blocked intake", len(values), err)
	}
	for _, summary := range values {
		if summary.Before != "" || summary.Input.Source != "" {
			t.Fatal("queue loaded full source")
		}
	}
	if _, err := db.SubmitChangeProposal(ctx, changeproposal.Input{Kind: "create", Locale: "fr", Name: "Encore", Source: "text", Summary: "create"}, g.ID, g.UserID, g.Username); err != nil {
		t.Fatal(err)
	}
}

func TestReviewedSourceBudgetDoesNotBlockIntake(t *testing.T) {
	db, _, g := proposalFixture(t)
	ctx := context.Background()
	source := strings.Repeat("a", changeproposal.MaxSourceBytes)
	for i := range 33 {
		p := submitProposal(t, db, g, changeproposal.Input{Kind: "create", Locale: "ko", Name: fmt.Sprintf("긴 문서 %d", i), Source: source, Summary: "create"})
		if _, err := db.DecideChangeProposal(ctx, p.ID, p.Digest(), false, "reviewer", ""); err != nil {
			t.Fatal(err)
		}
	}
	submitProposal(t, db, g, changeproposal.Input{Kind: "create", Locale: "ko", Name: "추가 문서", Source: "a", Summary: "create"})
}

func TestProposalRetentionIsShortConfigurableAndNeverRevivesOldSnapshots(t *testing.T) {
	db, _, g := proposalFixture(t)
	ctx := context.Background()
	policy, err := db.GetProposalPolicy(ctx)
	if err != nil || policy.RetentionDays != 2 {
		t.Fatal(policy, err)
	}
	p := submitProposal(t, db, g, changeproposal.Input{Kind: "create", Locale: "ko", Name: "기존 제안", Source: "source", Summary: "create"})
	if p.ExpiresAt.Sub(p.CreatedAt) != 48*time.Hour {
		t.Fatal(p.ExpiresAt)
	}
	for _, days := range []int{0, 4, -1} {
		if err := db.SetProposalPolicy(ctx, changeproposal.Policy{RetentionDays: days}, "reviewer"); !errors.Is(err, changeproposal.ErrInvalid) {
			t.Fatal(days, err)
		}
	}
	if err := db.SetProposalPolicy(ctx, changeproposal.Policy{RetentionDays: 1}, "reviewer"); err != nil {
		t.Fatal(err)
	}
	short, err := db.GetChangeProposal(ctx, p.ID)
	if err != nil || short.ExpiresAt.Sub(short.CreatedAt) != 24*time.Hour {
		t.Fatal(short, err)
	}
	if _, err := db.DecideChangeProposal(ctx, p.ID, p.Digest(), true, "reviewer", "html"); !errors.Is(err, changeproposal.ErrConflict) {
		t.Fatal("changed retention reused an old review digest", err)
	}
	for _, days := range []int{2, 3} {
		if err := db.SetProposalPolicy(ctx, changeproposal.Policy{RetentionDays: days}, "reviewer"); err != nil {
			t.Fatal(err)
		}
		newProposal := submitProposal(t, db, g, changeproposal.Input{Kind: "create", Locale: "ko", Name: fmt.Sprintf("기간 %d", days), Source: "source", Summary: "create"})
		if newProposal.ExpiresAt.Sub(newProposal.CreatedAt) != time.Duration(days)*24*time.Hour {
			t.Fatal(days, newProposal.ExpiresAt)
		}
	}
	stillShort, err := db.GetChangeProposal(ctx, p.ID)
	if err != nil || stillShort.ExpiresAt != short.ExpiresAt {
		t.Fatal("retention extended an old snapshot", err)
	}
}

func TestParallelProposalIntakeAndDuplicateRetry(t *testing.T) {
	db, _, g := proposalFixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := db.SubmitChangeProposal(ctx, changeproposal.Input{Kind: "create", Locale: "ko", Name: fmt.Sprintf("병렬 %d", i%4), Source: strings.Repeat("a", 128<<10), Summary: "create"}, g.ID, g.UserID, g.Username)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	values, err := db.ListChangeProposals(ctx)
	if err != nil || len(values) != 4 {
		t.Fatal("parallel duplicates not coalesced", len(values), err)
	}
}

func proposalFixture(t *testing.T) (*storage.BadgerDocumentStore, *oauth.Service, oauth.Grant) {
	t.Helper()
	db, err := storage.OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	for _, locale := range []string{"ko", "en", "fr"} {
		if _, err := db.ProposeEdition(ctx, edition.Edition{Tag: locale}); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ApproveEdition(ctx, locale, "admin", time.Now()); err != nil {
			t.Fatal(err)
		}
		messages := map[string]string{}
		for _, key := range edition.InterfaceMessageKeys() {
			messages[key] = "configured " + key
		}
		if _, err := db.UpdateInterfaceMessages(ctx, locale, messages); err != nil {
			t.Fatal(err)
		}
		if _, err := db.UpdateNavigation(ctx, locale, []edition.NavigationSection{{Title: "Navigation", Items: []edition.NavigationItem{{Label: "Main", Target: "Main"}}}}); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ActivateEdition(ctx, locale, "Main", time.Now()); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Save(ctx, document.Document{Locale: locale, Slug: techmark.Slugify("Main"), Title: "Main", Source: "main"}); err != nil {
			t.Fatal(err)
		}
	}
	s := oauth.New(db, "https://wiki.example")
	actor := oauth.Actor{ID: "admin-id", Username: "admin"}
	c, err := s.Register(ctx, "test", "https://chatgpt.example/callback", actor)
	if err != nil {
		t.Fatal(err)
	}
	verifier := strings.Repeat("v", 43)
	sum := sha256.Sum256([]byte(verifier))
	a := oauth.Authorization{ClientID: c.ID, RedirectURI: c.RedirectURI, ResponseType: "code", Challenge: base64.RawURLEncoding.EncodeToString(sum[:]), ChallengeMethod: "S256", Scope: oauth.ProposalScopes, Resource: "https://wiki.example/mcp"}
	p, _, err := s.Prepare(ctx, a, actor)
	if err != nil {
		t.Fatal(err)
	}
	callback, err := s.Decide(ctx, p.ID, actor, true)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(callback)
	tokens, err := s.Exchange(ctx, "authorization_code", u.Query().Get("code"), c.ID, c.RedirectURI, verifier, a.Resource, "", func(oauth.Grant) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	g, err := s.Access(ctx, tokens.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	return db, s, g
}
func submitProposal(t *testing.T, db changeproposal.Store, g oauth.Grant, in changeproposal.Input) changeproposal.Proposal {
	t.Helper()
	p, err := db.SubmitChangeProposal(context.Background(), in, g.ID, g.UserID, g.Username)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLargeProposalsKeepExactSourceAndHistory(t *testing.T) {
	db, _, g := proposalFixture(t)
	ctx := context.Background()
	// JSON escaping and XML encoding must not make an accepted text limit fail.
	source := strings.Repeat("<>&", changeproposal.MaxSourceBytes/3)
	in := changeproposal.Input{Kind: "create", Locale: "ko", Name: "긴 문서", Source: source, Summary: "long"}
	for i := 0; i < 3; i++ {
		p := submitProposal(t, db, g, in)
		if _, err := db.DecideChangeProposal(ctx, p.ID, p.Digest(), true, "reviewer", "safe html"); err != nil {
			t.Fatal(err)
		}
		doc, err := db.GetLatest(ctx, "ko", techmark.Slugify(in.Name))
		if err != nil || doc.Source != in.Source {
			t.Fatal("large source changed", err)
		}
		in.Kind, in.BaseRevision, in.Source = "edit", doc.Revision, source+strings.Repeat("a", i+1)
	}
	revisions, err := db.ListRevisions(ctx, "ko", techmark.Slugify(in.Name), 10)
	if err != nil || len(revisions) != 3 {
		t.Fatal("large revisions missing", err)
	}
	old, err := db.GetRevision(ctx, "ko", techmark.Slugify(in.Name), "r1")
	if err != nil || old.Source != source {
		t.Fatal("original large revision changed", err)
	}
}
func saveProposalPage(t *testing.T, db storage.DocumentStore, locale, name, source string) document.Document {
	t.Helper()
	doc, err := db.Save(context.Background(), document.Document{Locale: locale, Slug: techmark.Slugify(name), Title: name, Source: source})
	if err != nil {
		t.Fatal(err)
	}
	return doc
}
func TestProposalCreationReviewAndEditAreAtomicAndOneUse(t *testing.T) {
	db, _, g := proposalFixture(t)
	ctx := context.Background()
	in := changeproposal.Input{Kind: "create", Locale: "ko", Name: "테스트", Source: "# 테스트\n원문", Summary: "새 문서"}
	p := submitProposal(t, db, g, in)
	if _, err := db.GetLatest(ctx, "ko", techmark.Slugify(in.Name)); !errors.Is(err, storage.ErrDocumentNotFound) {
		t.Fatal("submission mutated a document", err)
	}
	retry := submitProposal(t, db, g, in)
	if retry.ID != p.ID {
		t.Fatal("duplicate submission was not idempotent")
	}
	if _, err := db.DecideChangeProposal(ctx, p.ID, "wrong", true, "reviewer", "html"); !errors.Is(err, changeproposal.ErrConflict) {
		t.Fatal(err)
	}
	if _, err := db.DecideChangeProposal(ctx, p.ID, p.Digest(), true, "reviewer", "safe html"); err != nil {
		t.Fatal(err)
	}
	doc, err := db.GetLatest(ctx, "ko", techmark.Slugify(in.Name))
	if err != nil || doc.Revision != "r1" || doc.Source != in.Source || doc.Author != "reviewer" {
		t.Fatalf("saved %+v %v", doc, err)
	}
	if _, err := db.DecideChangeProposal(ctx, p.ID, p.Digest(), true, "reviewer", "safe html"); !errors.Is(err, changeproposal.ErrUnavailable) {
		t.Fatal("replayed approval", err)
	}
	in.Kind, in.BaseRevision, in.Source = "edit", doc.Revision, "# 테스트\n새 본문"
	edit := submitProposal(t, db, g, in)
	if edit.Before != doc.Source {
		t.Fatal("wrong diff base")
	}
	if _, err := db.DecideChangeProposal(ctx, edit.ID, edit.Digest(), true, "reviewer", "new html"); err != nil {
		t.Fatal(err)
	}
	latest, _ := db.GetLatest(ctx, doc.Locale, doc.Slug)
	if latest.ID != doc.ID || latest.CreatedAt != doc.CreatedAt || latest.Revision != "r2" {
		t.Fatal("lost identity/history", latest)
	}
	old, err := db.GetRevision(ctx, doc.Locale, doc.Slug, "r1")
	if err != nil || old.Source != doc.Source {
		t.Fatal("lost original", err)
	}
	audits, err := db.ListAudit(ctx, document.AuditFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range audits {
		if strings.Contains(a.Detail, "본문") || strings.Contains(a.Detail, "원문") {
			t.Fatal("source copied into audit")
		}
	}
}
func TestProposalRejectConflictRevocationAndPrivacyReset(t *testing.T) {
	for _, mode := range []string{"reject", "edit", "move", "delete-recreate", "reset", "revoke", "disable", "policy"} {
		t.Run(mode, func(t *testing.T) {
			db, s, g := proposalFixture(t)
			ctx := context.Background()
			doc := saveProposalPage(t, db, "en", "Example", "old source")
			p := submitProposal(t, db, g, changeproposal.Input{Kind: "edit", Locale: doc.Locale, Name: doc.Title, BaseRevision: doc.Revision, Source: "new source", Summary: "update"})
			switch mode {
			case "reject":
				if _, err := db.DecideChangeProposal(ctx, p.ID, p.Digest(), false, "reviewer", ""); err != nil {
					t.Fatal(err)
				}
				got, _ := db.GetLatest(ctx, doc.Locale, doc.Slug)
				if got.Source != doc.Source {
					t.Fatal("rejection mutated")
				}
				return
			case "edit":
				doc.Source = "concurrent"
				if _, err := db.Save(ctx, doc); err != nil {
					t.Fatal(err)
				}
			case "move":
				if _, err := db.MoveDocumentAs(ctx, doc.Locale, doc.Slug, techmark.Slugify("Moved"), "Moved", "owner"); err != nil {
					t.Fatal(err)
				}
			case "delete-recreate":
				if _, err := db.DeleteDocument(ctx, doc.Locale, doc.Slug, "owner", ""); err != nil {
					t.Fatal(err)
				}
				saveProposalPage(t, db, doc.Locale, doc.Title, doc.Source)
			case "reset":
				if _, err := db.ResetDocumentHistory(ctx, doc.Locale, doc.Slug, doc.Revision, "owner"); err != nil {
					t.Fatal(err)
				}
				if _, err := db.GetChangeProposal(ctx, p.ID); !errors.Is(err, changeproposal.ErrNotFound) {
					t.Fatal("privacy snapshot survived", err)
				}
			case "revoke":
				if err := s.Revoke(ctx, g.ID, oauth.Actor{ID: g.UserID, Username: g.Username}); err != nil {
					t.Fatal(err)
				}
			case "disable":
				if err := s.Disable(ctx, g.ClientID, oauth.Actor{ID: g.UserID, Username: g.Username}); err != nil {
					t.Fatal(err)
				}
			case "policy":
				doc.EditPolicy = document.EditPolicyAdmins
				if _, err := db.Save(ctx, doc); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.DecideChangeProposal(ctx, p.ID, p.Digest(), true, "reviewer", "html"); err == nil {
				t.Fatal("stale approval succeeded")
			}
		})
	}
}
func TestProposalLinksForwardReverseAndPreventsOverwrite(t *testing.T) {
	for _, seed := range []string{"none", "source", "target", "both", "occupied", "changed"} {
		t.Run(seed, func(t *testing.T) {
			db, _, g := proposalFixture(t)
			ctx := context.Background()
			a := saveProposalPage(t, db, "ko", "시스템", "ko")
			b := saveProposalPage(t, db, "en", "System", "en")
			if seed == "source" || seed == "both" || seed == "occupied" || seed == "changed" {
				pages := map[string]document.WID{"ko": a.ID}
				if seed == "occupied" {
					other := saveProposalPage(t, db, "en", "Other", "other")
					pages["en"] = other.ID
				}
				if _, err := db.CreateItem(ctx, techdata.Item{Labels: map[string]string{"ko": a.Title}, WikiPages: pages}); err != nil {
					t.Fatal(err)
				}
			}
			if seed == "target" || seed == "both" {
				if _, err := db.CreateItem(ctx, techdata.Item{Labels: map[string]string{"en": b.Title}, WikiPages: map[string]document.WID{"en": b.ID}}); err != nil {
					t.Fatal(err)
				}
			}
			in := changeproposal.Input{Kind: "link", Locale: a.Locale, Name: a.Title, BaseRevision: a.Revision, Summary: "same concept", Target: &changeproposal.Page{Locale: b.Locale, Name: b.Title, Revision: b.Revision}}
			p, err := db.SubmitChangeProposal(ctx, in, g.ID, g.UserID, g.Username)
			if seed == "both" || seed == "occupied" {
				if !errors.Is(err, changeproposal.ErrConflict) {
					t.Fatal("overwrite accepted", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if seed == "changed" {
				item, _ := db.GetItemByWikiPage(ctx, a.ID)
				other := saveProposalPage(t, db, "fr", "Système", "fr")
				if _, err := db.AttachWikiPage(ctx, item.ID, other.Locale, other.ID, other.Title); err != nil {
					t.Fatal(err)
				}
				if _, err := db.DecideChangeProposal(ctx, p.ID, p.Digest(), true, "reviewer", ""); !errors.Is(err, changeproposal.ErrConflict) {
					t.Fatal("changed link graph applied", err)
				}
				return
			}
			if _, err := db.DecideChangeProposal(ctx, p.ID, p.Digest(), true, "reviewer", ""); err != nil {
				t.Fatal(err)
			}
			left, err := db.GetItemByWikiPage(ctx, a.ID)
			if err != nil {
				t.Fatal(err)
			}
			right, err := db.GetItemByWikiPage(ctx, b.ID)
			if err != nil {
				t.Fatal(err)
			}
			if left.ID != right.ID || left.WikiPages[a.Locale] != a.ID || left.WikiPages[b.Locale] != b.ID {
				t.Fatal("broken link invariant")
			}
			current, _ := db.GetLatest(ctx, a.Locale, a.Slug)
			if current.Revision != a.Revision {
				t.Fatal("link created content revision")
			}
		})
	}
}
func TestProposalParallelDecisionAppliesOnce(t *testing.T) {
	db, _, g := proposalFixture(t)
	ctx := context.Background()
	p := submitProposal(t, db, g, changeproposal.Input{Kind: "create", Locale: "fr", Name: "Article", Source: "text", Summary: "create"})
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := db.DecideChangeProposal(ctx, p.ID, p.Digest(), true, "reviewer", "html")
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("parallel approval count", success)
	}
	doc, _ := db.GetLatest(ctx, "fr", techmark.Slugify("Article"))
	if doc.Revision != "r1" {
		t.Fatal("applied twice", doc)
	}
}
