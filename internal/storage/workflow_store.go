package storage

import (
	"context"
	cryptoRand "crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	badger "github.com/dgraph-io/badger/v4"

	"techpedia/internal/document"
)

const (
	changePrefix               = "workflow:v1:change:"
	changeRevisionMarkerPrefix = "workflow:v1:change-revision:"
	draftPrefix                = "workflow:v1:draft:"
	trashPrefix                = "workflow:v1:trash:"
	auditPrefix                = "workflow:v1:audit:"
)

func appendChangeTxn(txn *badger.Txn, change document.Change) error {
	if change.CreatedAt.IsZero() {
		change.CreatedAt = time.Now().UTC()
	}
	change.ID = orderedWorkflowID(change.CreatedAt)
	data, err := json.Marshal(change)
	if err != nil {
		return err
	}
	return txn.Set([]byte(changePrefix+change.ID), data)
}

func appendRevisionChangeTxn(txn *badger.Txn, change document.Change) error {
	if err := appendChangeTxn(txn, change); err != nil {
		return err
	}
	return txn.Set([]byte(changeRevisionMarkerKey(change.WID, change.Locale, change.Slug, change.Revision)), []byte{1})
}

func changeRevisionMarkerKey(wid string, locale string, slug string, revision string) string {
	identity := workflowPart(locale) + ":" + workflowPart(slug)
	if strings.TrimSpace(wid) != "" {
		identity = workflowPart(wid)
	}
	return changeRevisionMarkerPrefix + identity + ":" + workflowPart(revision)
}

func (s *BadgerDocumentStore) ensureWorkflowChanges() error {
	return s.migrateDocuments(revisionSummaryMigration, []string{revisionDocumentPrefixRoot, legacyRevisionDocumentRoot}, func(txn *badger.Txn, revision document.Document, key string) error {
		if strings.HasPrefix(key, legacyRevisionDocumentRoot) {
			if _, err := txn.Get([]byte(revisionDocumentKey(revision.Locale, revision.Slug, revision.Revision))); err == nil {
				return nil
			} else if !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
		}
		if revision.ID == "" {
			latest, err := documentFromTxn(txn, latestDocumentKey(revision.Locale, revision.Slug))
			if err != nil && !errors.Is(err, ErrDocumentNotFound) {
				return err
			}
			revision.ID = latest.ID
		}
		if err := setRevisionSummaryTxn(txn, revision); err != nil {
			return err
		}
		if _, err := txn.Get([]byte(changeRevisionMarkerKey(revision.ID.String(), revision.Locale, revision.Slug, revision.Revision))); err == nil {
			return nil
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		action := document.ChangeEdit
		if revision.Revision == "r1" {
			action = document.ChangeCreate
		}
		return appendRevisionChangeTxn(txn, document.Change{WID: revision.ID.String(), Locale: revision.Locale, Slug: revision.Slug, Title: revision.Title, Revision: revision.Revision, Author: revision.Author, Summary: revision.Summary, Action: action, CreatedAt: revision.UpdatedAt})
	})
}

func orderedWorkflowID(at time.Time) string {
	random := make([]byte, 6)
	if _, err := cryptoRand.Read(random); err != nil {
		return fmt.Sprintf("%020d", at.UnixNano())
	}
	return fmt.Sprintf("%020d-%s", at.UnixNano(), hex.EncodeToString(random))
}

func workflowPart(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.TrimSpace(value)))
}

func draftKey(userID string, locale string, slug string) string {
	return draftPrefix + workflowPart(userID) + ":" + workflowPart(locale) + ":" + workflowPart(slug)
}

func (s *BadgerDocumentStore) SaveDraft(ctx context.Context, draft document.Draft) (document.Draft, error) {
	locale, ok := document.CanonicalLocale(draft.Locale)
	if !ok || strings.TrimSpace(draft.UserID) == "" || strings.TrimSpace(draft.Slug) == "" {
		return document.Draft{}, ErrInvalidLocale
	}
	draft.Locale = locale
	draft.UserID = strings.TrimSpace(draft.UserID)
	draft.Slug = strings.TrimSpace(draft.Slug)
	draft.UpdatedAt = time.Now().UTC()
	data, err := json.Marshal(draft)
	if err != nil {
		return document.Draft{}, err
	}
	err = s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		return txn.Set([]byte(draftKey(draft.UserID, draft.Locale, draft.Slug)), data)
	})
	return draft, err
}

func (s *BadgerDocumentStore) GetDraft(ctx context.Context, userID string, locale string, slug string) (document.Draft, error) {
	locale, ok := document.CanonicalLocale(locale)
	if !ok {
		return document.Draft{}, ErrInvalidLocale
	}
	var draft document.Draft
	err := s.db.View(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		item, err := txn.Get([]byte(draftKey(userID, locale, slug)))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrDocumentNotFound
		}
		if err != nil {
			return err
		}
		return item.Value(func(value []byte) error { return json.Unmarshal(value, &draft) })
	})
	return draft, err
}

func (s *BadgerDocumentStore) DeleteDraft(ctx context.Context, userID string, locale string, slug string) error {
	locale, ok := document.CanonicalLocale(locale)
	if !ok {
		return ErrInvalidLocale
	}
	return s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		return txn.Delete([]byte(draftKey(userID, locale, slug)))
	})
}

func (s *BadgerDocumentStore) RestoreRevision(ctx context.Context, locale string, slug string, revision string, baseRevision string, actor string, reason string) (document.Document, error) {
	locale, ok := document.CanonicalLocale(locale)
	if !ok {
		return document.Document{}, ErrInvalidLocale
	}
	var restored document.Document
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		current, err := documentFromTxn(txn, latestDocumentKey(locale, slug))
		if err != nil {
			return err
		}
		if current.Revision != strings.TrimSpace(baseRevision) {
			return ErrRevisionConflict
		}
		historical, err := documentFromTxn(txn, revisionDocumentKey(locale, slug, strings.TrimSpace(revision)))
		if err != nil {
			return err
		}
		restored = current
		restored.Source = historical.Source
		restored.HTML = historical.HTML
		restored.Author = strings.TrimSpace(actor)
		restored.Summary = "Restore " + historical.Revision
		if reason = strings.TrimSpace(reason); reason != "" {
			restored.Summary += ": " + reason
		}
		restored.UpdatedAt = time.Now().UTC()
		return saveDocumentTxnWithAction(txn, &restored, document.ChangeRestore)
	})
	if err == nil {
		s.invalidateSearchCache(locale)
	}
	return restored, err
}

// ResetDocumentHistory replaces every stored revision with the current safe
// document as r1. It is intentionally separate from ordinary deletion and
// restoration because it irreversibly removes public history data.
func (s *BadgerDocumentStore) ResetDocumentHistory(ctx context.Context, locale string, slug string, baseRevision string, actor string) (document.HistoryResetResult, error) {
	locale, ok := document.CanonicalLocale(locale)
	slug = strings.TrimSpace(slug)
	baseRevision = strings.TrimSpace(baseRevision)
	actor = strings.TrimSpace(actor)
	if !ok || slug == "" || baseRevision == "" || actor == "" {
		return document.HistoryResetResult{}, ErrDocumentNotFound
	}

	var result document.HistoryResetResult
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		current, err := documentFromTxn(txn, latestDocumentKey(locale, slug))
		if err != nil {
			return err
		}
		if current.Revision != baseRevision {
			return ErrRevisionConflict
		}

		if err := purgeDocumentProposalsTxn(ctx, txn, current.ID, locale, slug); err != nil {
			return err
		}
		if err := deleteRevisionSummariesTxn(txn, locale, slug); err != nil {
			return err
		}
		result.RemovedRevisions, err = deleteKeysByPrefixCount(txn, revisionDocumentPrefix(locale, slug))
		if err != nil {
			return err
		}
		if locale == document.DefaultLocale {
			legacyRemoved, err := deleteKeysByPrefixCount(txn, legacyRevisionDocumentPrefix(slug))
			if err != nil {
				return err
			}
			result.RemovedRevisions += legacyRemoved
			if err := txn.Delete([]byte(legacyLatestDocumentKey(slug))); err != nil {
				return err
			}
		}
		result.RemovedChanges, err = deleteDocumentChangesTxn(txn, current.ID, locale, slug)
		if err != nil {
			return err
		}
		result.RemovedDrafts, err = deleteDocumentDraftsTxn(txn, locale, slug)
		if err != nil {
			return err
		}
		result.RemovedSuggestions, err = deleteNamingSuggestionsForDocumentTxn(txn, locale, slug)
		if err != nil {
			return err
		}
		if current.ID != "" {
			if err := txn.Delete([]byte(trashPrefix + current.ID.String())); err != nil {
				return err
			}
			if err := deleteKeysByPrefix(txn, changeRevisionMarkerPrefix+workflowPart(current.ID.String())+":"); err != nil {
				return err
			}
		}
		if err := deleteKeysByPrefix(txn, changeRevisionMarkerPrefix+workflowPart(locale)+":"+workflowPart(slug)+":"); err != nil {
			return err
		}

		now := time.Now().UTC()
		current.Revision = "r1"
		current.Author = actor
		current.Summary = ""
		current.HTML = ""
		current.UpdatedAt = now
		blob, err := marshalDocumentXML(current)
		if err != nil {
			return err
		}
		if err := txn.Set([]byte(latestDocumentKey(locale, slug)), blob); err != nil {
			return err
		}
		if err := txn.Set([]byte(revisionDocumentKey(locale, slug, current.Revision)), blob); err != nil {
			return err
		}
		if err := setRevisionSummaryTxn(txn, current); err != nil {
			return err
		}
		if err := setDocumentReadIndexesTxn(txn, current); err != nil {
			return err
		}
		sequence := make([]byte, 8)
		binary.BigEndian.PutUint64(sequence, 1)
		if err := txn.Set([]byte(revisionSequenceKey(locale, slug)), sequence); err != nil {
			return err
		}
		if err := appendRevisionChangeTxn(txn, document.Change{
			WID: current.ID.String(), Locale: locale, Slug: slug, Title: current.Title,
			Revision: current.Revision, Author: actor, Action: document.ChangeCreate, CreatedAt: now,
		}); err != nil {
			return err
		}
		result.Document = current
		return nil
	})
	if err == nil {
		s.invalidateSearchCache(locale)
	}
	return result, err
}

func deleteDocumentChangesTxn(txn *badger.Txn, wid document.WID, locale string, slug string) (int, error) {
	prefix := []byte(changePrefix)
	opts := badger.DefaultIteratorOptions
	opts.Prefix = prefix
	it := txn.NewIterator(opts)
	keys := make([][]byte, 0)
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		var change document.Change
		if err := it.Item().Value(func(value []byte) error { return json.Unmarshal(value, &change) }); err != nil {
			it.Close()
			return 0, err
		}
		if (wid != "" && change.WID == wid.String()) || (change.Locale == locale && change.Slug == slug) {
			keys = append(keys, it.Item().KeyCopy(nil))
		}
	}
	it.Close()
	for _, key := range keys {
		if err := txn.Delete(key); err != nil {
			return 0, err
		}
	}
	return len(keys), nil
}

func deleteDocumentDraftsTxn(txn *badger.Txn, locale string, slug string) (int, error) {
	prefix := []byte(draftPrefix)
	opts := badger.DefaultIteratorOptions
	opts.Prefix = prefix
	it := txn.NewIterator(opts)
	keys := make([][]byte, 0)
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		var draft document.Draft
		if err := it.Item().Value(func(value []byte) error { return json.Unmarshal(value, &draft) }); err != nil {
			it.Close()
			return 0, err
		}
		if draft.Locale == locale && draft.Slug == slug {
			keys = append(keys, it.Item().KeyCopy(nil))
		}
	}
	it.Close()
	for _, key := range keys {
		if err := txn.Delete(key); err != nil {
			return 0, err
		}
	}
	return len(keys), nil
}

func deleteKeysByPrefixCount(txn *badger.Txn, prefixValue string) (int, error) {
	prefix := []byte(prefixValue)
	opts := badger.DefaultIteratorOptions
	opts.Prefix = prefix
	it := txn.NewIterator(opts)
	keys := make([][]byte, 0)
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		keys = append(keys, it.Item().KeyCopy(nil))
	}
	it.Close()
	for _, key := range keys {
		if err := txn.Delete(key); err != nil {
			return 0, err
		}
	}
	return len(keys), nil
}

func (s *BadgerDocumentStore) DeleteDocument(ctx context.Context, locale string, slug string, actor string, reason string) (document.TrashEntry, error) {
	locale, ok := document.CanonicalLocale(locale)
	if !ok {
		return document.TrashEntry{}, ErrInvalidLocale
	}
	var entry document.TrashEntry
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		doc, err := documentFromTxn(txn, latestDocumentKey(locale, slug))
		if err != nil {
			return err
		}
		if doc.ID == "" {
			return errors.New("document has no WID")
		}
		entry = document.TrashEntry{Document: doc, DeletedBy: strings.TrimSpace(actor), Reason: strings.TrimSpace(reason), DeletedAt: time.Now().UTC()}
		data, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		if err := txn.Set([]byte(trashPrefix+doc.ID.String()), data); err != nil {
			return err
		}
		if err := txn.Delete([]byte(latestDocumentKey(locale, slug))); err != nil {
			return err
		}
		if err := txn.Delete(documentWIDIndexKey(doc.ID)); err != nil {
			return err
		}
		if err := deleteDocumentReadIndexesTxn(txn, locale, slug); err != nil {
			return err
		}
		return appendChangeTxn(txn, document.Change{Locale: locale, Slug: slug, Title: doc.Title, Revision: doc.Revision, Author: entry.DeletedBy, Summary: entry.Reason, Action: document.ChangeDelete, CreatedAt: entry.DeletedAt})
	})
	if err == nil {
		s.invalidateSearchCache(locale)
	}
	return entry, err
}

func (s *BadgerDocumentStore) ListTrash(ctx context.Context, limit int) ([]document.TrashEntry, error) {
	if limit < 1 || limit > 500 {
		limit = 200
	}
	entries := make([]document.TrashEntry, 0)
	err := s.db.View(func(txn *badger.Txn) error {
		return collectJSONValues(ctx, txn, trashPrefix, func(value []byte) error {
			var entry document.TrashEntry
			if err := json.Unmarshal(value, &entry); err != nil {
				return err
			}
			entries = append(entries, entry)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].DeletedAt.After(entries[j].DeletedAt) })
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

func (s *BadgerDocumentStore) RestoreDeletedDocument(ctx context.Context, wid document.WID, actor string, reason string) (document.Document, error) {
	if err := wid.Validate(); err != nil {
		return document.Document{}, err
	}
	var restored document.Document
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		item, err := txn.Get([]byte(trashPrefix + wid.String()))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrDocumentNotFound
		}
		if err != nil {
			return err
		}
		var entry document.TrashEntry
		if err := item.Value(func(value []byte) error { return json.Unmarshal(value, &entry) }); err != nil {
			return err
		}
		restored = entry.Document
		if _, err := txn.Get([]byte(latestDocumentKey(restored.Locale, restored.Slug))); err == nil {
			return ErrDocumentExists
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		if exists, err := documentRedirectExistsTxn(txn, restored.Locale, restored.Slug); err != nil {
			return err
		} else if exists {
			return ErrDocumentExists
		}
		blob, err := marshalDocumentXML(restored)
		if err != nil {
			return err
		}
		if err := txn.Set([]byte(latestDocumentKey(restored.Locale, restored.Slug)), blob); err != nil {
			return err
		}
		if err := setDocumentReadIndexesTxn(txn, restored); err != nil {
			return err
		}
		if err := txn.Set(documentWIDIndexKey(wid), []byte(restored.Locale+"\x00"+restored.Slug)); err != nil {
			return err
		}
		if err := txn.Delete([]byte(trashPrefix + wid.String())); err != nil {
			return err
		}
		return appendChangeTxn(txn, document.Change{Locale: restored.Locale, Slug: restored.Slug, Title: restored.Title, Revision: restored.Revision, Author: strings.TrimSpace(actor), Summary: strings.TrimSpace(reason), Action: document.ChangeRestore, CreatedAt: time.Now().UTC()})
	})
	if err == nil {
		s.invalidateSearchCache(restored.Locale)
	}
	return restored, err
}

func (s *BadgerDocumentStore) ListChanges(ctx context.Context, filter document.ChangeFilter) ([]document.Change, error) {
	if filter.Limit < 1 || filter.Limit > 500 {
		filter.Limit = 100
	}
	changes := make([]document.Change, 0)
	err := s.db.View(func(txn *badger.Txn) error {
		return collectJSONValues(ctx, txn, changePrefix, func(value []byte) error {
			var change document.Change
			if err := json.Unmarshal(value, &change); err != nil {
				return err
			}
			if changeMatches(change, filter) {
				changes = append(changes, change)
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].CreatedAt.After(changes[j].CreatedAt) })
	if len(changes) > filter.Limit {
		changes = changes[:filter.Limit]
	}
	return changes, nil
}

func changeMatches(change document.Change, filter document.ChangeFilter) bool {
	if filter.Locale != "" && change.Locale != filter.Locale || filter.Author != "" && !strings.EqualFold(change.Author, filter.Author) || filter.Action != "" && change.Action != filter.Action {
		return false
	}
	if !filter.Since.IsZero() && change.CreatedAt.Before(filter.Since) || !filter.Until.IsZero() && change.CreatedAt.After(filter.Until) {
		return false
	}
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	return query == "" || strings.Contains(strings.ToLower(change.Title), query) || strings.Contains(strings.ToLower(change.Slug), query)
}

func (s *BadgerDocumentStore) AppendAudit(ctx context.Context, entry document.AuditEntry) (document.AuditEntry, error) {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	entry.ID = orderedWorkflowID(entry.CreatedAt)
	data, err := json.Marshal(entry)
	if err != nil {
		return document.AuditEntry{}, err
	}
	err = s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		return txn.Set([]byte(auditPrefix+entry.ID), data)
	})
	return entry, err
}

func (s *BadgerDocumentStore) ListAudit(ctx context.Context, filter document.AuditFilter) ([]document.AuditEntry, error) {
	if filter.Limit < 1 || filter.Limit > 500 {
		filter.Limit = 200
	}
	entries := make([]document.AuditEntry, 0)
	err := s.db.View(func(txn *badger.Txn) error {
		return collectJSONValues(ctx, txn, auditPrefix, func(value []byte) error {
			var entry document.AuditEntry
			if err := json.Unmarshal(value, &entry); err != nil {
				return err
			}
			if filter.Actor != "" && !strings.EqualFold(entry.Actor, filter.Actor) || filter.Action != "" && entry.Action != filter.Action || filter.Target != "" && !strings.Contains(strings.ToLower(entry.Target), strings.ToLower(filter.Target)) {
				return nil
			}
			entries = append(entries, entry)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].CreatedAt.After(entries[j].CreatedAt) })
	if len(entries) > filter.Limit {
		entries = entries[:filter.Limit]
	}
	return entries, nil
}

func collectJSONValues(ctx context.Context, txn *badger.Txn, prefixValue string, collect func([]byte) error) error {
	options := badger.DefaultIteratorOptions
	options.Prefix = []byte(prefixValue)
	iterator := txn.NewIterator(options)
	defer iterator.Close()
	for iterator.Seek(options.Prefix); iterator.ValidForPrefix(options.Prefix); iterator.Next() {
		if err := contextDone(ctx); err != nil {
			return err
		}
		if err := iterator.Item().Value(func(value []byte) error { return collect(append([]byte(nil), value...)) }); err != nil {
			return err
		}
	}
	return nil
}
