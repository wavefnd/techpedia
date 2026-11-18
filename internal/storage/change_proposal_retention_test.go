package storage_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"techpedia/internal/changeproposal"
	"techpedia/internal/storage"
)

func TestLegacySevenDayProposalMigrationAndPolicyPersistence(t *testing.T) {
	path := t.TempDir()
	db, err := badger.Open(badger.DefaultOptions(path).WithLogger(nil))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, age := range []time.Duration{time.Hour, 25 * time.Hour, 72 * time.Hour} {
		p := changeproposal.Proposal{ID: age.String(), CreatedAt: now.Add(-age), ExpiresAt: now.Add(-age).Add(7 * 24 * time.Hour), Status: "pending", Input: changeproposal.Input{Kind: "create", Locale: "ko", Name: "기존 제안", Source: "legacy source", Summary: "create"}}
		raw, _ := json.Marshal(p)
		if err := db.Update(func(tx *badger.Txn) error {
			entry := badger.NewEntry([]byte("mcp:v1:proposal:"+p.ID), raw)
			entry.ExpiresAt = uint64(p.ExpiresAt.Unix())
			return tx.SetEntry(entry)
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := storage.OpenBadgerDocumentStore(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	p, err := store.GetChangeProposal(ctx, time.Hour.String())
	if err != nil || p.ExpiresAt.Sub(p.CreatedAt) != 48*time.Hour || p.Input.Source != "legacy source" {
		t.Fatal("legacy retention not shortened", p, err)
	}
	if _, err := store.GetChangeProposal(ctx, (72 * time.Hour).String()); !errors.Is(err, changeproposal.ErrNotFound) {
		t.Fatal("expired legacy snapshot survived", err)
	}
	summaries, err := store.ListChangeProposals(ctx)
	if err != nil || len(summaries) != 2 || summaries[0].Input.Source != "" {
		t.Fatal("legacy summary index not initialized", err)
	}
	if err := store.SetProposalPolicy(ctx, changeproposal.Policy{RetentionDays: 1}, "reviewer"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetChangeProposal(ctx, (25 * time.Hour).String()); !errors.Is(err, changeproposal.ErrNotFound) {
		t.Fatal("shortened proposal snapshot survived", err)
	}
	summaries, err = store.ListChangeProposals(ctx)
	if err != nil || len(summaries) != 1 {
		t.Fatal("expired summary survived", len(summaries), err)
	}
	if err := store.SetProposalPolicy(ctx, changeproposal.Policy{RetentionDays: 3}, "reviewer"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = storage.OpenBadgerDocumentStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	policy, err := store.GetProposalPolicy(ctx)
	if err != nil || policy.RetentionDays != 3 {
		t.Fatal("policy lost on restart", policy, err)
	}
}
