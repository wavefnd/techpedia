package discussion

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"techpedia/internal/document"
)

var (
	ErrNotFound = errors.New("discussion not found")
	ErrInvalid  = errors.New("invalid discussion")
	ErrArchived = errors.New("discussion archived")
)

type Topic struct {
	ID         string    `json:"id"`
	Locale     string    `json:"locale"`
	Slug       string    `json:"slug"`
	PageTitle  string    `json:"page_title"`
	Subject    string    `json:"subject"`
	Body       string    `json:"body"`
	AuthorID   string    `json:"author_id"`
	Author     string    `json:"author"`
	ReplyCount int       `json:"reply_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	ArchivedAt time.Time `json:"archived_at,omitempty"`
	ArchivedBy string    `json:"archived_by,omitempty"`
}

type Reply struct {
	ID        string    `json:"id"`
	TopicID   string    `json:"topic_id"`
	Body      string    `json:"body"`
	AuthorID  string    `json:"author_id"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

type Thread struct {
	Topic   Topic
	Replies []Reply
}

type Store interface {
	CreateDiscussionTopic(ctx context.Context, topic Topic) (Topic, error)
	ListDiscussionTopics(ctx context.Context, locale string, slug string, limit int) ([]Topic, error)
	ListAllDiscussionTopics(ctx context.Context, query string, limit int) ([]Topic, error)
	GetDiscussionThread(ctx context.Context, topicID string) (Thread, error)
	AddDiscussionReply(ctx context.Context, reply Reply) (Reply, error)
	ArchiveDiscussionTopic(ctx context.Context, topicID string, archivedBy string) (Topic, error)
	DeleteDiscussionTopic(ctx context.Context, topicID string) error
	DeleteDiscussionReply(ctx context.Context, topicID string, replyID string) error
}

func NewID(prefix string) (string, error) {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(value), nil
}

func NormalizeTopic(value Topic) (Topic, error) {
	locale, ok := document.CanonicalLocale(value.Locale)
	if !ok {
		return Topic{}, ErrInvalid
	}
	value.Locale = locale
	value.ID = strings.TrimSpace(value.ID)
	value.Slug = strings.TrimSpace(value.Slug)
	value.PageTitle = strings.TrimSpace(value.PageTitle)
	value.Subject = strings.TrimSpace(value.Subject)
	value.Body = strings.TrimSpace(value.Body)
	value.AuthorID = strings.TrimSpace(value.AuthorID)
	value.Author = strings.TrimSpace(value.Author)
	if value.ID == "" || value.Slug == "" || value.PageTitle == "" || value.Subject == "" || value.Body == "" || value.AuthorID == "" || value.Author == "" {
		return Topic{}, ErrInvalid
	}
	if len([]rune(value.Subject)) > 160 || len([]rune(value.Body)) > 20000 || len([]rune(value.Author)) > 80 {
		return Topic{}, ErrInvalid
	}
	return value, nil
}

func NormalizeReply(value Reply) (Reply, error) {
	value.ID = strings.TrimSpace(value.ID)
	value.TopicID = strings.TrimSpace(value.TopicID)
	value.Body = strings.TrimSpace(value.Body)
	value.AuthorID = strings.TrimSpace(value.AuthorID)
	value.Author = strings.TrimSpace(value.Author)
	if value.ID == "" || value.TopicID == "" || value.Body == "" || value.AuthorID == "" || value.Author == "" {
		return Reply{}, ErrInvalid
	}
	if len([]rune(value.Body)) > 20000 || len([]rune(value.Author)) > 80 {
		return Reply{}, ErrInvalid
	}
	return value, nil
}
