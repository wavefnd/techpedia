package storage

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/dgraph-io/badger/v4"
	"techpedia/internal/changeproposal"
	"techpedia/internal/document"
	"techpedia/internal/edition"
	"techpedia/internal/oauth"
	"techpedia/internal/techdata"
	"techpedia/internal/techmark"
)

const proposalPrefix = "mcp:v1:proposal:"
const proposalPolicyKey = "mcp:v1:proposal-policy"
const proposalDuplicatePrefix = "mcp:v1:proposal-duplicate:"
const proposalSummaryPrefix = "mcp:v1:proposal-summary:"

func proposalDuplicateKey(p changeproposal.Proposal) []byte {
	raw, _ := json.Marshal(struct {
		GrantID      string
		Input        changeproposal.Input
		Base, Target changeproposal.Snapshot
	}{p.GrantID, p.Input, p.Base, p.Target})
	return []byte(proposalDuplicatePrefix + changeproposal.Hash(raw))
}

func proposalPolicyTxn(tx *badger.Txn) (changeproposal.Policy, error) {
	p := changeproposal.Policy{RetentionDays: changeproposal.DefaultRetentionDays}
	entry, err := tx.Get([]byte(proposalPolicyKey))
	if errors.Is(err, badger.ErrKeyNotFound) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	err = entry.Value(func(raw []byte) error { return json.Unmarshal(raw, &p) })
	return p, err
}

func (s *BadgerDocumentStore) GetProposalPolicy(ctx context.Context) (changeproposal.Policy, error) {
	var p changeproposal.Policy
	err := s.db.View(func(tx *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var err error
		p, err = proposalPolicyTxn(tx)
		return err
	})
	return p, err
}

// Rewrite records separately so large snapshots never form one oversized transaction.
// Shortening retention is irreversible; increasing it never revives expired proposals.
func (s *BadgerDocumentStore) capProposalRetention(ctx context.Context, days int, reindex bool) error {
	var ids []string
	err := s.db.View(func(tx *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix, opts.PrefetchValues = []byte(proposalPrefix), false
		it := tx.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			ids = append(ids, string(it.Item().KeyCopy(nil))[len(proposalPrefix):])
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, id := range ids {
		err = s.updateProposalTxn(ctx, func(tx *badger.Txn) error {
			p, err := proposalFromTxn(tx, id)
			if errors.Is(err, changeproposal.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			deadline := p.CreatedAt.Add(time.Duration(days) * 24 * time.Hour)
			changed := deadline.Before(p.ExpiresAt)
			if changed {
				p.ExpiresAt = deadline
			}
			if !time.Now().Before(p.ExpiresAt) {
				if err := deleteProposalDuplicateTxn(tx, p); err != nil {
					return err
				}
				if err := tx.Delete([]byte(proposalSummaryPrefix + id)); err != nil {
					return err
				}
				return tx.Delete([]byte(proposalPrefix + id))
			}
			if changed || reindex {
				return putProposalTxn(tx, p)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *BadgerDocumentStore) initializeProposalPolicy() error {
	policy, err := s.GetProposalPolicy(context.Background())
	if err != nil {
		return err
	}
	return s.capProposalRetention(context.Background(), policy.RetentionDays, true)
}

func (s *BadgerDocumentStore) SetProposalPolicy(ctx context.Context, p changeproposal.Policy, actor string) error {
	if p.RetentionDays < 1 || p.RetentionDays > 3 || actor == "" {
		return changeproposal.ErrInvalid
	}
	s.proposalMu.Lock()
	defer s.proposalMu.Unlock()
	if err := s.capProposalRetention(ctx, p.RetentionDays, false); err != nil {
		return err
	}
	return s.updateProposalTxn(ctx, func(tx *badger.Txn) error {
		raw, err := json.Marshal(p)
		if err != nil {
			return err
		}
		if err := tx.Set([]byte(proposalPolicyKey), raw); err != nil {
			return err
		}
		return (oauthTx{ctx, tx}).Audit(oauth.Event{Actor: actor, Action: "mcp.proposal_policy", Result: "success"})
	})
}

func (s *BadgerDocumentStore) updateProposalTxn(ctx context.Context, work func(*badger.Txn) error) error {
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := s.db.Update(work)
		if !errors.Is(err, badger.ErrConflict) || attempt >= 7 {
			return err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func proposalFromTxn(tx *badger.Txn, id string) (changeproposal.Proposal, error) {
	var p changeproposal.Proposal
	if len(id) > 80 || id == "" {
		return p, changeproposal.ErrNotFound
	}
	entry, err := tx.Get([]byte(proposalPrefix + id))
	if errors.Is(err, badger.ErrKeyNotFound) {
		return p, changeproposal.ErrNotFound
	}
	if err != nil {
		return p, err
	}
	err = entry.Value(func(raw []byte) error { return json.Unmarshal(raw, &p) })
	return p, err
}
func putProposalTxn(tx *badger.Txn, p changeproposal.Proposal) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	entry := badger.NewEntry([]byte(proposalPrefix+p.ID), raw)
	entry.ExpiresAt = uint64(p.ExpiresAt.Unix())
	if err := tx.SetEntry(entry); err != nil {
		return err
	}
	summary := p
	summary.Before, summary.Input.Source = "", ""
	raw, err = json.Marshal(summary)
	if err != nil {
		return err
	}
	metadata := badger.NewEntry([]byte(proposalSummaryPrefix+p.ID), raw)
	metadata.ExpiresAt = entry.ExpiresAt
	if err := tx.SetEntry(metadata); err != nil {
		return err
	}
	if p.Status == "pending" {
		index := badger.NewEntry(proposalDuplicateKey(p), []byte(p.ID))
		index.ExpiresAt = entry.ExpiresAt
		return tx.SetEntry(index)
	}
	return deleteProposalDuplicateTxn(tx, p)
}

func deleteProposalDuplicateTxn(tx *badger.Txn, p changeproposal.Proposal) error {
	key := proposalDuplicateKey(p)
	entry, err := tx.Get(key)
	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	id, err := entry.ValueCopy(nil)
	if err != nil {
		return err
	}
	if string(id) != p.ID {
		return nil
	}
	return tx.Delete(key)
}
func listProposalsTxn(ctx context.Context, tx *badger.Txn) ([]changeproposal.Proposal, error) {
	return listProposalEntriesTxn(ctx, tx, proposalPrefix)
}
func listProposalEntriesTxn(ctx context.Context, tx *badger.Txn, prefix string) ([]changeproposal.Proposal, error) {
	out := []changeproposal.Proposal{}
	opts := badger.DefaultIteratorOptions
	opts.Prefix = []byte(prefix)
	it := tx.NewIterator(opts)
	defer it.Close()
	for it.Rewind(); it.Valid(); it.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var p changeproposal.Proposal
		if err := it.Item().Value(func(raw []byte) error { return json.Unmarshal(raw, &p) }); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}
func activeProposalEdition(tx *badger.Txn, locale string) error {
	entry, err := tx.Get([]byte(editionKey(locale)))
	if errors.Is(err, badger.ErrKeyNotFound) {
		return changeproposal.ErrInvalid
	}
	if err != nil {
		return err
	}
	var e edition.Edition
	if err := entry.Value(func(raw []byte) error { return json.Unmarshal(raw, &e) }); err != nil {
		return err
	}
	if !e.Active() {
		return changeproposal.ErrUnavailable
	}
	if _, err := documentFromTxn(tx, latestDocumentKey(locale, techmark.Slugify(e.MainPageName))); err != nil {
		return changeproposal.ErrUnavailable
	}
	return nil
}
func proposalGrantTxn(ctx context.Context, tx *badger.Txn, p changeproposal.Proposal) error {
	var g oauth.Grant
	t := oauthTx{ctx, tx}
	if err := t.Get("grant:"+p.GrantID, &g); err != nil {
		return changeproposal.ErrUnavailable
	}
	var c oauth.Client
	if err := t.Get("client:"+g.ClientID, &c); err != nil {
		return changeproposal.ErrUnavailable
	}
	if g.Revoked || c.Disabled || !time.Now().Before(g.ExpiresAt) || !oauth.CanPropose(g.Scope) || g.UserID != p.UserID || g.Username != p.Username {
		return changeproposal.ErrUnavailable
	}
	return nil
}
func proposalItemTxn(tx *badger.Txn, wid document.WID) (techdata.Item, error) {
	entry, err := tx.Get(techdataWikiKey(wid))
	if errors.Is(err, badger.ErrKeyNotFound) {
		return techdata.Item{}, nil
	}
	if err != nil {
		return techdata.Item{}, err
	}
	raw, err := entry.ValueCopy(nil)
	if err != nil {
		return techdata.Item{}, err
	}
	id, err := techdata.ParseTID(string(raw))
	if err != nil {
		return techdata.Item{}, err
	}
	return readTechdataItem(tx, id)
}
func proposalSnapshotTxn(tx *badger.Txn, page changeproposal.Page) (document.Document, changeproposal.Snapshot, error) {
	doc, err := documentFromTxn(tx, latestDocumentKey(page.Locale, techmark.Slugify(page.Name)))
	if err != nil {
		return doc, changeproposal.Snapshot{}, err
	}
	if len(doc.Source) > changeproposal.MaxSourceBytes {
		return doc, changeproposal.Snapshot{}, changeproposal.ErrSourceTooLarge
	}
	if doc.Title != page.Name || doc.Revision != page.Revision || doc.EditPolicy == document.EditPolicyAdmins {
		return doc, changeproposal.Snapshot{}, changeproposal.ErrConflict
	}
	item, err := proposalItemTxn(tx, doc.ID)
	if err != nil {
		return doc, changeproposal.Snapshot{}, err
	}
	data, _ := json.Marshal(doc)
	links, _ := json.Marshal(item)
	return doc, changeproposal.Snapshot{WID: doc.ID, Fingerprint: changeproposal.Hash(data), LinksFingerprint: changeproposal.Hash(links)}, nil
}

func (s *BadgerDocumentStore) SubmitChangeProposal(ctx context.Context, in changeproposal.Input, grantID, userID, username string) (changeproposal.Proposal, error) {
	if err := changeproposal.Validate(in); err != nil {
		return changeproposal.Proposal{}, err
	}
	if grantID == "" || userID == "" || username == "" {
		return changeproposal.Proposal{}, changeproposal.ErrInvalid
	}
	now := time.Now().UTC()
	p := changeproposal.Proposal{ID: orderedWorkflowID(now), Input: in, GrantID: grantID, UserID: userID, Username: username, Status: "pending", CreatedAt: now}
	template := p
	s.proposalMu.Lock()
	defer s.proposalMu.Unlock()
	err := s.updateProposalTxn(ctx, func(tx *badger.Txn) error {
		p = template
		policy, err := proposalPolicyTxn(tx)
		if err != nil {
			return err
		}
		p.ExpiresAt = now.Add(time.Duration(policy.RetentionDays) * 24 * time.Hour)
		if err := proposalGrantTxn(ctx, tx, p); err != nil {
			return err
		}
		if err := prepareProposalTxn(tx, &p); err != nil {
			return err
		}
		entry, err := tx.Get(proposalDuplicateKey(p))
		if err == nil {
			raw, err := entry.ValueCopy(nil)
			if err != nil {
				return err
			}
			existing, err := proposalFromTxn(tx, string(raw))
			if err == nil && existing.Status == "pending" {
				p = existing
				return nil
			}
			if err != nil && !errors.Is(err, changeproposal.ErrNotFound) {
				return err
			}
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		if err := putProposalTxn(tx, p); err != nil {
			return err
		}
		return (oauthTx{ctx, tx}).Audit(oauth.Event{Actor: username, Action: "mcp.proposal_submit", Target: p.ID, Result: "success"})
	})
	return p, err
}

// Materialize exact patches and all review snapshots in the submission transaction.
func prepareProposalTxn(tx *badger.Txn, p *changeproposal.Proposal) error {
	in := p.Input
	if err := activeProposalEdition(tx, in.Locale); err != nil {
		return err
	}
	if in.Kind == "create" {
		if _, err := documentFromTxn(tx, latestDocumentKey(in.Locale, techmark.Slugify(in.Name))); err == nil {
			return ErrDocumentExists
		} else if !errors.Is(err, ErrDocumentNotFound) {
			return err
		}
		if exists, err := documentRedirectExistsTxn(tx, in.Locale, techmark.Slugify(in.Name)); err != nil {
			return err
		} else if exists {
			return ErrDocumentExists
		}
	} else {
		doc, snap, err := proposalSnapshotTxn(tx, changeproposal.Page{Locale: in.Locale, Name: in.Name, Revision: in.BaseRevision})
		if err != nil {
			return err
		}
		p.Base, p.Before = snap, doc.Source
		if len(in.Edits) != 0 {
			in.Source, err = changeproposal.ApplyEdits(doc.Source, in.Edits)
			if err != nil {
				return err
			}
			in.Edits = nil
		}
		if in.Kind == "edit" && doc.Source == in.Source {
			return changeproposal.ErrInvalid
		}
	}
	p.Input = in
	if err := changeproposal.Validate(in); err != nil {
		return err
	}
	if in.Kind == "link" {
		if err := activeProposalEdition(tx, in.Target.Locale); err != nil {
			return err
		}
		_, snap, err := proposalSnapshotTxn(tx, *in.Target)
		if err != nil {
			return err
		}
		p.Target = snap
		if err := checkProposalLinkTxn(tx, *p); err != nil {
			return err
		}
		p.Before = ""
	}
	return nil
}

func (s *BadgerDocumentStore) AmendChangeProposal(ctx context.Context, amendment changeproposal.Amendment, grantID, userID, username string) (changeproposal.Proposal, error) {
	var result changeproposal.Proposal
	if grantID == "" || userID == "" || username == "" || (amendment.Source == "" && len(amendment.Edits) == 0) || (amendment.Source != "" && len(amendment.Edits) != 0) {
		return result, changeproposal.ErrInvalid
	}
	if len(amendment.Source) > changeproposal.MaxSourceBytes {
		return result, changeproposal.ErrSourceTooLarge
	}
	if len(amendment.Edits) != 0 {
		if err := changeproposal.ValidateEdits(amendment.Edits); err != nil {
			return result, err
		}
	}
	now := time.Now().UTC()
	id := orderedWorkflowID(now)
	s.proposalMu.Lock()
	defer s.proposalMu.Unlock()
	err := s.updateProposalTxn(ctx, func(tx *badger.Txn) error {
		old, err := proposalFromTxn(tx, amendment.ProposalID)
		if err != nil {
			return err
		}
		if old.GrantID != grantID || old.UserID != userID || old.Username != username {
			return changeproposal.ErrNotFound
		}
		if err := proposalGrantTxn(ctx, tx, old); err != nil {
			return err
		}
		if !time.Now().Before(old.ExpiresAt) || (old.Input.Kind != "create" && old.Input.Kind != "edit") {
			return changeproposal.ErrUnavailable
		}
		in := old.Input
		in.Source, in.Edits, in.Summary = amendment.Source, nil, amendment.Summary
		if len(amendment.Edits) != 0 {
			in.Source, err = changeproposal.ApplyEdits(old.Input.Source, amendment.Edits)
			if err != nil {
				return err
			}
		}
		if err := changeproposal.Validate(in); err != nil {
			return err
		}
		if old.Status == "superseded" {
			next, err := proposalFromTxn(tx, old.SupersededBy)
			if err == nil && next.Input.Source == in.Source && next.Input.Summary == in.Summary {
				result = next
				return nil
			}
			return changeproposal.ErrConflict
		}
		if old.Status != "pending" {
			return changeproposal.ErrUnavailable
		}
		if in.Source == old.Input.Source && in.Summary == old.Input.Summary {
			result = old
			return nil
		}
		result = changeproposal.Proposal{ID: id, Input: in, GrantID: grantID, UserID: userID, Username: username, Status: "pending", CreatedAt: old.CreatedAt, ExpiresAt: old.ExpiresAt, Supersedes: old.ID, AmendedAt: now}
		if err := prepareProposalTxn(tx, &result); err != nil {
			return err
		}
		if result.Base != old.Base || result.Target != old.Target {
			return changeproposal.ErrConflict
		}
		if entry, err := tx.Get(proposalDuplicateKey(result)); err == nil {
			raw, err := entry.ValueCopy(nil)
			if err != nil {
				return err
			}
			if string(raw) != old.ID {
				return changeproposal.ErrConflict
			}
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		old.Status, old.SupersededBy = "superseded", result.ID
		if err := putProposalTxn(tx, old); err != nil {
			return err
		}
		if err := putProposalTxn(tx, result); err != nil {
			return err
		}
		return (oauthTx{ctx, tx}).Audit(oauth.Event{Actor: username, Action: "mcp.proposal_amend", Target: result.ID, Result: "success"})
	})
	return result, err
}
func checkProposalLinkTxn(tx *badger.Txn, p changeproposal.Proposal) error {
	a, err := proposalItemTxn(tx, p.Base.WID)
	if err != nil {
		return err
	}
	b, err := proposalItemTxn(tx, p.Target.WID)
	if err != nil {
		return err
	}
	if a.ID != "" && b.ID != "" {
		return changeproposal.ErrConflict
	}
	if a.ID != "" && a.WikiPages[p.Input.Target.Locale] != "" {
		return changeproposal.ErrConflict
	}
	if b.ID != "" && b.WikiPages[p.Input.Locale] != "" {
		return changeproposal.ErrConflict
	}
	return nil
}
func (s *BadgerDocumentStore) GetChangeProposal(ctx context.Context, id string) (changeproposal.Proposal, error) {
	var p changeproposal.Proposal
	err := s.db.View(func(tx *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var err error
		p, err = proposalFromTxn(tx, id)
		return err
	})
	return p, err
}
func (s *BadgerDocumentStore) ListChangeProposals(ctx context.Context) ([]changeproposal.Proposal, error) {
	var out []changeproposal.Proposal
	err := s.db.View(func(tx *badger.Txn) error {
		var err error
		out, err = listProposalEntriesTxn(ctx, tx, proposalSummaryPrefix)
		return err
	})
	return out, err
}
func (s *BadgerDocumentStore) DecideChangeProposal(ctx context.Context, id, digest string, allow bool, actor, html string) (changeproposal.Proposal, error) {
	var p changeproposal.Proposal
	err := s.db.Update(func(tx *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var err error
		p, err = proposalFromTxn(tx, id)
		if err != nil {
			return err
		}
		if p.Status != "pending" || !time.Now().Before(p.ExpiresAt) {
			return changeproposal.ErrUnavailable
		}
		if actor == "" || digest != p.Digest() {
			return changeproposal.ErrConflict
		}
		p.Status = "rejected"
		if allow {
			if err := proposalGrantTxn(ctx, tx, p); err != nil {
				return err
			}
			if err := changeproposal.Validate(p.Input); err != nil {
				return err
			}
			if err := activeProposalEdition(tx, p.Input.Locale); err != nil {
				return err
			}
			var doc document.Document
			if p.Input.Kind == "create" {
				if _, err := documentFromTxn(tx, latestDocumentKey(p.Input.Locale, techmark.Slugify(p.Input.Name))); err == nil {
					return changeproposal.ErrConflict
				} else if !errors.Is(err, ErrDocumentNotFound) {
					return err
				}
				doc = document.Document{Locale: p.Input.Locale, Slug: techmark.Slugify(p.Input.Name), Title: p.Input.Name, EditPolicy: document.EditPolicyEditors}
			} else {
				var snap changeproposal.Snapshot
				doc, snap, err = proposalSnapshotTxn(tx, changeproposal.Page{Locale: p.Input.Locale, Name: p.Input.Name, Revision: p.Input.BaseRevision})
				if err != nil || snap != p.Base {
					return changeproposal.ErrConflict
				}
			}
			if p.Input.Kind == "link" {
				if err := activeProposalEdition(tx, p.Input.Target.Locale); err != nil {
					return err
				}
				target, snap, err := proposalSnapshotTxn(tx, *p.Input.Target)
				if err != nil || snap != p.Target {
					return changeproposal.ErrConflict
				}
				if err := checkProposalLinkTxn(tx, p); err != nil {
					return err
				}
				item, err := proposalItemTxn(tx, doc.ID)
				if err != nil {
					return err
				}
				if item.ID == "" {
					other, err := proposalItemTxn(tx, target.ID)
					if err != nil {
						return err
					}
					if other.ID != "" {
						doc, target = target, doc
					}
				}
				item, err = techdataItemForSourceTxn(ctx, tx, doc)
				if err != nil {
					return err
				}
				if _, err := attachWikiPageTxn(tx, item, target.Locale, target.ID, target.Title); err != nil {
					return err
				}
			} else {
				doc.Source, doc.HTML, doc.Author, doc.Summary = p.Input.Source, html, actor, p.Input.Summary
				if err := prepareDocumentForSave(ctx, &doc); err != nil {
					return err
				}
				if err := saveDocumentTxn(tx, &doc); err != nil {
					return err
				}
				p.ResultRevision = doc.Revision
			}
			p.Status = "applied"
		}
		p.DecidedAt, p.DecidedBy = time.Now().UTC(), actor
		if err := putProposalTxn(tx, p); err != nil {
			return err
		}
		return (oauthTx{ctx, tx}).Audit(oauth.Event{Actor: actor, Action: "mcp.proposal_decide", Target: p.ID, Result: p.Status})
	})
	if errors.Is(err, badger.ErrConflict) {
		err = changeproposal.ErrConflict
	}
	if err == nil && allow {
		s.invalidateSearchCache(p.Input.Locale)
		if p.Input.Target != nil {
			s.invalidateSearchCache(p.Input.Target.Locale)
		}
	}
	return p, err
}

// A privacy history reset also erases proposal snapshots, including reviewed ones.
func purgeDocumentProposalsTxn(ctx context.Context, tx *badger.Txn, wid document.WID, locale, slug string) error {
	values, err := listProposalsTxn(ctx, tx)
	if err != nil {
		return err
	}
	for _, p := range values {
		if p.Base.WID == wid || p.Target.WID == wid || (p.Input.Locale == locale && techmark.Slugify(p.Input.Name) == slug) {
			if err := deleteProposalDuplicateTxn(tx, p); err != nil {
				return err
			}
			if err := tx.Delete([]byte(proposalSummaryPrefix + p.ID)); err != nil {
				return err
			}
			if err := tx.Delete([]byte(proposalPrefix + p.ID)); err != nil {
				return err
			}
		}
	}
	return nil
}
