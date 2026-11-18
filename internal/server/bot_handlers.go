package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"techpedia/internal/auth"
	"techpedia/internal/bot"
	"techpedia/internal/document"
	"techpedia/internal/storage"
	"techpedia/internal/techmark"
)

type namingSuggestionAcceptRequest struct {
	ProposedName   string  `json:"proposed_name"`
	ProposedSource *string `json:"proposed_source"`
}

func (s *Server) handleScanNamingIndex(w http.ResponseWriter, r *http.Request) {
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
	items, err := s.store.ListItems(r.Context())
	if err != nil {
		http.Error(w, "failed to load Techdata naming data", http.StatusInternalServerError)
		return
	}
	editions, err := s.store.ListEditions(r.Context())
	if err != nil {
		http.Error(w, "failed to load language editions", http.StatusInternalServerError)
		return
	}
	documents := make([]document.Document, 0)
	created := 0
	existing := 0
	for _, value := range editions {
		if !value.Active() || (requestedLocale != "" && value.Tag != requestedLocale) {
			continue
		}
		editionDocuments, err := s.store.ListAllLatest(r.Context(), value.Tag)
		if err != nil {
			http.Error(w, "failed to load documents", http.StatusInternalServerError)
			return
		}
		documents = append(documents, editionDocuments...)
	}
	nameIndex := bot.BuildNameIndex(documents, items)
	crawl := bot.CrawlDocuments(documents)
	for _, doc := range crawl.Documents {
		suggestion, found := bot.ScanDocument(doc, nameIndex)
		if !found {
			continue
		}
		_, wasCreated, err := s.store.CreateNamingSuggestion(r.Context(), suggestion)
		if err != nil {
			http.Error(w, "failed to store naming suggestion", http.StatusInternalServerError)
			return
		}
		if wasCreated {
			created++
		} else {
			existing++
		}
	}
	indexStats := nameIndex.Stats()
	writeJSON(w, http.StatusOK, map[string]any{
		"generator_version": indexStats.GeneratorVersion,
		"indexed_documents": indexStats.Documents,
		"indexed_terms":     indexStats.Terms,
		"index_collisions":  indexStats.Collisions,
		"gram_keys":         indexStats.GramKeys,
		"gram_postings":     indexStats.GramPostings,
		"documents_scanned": crawl.DocumentCount,
		"links_discovered":  crawl.LinkCount,
		"missing_links":     crawl.MissingLinks,
		"orphan_documents":  crawl.OrphanDocuments,
		"created":           created,
		"existing":          existing,
	})
}

func (s *Server) handleListNamingSuggestions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, auth.PermissionAdmin); !ok {
		return
	}
	status := bot.SuggestionStatus(strings.TrimSpace(r.URL.Query().Get("status")))
	limit := 100
	if parsed, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && parsed > 0 && parsed <= 500 {
		limit = parsed
	}
	values, err := s.store.ListNamingSuggestions(r.Context(), status, limit)
	if errors.Is(err, bot.ErrInvalid) {
		http.Error(w, "invalid suggestion status", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "failed to load naming suggestions", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"suggestions": values})
}

func (s *Server) handleAcceptNamingSuggestion(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionAdmin)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var request namingSuggestionAcceptRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	suggestion, err := s.store.GetNamingSuggestion(r.Context(), r.PathValue("suggestionID"))
	if errors.Is(err, bot.ErrNotFound) {
		http.Error(w, "naming suggestion not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load naming suggestion", http.StatusInternalServerError)
		return
	}
	if suggestion.Status != bot.SuggestionOpen {
		http.Error(w, "naming suggestion already reviewed", http.StatusConflict)
		return
	}
	doc, err := s.store.GetLatest(r.Context(), suggestion.Locale, suggestion.Slug)
	if errors.Is(err, storage.ErrDocumentNotFound) {
		http.Error(w, "document changed after this suggestion; scan again", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "failed to load document", http.StatusInternalServerError)
		return
	}
	if doc.Revision != suggestion.BaseRevision || doc.Title != suggestion.PageName || doc.Source != suggestion.OriginalSource {
		http.Error(w, "document changed after this suggestion; scan again", http.StatusConflict)
		return
	}

	proposedName := strings.TrimSpace(request.ProposedName)
	if proposedName == "" {
		proposedName = suggestion.ProposedName
	}
	proposedSource := suggestion.ProposedSource
	if request.ProposedSource != nil {
		proposedSource = *request.ProposedSource
	}
	newSlug := techmark.Slugify(proposedName)
	if newSlug == "" {
		http.Error(w, "invalid proposed document name", http.StatusBadRequest)
		return
	}
	currentIsMainPage, err := s.isEditionMainPage(r.Context(), doc.Locale, doc.Slug)
	if err != nil {
		http.Error(w, "failed to inspect language edition", http.StatusInternalServerError)
		return
	}
	if currentIsMainPage && newSlug != doc.Slug {
		http.Error(w, "language edition main page cannot be moved", http.StatusConflict)
		return
	}
	if techmark.ContainsSpecialSyntax(proposedSource) && doc.EditPolicy != document.EditPolicyAdmins {
		http.Error(w, "special TechMark syntax requires an administrator-only document", http.StatusBadRequest)
		return
	}
	if techmark.ContainsHomePageSyntax(proposedSource) && (!currentIsMainPage || newSlug != doc.Slug) {
		http.Error(w, "homepage TechMark syntax is only allowed on the edition main page", http.StatusBadRequest)
		return
	}

	if newSlug != doc.Slug {
		doc, err = s.store.MoveDocumentAs(r.Context(), doc.Locale, doc.Slug, newSlug, proposedName, user.Username)
		if errors.Is(err, storage.ErrDocumentExists) {
			http.Error(w, "target document already exists", http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, "failed to move document", http.StatusInternalServerError)
			return
		}
	}

	if doc.Title != proposedName || doc.Source != proposedSource {
		doc.Title = proposedName
		doc.Source = proposedSource
		doc.HTML = s.parser.RenderWithLocale(proposedSource, doc.Locale)
		doc.Author = user.Username
		doc.Summary = "명명 정책 봇 제안 수락"
		if _, err := s.store.SaveIfRevision(r.Context(), doc, doc.Revision); errors.Is(err, storage.ErrRevisionConflict) {
			http.Error(w, "document changed while applying the suggestion", http.StatusConflict)
			return
		} else if err != nil {
			http.Error(w, "failed to apply naming suggestion", http.StatusInternalServerError)
			return
		}
	}

	reviewed, err := s.store.ReviewNamingSuggestion(r.Context(), suggestion.ID, bot.SuggestionAccepted, user.Username, proposedName, proposedSource)
	if errors.Is(err, bot.ErrReviewed) {
		http.Error(w, "naming suggestion already reviewed", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "failed to record naming review", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, reviewed)
}

func (s *Server) handleRejectNamingSuggestion(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionAdmin)
	if !ok {
		return
	}
	suggestion, err := s.store.GetNamingSuggestion(r.Context(), r.PathValue("suggestionID"))
	if errors.Is(err, bot.ErrNotFound) {
		http.Error(w, "naming suggestion not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load naming suggestion", http.StatusInternalServerError)
		return
	}
	reviewed, err := s.store.ReviewNamingSuggestion(r.Context(), suggestion.ID, bot.SuggestionRejected, user.Username, suggestion.ProposedName, suggestion.ProposedSource)
	if errors.Is(err, bot.ErrReviewed) {
		http.Error(w, "naming suggestion already reviewed", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "failed to reject naming suggestion", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, reviewed)
}
