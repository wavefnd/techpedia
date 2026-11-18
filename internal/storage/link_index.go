package storage

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	badger "github.com/dgraph-io/badger/v4"

	"techpedia/internal/document"
	"techpedia/internal/techmark"
)

const readIndexesMigration = "document-read-indexes-v1"

type linkIndexEntry struct {
	SchemaVersion int                   `json:"schema_version"`
	Page          document.LinkDocument `json:"page"`
}

func linkDocumentPrefix(locale string) string {
	return "wiki:v1:links:" + document.NormalizeLocale(locale) + ":"
}

func linkDocumentKey(locale, slug string) []byte {
	return []byte(linkDocumentPrefix(locale) + hex.EncodeToString([]byte(slug)))
}

func backlinkPrefix(locale, slug string) string {
	return "wiki:v1:backlink:" + document.NormalizeLocale(locale) + ":" + hex.EncodeToString([]byte(slug)) + ":"
}

func backlinkKey(locale, target, source string) []byte {
	return []byte(backlinkPrefix(locale, target) + hex.EncodeToString([]byte(source)))
}

func readLinkIndex(txn *badger.Txn, locale, slug string) (document.LinkDocument, error) {
	item, err := txn.Get(linkDocumentKey(locale, slug))
	if err != nil {
		return document.LinkDocument{}, err
	}
	var entry linkIndexEntry
	if err := item.Value(func(data []byte) error { return json.Unmarshal(data, &entry) }); err != nil {
		return document.LinkDocument{}, err
	}
	if entry.SchemaVersion != 1 {
		return document.LinkDocument{}, errors.New("invalid link index schema")
	}
	return entry.Page, nil
}

func deleteDocumentReadIndexesTxn(txn *badger.Txn, locale, slug string) error {
	page, err := readLinkIndex(txn, locale, slug)
	if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
		return err
	}
	for _, target := range page.Targets {
		if err := txn.Delete(backlinkKey(locale, techmark.Slugify(target), slug)); err != nil {
			return err
		}
	}
	if err := txn.Delete(linkDocumentKey(locale, slug)); err != nil {
		return err
	}
	return txn.Delete(searchDocumentKey(locale, slug))
}

func setDocumentReadIndexesTxn(txn *badger.Txn, doc document.Document) error {
	if err := deleteDocumentReadIndexesTxn(txn, doc.Locale, doc.Slug); err != nil {
		return err
	}
	targets := make([]string, 0)
	seen := make(map[string]bool)
	for _, target := range techmark.InternalLinkTargets(doc.Source) {
		slug := techmark.Slugify(target)
		if seen[slug] {
			continue
		}
		seen[slug] = true
		targets = append(targets, target)
		if slug != doc.Slug {
			if err := txn.Set(backlinkKey(doc.Locale, slug, doc.Slug), []byte(doc.Slug)); err != nil {
				return err
			}
		}
	}
	metadata := document.Document{ID: doc.ID, Locale: doc.Locale, Slug: doc.Slug, Title: doc.Title, Revision: doc.Revision, CreatedAt: doc.CreatedAt, UpdatedAt: doc.UpdatedAt}
	data, err := json.Marshal(linkIndexEntry{SchemaVersion: 1, Page: document.LinkDocument{Document: metadata, Targets: targets}})
	if err != nil {
		return err
	}
	if err := txn.Set(linkDocumentKey(doc.Locale, doc.Slug), data); err != nil {
		return err
	}
	return setSearchDocumentTxn(txn, doc)
}

func (s *BadgerDocumentStore) ensureDocumentReadIndexes() error {
	return s.migrateDocuments(readIndexesMigration, []string{latestDocumentPrefix, legacyLatestDocumentPrefix}, func(txn *badger.Txn, doc document.Document, key string) error {
		if strings.HasPrefix(key, legacyLatestDocumentPrefix) {
			if _, err := txn.Get([]byte(latestDocumentKey(doc.Locale, doc.Slug))); err == nil {
				return nil
			} else if !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
		}
		return setDocumentReadIndexesTxn(txn, doc)
	})
}

func (s *BadgerDocumentStore) ListLinkDocuments(ctx context.Context, locale string) ([]document.LinkDocument, error) {
	pages := make([]document.LinkDocument, 0)
	err := s.db.View(func(txn *badger.Txn) error {
		return collectJSONValues(ctx, txn, linkDocumentPrefix(locale), func(data []byte) error {
			var entry linkIndexEntry
			if err := json.Unmarshal(data, &entry); err != nil {
				return err
			}
			if entry.SchemaVersion != 1 {
				return errors.New("invalid link index schema")
			}
			pages = append(pages, entry.Page)
			return nil
		})
	})
	return pages, err
}

func (s *BadgerDocumentStore) ListBacklinks(ctx context.Context, locale, slug string) ([]document.Document, error) {
	locale = document.NormalizeLocale(locale)
	pages := make([]document.Document, 0)
	err := s.db.View(func(txn *badger.Txn) error {
		prefix := []byte(backlinkPrefix(locale, slug))
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			if err := contextDone(ctx); err != nil {
				return err
			}
			var source string
			if err := it.Item().Value(func(data []byte) error { source = string(data); return nil }); err != nil {
				return err
			}
			page, err := readLinkIndex(txn, locale, source)
			if err != nil {
				return err
			}
			pages = append(pages, page.Document)
		}
		return contextDone(ctx)
	})
	sort.SliceStable(pages, func(i, j int) bool { return strings.ToLower(pages[i].Title) < strings.ToLower(pages[j].Title) })
	return pages, err
}
