package storage

import (
	"context"
	"errors"
	"testing"

	"techpedia/internal/bot"
)

func TestNamingBotStoreLifecycle(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	suggestion := bot.NamingSuggestion{
		SchemaVersion: bot.NamingSuggestionSchemaVersion,
		ID:            "ns2_test", Locale: "ko", Slug: "dennis-ritchie", PageName: "Dennis Ritchie", BaseRevision: "r1",
		OriginalSource: "# Dennis Ritchie", ProposedName: "데니스 리치", ProposedSource: "# 데니스 리치",
		Findings: []bot.Finding{{CheckerID: "name-index:v1", Area: "link", Before: "C", After: "C (프로그래밍 언어)", MatchKind: bot.NameMatchExact, Source: bot.NameSourceGenerated}},
	}
	createdSuggestion, created, err := store.CreateNamingSuggestion(ctx, suggestion)
	if err != nil || !created || createdSuggestion.Status != bot.SuggestionOpen {
		t.Fatalf("create suggestion: %#v, created=%v, %v", createdSuggestion, created, err)
	}
	invalidSchema := suggestion
	invalidSchema.ID = "ns2_invalid_schema"
	invalidSchema.SchemaVersion = 0
	if _, _, err := store.CreateNamingSuggestion(ctx, invalidSchema); !errors.Is(err, bot.ErrInvalid) {
		t.Fatalf("expected invalid schema error, got %v", err)
	}
	duplicate, created, err := store.CreateNamingSuggestion(ctx, suggestion)
	if err != nil || created || duplicate.ID != suggestion.ID {
		t.Fatalf("deduplicate suggestion: %#v, created=%v, %v", duplicate, created, err)
	}
	open, err := store.ListNamingSuggestions(ctx, bot.SuggestionOpen, 10)
	if err != nil || len(open) != 1 {
		t.Fatalf("list open suggestions: %#v, %v", open, err)
	}
	reviewed, err := store.ReviewNamingSuggestion(ctx, suggestion.ID, bot.SuggestionAccepted, "admin", "데니스 리치", "# 데니스 리치\n")
	if err != nil || reviewed.Status != bot.SuggestionAccepted || reviewed.ReviewedBy != "admin" || reviewed.ReviewedAt.IsZero() {
		t.Fatalf("review suggestion: %#v, %v", reviewed, err)
	}
	if _, err := store.ReviewNamingSuggestion(ctx, suggestion.ID, bot.SuggestionRejected, "admin", "", ""); !errors.Is(err, bot.ErrReviewed) {
		t.Fatalf("expected reviewed error, got %v", err)
	}
}
