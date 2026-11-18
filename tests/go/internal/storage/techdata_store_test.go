package storage

import (
	"context"
	"errors"
	"testing"

	badger "github.com/dgraph-io/badger/v4"

	"techpedia/internal/document"
	"techpedia/internal/techdata"
)

func TestTechdataItemLifecycle(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	item, err := store.CreateItem(ctx, techdata.Item{
		Labels: map[string]string{
			"ko": " C ",
			"en": "C",
		},
		Descriptions: map[string]string{
			"ko": "프로그래밍 언어",
		},
		Aliases: map[string][]string{
			"ko": {"C 언어", "C 언어", " 씨언어 "},
		},
	})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	if item.ID != "T1" || item.Version != 1 {
		t.Fatalf("expected first item T1 version 1, got %#v", item)
	}
	if item.Labels["ko"] != "C" || len(item.Aliases["ko"]) != 2 {
		t.Fatalf("expected normalized localized data, got %#v", item)
	}
	items, err := store.ListItems(ctx)
	if err != nil || len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("list Techdata items: %#v, %v", items, err)
	}

	item, err = store.AttachWikiPage(ctx, item.ID, "ko", "W135", "C")
	if err != nil {
		t.Fatalf("attach ko page: %v", err)
	}
	item, err = store.AttachWikiPage(ctx, item.ID, "en", "W921", "C programming language")
	if err != nil {
		t.Fatalf("attach en page: %v", err)
	}
	if item.WikiPages["ko"] != "W135" || item.WikiPages["en"] != "W921" {
		t.Fatalf("unexpected wiki links: %#v", item.WikiPages)
	}
	if item.Labels["en"] != "C" {
		t.Fatalf("expected the existing en label to be preserved: %#v", item.Labels)
	}
	unlabeled, err := store.CreateItem(ctx, techdata.Item{})
	if err != nil {
		t.Fatalf("create unlabeled item: %v", err)
	}
	unlabeled, err = store.AttachWikiPage(ctx, unlabeled.ID, "en", "W922", "License")
	if err != nil || unlabeled.Labels["en"] != "License" {
		t.Fatalf("expected attached page title to initialize the missing en label: %#v, %v", unlabeled, err)
	}

	byWiki, err := store.GetItemByWikiPage(ctx, "W135")
	if err != nil {
		t.Fatalf("get by wiki page: %v", err)
	}
	if byWiki.ID != item.ID {
		t.Fatalf("expected %s, got %s", item.ID, byWiki.ID)
	}

	item, err = store.DetachWikiPage(ctx, item.ID, "en")
	if err != nil {
		t.Fatalf("detach en page: %v", err)
	}
	if _, ok := item.WikiPages["en"]; ok {
		t.Fatalf("expected en page to be detached: %#v", item.WikiPages)
	}
	if _, err := store.GetItemByWikiPage(ctx, "W921"); !errors.Is(err, techdata.ErrItemNotFound) {
		t.Fatalf("expected detached reverse index to be gone, got %v", err)
	}
}

func TestBackfillTechdataWikiPageLabelsFromDocumentTitles(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	ko, err := store.Save(ctx, document.Document{Locale: "ko", Slug: "license", Title: "라이선스", Source: "# 라이선스"})
	if err != nil {
		t.Fatalf("save ko document: %v", err)
	}
	en, err := store.Save(ctx, document.Document{Locale: "en", Slug: "license", Title: "License", Source: "# License"})
	if err != nil {
		t.Fatalf("save en document: %v", err)
	}
	item, err := store.CreateItem(ctx, techdata.Item{
		Labels:    map[string]string{"ko": "사용자 지정 라이선스"},
		WikiPages: map[string]document.WID{"ko": ko.ID, "en": en.ID},
	})
	if err != nil {
		t.Fatalf("create legacy item: %v", err)
	}
	if err := store.ensureTechdataWikiPageLabels(); err != nil {
		t.Fatalf("backfill labels: %v", err)
	}
	item, err = store.GetItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if item.Labels["ko"] != "사용자 지정 라이선스" || item.Labels["en"] != "License" {
		t.Fatalf("backfill did not preserve existing labels and fill missing labels: %#v", item.Labels)
	}
}

func TestTechdataWikiLinkConflicts(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	first, err := store.CreateItem(ctx, techdata.Item{})
	if err != nil {
		t.Fatalf("create first item: %v", err)
	}
	second, err := store.CreateItem(ctx, techdata.Item{})
	if err != nil {
		t.Fatalf("create second item: %v", err)
	}

	if _, err := store.AttachWikiPage(ctx, first.ID, "ko", "W1", "First"); err != nil {
		t.Fatalf("attach first wiki page: %v", err)
	}
	if _, err := store.AttachWikiPage(ctx, first.ID, "ko", "W2", "Second"); !errors.Is(err, techdata.ErrLocaleOccupied) {
		t.Fatalf("expected locale conflict, got %v", err)
	}
	if _, err := store.AttachWikiPage(ctx, second.ID, "en", "W1", "First"); !errors.Is(err, techdata.ErrWikiPageLinked) {
		t.Fatalf("expected reverse link conflict, got %v", err)
	}

	stored, err := store.GetItem(ctx, second.ID)
	if err != nil {
		t.Fatalf("get second item: %v", err)
	}
	if len(stored.WikiPages) != 0 {
		t.Fatalf("conflicting write left a partial link: %#v", stored.WikiPages)
	}
}

func TestCreateTechdataItemRejectsDuplicateWikiPage(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	_, err = store.CreateItem(context.Background(), techdata.Item{
		WikiPages: map[string]document.WID{
			"ko": "W1",
			"en": "W1",
		},
	})
	if !errors.Is(err, techdata.ErrWikiPageLinked) {
		t.Fatalf("expected duplicate wiki page error, got %v", err)
	}
}

func TestCreateTechdataItemBlocksPotentialDuplicate(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	existing, err := store.CreateItem(ctx, techdata.Item{
		Labels:    map[string]string{"en": "C++"},
		Aliases:   map[string][]string{"en": {"C Plus Plus"}},
		WikiPages: map[string]document.WID{"en": "W1"},
	})
	if err != nil {
		t.Fatalf("create existing item: %v", err)
	}

	_, err = store.CreateItem(ctx, techdata.Item{
		Labels:    map[string]string{"ko": "Ｃ＋＋"},
		WikiPages: map[string]document.WID{"ko": "W2"},
	})
	var duplicate *techdata.DuplicateItemError
	if !errors.As(err, &duplicate) || !errors.Is(err, techdata.ErrPotentialDuplicate) {
		t.Fatalf("expected potential duplicate error, got %v", err)
	}
	if len(duplicate.Candidates) != 1 || duplicate.Candidates[0].ID != existing.ID {
		t.Fatalf("unexpected duplicate candidates: %#v", duplicate.Candidates)
	}
	if _, err := store.GetItemByWikiPage(ctx, "W2"); !errors.Is(err, techdata.ErrItemNotFound) {
		t.Fatalf("blocked duplicate left a reverse wiki link: %v", err)
	}

	distinct, err := store.CreateItem(ctx, techdata.Item{
		Labels:    map[string]string{"ko": "C++"},
		WikiPages: map[string]document.WID{"ko": "W2"},
	}, techdata.CreateItemOptions{ConfirmDistinct: true})
	if err != nil {
		t.Fatalf("create confirmed distinct item: %v", err)
	}
	if distinct.ID != "T2" {
		t.Fatalf("blocked duplicate consumed a TID: got %s", distinct.ID)
	}
}

func TestCreateTechdataItemAllowsOccupiedSameLocaleHomonym(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	if _, err := store.CreateItem(ctx, techdata.Item{
		Labels:    map[string]string{"en": "Mercury"},
		WikiPages: map[string]document.WID{"en": "W1"},
	}); err != nil {
		t.Fatalf("create first homonym: %v", err)
	}
	if _, err := store.CreateItem(ctx, techdata.Item{
		Labels:    map[string]string{"en": "Mercury"},
		WikiPages: map[string]document.WID{"en": "W2"},
	}); err != nil {
		t.Fatalf("same-locale occupied item should be treated as a homonym: %v", err)
	}
}

func TestDeleteTechdataItemRemovesOnlyUnusedItemAndDoesNotReuseTID(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	orphan, err := store.CreateItem(ctx, techdata.Item{
		Labels:       map[string]string{"ko": "잘못 만든 항목"},
		Descriptions: map[string]string{"ko": "삭제할 수 있는 메타데이터"},
		Aliases:      map[string][]string{"ko": {"실수"}},
	})
	if err != nil {
		t.Fatalf("create orphan: %v", err)
	}
	deleted, err := store.DeleteItem(ctx, orphan.ID)
	if err != nil {
		t.Fatalf("delete orphan: %v", err)
	}
	if deleted.ID != orphan.ID {
		t.Fatalf("deleted wrong item: got %s want %s", deleted.ID, orphan.ID)
	}
	if _, err := store.GetItem(ctx, orphan.ID); !errors.Is(err, techdata.ErrItemNotFound) {
		t.Fatalf("deleted item is still readable: %v", err)
	}
	next, err := store.CreateItem(ctx, techdata.Item{Labels: map[string]string{"ko": "다음 항목"}})
	if err != nil {
		t.Fatalf("create next item: %v", err)
	}
	if next.ID != "T2" {
		t.Fatalf("deleted TID was reused: got %s", next.ID)
	}
}

func TestDeleteTechdataItemRejectsLiveRelationships(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	linked, err := store.CreateItem(ctx, techdata.Item{WikiPages: map[string]document.WID{"ko": "W1"}})
	if err != nil {
		t.Fatalf("create linked item: %v", err)
	}
	if _, err := store.DeleteItem(ctx, linked.ID); !errors.Is(err, techdata.ErrItemInUse) {
		t.Fatalf("expected linked item rejection, got %v", err)
	}

	withStatement, err := store.CreateItem(ctx, techdata.Item{Statements: []techdata.Statement{{
		Property: "P1", Value: techdata.Value{Type: techdata.DataTypeString, Text: "value"},
	}}})
	if err != nil {
		t.Fatalf("create statement item: %v", err)
	}
	if _, err := store.DeleteItem(ctx, withStatement.ID); !errors.Is(err, techdata.ErrItemInUse) {
		t.Fatalf("expected statement item rejection, got %v", err)
	}

	target, err := store.CreateItem(ctx, techdata.Item{Labels: map[string]string{"en": "Reference target"}})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	_, err = store.CreateItem(ctx, techdata.Item{Statements: []techdata.Statement{
		{Property: "P1", Value: techdata.Value{Type: techdata.DataTypeItem, Item: target.ID}},
		{Property: "P2", Value: techdata.Value{Type: techdata.DataTypeQuantity, Amount: "1", Unit: target.ID}},
		{Property: "P3", Value: techdata.Value{Type: techdata.DataTypeString, Text: "qualified"}, Qualifiers: []techdata.Snak{{Property: "P4", Value: techdata.Value{Type: techdata.DataTypeItem, Item: target.ID}}}},
		{Property: "P5", Value: techdata.Value{Type: techdata.DataTypeString, Text: "referenced"}, References: []techdata.Reference{{Snaks: []techdata.Snak{{Property: "P6", Value: techdata.Value{Type: techdata.DataTypeQuantity, Amount: "1", Unit: target.ID}}}}}},
	}})
	if err != nil {
		t.Fatalf("create referring item: %v", err)
	}
	if _, err := store.DeleteItem(ctx, target.ID); !errors.Is(err, techdata.ErrItemInUse) {
		t.Fatalf("expected incoming reference rejection, got %v", err)
	}
}

func TestDeleteTechdataItemRejectsStaleReverseWikiIndex(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	target, err := store.CreateItem(ctx, techdata.Item{})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := store.db.Update(func(txn *badger.Txn) error {
		return txn.Set(techdataWikiKey("W99"), []byte(target.ID))
	}); err != nil {
		t.Fatalf("create stale reverse index: %v", err)
	}
	if _, err := store.DeleteItem(ctx, target.ID); !errors.Is(err, techdata.ErrItemInUse) {
		t.Fatalf("expected stale reverse index rejection, got %v", err)
	}
}
