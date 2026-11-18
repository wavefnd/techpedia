package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"techpedia/internal/auth"
	"techpedia/internal/document"
	"techpedia/internal/storage"
	"techpedia/internal/techmark"
)

func (s *Server) handleListDocumentRedirects(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, auth.PermissionAdmin); !ok {
		return
	}
	locale := strings.TrimSpace(r.URL.Query().Get("locale"))
	if locale != "" {
		canonical, ok := document.CanonicalLocale(locale)
		if !ok {
			http.Error(w, "invalid language tag", http.StatusBadRequest)
			return
		}
		locale = canonical
	}
	limit := 300
	if parsed, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && parsed > 0 && parsed <= 500 {
		limit = parsed
	}
	redirects, err := s.store.ListDocumentRedirects(r.Context(), locale, limit)
	if err != nil {
		http.Error(w, "failed to load document redirects", http.StatusInternalServerError)
		return
	}
	response := make([]adminDocumentRedirectResponse, 0, len(redirects))
	for _, redirect := range redirects {
		response = append(response, newAdminDocumentRedirectResponse(redirect))
	}
	writeJSON(w, http.StatusOK, map[string]any{"redirects": response})
}

func (s *Server) handleCreateDocumentRedirect(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionAdmin)
	if !ok {
		return
	}
	var req adminDocumentRedirectRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	locale, valid := document.CanonicalLocale(req.Locale)
	name := strings.TrimSpace(req.Name)
	targetName := strings.TrimSpace(req.Target)
	if !valid || name == "" || targetName == "" {
		http.Error(w, "language, redirect name, and target document are required", http.StatusBadRequest)
		return
	}
	if !s.requireActiveEdition(w, r, locale) {
		return
	}
	slug := techmark.Slugify(name)
	if _, exists, err := s.systemPage(r.Context(), locale, slug); err != nil {
		http.Error(w, "failed to inspect system pages", http.StatusInternalServerError)
		return
	} else if exists {
		http.Error(w, "redirect name is already used by a system page", http.StatusConflict)
		return
	}
	redirect, err := s.store.CreateDocumentRedirect(r.Context(), locale, name, techmark.Slugify(targetName), user.Username)
	if errors.Is(err, storage.ErrDocumentNotFound) {
		s.recordAudit(r.Context(), user.Username, "document.redirect_create", "redirect", locale+":"+slug, name+" -> "+targetName, "failed")
		http.Error(w, "target document not found", http.StatusNotFound)
		return
	}
	if errors.Is(err, storage.ErrDocumentExists) || errors.Is(err, storage.ErrRedirectCycle) {
		s.recordAudit(r.Context(), user.Username, "document.redirect_create", "redirect", locale+":"+slug, name+" -> "+targetName, "failed")
		http.Error(w, "redirect name is already in use", http.StatusConflict)
		return
	}
	if err != nil {
		s.recordAudit(r.Context(), user.Username, "document.redirect_create", "redirect", locale+":"+slug, name+" -> "+targetName, "failed")
		http.Error(w, "failed to create document redirect", http.StatusInternalServerError)
		return
	}
	s.recordAudit(r.Context(), user.Username, "document.redirect_create", "redirect", locale+":"+slug, redirect.Name+" -> "+redirect.TargetName, "success")
	writeJSON(w, http.StatusCreated, newAdminDocumentRedirectResponse(redirect))
}

func (s *Server) handleDeleteDocumentRedirect(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionAdmin)
	if !ok {
		return
	}
	locale, slug, valid := pageLocaleSlug(r)
	if !valid || slug == "" {
		http.Error(w, "invalid redirect name", http.StatusBadRequest)
		return
	}
	redirect, err := s.store.DeleteDocumentRedirect(r.Context(), locale, slug)
	if errors.Is(err, storage.ErrDocumentNotFound) {
		http.Error(w, "document redirect not found", http.StatusNotFound)
		return
	}
	if err != nil {
		s.recordAudit(r.Context(), user.Username, "document.redirect_delete", "redirect", locale+":"+slug, "", "failed")
		http.Error(w, "failed to delete document redirect", http.StatusInternalServerError)
		return
	}
	s.recordAudit(r.Context(), user.Username, "document.redirect_delete", "redirect", locale+":"+slug, redirect.Name+" -> "+redirect.TargetName, "success")
	w.WriteHeader(http.StatusNoContent)
}

func newAdminDocumentRedirectResponse(redirect document.Redirect) adminDocumentRedirectResponse {
	response := adminDocumentRedirectResponse{
		Locale:       redirect.Locale,
		Name:         redirect.Name,
		URL:          wikiURL(redirect.Locale, redirect.Name),
		TargetName:   redirect.TargetName,
		TargetExists: redirect.TargetExists,
		CreatedBy:    redirect.CreatedBy,
		CreatedAt:    redirect.CreatedAt.UTC().Format(time.RFC3339),
	}
	if redirect.TargetExists {
		response.TargetURL = wikiURL(redirect.Locale, redirect.TargetName)
	}
	return response
}
