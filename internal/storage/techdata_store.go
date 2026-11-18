package storage

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	badger "github.com/dgraph-io/badger/v4"

	"techpedia/internal/document"
	"techpedia/internal/techdata"
)

var _ techdata.Store = (*BadgerDocumentStore)(nil)

const (
	techdataItemPrefix = "techdata:v1:item:"
	techdataWikiPrefix = "techdata:v1:wiki:"
	techdataTIDCounter = "techdata:v1:counter:tid"
)

func (s *BadgerDocumentStore) CreateItem(ctx context.Context, input techdata.Item, options ...techdata.CreateItemOptions) (techdata.Item, error) {
	if err := ctx.Err(); err != nil {
		return techdata.Item{}, err
	}
	if input.ID != "" {
		return techdata.Item{}, fmt.Errorf("create techdata item: ID is assigned by the store")
	}

	item, err := techdata.NormalizeItem(input)
	if err != nil {
		return techdata.Item{}, err
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		for _, wid := range item.WikiPages {
			if _, err := txn.Get(techdataWikiKey(wid)); err == nil {
				return techdata.ErrWikiPageLinked
			} else if !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
		}
		confirmDistinct := len(options) > 0 && options[0].ConfirmDistinct
		if !confirmDistinct {
			candidates, err := duplicateTechdataItemsTxn(ctx, txn, item)
			if err != nil {
				return err
			}
			if len(candidates) > 0 {
				return &techdata.DuplicateItemError{Candidates: candidates}
			}
		}

		number, err := nextTechdataID(txn, techdataTIDCounter)
		if err != nil {
			return err
		}
		item.ID, err = techdata.NewTID(number)
		if err != nil {
			return err
		}

		now := time.Now().UTC()
		item.Version = 1
		item.CreatedAt = now
		item.UpdatedAt = now
		if err := setTechdataItem(txn, item); err != nil {
			return err
		}
		for _, wid := range item.WikiPages {
			if err := txn.Set(techdataWikiKey(wid), []byte(item.ID)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return techdata.Item{}, err
	}
	return item, nil
}

func duplicateTechdataItemsTxn(ctx context.Context, txn *badger.Txn, input techdata.Item) ([]techdata.Item, error) {
	names := techdataIdentityNames(input)
	locales := techdataIdentityLocales(input)
	if len(names) == 0 || len(locales) == 0 {
		return nil, nil
	}
	options := badger.DefaultIteratorOptions
	options.Prefix = []byte(techdataItemPrefix)
	iterator := txn.NewIterator(options)
	defer iterator.Close()
	candidates := make([]techdata.Item, 0)
	for iterator.Seek(options.Prefix); iterator.ValidForPrefix(options.Prefix); iterator.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var candidate techdata.Item
		if err := iterator.Item().Value(func(value []byte) error { return json.Unmarshal(value, &candidate) }); err != nil {
			return nil, err
		}
		normalized, err := techdata.NormalizeItem(candidate)
		if err != nil {
			return nil, err
		}
		if !hasAvailableTechdataLocale(normalized, locales) || !techdataNamesOverlap(names, techdataIdentityNames(normalized)) {
			continue
		}
		candidates = append(candidates, normalized)
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	return candidates, nil
}

func techdataIdentityNames(item techdata.Item) map[string]struct{} {
	result := make(map[string]struct{})
	for _, value := range item.Labels {
		if normalized := techdata.NormalizeIdentityName(value); normalized != "" {
			result[normalized] = struct{}{}
		}
	}
	for _, values := range item.Aliases {
		for _, value := range values {
			if normalized := techdata.NormalizeIdentityName(value); normalized != "" {
				result[normalized] = struct{}{}
			}
		}
	}
	return result
}

func techdataIdentityLocales(item techdata.Item) map[string]struct{} {
	result := make(map[string]struct{}, len(item.Labels)+len(item.Aliases)+len(item.WikiPages))
	for locale := range item.Labels {
		result[locale] = struct{}{}
	}
	for locale := range item.Aliases {
		result[locale] = struct{}{}
	}
	for locale := range item.WikiPages {
		result[locale] = struct{}{}
	}
	return result
}

func hasAvailableTechdataLocale(item techdata.Item, locales map[string]struct{}) bool {
	for locale := range locales {
		if item.WikiPages[locale] == "" {
			return true
		}
	}
	return false
}

func techdataNamesOverlap(left map[string]struct{}, right map[string]struct{}) bool {
	for value := range left {
		if _, exists := right[value]; exists {
			return true
		}
	}
	return false
}

func (s *BadgerDocumentStore) GetItem(ctx context.Context, id techdata.TID) (techdata.Item, error) {
	if err := ctx.Err(); err != nil {
		return techdata.Item{}, err
	}
	if err := id.Validate(); err != nil {
		return techdata.Item{}, err
	}

	var item techdata.Item
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		item, err = readTechdataItem(txn, id)
		return err
	})
	return item, err
}

func (s *BadgerDocumentStore) DeleteItem(ctx context.Context, id techdata.TID) (techdata.Item, error) {
	if err := ctx.Err(); err != nil {
		return techdata.Item{}, err
	}
	if err := id.Validate(); err != nil {
		return techdata.Item{}, err
	}

	var deleted techdata.Item
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, err := readTechdataItem(txn, id)
		if err != nil {
			return err
		}
		if len(item.WikiPages) > 0 || len(item.Statements) > 0 {
			return techdata.ErrItemInUse
		}

		itemOptions := badger.DefaultIteratorOptions
		itemOptions.Prefix = []byte(techdataItemPrefix)
		itemIterator := txn.NewIterator(itemOptions)
		for itemIterator.Seek(itemOptions.Prefix); itemIterator.ValidForPrefix(itemOptions.Prefix); itemIterator.Next() {
			if err := ctx.Err(); err != nil {
				itemIterator.Close()
				return err
			}
			var candidate techdata.Item
			if err := itemIterator.Item().Value(func(value []byte) error { return json.Unmarshal(value, &candidate) }); err != nil {
				itemIterator.Close()
				return err
			}
			if candidate.ID != id && itemReferencesTID(candidate, id) {
				itemIterator.Close()
				return techdata.ErrItemInUse
			}
		}
		itemIterator.Close()

		wikiOptions := badger.DefaultIteratorOptions
		wikiOptions.Prefix = []byte(techdataWikiPrefix)
		wikiIterator := txn.NewIterator(wikiOptions)
		for wikiIterator.Seek(wikiOptions.Prefix); wikiIterator.ValidForPrefix(wikiOptions.Prefix); wikiIterator.Next() {
			if err := ctx.Err(); err != nil {
				wikiIterator.Close()
				return err
			}
			value, err := wikiIterator.Item().ValueCopy(nil)
			if err != nil {
				wikiIterator.Close()
				return err
			}
			if string(value) == id.String() {
				wikiIterator.Close()
				return techdata.ErrItemInUse
			}
		}
		wikiIterator.Close()

		if err := txn.Delete(techdataItemKey(id)); err != nil {
			return err
		}
		deleted = item
		return nil
	})
	return deleted, err
}

func itemReferencesTID(item techdata.Item, id techdata.TID) bool {
	for _, statement := range item.Statements {
		if valueReferencesTID(statement.Value, id) {
			return true
		}
		for _, qualifier := range statement.Qualifiers {
			if valueReferencesTID(qualifier.Value, id) {
				return true
			}
		}
		for _, reference := range statement.References {
			for _, snak := range reference.Snaks {
				if valueReferencesTID(snak.Value, id) {
					return true
				}
			}
		}
	}
	return false
}

func valueReferencesTID(value techdata.Value, id techdata.TID) bool {
	return value.Item == id || value.Unit == id
}

func (s *BadgerDocumentStore) ListItems(ctx context.Context) ([]techdata.Item, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	items := make([]techdata.Item, 0)
	err := s.db.View(func(txn *badger.Txn) error {
		options := badger.DefaultIteratorOptions
		options.Prefix = []byte(techdataItemPrefix)
		iterator := txn.NewIterator(options)
		defer iterator.Close()
		for iterator.Seek(options.Prefix); iterator.ValidForPrefix(options.Prefix); iterator.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			var item techdata.Item
			if err := iterator.Item().Value(func(value []byte) error { return json.Unmarshal(value, &item) }); err != nil {
				return err
			}
			normalized, err := techdata.NormalizeItem(item)
			if err != nil {
				return err
			}
			items = append(items, normalized)
		}
		return nil
	})
	sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, err
}

func (s *BadgerDocumentStore) GetItemByWikiPage(ctx context.Context, wid document.WID) (techdata.Item, error) {
	if err := ctx.Err(); err != nil {
		return techdata.Item{}, err
	}
	if err := wid.Validate(); err != nil {
		return techdata.Item{}, err
	}

	var result techdata.Item
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(techdataWikiKey(wid))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return techdata.ErrItemNotFound
		}
		if err != nil {
			return err
		}

		var id techdata.TID
		if err := item.Value(func(value []byte) error {
			parsed, err := techdata.ParseTID(string(value))
			if err != nil {
				return err
			}
			id = parsed
			return nil
		}); err != nil {
			return err
		}
		result, err = readTechdataItem(txn, id)
		return err
	})
	return result, err
}

func (s *BadgerDocumentStore) AttachWikiPage(ctx context.Context, id techdata.TID, locale string, wid document.WID, label string) (techdata.Item, error) {
	if err := ctx.Err(); err != nil {
		return techdata.Item{}, err
	}
	if err := id.Validate(); err != nil {
		return techdata.Item{}, err
	}
	if err := wid.Validate(); err != nil {
		return techdata.Item{}, err
	}
	if !document.IsSupportedLocale(locale) {
		return techdata.Item{}, fmt.Errorf("%w: %s", techdata.ErrUnsupportedLocale, locale)
	}
	label = strings.TrimSpace(label)

	var result techdata.Item
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, err := readTechdataItem(txn, id)
		if err != nil {
			return err
		}
		result, err = attachWikiPageTxn(txn, item, locale, wid, label)
		if err != nil {
			return err
		}
		return nil
	})
	return result, err
}

func attachWikiPageTxn(txn *badger.Txn, item techdata.Item, locale string, wid document.WID, label string) (techdata.Item, error) {
	label = strings.TrimSpace(label)
	if current, ok := item.WikiPages[locale]; ok {
		if current != wid {
			return techdata.Item{}, techdata.ErrLocaleOccupied
		}
		if item.Labels[locale] == "" && label != "" {
			item.Labels[locale] = label
			item.Version++
			item.UpdatedAt = time.Now().UTC()
			if err := setTechdataItem(txn, item); err != nil {
				return techdata.Item{}, err
			}
		}
		return item, nil
	}
	for linkedLocale, current := range item.WikiPages {
		if current == wid && linkedLocale != locale {
			return techdata.Item{}, techdata.ErrWikiPageLinked
		}
	}
	reverse, err := txn.Get(techdataWikiKey(wid))
	if err == nil {
		value, err := reverse.ValueCopy(nil)
		if err != nil {
			return techdata.Item{}, err
		}
		if string(value) != item.ID.String() {
			return techdata.Item{}, techdata.ErrWikiPageLinked
		}
		return techdata.Item{}, techdata.ErrWikiPageLinked
	}
	if !errors.Is(err, badger.ErrKeyNotFound) {
		return techdata.Item{}, err
	}
	item.WikiPages[locale] = wid
	if item.Labels[locale] == "" && label != "" {
		item.Labels[locale] = label
	}
	item.Version++
	item.UpdatedAt = time.Now().UTC()
	if err := setTechdataItem(txn, item); err != nil {
		return techdata.Item{}, err
	}
	if err := txn.Set(techdataWikiKey(wid), []byte(item.ID)); err != nil {
		return techdata.Item{}, err
	}
	return item, nil
}

func (s *BadgerDocumentStore) ensureTechdataWikiPageLabels() error {
	ctx := context.Background()
	items, err := s.ListItems(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		missing := make(map[string]string)
		for locale, wid := range item.WikiPages {
			if strings.TrimSpace(item.Labels[locale]) != "" {
				continue
			}
			doc, err := s.GetLatestByWID(ctx, wid)
			if errors.Is(err, ErrDocumentNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if doc.Locale == locale && strings.TrimSpace(doc.Title) != "" {
				missing[locale] = strings.TrimSpace(doc.Title)
			}
		}
		if len(missing) == 0 {
			continue
		}
		if err := s.db.Update(func(txn *badger.Txn) error {
			current, err := readTechdataItem(txn, item.ID)
			if err != nil {
				return err
			}
			changed := false
			for locale, label := range missing {
				if strings.TrimSpace(current.Labels[locale]) == "" {
					current.Labels[locale] = label
					changed = true
				}
			}
			if !changed {
				return nil
			}
			current.Version++
			current.UpdatedAt = time.Now().UTC()
			return setTechdataItem(txn, current)
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *BadgerDocumentStore) DetachWikiPage(ctx context.Context, id techdata.TID, locale string) (techdata.Item, error) {
	if err := ctx.Err(); err != nil {
		return techdata.Item{}, err
	}
	if err := id.Validate(); err != nil {
		return techdata.Item{}, err
	}
	if !document.IsSupportedLocale(locale) {
		return techdata.Item{}, fmt.Errorf("%w: %s", techdata.ErrUnsupportedLocale, locale)
	}

	var result techdata.Item
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, err := readTechdataItem(txn, id)
		if err != nil {
			return err
		}
		wid, ok := item.WikiPages[locale]
		if !ok {
			result = item
			return nil
		}

		delete(item.WikiPages, locale)
		item.Version++
		item.UpdatedAt = time.Now().UTC()
		if err := setTechdataItem(txn, item); err != nil {
			return err
		}
		if err := txn.Delete(techdataWikiKey(wid)); err != nil {
			return err
		}
		result = item
		return nil
	})
	return result, err
}

func nextTechdataID(txn *badger.Txn, key string) (uint64, error) {
	var current uint64
	item, err := txn.Get([]byte(key))
	if err == nil {
		if err := item.Value(func(value []byte) error {
			if len(value) != 8 {
				return fmt.Errorf("invalid Techdata counter %q", key)
			}
			current = binary.BigEndian.Uint64(value)
			return nil
		}); err != nil {
			return 0, err
		}
	} else if !errors.Is(err, badger.ErrKeyNotFound) {
		return 0, err
	}

	current++
	if current == 0 {
		return 0, fmt.Errorf("Techdata identifier space exhausted")
	}
	value := make([]byte, 8)
	binary.BigEndian.PutUint64(value, current)
	if err := txn.Set([]byte(key), value); err != nil {
		return 0, err
	}
	return current, nil
}

func readTechdataItem(txn *badger.Txn, id techdata.TID) (techdata.Item, error) {
	entry, err := txn.Get(techdataItemKey(id))
	if errors.Is(err, badger.ErrKeyNotFound) {
		return techdata.Item{}, techdata.ErrItemNotFound
	}
	if err != nil {
		return techdata.Item{}, err
	}

	var result techdata.Item
	if err := entry.Value(func(value []byte) error {
		return json.Unmarshal(value, &result)
	}); err != nil {
		return techdata.Item{}, err
	}
	return techdata.NormalizeItem(result)
}

func setTechdataItem(txn *badger.Txn, item techdata.Item) error {
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	return txn.Set(techdataItemKey(item.ID), data)
}

func techdataItemKey(id techdata.TID) []byte {
	return []byte(techdataItemPrefix + id.String())
}

func techdataWikiKey(wid document.WID) []byte {
	return []byte(techdataWikiPrefix + wid.String())
}
