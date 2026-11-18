package storage

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	badger "github.com/dgraph-io/badger/v4"

	"techpedia/internal/document"
	"techpedia/internal/edition"
)

const editionPrefix = "edition/v1/"

func (s *BadgerDocumentStore) ListEditions(ctx context.Context) ([]edition.Edition, error) {
	values := make([]edition.Edition, 0)
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(editionPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			var value edition.Edition
			if err := it.Item().Value(func(data []byte) error { return json.Unmarshal(data, &value) }); err != nil {
				return err
			}
			values = append(values, value)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].Active() != values[j].Active() {
			return values[i].Active()
		}
		iOrder := values[i].PortalOrder
		jOrder := values[j].PortalOrder
		if iOrder <= 0 {
			iOrder = int(^uint(0) >> 1)
		}
		if jOrder <= 0 {
			jOrder = int(^uint(0) >> 1)
		}
		if iOrder != jOrder {
			return iOrder < jOrder
		}
		return values[i].Tag < values[j].Tag
	})
	return values, nil
}

func (s *BadgerDocumentStore) GetEdition(ctx context.Context, tag string) (edition.Edition, error) {
	tag, ok := document.CanonicalLocale(tag)
	if !ok {
		return edition.Edition{}, edition.ErrInvalid
	}
	var value edition.Edition
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(editionKey(tag)))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return edition.ErrNotFound
		}
		if err != nil {
			return err
		}
		return item.Value(func(data []byte) error { return json.Unmarshal(data, &value) })
	})
	return value, err
}

func (s *BadgerDocumentStore) ProposeEdition(ctx context.Context, value edition.Edition) (edition.Edition, error) {
	value.Status = edition.StatusPending
	value.MainPageName = ""
	value.ApprovedBy = ""
	value.ApprovedAt = time.Time{}
	value.ActivatedAt = time.Time{}
	if value.RequestedAt.IsZero() {
		value.RequestedAt = time.Now().UTC()
	}
	value, err := edition.Normalize(value)
	if err != nil {
		return edition.Edition{}, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return edition.Edition{}, err
	}
	err = s.db.Update(func(txn *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		key := []byte(editionKey(value.Tag))
		if _, err := txn.Get(key); err == nil {
			return edition.ErrAlreadyExists
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		return txn.Set(key, data)
	})
	return value, err
}

func (s *BadgerDocumentStore) ApproveEdition(ctx context.Context, tag string, approvedBy string, approvedAt time.Time) (edition.Edition, error) {
	tag, ok := document.CanonicalLocale(tag)
	approvedBy = strings.TrimSpace(approvedBy)
	if !ok || approvedBy == "" {
		return edition.Edition{}, edition.ErrInvalid
	}
	var value edition.Edition
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, err := txn.Get([]byte(editionKey(tag)))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return edition.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := item.Value(func(data []byte) error { return json.Unmarshal(data, &value) }); err != nil {
			return err
		}
		if value.Status != edition.StatusPending {
			return edition.ErrInvalid
		}
		value.Status = edition.StatusApproved
		value.ApprovedBy = approvedBy
		value.ApprovedAt = approvedAt.UTC()
		value, err = edition.Normalize(value)
		if err != nil {
			return err
		}
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return txn.Set([]byte(editionKey(tag)), data)
	})
	return value, err
}

func (s *BadgerDocumentStore) RejectEdition(ctx context.Context, tag string) (edition.Edition, error) {
	tag, ok := document.CanonicalLocale(tag)
	if !ok {
		return edition.Edition{}, edition.ErrInvalid
	}
	var value edition.Edition
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		key := []byte(editionKey(tag))
		item, err := txn.Get(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return edition.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := item.Value(func(data []byte) error { return json.Unmarshal(data, &value) }); err != nil {
			return err
		}
		if value.Status != edition.StatusPending {
			return edition.ErrInvalid
		}
		return txn.Delete(key)
	})
	return value, err
}

func (s *BadgerDocumentStore) ActivateEdition(ctx context.Context, tag string, mainPageName string, activatedAt time.Time) (edition.Edition, error) {
	tag, ok := document.CanonicalLocale(tag)
	if !ok {
		return edition.Edition{}, edition.ErrInvalid
	}
	mainPageName = strings.TrimSpace(mainPageName)
	if mainPageName == "" {
		return edition.Edition{}, edition.ErrInvalid
	}
	var value edition.Edition
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, err := txn.Get([]byte(editionKey(tag)))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return edition.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := item.Value(func(data []byte) error { return json.Unmarshal(data, &value) }); err != nil {
			return err
		}
		if value.Status != edition.StatusApproved {
			return edition.ErrInvalid
		}
		value.Status = edition.StatusActive
		value.MainPageName = mainPageName
		value.ActivatedAt = activatedAt.UTC()
		normalized, normalizeErr := edition.Normalize(value)
		if normalizeErr != nil {
			return normalizeErr
		}
		value = normalized
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return txn.Set([]byte(editionKey(tag)), data)
	})
	return value, err
}

func (s *BadgerDocumentStore) UpdateInterfaceMessages(ctx context.Context, tag string, messages map[string]string) (edition.Edition, error) {
	tag, ok := document.CanonicalLocale(tag)
	if !ok {
		return edition.Edition{}, edition.ErrInvalid
	}
	var value edition.Edition
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, err := txn.Get([]byte(editionKey(tag)))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return edition.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := item.Value(func(data []byte) error { return json.Unmarshal(data, &value) }); err != nil {
			return err
		}
		if !value.Configurable() {
			return edition.ErrInvalid
		}
		value.InterfaceMessages = messages
		normalized, err := edition.Normalize(value)
		if err != nil {
			return err
		}
		value = normalized
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return txn.Set([]byte(editionKey(tag)), data)
	})
	return value, err
}

func (s *BadgerDocumentStore) FillInterfaceMessages(ctx context.Context, tag string, messages map[string]string) (edition.Edition, int, error) {
	tag, ok := document.CanonicalLocale(tag)
	if !ok {
		return edition.Edition{}, 0, edition.ErrInvalid
	}
	var value edition.Edition
	filled := 0
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, err := txn.Get([]byte(editionKey(tag)))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return edition.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := item.Value(func(data []byte) error { return json.Unmarshal(data, &value) }); err != nil {
			return err
		}
		if !value.Configurable() {
			return edition.ErrInvalid
		}
		if value.InterfaceMessages == nil {
			value.InterfaceMessages = make(map[string]string)
		}
		for key, message := range messages {
			if strings.TrimSpace(value.InterfaceMessages[key]) != "" || strings.TrimSpace(message) == "" {
				continue
			}
			value.InterfaceMessages[key] = message
			filled++
		}
		normalized, err := edition.Normalize(value)
		if err != nil {
			return err
		}
		value = normalized
		if filled == 0 {
			return nil
		}
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return txn.Set([]byte(editionKey(tag)), data)
	})
	return value, filled, err
}

func (s *BadgerDocumentStore) UpdateNavigation(ctx context.Context, tag string, navigation []edition.NavigationSection) (edition.Edition, error) {
	tag, ok := document.CanonicalLocale(tag)
	if !ok {
		return edition.Edition{}, edition.ErrInvalid
	}
	var value edition.Edition
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, err := txn.Get([]byte(editionKey(tag)))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return edition.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := item.Value(func(data []byte) error { return json.Unmarshal(data, &value) }); err != nil {
			return err
		}
		if !value.Configurable() {
			return edition.ErrInvalid
		}
		value.Navigation = navigation
		normalized, err := edition.Normalize(value)
		if err != nil {
			return err
		}
		value = normalized
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return txn.Set([]byte(editionKey(tag)), data)
	})
	return value, err
}

func (s *BadgerDocumentStore) UpdatePortal(ctx context.Context, tag string, portalOrder int, countryCodes []string) (edition.Edition, error) {
	tag, ok := document.CanonicalLocale(tag)
	if !ok {
		return edition.Edition{}, edition.ErrInvalid
	}
	var value edition.Edition
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, err := txn.Get([]byte(editionKey(tag)))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return edition.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := item.Value(func(data []byte) error { return json.Unmarshal(data, &value) }); err != nil {
			return err
		}
		value.PortalOrder = portalOrder
		value.CountryCodes = countryCodes
		normalized, err := edition.Normalize(value)
		if err != nil {
			return err
		}
		value = normalized
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return txn.Set([]byte(editionKey(tag)), data)
	})
	return value, err
}

func editionKey(tag string) string {
	return editionPrefix + tag
}
