package storage

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	badger "github.com/dgraph-io/badger/v4"

	"techpedia/internal/document"
)

const revisionSummaryMigration = "revision-summaries-v1"

type revisionIndexEntry struct {
	SchemaVersion int                      `json:"schema_version"`
	Summary       document.RevisionSummary `json:"summary"`
	RecentKey     string                   `json:"recent_key"`
}

func revisionSummaryPrefix(locale, slug string) string {
	return "wiki:v1:history:" + document.NormalizeLocale(locale) + ":" + workflowPart(slug) + ":"
}

func recentSummaryPrefix(locale string) string {
	return "wiki:v1:recent-revision:" + document.NormalizeLocale(locale) + ":"
}

// Fixed-width UTC digits sort RFC3339 timestamps, including zero times.
// Complementing them keeps newest-first reads forward and bounded.
func descendingRevisionTime(at time.Time) string {
	value := []byte(at.UTC().Format("20060102150405.000000000"))
	for i, digit := range value {
		if digit >= '0' && digit <= '9' {
			value[i] = '9' - (digit - '0')
		}
	}
	return string(value)
}

func setRevisionSummaryTxn(txn *badger.Txn, doc document.Document) error {
	order := descendingRevisionTime(doc.UpdatedAt)
	recentKey := recentSummaryPrefix(doc.Locale) + order + ":" + hex.EncodeToString([]byte(doc.Slug)) + ":" + doc.Revision
	entry := revisionIndexEntry{SchemaVersion: 1, Summary: document.NewRevisionSummary(doc), RecentKey: recentKey}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if err := txn.Set([]byte(revisionSummaryPrefix(doc.Locale, doc.Slug)+order+":"+doc.Revision), data); err != nil {
		return err
	}
	return txn.Set([]byte(recentKey), data)
}

func readRevisionIndex(data []byte) (revisionIndexEntry, error) {
	var entry revisionIndexEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return entry, err
	}
	if entry.SchemaVersion != 1 {
		return entry, errors.New("invalid revision index schema")
	}
	return entry, nil
}

func (s *BadgerDocumentStore) listRevisionSummaries(ctx context.Context, prefix string, limit int) ([]document.RevisionSummary, error) {
	values := make([]document.RevisionSummary, 0, limit)
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		opts.PrefetchSize = min(limit, 16)
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(opts.Prefix); it.ValidForPrefix(opts.Prefix) && len(values) < limit; it.Next() {
			if err := contextDone(ctx); err != nil {
				return err
			}
			if err := it.Item().Value(func(data []byte) error {
				entry, err := readRevisionIndex(data)
				if err == nil {
					values = append(values, entry.Summary)
				}
				return err
			}); err != nil {
				return err
			}
		}
		return contextDone(ctx)
	})
	return values, err
}

func deleteRevisionSummariesTxn(txn *badger.Txn, locale, slug string) error {
	prefix := []byte(revisionSummaryPrefix(locale, slug))
	opts := badger.DefaultIteratorOptions
	opts.Prefix = prefix
	opts.PrefetchSize = 1
	it := txn.NewIterator(opts)
	defer it.Close()
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		var entry revisionIndexEntry
		if err := it.Item().Value(func(data []byte) error {
			var err error
			entry, err = readRevisionIndex(data)
			return err
		}); err != nil {
			return err
		}
		if !strings.HasPrefix(entry.RecentKey, recentSummaryPrefix(locale)) {
			return errors.New("invalid revision recent index")
		}
		if err := txn.Delete([]byte(entry.RecentKey)); err != nil {
			return err
		}
		if err := txn.Delete(it.Item().KeyCopy(nil)); err != nil {
			return err
		}
	}
	return nil
}

func migrationDoneKey(name string) []byte { return []byte("migration:v1:" + name + ":done") }

// The store is not published until migrations finish. Each record and its
// cursor commit together, bounding memory and making interrupted opens resumable.
func (s *BadgerDocumentStore) migrateDocuments(name string, prefixes []string, apply func(*badger.Txn, document.Document, string) error) error {
	var done bool
	if err := s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(migrationDoneKey(name))
		done = err == nil
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		return err
	}); err != nil || done {
		return err
	}
	for _, prefix := range prefixes {
		cursorKey := []byte("migration:v1:" + name + ":cursor:" + hex.EncodeToString([]byte(prefix)))
		var cursor []byte
		if err := s.db.View(func(txn *badger.Txn) error {
			item, err := txn.Get(cursorKey)
			if errors.Is(err, badger.ErrKeyNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			cursor, err = item.ValueCopy(nil)
			return err
		}); err != nil {
			return err
		}
		if err := s.db.View(func(txn *badger.Txn) error {
			opts := badger.DefaultIteratorOptions
			opts.Prefix = []byte(prefix)
			opts.PrefetchValues = false
			it := txn.NewIterator(opts)
			defer it.Close()
			if len(cursor) == 0 {
				it.Seek(opts.Prefix)
			} else {
				it.Seek(cursor)
				if it.ValidForPrefix(opts.Prefix) && bytes.Equal(it.Item().Key(), cursor) {
					it.Next()
				}
			}
			for ; it.ValidForPrefix(opts.Prefix); it.Next() {
				key := it.Item().KeyCopy(nil)
				doc, err := documentFromItem(it.Item())
				if err != nil {
					return err
				}
				if err := s.db.Update(func(update *badger.Txn) error {
					if err := apply(update, doc, string(key)); err != nil {
						return err
					}
					return update.Set(cursorKey, key)
				}); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return s.db.Update(func(txn *badger.Txn) error { return txn.Set(migrationDoneKey(name), []byte{1}) })
}

func (s *BadgerDocumentStore) startupOnce(name string, apply func() error) error {
	var done bool
	if err := s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(migrationDoneKey(name))
		done = err == nil
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		return err
	}); err != nil || done {
		return err
	}
	if err := apply(); err != nil {
		return err
	}
	return s.db.Update(func(txn *badger.Txn) error { return txn.Set(migrationDoneKey(name), []byte{1}) })
}
