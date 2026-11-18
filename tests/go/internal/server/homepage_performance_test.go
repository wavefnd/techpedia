package server

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"techpedia/internal/config"
	"techpedia/internal/document"
	"techpedia/internal/storage"
	"techpedia/internal/techmark"
)

type countedHomeStore struct {
	Store
	bodyReads  int
	wholeReads int
}

func (s *countedHomeStore) GetLatest(ctx context.Context, locale, slug string) (document.Document, error) {
	s.bodyReads++
	return s.Store.GetLatest(ctx, locale, slug)
}

func (s *countedHomeStore) ListAllLatest(ctx context.Context, locale string) ([]document.Document, error) {
	s.wholeReads++
	return s.Store.ListAllLatest(ctx, locale)
}

func TestFeaturedPagesReadOnlySelectedArticleBodies(t *testing.T) {
	store, err := storage.OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	for i := 0; i < 40; i++ {
		if _, err := store.Save(ctx, document.Document{Locale: "en", Slug: fmt.Sprintf("Article_%02d", i), Title: fmt.Sprintf("Article %02d", i), Source: "Ordinary readable article"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Save(ctx, document.Document{Locale: "en", Slug: "Special", Title: "Special", Source: `<wiki-recent-changes></wiki-recent-changes>`}); err != nil {
		t.Fatal(err)
	}
	counted := &countedHomeStore{Store: store}
	app := New(config.Config{}, counted, emptyAuth{}, techmark.NewParser())
	value, err := app.renderFeaturedPages(ctx, document.Document{Locale: "en", Slug: "Main_Page"}, `<wiki-featured-pages limit="2"></wiki-featured-pages>`)
	if err != nil {
		t.Fatal(err)
	}
	if counted.wholeReads != 0 || counted.bodyReads < 2 || counted.bodyReads > 3 {
		t.Fatalf("expected two bodies plus at most one skipped special page, got whole=%d body=%d", counted.wholeReads, counted.bodyReads)
	}
	if strings.Count(value, "<li>") != 2 || !strings.Contains(value, `/en/wiki/Article_`) || strings.Contains(value, `/en/wiki/Special`) {
		t.Fatalf("document links and special-page exclusion changed: %s", value)
	}
}
