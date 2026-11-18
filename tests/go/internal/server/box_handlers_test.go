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

	"techpedia/internal/auth"
	"techpedia/internal/box"
	"techpedia/internal/document"
	"techpedia/internal/storage"
	"techpedia/internal/techdata"
	"techpedia/internal/techmark"
)

func boxRequest(s *Server, method, path, body, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", s.publicURL)
	if token != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func TestBoxAPIPermissionsSourceSizeAndSnapshotConflicts(t *testing.T) {
	s, accounts := oauthTestServer(t)
	db := s.store.(*storage.BadgerDocumentStore)
	configureTestEdition(t, db, "ko", "대문")
	path := "/api/admin/boxes/Common/ko"
	for _, method := range []string{"GET", "PUT"} {
		if w := boxRequest(s, method, path, `{"source":"text"}`, ""); w.Code != 401 {
			t.Fatal(w.Code, w.Body)
		}
	}
	accounts.user.Permissions = []auth.Permission{auth.PermissionEdit}
	if w := boxRequest(s, "PUT", path, `{"source":"text"}`, "admin-session"); w.Code != 403 {
		t.Fatal(w.Code, w.Body)
	}
	if w := boxRequest(s, "GET", "/api/editor/boxes", "", "admin-session"); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	accounts.user.Permissions = []auth.Permission{auth.PermissionAdmin}
	source := strings.Repeat("문자🙂 ", 3000)
	encoded, _ := json.Marshal(map[string]any{"source": source, "base_version": 0, "fallback_locale": ""})
	w := boxRequest(s, "PUT", path, string(encoded), "admin-session")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w := boxRequest(s, "PUT", path, string(encoded), "admin-session"); w.Code != 409 {
		t.Fatal(w.Code, w.Body)
	}
	if w := boxRequest(s, "GET", path, "", "admin-session"); w.Code != 200 || !strings.Contains(w.Body.String(), source) {
		t.Fatal(w.Code, w.Body)
	}
	r := httptest.NewRequest("PUT", path, strings.NewReader(string(encoded)))
	r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "admin-session"})
	r.Header.Set("Origin", "https://evil.example")
	r.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	for _, body := range []string{`{"source":"text","base_version":1} {}`, `{"source":"text","unknown":true}`, `{"source":"<wiki-box name=\"Common\" />","base_version":1}`} {
		if w := boxRequest(s, "PUT", path, body, "admin-session"); w.Code != 400 {
			t.Fatal(body, w.Code, w.Body)
		}
	}
}

func TestSharedBoxesRenderCurrentLocaleFallbackFreshUpdatesAndSEO(t *testing.T) {
	s, _ := oauthTestServer(t)
	db := s.store.(*storage.BadgerDocumentStore)
	ctx := context.Background()
	for _, l := range []string{"ko", "en", "ar"} {
		configureTestEdition(t, db, l, "Main")
	}
	in := box.Save{Name: "Common", Locale: "ko", Source: `<wiki-navbox title="한국어"><!--ignored-->` + "\n- 분류: [[Linux]]\n</wiki-navbox>"}
	if _, err := db.SaveBox(ctx, in, "admin"); err != nil {
		t.Fatal(err)
	}
	in.Locale = "en"
	in.Source = "<wiki-alert>English [[Linux]]</wiki-alert>"
	in.BaseVersion = 1
	if _, err := db.SaveBox(ctx, in, "admin"); err != nil {
		t.Fatal(err)
	}
	for _, l := range []string{"ko", "en", "ar"} {
		_, err := db.Save(ctx, document.Document{Locale: l, Title: "Article", Slug: "article", Source: "Body\n\n<wiki-box name=\"Common\" />"})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ locale, want, not string }{{"ko", "한국어", "English"}, {"en", "English", "한국어"}, {"ar", "Body", "한국어"}} {
		w := boxRequest(s, "GET", "/api/pages/"+tc.locale+"/Article", "", "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), tc.want) || strings.Contains(w.Body.String(), tc.not) || strings.Contains(w.Body.String(), "<wiki-box") {
			t.Fatal(w.Code, w.Body)
		}
	}
	in.Locale = "ar"
	in.Source = "<wiki-alert>العربية [[Linux]]</wiki-alert>"
	in.BaseVersion = 2
	in.FallbackLocale = "ar"
	if _, err := db.SaveBox(ctx, in, "admin"); err != nil {
		t.Fatal(err)
	}
	doc := document.Document{Locale: "fr", Source: `<wiki-box name="Common" />`}
	rendered, err := s.renderDocument(ctx, doc)
	if err != nil || !strings.Contains(rendered, `lang="ar" dir="rtl"`) || !strings.Contains(rendered, `/ar/wiki/Linux`) {
		t.Fatal(rendered, err)
	}
	in.Locale = "en"
	in.Source = "<wiki-alert>Updated English</wiki-alert>"
	in.BaseVersion = 3
	if _, err := db.SaveBox(ctx, in, "admin"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/pages/en/Article", "/en/wiki/Article"} {
		w := boxRequest(s, "GET", path, "", "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), "Updated English") {
			t.Fatal(w.Code, w.Body)
		}
	}
	rendered, err = s.renderDocument(ctx, document.Document{Locale: "en", Source: "`<wiki-box name=\"Common\" />`\n\n```\n<wiki-box name=\"Common\" />\n```"})
	if err != nil || strings.Contains(rendered, "Updated English") {
		t.Fatal(rendered, err)
	}
	rendered, err = s.renderDocument(ctx, document.Document{Locale: "en", Source: strings.Repeat(`<wiki-box name="Common" />`+"\n", 1000)})
	if err != nil || strings.Count(rendered, "Updated English") != 64 {
		t.Fatal("unbounded references", len(rendered), err)
	}
}

func TestBoxLanguageDraftUsesRealTechdataLinksWithoutGuessingOrWriting(t *testing.T) {
	s, _ := oauthTestServer(t)
	db := s.store.(*storage.BadgerDocumentStore)
	ctx := context.Background()
	for _, locale := range []string{"ko", "en"} {
		configureTestEdition(t, db, locale, "Main")
	}
	ko, err := db.Save(ctx, document.Document{Locale: "ko", Title: "운영체제", Slug: techmark.Slugify("운영체제"), Source: "content"})
	if err != nil {
		t.Fatal(err)
	}
	en, err := db.Save(ctx, document.Document{Locale: "en", Title: "Operating system", Slug: techmark.Slugify("Operating system"), Source: "content"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.CreateItem(ctx, techdata.Item{Labels: map[string]string{"ko": "운영체제"}, WikiPages: map[string]document.WID{"ko": ko.ID, "en": en.ID}})
	if err != nil {
		t.Fatal(err)
	}
	in := box.Save{Name: "OS", Locale: "ko", Source: "<wiki-navbox title=\"운영체제\">\n- 계열: [[운영체제|체제]], [[없는 문서]]\n</wiki-navbox>\n`[[운영체제]]`"}
	if _, err := db.SaveBox(ctx, in, "admin"); err != nil {
		t.Fatal(err)
	}
	w := boxRequest(s, "GET", "/api/admin/boxes/OS/draft/en?from=ko", "", "admin-session")
	var result struct {
		Source      string
		Unresolved  []string
		BaseVersion uint64 `json:"base_version"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil {
		t.Fatal(w.Code, w.Body)
	}
	if !strings.Contains(result.Source, "[[Operating system|체제]]") || strings.Contains(result.Source, "[[없는 문서]]") || !strings.Contains(result.Source, "`[[운영체제]]`") || len(result.Unresolved) != 1 || result.BaseVersion != 1 {
		t.Fatal(fmt.Sprintf("%+v", result))
	}
	v, _ := db.GetBox(ctx, "OS")
	if v.Version != 1 || len(v.Locales) != 1 {
		t.Fatal("copy saved without approval", v)
	}
}

func TestSharedBoxDateOnlyValidatorsNeverReturnStaleArticle(t *testing.T) {
	s, _ := oauthTestServer(t)
	db := s.store.(*storage.BadgerDocumentStore)
	ctx := context.Background()
	configureTestEdition(t, db, "en", "Main")
	_, err := db.Save(ctx, document.Document{Locale: "en", Title: "Article", Slug: "article", Source: `<wiki-box name="Later" />`})
	if err != nil {
		t.Fatal(err)
	}
	first := boxRequest(s, "GET", "/en/wiki/Article", "", "")
	if first.Code != 200 || first.Header().Get("Last-Modified") != "" {
		t.Fatal(first.Code, first.Header())
	}
	if _, err := db.SaveBox(ctx, box.Save{Name: "Later", Locale: "en", Source: "Now available"}, "admin"); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/en/wiki/Article", nil)
	r.Header.Set("If-Modified-Since", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Now available") || w.Header().Get("ETag") == first.Header().Get("ETag") {
		t.Fatal(w.Code, w.Body, w.Header())
	}
	r = httptest.NewRequest("GET", "/en/wiki/Article", nil)
	r.Header.Set("If-None-Match", w.Header().Get("ETag"))
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 304 {
		t.Fatal(w.Code)
	}
}
