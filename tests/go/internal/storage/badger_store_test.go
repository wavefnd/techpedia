package storage

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	badger "github.com/dgraph-io/badger/v4"

	"techpedia/internal/document"
)

func TestSearchLatestDocuments(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open badger store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	docs := []document.Document{
		{
			Locale: "ko",
			Slug:   "c",
			Title:  "C",
			Source: "# C\n\nC는 Dennis Ritchie가 설계한 프로그래밍 언어다.",
		},
		{
			Locale: "ko",
			Slug:   "c-plus-plus",
			Title:  "C++",
			Source: "# C++\n\nC++는 C 계열의 프로그래밍 언어다.",
		},
		{
			Locale: "ko",
			Slug:   "rust",
			Title:  "Rust",
			Source: "# Rust\n\nRust는 시스템 프로그래밍 언어다.",
		},
	}

	for _, doc := range docs {
		if _, err := store.Save(ctx, doc); err != nil {
			t.Fatalf("save %s: %v", doc.Title, err)
		}
	}

	results, err := store.Search(ctx, "ko", "C", 8)
	if err != nil {
		t.Fatalf("search C: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected C and C++ results, got %#v", results)
	}
	if results[0].Slug != "c" {
		t.Fatalf("expected exact C result first, got %#v", results[0])
	}
	if results[1].Slug != "c-plus-plus" {
		t.Fatalf("expected C++ prefix result second, got %#v", results[1])
	}

	results, err = store.Search(ctx, "ko", "Ritchie", 8)
	if err != nil {
		t.Fatalf("search Ritchie: %v", err)
	}
	if len(results) == 0 || results[0].Slug != "c" {
		t.Fatalf("expected body match for C, got %#v", results)
	}

	results, err = store.Search(ctx, "ko", "C언어", 8)
	if err != nil {
		t.Fatalf("search C alias: %v", err)
	}
	if len(results) == 0 || results[0].Slug != "c" {
		t.Fatalf("expected C alias to resolve to C, got %#v", results)
	}

	latest, err := store.ListLatest(ctx, "ko", 10)
	if err != nil {
		t.Fatalf("list latest: %v", err)
	}
	if len(latest) != 3 {
		t.Fatalf("expected three latest docs, got %#v", latest)
	}

	random, err := store.RandomLatest(ctx, "ko")
	if err != nil {
		t.Fatalf("random latest: %v", err)
	}
	if random.Locale != "ko" || random.Slug == "" {
		t.Fatalf("expected random ko document, got %#v", random)
	}
}

func TestSearchSupportsFullPageLimit(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open badger store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	for index := 0; index < 40; index++ {
		title := fmt.Sprintf("Search result %02d", index)
		if _, err := store.Save(ctx, document.Document{
			Locale: "en",
			Slug:   fmt.Sprintf("search-result-%02d", index),
			Title:  title,
			Source: "Search page capacity test.",
		}); err != nil {
			t.Fatalf("save %s: %v", title, err)
		}
	}

	results, err := store.Search(ctx, "en", "Search result", 50)
	if err != nil {
		t.Fatalf("search full page: %v", err)
	}
	if len(results) != 40 {
		t.Fatalf("expected all 40 matching documents, got %d", len(results))
	}
}

func TestCountLatestByLocaleTracksDocumentLifecycle(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open badger store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	var korean document.Document
	for _, doc := range []document.Document{
		{Locale: "ko", Slug: "c", Title: "C", Source: "C language"},
		{Locale: "ko", Slug: "rust", Title: "Rust", Source: "Rust language"},
		{Locale: "en", Slug: "operating-system", Title: "Operating System", Source: "Operating systems"},
		{Locale: "es", Slug: "portada", Title: "Portada", Source: "Pagina principal"},
	} {
		saved, saveErr := store.Save(ctx, doc)
		if saveErr != nil {
			t.Fatalf("save %s/%s: %v", doc.Locale, doc.Title, saveErr)
		}
		if doc.Locale == "ko" && doc.Slug == "rust" {
			korean = saved
		}
	}

	counts, err := store.CountLatestByLocale(ctx)
	if err != nil {
		t.Fatalf("count latest documents: %v", err)
	}
	if counts["ko"] != 2 || counts["en"] != 1 || counts["es"] != 1 {
		t.Fatalf("unexpected locale counts: %#v", counts)
	}
	counts["ko"] = 99
	cached, err := store.CountLatestByLocale(ctx)
	if err != nil || cached["ko"] != 2 {
		t.Fatalf("callers must not mutate cached counts: %#v, %v", cached, err)
	}

	if _, err := store.DeleteDocument(ctx, "ko", korean.Slug, "admin", "count test"); err != nil {
		t.Fatalf("delete Korean document: %v", err)
	}
	counts, err = store.CountLatestByLocale(ctx)
	if err != nil || counts["ko"] != 1 {
		t.Fatalf("deleted document remained in count: %#v, %v", counts, err)
	}
	if _, err := store.RestoreDeletedDocument(ctx, korean.ID, "admin", "count test"); err != nil {
		t.Fatalf("restore Korean document: %v", err)
	}
	counts, err = store.CountLatestByLocale(ctx)
	if err != nil || counts["ko"] != 2 {
		t.Fatalf("restored document missing from count: %#v, %v", counts, err)
	}
}

func TestSearchCacheInvalidatesForDocumentLifecycle(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open badger store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	saved, err := store.Save(ctx, document.Document{Locale: "ko", Slug: "c-plus-plus", Title: "C++", Source: "C++\n\n범용 프로그래밍 언어이다."})
	if err != nil {
		t.Fatalf("save initial document: %v", err)
	}
	if _, err := store.Search(ctx, "ko", "C++", 8); err != nil {
		t.Fatalf("populate search cache: %v", err)
	}

	saved.Source = "C++\n\n캐시무효화가 반영된 프로그래밍 언어이다."
	saved, err = store.Save(ctx, saved)
	if err != nil {
		t.Fatalf("update cached document: %v", err)
	}
	results, err := store.Search(ctx, "ko", "캐시무효화", 8)
	if err != nil || len(results) != 1 || results[0].Title != "C++" {
		t.Fatalf("updated document missing after cache invalidation: %#v, %v", results, err)
	}

	moved, err := store.MoveDocument(ctx, "ko", saved.Slug, "cxx", "CXX")
	if err != nil {
		t.Fatalf("move cached document: %v", err)
	}
	results, err = store.Search(ctx, "ko", "CXX", 8)
	if err != nil || len(results) != 1 || results[0].Slug != "cxx" {
		t.Fatalf("moved document missing after cache invalidation: %#v, %v", results, err)
	}

	if _, err := store.DeleteDocument(ctx, "ko", moved.Slug, "admin", "test"); err != nil {
		t.Fatalf("delete cached document: %v", err)
	}
	if results, err = store.Search(ctx, "ko", "CXX", 8); err != nil || len(results) != 0 {
		t.Fatalf("deleted document remained in search cache: %#v, %v", results, err)
	}
	if _, err := store.RestoreDeletedDocument(ctx, moved.ID, "admin", "test"); err != nil {
		t.Fatalf("restore cached document: %v", err)
	}
	if results, err = store.Search(ctx, "ko", "CXX", 8); err != nil || len(results) != 1 {
		t.Fatalf("restored document missing from search cache: %#v, %v", results, err)
	}
}

func TestDocumentWIDIsAssignedAndPreserved(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open badger store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	first, err := store.Save(ctx, document.Document{Locale: "ko", Slug: "C", Title: "C", Source: "first"})
	if err != nil {
		t.Fatalf("save first revision: %v", err)
	}
	if err := first.ID.Validate(); err != nil {
		t.Fatalf("invalid assigned WID %q: %v", first.ID, err)
	}
	second, err := store.Save(ctx, document.Document{Locale: "ko", Slug: "C", Title: "C", Source: "second"})
	if err != nil {
		t.Fatalf("save second revision: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("WID changed across revisions: %q -> %q", first.ID, second.ID)
	}
	byWID, err := store.GetLatestByWID(ctx, first.ID)
	if err != nil || byWID.ID != first.ID || byWID.Source != "second" {
		t.Fatalf("get latest by WID: %#v, %v", byWID, err)
	}
	moved, err := store.MoveDocument(ctx, "ko", "C", "C_language", "C language")
	if err != nil {
		t.Fatalf("move document: %v", err)
	}
	byWID, err = store.GetLatestByWID(ctx, first.ID)
	if err != nil || byWID.Slug != moved.Slug || byWID.Title != moved.Title {
		t.Fatalf("WID lookup did not follow move: %#v, %v", byWID, err)
	}
	if moved.ID != first.ID {
		t.Fatalf("WID changed during move: %q -> %q", first.ID, moved.ID)
	}
}

func TestListRecentRevisionsIncludesEveryRevisionForLocale(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open badger store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	for _, doc := range []document.Document{
		{Locale: "ko", Slug: "c", Title: "C", Source: "첫 리비전", Author: "alice"},
		{Locale: "en", Slug: "c", Title: "C", Source: "English revision", Author: "bob"},
		{Locale: "ko", Slug: "rust", Title: "Rust", Source: "Rust 리비전", Author: "carol"},
		{Locale: "ko", Slug: "c", Title: "C", Source: "두 번째 리비전", Author: "alice"},
	} {
		if _, err := store.Save(ctx, doc); err != nil {
			t.Fatalf("save %s/%s: %v", doc.Locale, doc.Title, err)
		}
		time.Sleep(time.Millisecond)
	}

	revisions, err := store.ListRecentRevisions(ctx, "ko", 10)
	if err != nil {
		t.Fatalf("list recent revisions: %v", err)
	}
	if len(revisions) != 3 {
		t.Fatalf("expected all three Korean revisions, got %#v", revisions)
	}
	if revisions[0].Title != "C" || revisions[0].Revision != "r2" {
		t.Fatalf("expected latest C revision first, got %#v", revisions[0])
	}
	for index := 1; index < len(revisions); index++ {
		previous, _ := time.Parse(time.RFC3339, revisions[index-1].UpdatedAt)
		current, _ := time.Parse(time.RFC3339, revisions[index].UpdatedAt)
		if previous.Before(current) {
			t.Fatalf("recent revisions are not newest first: %#v", revisions)
		}
	}
}

func TestPageViewsProduceLocaleAwarePopularDocuments(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open badger store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	for _, doc := range []document.Document{
		{Locale: "ko", Slug: "c", Title: "C", Source: "C"},
		{Locale: "ko", Slug: "rust", Title: "Rust", Source: "Rust"},
		{Locale: "en", Slug: "c", Title: "C", Source: "C"},
	} {
		if _, err := store.Save(ctx, doc); err != nil {
			t.Fatalf("save %s/%s: %v", doc.Locale, doc.Slug, err)
		}
	}

	now := time.Now().UTC()
	for index := 0; index < 3; index++ {
		if err := store.RecordPageView(ctx, "ko", "c", now); err != nil {
			t.Fatalf("record C view: %v", err)
		}
	}
	if err := store.RecordPageView(ctx, "ko", "rust", now); err != nil {
		t.Fatalf("record Rust view: %v", err)
	}
	for index := 0; index < 4; index++ {
		if err := store.RecordPageView(ctx, "ko", "rust", now.AddDate(0, 0, -8)); err != nil {
			t.Fatalf("record old Rust view: %v", err)
		}
	}
	for index := 0; index < 10; index++ {
		if err := store.RecordPageView(ctx, "en", "c", now); err != nil {
			t.Fatalf("record English C view: %v", err)
		}
	}

	popular, err := store.ListPopular(ctx, "ko", now.AddDate(0, 0, -6), 10)
	if err != nil {
		t.Fatalf("list popular: %v", err)
	}
	if len(popular) != 2 {
		t.Fatalf("expected two Korean documents, got %#v", popular)
	}
	if popular[0].Document.Slug != "c" || popular[0].Views != 3 {
		t.Fatalf("expected C first with three views, got %#v", popular[0])
	}
	if popular[1].Document.Slug != "rust" || popular[1].Views != 1 {
		t.Fatalf("expected only recent Rust view, got %#v", popular[1])
	}

	var metricID string
	err = store.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(pageMetricIdentityKey("ko", "c")))
		if err != nil {
			return err
		}
		return item.Value(func(value []byte) error {
			metricID = string(value)
			return nil
		})
	})
	if err != nil {
		t.Fatalf("read metric identity: %v", err)
	}
	if metricID == "" || metricID == "c" {
		t.Fatalf("metric identity must be opaque, got %q", metricID)
	}
}

func TestSaveRejectsInvalidLocaleInsteadOfFallingBack(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	if _, err := store.Save(context.Background(), document.Document{Locale: "not_a_language", Slug: "bad", Title: "Bad"}); !errors.Is(err, ErrInvalidLocale) {
		t.Fatalf("expected invalid locale error, got %v", err)
	}
}

func TestSearchKeepsLocalesSeparate(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open badger store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if _, err := store.Save(ctx, document.Document{
		Locale: "ko",
		Group:  "operating-systems",
		Slug:   "operating-systems",
		Title:  "운영체제",
		Source: "# 운영체제\n\n운영체제는 시스템 소프트웨어다.",
	}); err != nil {
		t.Fatalf("save ko: %v", err)
	}
	if _, err := store.Save(ctx, document.Document{
		Locale: "en",
		Group:  "operating-systems",
		Slug:   "operating-systems",
		Title:  "Operating systems",
		Source: "# Operating systems\n\nOperating systems manage computers.",
	}); err != nil {
		t.Fatalf("save en: %v", err)
	}

	koResults, err := store.Search(ctx, "ko", "운영체제", 8)
	if err != nil {
		t.Fatalf("search ko: %v", err)
	}
	if len(koResults) != 1 || koResults[0].Locale != "ko" {
		t.Fatalf("expected only ko result, got %#v", koResults)
	}

	enResults, err := store.Search(ctx, "en", "Operating", 8)
	if err != nil {
		t.Fatalf("search en: %v", err)
	}
	if len(enResults) != 1 || enResults[0].Locale != "en" {
		t.Fatalf("expected only en result, got %#v", enResults)
	}

	translations, err := store.GetTranslations(ctx, "operating-systems")
	if err != nil {
		t.Fatalf("translations: %v", err)
	}
	if len(translations) != 2 {
		t.Fatalf("expected two translations, got %#v", translations)
	}
}

func TestRevisionsUsePerDocumentSequence(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open badger store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	doc := document.Document{
		Locale:  "ko",
		Slug:    "c",
		Title:   "C",
		Source:  "# C\n\n첫 문서",
		Author:  "editor",
		Summary: "문서 생성",
	}
	first, err := store.Save(ctx, doc)
	if err != nil {
		t.Fatalf("save first revision: %v", err)
	}
	doc.Source = "# C\n\n두 번째 문서"
	doc.Summary = "설명 보강"
	second, err := store.Save(ctx, doc)
	if err != nil {
		t.Fatalf("save second revision: %v", err)
	}
	if first.Revision != "r1" || second.Revision != "r2" {
		t.Fatalf("expected r1 and r2, got %q and %q", first.Revision, second.Revision)
	}
	revision, err := store.GetRevision(ctx, "ko", "c", "r2")
	if err != nil {
		t.Fatalf("get r2: %v", err)
	}
	if revision.Summary != "설명 보강" || revision.Author != "editor" {
		t.Fatalf("revision metadata was not preserved: %#v", revision)
	}
}
