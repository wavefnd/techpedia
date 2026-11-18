package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"techpedia/internal/auth"
	"techpedia/internal/document"
	"techpedia/internal/edition"
	"techpedia/internal/storage"
	"techpedia/internal/techmark"
	"techpedia/internal/translation"
)

func (s *Server) handleListEditions(w http.ResponseWriter, r *http.Request) {
	values, err := s.store.ListEditions(r.Context())
	if err != nil {
		http.Error(w, "failed to load language editions", http.StatusInternalServerError)
		return
	}
	counts, err := s.store.CountLatestByLocale(r.Context())
	if err != nil {
		http.Error(w, "failed to count language edition documents", http.StatusInternalServerError)
		return
	}
	responses := make([]editionResponse, 0, len(values))
	for _, value := range values {
		response := newEditionResponse(value)
		response.DocumentCount = counts[value.Tag]
		responses = append(responses, response)
	}
	writeJSON(w, http.StatusOK, map[string]any{"editions": responses})
}

func (s *Server) handleLanguageCatalog(w http.ResponseWriter, r *http.Request) {
	values, err := s.store.ListEditions(r.Context())
	if err != nil {
		http.Error(w, "failed to load registered language editions", http.StatusInternalServerError)
		return
	}
	excluded := make(map[string]struct{}, len(values))
	for _, value := range values {
		excluded[strings.ToLower(value.Tag)] = struct{}{}
	}
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil {
		limit = 30
	}
	results := edition.SearchLanguageCatalog(r.URL.Query().Get("q"), excluded, limit)
	writeJSON(w, http.StatusOK, map[string]any{"languages": results})
}

func (s *Server) handleCountryCatalog(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"countries": edition.CountryCatalog()})
}

func (s *Server) handleProposeEdition(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionEdit)
	if !ok {
		return
	}
	var req editionProposalRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	name, nativeName, direction, valid := edition.DescribeLanguage(req.Tag)
	if !valid {
		http.Error(w, "use one registered IANA language code without script or region subtags", http.StatusBadRequest)
		return
	}
	value, err := s.store.ProposeEdition(r.Context(), edition.Edition{
		Tag:         req.Tag,
		Name:        name,
		NativeName:  nativeName,
		Direction:   direction,
		Status:      edition.StatusPending,
		RequestedBy: user.Username,
		RequestedAt: time.Now().UTC(),
	})
	if errors.Is(err, edition.ErrAlreadyExists) {
		http.Error(w, "language edition already exists", http.StatusConflict)
		return
	}
	if errors.Is(err, edition.ErrInvalid) {
		http.Error(w, "invalid language edition code", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "failed to propose language edition", http.StatusInternalServerError)
		return
	}
	response := newEditionResponse(value)
	writeJSON(w, http.StatusCreated, response)
}

func (s *Server) handleApproveEdition(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionAdmin)
	if !ok {
		return
	}
	tag, valid := document.CanonicalLocale(r.PathValue("locale"))
	if !valid {
		http.Error(w, "invalid language edition code", http.StatusBadRequest)
		return
	}
	value, err := s.store.ApproveEdition(r.Context(), tag, user.Username, time.Now().UTC())
	if errors.Is(err, edition.ErrNotFound) {
		http.Error(w, "language edition proposal not found", http.StatusNotFound)
		return
	}
	if errors.Is(err, edition.ErrInvalid) {
		http.Error(w, "language edition proposal is not awaiting review", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "failed to approve language edition", http.StatusInternalServerError)
		return
	}
	s.recordAudit(r.Context(), user.Username, "edition.approve", "edition", tag, "", "success")
	response := newEditionResponse(value)
	if s.interfaceTranslator != nil {
		translated, filled, translateErr := s.translateEditionInterface(r.Context(), tag)
		if translateErr != nil {
			response.InterfaceTranslationStatus = "failed"
			s.recordAudit(r.Context(), user.Username, "edition.interface_translate", "edition", tag, "", "failed")
			log.Printf("interface translation for %s failed after approval: %v", tag, translateErr)
		} else {
			response = newEditionResponse(translated)
			response.InterfaceTranslationStatus = "complete"
			response.InterfaceTranslationFilled = filled
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleRejectEdition(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionAdmin)
	if !ok {
		return
	}
	tag, valid := document.CanonicalLocale(r.PathValue("locale"))
	if !valid {
		http.Error(w, "invalid language edition code", http.StatusBadRequest)
		return
	}
	var req editionRejectionRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Confirmation) != tag {
		http.Error(w, "language edition confirmation does not match", http.StatusBadRequest)
		return
	}
	if _, err := s.store.RejectEdition(r.Context(), tag); errors.Is(err, edition.ErrNotFound) {
		http.Error(w, "language edition proposal not found", http.StatusNotFound)
		return
	} else if errors.Is(err, edition.ErrInvalid) {
		http.Error(w, "only an unapproved language proposal can be rejected", http.StatusConflict)
		return
	} else if err != nil {
		http.Error(w, "failed to reject language edition", http.StatusInternalServerError)
		return
	}
	s.recordAudit(r.Context(), user.Username, "edition.reject", "edition", tag, "", "success")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleActivateEdition(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionAdmin)
	if !ok {
		return
	}
	tag, valid := document.CanonicalLocale(r.PathValue("locale"))
	if !valid {
		http.Error(w, "invalid language edition code", http.StatusBadRequest)
		return
	}
	value, err := s.store.GetEdition(r.Context(), tag)
	if errors.Is(err, edition.ErrNotFound) {
		http.Error(w, "language edition must be proposed first", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load language edition", http.StatusInternalServerError)
		return
	}
	if value.Active() {
		http.Error(w, "language edition is already active", http.StatusConflict)
		return
	}
	if value.Status != edition.StatusApproved {
		http.Error(w, "language edition must be approved before activation", http.StatusConflict)
		return
	}
	var req editionActivationRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.MainPageName)
	source := strings.TrimSpace(req.Source)
	if name == "" || source == "" {
		http.Error(w, "main page name and source are required", http.StatusBadRequest)
		return
	}
	slug := techmark.Slugify(name)
	existing, getErr := s.store.GetLatest(r.Context(), tag, slug)
	if getErr != nil && !errors.Is(getErr, storage.ErrDocumentNotFound) {
		http.Error(w, "failed to inspect main page", http.StatusInternalServerError)
		return
	}
	editPolicy := document.EditPolicyEditors
	if techmark.ContainsSpecialSyntax(source) {
		editPolicy = document.EditPolicyAdmins
	}
	doc := document.Document{
		Locale:     tag,
		Group:      "main",
		Slug:       slug,
		Title:      name,
		Source:     source,
		HTML:       s.parser.RenderWithLocale(source, tag),
		Author:     user.Username,
		Summary:    "Activate " + value.Name + " language edition",
		EditPolicy: editPolicy,
		CreatedAt:  existing.CreatedAt,
	}
	if _, err := s.store.SaveIfRevision(r.Context(), doc, existing.Revision); err != nil {
		http.Error(w, "failed to create main page", http.StatusInternalServerError)
		return
	}
	if _, err := s.store.UpdateInterfaceMessages(r.Context(), tag, req.InterfaceMessages); err != nil {
		http.Error(w, "main page saved but interface messages failed validation", http.StatusBadRequest)
		return
	}
	if _, err := s.store.UpdateNavigation(r.Context(), tag, req.Navigation); err != nil {
		http.Error(w, "main page saved but navigation failed validation", http.StatusBadRequest)
		return
	}
	value, err = s.store.ActivateEdition(r.Context(), tag, name, time.Now().UTC())
	if errors.Is(err, edition.ErrInvalid) {
		http.Error(w, "main page saved but interface messages or navigation are incomplete", http.StatusBadRequest)
		return
	}
	if err != nil {
		s.recordAudit(r.Context(), user.Username, "edition.activate", "edition", tag, err.Error(), "failed")
		http.Error(w, "main page saved but language edition activation failed", http.StatusInternalServerError)
		return
	}
	s.recordAudit(r.Context(), user.Username, "edition.activate", "edition", tag, name, "success")
	writeJSON(w, http.StatusOK, newEditionResponse(value))
}

func (s *Server) handleUpdateEditionInterface(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionAdmin)
	if !ok {
		return
	}
	tag, valid := document.CanonicalLocale(r.PathValue("locale"))
	if !valid {
		http.Error(w, "invalid language edition code", http.StatusBadRequest)
		return
	}
	var req editionInterfaceRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	value, err := s.store.UpdateInterfaceMessages(r.Context(), tag, req.InterfaceMessages)
	if errors.Is(err, edition.ErrNotFound) {
		http.Error(w, "language edition not found", http.StatusNotFound)
		return
	}
	if errors.Is(err, edition.ErrInvalid) {
		http.Error(w, "invalid interface messages", http.StatusBadRequest)
		return
	}
	if err != nil {
		s.recordAudit(r.Context(), user.Username, "edition.interface", "edition", tag, err.Error(), "failed")
		http.Error(w, "failed to update interface messages", http.StatusInternalServerError)
		return
	}
	s.recordAudit(r.Context(), user.Username, "edition.interface", "edition", tag, "interface messages", "success")
	writeJSON(w, http.StatusOK, newEditionResponse(value))
}

func (s *Server) handleTranslateEditionInterface(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionAdmin)
	if !ok {
		return
	}
	tag, valid := document.CanonicalLocale(r.PathValue("locale"))
	if !valid {
		http.Error(w, "invalid language edition code", http.StatusBadRequest)
		return
	}
	if s.interfaceTranslator == nil {
		http.Error(w, "automatic interface translation is disabled", http.StatusServiceUnavailable)
		return
	}
	if value, err := s.store.GetEdition(r.Context(), tag); errors.Is(err, edition.ErrNotFound) {
		http.Error(w, "language edition not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "failed to load language edition", http.StatusInternalServerError)
		return
	} else if !value.Configurable() {
		http.Error(w, "language edition must be approved before translation", http.StatusConflict)
		return
	}
	value, filled, err := s.translateEditionInterface(r.Context(), tag)
	if err != nil {
		s.recordAudit(r.Context(), user.Username, "edition.interface_translate", "edition", tag, "", "failed")
		log.Printf("interface translation for %s failed: %v", tag, err)
		http.Error(w, "automatic interface translation is temporarily unavailable", http.StatusBadGateway)
		return
	}
	s.recordAudit(r.Context(), user.Username, "edition.interface_translate", "edition", tag, "filled="+strconv.Itoa(filled), "success")
	response := newEditionResponse(value)
	response.InterfaceTranslationStatus = "complete"
	response.InterfaceTranslationFilled = filled
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) translateEditionInterface(ctx context.Context, tag string) (edition.Edition, int, error) {
	if s.interfaceTranslator == nil {
		return edition.Edition{}, 0, translation.ErrTranslationUnavailable
	}
	messages, err := s.interfaceTranslator.TranslateInterface(ctx, tag)
	if err != nil {
		return edition.Edition{}, 0, err
	}
	return s.store.FillInterfaceMessages(ctx, tag, messages)
}

func (s *Server) handleUpdateEditionNavigation(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionAdmin)
	if !ok {
		return
	}
	tag, valid := document.CanonicalLocale(r.PathValue("locale"))
	if !valid {
		http.Error(w, "invalid language edition code", http.StatusBadRequest)
		return
	}
	var req editionNavigationRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	value, err := s.store.UpdateNavigation(r.Context(), tag, req.Navigation)
	if errors.Is(err, edition.ErrNotFound) {
		http.Error(w, "language edition not found", http.StatusNotFound)
		return
	}
	if errors.Is(err, edition.ErrInvalid) {
		http.Error(w, "invalid edition navigation", http.StatusBadRequest)
		return
	}
	if err != nil {
		s.recordAudit(r.Context(), user.Username, "edition.navigation", "edition", tag, err.Error(), "failed")
		http.Error(w, "failed to update edition navigation", http.StatusInternalServerError)
		return
	}
	s.recordAudit(r.Context(), user.Username, "edition.navigation", "edition", tag, "navigation", "success")
	writeJSON(w, http.StatusOK, newEditionResponse(value))
}

func (s *Server) handleUpdateEditionPortal(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionAdmin)
	if !ok {
		return
	}
	tag, valid := document.CanonicalLocale(r.PathValue("locale"))
	if !valid {
		http.Error(w, "invalid language edition code", http.StatusBadRequest)
		return
	}
	var req editionPortalRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	value, err := s.store.UpdatePortal(r.Context(), tag, req.PortalOrder, req.CountryCodes)
	if errors.Is(err, edition.ErrNotFound) {
		http.Error(w, "language edition not found", http.StatusNotFound)
		return
	}
	if errors.Is(err, edition.ErrInvalid) {
		http.Error(w, "invalid portal order or country code", http.StatusBadRequest)
		return
	}
	if err != nil {
		s.recordAudit(r.Context(), user.Username, "edition.portal", "edition", tag, err.Error(), "failed")
		http.Error(w, "failed to update language portal metadata", http.StatusInternalServerError)
		return
	}
	s.recordAudit(r.Context(), user.Username, "edition.portal", "edition", tag, "portal metadata", "success")
	writeJSON(w, http.StatusOK, newEditionResponse(value))
}

func (s *Server) requireActiveEdition(w http.ResponseWriter, r *http.Request, tag string) bool {
	_, err := s.reader.ActiveEdition(r.Context(), tag)
	if err != nil {
		writeReadError(w, err)
		return false
	}
	return true
}
