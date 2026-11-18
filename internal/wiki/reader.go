// Package wiki provides application operations shared by HTTP and external integrations.
package wiki

import (
	"context"
	"errors"
	"strings"

	"techpedia/internal/document"
	"techpedia/internal/edition"
	"techpedia/internal/storage"
	"techpedia/internal/techmark"
)

var (
	ErrInvalidLocale   = errors.New("invalid language edition code")
	ErrInactiveEdition = errors.New("language edition is not active; ask an administrator to create its main page")
)

// ReadStore deliberately contains no mutation, account or Techdata management operations.
type ReadStore interface {
	GetEdition(context.Context, string) (edition.Edition, error)
	ListEditions(context.Context) ([]edition.Edition, error)
	GetLatest(context.Context, string, string) (document.Document, error)
	GetDocumentRedirect(context.Context, string, string) (string, error)
	Search(context.Context, string, string, int) ([]document.SearchResult, error)
}

type Reader struct {
	store ReadStore
}

func NewReader(store ReadStore) *Reader { return &Reader{store: store} }

func (r *Reader) ActiveEdition(ctx context.Context, locale string) (edition.Edition, error) {
	tag, ok := document.CanonicalLocale(locale)
	if !ok {
		return edition.Edition{}, ErrInvalidLocale
	}
	value, err := r.store.GetEdition(ctx, tag)
	if errors.Is(err, edition.ErrNotFound) || (err == nil && !value.Active()) {
		return edition.Edition{}, ErrInactiveEdition
	}
	if errors.Is(err, edition.ErrInvalid) {
		return edition.Edition{}, ErrInvalidLocale
	}
	return value, err
}

func (r *Reader) ActiveEditions(ctx context.Context) ([]edition.Edition, error) {
	values, err := r.store.ListEditions(ctx)
	if err != nil {
		return nil, err
	}
	active := make([]edition.Edition, 0, len(values))
	for _, value := range values {
		if value.Active() {
			active = append(active, value)
		}
	}
	return active, nil
}

func (r *Reader) Page(ctx context.Context, locale, name string) (document.Document, error) {
	value, err := r.ActiveEdition(ctx, locale)
	if err != nil {
		return document.Document{}, err
	}
	slug := techmark.Slugify(name)
	doc, err := r.store.GetLatest(ctx, value.Tag, slug)
	if !errors.Is(err, storage.ErrDocumentNotFound) {
		return doc, err
	}
	target, err := r.store.GetDocumentRedirect(ctx, value.Tag, slug)
	if err != nil {
		return document.Document{}, err
	}
	return r.store.GetLatest(ctx, value.Tag, target)
}

type SearchResult struct {
	Query      string
	Results    []document.SearchResult
	ExactMatch bool
}

func (r *Reader) Search(ctx context.Context, locale, query string, limit int) (SearchResult, error) {
	value, err := r.ActiveEdition(ctx, locale)
	if err != nil {
		return SearchResult{}, err
	}
	query = strings.TrimSpace(query)
	result := SearchResult{Query: query, Results: []document.SearchResult{}}
	if query == "" {
		return result, nil
	}
	limit = max(1, min(limit, 50))
	result.Results, err = r.store.Search(ctx, value.Tag, query, limit)
	if err != nil {
		return SearchResult{}, err
	}
	target, err := r.store.GetDocumentRedirect(ctx, value.Tag, techmark.Slugify(query))
	if err == nil {
		doc, lookupErr := r.store.GetLatest(ctx, value.Tag, target)
		if lookupErr == nil {
			matches := []document.SearchResult{document.NewSearchResult(doc, techmark.Excerpt(doc.Source, doc.Title, 96), 1<<30)}
			for _, entry := range result.Results {
				if entry.Slug != doc.Slug && len(matches) < limit {
					matches = append(matches, entry)
				}
			}
			result.Results = matches
			result.ExactMatch = true
		} else if !errors.Is(lookupErr, storage.ErrDocumentNotFound) {
			return SearchResult{}, lookupErr
		}
	} else if !errors.Is(err, storage.ErrDocumentNotFound) {
		return SearchResult{}, err
	}
	for _, entry := range result.Results {
		if strings.EqualFold(entry.Title, query) {
			result.ExactMatch = true
		}
	}
	return result, nil
}
