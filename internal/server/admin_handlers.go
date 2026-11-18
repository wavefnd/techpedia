package server

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"techpedia/internal/auth"
	"techpedia/internal/buildinfo"
	"techpedia/internal/document"
	"techpedia/internal/storage"
)

func (s *Server) handlePublicConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"engine": map[string]string{
			"version": buildinfo.Version,
		},
		"discussion": map[string]any{
			"enabled": true,
			"mode":    "board",
		},
	})
}

func (s *Server) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, auth.PermissionAdmin); !ok {
		return
	}
	editions, err := s.store.ListEditions(r.Context())
	if err != nil {
		http.Error(w, "failed to load language editions", http.StatusInternalServerError)
		return
	}
	users, err := s.accounts.ListUsers(r.Context())
	if err != nil {
		http.Error(w, "failed to load users", http.StatusInternalServerError)
		return
	}
	response := adminDashboardResponse{Users: len(users), ByLocale: []adminLocaleCount{}, RecentDocuments: []adminDocumentSummary{}}
	allDocuments := make([]adminDocumentSummary, 0)
	for _, value := range editions {
		if !value.Active() {
			response.PendingEditions++
			continue
		}
		response.ActiveEditions++
		documents, err := s.adminDocumentsForLocale(r.Context(), value.Tag)
		if err != nil {
			http.Error(w, "failed to load documents", http.StatusInternalServerError)
			return
		}
		response.Documents += len(documents)
		response.ByLocale = append(response.ByLocale, adminLocaleCount{Locale: value.Tag, Documents: len(documents)})
		for _, doc := range documents {
			if !isSystemSlug(doc.Slug) {
				allDocuments = append(allDocuments, s.newAdminDocumentSummary(r.Context(), doc))
			}
		}
	}
	sort.Slice(allDocuments, func(i, j int) bool { return allDocuments[i].UpdatedAt > allDocuments[j].UpdatedAt })
	if len(allDocuments) > 8 {
		allDocuments = allDocuments[:8]
	}
	response.RecentDocuments = allDocuments
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleAdminPages(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, auth.PermissionAdmin); !ok {
		return
	}
	requestedLocale := strings.TrimSpace(r.URL.Query().Get("locale"))
	if requestedLocale != "" {
		canonical, ok := document.CanonicalLocale(requestedLocale)
		if !ok {
			http.Error(w, "invalid language tag", http.StatusBadRequest)
			return
		}
		requestedLocale = canonical
	}
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	limit := 200
	if parsed, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && parsed > 0 && parsed <= 500 {
		limit = parsed
	}
	editions, err := s.store.ListEditions(r.Context())
	if err != nil {
		http.Error(w, "failed to load language editions", http.StatusInternalServerError)
		return
	}
	results := make([]adminDocumentSummary, 0)
	for _, value := range editions {
		if !value.Active() || (requestedLocale != "" && value.Tag != requestedLocale) {
			continue
		}
		documents, err := s.adminDocumentsForLocale(r.Context(), value.Tag)
		if err != nil {
			http.Error(w, "failed to load documents", http.StatusInternalServerError)
			return
		}
		for _, doc := range documents {
			if query != "" && !strings.Contains(strings.ToLower(doc.Title+" "+doc.Source), query) {
				continue
			}
			results = append(results, s.newAdminDocumentSummary(r.Context(), doc))
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].UpdatedAt > results[j].UpdatedAt })
	if len(results) > limit {
		results = results[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": results})
}

func (s *Server) handleAdminPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, auth.PermissionAdmin); !ok {
		return
	}
	locale, slug, ok := pageLocaleSlug(r)
	if !ok || slug == "" {
		http.Error(w, "invalid document name", http.StatusBadRequest)
		return
	}
	value, err := s.store.GetLatest(r.Context(), locale, slug)
	if errors.Is(err, storage.ErrDocumentNotFound) {
		http.Error(w, "document not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load document", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, s.newAdminDocumentSummary(r.Context(), value))
}

func (s *Server) newAdminDocumentSummary(ctx context.Context, value document.Document) adminDocumentSummary {
	tid := ""
	if value.ID != "" {
		if item, err := s.store.GetItemByWikiPage(ctx, value.ID); err == nil {
			tid = item.ID.String()
		}
	}
	return newAdminDocumentSummary(value, tid)
}

func newAdminDocumentSummary(value document.Document, tid string) adminDocumentSummary {
	return adminDocumentSummary{
		WID:        value.ID.String(),
		TID:        tid,
		Locale:     value.Locale,
		Name:       value.Title,
		URL:        wikiURL(value.Locale, value.Title),
		EditURL:    editorURL(value.Locale, value.Title),
		Revision:   value.Revision,
		Author:     value.Author,
		UpdatedAt:  value.UpdatedAt.UTC().Format(time.RFC3339),
		System:     isSystemSlug(value.Slug),
		Editable:   true,
		EditPolicy: document.NormalizeEditPolicy(value.EditPolicy),
	}
}

func (s *Server) adminDocumentsForLocale(ctx context.Context, locale string) ([]document.Document, error) {
	documents, err := s.store.ListAllLatest(ctx, locale)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(documents)+len(systemPageSlugs))
	for _, value := range documents {
		seen[canonicalSystemSlug(locale, value.Slug)] = struct{}{}
	}
	for _, slug := range systemPageSlugs {
		if _, exists := seen[slug]; exists {
			continue
		}
		value, exists, err := s.systemPage(ctx, locale, slug)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		documents = append(documents, value)
		seen[value.Slug] = struct{}{}
	}
	return documents, nil
}
