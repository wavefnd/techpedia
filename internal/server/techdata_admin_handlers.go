package server

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"techpedia/internal/auth"
	"techpedia/internal/document"
	"techpedia/internal/storage"
	"techpedia/internal/techdata"
)

type pageTechdataLinkRequest struct {
	TID             string `json:"tid"`
	ConfirmDistinct bool   `json:"confirm_distinct"`
}

type deleteTechdataItemRequest struct {
	Confirmation string `json:"confirmation"`
}

func (s *Server) handleListTechdataItems(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, auth.PermissionTechdata); !ok {
		return
	}
	items, err := s.store.ListItems(r.Context())
	if err != nil {
		http.Error(w, "failed to load techdata items", http.StatusInternalServerError)
		return
	}
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	filtered := make([]techdata.Item, 0, len(items))
	for _, item := range items {
		if query != "" && !techdataItemMatches(item, query) {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt) })
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": filtered})
}

func techdataItemMatches(item techdata.Item, query string) bool {
	if strings.Contains(strings.ToLower(item.ID.String()), query) {
		return true
	}
	for _, value := range item.Labels {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	for _, value := range item.Descriptions {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	for _, aliases := range item.Aliases {
		for _, value := range aliases {
			if strings.Contains(strings.ToLower(value), query) {
				return true
			}
		}
	}
	return false
}

func (s *Server) handleDeleteTechdataItem(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor, ok := s.requirePermission(w, r, auth.PermissionOwner)
	if !ok {
		return
	}
	id, err := techdata.ParseTID(r.PathValue("tid"))
	if err != nil {
		http.Error(w, "invalid TID", http.StatusBadRequest)
		return
	}
	var req deleteTechdataItemRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Confirmation) != id.String() {
		http.Error(w, "type the exact TID to confirm deletion", http.StatusBadRequest)
		return
	}
	if _, err := s.store.DeleteItem(r.Context(), id); err != nil {
		s.recordAudit(r.Context(), actor.Username, "techdata.item_delete", "techdata", id.String(), "", "failed")
		switch {
		case errors.Is(err, techdata.ErrItemNotFound):
			http.Error(w, "techdata item not found", http.StatusNotFound)
		case errors.Is(err, techdata.ErrItemInUse):
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "techdata item still has document links, statements, or incoming references",
				"code":  "techdata_item_in_use",
			})
		default:
			http.Error(w, "failed to delete techdata item", http.StatusInternalServerError)
		}
		return
	}
	s.recordAudit(r.Context(), actor.Username, "techdata.item_delete", "techdata", id.String(), "", "success")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLinkPageTechdata(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, auth.PermissionTechdata)
	if !ok {
		return
	}
	locale, slug, ok := pageLocaleSlug(r)
	if !ok || slug == "" {
		http.Error(w, "invalid document name", http.StatusBadRequest)
		return
	}
	doc, err := s.store.GetLatest(r.Context(), locale, slug)
	if errors.Is(err, storage.ErrDocumentNotFound) {
		http.Error(w, "document not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load document", http.StatusInternalServerError)
		return
	}
	if _, err := s.store.GetItemByWikiPage(r.Context(), doc.ID); err == nil {
		http.Error(w, "document is already linked to a techdata item", http.StatusConflict)
		return
	} else if !errors.Is(err, techdata.ErrItemNotFound) {
		http.Error(w, "failed to inspect techdata link", http.StatusInternalServerError)
		return
	}
	var req pageTechdataLinkRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	var item techdata.Item
	if strings.TrimSpace(req.TID) == "" {
		item, err = s.store.CreateItem(r.Context(), techdata.Item{
			Labels:    map[string]string{locale: doc.Title},
			WikiPages: map[string]document.WID{locale: doc.ID},
		}, techdata.CreateItemOptions{ConfirmDistinct: req.ConfirmDistinct})
	} else {
		id, parseErr := techdata.ParseTID(req.TID)
		if parseErr != nil {
			http.Error(w, "invalid TID", http.StatusBadRequest)
			return
		}
		item, err = s.store.AttachWikiPage(r.Context(), id, locale, doc.ID, doc.Title)
	}
	if errors.Is(err, techdata.ErrItemNotFound) {
		http.Error(w, "techdata item not found", http.StatusNotFound)
		return
	}
	if errors.Is(err, techdata.ErrLocaleOccupied) || errors.Is(err, techdata.ErrWikiPageLinked) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if writeTechdataDuplicateConflict(w, err) {
		s.recordAudit(r.Context(), actor.Username, "techdata.create_duplicate_blocked", "document", locale+":"+slug, err.Error(), "blocked")
		return
	}
	if err != nil {
		s.recordAudit(r.Context(), actor.Username, "techdata.link", "document", locale+":"+slug, err.Error(), "failed")
		http.Error(w, "failed to link techdata item", http.StatusInternalServerError)
		return
	}
	s.recordAudit(r.Context(), actor.Username, "techdata.link", "document", locale+":"+slug, item.ID.String(), "success")
	writeJSON(w, http.StatusOK, item)
}

func writeTechdataDuplicateConflict(w http.ResponseWriter, err error) bool {
	var duplicate *techdata.DuplicateItemError
	if !errors.As(err, &duplicate) {
		return false
	}
	writeJSON(w, http.StatusConflict, map[string]any{
		"error":      "potential duplicate Techdata item found",
		"code":       "techdata_duplicate_candidate",
		"candidates": duplicate.Candidates,
	})
	return true
}

func (s *Server) handleUnlinkPageTechdata(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, auth.PermissionTechdata)
	if !ok {
		return
	}
	locale, slug, ok := pageLocaleSlug(r)
	if !ok || slug == "" {
		http.Error(w, "invalid document name", http.StatusBadRequest)
		return
	}
	doc, err := s.store.GetLatest(r.Context(), locale, slug)
	if errors.Is(err, storage.ErrDocumentNotFound) {
		http.Error(w, "document not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load document", http.StatusInternalServerError)
		return
	}
	item, err := s.store.GetItemByWikiPage(r.Context(), doc.ID)
	if errors.Is(err, techdata.ErrItemNotFound) {
		http.Error(w, "document is not linked to techdata", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to inspect techdata link", http.StatusInternalServerError)
		return
	}
	if _, err := s.store.DetachWikiPage(r.Context(), item.ID, locale); err != nil {
		s.recordAudit(r.Context(), actor.Username, "techdata.unlink", "document", locale+":"+slug, err.Error(), "failed")
		http.Error(w, "failed to unlink techdata item", http.StatusInternalServerError)
		return
	}
	s.recordAudit(r.Context(), actor.Username, "techdata.unlink", "document", locale+":"+slug, item.ID.String(), "success")
	w.WriteHeader(http.StatusNoContent)
}
