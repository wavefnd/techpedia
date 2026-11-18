package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"techpedia/internal/document"
	"techpedia/internal/techdata"
	"techpedia/internal/techmark"
)

const (
	graphCacheTTL      = 30 * time.Second
	graphCompactNodes  = 600
	graphBalancedNodes = 1500
	graphMaxNodes      = 5000
	graphCacheMaxBytes = 16 << 20
	graphCacheEntries  = 12
)

type graphCacheEntry struct {
	body      []byte
	etag      string
	expiresAt time.Time
}

type graphNodeResponse struct {
	ID     string `json:"id"`
	Locale string `json:"locale"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	Degree int    `json:"degree"`
}

type graphEdgeResponse struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
}

type graphStatsResponse struct {
	Documents int  `json:"documents"`
	Visible   int  `json:"visible"`
	Links     int  `json:"links"`
	Missing   int  `json:"missing"`
	Truncated bool `json:"truncated"`
}

type graphResponse struct {
	Scope  string              `json:"scope"`
	Locale string              `json:"locale,omitempty"`
	Nodes  []graphNodeResponse `json:"nodes"`
	Edges  []graphEdgeResponse `json:"edges"`
	Stats  graphStatsResponse  `json:"stats"`
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	locale := strings.TrimSpace(r.URL.Query().Get("locale"))
	if locale != "" {
		canonical, ok := document.CanonicalLocale(locale)
		if !ok {
			http.Error(w, "invalid language tag", http.StatusBadRequest)
			return
		}
		locale = canonical
		if !s.requireActiveEdition(w, r, locale) {
			return
		}
	}
	limit, ok := graphNodeLimit(r.URL.Query().Get("limit"))
	if !ok {
		http.Error(w, "invalid graph node limit", http.StatusBadRequest)
		return
	}

	scopeKey := locale
	if scopeKey == "" {
		scopeKey = "all"
	}
	cacheKey := scopeKey + ":" + strconv.Itoa(limit)
	if cached, ok := s.cachedGraph(cacheKey); ok {
		writeGraphResponse(w, r, cached)
		return
	}
	s.graphBuildMu.Lock()
	defer s.graphBuildMu.Unlock()
	if r.Context().Err() != nil {
		return
	}
	if cached, ok := s.cachedGraph(cacheKey); ok {
		writeGraphResponse(w, r, cached)
		return
	}
	s.graphMu.Lock()
	generation := s.graphGeneration
	s.graphMu.Unlock()

	payload, err := s.buildGraph(r.Context(), locale, limit)
	if err != nil {
		http.Error(w, "failed to build graph", http.StatusInternalServerError)
		return
	}
	body, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "failed to encode graph", http.StatusInternalServerError)
		return
	}
	digest := sha256.Sum256(body)
	entry := graphCacheEntry{
		body:      body,
		etag:      `"` + hex.EncodeToString(digest[:12]) + `"`,
		expiresAt: time.Now().Add(graphCacheTTL),
	}
	s.cacheGraph(cacheKey, entry, generation)
	writeGraphResponse(w, r, entry)
}

func graphNodeLimit(value string) (int, bool) {
	if value == "" || value == "all" {
		return 0, true
	}
	switch value {
	case strconv.Itoa(graphMaxNodes):
		return graphMaxNodes, true
	case strconv.Itoa(graphCompactNodes):
		return graphCompactNodes, true
	case strconv.Itoa(graphBalancedNodes):
		return graphBalancedNodes, true
	default:
		return 0, false
	}
}

func (s *Server) cachedGraph(key string) (graphCacheEntry, bool) {
	s.graphMu.Lock()
	defer s.graphMu.Unlock()
	entry, ok := s.graph[key]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(s.graph, key)
		return graphCacheEntry{}, false
	}
	return entry, true
}

func (s *Server) invalidateGraphCache() {
	s.graphMu.Lock()
	defer s.graphMu.Unlock()
	clear(s.graph)
	s.graphGeneration++
}

func (s *Server) cacheGraph(key string, entry graphCacheEntry, generation uint64) {
	s.graphMu.Lock()
	defer s.graphMu.Unlock()
	if generation != s.graphGeneration || len(entry.body) > graphCacheMaxBytes {
		return
	}
	bytes := 0
	now := time.Now()
	for name, cached := range s.graph {
		if now.After(cached.expiresAt) || name == key {
			delete(s.graph, name)
		} else {
			bytes += len(cached.body)
		}
	}
	for len(s.graph) >= graphCacheEntries || bytes+len(entry.body) > graphCacheMaxBytes {
		oldest := ""
		for name, cached := range s.graph {
			if oldest == "" || cached.expiresAt.Before(s.graph[oldest].expiresAt) {
				oldest = name
			}
		}
		if oldest == "" {
			break
		}
		bytes -= len(s.graph[oldest].body)
		delete(s.graph, oldest)
	}
	s.graph[key] = entry
}

func writeGraphResponse(w http.ResponseWriter, r *http.Request, entry graphCacheEntry) {
	w.Header().Set("Cache-Control", "public, max-age=30, stale-while-revalidate=30")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("ETag", entry.etag)
	if r.Header.Get("If-None-Match") == entry.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(entry.body)
}

func (s *Server) buildGraph(ctx context.Context, locale string, limit int) (graphResponse, error) {
	editions, err := s.store.ListEditions(ctx)
	if err != nil {
		return graphResponse{}, err
	}
	documents := make([]document.LinkDocument, 0)
	for _, value := range editions {
		if !value.Active() || (locale != "" && value.Tag != locale) {
			continue
		}
		current, err := s.store.ListLinkDocuments(ctx, value.Tag)
		if err != nil {
			return graphResponse{}, err
		}
		documents = append(documents, current...)
	}

	var items []techdata.Item
	if locale == "" {
		items, err = s.store.ListItems(ctx)
		if err != nil {
			return graphResponse{}, err
		}
	}
	return graphSnapshot(ctx, documents, items, locale, limit)
}

// Public visualization needs adjacency, not analyzer priority. In particular it
// must not allocate the dense N-by-N PageRank matrix used by the bot crawler.
func graphSnapshot(ctx context.Context, documents []document.LinkDocument, items []techdata.Item, locale string, limit int) (graphResponse, error) {
	byDocument := make(map[string]int, len(documents))
	unique := make([]document.LinkDocument, 0, len(documents))
	for _, raw := range documents {
		if err := ctx.Err(); err != nil {
			return graphResponse{}, err
		}
		raw.Document = document.NormalizeDocument(raw.Document)
		key := graphDocumentKey(raw.Document)
		if _, exists := byDocument[key]; !exists {
			byDocument[key] = len(unique)
			unique = append(unique, raw)
		}
	}
	sort.Slice(unique, func(i, j int) bool {
		left, right := unique[i].Document, unique[j].Document
		if left.Locale != right.Locale {
			return left.Locale < right.Locale
		}
		return left.Slug < right.Slug
	})
	visible := unique
	truncated := limit > 0 && len(visible) > limit
	if truncated {
		visible = visible[:limit]
	}
	nodes := make([]graphNodeResponse, len(visible))
	nodeByDocument := make(map[string]string, len(visible))
	nodeByWID := make(map[document.WID]string, len(visible))
	for index, entry := range visible {
		doc := entry.Document
		id := strconv.FormatInt(int64(index), 36)
		nodes[index] = graphNodeResponse{ID: id, Locale: doc.Locale, Name: doc.Title, URL: wikiURL(doc.Locale, doc.Title)}
		nodeByDocument[graphDocumentKey(doc)] = id
		if doc.ID != "" {
			nodeByWID[doc.ID] = id
		}
	}

	edges := make([]graphEdgeResponse, 0)
	addEdge := func(source string, target string, kind string) {
		if source == "" || target == "" || source == target {
			return
		}
		edges = append(edges, graphEdgeResponse{Source: source, Target: target, Kind: kind})
	}
	missing := 0
	for _, entry := range unique {
		if err := ctx.Err(); err != nil {
			return graphResponse{}, err
		}
		source := nodeByDocument[graphDocumentKey(entry.Document)]
		seen := make(map[string]struct{}, len(entry.Targets))
		for index, target := range entry.Targets {
			if index%256 == 0 && ctx.Err() != nil {
				return graphResponse{}, ctx.Err()
			}
			key := entry.Document.Locale + "\x00" + techmark.Slugify(strings.TrimSpace(strings.ReplaceAll(target, "_", " ")))
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			if _, exists := byDocument[key]; !exists {
				missing++
				continue
			}
			addEdge(source, nodeByDocument[key], "link")
		}
	}
	addTranslationEdges(items, nodeByWID, addEdge)
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Source != edges[j].Source {
			return edges[i].Source < edges[j].Source
		}
		if edges[i].Target != edges[j].Target {
			return edges[i].Target < edges[j].Target
		}
		return edges[i].Kind < edges[j].Kind
	})
	for _, edge := range edges {
		source, sourceOK := graphNodeIndex(edge.Source, nodes)
		target, targetOK := graphNodeIndex(edge.Target, nodes)
		if sourceOK {
			nodes[source].Degree++
		}
		if targetOK {
			nodes[target].Degree++
		}
	}
	scope := "language"
	if locale == "" {
		scope = "all"
	}
	return graphResponse{
		Scope: scope, Locale: locale, Nodes: nodes, Edges: edges,
		Stats: graphStatsResponse{
			Documents: len(unique), Visible: len(nodes), Links: len(edges),
			Missing: missing, Truncated: truncated,
		},
	}, nil
}

func addTranslationEdges(items []techdata.Item, nodeByWID map[document.WID]string, add func(string, string, string)) {
	for _, item := range items {
		locales := make([]string, 0, len(item.WikiPages))
		for locale := range item.WikiPages {
			locales = append(locales, locale)
		}
		sort.Strings(locales)
		root := ""
		for _, locale := range locales {
			id := nodeByWID[item.WikiPages[locale]]
			if id == "" {
				continue
			}
			if root == "" {
				root = id
				continue
			}
			add(root, id, "translation")
		}
	}
}

func graphDocumentKey(doc document.Document) string {
	return document.NormalizeLocale(doc.Locale) + "\x00" + strings.TrimSpace(doc.Slug)
}

func graphNodeIndex(id string, nodes []graphNodeResponse) (int, bool) {
	value, err := strconv.ParseInt(id, 36, 32)
	if err != nil || value < 0 || int(value) >= len(nodes) {
		return 0, false
	}
	return int(value), true
}
