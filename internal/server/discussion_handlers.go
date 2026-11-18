package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"techpedia/internal/auth"
	"techpedia/internal/discussion"
	"techpedia/internal/storage"
)

type discussionWriteRequest struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type discussionTopicResponse struct {
	ID         string `json:"id"`
	Locale     string `json:"locale"`
	PageTitle  string `json:"page_title"`
	PageURL    string `json:"page_url"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
	Author     string `json:"author"`
	ReplyCount int    `json:"reply_count"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	Archived   bool   `json:"archived"`
	ArchivedAt string `json:"archived_at,omitempty"`
	CanArchive bool   `json:"can_archive"`
	CanDelete  bool   `json:"can_delete"`
}

type discussionReplyResponse struct {
	ID        string `json:"id"`
	Body      string `json:"body"`
	Author    string `json:"author"`
	CreatedAt string `json:"created_at"`
	CanDelete bool   `json:"can_delete"`
}

type discussionThreadResponse struct {
	Topic   discussionTopicResponse   `json:"topic"`
	Replies []discussionReplyResponse `json:"replies"`
}

func (s *Server) handleListDiscussionTopics(w http.ResponseWriter, r *http.Request) {
	locale, slug, ok := pageLocaleSlug(r)
	if !ok {
		http.Error(w, "invalid locale", http.StatusBadRequest)
		return
	}
	if !s.requireActiveEdition(w, r, locale) {
		return
	}
	if _, err := s.store.GetLatest(r.Context(), locale, slug); err != nil {
		if errors.Is(err, storage.ErrDocumentNotFound) {
			http.Error(w, "document not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	topics, err := s.store.ListDiscussionTopics(r.Context(), locale, slug, 100)
	if err != nil {
		http.Error(w, "failed to load discussion topics", http.StatusInternalServerError)
		return
	}
	user, _ := s.currentUser(r)
	response := make([]discussionTopicResponse, 0, len(topics))
	for _, topic := range topics {
		response = append(response, newDiscussionTopicResponse(topic, user))
	}
	writeJSON(w, http.StatusOK, map[string]any{"topics": response})
}

func (s *Server) handleCreateDiscussionTopic(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "login required", http.StatusUnauthorized)
		return
	}
	locale, slug, ok := pageLocaleSlug(r)
	if !ok {
		http.Error(w, "invalid locale", http.StatusBadRequest)
		return
	}
	if !s.requireActiveEdition(w, r, locale) {
		return
	}
	doc, err := s.store.GetLatest(r.Context(), locale, slug)
	if err != nil {
		if errors.Is(err, storage.ErrDocumentNotFound) {
			http.Error(w, "document not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var req discussionWriteRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	id, err := discussion.NewID("dt_")
	if err != nil {
		http.Error(w, "failed to create discussion id", http.StatusInternalServerError)
		return
	}
	topic, err := s.store.CreateDiscussionTopic(r.Context(), discussion.Topic{
		ID: id, Locale: locale, Slug: slug, PageTitle: doc.Title,
		Subject: req.Subject, Body: req.Body, AuthorID: user.ID, Author: user.DisplayName,
	})
	if errors.Is(err, discussion.ErrInvalid) {
		http.Error(w, "subject and body are required", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "failed to create discussion topic", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, newDiscussionTopicResponse(topic, user))
}

func (s *Server) handleGetDiscussionThread(w http.ResponseWriter, r *http.Request) {
	thread, err := s.store.GetDiscussionThread(r.Context(), r.PathValue("topicID"))
	if errors.Is(err, discussion.ErrNotFound) {
		http.Error(w, "discussion topic not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load discussion topic", http.StatusInternalServerError)
		return
	}
	user, _ := s.currentUser(r)
	writeJSON(w, http.StatusOK, newDiscussionThreadResponse(thread, user))
}

func (s *Server) handleCreateDiscussionReply(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "login required", http.StatusUnauthorized)
		return
	}
	var req discussionWriteRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	id, err := discussion.NewID("dr_")
	if err != nil {
		http.Error(w, "failed to create reply id", http.StatusInternalServerError)
		return
	}
	reply, err := s.store.AddDiscussionReply(r.Context(), discussion.Reply{
		ID: id, TopicID: r.PathValue("topicID"), Body: req.Body, AuthorID: user.ID, Author: user.DisplayName,
	})
	if errors.Is(err, discussion.ErrInvalid) {
		http.Error(w, "reply body is required", http.StatusBadRequest)
		return
	}
	if errors.Is(err, discussion.ErrNotFound) {
		http.Error(w, "discussion topic not found", http.StatusNotFound)
		return
	}
	if errors.Is(err, discussion.ErrArchived) {
		http.Error(w, "discussion topic is archived", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "failed to create discussion reply", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, newDiscussionReplyResponse(reply, user, false))
}

func (s *Server) handleArchiveDiscussionTopic(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "login required", http.StatusUnauthorized)
		return
	}
	thread, err := s.store.GetDiscussionThread(r.Context(), r.PathValue("topicID"))
	if errors.Is(err, discussion.ErrNotFound) {
		http.Error(w, "discussion topic not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load discussion topic", http.StatusInternalServerError)
		return
	}
	if thread.Topic.AuthorID != user.ID && !user.Has(auth.PermissionAdmin) {
		http.Error(w, "discussion owner or administrator required", http.StatusForbidden)
		return
	}
	topic, err := s.store.ArchiveDiscussionTopic(r.Context(), thread.Topic.ID, user.ID)
	if err != nil {
		http.Error(w, "failed to archive discussion topic", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, newDiscussionTopicResponse(topic, user))
}

func (s *Server) handleDeleteDiscussionTopic(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, auth.PermissionAdmin); !ok {
		return
	}
	if err := s.store.DeleteDiscussionTopic(r.Context(), r.PathValue("topicID")); errors.Is(err, discussion.ErrNotFound) {
		http.Error(w, "discussion topic not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "failed to delete discussion topic", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteDiscussionReply(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "login required", http.StatusUnauthorized)
		return
	}
	thread, err := s.store.GetDiscussionThread(r.Context(), r.PathValue("topicID"))
	if errors.Is(err, discussion.ErrNotFound) {
		http.Error(w, "discussion topic not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load discussion topic", http.StatusInternalServerError)
		return
	}
	var target *discussion.Reply
	for index := range thread.Replies {
		if thread.Replies[index].ID == r.PathValue("replyID") {
			target = &thread.Replies[index]
			break
		}
	}
	if target == nil {
		http.Error(w, "discussion reply not found", http.StatusNotFound)
		return
	}
	if target.AuthorID != user.ID && !user.Has(auth.PermissionAdmin) {
		http.Error(w, "discussion owner or administrator required", http.StatusForbidden)
		return
	}
	if !thread.Topic.ArchivedAt.IsZero() && !user.Has(auth.PermissionAdmin) {
		http.Error(w, "archived discussions are read-only", http.StatusConflict)
		return
	}
	if err := s.store.DeleteDiscussionReply(r.Context(), thread.Topic.ID, target.ID); err != nil {
		http.Error(w, "failed to delete discussion reply", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminDiscussionTopics(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionAdmin)
	if !ok {
		return
	}
	topics, err := s.store.ListAllDiscussionTopics(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")), 200)
	if err != nil {
		http.Error(w, "failed to load discussion topics", http.StatusInternalServerError)
		return
	}
	response := make([]discussionTopicResponse, 0, len(topics))
	for _, topic := range topics {
		response = append(response, newDiscussionTopicResponse(topic, user))
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(response), "topics": response})
}

func (s *Server) handleAdminDeleteDiscussionTopic(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, auth.PermissionAdmin); !ok {
		return
	}
	if err := s.store.DeleteDiscussionTopic(r.Context(), r.PathValue("topicID")); errors.Is(err, discussion.ErrNotFound) {
		http.Error(w, "discussion topic not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "failed to delete discussion topic", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func newDiscussionTopicResponse(topic discussion.Topic, user auth.User) discussionTopicResponse {
	archivedAt := ""
	if !topic.ArchivedAt.IsZero() {
		archivedAt = topic.ArchivedAt.UTC().Format(time.RFC3339)
	}
	return discussionTopicResponse{
		ID: topic.ID, Locale: topic.Locale, PageTitle: topic.PageTitle,
		PageURL: wikiURL(topic.Locale, topic.PageTitle), Subject: topic.Subject, Body: topic.Body,
		Author: topic.Author, ReplyCount: topic.ReplyCount,
		CreatedAt: topic.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: topic.UpdatedAt.UTC().Format(time.RFC3339),
		Archived: !topic.ArchivedAt.IsZero(), ArchivedAt: archivedAt,
		CanArchive: topic.ArchivedAt.IsZero() && user.ID != "" && (user.ID == topic.AuthorID || user.Has(auth.PermissionAdmin)),
		CanDelete:  user.Has(auth.PermissionAdmin),
	}
}

func newDiscussionReplyResponse(reply discussion.Reply, user auth.User, archived bool) discussionReplyResponse {
	return discussionReplyResponse{
		ID: reply.ID, Body: reply.Body, Author: reply.Author, CreatedAt: reply.CreatedAt.UTC().Format(time.RFC3339),
		CanDelete: user.ID != "" && ((!archived && user.ID == reply.AuthorID) || user.Has(auth.PermissionAdmin)),
	}
}

func newDiscussionThreadResponse(thread discussion.Thread, user auth.User) discussionThreadResponse {
	replies := make([]discussionReplyResponse, 0, len(thread.Replies))
	for _, reply := range thread.Replies {
		replies = append(replies, newDiscussionReplyResponse(reply, user, !thread.Topic.ArchivedAt.IsZero()))
	}
	return discussionThreadResponse{Topic: newDiscussionTopicResponse(thread.Topic, user), Replies: replies}
}
