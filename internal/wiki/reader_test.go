package wiki

import (
	"context"
	"errors"
	"testing"

	"techpedia/internal/document"
	"techpedia/internal/edition"
	"techpedia/internal/storage"
	"techpedia/internal/techmark"
)

type readStore struct {
	editions  []edition.Edition
	docs      map[string]document.Document
	redirects map[string]string
	err       error
}

func (s readStore) ListEditions(ctx context.Context) ([]edition.Edition, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return s.editions, s.err
}
func (s readStore) GetEdition(ctx context.Context, tag string) (edition.Edition, error) {
	if ctx.Err() != nil {
		return edition.Edition{}, ctx.Err()
	}
	if s.err != nil {
		return edition.Edition{}, s.err
	}
	for _, e := range s.editions {
		if e.Tag == tag {
			return e, nil
		}
	}
	return edition.Edition{}, edition.ErrNotFound
}
func (s readStore) GetLatest(ctx context.Context, locale, slug string) (document.Document, error) {
	if ctx.Err() != nil {
		return document.Document{}, ctx.Err()
	}
	if doc, ok := s.docs[locale+":"+slug]; ok {
		return doc, nil
	}
	return document.Document{}, storage.ErrDocumentNotFound
}
func (s readStore) GetDocumentRedirect(_ context.Context, locale, slug string) (string, error) {
	if target, ok := s.redirects[locale+":"+slug]; ok {
		return target, nil
	}
	return "", storage.ErrDocumentNotFound
}
func (s readStore) Search(_ context.Context, locale, _ string, _ int) ([]document.SearchResult, error) {
	values := []document.SearchResult{}
	for _, doc := range s.docs {
		if doc.Locale == locale {
			values = append(values, document.NewSearchResult(doc, "snippet", 1))
		}
	}
	return values, nil
}

func TestReaderEditionRulesAndNoSyntheticContent(t *testing.T) {
	store := readStore{editions: []edition.Edition{
		{Tag: "ko", Status: edition.StatusActive, MainPageName: "대문"},
		{Tag: "en", Status: edition.StatusActive, MainPageName: "Main Page"},
		{Tag: "fr", Status: edition.StatusActive, MainPageName: "Accueil"},
		{Tag: "ar", Status: edition.StatusApproved},
		{Tag: "de", Status: edition.StatusActive},
	}}
	reader := NewReader(store)
	values, err := reader.ActiveEditions(context.Background())
	if err != nil || len(values) != 3 {
		t.Fatalf("active editions = %+v, %v", values, err)
	}
	for _, tag := range []string{"ko", "en", "fr"} {
		if _, err := reader.ActiveEdition(context.Background(), tag); err != nil {
			t.Fatalf("%s: %v", tag, err)
		}
		if _, err := reader.Page(context.Background(), tag, "Main Page"); !errors.Is(err, storage.ErrDocumentNotFound) {
			t.Fatalf("synthetic %s content: %v", tag, err)
		}
	}
	for _, tag := range []string{"ar", "de", "ja"} {
		if _, err := reader.Page(context.Background(), tag, "Article"); !errors.Is(err, ErrInactiveEdition) {
			t.Fatalf("inactive %s: %v", tag, err)
		}
	}
	for _, tag := range []string{"zh-Hans", "pt-BR", "und", "x-private"} {
		if _, err := reader.Search(context.Background(), tag, "x", 5); !errors.Is(err, ErrInvalidLocale) {
			t.Fatalf("invalid %s: %v", tag, err)
		}
	}
}

func TestReaderResolvesRedirectAndPrioritizesUniqueTarget(t *testing.T) {
	doc := document.Document{Locale: "en", Title: "Berkeley Software Distribution", Slug: techmark.Slugify("Berkeley Software Distribution"), Source: "Operating system", Revision: "r3"}
	reader := NewReader(readStore{editions: []edition.Edition{{Tag: "en", Status: edition.StatusActive, MainPageName: "Main Page"}}, docs: map[string]document.Document{"en:" + doc.Slug: doc}, redirects: map[string]string{"en:" + techmark.Slugify("BSD"): doc.Slug}})
	page, err := reader.Page(context.Background(), "en", "BSD")
	if err != nil || page.Title != doc.Title {
		t.Fatalf("redirect read = %+v, %v", page, err)
	}
	result, err := reader.Search(context.Background(), "en", "BSD", 1)
	if err != nil || !result.ExactMatch || len(result.Results) != 1 || result.Results[0].Title != doc.Title {
		t.Fatalf("redirect search = %+v, %v", result, err)
	}
	result, err = reader.Search(context.Background(), "en", "", 50)
	if err != nil || len(result.Results) != 0 || result.ExactMatch {
		t.Fatalf("empty search = %+v, %v", result, err)
	}
}

func TestReaderPropagatesStoreErrorsAndCancellation(t *testing.T) {
	errUnavailable := errors.New("store unavailable")
	reader := NewReader(readStore{err: errUnavailable})
	if _, err := reader.Page(context.Background(), "ko", "Article"); !errors.Is(err, errUnavailable) {
		t.Fatalf("error lost: %v", err)
	}
	if _, err := reader.ActiveEditions(context.Background()); !errors.Is(err, errUnavailable) {
		t.Fatalf("error lost: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reader.Page(ctx, "ko", "Article"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
