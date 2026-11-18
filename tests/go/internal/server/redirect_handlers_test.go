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

func TestAdministratorManagedDocumentRedirects(t *testing.T) {
	store, err := storage.OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	configureTestEdition(t, store, "en", "Main Page")
	canonical, err := store.Save(context.Background(), document.Document{
		Locale: "en", Slug: techmark.Slugify("Berkeley Software Distribution"), Title: "Berkeley Software Distribution", Source: "# Berkeley Software Distribution",
	})
	if err != nil {
		t.Fatal(err)
	}

	accounts, err := auth.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer accounts.Close()
	if _, err := accounts.Register(context.Background(), auth.Registration{Username: "redirect_editor", Password: "long-editor-password"}); err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.Register(context.Background(), auth.Registration{Username: "redirect_admin", Password: "long-admin-password", Permissions: []auth.Permission{auth.PermissionAdmin}}); err != nil {
		t.Fatal(err)
	}
	_, editorToken, _, _ := accounts.Login(context.Background(), "redirect_editor", "long-editor-password")
	_, adminToken, _, _ := accounts.Login(context.Background(), "redirect_admin", "long-admin-password")
	srv := New(config.Config{}, store, accounts, techmark.NewParser())

	requestBody := `{"locale":"en","name":"BSD","target":"Berkeley Software Distribution"}`
	editorReq := httptest.NewRequest(http.MethodPost, "/api/admin/redirects", strings.NewReader(requestBody))
	editorReq.Header.Set("Content-Type", "application/json")
	editorReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: editorToken})
	editorRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(editorRec, editorReq)
	if editorRec.Code != http.StatusForbidden {
		t.Fatalf("editor created redirect: %d: %s", editorRec.Code, editorRec.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/admin/redirects", strings.NewReader(requestBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated || !strings.Contains(createRec.Body.String(), `"name":"BSD"`) || strings.Contains(createRec.Body.String(), canonical.ID.String()) {
		t.Fatalf("administrator could not create redirect or response leaked WID: %d: %s", createRec.Code, createRec.Body.String())
	}

	publicRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(publicRec, httptest.NewRequest(http.MethodGet, "/en/wiki/BSD", nil))
	if publicRec.Code != http.StatusPermanentRedirect || publicRec.Header().Get("Location") != "/en/wiki/Berkeley_Software_Distribution" {
		t.Fatalf("public alias did not redirect: %d location=%q", publicRec.Code, publicRec.Header().Get("Location"))
	}
	apiRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(apiRec, httptest.NewRequest(http.MethodGet, "/api/pages/en/BSD", nil))
	if apiRec.Code != http.StatusOK || !strings.Contains(apiRec.Body.String(), `"name":"Berkeley Software Distribution"`) || strings.Contains(apiRec.Body.String(), canonical.ID.String()) {
		t.Fatalf("public API did not resolve alias safely: %d: %s", apiRec.Code, apiRec.Body.String())
	}
	linkRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(linkRec, httptest.NewRequest(http.MethodGet, "/api/links/resolve?locale=en&target=BSD", nil))
	if linkRec.Code != http.StatusOK || !strings.Contains(linkRec.Body.String(), `"url":"/en/wiki/Berkeley_Software_Distribution"`) || !strings.Contains(linkRec.Body.String(), `"exists":true`) {
		t.Fatalf("internal link did not resolve alias: %d: %s", linkRec.Code, linkRec.Body.String())
	}
	searchRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(searchRec, httptest.NewRequest(http.MethodGet, "/api/search?locale=en&q=BSD", nil))
	if searchRec.Code != http.StatusOK || !strings.Contains(searchRec.Body.String(), `"name":"Berkeley Software Distribution"`) || strings.Contains(searchRec.Body.String(), `"create"`) {
		t.Fatalf("exact alias search did not return canonical document: %d: %s", searchRec.Code, searchRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/admin/redirects?locale=en", nil)
	listReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	var list struct {
		Redirects []adminDocumentRedirectResponse `json:"redirects"`
	}
	if listRec.Code != http.StatusOK || json.Unmarshal(listRec.Body.Bytes(), &list) != nil || len(list.Redirects) != 1 || list.Redirects[0].TargetName != canonical.Title {
		t.Fatalf("could not list redirects: %d: %s", listRec.Code, listRec.Body.String())
	}

	moved, err := store.MoveDocument(context.Background(), "en", canonical.Slug, techmark.Slugify("BSD family"), "BSD family")
	if err != nil {
		t.Fatal(err)
	}
	publicAfterMove := httptest.NewRecorder()
	srv.Handler().ServeHTTP(publicAfterMove, httptest.NewRequest(http.MethodGet, "/en/wiki/BSD", nil))
	if publicAfterMove.Code != http.StatusPermanentRedirect || publicAfterMove.Header().Get("Location") != "/en/wiki/BSD_family" {
		t.Fatalf("alias did not follow target move to %s: %d location=%q", moved.ID, publicAfterMove.Code, publicAfterMove.Header().Get("Location"))
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/admin/redirects/en/BSD", nil)
	deleteReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	deleteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("administrator could not delete redirect: %d: %s", deleteRec.Code, deleteRec.Body.String())
	}
	missingRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(missingRec, httptest.NewRequest(http.MethodGet, "/en/wiki/BSD", nil))
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("deleted redirect still works: %d", missingRec.Code)
	}
}
