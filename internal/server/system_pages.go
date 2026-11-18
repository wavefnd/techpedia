package server

import (
	"context"
	"errors"
	"strings"

	"techpedia/internal/document"
	"techpedia/internal/storage"
)

// Content pages are installation data. The engine does not synthesize locale-specific articles.
var systemPageSlugs = []string{}

func (s *Server) systemPage(context.Context, string, string) (document.Document, bool, error) {
	return document.Document{}, false, nil
}

func isSystemSlug(string) bool {
	return false
}

func canonicalSystemSlug(_ string, slug string) string {
	return strings.TrimSpace(slug)
}

func systemTitle(_ string, slug string) string {
	return strings.ReplaceAll(strings.TrimSpace(slug), "_", " ")
}

func (s *Server) appendSystemSearchResults(_ context.Context, _ string, _ string, results []document.SearchResult, _ int) []document.SearchResult {
	return results
}

func isDocumentNotFound(err error) bool {
	return errors.Is(err, storage.ErrDocumentNotFound)
}
