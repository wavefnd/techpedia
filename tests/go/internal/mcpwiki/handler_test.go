package mcpwiki

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"techpedia/internal/document"
	"techpedia/internal/edition"
	"techpedia/internal/storage"
	"techpedia/internal/techmark"
	"techpedia/internal/wiki"
)

type testStore struct {
	doc     document.Document
	err     error
	block   chan struct{}
	started chan struct{}
}

func (s *testStore) GetEdition(ctx context.Context, tag string) (edition.Edition, error) {
	if _, ok := ctx.Deadline(); !ok {
		return edition.Edition{}, errors.New("missing tool deadline")
	}
	if tag == "en" || tag == "ko" || tag == "fr" {
		return edition.Edition{Tag: tag, Status: edition.StatusActive, MainPageName: "Main Page"}, nil
	}
	return edition.Edition{}, edition.ErrNotFound
}
func (s *testStore) ListEditions(ctx context.Context) ([]edition.Edition, error) {
	if s.block != nil {
		s.started <- struct{}{}
		select {
		case <-s.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return []edition.Edition{{Tag: "en", Status: edition.StatusActive, MainPageName: "Main Page", RequestedBy: "private-user"}, {Tag: "ar", Status: edition.StatusPending}}, s.err
}
func (s *testStore) GetLatest(_ context.Context, locale, slug string) (document.Document, error) {
	if s.err != nil {
		return document.Document{}, s.err
	}
	if locale == s.doc.Locale && slug == s.doc.Slug {
		return s.doc, nil
	}
	return document.Document{}, storage.ErrDocumentNotFound
}
func (s *testStore) GetDocumentRedirect(_ context.Context, locale, slug string) (string, error) {
	if locale == s.doc.Locale && slug == techmark.Slugify("BSD") {
		return s.doc.Slug, nil
	}
	return "", storage.ErrDocumentNotFound
}
func (s *testStore) Search(_ context.Context, _, _ string, _ int) ([]document.SearchResult, error) {
	return []document.SearchResult{document.NewSearchResult(s.doc, "snippet", 999)}, s.err
}

func testHandler(t *testing.T, store *testStore) http.Handler {
	t.Helper()
	h, err := NewHandler(wiki.NewReader(store), "https://wiki.example")
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func rpc(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "https://wiki.example/mcp", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("MCP-Protocol-Version", "2025-11-25")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func toolCall(t *testing.T, handler http.Handler, name string, args any) map[string]json.RawMessage {
	t.Helper()
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
	if err != nil {
		t.Fatal(err)
	}
	w := rpc(t, handler, string(body))
	var envelope struct {
		Result map[string]json.RawMessage `json:"result"`
		Error  json.RawMessage            `json:"error"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &envelope) != nil || envelope.Error != nil {
		t.Fatalf("RPC failed: %d %s", w.Code, w.Body.String())
	}
	return envelope.Result
}

func TestInitializationAndReadOnlySchemas(t *testing.T) {
	h := testHandler(t, &testStore{})
	w := rpc(t, h, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"protocolVersion":"2025-11-25"`) || w.Header().Get("Mcp-Session-Id") != "" {
		t.Fatalf("initialize: %d %s", w.Code, w.Body.String())
	}
	w = rpc(t, h, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	var result struct {
		Result struct {
			Tools []mcp.Tool `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Result.Tools) != 6 {
		t.Fatalf("tool count: %s", w.Body.String())
	}
	for _, tool := range result.Result.Tools {
		if tool.InputSchema == nil || tool.OutputSchema == nil || tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint || tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
			t.Fatalf("bad read-only metadata: %+v", tool)
		}
	}
	w = rpc(t, h, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"save_page","arguments":{"confirmed":true}}}`)
	if !strings.Contains(w.Body.String(), `"error"`) || !strings.Contains(w.Body.String(), `unknown tool`) {
		t.Fatalf("unimplemented write allowed: %s", w.Body.String())
	}
}

func TestPublicOutputsAndChunkedUnicodeSource(t *testing.T) {
	source := strings.Repeat("한글🙂abc", 8000)
	store := &testStore{doc: document.Document{ID: "W135", Locale: "en", Slug: techmark.Slugify("Berkeley Software Distribution"), Title: "Berkeley Software Distribution", Source: source, Revision: "r7", Group: "secret-group", Author: "private-author"}}
	h := testHandler(t, store)
	for _, name := range []string{"list_editions", "search_pages"} {
		args := map[string]any{}
		if name == "search_pages" {
			args = map[string]any{"locale": "en", "query": "BSD"}
		}
		result := toolCall(t, h, name, args)
		data := string(result["structuredContent"])
		for _, forbidden := range []string{"W135", "private-user", "private-author", "secret-group", `"score"`, `"tid"`, `"pid"`, `"slug"`, `"requested_by"`} {
			if strings.Contains(data, forbidden) {
				t.Fatalf("public metadata leaked %q: %s", forbidden, data)
			}
		}
	}
	offset, revision := 0, ""
	var rebuilt strings.Builder
	for {
		result := toolCall(t, h, "read_page", map[string]any{"locale": "en", "name": "BSD", "offset": offset, "revision": revision})
		var value readOutput
		if err := json.Unmarshal(result["structuredContent"], &value); err != nil {
			t.Fatal(err)
		}
		if len(value.Source) > maxSourceBytes || !utf8.ValidString(value.Source) || value.Offset != offset || value.Revision != "r7" || value.URL != "https://wiki.example/en/wiki/Berkeley_Software_Distribution" {
			t.Fatalf("invalid source chunk: offset %d, result %+v", offset, value)
		}
		rebuilt.WriteString(value.Source)
		if value.Complete {
			break
		}
		if value.NextOffset <= offset {
			t.Fatal("chunk made no progress")
		}
		offset, revision = value.NextOffset, value.Revision
	}
	if rebuilt.String() != source {
		t.Fatal("chunks did not reconstruct exact source")
	}
	result := toolCall(t, h, "read_page", map[string]any{"locale": "en", "name": "BSD", "offset": 3, "revision": "r6"})
	if string(result["isError"]) != "true" {
		t.Fatal("changed revision accepted")
	}
}

func TestInvalidInputsAndStoreErrorsAreSafe(t *testing.T) {
	store := &testStore{doc: document.Document{Locale: "en", Slug: "page", Title: "Page", Source: "한글", Revision: "r1"}}
	h := testHandler(t, store)
	for _, args := range []map[string]any{
		{"locale": "zh-Hans", "name": "Page"}, {"locale": "pt-BR", "name": "Page"}, {"locale": "ar", "name": "Page"},
		{"locale": "en", "name": "missing"}, {"locale": "en", "name": ""}, {"locale": "en", "name": "Page", "offset": -1},
		{"locale": "en", "name": "Page", "offset": 1, "revision": "r1"}, {"locale": "en", "name": "Page", "offset": 99, "revision": "r1"},
		{"locale": "en", "name": "Page", "offset": 3}, {"locale": "en", "name": "Page", "confirmed": true},
	} {
		result := toolCall(t, h, "read_page", args)
		if string(result["isError"]) != "true" {
			t.Fatalf("invalid input accepted: %v -> %v", args, result)
		}
	}
	for _, args := range []map[string]any{{"locale": "en", "query": strings.Repeat("a", 257)}, {"locale": "en", "query": "Page", "limit": 51}, {"locale": "en", "query": "Page", "limit": -1}} {
		if result := toolCall(t, h, "search_pages", args); string(result["isError"]) != "true" {
			t.Fatalf("invalid search accepted: %v", args)
		}
	}
	store.err = errors.New("secret-token and private database path")
	result := toolCall(t, h, "read_page", map[string]any{"locale": "en", "name": "Page"})
	data, _ := json.Marshal(result)
	if string(result["isError"]) != "true" || strings.Contains(string(data), "secret-token") {
		t.Fatalf("unsafe error: %s", data)
	}
}

func TestAdminOnlySourceIsNeverReturnedWithoutOAuth(t *testing.T) {
	store := &testStore{doc: document.Document{Locale: "en", Slug: "protected", Title: "Protected", Source: "private source", Revision: "r1", EditPolicy: document.EditPolicyAdmins}}
	result := toolCall(t, testHandler(t, store), "read_page", map[string]any{"locale": "en", "name": "Protected"})
	data, _ := json.Marshal(result)
	if string(result["isError"]) != "true" || strings.Contains(string(data), "private source") {
		t.Fatalf("protected source disclosed: %s", data)
	}
}

func TestHTTPBoundary(t *testing.T) {
	h := testHandler(t, &testStore{})
	for _, tc := range []struct {
		method, host, origin, contentType, body string
		status                                  int
	}{
		{"POST", "evil.example", "", "application/json", `{}`, 403},
		{"POST", "wiki.example", "https://evil.example", "application/json", `{}`, 403},
		{"POST", "wiki.example", "null", "application/json", `{}`, 403},
		{"POST", "wiki.example", "", "text/plain", `{}`, 415},
		{"GET", "wiki.example", "", "application/json", ``, 405},
		{"DELETE", "wiki.example", "", "application/json", ``, 405},
		{"POST", "wiki.example", "", "application/json", strings.Repeat(" ", maxRequestBytes+1), 413},
		{"POST", "wiki.example", "", "application/json", `{broken`, 400},
	} {
		r := httptest.NewRequest(tc.method, "https://"+tc.host+"/mcp", strings.NewReader(tc.body))
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Content-Type", tc.contentType)
		r.Header.Set("Accept", "application/json, text/event-stream")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%+v -> %d %s", tc, w.Code, w.Body.String())
		}
	}
	for _, origin := range []string{"ftp://wiki.example", "https://user:password@wiki.example", "https://wiki.example/path", "https://wiki.example?secret=x", ""} {
		if _, err := NewHandler(wiki.NewReader(&testStore{}), origin); err == nil {
			t.Fatalf("invalid origin accepted: %q", origin)
		}
	}
}

func TestConcurrentRequestLimit(t *testing.T) {
	store := &testStore{block: make(chan struct{}), started: make(chan struct{}, 4)}
	h := testHandler(t, store)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rpc(t, h, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_editions","arguments":{}}}`)
		}()
	}
	for range 4 {
		<-store.started
	}
	w := rpc(t, h, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("overload = %d", w.Code)
	}
	close(store.block)
	wg.Wait()
}

func TestSDKClientConnectsOverRealHTTP(t *testing.T) {
	store := &testStore{}
	var handler http.Handler
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r) }))
	defer srv.Close()
	var err error
	handler, err = NewHandler(wiki.NewReader(store), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "integration-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: srv.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	if err != nil || len(tools.Tools) != 6 {
		t.Fatalf("list over HTTP: %+v %v", tools, err)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_editions", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("call over HTTP: %+v %v", result, err)
	}
	res, err := http.Get(srv.URL + "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode != 405 {
		t.Fatalf("GET = %d", res.StatusCode)
	}
}
