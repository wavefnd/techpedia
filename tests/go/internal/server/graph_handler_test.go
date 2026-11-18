package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"techpedia/internal/config"
	"techpedia/internal/document"
	"techpedia/internal/edition"
	"techpedia/internal/storage"
	"techpedia/internal/techdata"
	"techpedia/internal/techmark"
)

type graphScaleStore struct {
	emptyStore
	pages []document.LinkDocument
}

func (s graphScaleStore) ListEditions(context.Context) ([]edition.Edition, error) {
	return []edition.Edition{{Tag: "en", Status: edition.StatusActive, MainPageName: "Main Page"}}, nil
}

func (s graphScaleStore) ListLinkDocuments(context.Context, string) ([]document.LinkDocument, error) {
	return s.pages, nil
}

func graphScaleDocuments(count int) []document.LinkDocument {
	pages := make([]document.LinkDocument, count)
	for i := range pages {
		name := fmt.Sprintf("Page %06d", i)
		pages[i] = document.LinkDocument{
			Document: document.Document{ID: document.WID(fmt.Sprintf("W%d", i+1)), Locale: "en", Title: name, Slug: techmark.Slugify(name)},
			Targets:  []string{fmt.Sprintf("Page %06d", (i+1)%count), fmt.Sprintf("Page %06d", (i+7)%count)},
		}
	}
	return pages
}

func TestGraphDefaultIncludesEveryDocumentBeyondLegacyLimits(t *testing.T) {
	pages := graphScaleDocuments(10001)
	srv := New(config.Config{}, graphScaleStore{pages: pages}, emptyAuth{}, techmark.NewParser())
	for _, suffix := range []string{"", "?limit=all", "?limit=1500", "?limit=5000"} {
		recorder := httptest.NewRecorder()
		srv.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/graph"+suffix, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: HTTP %d: %s", suffix, recorder.Code, recorder.Body.String())
		}
		var graph graphResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &graph); err != nil {
			t.Fatal(err)
		}
		want := len(pages)
		if suffix == "?limit=1500" {
			want = 1500
		} else if suffix == "?limit=5000" {
			want = 5000
		}
		if len(graph.Nodes) != want || graph.Stats.Visible != want || graph.Stats.Documents != len(pages) || graph.Stats.Truncated != (want < len(pages)) {
			t.Fatalf("%s: unexpected counts: %+v / %d", suffix, graph.Stats, len(graph.Nodes))
		}
		if want == len(pages) && len(graph.Edges) != 2*len(pages) {
			t.Fatalf("full graph lost edges: %d", len(graph.Edges))
		}
	}
}

func TestGraphSnapshotDeduplicatesTargetsAndDocumentsAndObservesCancellation(t *testing.T) {
	pages := graphScaleDocuments(3)
	pages[0].Targets = []string{pages[0].Document.Title, pages[1].Document.Title, strings.ReplaceAll(pages[1].Document.Title, " ", "_"), "Missing", "Missing"}
	pages = append(pages, pages[0])
	graph, err := graphSnapshot(context.Background(), pages, nil, "en", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 3 || len(graph.Edges) != 3 || graph.Stats.Missing != 1 {
		t.Fatalf("deduplication failed: %+v / %d edges", graph.Stats, len(graph.Edges))
	}
	for _, edge := range graph.Edges {
		if edge.Source == edge.Target {
			t.Fatal("self edge retained")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := graphSnapshot(ctx, pages, nil, "en", 0); err != context.Canceled {
		t.Fatalf("cancelled graph build returned %v", err)
	}
}

func BenchmarkGraphSnapshot(b *testing.B) {
	for _, count := range []int{2000, 10000, 50000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			pages := graphScaleDocuments(count)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				graph, err := graphSnapshot(context.Background(), pages, nil, "en", 0)
				if err != nil || len(graph.Nodes) != count {
					b.Fatal("incomplete graph", err)
				}
			}
		})
	}
}

func TestGraphCacheHasGlobalBudgetAndRejectsInvalidatedBuilds(t *testing.T) {
	srv := New(config.Config{}, emptyStore{}, emptyAuth{}, techmark.NewParser())
	entry := graphCacheEntry{body: make([]byte, 2<<20), expiresAt: time.Now().Add(time.Hour)}
	for i := range 30 {
		srv.cacheGraph(fmt.Sprint(i), entry, srv.graphGeneration)
		bytes := 0
		for _, value := range srv.graph {
			bytes += len(value.body)
		}
		if bytes > graphCacheMaxBytes || len(srv.graph) > graphCacheEntries {
			t.Fatal("graph cache budget exceeded")
		}
	}
	generation := srv.graphGeneration
	srv.invalidateGraphCache()
	srv.cacheGraph("old", entry, generation)
	if _, ok := srv.cachedGraph("old"); ok {
		t.Fatal("invalidated build repopulated graph cache")
	}
	srv.cacheGraph("new", entry, srv.graphGeneration)
	if _, ok := srv.cachedGraph("new"); !ok {
		t.Fatal("new graph build not cached")
	}
	srv.cacheGraph("oversize", graphCacheEntry{body: make([]byte, graphCacheMaxBytes+1), expiresAt: entry.expiresAt}, srv.graphGeneration)
	if _, ok := srv.cachedGraph("oversize"); ok {
		t.Fatal("oversized graph retained")
	}
}

func TestGraphAPIUsesLinksAndTechdataWithoutExposingStableIDs(t *testing.T) {
	store, err := storage.OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open document store: %v", err)
	}
	defer store.Close()
	configureTestEdition(t, store, "ko", "대문")
	configureTestEdition(t, store, "en", "Main Page")

	ctx := context.Background()
	koA, err := store.Save(ctx, document.Document{Locale: "ko", Slug: techmark.Slugify("운영체제"), Title: "운영체제", Source: "# 운영체제\n\n[[커널]]"})
	if err != nil {
		t.Fatalf("save Korean source document: %v", err)
	}
	if _, err := store.Save(ctx, document.Document{Locale: "ko", Slug: techmark.Slugify("커널"), Title: "커널", Source: "# 커널"}); err != nil {
		t.Fatalf("save Korean target document: %v", err)
	}
	enA, err := store.Save(ctx, document.Document{Locale: "en", Slug: techmark.Slugify("Operating System"), Title: "Operating System", Source: "# Operating System"})
	if err != nil {
		t.Fatalf("save English document: %v", err)
	}
	if _, err := store.CreateItem(ctx, techdata.Item{
		Labels:    map[string]string{"ko": "운영체제", "en": "Operating System"},
		WikiPages: map[string]document.WID{"ko": koA.ID, "en": enA.ID},
	}); err != nil {
		t.Fatalf("create Techdata item: %v", err)
	}

	srv := New(config.Config{}, store, emptyAuth{}, techmark.NewParser())
	languageRequest := httptest.NewRequest(http.MethodGet, "/api/graph?locale=ko", nil)
	languageRecorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(languageRecorder, languageRequest)
	if languageRecorder.Code != http.StatusOK {
		t.Fatalf("language graph status %d: %s", languageRecorder.Code, languageRecorder.Body.String())
	}
	var languageGraph graphResponse
	if err := json.Unmarshal(languageRecorder.Body.Bytes(), &languageGraph); err != nil {
		t.Fatalf("decode language graph: %v", err)
	}
	if languageGraph.Scope != "language" || languageGraph.Locale != "ko" || len(languageGraph.Nodes) != 2 || len(languageGraph.Edges) != 1 || languageGraph.Edges[0].Kind != "link" {
		t.Fatalf("unexpected language graph: %#v", languageGraph)
	}
	if body := languageRecorder.Body.String(); strings.Contains(body, string(koA.ID)) || strings.Contains(body, `"tid"`) || strings.Contains(body, `"wid"`) {
		t.Fatalf("public graph exposed a stable identifier: %s", body)
	}

	compactRequest := httptest.NewRequest(http.MethodGet, "/api/graph?locale=ko&limit=600", nil)
	compactRecorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(compactRecorder, compactRequest)
	if compactRecorder.Code != http.StatusOK {
		t.Fatalf("compact graph status = %d: %s", compactRecorder.Code, compactRecorder.Body.String())
	}
	invalidRecorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(invalidRecorder, httptest.NewRequest(http.MethodGet, "/api/graph?locale=ko&limit=42", nil))
	if invalidRecorder.Code != http.StatusBadRequest {
		t.Fatalf("unsupported graph limit status = %d", invalidRecorder.Code)
	}

	cachedRequest := httptest.NewRequest(http.MethodGet, "/api/graph?locale=ko", nil)
	cachedRequest.Header.Set("If-None-Match", languageRecorder.Header().Get("ETag"))
	cachedRecorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(cachedRecorder, cachedRequest)
	if cachedRecorder.Code != http.StatusNotModified {
		t.Fatalf("expected conditional graph response, got %d: %s", cachedRecorder.Code, cachedRecorder.Body.String())
	}

	allRequest := httptest.NewRequest(http.MethodGet, "/api/graph", nil)
	allRecorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(allRecorder, allRequest)
	if allRecorder.Code != http.StatusOK {
		t.Fatalf("all-language graph status %d: %s", allRecorder.Code, allRecorder.Body.String())
	}
	var allGraph graphResponse
	if err := json.Unmarshal(allRecorder.Body.Bytes(), &allGraph); err != nil {
		t.Fatalf("decode all-language graph: %v", err)
	}
	translationEdges := 0
	for _, edge := range allGraph.Edges {
		if edge.Kind == "translation" {
			translationEdges++
		}
	}
	if allGraph.Scope != "all" || len(allGraph.Nodes) != 3 || translationEdges != 1 {
		t.Fatalf("unexpected all-language graph: %#v", allGraph)
	}
}

func TestLocalizedGraphRouteServesFrontend(t *testing.T) {
	store, err := storage.OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open document store: %v", err)
	}
	defer store.Close()
	configureTestEdition(t, store, "ko", "대문")
	srv := New(config.Config{}, store, emptyAuth{}, techmark.NewParser())
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ko/graph", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `<div id="app">`) {
		t.Fatalf("localized graph route did not serve frontend: %d: %s", recorder.Code, recorder.Body.String())
	}
}
