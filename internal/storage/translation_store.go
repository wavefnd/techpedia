package storage

import (
	"context"
	"errors"
	"time"

	badger "github.com/dgraph-io/badger/v4"

	"techpedia/internal/document"
	"techpedia/internal/techdata"
)

func (s *BadgerDocumentStore) SaveTranslation(ctx context.Context, doc document.Document, sourceWID document.WID) (document.Document, error) {
	if err := sourceWID.Validate(); err != nil {
		return document.Document{}, err
	}
	if err := prepareDocumentForSave(ctx, &doc); err != nil {
		return document.Document{}, err
	}

	err := s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		source, err := documentByWIDFromTxn(txn, sourceWID)
		if err != nil {
			return err
		}
		if source.Locale == doc.Locale {
			return techdata.ErrLocaleOccupied
		}
		if _, err := documentFromTxn(txn, latestDocumentKey(doc.Locale, doc.Slug)); err == nil {
			return ErrDocumentExists
		} else if !errors.Is(err, ErrDocumentNotFound) {
			return err
		}
		if err := saveDocumentTxn(txn, &doc); err != nil {
			return err
		}
		item, err := techdataItemForSourceTxn(ctx, txn, source)
		if err != nil {
			return err
		}
		_, err = attachWikiPageTxn(txn, item, doc.Locale, doc.ID, doc.Title)
		return err
	})
	if err != nil {
		return document.Document{}, err
	}
	s.invalidateSearchCache(doc.Locale)
	return doc, nil
}

func documentByWIDFromTxn(txn *badger.Txn, wid document.WID) (document.Document, error) {
	item, err := txn.Get(documentWIDIndexKey(wid))
	if errors.Is(err, badger.ErrKeyNotFound) {
		return document.Document{}, ErrDocumentNotFound
	}
	if err != nil {
		return document.Document{}, err
	}
	value, err := item.ValueCopy(nil)
	if err != nil {
		return document.Document{}, err
	}
	locale, slug, ok := splitDocumentWIDTarget(string(value))
	if !ok {
		return document.Document{}, ErrDocumentNotFound
	}
	return documentFromTxn(txn, latestDocumentKey(locale, slug))
}

func techdataItemForSourceTxn(ctx context.Context, txn *badger.Txn, source document.Document) (techdata.Item, error) {
	reverse, err := txn.Get(techdataWikiKey(source.ID))
	if err == nil {
		value, err := reverse.ValueCopy(nil)
		if err != nil {
			return techdata.Item{}, err
		}
		id, err := techdata.ParseTID(string(value))
		if err != nil {
			return techdata.Item{}, err
		}
		return readTechdataItem(txn, id)
	}
	if !errors.Is(err, badger.ErrKeyNotFound) {
		return techdata.Item{}, err
	}
	item, err := techdata.NormalizeItem(techdata.Item{
		Labels:    map[string]string{source.Locale: source.Title},
		WikiPages: map[string]document.WID{source.Locale: source.ID},
	})
	if err != nil {
		return techdata.Item{}, err
	}
	candidates, err := duplicateTechdataItemsTxn(ctx, txn, item)
	if err != nil {
		return techdata.Item{}, err
	}
	if len(candidates) > 0 {
		return techdata.Item{}, &techdata.DuplicateItemError{Candidates: candidates}
	}
	number, err := nextTechdataID(txn, techdataTIDCounter)
	if err != nil {
		return techdata.Item{}, err
	}
	item.ID, err = techdata.NewTID(number)
	if err != nil {
		return techdata.Item{}, err
	}
	now := time.Now().UTC()
	item.Version = 1
	item.CreatedAt = now
	item.UpdatedAt = now
	if err := setTechdataItem(txn, item); err != nil {
		return techdata.Item{}, err
	}
	if err := txn.Set(techdataWikiKey(source.ID), []byte(item.ID)); err != nil {
		return techdata.Item{}, err
	}
	return item, nil
}
