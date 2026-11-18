package storage

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	badger "github.com/dgraph-io/badger/v4"

	"techpedia/internal/bot"
	"techpedia/internal/document"
)

const (
	namingSuggestionPrefix       = "bot:v2:naming-suggestion:"
	legacyNamingSuggestionPrefix = "bot:v1:naming-suggestion:"
)

func (s *BadgerDocumentStore) CreateNamingSuggestion(ctx context.Context, suggestion bot.NamingSuggestion) (bot.NamingSuggestion, bool, error) {
	value, err := normalizeNamingSuggestion(suggestion)
	if err != nil {
		return bot.NamingSuggestion{}, false, err
	}
	now := time.Now().UTC()
	value.Status = bot.SuggestionOpen
	value.CreatedAt = now
	value.UpdatedAt = now
	encoded, err := json.Marshal(value)
	if err != nil {
		return bot.NamingSuggestion{}, false, err
	}
	created := false
	err = s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		key := []byte(namingSuggestionKey(value.ID))
		item, err := txn.Get(key)
		if err == nil {
			return item.Value(func(data []byte) error { return json.Unmarshal(data, &value) })
		}
		if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		created = true
		return txn.Set(key, encoded)
	})
	return value, created, err
}

func (s *BadgerDocumentStore) GetNamingSuggestion(ctx context.Context, id string) (bot.NamingSuggestion, error) {
	var value bot.NamingSuggestion
	err := s.db.View(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		item, err := txn.Get([]byte(namingSuggestionKey(strings.TrimSpace(id))))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return bot.ErrNotFound
		}
		if err != nil {
			return err
		}
		return item.Value(func(data []byte) error { return json.Unmarshal(data, &value) })
	})
	return value, err
}

func (s *BadgerDocumentStore) ListNamingSuggestions(ctx context.Context, status bot.SuggestionStatus, limit int) ([]bot.NamingSuggestion, error) {
	if status != "" && !bot.ValidSuggestionStatus(status) {
		return nil, bot.ErrInvalid
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	values := make([]bot.NamingSuggestion, 0, limit)
	err := s.db.View(func(txn *badger.Txn) error {
		return collectJSONPrefix(ctx, txn, namingSuggestionPrefix, func(data []byte) error {
			var value bot.NamingSuggestion
			if err := json.Unmarshal(data, &value); err != nil {
				return err
			}
			if status == "" || value.Status == status {
				values = append(values, value)
			}
			return nil
		})
	})
	sort.Slice(values, func(i, j int) bool { return values[i].UpdatedAt.After(values[j].UpdatedAt) })
	if len(values) > limit {
		values = values[:limit]
	}
	return values, err
}

func (s *BadgerDocumentStore) ReviewNamingSuggestion(ctx context.Context, id string, status bot.SuggestionStatus, reviewedBy string, proposedName string, proposedSource string) (bot.NamingSuggestion, error) {
	if status != bot.SuggestionAccepted && status != bot.SuggestionRejected {
		return bot.NamingSuggestion{}, bot.ErrInvalid
	}
	reviewedBy = strings.TrimSpace(reviewedBy)
	if reviewedBy == "" {
		return bot.NamingSuggestion{}, bot.ErrInvalid
	}
	var value bot.NamingSuggestion
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		key := []byte(namingSuggestionKey(strings.TrimSpace(id)))
		item, err := txn.Get(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return bot.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := item.Value(func(data []byte) error { return json.Unmarshal(data, &value) }); err != nil {
			return err
		}
		if value.Status != bot.SuggestionOpen {
			return bot.ErrReviewed
		}
		if status == bot.SuggestionAccepted {
			proposedName = strings.TrimSpace(proposedName)
			if proposedName == "" {
				return bot.ErrInvalid
			}
			value.ProposedName = proposedName
			value.ProposedSource = proposedSource
		}
		now := time.Now().UTC()
		value.Status = status
		value.ReviewedBy = reviewedBy
		value.ReviewedAt = now
		value.UpdatedAt = now
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return txn.Set(key, encoded)
	})
	return value, err
}

func normalizeNamingSuggestion(value bot.NamingSuggestion) (bot.NamingSuggestion, error) {
	locale, ok := document.CanonicalLocale(value.Locale)
	if !ok || value.SchemaVersion != bot.NamingSuggestionSchemaVersion || !strings.HasPrefix(value.ID, "ns2_") {
		return bot.NamingSuggestion{}, bot.ErrInvalid
	}
	value.Locale = locale
	value.Slug = strings.TrimSpace(value.Slug)
	value.PageName = strings.TrimSpace(value.PageName)
	value.ProposedName = strings.TrimSpace(value.ProposedName)
	if value.Slug == "" || value.PageName == "" || value.BaseRevision == "" || value.ProposedName == "" || len(value.Findings) == 0 {
		return bot.NamingSuggestion{}, bot.ErrInvalid
	}
	return value, nil
}

func collectJSONPrefix(ctx context.Context, txn *badger.Txn, prefixValue string, collect func([]byte) error) error {
	prefix := []byte(prefixValue)
	options := badger.DefaultIteratorOptions
	options.Prefix = prefix
	it := txn.NewIterator(options)
	defer it.Close()
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		if err := contextDone(ctx); err != nil {
			return err
		}
		if err := it.Item().Value(func(data []byte) error { return collect(data) }); err != nil {
			return err
		}
	}
	return nil
}

func namingSuggestionKey(id string) string {
	return namingSuggestionPrefix + id
}

func deleteNamingSuggestionsForDocumentTxn(txn *badger.Txn, locale string, slug string) (int, error) {
	removed := 0
	for _, prefixValue := range []string{namingSuggestionPrefix, legacyNamingSuggestionPrefix} {
		prefix := []byte(prefixValue)
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		it := txn.NewIterator(opts)
		keys := make([][]byte, 0)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var value namingSuggestionDocumentIdentity
			if err := it.Item().Value(func(data []byte) error { return json.Unmarshal(data, &value) }); err != nil {
				it.Close()
				return 0, err
			}
			if value.Locale == locale && value.Slug == slug {
				keys = append(keys, it.Item().KeyCopy(nil))
			}
		}
		it.Close()
		for _, key := range keys {
			if err := txn.Delete(key); err != nil {
				return 0, err
			}
			removed++
		}
	}
	return removed, nil
}

// namingSuggestionDocumentIdentity is deliberately minimal so privacy cleanup
// can remove both current and retired suggestion schemas.
type namingSuggestionDocumentIdentity struct {
	Locale string `json:"locale"`
	Slug   string `json:"slug"`
}
