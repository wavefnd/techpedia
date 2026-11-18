package server

import (
	"errors"
	"net/http"
	"strings"

	"techpedia/internal/auth"
	"techpedia/internal/bot"
	"techpedia/internal/document"
	"techpedia/internal/storage"
)

type linkPageResponse struct {
	Locale  string `json:"locale"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	EditURL string `json:"edit_url,omitempty"`
}

type missingLinkResponse struct {
	Source linkPageResponse `json:"source"`
	Target linkPageResponse `json:"target"`
}

func (s *Server) handlePageBacklinks(w http.ResponseWriter, r *http.Request) {
	locale, slug, ok := pageLocaleSlug(r)
	if !ok || slug == "" {
		http.Error(w, "invalid document name", http.StatusBadRequest)
		return
	}
	if !s.requireActiveEdition(w, r, locale) {
		return
	}
	if _, err := s.store.GetLatest(r.Context(), locale, slug); errors.Is(err, storage.ErrDocumentNotFound) {
		http.Error(w, "document not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "failed to load document", http.StatusInternalServerError)
		return
	}
	backlinks, err := s.store.ListBacklinks(r.Context(), locale, slug)
	if err != nil {
		http.Error(w, "failed to load documents", http.StatusInternalServerError)
		return
	}
	pages := make([]linkPageResponse, 0, len(backlinks))
	for _, doc := range backlinks {
		pages = append(pages, newLinkPageResponse(doc, false))
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(pages), "pages": pages})
}

func (s *Server) handleAdminLinkReport(w http.ResponseWriter, r *http.Request) {
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
	editions, err := s.store.ListEditions(r.Context())
	if err != nil {
		http.Error(w, "failed to load language editions", http.StatusInternalServerError)
		return
	}
	documents := make([]document.LinkDocument, 0)
	for _, value := range editions {
		if !value.Active() || (requestedLocale != "" && value.Tag != requestedLocale) {
			continue
		}
		editionDocuments, err := s.store.ListLinkDocuments(r.Context(), value.Tag)
		if err != nil {
			http.Error(w, "failed to load documents", http.StatusInternalServerError)
			return
		}
		documents = append(documents, editionDocuments...)
	}
	analysis := bot.AnalyzeLinkDocuments(documents)
	missing := make([]missingLinkResponse, 0, len(analysis.Missing))
	for _, value := range analysis.Missing {
		missing = append(missing, missingLinkResponse{
			Source: newLinkPageResponse(value.Source, true),
			Target: linkPageResponse{
				Locale:  value.Source.Locale,
				Name:    value.TargetName,
				URL:     wikiURL(value.Source.Locale, value.TargetName),
				EditURL: editorURL(value.Source.Locale, value.TargetName),
			},
		})
	}
	orphans := make([]linkPageResponse, 0, len(analysis.OrphanDocuments))
	for _, doc := range analysis.OrphanDocuments {
		orphans = append(orphans, newLinkPageResponse(doc, true))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"documents":        len(analysis.Documents),
		"links":            analysis.LinkCount,
		"missing_links":    missing,
		"orphan_documents": orphans,
	})
}

func newLinkPageResponse(doc document.Document, admin bool) linkPageResponse {
	result := linkPageResponse{Locale: doc.Locale, Name: doc.Title, URL: wikiURL(doc.Locale, doc.Title)}
	if admin {
		result.EditURL = editorURL(doc.Locale, doc.Title)
	}
	return result
}
