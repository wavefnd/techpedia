package storage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"techpedia/internal/box"
	"techpedia/internal/document"
	"techpedia/internal/edition"
)

const boxPrefix = "box:v1:definition:"
const boxContentPrefix = "box:v1:content:"
const boxGenerationKey = "box:v1:generation"

var _ BoxStore = (*BadgerDocumentStore)(nil)

func boxKey(name string) string { return base64.RawURLEncoding.EncodeToString([]byte(name)) }

func boxJSON(tx *badger.Txn, key string, value any) error {
	item, err := tx.Get([]byte(key))
	if errors.Is(err, badger.ErrKeyNotFound) {
		return box.ErrNotFound
	}
	if err != nil {
		return err
	}
	return item.Value(func(raw []byte) error { return json.Unmarshal(raw, value) })
}

func setBoxJSON(tx *badger.Txn, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return tx.Set([]byte(key), raw)
}

func (s *BadgerDocumentStore) GetBox(ctx context.Context, name string) (box.Definition, error) {
	var v box.Definition
	err := s.db.View(func(tx *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return boxJSON(tx, boxPrefix+boxKey(name), &v)
	})
	return v, err
}

func (s *BadgerDocumentStore) GetBoxContent(ctx context.Context, name, locale string) (box.Content, error) {
	var v box.Content
	err := s.db.View(func(tx *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return boxJSON(tx, boxContentPrefix+boxKey(name)+":"+locale, &v)
	})
	return v, err
}

func (s *BadgerDocumentStore) GetBoxRevision(ctx context.Context, name string, version uint64) (box.Entry, error) {
	var v box.Entry
	err := s.db.View(func(tx *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return boxJSON(tx, fmt.Sprintf("box:v1:revision:%s:%020d", boxKey(name), version), &v)
	})
	return v, err
}

// The cursor is the last resource name; metadata-only iteration never reads
// multilingual source blobs. Each request has a bounded result size.
func (s *BadgerDocumentStore) ListBoxes(ctx context.Context, query, after string, limit int) (box.Page, error) {
	result := box.Page{Boxes: []box.Definition{}}
	if limit < 1 || limit > 50 {
		limit = 50
	}
	query = strings.ToLower(box.NormalizeName(query))
	err := s.db.View(func(tx *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(boxPrefix)
		it := tx.NewIterator(opts)
		defer it.Close()
		start := boxPrefix
		if after != "" {
			start += boxKey(after)
		}
		for it.Seek([]byte(start)); it.ValidForPrefix([]byte(boxPrefix)); it.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			var v box.Definition
			if err := it.Item().Value(func(raw []byte) error { return json.Unmarshal(raw, &v) }); err != nil {
				return err
			}
			if v.Name == after || !strings.Contains(strings.ToLower(v.Name), query) {
				continue
			}
			if len(result.Boxes) == limit {
				result.Next = result.Boxes[len(result.Boxes)-1].Name
				break
			}
			result.Boxes = append(result.Boxes, v)
		}
		return nil
	})
	return result, err
}

func (s *BadgerDocumentStore) SaveBox(ctx context.Context, in box.Save, actor string) (box.Entry, error) {
	if err := box.Validate(in); err != nil {
		return box.Entry{}, err
	}
	if strings.TrimSpace(actor) == "" {
		return box.Entry{}, box.ErrInvalid
	}
	var result box.Entry
	err := s.db.Update(func(tx *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var v box.Definition
		err := boxJSON(tx, boxPrefix+boxKey(in.Name), &v)
		if errors.Is(err, box.ErrNotFound) {
			v = box.Definition{Name: in.Name, Locales: []string{}}
		} else if err != nil {
			return err
		}
		if v.Version != in.BaseVersion {
			return box.ErrConflict
		}
		for _, locale := range []string{in.Locale, in.FallbackLocale} {
			if locale == "" {
				continue
			}
			var e edition.Edition
			if err := boxJSON(tx, editionPrefix+locale, &e); err != nil {
				return box.ErrInvalid
			}
			if e.Status != edition.StatusApproved && !e.Active() {
				return box.ErrInvalid
			}
		}
		if in.FallbackLocale != "" && in.FallbackLocale != in.Locale && !slices.Contains(v.Locales, in.FallbackLocale) {
			return box.ErrInvalid
		}
		if !slices.Contains(v.Locales, in.Locale) {
			v.Locales = append(v.Locales, in.Locale)
			slices.Sort(v.Locales)
		}
		v.Version++
		v.FallbackLocale = in.FallbackLocale
		v.UpdatedAt = time.Now().UTC()
		result = box.Entry{Definition: v, Content: box.Content{Locale: in.Locale, Source: in.Source, Author: actor, Version: v.Version, UpdatedAt: v.UpdatedAt}}
		if err := setBoxJSON(tx, boxPrefix+boxKey(in.Name), v); err != nil {
			return err
		}
		if err := setBoxJSON(tx, boxContentPrefix+boxKey(in.Name)+":"+in.Locale, result.Content); err != nil {
			return err
		}
		if err := setBoxJSON(tx, fmt.Sprintf("box:v1:revision:%s:%020d", boxKey(in.Name), v.Version), result); err != nil {
			return err
		}
		// Media deletion reads this generation key to detect a concurrent newly
		// created box, not just modifications found by its prefix iterator.
		if err := tx.Set([]byte(boxGenerationKey), []byte(orderedWorkflowID(v.UpdatedAt))); err != nil {
			return err
		}
		audit := document.AuditEntry{ID: orderedWorkflowID(v.UpdatedAt), Actor: actor, Action: "box.save", TargetType: "box", Target: in.Name, Detail: fmt.Sprintf("locale=%s version=%d", in.Locale, v.Version), Result: "success", CreatedAt: v.UpdatedAt}
		return setBoxJSON(tx, auditPrefix+audit.ID, audit)
	})
	if errors.Is(err, badger.ErrConflict) {
		err = box.ErrConflict
	}
	return result, err
}
