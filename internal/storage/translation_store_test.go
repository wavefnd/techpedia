package storage

import (
	"context"
	"errors"
	"testing"

	"techpedia/internal/document"
	"techpedia/internal/techdata"
)

func TestSaveTranslationCreatesAndLinksTechdataItem(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	source, err := store.Save(ctx, document.Document{Locale: "ko", Slug: "operating-system", Title: "운영체제", Source: "# 운영체제"})
	if err != nil {
		t.Fatalf("save source: %v", err)
	}
	translation, err := store.SaveTranslation(ctx, document.Document{Locale: "en", Slug: "operating-system", Title: "Operating System", Source: "# Operating System"}, source.ID)
	if err != nil {
		t.Fatalf("save translation: %v", err)
	}
	item, err := store.GetItemByWikiPage(ctx, source.ID)
	if err != nil {
		t.Fatalf("get source item: %v", err)
	}
	translatedItem, err := store.GetItemByWikiPage(ctx, translation.ID)
	if err != nil {
		t.Fatalf("get translation item: %v", err)
	}
	if item.ID != translatedItem.ID || item.WikiPages["ko"] != source.ID || item.WikiPages["en"] != translation.ID {
		t.Fatalf("translation was not linked to one item: %#v", item)
	}
	if item.Labels["ko"] != "운영체제" || item.Labels["en"] != "Operating System" {
		t.Fatalf("document names were not used as initial labels: %#v", item.Labels)
	}
}

func TestSaveTranslationRollsBackDocumentWhenLocaleIsOccupied(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	source, err := store.Save(ctx, document.Document{Locale: "ko", Slug: "license", Title: "라이선스", Source: "# 라이선스"})
	if err != nil {
		t.Fatalf("save source: %v", err)
	}
	existing, err := store.Save(ctx, document.Document{Locale: "en", Slug: "license", Title: "License", Source: "# License"})
	if err != nil {
		t.Fatalf("save existing translation: %v", err)
	}
	if _, err := store.CreateItem(ctx, techdata.Item{
		Labels:    map[string]string{"ko": "라이선스", "en": "License"},
		WikiPages: map[string]document.WID{"ko": source.ID, "en": existing.ID},
	}); err != nil {
		t.Fatalf("create item: %v", err)
	}
	_, err = store.SaveTranslation(ctx, document.Document{Locale: "en", Slug: "software-license", Title: "Software License", Source: "# Software License"}, source.ID)
	if !errors.Is(err, techdata.ErrLocaleOccupied) {
		t.Fatalf("expected occupied locale conflict, got %v", err)
	}
	if _, err := store.GetLatest(ctx, "en", "software-license"); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("conflicting translation left a saved document: %v", err)
	}
}

func TestSaveTranslationDoesNotOverwriteExistingTarget(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	source, err := store.Save(ctx, document.Document{Locale: "ko", Slug: "database", Title: "데이터베이스", Source: "# 데이터베이스"})
	if err != nil {
		t.Fatalf("save source: %v", err)
	}
	existing, err := store.Save(ctx, document.Document{Locale: "en", Slug: "database", Title: "Database", Source: "original"})
	if err != nil {
		t.Fatalf("save target: %v", err)
	}
	_, err = store.SaveTranslation(ctx, document.Document{Locale: "en", Slug: "database", Title: "Database", Source: "replacement"}, source.ID)
	if !errors.Is(err, ErrDocumentExists) {
		t.Fatalf("expected existing target conflict, got %v", err)
	}
	persisted, err := store.GetLatest(ctx, "en", "database")
	if err != nil {
		t.Fatalf("load target after conflict: %v", err)
	}
	if persisted.ID != existing.ID || persisted.Source != "original" || persisted.Revision != "r1" {
		t.Fatalf("existing target changed after conflict: %#v", persisted)
	}
}

func TestSaveTranslationRollsBackWhenSourceWouldCreateDuplicateTechdata(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	source, err := store.Save(ctx, document.Document{Locale: "ko", Slug: "c", Title: "C", Source: "# C"})
	if err != nil {
		t.Fatalf("save source: %v", err)
	}
	existing, err := store.Save(ctx, document.Document{Locale: "en", Slug: "c", Title: "C", Source: "# C"})
	if err != nil {
		t.Fatalf("save existing English page: %v", err)
	}
	item, err := store.CreateItem(ctx, techdata.Item{
		Labels:    map[string]string{"en": "C"},
		WikiPages: map[string]document.WID{"en": existing.ID},
	})
	if err != nil {
		t.Fatalf("create existing Techdata item: %v", err)
	}

	_, err = store.SaveTranslation(ctx, document.Document{Locale: "es", Slug: "c", Title: "C", Source: "# C"}, source.ID)
	var duplicate *techdata.DuplicateItemError
	if !errors.As(err, &duplicate) || len(duplicate.Candidates) != 1 || duplicate.Candidates[0].ID != item.ID {
		t.Fatalf("expected existing Techdata candidate, got %#v, %v", duplicate, err)
	}
	if _, err := store.GetLatest(ctx, "es", "c"); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("duplicate translation left a saved target document: %v", err)
	}
	if _, err := store.GetItemByWikiPage(ctx, source.ID); !errors.Is(err, techdata.ErrItemNotFound) {
		t.Fatalf("duplicate translation linked the source document: %v", err)
	}
}
