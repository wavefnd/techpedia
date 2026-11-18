package storage

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestSearchBudgetFallbackRetainsRankingAndSnippets(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	for _, title := range []string{"Alpha", "beta", "Gamma", "delta", "epsilon"} {
		saveIndexFixture(t, store, "en", title, "needle [[Target]] "+strings.Repeat("longbody ", 300))
	}
	want, err := store.Search(ctx, "en", "needle", 3)
	if err != nil || len(want) != 3 {
		t.Fatalf("search: %#v, %v", want, err)
	}
	for _, candidate := range store.searchCache["en"].candidates {
		if candidate.doc.Source != "" || candidate.doc.HTML != "" || candidate.doc.Author != "" {
			t.Fatal("raw bodies retained in search cache")
		}
	}
	store.invalidateSearchCache("en")
	store.searchMaxBytes = 1024
	for range 2 {
		got, err := store.Search(ctx, "en", "needle", 3)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("streamed search differs: %#v, %v", got, err)
		}
		entry := store.searchCache["en"]
		if entry.candidates != nil || entry.bytes != 0 || store.searchCacheBytes != 0 {
			t.Fatal("oversized snapshot retained")
		}
	}
}

func TestSearchCacheBudgetIsSharedAcrossEditions(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	store.searchMaxBytes = 10 << 10
	for _, locale := range []string{"ko", "en", "ar", "fr", "es", "de", "it", "ru", "ja", "zh"} {
		saveIndexFixture(t, store, locale, "Alpha", "needle "+strings.Repeat("body ", 900))
		if _, err := store.Search(ctx, locale, "needle", 8); err != nil {
			t.Fatal(err)
		}
		if store.searchCacheBytes > store.searchMaxBytes || len(store.searchCache) > searchCacheLocales {
			t.Fatal("global search cache budget exceeded")
		}
	}
	// An evicted edition must remain fully searchable; this is not an edition cap.
	results, err := store.Search(ctx, "ko", "needle", 8)
	if err != nil || len(results) != 1 {
		t.Fatalf("evicted edition search: %#v, %v", results, err)
	}
	store.invalidateSearchCache("ko")
	if store.searchCacheBytes < 0 {
		t.Fatal("invalid cache byte accounting")
	}
}
