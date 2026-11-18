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
	"techpedia/internal/media"
	"techpedia/internal/techmark"
)

const (
	mediaFilePrefix    = "media:v1:file:"
	mediaHashPrefix    = "media:v1:hash:"
	mediaDeletedPrefix = "media:v1:deleted-name:"
)

func (s *BadgerDocumentStore) SaveMedia(ctx context.Context, file media.File) (media.File, error) {
	if err := media.Validate(file); err != nil {
		return media.File{}, err
	}
	file.SearchName = media.SearchKey(file.Name)
	blob, err := json.Marshal(file)
	if err != nil {
		return media.File{}, err
	}
	key := mediaFileKey(file.Name)
	hashKey := mediaHashKey(file.Hash)
	err = s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		if _, err := txn.Get([]byte(mediaDeletedPrefix + strings.ToLower(file.Name))); err == nil {
			return media.ErrAlreadyExists
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		if _, err := txn.Get(key); err == nil {
			return media.ErrAlreadyExists
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		if item, err := txn.Get(hashKey); err == nil {
			var existingName string
			if err := item.Value(func(value []byte) error {
				existingName = string(value)
				return nil
			}); err != nil {
				return err
			}
			if existingName != file.Name {
				return media.ErrAlreadyExists
			}
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		if err := txn.Set(key, blob); err != nil {
			return err
		}
		return txn.Set(hashKey, []byte(file.Name))
	})
	if err != nil {
		return media.File{}, err
	}
	return file, nil
}

func (s *BadgerDocumentStore) DeleteMedia(ctx context.Context, name, expectedHash, actor string) (media.File, error) {
	var file media.File
	normalized, err := media.NormalizeName(name)
	if err != nil || normalized != name || actor == "" {
		return file, media.ErrInvalidName
	}
	err = s.db.Update(func(tx *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry, err := tx.Get(mediaFileKey(name))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return media.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := entry.Value(func(raw []byte) error { return json.Unmarshal(raw, &file) }); err != nil {
			return err
		}
		if file.Name != name || file.Hash != expectedHash {
			return media.ErrInvalidName
		}
		// A concurrent new document must invalidate this snapshot as well as edits
		// to existing records observed by the prefix iterator.
		if _, err := tx.Get([]byte(documentWIDCounter)); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		if _, err := tx.Get([]byte(boxGenerationKey)); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(latestDocumentPrefix)
		it := tx.NewIterator(opts)
		defer it.Close()
		parser := techmark.NewParser()
		for it.Rewind(); it.Valid(); it.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			var doc document.Document
			if err := it.Item().Value(func(raw []byte) error { var err error; doc, err = unmarshalDocumentXML(raw); return err }); err != nil {
				return err
			}
			if !strings.Contains(doc.Source, "<") {
				continue
			}
			if parser.UsesMediaFile(doc.Source, file.Name) {
				return media.ErrInUse
			}
			parser.ClearCache()
		}
		boxOpts := badger.DefaultIteratorOptions
		boxOpts.Prefix = []byte(boxContentPrefix)
		boxes := tx.NewIterator(boxOpts)
		defer boxes.Close()
		for boxes.Rewind(); boxes.ValidForPrefix([]byte(boxContentPrefix)); boxes.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			var content struct {
				Source string `json:"source"`
			}
			if err := boxes.Item().Value(func(raw []byte) error { return json.Unmarshal(raw, &content) }); err != nil {
				return err
			}
			if parser.UsesMediaFile(content.Source, file.Name) {
				return media.ErrInUse
			}
			parser.ClearCache()
		}
		if err := tx.Delete(mediaFileKey(file.Name)); err != nil {
			return err
		}
		if err := tx.Delete(mediaHashKey(file.Hash)); err != nil {
			return err
		}
		// Reserve old public URLs: reusing a filename could show a new upload in
		// old revisions and conflict with immutable browser/CDN caches.
		if err := tx.Set([]byte(mediaDeletedPrefix+strings.ToLower(file.Name)), []byte{1}); err != nil {
			return err
		}
		now := time.Now().UTC()
		audit := document.AuditEntry{ID: orderedWorkflowID(now), Actor: actor, Action: "media.delete", TargetType: "media", Target: file.Name, Result: "success", CreatedAt: now}
		raw, err := json.Marshal(audit)
		if err != nil {
			return err
		}
		return tx.Set([]byte(auditPrefix+audit.ID), raw)
	})
	return file, err
}

func (s *BadgerDocumentStore) GetMedia(ctx context.Context, name string) (media.File, error) {
	normalized, err := media.NormalizeName(name)
	if err != nil {
		return media.File{}, err
	}
	var result media.File
	err = s.db.View(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		item, err := txn.Get(mediaFileKey(normalized))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return media.ErrNotFound
		}
		if err != nil {
			return err
		}
		return item.Value(func(value []byte) error { return json.Unmarshal(value, &result) })
	})
	return result, err
}

func (s *BadgerDocumentStore) GetMediaByHash(ctx context.Context, hash string) (media.File, error) {
	var name string
	err := s.db.View(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		item, err := txn.Get(mediaHashKey(hash))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return media.ErrNotFound
		}
		if err != nil {
			return err
		}
		return item.Value(func(value []byte) error {
			name = string(value)
			return nil
		})
	})
	if err != nil {
		return media.File{}, err
	}
	return s.GetMedia(ctx, name)
}

func (s *BadgerDocumentStore) SearchMedia(ctx context.Context, query string, limit int) ([]media.File, error) {
	if limit < 1 {
		limit = 24
	}
	if limit > 100 {
		limit = 100
	}
	query = media.SearchKey(query)
	results := make([]media.File, 0, limit)
	err := s.db.View(func(txn *badger.Txn) error {
		options := badger.DefaultIteratorOptions
		options.Prefix = []byte(mediaFilePrefix)
		iterator := txn.NewIterator(options)
		defer iterator.Close()
		for iterator.Rewind(); iterator.Valid(); iterator.Next() {
			if err := contextDone(ctx); err != nil {
				return err
			}
			var file media.File
			if err := iterator.Item().Value(func(value []byte) error { return json.Unmarshal(value, &file) }); err != nil {
				return err
			}
			if query != "" && !strings.Contains(file.SearchName, query) && !strings.Contains(strings.ToLower(file.OriginalName), query) {
				continue
			}
			results = append(results, file)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].CreatedAt.Equal(results[j].CreatedAt) {
			return results[i].Name < results[j].Name
		}
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func mediaFileKey(name string) []byte {
	return []byte(mediaFilePrefix + strings.ToLower(name))
}

func mediaHashKey(hash string) []byte {
	return []byte(mediaHashPrefix + strings.ToLower(strings.TrimSpace(hash)))
}
