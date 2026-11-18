package storage

import (
	"context"
	"time"

	"techpedia/internal/box"
	"techpedia/internal/document"
	"techpedia/internal/media"
)

type BoxStore interface{ box.Store }

// MediaStore includes owner-triggered metadata deletion, reference checks,
// name reservation and the atomic audit. Disk cleanup remains in media.Manager.
type MediaStore interface {
	media.Store
	media.DeletionStore
}

type PopularDocument struct {
	Document document.Document
	Views    uint64
}

type DocumentStore interface {
	Save(ctx context.Context, doc document.Document) (document.Document, error)
	SaveIfRevision(ctx context.Context, doc document.Document, baseRevision string) (document.Document, error)
	SaveTranslation(ctx context.Context, doc document.Document, sourceWID document.WID) (document.Document, error)
	MoveDocument(ctx context.Context, locale string, oldSlug string, newSlug string, newTitle string) (document.Document, error)
	MoveDocumentAs(ctx context.Context, locale string, oldSlug string, newSlug string, newTitle string, actor string) (document.Document, error)
	CreateDocumentRedirect(ctx context.Context, locale string, name string, targetSlug string, actor string) (document.Redirect, error)
	DeleteDocumentRedirect(ctx context.Context, locale string, slug string) (document.Redirect, error)
	ListDocumentRedirects(ctx context.Context, locale string, limit int) ([]document.Redirect, error)
	GetDocumentRedirect(ctx context.Context, locale string, slug string) (string, error)
	GetLatest(ctx context.Context, locale string, slug string) (document.Document, error)
	GetLatestByWID(ctx context.Context, wid document.WID) (document.Document, error)
	GetRevision(ctx context.Context, locale string, slug string, revision string) (document.Document, error)
	ListRevisions(ctx context.Context, locale string, slug string, limit int) ([]document.RevisionSummary, error)
	ListRecentRevisions(ctx context.Context, locale string, limit int) ([]document.RevisionSummary, error)
	ListChanges(ctx context.Context, filter document.ChangeFilter) ([]document.Change, error)
	RestoreRevision(ctx context.Context, locale string, slug string, revision string, baseRevision string, actor string, reason string) (document.Document, error)
	ResetDocumentHistory(ctx context.Context, locale string, slug string, baseRevision string, actor string) (document.HistoryResetResult, error)
	SaveDraft(ctx context.Context, draft document.Draft) (document.Draft, error)
	GetDraft(ctx context.Context, userID string, locale string, slug string) (document.Draft, error)
	DeleteDraft(ctx context.Context, userID string, locale string, slug string) error
	DeleteDocument(ctx context.Context, locale string, slug string, actor string, reason string) (document.TrashEntry, error)
	ListTrash(ctx context.Context, limit int) ([]document.TrashEntry, error)
	RestoreDeletedDocument(ctx context.Context, wid document.WID, actor string, reason string) (document.Document, error)
	AppendAudit(ctx context.Context, entry document.AuditEntry) (document.AuditEntry, error)
	ListAudit(ctx context.Context, filter document.AuditFilter) ([]document.AuditEntry, error)
	GetTranslations(ctx context.Context, group string) ([]document.Document, error)
	Search(ctx context.Context, locale string, query string, limit int) ([]document.SearchResult, error)
	Exists(ctx context.Context, locale string, slug string) (document.Document, bool, error)
	ListLatest(ctx context.Context, locale string, limit int) ([]document.Document, error)
	ListAllLatest(ctx context.Context, locale string) ([]document.Document, error)
	ListLinkDocuments(ctx context.Context, locale string) ([]document.LinkDocument, error)
	ListBacklinks(ctx context.Context, locale string, slug string) ([]document.Document, error)
	CountLatestByLocale(ctx context.Context) (map[string]int, error)
	RandomLatest(ctx context.Context, locale string) (document.Document, error)
	RecordPageView(ctx context.Context, locale string, slug string, viewedAt time.Time) error
	ListPopular(ctx context.Context, locale string, since time.Time, limit int) ([]PopularDocument, error)
	Close() error
}
