package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"techpedia/internal/auth"
	"techpedia/internal/document"
	"techpedia/internal/storage"
)

type revisionDiffLine struct {
	Kind    string `json:"kind"`
	OldLine int    `json:"old_line,omitempty"`
	NewLine int    `json:"new_line,omitempty"`
	Text    string `json:"text"`
}

type draftRequest struct {
	Name         string `json:"name"`
	Source       string `json:"source"`
	Summary      string `json:"summary"`
	BaseRevision string `json:"base_revision"`
}

type restoreRevisionRequest struct {
	Revision     string `json:"revision"`
	BaseRevision string `json:"base_revision"`
	Reason       string `json:"reason"`
}

type reasonRequest struct {
	Reason string `json:"reason"`
}

type resetDocumentHistoryRequest struct {
	BaseRevision string `json:"base_revision"`
	Confirmation string `json:"confirmation"`
}

func (s *Server) handleRevisionDiff(w http.ResponseWriter, r *http.Request) {
	setRevisionPrivacyHeaders(w)
	locale, slug, ok := pageLocaleSlug(r)
	if !ok || slug == "" {
		http.Error(w, "invalid document name", http.StatusBadRequest)
		return
	}
	if !s.requireActiveEdition(w, r, locale) {
		return
	}
	current, err := s.store.GetLatest(r.Context(), locale, slug)
	if err != nil {
		http.Error(w, "document not found", http.StatusNotFound)
		return
	}
	if current.EditPolicy == document.EditPolicyAdmins {
		user, authErr := s.currentUser(r)
		if authErr != nil || !user.Has(auth.PermissionAdmin) {
			http.Error(w, "administrator permission required", http.StatusForbidden)
			return
		}
	}
	from, to := strings.TrimSpace(r.URL.Query().Get("from")), strings.TrimSpace(r.URL.Query().Get("to"))
	if !isPublicRevision(from) || !isPublicRevision(to) || from == to {
		http.Error(w, "two different public revisions are required", http.StatusBadRequest)
		return
	}
	oldDoc, err := s.store.GetRevision(r.Context(), locale, slug, from)
	if err != nil {
		http.Error(w, "revision not found", http.StatusNotFound)
		return
	}
	newDoc, err := s.store.GetRevision(r.Context(), locale, slug, to)
	if err != nil {
		http.Error(w, "revision not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "lines": lineDiff(oldDoc.Source, newDoc.Source)})
}

func setRevisionPrivacyHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Robots-Tag", "noindex,noarchive")
}

func lineDiff(oldSource string, newSource string) []revisionDiffLine {
	oldLines, newLines := strings.Split(oldSource, "\n"), strings.Split(newSource, "\n")
	if len(oldLines)*len(newLines) > 2_000_000 {
		return boundedLineDiff(oldLines, newLines)
	}
	dp := make([][]int, len(oldLines)+1)
	for i := range dp {
		dp[i] = make([]int, len(newLines)+1)
	}
	for i := len(oldLines) - 1; i >= 0; i-- {
		for j := len(newLines) - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	result := make([]revisionDiffLine, 0, len(oldLines)+len(newLines))
	for i, j := 0, 0; i < len(oldLines) || j < len(newLines); {
		if i < len(oldLines) && j < len(newLines) && oldLines[i] == newLines[j] {
			result = append(result, revisionDiffLine{Kind: "equal", OldLine: i + 1, NewLine: j + 1, Text: oldLines[i]})
			i++
			j++
		} else if j < len(newLines) && (i == len(oldLines) || dp[i][j+1] > dp[i+1][j]) {
			result = append(result, revisionDiffLine{Kind: "added", NewLine: j + 1, Text: newLines[j]})
			j++
		} else {
			result = append(result, revisionDiffLine{Kind: "removed", OldLine: i + 1, Text: oldLines[i]})
			i++
		}
	}
	return result
}

func boundedLineDiff(oldLines []string, newLines []string) []revisionDiffLine {
	prefix := 0
	for prefix < len(oldLines) && prefix < len(newLines) && oldLines[prefix] == newLines[prefix] {
		prefix++
	}
	oldEnd, newEnd := len(oldLines), len(newLines)
	for oldEnd > prefix && newEnd > prefix && oldLines[oldEnd-1] == newLines[newEnd-1] {
		oldEnd--
		newEnd--
	}
	result := make([]revisionDiffLine, 0, len(oldLines)+len(newLines))
	for i := 0; i < prefix; i++ {
		result = append(result, revisionDiffLine{Kind: "equal", OldLine: i + 1, NewLine: i + 1, Text: oldLines[i]})
	}
	for i := prefix; i < oldEnd; i++ {
		result = append(result, revisionDiffLine{Kind: "removed", OldLine: i + 1, Text: oldLines[i]})
	}
	for i := prefix; i < newEnd; i++ {
		result = append(result, revisionDiffLine{Kind: "added", NewLine: i + 1, Text: newLines[i]})
	}
	for i, j := oldEnd, newEnd; i < len(oldLines) && j < len(newLines); i, j = i+1, j+1 {
		result = append(result, revisionDiffLine{Kind: "equal", OldLine: i + 1, NewLine: j + 1, Text: oldLines[i]})
	}
	return result
}

func (s *Server) handleRestoreRevision(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionEdit)
	if !ok {
		return
	}
	locale, slug, ok := pageLocaleSlug(r)
	if !ok || slug == "" {
		http.Error(w, "invalid document name", http.StatusBadRequest)
		return
	}
	current, err := s.store.GetLatest(r.Context(), locale, slug)
	if errors.Is(err, storage.ErrDocumentNotFound) {
		http.Error(w, "document not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load document", http.StatusInternalServerError)
		return
	}
	if current.EditPolicy == document.EditPolicyAdmins && !user.Has(auth.PermissionAdmin) {
		http.Error(w, "administrator permission required", http.StatusForbidden)
		return
	}
	var req restoreRevisionRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !isPublicRevision(req.Revision) {
		http.Error(w, "invalid revision", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.BaseRevision) != current.Revision {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "document changed before the restore", "current_revision": current.Revision})
		return
	}
	restored, err := s.store.RestoreRevision(r.Context(), locale, slug, req.Revision, current.Revision, user.Username, req.Reason)
	if errors.Is(err, storage.ErrRevisionConflict) {
		if latest, latestErr := s.store.GetLatest(r.Context(), locale, slug); latestErr == nil {
			current = latest
		}
		writeJSON(w, http.StatusConflict, map[string]any{"error": "document changed before the restore", "current_revision": current.Revision})
		return
	}
	if errors.Is(err, storage.ErrDocumentNotFound) {
		http.Error(w, "revision not found", http.StatusNotFound)
		return
	}
	if err != nil {
		s.recordAudit(r.Context(), user.Username, "revision.restore", "document", locale+":"+slug, req.Revision, "failed")
		http.Error(w, "failed to restore revision", http.StatusInternalServerError)
		return
	}
	s.recordAudit(r.Context(), user.Username, "revision.restore", "document", locale+":"+slug, req.Revision+" -> "+restored.Revision, "success")
	writeJSON(w, http.StatusOK, adminPageResponse{Locale: restored.Locale, Name: restored.Title, Source: restored.Source, EditPolicy: restored.EditPolicy, Revision: restored.Revision, UpdatedAt: restored.UpdatedAt.Format(time.RFC3339)})
}

func (s *Server) handleResetDocumentHistory(w http.ResponseWriter, r *http.Request) {
	setRevisionPrivacyHeaders(w)
	user, ok := s.requirePermission(w, r, auth.PermissionOwner)
	if !ok {
		return
	}
	locale, slug, ok := pageLocaleSlug(r)
	if !ok || slug == "" {
		http.Error(w, "invalid document name", http.StatusBadRequest)
		return
	}
	var req resetDocumentHistoryRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	current, err := s.store.GetLatest(r.Context(), locale, slug)
	if errors.Is(err, storage.ErrDocumentNotFound) {
		http.Error(w, "document not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load document", http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(req.Confirmation) != current.Title {
		http.Error(w, "document name confirmation does not match", http.StatusBadRequest)
		return
	}

	result, err := s.store.ResetDocumentHistory(r.Context(), locale, slug, req.BaseRevision, user.Username)
	if errors.Is(err, storage.ErrRevisionConflict) {
		latest, latestErr := s.store.GetLatest(r.Context(), locale, slug)
		if latestErr == nil {
			current = latest
		}
		writeJSON(w, http.StatusConflict, map[string]any{"error": "document changed before history reset", "current_revision": current.Revision})
		return
	}
	if errors.Is(err, storage.ErrDocumentNotFound) {
		http.Error(w, "document not found", http.StatusNotFound)
		return
	}
	if err != nil {
		s.recordAudit(r.Context(), user.Username, "document.history_reset", "document", locale+":"+slug, "", "failed")
		http.Error(w, "failed to reset document history", http.StatusInternalServerError)
		return
	}

	s.invalidateGraphCache()
	s.parser.ClearCache()
	detail := fmt.Sprintf("revisions=%d changes=%d drafts=%d suggestions=%d", result.RemovedRevisions, result.RemovedChanges, result.RemovedDrafts, result.RemovedSuggestions)
	s.recordAudit(r.Context(), user.Username, "document.history_reset", "document", locale+":"+slug, detail, "success")
	w.Header().Set("Clear-Site-Data", `"cache"`)
	writeJSON(w, http.StatusOK, map[string]any{
		"locale": locale, "name": result.Document.Title, "url": wikiURL(locale, result.Document.Title),
		"revision": result.Document.Revision, "updated_at": result.Document.UpdatedAt.Format(time.RFC3339),
		"removed_revisions": result.RemovedRevisions, "removed_changes": result.RemovedChanges,
		"removed_drafts": result.RemovedDrafts, "removed_suggestions": result.RemovedSuggestions,
	})
}

func (s *Server) handleGetDraft(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionEdit)
	if !ok {
		return
	}
	locale, slug, valid := pageLocaleSlug(r)
	if !valid || slug == "" {
		http.Error(w, "invalid document name", http.StatusBadRequest)
		return
	}
	draft, err := s.store.GetDraft(r.Context(), user.ID, locale, slug)
	if errors.Is(err, storage.ErrDocumentNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"draft": nil})
		return
	}
	if err != nil {
		http.Error(w, "failed to load draft", http.StatusInternalServerError)
		return
	}
	draft.UserID = ""
	writeJSON(w, http.StatusOK, map[string]any{"draft": draft})
}

func (s *Server) handleSaveDraft(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionEdit)
	if !ok {
		return
	}
	locale, slug, valid := pageLocaleSlug(r)
	if !valid || slug == "" {
		http.Error(w, "invalid document name", http.StatusBadRequest)
		return
	}
	var req draftRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	draft, err := s.store.SaveDraft(r.Context(), document.Draft{UserID: user.ID, Locale: locale, Slug: slug, Name: strings.TrimSpace(req.Name), Source: req.Source, Summary: strings.TrimSpace(req.Summary), BaseRevision: strings.TrimSpace(req.BaseRevision)})
	if err != nil {
		http.Error(w, "failed to save draft", http.StatusInternalServerError)
		return
	}
	draft.UserID = ""
	writeJSON(w, http.StatusOK, draft)
}

func (s *Server) handleDeleteDraft(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionEdit)
	if !ok {
		return
	}
	locale, slug, valid := pageLocaleSlug(r)
	if !valid || slug == "" {
		http.Error(w, "invalid document name", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteDraft(r.Context(), user.ID, locale, slug); err != nil {
		http.Error(w, "failed to delete draft", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRecentChanges(w http.ResponseWriter, r *http.Request) {
	filter, ok := changeFilterFromRequest(r)
	if !ok {
		http.Error(w, "invalid recent changes filter", http.StatusBadRequest)
		return
	}
	changes, err := s.store.ListChanges(r.Context(), filter)
	if err != nil {
		http.Error(w, "failed to load recent changes", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changes": publicChanges(changes)})
}

func (s *Server) handleAccountContributions(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "login required", http.StatusUnauthorized)
		return
	}
	filter, ok := changeFilterFromRequest(r)
	if !ok {
		http.Error(w, "invalid contribution filter", http.StatusBadRequest)
		return
	}
	filter.Author = user.Username
	changes, err := s.store.ListChanges(r.Context(), filter)
	if err != nil {
		http.Error(w, "failed to load contributions", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changes": publicChanges(changes)})
}

func changeFilterFromRequest(r *http.Request) (document.ChangeFilter, bool) {
	filter := document.ChangeFilter{Locale: strings.TrimSpace(r.URL.Query().Get("locale")), Author: strings.TrimSpace(r.URL.Query().Get("user")), Query: strings.TrimSpace(r.URL.Query().Get("q")), Action: document.ChangeAction(strings.TrimSpace(r.URL.Query().Get("action")))}
	if filter.Locale != "" {
		locale, ok := document.CanonicalLocale(filter.Locale)
		if !ok {
			return filter, false
		}
		filter.Locale = locale
	}
	if filter.Action != "" && filter.Action != document.ChangeCreate && filter.Action != document.ChangeEdit && filter.Action != document.ChangeMove && filter.Action != document.ChangeRestore && filter.Action != document.ChangeDelete {
		return filter, false
	}
	if value := r.URL.Query().Get("since"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return filter, false
		}
		filter.Since = parsed
	}
	if value := r.URL.Query().Get("until"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return filter, false
		}
		filter.Until = parsed
	}
	filter.Limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	return filter, true
}

func publicChanges(changes []document.Change) []map[string]any {
	result := make([]map[string]any, 0, len(changes))
	for _, change := range changes {
		result = append(result, map[string]any{"locale": change.Locale, "name": change.Title, "url": wikiURL(change.Locale, change.Title), "revision": change.Revision, "author": change.Author, "summary": change.Summary, "action": change.Action, "created_at": change.CreatedAt.Format(time.RFC3339)})
	}
	return result
}

func (s *Server) handleDeletePage(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionOwner)
	if !ok {
		return
	}
	locale, slug, valid := pageLocaleSlug(r)
	if !valid || slug == "" {
		http.Error(w, "invalid document name", http.StatusBadRequest)
		return
	}
	main, err := s.isEditionMainPage(r.Context(), locale, slug)
	if err != nil {
		http.Error(w, "failed to inspect language edition", http.StatusInternalServerError)
		return
	}
	if main {
		http.Error(w, "language edition main page cannot be deleted", http.StatusConflict)
		return
	}
	var req reasonRequest
	if err := decodeOptionalJSON(r, &req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	entry, err := s.store.DeleteDocument(r.Context(), locale, slug, user.Username, req.Reason)
	if errors.Is(err, storage.ErrDocumentNotFound) {
		http.Error(w, "document not found", http.StatusNotFound)
		return
	}
	if err != nil {
		s.recordAudit(r.Context(), user.Username, "document.delete", "document", locale+":"+slug, req.Reason, "failed")
		http.Error(w, "failed to delete document", http.StatusInternalServerError)
		return
	}
	s.recordAudit(r.Context(), user.Username, "document.delete", "document", entry.Document.ID.String(), req.Reason, "success")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListTrash(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, auth.PermissionAdmin); !ok {
		return
	}
	entries, err := s.store.ListTrash(r.Context(), 300)
	if err != nil {
		http.Error(w, "failed to load trash", http.StatusInternalServerError)
		return
	}
	items := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		items = append(items, map[string]any{"wid": entry.Document.ID, "locale": entry.Document.Locale, "name": entry.Document.Title, "revision": entry.Document.Revision, "deleted_by": entry.DeletedBy, "reason": entry.Reason, "deleted_at": entry.DeletedAt.Format(time.RFC3339)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleRestoreDeletedPage(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionAdmin)
	if !ok {
		return
	}
	wid, err := document.ParseWID(r.PathValue("wid"))
	if err != nil {
		http.Error(w, "invalid WID", http.StatusBadRequest)
		return
	}
	var req reasonRequest
	if err := decodeOptionalJSON(r, &req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	restored, err := s.store.RestoreDeletedDocument(r.Context(), wid, user.Username, req.Reason)
	if errors.Is(err, storage.ErrDocumentNotFound) {
		http.Error(w, "trash item not found", http.StatusNotFound)
		return
	}
	if errors.Is(err, storage.ErrDocumentExists) {
		http.Error(w, "document name is already occupied", http.StatusConflict)
		return
	}
	if err != nil {
		s.recordAudit(r.Context(), user.Username, "document.restore", "document", wid.String(), req.Reason, "failed")
		http.Error(w, "failed to restore document", http.StatusInternalServerError)
		return
	}
	s.recordAudit(r.Context(), user.Username, "document.restore", "document", wid.String(), req.Reason, "success")
	writeJSON(w, http.StatusOK, map[string]any{"locale": restored.Locale, "name": restored.Title, "url": wikiURL(restored.Locale, restored.Title)})
}

func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, auth.PermissionAdmin); !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	entries, err := s.store.ListAudit(r.Context(), document.AuditFilter{Actor: strings.TrimSpace(r.URL.Query().Get("actor")), Action: strings.TrimSpace(r.URL.Query().Get("action")), Target: strings.TrimSpace(r.URL.Query().Get("target")), Limit: limit})
	if err != nil {
		http.Error(w, "failed to load audit log", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (s *Server) recordAudit(ctx context.Context, actor string, action string, targetType string, target string, detail string, result string) {
	if _, err := s.store.AppendAudit(ctx, document.AuditEntry{Actor: actor, Action: action, TargetType: targetType, Target: target, Detail: detail, Result: result}); err != nil {
		log.Printf("append audit log: %v", err)
	}
}

func decodeOptionalJSON(r *http.Request, target any) error {
	if r.Body == nil {
		return nil
	}
	err := json.NewDecoder(r.Body).Decode(target)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}
