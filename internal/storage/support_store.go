package storage

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	badger "github.com/dgraph-io/badger/v4"

	"techpedia/internal/support"
)

const supportSettingsKey = "support:v1:settings"

func (s *BadgerDocumentStore) GetSupportSettings(ctx context.Context) (support.Settings, error) {
	result := support.DefaultSettings()
	err := s.db.View(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		item, err := txn.Get([]byte(supportSettingsKey))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return item.Value(func(value []byte) error { return json.Unmarshal(value, &result) })
	})
	if err != nil {
		return support.Settings{}, err
	}
	return support.NormalizeSettings(result)
}

func (s *BadgerDocumentStore) SaveSupportSettings(ctx context.Context, value support.Settings) (support.Settings, error) {
	normalized, err := support.NormalizeSettings(value)
	if err != nil {
		return support.Settings{}, err
	}
	normalized.UpdatedAt = time.Now().UTC()
	blob, err := json.Marshal(normalized)
	if err != nil {
		return support.Settings{}, err
	}
	if err := s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		return txn.Set([]byte(supportSettingsKey), blob)
	}); err != nil {
		return support.Settings{}, err
	}
	return normalized, nil
}
