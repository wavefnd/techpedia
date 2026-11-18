package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"techpedia/internal/auth"
	"techpedia/internal/config"
	"techpedia/internal/document"
	"techpedia/internal/storage"
	"techpedia/internal/techmark"
)

func TestMCPEndpointRequiresOptInAndValidPublicOrigin(t *testing.T) {
	for _, cfg := range []config.Config{{}, {MCPEnabled: true, PublicURL: "https://wiki.example/unexpected/path"}} {
		srv := New(cfg, emptyStore{}, emptyAuth{}, techmark.NewParser())
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, httptest.NewRequest(method, "/mcp", nil))
			if w.Code != http.StatusNotFound {
				t.Fatalf("disabled MCP: %d", w.Code)
			}
		}
	}
}

func TestMCPReadsSameBadgerDocumentsWithoutWriting(t *testing.T) {
	store, err := storage.OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	configureTestEdition(t, store, "en", "Main Page")
	configureTestEdition(t, store, "fr", "Accueil")
	doc, err := store.Save(context.Background(), document.Document{Locale: "en", Title: "Berkeley Software Distribution", Slug: techmark.Slugify("Berkeley Software Distribution"), Source: "# BSD\n\n[[Unix]]", Author: "test-editor"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateDocumentRedirect(context.Background(), "en", "BSD", doc.Slug, "test-editor"); err != nil {
		t.Fatal(err)
	}
	accounts := &oauthTestAccounts{user: auth.User{ID: "admin-id", Username: "admin", Permissions: []auth.Permission{auth.PermissionAdmin}}}
	srv := New(config.Config{MCPEnabled: true, PublicURL: "https://wiki.example"}, store, accounts, techmark.NewParser())
	tokens := testOAuthTokens(t, srv)
	request := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "https://wiki.example/mcp", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		r.Header.Set("MCP-Protocol-Version", "2025-11-25")
		r.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, r)
		return w
	}
	read := request(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_page","arguments":{"locale":"en","name":"BSD"}}}`)
	var result struct {
		Result struct {
			Structured struct{ Name, Source, Revision, URL string } `json:"structuredContent"`
		}
	}
	if err := json.Unmarshal(read.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if read.Code != 200 || result.Result.Structured.Source != doc.Source || result.Result.Structured.Revision != doc.Revision || result.Result.Structured.Name != doc.Title {
		t.Fatalf("MCP source: %d %s", read.Code, read.Body.String())
	}
	for _, forbidden := range []string{doc.ID.String(), `"author"`, `"wid"`, `"tid"`, `"slug"`} {
		if strings.Contains(read.Body.String(), forbidden) {
			t.Fatalf("MCP leaked %q", forbidden)
		}
	}
	public := httptest.NewRecorder()
	srv.Handler().ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/api/pages/en/BSD", nil))
	if public.Code != 200 || !strings.Contains(public.Body.String(), `"name":"`+doc.Title+`"`) {
		t.Fatalf("HTTP disagrees with MCP: %d %s", public.Code, public.Body.String())
	}
	search := request(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search_pages","arguments":{"locale":"en","query":"BSD"}}}`)
	if search.Code != 200 || !strings.Contains(search.Body.String(), `Berkeley_Software_Distribution`) {
		t.Fatalf("MCP redirect search: %d %s", search.Code, search.Body.String())
	}
	editions := request(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_editions","arguments":{}}}`)
	if !strings.Contains(editions.Body.String(), `"locale":"fr"`) {
		t.Fatalf("new edition missing: %s", editions.Body.String())
	}
	revisions, err := store.ListRevisions(context.Background(), "en", doc.Slug, 10)
	if err != nil || len(revisions) != 1 {
		t.Fatalf("read created revisions: %+v %v", revisions, err)
	}
}
