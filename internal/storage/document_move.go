package storage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	badger "github.com/dgraph-io/badger/v4"

	"techpedia/internal/discussion"
	"techpedia/internal/document"
)

type storedDocumentRevision struct {
	key string
	doc document.Document
}

func (s *BadgerDocumentStore) MoveDocument(ctx context.Context, locale string, oldSlug string, newSlug string, newTitle string) (document.Document, error) {
	return s.MoveDocumentAs(ctx, locale, oldSlug, newSlug, newTitle, "")
}

func (s *BadgerDocumentStore) MoveDocumentAs(ctx context.Context, locale string, oldSlug string, newSlug string, newTitle string, actor string) (document.Document, error) {
	canonicalLocale, ok := document.CanonicalLocale(locale)
	oldSlug = strings.TrimSpace(oldSlug)
	newSlug = strings.TrimSpace(newSlug)
	newTitle = strings.TrimSpace(newTitle)
	if !ok || oldSlug == "" || newSlug == "" || newTitle == "" {
		return document.Document{}, ErrDocumentNotFound
	}
	titleOnly := oldSlug == newSlug

	var moved document.Document
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		latest, err := documentFromTxn(txn, latestDocumentKey(canonicalLocale, oldSlug))
		if err != nil {
			return err
		}
		oldTitle := latest.Title
		if titleOnly {
			if oldTitle == newTitle {
				return ErrDocumentExists
			}
		} else {
			if _, err := txn.Get([]byte(latestDocumentKey(canonicalLocale, newSlug))); err == nil {
				return ErrDocumentExists
			} else if !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
			if _, err := txn.Get([]byte(documentRedirectKey(canonicalLocale, newSlug))); err == nil {
				return ErrDocumentExists
			} else if !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
			if _, err := txn.Get([]byte(documentAliasKey(canonicalLocale, newSlug))); err == nil {
				return ErrDocumentExists
			} else if !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
		}

		revisions, err := documentRevisionsFromTxn(txn, canonicalLocale, oldSlug)
		if err != nil {
			return err
		}
		if err := deleteRevisionSummariesTxn(txn, canonicalLocale, oldSlug); err != nil {
			return err
		}
		if err := deleteDocumentReadIndexesTxn(txn, canonicalLocale, oldSlug); err != nil {
			return err
		}
		for _, revision := range revisions {
			revision.doc.ID = latest.ID
			revision.doc.Slug = newSlug
			revision.doc.Title = newTitle
			blob, err := marshalDocumentXML(revision.doc)
			if err != nil {
				return err
			}
			if err := txn.Set([]byte(revisionDocumentKey(canonicalLocale, newSlug, revision.doc.Revision)), blob); err != nil {
				return err
			}
			if err := setRevisionSummaryTxn(txn, revision.doc); err != nil {
				return err
			}
		}

		latest.Slug = newSlug
		latest.Title = newTitle
		blob, err := marshalDocumentXML(latest)
		if err != nil {
			return err
		}
		if err := txn.Set([]byte(latestDocumentKey(canonicalLocale, newSlug)), blob); err != nil {
			return err
		}
		if err := setDocumentReadIndexesTxn(txn, latest); err != nil {
			return err
		}
		if latest.ID != "" {
			if err := txn.Set(documentWIDIndexKey(latest.ID), []byte(canonicalLocale+"\x00"+newSlug)); err != nil {
				return err
			}
		}
		if !titleOnly {
			if err := copyOptionalKey(txn, revisionSequenceKey(canonicalLocale, oldSlug), revisionSequenceKey(canonicalLocale, newSlug)); err != nil {
				return err
			}
			if err := copyOptionalKey(txn, pageMetricIdentityKey(canonicalLocale, oldSlug), pageMetricIdentityKey(canonicalLocale, newSlug)); err != nil {
				return err
			}
		}
		if err := moveDiscussionPageIndexes(txn, canonicalLocale, oldSlug, newSlug, newTitle); err != nil {
			return err
		}

		if !titleOnly {
			if err := txn.Delete([]byte(latestDocumentKey(canonicalLocale, oldSlug))); err != nil {
				return err
			}
			for _, revision := range revisions {
				if err := txn.Delete([]byte(revision.key)); err != nil {
					return err
				}
			}
			if err := txn.Delete([]byte(revisionSequenceKey(canonicalLocale, oldSlug))); err != nil {
				return err
			}
			if err := txn.Delete([]byte(pageMetricIdentityKey(canonicalLocale, oldSlug))); err != nil {
				return err
			}
			if err := txn.Set([]byte(documentRedirectKey(canonicalLocale, oldSlug)), []byte(newSlug)); err != nil {
				return err
			}
		}
		if err := appendChangeTxn(txn, document.Change{Locale: canonicalLocale, Slug: newSlug, Title: newTitle, Revision: latest.Revision, Author: strings.TrimSpace(actor), Summary: oldTitle + " -> " + newTitle, Action: document.ChangeMove}); err != nil {
			return err
		}
		moved = latest
		return nil
	})
	if err == nil {
		s.invalidateSearchCache(canonicalLocale)
	}
	return moved, err
}

func (s *BadgerDocumentStore) GetDocumentRedirect(ctx context.Context, locale string, slug string) (string, error) {
	locale, ok := document.CanonicalLocale(locale)
	if !ok {
		return "", ErrInvalidLocale
	}
	var target string
	err := s.db.View(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		doc, err := documentRedirectTargetTxn(txn, locale, strings.TrimSpace(slug))
		if err != nil {
			return err
		}
		target = doc.Slug
		return nil
	})
	return target, err
}

func documentFromTxn(txn *badger.Txn, key string) (document.Document, error) {
	item, err := txn.Get([]byte(key))
	if errors.Is(err, badger.ErrKeyNotFound) {
		return document.Document{}, ErrDocumentNotFound
	}
	if err != nil {
		return document.Document{}, err
	}
	return documentFromItem(item)
}

func documentRevisionsFromTxn(txn *badger.Txn, locale string, slug string) ([]storedDocumentRevision, error) {
	prefix := []byte(revisionDocumentPrefix(locale, slug))
	opts := badger.DefaultIteratorOptions
	opts.Prefix = prefix
	it := txn.NewIterator(opts)
	defer it.Close()
	revisions := make([]storedDocumentRevision, 0)
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		doc, err := documentFromItem(it.Item())
		if err != nil {
			return nil, err
		}
		revisions = append(revisions, storedDocumentRevision{key: string(it.Item().KeyCopy(nil)), doc: doc})
	}
	return revisions, nil
}

func copyOptionalKey(txn *badger.Txn, oldKey string, newKey string) error {
	item, err := txn.Get([]byte(oldKey))
	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	value, err := item.ValueCopy(nil)
	if err != nil {
		return err
	}
	return txn.Set([]byte(newKey), value)
}

func moveDiscussionPageIndexes(txn *badger.Txn, locale string, oldSlug string, newSlug string, newTitle string) error {
	prefix := []byte(discussionPageIndexPrefix(locale, oldSlug))
	opts := badger.DefaultIteratorOptions
	opts.Prefix = prefix
	it := txn.NewIterator(opts)
	defer it.Close()
	type indexedTopic struct {
		oldKey []byte
		topic  discussion.Topic
	}
	topics := make([]indexedTopic, 0)
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		var topicID string
		if err := it.Item().Value(func(value []byte) error { topicID = string(value); return nil }); err != nil {
			return err
		}
		topic, err := discussionTopicFromTxn(txn, topicID)
		if err != nil {
			return err
		}
		topics = append(topics, indexedTopic{oldKey: it.Item().KeyCopy(nil), topic: topic})
	}
	for _, indexed := range topics {
		indexed.topic.Slug = newSlug
		indexed.topic.PageTitle = newTitle
		encoded, err := json.Marshal(indexed.topic)
		if err != nil {
			return err
		}
		if err := txn.Set([]byte(discussionTopicKey(indexed.topic.ID)), encoded); err != nil {
			return err
		}
		if err := txn.Set([]byte(discussionPageIndexKey(indexed.topic)), []byte(indexed.topic.ID)); err != nil {
			return err
		}
		if oldSlug != newSlug {
			if err := txn.Delete(indexed.oldKey); err != nil {
				return err
			}
		}
	}
	return nil
}
