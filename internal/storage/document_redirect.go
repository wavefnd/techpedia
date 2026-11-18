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
	"techpedia/internal/techmark"
)

const documentAliasSchemaVersion = 1

type storedDocumentAlias struct {
	SchemaVersion int          `json:"schema_version"`
	Locale        string       `json:"locale"`
	Name          string       `json:"name"`
	Slug          string       `json:"slug"`
	TargetWID     document.WID `json:"target_wid"`
	TargetName    string       `json:"target_name"`
	CreatedBy     string       `json:"created_by"`
	CreatedAt     time.Time    `json:"created_at"`
}

func (s *BadgerDocumentStore) CreateDocumentRedirect(ctx context.Context, locale string, name string, targetSlug string, actor string) (document.Redirect, error) {
	canonicalLocale, ok := document.CanonicalLocale(locale)
	name = strings.TrimSpace(name)
	targetSlug = strings.TrimSpace(targetSlug)
	if !ok {
		return document.Redirect{}, ErrInvalidLocale
	}
	if name == "" || targetSlug == "" {
		return document.Redirect{}, ErrDocumentNotFound
	}
	slug := techmark.Slugify(name)
	alias := storedDocumentAlias{
		SchemaVersion: documentAliasSchemaVersion,
		Locale:        canonicalLocale,
		Name:          name,
		Slug:          slug,
		CreatedBy:     strings.TrimSpace(actor),
		CreatedAt:     time.Now().UTC(),
	}
	var target document.Document
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		if _, err := txn.Get([]byte(latestDocumentKey(canonicalLocale, slug))); err == nil {
			return ErrDocumentExists
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		if exists, err := documentRedirectExistsTxn(txn, canonicalLocale, slug); err != nil {
			return err
		} else if exists {
			return ErrDocumentExists
		}

		var err error
		target, err = documentFromTxn(txn, latestDocumentKey(canonicalLocale, targetSlug))
		if errors.Is(err, ErrDocumentNotFound) {
			target, err = documentRedirectTargetTxn(txn, canonicalLocale, targetSlug)
		}
		if err != nil {
			return err
		}
		if target.Locale != canonicalLocale || target.ID == "" {
			return ErrDocumentNotFound
		}
		alias.TargetWID = target.ID
		alias.TargetName = target.Title
		data, err := json.Marshal(alias)
		if err != nil {
			return err
		}
		return txn.Set([]byte(documentAliasKey(canonicalLocale, slug)), data)
	})
	if err != nil {
		return document.Redirect{}, err
	}
	return newDocumentRedirect(alias, target), nil
}

func (s *BadgerDocumentStore) DeleteDocumentRedirect(ctx context.Context, locale string, slug string) (document.Redirect, error) {
	canonicalLocale, ok := document.CanonicalLocale(locale)
	slug = strings.TrimSpace(slug)
	if !ok {
		return document.Redirect{}, ErrInvalidLocale
	}
	if slug == "" {
		return document.Redirect{}, ErrDocumentNotFound
	}
	var alias storedDocumentAlias
	var target document.Document
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		item, err := txn.Get([]byte(documentAliasKey(canonicalLocale, slug)))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrDocumentNotFound
		}
		if err != nil {
			return err
		}
		if err := item.Value(func(value []byte) error { return decodeStoredDocumentAlias(value, &alias) }); err != nil {
			return err
		}
		target, _ = documentByWIDFromTxn(txn, alias.TargetWID)
		return txn.Delete([]byte(documentAliasKey(canonicalLocale, slug)))
	})
	if err != nil {
		return document.Redirect{}, err
	}
	return newDocumentRedirect(alias, target), nil
}

func (s *BadgerDocumentStore) ListDocumentRedirects(ctx context.Context, locale string, limit int) ([]document.Redirect, error) {
	if limit < 1 || limit > 500 {
		limit = 300
	}
	prefix := documentAliasPrefix
	if strings.TrimSpace(locale) != "" {
		canonicalLocale, ok := document.CanonicalLocale(locale)
		if !ok {
			return nil, ErrInvalidLocale
		}
		prefix = documentAliasLocalePrefix(canonicalLocale)
	}
	redirects := make([]document.Redirect, 0)
	err := s.db.View(func(txn *badger.Txn) error {
		iteratorOptions := badger.DefaultIteratorOptions
		iteratorOptions.Prefix = []byte(prefix)
		iterator := txn.NewIterator(iteratorOptions)
		defer iterator.Close()
		for iterator.Seek(iteratorOptions.Prefix); iterator.ValidForPrefix(iteratorOptions.Prefix); iterator.Next() {
			if err := contextDone(ctx); err != nil {
				return err
			}
			var alias storedDocumentAlias
			if err := iterator.Item().Value(func(value []byte) error { return decodeStoredDocumentAlias(value, &alias) }); err != nil {
				return err
			}
			target, _ := documentByWIDFromTxn(txn, alias.TargetWID)
			redirects = append(redirects, newDocumentRedirect(alias, target))
			if len(redirects) >= limit {
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(redirects, func(i, j int) bool {
		if redirects[i].Locale != redirects[j].Locale {
			return redirects[i].Locale < redirects[j].Locale
		}
		return strings.ToLower(redirects[i].Name) < strings.ToLower(redirects[j].Name)
	})
	return redirects, nil
}

func documentRedirectExistsTxn(txn *badger.Txn, locale string, slug string) (bool, error) {
	for _, key := range []string{documentAliasKey(locale, slug), documentRedirectKey(locale, slug)} {
		if _, err := txn.Get([]byte(key)); err == nil {
			return true, nil
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return false, err
		}
	}
	return false, nil
}

func documentRedirectTargetTxn(txn *badger.Txn, locale string, slug string) (document.Document, error) {
	seen := make(map[string]struct{})
	current := strings.TrimSpace(slug)
	for current != "" {
		if _, exists := seen[current]; exists {
			return document.Document{}, ErrRedirectCycle
		}
		seen[current] = struct{}{}

		if item, err := txn.Get([]byte(documentAliasKey(locale, current))); err == nil {
			var alias storedDocumentAlias
			if err := item.Value(func(value []byte) error { return decodeStoredDocumentAlias(value, &alias) }); err != nil {
				return document.Document{}, err
			}
			target, err := documentByWIDFromTxn(txn, alias.TargetWID)
			if err != nil || target.Locale != locale {
				return document.Document{}, ErrDocumentNotFound
			}
			return target, nil
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return document.Document{}, err
		}

		item, err := txn.Get([]byte(documentRedirectKey(locale, current)))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return document.Document{}, ErrDocumentNotFound
		}
		if err != nil {
			return document.Document{}, err
		}
		if err := item.Value(func(value []byte) error {
			current = strings.TrimSpace(string(value))
			return nil
		}); err != nil {
			return document.Document{}, err
		}
		if target, err := documentFromTxn(txn, latestDocumentKey(locale, current)); err == nil {
			return target, nil
		} else if !errors.Is(err, ErrDocumentNotFound) {
			return document.Document{}, err
		}
	}
	return document.Document{}, ErrDocumentNotFound
}

func newDocumentRedirect(alias storedDocumentAlias, target document.Document) document.Redirect {
	targetName := target.Title
	if targetName == "" {
		targetName = alias.TargetName
	}
	return document.Redirect{
		Locale:       alias.Locale,
		Name:         alias.Name,
		Slug:         alias.Slug,
		TargetWID:    alias.TargetWID,
		TargetName:   targetName,
		TargetSlug:   target.Slug,
		TargetExists: target.ID != "",
		CreatedBy:    alias.CreatedBy,
		CreatedAt:    alias.CreatedAt,
	}
}

func decodeStoredDocumentAlias(value []byte, alias *storedDocumentAlias) error {
	if err := json.Unmarshal(value, alias); err != nil {
		return err
	}
	locale, ok := document.CanonicalLocale(alias.Locale)
	if alias.SchemaVersion != documentAliasSchemaVersion || !ok || locale != alias.Locale || alias.Name == "" || alias.Slug == "" || alias.TargetWID.Validate() != nil {
		return errors.New("invalid document alias record")
	}
	return nil
}
