package storage

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	badger "github.com/dgraph-io/badger/v4"

	"techpedia/internal/bot"
	"techpedia/internal/document"
)

func TestPopularDocumentsReadOnlySelectedArticleXML(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	a := saveIndexFixture(t, store, "en", "Selected", "full selected source")
	b := saveIndexFixture(t, store, "en", "Unselected", "unselected source")
	for i := 0; i < 2; i++ {
		if err := store.RecordPageView(ctx, a.Locale, a.Slug, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.RecordPageView(ctx, b.Locale, b.Slug, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := store.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(latestDocumentKey(b.Locale, b.Slug)), []byte("broken unselected XML"))
	}); err != nil {
		t.Fatal(err)
	}
	values, err := store.ListPopular(ctx, "en", time.Now().UTC(), 1)
	if err != nil || len(values) != 1 || values[0].Document.Source != a.Source || values[0].Views != 2 {
		t.Fatalf("selected source and ranking must be preserved without decoding unselected XML: %#v, %v", values, err)
	}
}

func saveIndexFixture(t *testing.T, store DocumentStore, locale, title, source string) document.Document {
	t.Helper()
	doc, err := store.Save(context.Background(), document.Document{Locale: locale, Slug: strings.ToLower(strings.ReplaceAll(title, " ", "-")), Title: title, Source: source})
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestReadIndexesDoNotDecodeArticleXML(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	var current document.Document
	for i := range 30 {
		current = saveIndexFixture(t, store, "en", "Alpha", fmt.Sprintf("[[Beta]] revision %d", i))
	}
	saveIndexFixture(t, store, "en", "Beta", "[[Beta]]")
	// Corrupt the fixture's raw XML after indexing. Read-only derived queries
	// must not even inspect it; real source/history reads must still fail.
	if err := store.db.Update(func(txn *badger.Txn) error {
		if err := txn.Set([]byte(latestDocumentKey("en", "alpha")), []byte("broken XML")); err != nil {
			return err
		}
		return txn.Set([]byte(revisionDocumentKey("en", "alpha", current.Revision)), []byte("broken XML"))
	}); err != nil {
		t.Fatal(err)
	}
	revisions, err := store.ListRevisions(ctx, "en", "alpha", 8)
	if err != nil || len(revisions) != 8 || revisions[0].Revision != "r30" {
		t.Fatalf("bounded history: %#v, %v", revisions, err)
	}
	if _, err := store.GetRevision(ctx, "en", "alpha", "r30"); err == nil {
		t.Fatal("source read unexpectedly used a summary")
	}
	backlinks, err := store.ListBacklinks(ctx, "en", "beta")
	if err != nil || len(backlinks) != 1 || backlinks[0].Title != "Alpha" {
		t.Fatalf("backlinks: %#v, %v", backlinks, err)
	}
	pages, err := store.ListLinkDocuments(ctx, "en")
	if err != nil || len(pages) != 2 {
		t.Fatalf("graph metadata: %#v, %v", pages, err)
	}
	for _, page := range pages {
		if page.Document.Source != "" || page.Document.HTML != "" || page.Document.Author != "" {
			t.Fatal("article body retained in link index")
		}
	}
	results, err := store.Search(ctx, "en", "revision", 8)
	if err != nil || len(results) != 1 {
		t.Fatalf("persisted search: %#v, %v", results, err)
	}
}

func TestReadIndexesFollowSaveMoveDeleteAndRestore(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	a := saveIndexFixture(t, store, "en", "Alpha", "[[Beta]] oldtoken")
	saveIndexFixture(t, store, "en", "Beta", "target")
	if _, err := store.Search(ctx, "en", "oldtoken", 8); err != nil {
		t.Fatal(err)
	}
	a.Source = "[[Gamma]] newtoken"
	a, err = store.SaveIfRevision(ctx, a, a.Revision)
	if err != nil {
		t.Fatal(err)
	}
	links, err := store.ListBacklinks(ctx, "en", "beta")
	if err != nil || len(links) != 0 {
		t.Fatalf("stale reverse links: %#v, %v", links, err)
	}
	moved, err := store.MoveDocument(ctx, "en", "alpha", "delta", "Delta")
	if err != nil || moved.ID != a.ID {
		t.Fatalf("move: %#v, %v", moved, err)
	}
	history, err := store.ListRevisions(ctx, "en", "delta", 100)
	if err != nil || len(history) != 2 || history[0].Title != "Delta" {
		t.Fatalf("moved history: %#v, %v", history, err)
	}
	old, err := store.ListRevisions(ctx, "en", "alpha", 100)
	if err != nil || len(old) != 0 {
		t.Fatalf("old history: %#v, %v", old, err)
	}
	links, err = store.ListBacklinks(ctx, "en", "gamma")
	if err != nil || len(links) != 1 || links[0].Slug != "delta" {
		t.Fatalf("moved reverse links: %#v, %v", links, err)
	}
	if _, err := store.DeleteDocument(ctx, "en", "delta", "owner", "test"); err != nil {
		t.Fatal(err)
	}
	links, err = store.ListBacklinks(ctx, "en", "gamma")
	if err != nil || len(links) != 0 {
		t.Fatalf("deleted links: %#v, %v", links, err)
	}
	results, err := store.Search(ctx, "en", "newtoken", 8)
	if err != nil || len(results) != 0 {
		t.Fatalf("deleted search entry: %#v, %v", results, err)
	}
	if _, err := store.RestoreDeletedDocument(ctx, a.ID, "owner", "test"); err != nil {
		t.Fatal(err)
	}
	links, err = store.ListBacklinks(ctx, "en", "gamma")
	if err != nil || len(links) != 1 {
		t.Fatalf("restored links: %#v, %v", links, err)
	}
	results, err = store.Search(ctx, "en", "newtoken", 8)
	if err != nil || len(results) != 1 || results[0].Title != "Delta" {
		t.Fatalf("restored search entry: %#v, %v", results, err)
	}
	if _, err := store.RestoreRevision(ctx, "en", "delta", "r1", moved.Revision, "owner", "test"); err != nil {
		t.Fatal(err)
	}
	links, err = store.ListBacklinks(ctx, "en", "beta")
	if err != nil || len(links) != 1 {
		t.Fatalf("restored historical links: %#v, %v", links, err)
	}
	results, err = store.Search(ctx, "en", "newtoken", 8)
	if err != nil || len(results) != 0 {
		t.Fatalf("restored source retained new search: %#v, %v", results, err)
	}
}

func TestReadIndexMigrationBackfillsAndRunsOnce(t *testing.T) {
	path := t.TempDir()
	store, err := OpenBadgerDocumentStore(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	saveIndexFixture(t, store, "en", "Alpha", "[[Beta]] uniquequery")
	saveIndexFixture(t, store, "en", "Beta", "target")
	if err := store.db.Update(func(txn *badger.Txn) error {
		for _, prefix := range []string{"wiki:v1:history:", "wiki:v1:recent-revision:", "wiki:v1:links:", "wiki:v1:backlink:", "search:v1:document:", "migration:v1:" + revisionSummaryMigration + ":", "migration:v1:" + readIndexesMigration + ":"} {
			if _, err := deleteKeysByPrefixCount(txn, prefix); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenBadgerDocumentStore(path)
	if err != nil {
		t.Fatal(err)
	}
	history, err := store.ListRevisions(ctx, "en", "alpha", 8)
	if err != nil || len(history) != 1 {
		t.Fatalf("history migration: %#v, %v", history, err)
	}
	links, err := store.ListBacklinks(ctx, "en", "beta")
	if err != nil || len(links) != 1 {
		t.Fatalf("link migration: %#v, %v", links, err)
	}
	results, err := store.Search(ctx, "en", "uniquequery", 8)
	if err != nil || len(results) != 1 {
		t.Fatalf("search migration: %#v, %v", results, err)
	}
	changes, err := store.ListChanges(ctx, document.ChangeFilter{Locale: "en", Limit: 100})
	if err != nil || len(changes) != 2 {
		t.Fatalf("duplicate migration events: %#v, %v", changes, err)
	}
	// A completed migration must not rescan revision/latest XML on every open.
	if err := store.db.Update(func(txn *badger.Txn) error {
		if err := txn.Set([]byte(revisionDocumentKey("en", "alpha", "r1")), []byte("invalid")); err != nil {
			return err
		}
		return txn.Set([]byte(latestDocumentKey("en", "alpha")), []byte("invalid"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenBadgerDocumentStore(path)
	if err != nil {
		t.Fatalf("completed migration rescanned raw XML: %v", err)
	}
	defer store.Close()
}

func TestDocumentMigrationResumesAfterCommittedCursor(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, title := range []string{"Alpha", "Beta", "Gamma"} {
		saveIndexFixture(t, store, "en", title, "text")
	}
	counts := map[string]int{}
	failure := errors.New("interrupted migration")
	apply := func(txn *badger.Txn, doc document.Document, _ string) error {
		counts[doc.Slug]++
		if doc.Slug == "beta" && counts[doc.Slug] == 1 {
			return failure
		}
		return txn.Set([]byte("test:migrated:"+doc.Slug), []byte{1})
	}
	if err := store.migrateDocuments("test-resume", []string{latestDocumentPrefix}, apply); !errors.Is(err, failure) {
		t.Fatalf("expected interruption, got %v", err)
	}
	if err := store.migrateDocuments("test-resume", []string{latestDocumentPrefix}, apply); err != nil {
		t.Fatal(err)
	}
	if err := store.migrateDocuments("test-resume", []string{latestDocumentPrefix}, apply); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(counts, map[string]int{"alpha": 1, "beta": 2, "gamma": 1}) {
		t.Fatalf("committed records rescanned: %#v", counts)
	}
}

func TestStoredLinksMatchSourceAnalysisAcrossLocales(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	for _, locale := range []string{"ko", "en", "ar"} {
		saveIndexFixture(t, store, locale, "Alpha", "[[Beta]] [[Beta|same]] [[Alpha]] [[Missing]]\n```\n[[Ignored]]\n```")
		saveIndexFixture(t, store, locale, "Beta", "[[Alpha]]")
		original, err := store.ListLatest(ctx, locale, 0)
		if err != nil {
			t.Fatal(err)
		}
		derived, err := store.ListLinkDocuments(ctx, locale)
		if err != nil {
			t.Fatal(err)
		}
		// Normalize only the fields the index deliberately does not retain.
		want := bot.AnalyzeLinks(original)
		for i := range derived {
			for _, doc := range original {
				if derived[i].Document.ID == doc.ID {
					derived[i].Document = doc
				}
			}
		}
		got := bot.AnalyzeLinkDocuments(derived)
		// Equal PageRank cycles can differ in floating-point tie order; compare
		// the graph, not that unspecified order or map-driven edge insertion.
		for _, analysis := range []*bot.LinkAnalysis{&want, &got} {
			sort.Slice(analysis.Documents, func(i, j int) bool { return analysis.Documents[i].Slug < analysis.Documents[j].Slug })
			sort.Slice(analysis.Edges, func(i, j int) bool { return analysis.Edges[i].Source.Slug < analysis.Edges[j].Source.Slug })
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s derived graph differs:\n%#v\n%#v", locale, got, want)
		}
		links, err := store.ListBacklinks(ctx, locale, "beta")
		if err != nil || len(links) != len(want.BacklinksFor(locale, "beta")) {
			t.Fatalf("%s reverse index: %#v, %v", locale, links, err)
		}
	}
}
