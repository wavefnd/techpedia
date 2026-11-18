package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"techpedia/internal/analytics"
	"techpedia/internal/auth"
	"techpedia/internal/bot"
	"techpedia/internal/buildinfo"
	"techpedia/internal/config"
	"techpedia/internal/discussion"
	"techpedia/internal/document"
	"techpedia/internal/edition"
	"techpedia/internal/media"
	"techpedia/internal/storage"
	"techpedia/internal/support"
	"techpedia/internal/techdata"
	"techpedia/internal/techmark"
)

func TestContentPagesAreNotSynthesized(t *testing.T) {
	srv := New(config.Config{}, emptyStore{}, emptyAuth{}, techmark.NewParser())

	req := httptest.NewRequest(http.MethodGet, "/api/pages/ko/copyright", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected administrator-created copyright page to be absent, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLanguageCatalogIsSearchableAndExcludesRegisteredEditions(t *testing.T) {
	srv := New(config.Config{}, emptyStore{}, emptyAuth{}, techmark.NewParser())

	req := httptest.NewRequest(http.MethodGet, "/api/language-catalog?q=Arabic&limit=10", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"tag":"ar"`) {
		t.Fatalf("expected Arabic catalog result, got %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/language-catalog?q=Korean&limit=10", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"tag":"ko"`) {
		t.Fatalf("registered Korean edition must be excluded, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLocalizedWikiRouteServesIndex(t *testing.T) {
	doc := document.Document{Locale: "ko", Group: "main", Slug: "main", Title: "대문", Source: "# 대문", Revision: "r1", UpdatedAt: time.Now().UTC()}
	srv := New(config.Config{PublicURL: "https://techpedia.example"}, revisionStore{emptyStore: emptyStore{}, doc: doc}, emptyAuth{}, techmark.NewParser())

	req := httptest.NewRequest(http.MethodGet, "/ko/wiki/대문", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("expected html content type, got %q", rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	for _, expected := range []string{
		`<html lang="ko" dir="ltr" data-writing-system="Kore">`,
		"<title>대문 - TechPedia</title>",
		`rel="canonical" href="https://techpedia.example/ko/wiki/%EB%8C%80%EB%AC%B8"`,
		`"@type":["Article","TechArticle"]`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected localized index to contain %q: %s", expected, body)
		}
	}
}

func TestLocalizedSearchRouteServesActiveEditionIndex(t *testing.T) {
	srv := New(config.Config{}, emptyStore{}, emptyAuth{}, techmark.NewParser())

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(method, "/ko/search?q=C", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s localized search route: expected 200, got %d: %s", method, rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Content-Language") != "ko" || rec.Header().Get("X-Robots-Tag") != "noindex,follow" {
			t.Fatalf("%s localized search metadata: language=%q robots=%q", method, rec.Header().Get("Content-Language"), rec.Header().Get("X-Robots-Tag"))
		}
		if method == http.MethodHead && rec.Body.Len() != 0 {
			t.Fatalf("HEAD localized search route returned a body")
		}
	}
}

func TestLocalizedSearchRouteRejectsInactiveEdition(t *testing.T) {
	srv := New(config.Config{}, emptyStore{}, emptyAuth{}, techmark.NewParser())
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/es/search?q=C", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected inactive search edition to return 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChangelogRouteIsCanonicalAndIndexable(t *testing.T) {
	srv := New(config.Config{PublicURL: "https://techpedia.example"}, emptyStore{}, emptyAuth{}, techmark.NewParser())
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(method, "/changelog", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s changelog route: expected 200, got %d: %s", method, rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Content-Language") != "ko" || rec.Header().Get("X-Robots-Tag") != "" {
			t.Fatalf("%s changelog metadata: language=%q robots=%q", method, rec.Header().Get("Content-Language"), rec.Header().Get("X-Robots-Tag"))
		}
		if method == http.MethodGet && (!strings.Contains(rec.Body.String(), "<title>TechPedia 변경 기록</title>") || !strings.Contains(rec.Body.String(), `rel="canonical" href="https://techpedia.example/changelog"`)) {
			t.Fatalf("changelog metadata missing: %s", rec.Body.String())
		}
		if method == http.MethodHead && rec.Body.Len() != 0 {
			t.Fatalf("HEAD changelog route returned a body")
		}
	}
}

func TestSearchAPIRouteIsNotCapturedAsLanguageEdition(t *testing.T) {
	srv := New(config.Config{}, apiEditionStore{emptyStore: emptyStore{}}, emptyAuth{}, techmark.NewParser())
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/search?locale=ko&q=C&limit=50", nil))

	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("search API route was captured by locale dispatch: status=%d content-type=%q body=%s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	var response publicSearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode search API response: %v", err)
	}
	if response.Query != "C" {
		t.Fatalf("search API query = %q, want C", response.Query)
	}
}

func TestParseSearchLimitSupportsFullSearchPage(t *testing.T) {
	if got := parseSearchLimit("50"); got != 50 {
		t.Fatalf("expected full search page limit 50, got %d", got)
	}
	if got := parseSearchLimit("500"); got != 50 {
		t.Fatalf("expected search limit to clamp to 50, got %d", got)
	}
}

func TestRootServesIndexableLanguagePortal(t *testing.T) {
	srv := New(config.Config{}, emptyStore{}, emptyAuth{}, techmark.NewParser())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	body := rec.Body.String()
	if rec.Code != http.StatusOK || rec.Header().Get("Location") != "" || rec.Header().Get("Content-Language") != "en" {
		t.Fatalf("expected root language portal, got %d location=%q: %s", rec.Code, rec.Header().Get("Location"), body)
	}
	for _, expected := range []string{`<html lang="en" dir="ltr" data-writing-system="Latn">`, `class="language-portal"`, `id="techpedia-portal-data"`, `href="/ko/wiki/대문"`, `"@type":"WebSite"`, `"@type":"Organization"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected indexable language portal to contain %q: %s", expected, body)
		}
	}
	if strings.Contains(body, `<link rel="stylesheet" crossorigin href="/assets/index-`) || !strings.Contains(body, `/assets/LanguagePortalView-`) {
		t.Fatalf("language portal did not replace the global stylesheet with its route stylesheet: %s", body)
	}
}

func TestRenderIndexIncludesRTLWritingSystem(t *testing.T) {
	result := string(renderIndex([]byte(`<html lang="ko"><head><title>TechPedia</title></head><body><div id="app"></div></body></html>`), indexMetadata{
		Lang:      "ar",
		Direction: "rtl",
		Title:     "TechPedia",
	}))

	if !strings.Contains(result, `<html lang="ar" dir="rtl" data-writing-system="Arab">`) {
		t.Fatalf("expected Arabic writing-system metadata: %s", result)
	}
}

func TestUnknownWebRouteServesDedicatedNotFoundPage(t *testing.T) {
	srv := New(config.Config{}, emptyStore{}, emptyAuth{}, techmark.NewParser())
	req := httptest.NewRequest(http.MethodGet, "/missing/public/path", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	body := rec.Body.String()
	if rec.Code != http.StatusNotFound || rec.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("expected HTML 404 page, got %d content-type=%q: %s", rec.Code, rec.Header().Get("Content-Type"), body)
	}
	for _, expected := range []string{
		`<title>Page not found - TechPedia</title>`,
		`content="noindex,follow,noarchive"`,
		`class="not-found-content"`,
		`href="/"`,
		`Go to language portal`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected 404 page to contain %q: %s", expected, body)
		}
	}
	if rec.Header().Get("X-Robots-Tag") != "noindex,follow,noarchive" {
		t.Fatalf("unexpected robots header: %q", rec.Header().Get("X-Robots-Tag"))
	}
}

func TestUnknownAPIRouteDoesNotServeBrowserNotFoundPage(t *testing.T) {
	srv := New(config.Config{}, emptyStore{}, emptyAuth{}, techmark.NewParser())
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/legacy", nil))

	if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), `not-found-content`) {
		t.Fatalf("expected plain API 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLanguagePortalUsesDocumentCounts(t *testing.T) {
	store := countedEditionStore{emptyStore: emptyStore{}}
	srv := New(config.Config{}, store, emptyAuth{}, techmark.NewParser())

	apiRequest := httptest.NewRequest(http.MethodGet, "/api/languages", nil)
	apiRecorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(apiRecorder, apiRequest)
	if apiRecorder.Code != http.StatusOK {
		t.Fatalf("list language editions: %d: %s", apiRecorder.Code, apiRecorder.Body.String())
	}
	var payload struct {
		Editions []editionResponse `json:"editions"`
	}
	if err := json.Unmarshal(apiRecorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode language editions: %v", err)
	}
	counts := make(map[string]int, len(payload.Editions))
	for _, value := range payload.Editions {
		counts[value.Tag] = value.DocumentCount
	}
	if counts["ko"] != 8 || counts["en"] != 3 || counts["es"] != 1 {
		t.Fatalf("unexpected document counts in API: %#v", counts)
	}

	rootRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	rootRecorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rootRecorder, rootRequest)
	body := rootRecorder.Body.String()
	start := strings.Index(body, `class="language-portal"`)
	if start < 0 {
		t.Fatalf("language portal initial HTML is missing: %s", body)
	}
	end := strings.Index(body[start:], `</main>`)
	if end < 0 {
		t.Fatalf("language portal initial HTML is incomplete: %s", body)
	}
	portal := body[start : start+end]
	koIndex := strings.Index(portal, `hreflang="ko"`)
	enIndex := strings.Index(portal, `hreflang="en"`)
	esIndex := strings.Index(portal, `hreflang="es"`)
	if koIndex < 0 || enIndex < 0 || esIndex < 0 || !(koIndex < enIndex && enIndex < esIndex) {
		t.Fatalf("initial portal is not sorted by descending document count: %s", portal)
	}
}

func TestSupportPageIsHiddenUntilEnabled(t *testing.T) {
	disabled := New(config.Config{PublicURL: "https://techpedia.example"}, emptyStore{}, emptyAuth{}, techmark.NewParser())
	request := httptest.NewRequest(http.MethodGet, "/support", nil)
	recorder := httptest.NewRecorder()
	disabled.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), `content="noindex,nofollow"`) {
		t.Fatalf("disabled support page must be a noindex 404: %d: %s", recorder.Code, recorder.Body.String())
	}
	apiRequest := httptest.NewRequest(http.MethodGet, "/api/support", nil)
	apiRecorder := httptest.NewRecorder()
	disabled.Handler().ServeHTTP(apiRecorder, apiRequest)
	if apiRecorder.Code != http.StatusOK || !strings.Contains(apiRecorder.Body.String(), `"enabled":false`) || !strings.Contains(apiRecorder.Body.String(), `"wallets":[]`) {
		t.Fatalf("disabled public support settings must be an empty 200 response: %d: %s", apiRecorder.Code, apiRecorder.Body.String())
	}

	settings := support.Settings{
		Enabled: true, Title: "Support <TechPedia>", Description: "Official addresses only.",
		Wallets: []support.Wallet{{Asset: "BTC", Network: "Bitcoin", Address: "bc1qexampleaddress", Memo: "<none>"}},
	}
	enabled := New(config.Config{PublicURL: "https://techpedia.example"}, supportStore{emptyStore: emptyStore{}, settings: settings}, emptyAuth{}, techmark.NewParser())
	request = httptest.NewRequest(http.MethodGet, "/support", nil)
	recorder = httptest.NewRecorder()
	enabled.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `Support &lt;TechPedia&gt;`) || strings.Contains(recorder.Body.String(), `<none>`) {
		t.Fatalf("enabled support page did not safely render: %d: %s", recorder.Code, recorder.Body.String())
	}
	apiRequest = httptest.NewRequest(http.MethodGet, "/api/support", nil)
	apiRecorder = httptest.NewRecorder()
	enabled.Handler().ServeHTTP(apiRecorder, apiRequest)
	if apiRecorder.Code != http.StatusOK || !strings.Contains(apiRecorder.Body.String(), `"address":"bc1qexampleaddress"`) {
		t.Fatalf("public support API omitted wallet: %d: %s", apiRecorder.Code, apiRecorder.Body.String())
	}
	sitemapRequest := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	sitemapRecorder := httptest.NewRecorder()
	enabled.Handler().ServeHTTP(sitemapRecorder, sitemapRequest)
	if sitemapRecorder.Code != http.StatusOK || !strings.Contains(sitemapRecorder.Body.String(), `<loc>https://techpedia.example/support</loc>`) {
		t.Fatalf("enabled support page missing from sitemap: %d: %s", sitemapRecorder.Code, sitemapRecorder.Body.String())
	}
}

func TestPublicConfigExposesEngineVersion(t *testing.T) {
	srv := New(config.Config{}, emptyStore{}, emptyAuth{}, techmark.NewParser())
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if buildinfo.Version == "" || rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"engine":{"version":"`+buildinfo.Version+`"}`) {
		t.Fatalf("expected public engine version, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestVersionedFrontendAssetsUseImmutableCache(t *testing.T) {
	entries, err := staticFS.ReadDir("static/dist/assets")
	if err != nil || len(entries) == 0 {
		t.Fatalf("read embedded frontend assets: %v", err)
	}
	asset := entries[0].Name()
	srv := New(config.Config{}, emptyStore{}, emptyAuth{}, techmark.NewParser())
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/"+asset, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("asset status = %d for %s", rec.Code, asset)
	}
	if got, want := rec.Header().Get("Cache-Control"), "public, max-age=31536000, immutable"; got != want {
		t.Fatalf("Cache-Control = %q, want %q", got, want)
	}
	if got, want := rec.Header().Get("X-Robots-Tag"), "noindex,noarchive"; got != want {
		t.Fatalf("asset X-Robots-Tag = %q, want %q", got, want)
	}
}

func TestEmbeddedFontsUseImmutableCacheWithoutDirectoryListing(t *testing.T) {
	entries, err := staticFS.ReadDir("static/dist/fonts/noto-cjk/subsets")
	if err != nil || len(entries) == 0 {
		t.Fatalf("read embedded font assets: %v", err)
	}
	font := ""
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".woff2") {
			font = entry.Name()
			break
		}
	}
	if font == "" {
		t.Fatal("embedded CJK font slices are missing")
	}

	srv := New(config.Config{}, emptyStore{}, emptyAuth{}, techmark.NewParser())
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/fonts/noto-cjk/subsets/"+font, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("font status = %d for %s", rec.Code, font)
	}
	if got, want := rec.Header().Get("Content-Type"), "font/woff2"; got != want {
		t.Fatalf("font Content-Type = %q, want %q", got, want)
	}
	if got, want := rec.Header().Get("Cache-Control"), "public, max-age=31536000, immutable"; got != want {
		t.Fatalf("font Cache-Control = %q, want %q", got, want)
	}
	if got, want := rec.Header().Get("X-Robots-Tag"), "noindex,noarchive"; got != want {
		t.Fatalf("font X-Robots-Tag = %q, want %q", got, want)
	}

	directory := httptest.NewRecorder()
	srv.Handler().ServeHTTP(directory, httptest.NewRequest(http.MethodGet, "/fonts/noto-cjk/subsets/", nil))
	if directory.Code != http.StatusNotFound || strings.Contains(directory.Body.String(), font) {
		t.Fatalf("font directory must not be listed: status=%d body=%s", directory.Code, directory.Body.String())
	}
}

func TestSecurityHeadersUseReportOnlyCSPAndOriginIsolation(t *testing.T) {
	srv := New(config.Config{PublicURL: "https://techpedia.example"}, emptyStore{}, emptyAuth{}, techmark.NewParser())
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	policy := rec.Header().Get("Content-Security-Policy-Report-Only")
	for _, directive := range []string{
		"default-src 'self'",
		"object-src 'none'",
		"frame-ancestors 'none'",
		"img-src 'self' data: https:",
		"script-src 'self' https://static.cloudflareinsights.com",
		"connect-src 'self' https://cloudflareinsights.com",
		"require-trusted-types-for 'script'",
		"trusted-types techpedia vue dompurify",
		"report-uri /api/csp-reports",
		"report-to techpedia-csp",
	} {
		if !strings.Contains(policy, directive) {
			t.Fatalf("report-only CSP is missing %q: %q", directive, policy)
		}
	}
	if got, want := rec.Header().Get("Reporting-Endpoints"), `techpedia-csp="https://techpedia.example/api/csp-reports"`; got != want {
		t.Fatalf("Reporting-Endpoints = %q, want %q", got, want)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != "" {
		t.Fatalf("enforcing CSP must be absent by default, got %q", got)
	}
	for header, want := range map[string]string{
		"Cross-Origin-Opener-Policy":   "same-origin",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Origin-Agent-Cluster":         "?1",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Fatalf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestSecurityHeadersCanEnableCSPEnforcement(t *testing.T) {
	srv := New(config.Config{CSPMode: "enforce"}, emptyStore{}, emptyAuth{}, techmark.NewParser())
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "require-trusted-types-for 'script'") {
		t.Fatalf("enforcing CSP is missing Trusted Types: %q", got)
	}
	if got := rec.Header().Get("Content-Security-Policy-Report-Only"); got != "" {
		t.Fatalf("report-only CSP must be absent in enforce mode, got %q", got)
	}
}

func TestCSPReportEndpointAcceptsReportingAndLegacyFormats(t *testing.T) {
	srv := New(config.Config{}, emptyStore{}, emptyAuth{}, techmark.NewParser())
	for _, test := range []struct {
		contentType string
		body        string
	}{
		{
			contentType: "application/reports+json",
			body:        `[{"type":"csp-violation","body":{"disposition":"report","effectiveDirective":"script-src-elem","blockedURL":"https://example.invalid/a.js"}}]`,
		},
		{
			contentType: "application/csp-report",
			body:        `{"csp-report":{"disposition":"report","violated-directive":"img-src","blocked-uri":"https://example.invalid/a.png"}}`,
		},
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/csp-reports", strings.NewReader(test.body))
		req.Header.Set("Content-Type", test.contentType)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("CSP report %s status = %d: %s", test.contentType, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("CSP report Cache-Control = %q", got)
		}
	}
}

func TestParseCSPReportsNormalizesFieldNames(t *testing.T) {
	reports := parseCSPReports([]byte(`[{"body":{"disposition":"enforce","effectiveDirective":"script-src-elem","blockedURL":"inline","sourceFile":"https://techpedia.example/assets/app.js"}}]`))
	if len(reports) != 1 {
		t.Fatalf("parsed reports = %d, want 1", len(reports))
	}
	if got, want := reports[0].Directive, "script-src-elem"; got != want {
		t.Fatalf("directive = %q, want %q", got, want)
	}
	if got, want := reports[0].BlockedURL, "inline"; got != want {
		t.Fatalf("blocked URL = %q, want %q", got, want)
	}
}

func TestDocumentStatisticsCountsSourceLinesAndUnicodeCharacters(t *testing.T) {
	lines, characters := documentStatistics("# 제목\r\n\r\n본문 😀\r\n")
	if lines != 3 || characters != 8 {
		t.Fatalf("expected 3 lines and 8 characters, got %d lines and %d characters", lines, characters)
	}
	if lines, characters := documentStatistics(""); lines != 0 || characters != 0 {
		t.Fatalf("empty source must have zero length, got %d lines and %d characters", lines, characters)
	}
}

func TestMissingWikiRouteServesCreationShellWith404AndNoIndex(t *testing.T) {
	srv := New(config.Config{PublicURL: "https://techpedia.example"}, emptyStore{}, emptyAuth{}, techmark.NewParser())

	req := httptest.NewRequest(http.MethodGet, "/ko/wiki/운영체제", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<div id="app"></div>`) {
		t.Fatalf("expected Vue shell for missing document: %s", body)
	}
	if !strings.Contains(body, `content="noindex,follow,noarchive"`) {
		t.Fatalf("expected missing document to be excluded from indexing: %s", body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("X-Robots-Tag") != "noindex,follow,noarchive" {
		t.Fatalf("missing document cache/index headers are unsafe: %#v", rec.Header())
	}
	if strings.Contains(body, `"@type":["Article","TechArticle"]`) {
		t.Fatalf("missing document must not emit article structured data: %s", body)
	}
}

func TestWikiRouteHeadMatchesGetWithoutBody(t *testing.T) {
	doc := document.Document{
		Locale:    "ko",
		Slug:      "linux",
		Title:     "Linux",
		Source:    "# Linux\n\nLinux는 운영체제 커널이다.",
		Revision:  "r1",
		CreatedAt: time.Date(2026, 7, 19, 1, 2, 3, 0, time.UTC),
		UpdatedAt: time.Date(2026, 7, 20, 1, 2, 3, 0, time.UTC),
	}
	for _, test := range []struct {
		name   string
		path   string
		status int
		store  Store
	}{
		{name: "existing document", path: "/ko/wiki/Linux", status: http.StatusOK, store: revisionStore{emptyStore: emptyStore{}, doc: doc}},
		{name: "missing document", path: "/ko/wiki/없는문서", status: http.StatusNotFound, store: emptyStore{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			srv := New(config.Config{PublicURL: "https://techpedia.example"}, test.store, emptyAuth{}, techmark.NewParser())
			getRecorder := httptest.NewRecorder()
			srv.Handler().ServeHTTP(getRecorder, httptest.NewRequest(http.MethodGet, test.path, nil))

			headRecorder := httptest.NewRecorder()
			srv.Handler().ServeHTTP(headRecorder, httptest.NewRequest(http.MethodHead, test.path, nil))

			if getRecorder.Code != test.status || headRecorder.Code != test.status {
				t.Fatalf("GET status = %d, HEAD status = %d, want %d", getRecorder.Code, headRecorder.Code, test.status)
			}
			if headRecorder.Body.Len() != 0 {
				t.Fatalf("HEAD response must not contain a body: %q", headRecorder.Body.String())
			}
			for _, header := range []string{"Content-Type", "Content-Length", "Content-Language", "Cache-Control", "ETag", "Last-Modified", "X-Robots-Tag"} {
				if got, want := headRecorder.Header().Get(header), getRecorder.Header().Get(header); got != want {
					t.Fatalf("HEAD %s = %q, GET = %q", header, got, want)
				}
			}
			if got, want := headRecorder.Header().Get("Content-Length"), strconv.Itoa(getRecorder.Body.Len()); got != want {
				t.Fatalf("HEAD Content-Length = %q, GET body length = %q", got, want)
			}
		})
	}
}

func TestStoredWikiRouteEmitsArticleMetadata(t *testing.T) {
	doc := document.Document{
		Locale:    "ko",
		Slug:      "c",
		Title:     "C (프로그래밍 언어)",
		Source:    "# C\n\nC는 범용 프로그래밍 언어이다.",
		HTML:      "<h1>C</h1>",
		Revision:  "r2",
		CreatedAt: time.Date(2026, 7, 13, 1, 2, 3, 0, time.UTC),
		UpdatedAt: time.Date(2026, 7, 14, 1, 2, 3, 0, time.UTC),
	}
	srv := New(config.Config{PublicURL: "https://techpedia.example"}, revisionStore{emptyStore: emptyStore{}, doc: doc}, emptyAuth{}, techmark.NewParser())

	req := httptest.NewRequest(http.MethodGet, "/ko/wiki/C_(프로그래밍_언어)", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, expected := range []string{
		`content="C는 범용 프로그래밍 언어이다."`,
		`"@type":["Article","TechArticle"]`,
		`"author":{"@type":"Organization","name":"TechPedia contributors","url":"https://techpedia.example/"}`,
		`"datePublished":"2026-07-13T01:02:03Z"`,
		`"dateModified":"2026-07-14T01:02:03Z"`,
		`property="og:type" content="article"`,
		`name="twitter:card" content="summary"`,
		`<script id="techpedia-page-data" type="application/json">`,
		`"html":""`,
		`class="article-view seo-initial-content"`,
		`<div class="techmark-content"><h1>C</h1>`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected article metadata %q: %s", expected, body)
		}
	}
	if strings.Count(body, `<h1>C</h1>`) != 1 {
		t.Fatalf("rendered article body must not be duplicated in bootstrap data: %s", body)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=0, must-revalidate" || rec.Header().Get("ETag") == "" {
		t.Fatalf("article revalidation headers missing: %#v", rec.Header())
	}
	if got := rec.Header().Get("Last-Modified"); got != "" {
		t.Fatalf("HTML must not validate its dependencies using the document date: %q", got)
	}

	etagRequest := httptest.NewRequest(http.MethodGet, "/ko/wiki/C_(프로그래밍_언어)", nil)
	etagRequest.Header.Set("If-None-Match", rec.Header().Get("ETag"))
	etagRecorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(etagRecorder, etagRequest)
	if etagRecorder.Code != http.StatusNotModified || etagRecorder.Body.Len() != 0 {
		t.Fatalf("article ETag revalidation = %d body=%q", etagRecorder.Code, etagRecorder.Body.String())
	}

	modifiedRequest := httptest.NewRequest(http.MethodGet, "/ko/wiki/C_(프로그래밍_언어)", nil)
	modifiedRequest.Header.Set("If-Modified-Since", doc.UpdatedAt.UTC().Format(http.TimeFormat))
	modifiedRecorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(modifiedRecorder, modifiedRequest)
	if modifiedRecorder.Code != http.StatusOK || modifiedRecorder.Body.Len() == 0 {
		t.Fatalf("article timestamp revalidation = %d body=%q", modifiedRecorder.Code, modifiedRecorder.Body.String())
	}
}

func TestRecordPageViewUsesSmallDedicatedEndpoint(t *testing.T) {
	doc := document.Document{Locale: "ko", Slug: techmark.Slugify("C"), Title: "C", Revision: "r1"}
	store := &pageViewStore{revisionStore: revisionStore{emptyStore: emptyStore{}, doc: doc}}
	srv := New(config.Config{}, store, emptyAuth{}, techmark.NewParser())
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/pages/ko/C/views", nil))

	if recorder.Code != http.StatusNoContent || recorder.Body.Len() != 0 {
		t.Fatalf("page view response = %d body=%q", recorder.Code, recorder.Body.String())
	}
	if store.views != 1 || store.locale != "ko" || store.slug != doc.Slug {
		t.Fatalf("recorded page view = count:%d locale:%q slug:%q", store.views, store.locale, store.slug)
	}
}

func TestPublicPageDescriptionDoesNotRepeatDocumentTitle(t *testing.T) {
	response := newPublicPageResponse(document.Document{
		Locale: "ko", Slug: "c-plus-plus", Title: "C++",
		Source: "C++\n\nC++는 C에서 파생된 범용 프로그래밍 언어이다.",
	}, nil)
	if got, want := response.Description, "C++는 C에서 파생된 범용 프로그래밍 언어이다."; got != want {
		t.Fatalf("description = %q, want %q", got, want)
	}
}

func TestRobotsAndSitemapUseConfiguredPublicURL(t *testing.T) {
	srv := New(config.Config{PublicURL: "https://techpedia.example"}, emptyStore{}, emptyAuth{}, techmark.NewParser())

	robotsReq := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	robotsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(robotsRec, robotsReq)
	if robotsRec.Code != http.StatusOK || !strings.Contains(robotsRec.Body.String(), "Sitemap: https://techpedia.example/sitemap.xml") {
		t.Fatalf("unexpected robots response %d: %s", robotsRec.Code, robotsRec.Body.String())
	}
	if !strings.Contains(robotsRec.Body.String(), "Disallow: /admin/") {
		t.Fatalf("robots response did not exclude private routes: %s", robotsRec.Body.String())
	}
	for _, unexpected := range []string{"GPTBot", "ClaudeBot", "Google-Extended", "Disallow: /assets/"} {
		if strings.Contains(robotsRec.Body.String(), unexpected) {
			t.Fatalf("robots response must not apply crawler-specific asset restrictions %q: %s", unexpected, robotsRec.Body.String())
		}
	}

	sitemapReq := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	sitemapRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(sitemapRec, sitemapReq)
	if sitemapRec.Code != http.StatusOK {
		t.Fatalf("unexpected sitemap status %d: %s", sitemapRec.Code, sitemapRec.Body.String())
	}
	if strings.Contains(sitemapRec.Body.String(), "/wiki/") {
		t.Fatalf("empty storage must not synthesize sitemap documents: %s", sitemapRec.Body.String())
	}
	if !strings.Contains(sitemapRec.Body.String(), `<loc>https://techpedia.example/</loc>`) {
		t.Fatalf("sitemap must include the indexable language portal: %s", sitemapRec.Body.String())
	}
	if !strings.Contains(sitemapRec.Body.String(), `<loc>https://techpedia.example/changelog</loc>`) {
		t.Fatalf("sitemap must include the changelog: %s", sitemapRec.Body.String())
	}
	if sitemapRec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("expected security headers on public responses")
	}
	if sitemapRec.Header().Get("ETag") == "" || sitemapRec.Header().Get("Cache-Control") != "public, max-age=300, stale-while-revalidate=60" {
		t.Fatalf("sitemap revalidation headers missing: %#v", sitemapRec.Header())
	}
	conditionalReq := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	conditionalReq.Header.Set("If-None-Match", sitemapRec.Header().Get("ETag"))
	conditionalRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(conditionalRec, conditionalReq)
	if conditionalRec.Code != http.StatusNotModified || conditionalRec.Body.Len() != 0 {
		t.Fatalf("sitemap ETag revalidation = %d body=%q", conditionalRec.Code, conditionalRec.Body.String())
	}
}

func TestSecurityTextFollowsRFC9116AndSupportsHead(t *testing.T) {
	srv := New(config.Config{PublicURL: "https://techpedia.example"}, emptyStore{}, emptyAuth{}, techmark.NewParser())

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/security.txt", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("security.txt status = %d: %s", rec.Code, rec.Body.String())
	}
	if got, want := rec.Header().Get("Content-Type"), "text/plain; charset=utf-8"; got != want {
		t.Fatalf("security.txt Content-Type = %q, want %q", got, want)
	}
	for _, line := range []string{
		"Contact: mailto:luna@lunastev.org",
		"Expires: " + securityTextExpires,
		"Preferred-Languages: ko, en",
		"Canonical: https://techpedia.example/.well-known/security.txt",
	} {
		if !strings.Contains(rec.Body.String(), line+"\n") {
			t.Fatalf("security.txt is missing %q: %s", line, rec.Body.String())
		}
	}
	if rec.Header().Get("ETag") == "" {
		t.Fatalf("security.txt must provide an ETag")
	}

	head := httptest.NewRecorder()
	srv.Handler().ServeHTTP(head, httptest.NewRequest(http.MethodHead, "/.well-known/security.txt", nil))
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Fatalf("HEAD security.txt = %d body=%q", head.Code, head.Body.String())
	}
	if head.Header().Get("Content-Length") != rec.Header().Get("Content-Length") {
		t.Fatalf("HEAD security.txt Content-Length = %q, want %q", head.Header().Get("Content-Length"), rec.Header().Get("Content-Length"))
	}

	conditional := httptest.NewRecorder()
	conditionalRequest := httptest.NewRequest(http.MethodGet, "/.well-known/security.txt", nil)
	conditionalRequest.Header.Set("If-None-Match", rec.Header().Get("ETag"))
	srv.Handler().ServeHTTP(conditional, conditionalRequest)
	if conditional.Code != http.StatusNotModified || conditional.Body.Len() != 0 {
		t.Fatalf("conditional security.txt = %d body=%q", conditional.Code, conditional.Body.String())
	}

	legacy := httptest.NewRecorder()
	srv.Handler().ServeHTTP(legacy, httptest.NewRequest(http.MethodGet, "/security.txt", nil))
	if legacy.Code != http.StatusPermanentRedirect || legacy.Header().Get("Location") != "/.well-known/security.txt" {
		t.Fatalf("legacy security.txt redirect = %d location=%q", legacy.Code, legacy.Header().Get("Location"))
	}
}

func TestSecurityTextOmitsNonHTTPSCanonicalURL(t *testing.T) {
	srv := New(config.Config{PublicURL: "http://localhost:8080"}, emptyStore{}, emptyAuth{}, techmark.NewParser())
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/security.txt", nil))
	if strings.Contains(rec.Body.String(), "Canonical:") {
		t.Fatalf("security.txt must not publish a non-HTTPS canonical URI: %s", rec.Body.String())
	}
}

func TestSuccessfulAssetRequestsAreOmittedFromAccessLog(t *testing.T) {
	for _, test := range []struct {
		path   string
		status int
		want   bool
	}{
		{path: "/assets/index.js", status: http.StatusOK, want: false},
		{path: "/assets/index.js", status: http.StatusPartialContent, want: false},
		{path: "/assets/index.js", status: http.StatusNotModified, want: false},
		{path: "/assets/missing.js", status: http.StatusNotFound, want: true},
		{path: "/api/csp-reports", status: http.StatusNoContent, want: false},
		{path: "/api/csp-reports", status: http.StatusBadRequest, want: true},
		{path: "/ko/wiki/Linux", status: http.StatusOK, want: true},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		if got := shouldLogRequest(request, test.status); got != test.want {
			t.Fatalf("shouldLogRequest(%q, %d) = %t, want %t", test.path, test.status, got, test.want)
		}
	}
}

func TestMissingAssetsNeverReceiveImmutableCachePolicy(t *testing.T) {
	srv := New(config.Config{}, emptyStore{}, emptyAuth{}, techmark.NewParser())
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, path := range []string{"/assets/missing-admin-chunk.js", "/assets/", "/fonts/missing-font.woff2"} {
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, httptest.NewRequest(method, path, nil))
			if w.Code != 404 || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("%s %s: status %d cache %q", method, path, w.Code, w.Header().Get("Cache-Control"))
			}
		}
	}
}

func TestSitemapReflectsCurrentDocumentsAndActiveEditions(t *testing.T) {
	store, err := storage.OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open document store: %v", err)
	}
	defer store.Close()
	configureTestEdition(t, store, "ko", "대문")
	if _, err := store.Save(context.Background(), document.Document{Locale: "ko", Slug: techmark.Slugify("이전 이름"), Title: "이전 이름", Source: "# 이전 이름"}); err != nil {
		t.Fatalf("save sitemap document: %v", err)
	}
	name, nativeName, direction, ok := edition.DescribeLanguage("fr")
	if !ok {
		t.Fatal("describe pending French edition")
	}
	if _, err := store.ProposeEdition(context.Background(), edition.Edition{Tag: "fr", Name: name, NativeName: nativeName, Direction: direction, Status: edition.StatusPending}); err != nil {
		t.Fatalf("propose pending edition: %v", err)
	}
	if _, err := store.Save(context.Background(), document.Document{Locale: "fr", Slug: techmark.Slugify("Brouillon"), Title: "Brouillon", Source: "# Brouillon"}); err != nil {
		t.Fatalf("save pending-edition document: %v", err)
	}

	srv := New(config.Config{PublicURL: "https://techpedia.example"}, store, emptyAuth{}, techmark.NewParser())
	requestSitemap := func() string {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("sitemap status %d: %s", rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	body := requestSitemap()
	if !strings.Contains(body, srv.absoluteURL(wikiURL("ko", "이전 이름"))) || strings.Contains(body, "/fr/wiki/Brouillon") {
		t.Fatalf("sitemap did not include only active-edition documents: %s", body)
	}
	if !strings.Contains(body, "<lastmod>") || !strings.Contains(body, "T") || !strings.Contains(body, "Z</lastmod>") {
		t.Fatalf("sitemap must expose precise UTC lastmod timestamps: %s", body)
	}
	if _, err := store.MoveDocument(context.Background(), "ko", techmark.Slugify("이전 이름"), techmark.Slugify("새 이름"), "새 이름"); err != nil {
		t.Fatalf("move sitemap document: %v", err)
	}
	body = requestSitemap()
	if strings.Contains(body, srv.absoluteURL(wikiURL("ko", "이전 이름"))) || !strings.Contains(body, srv.absoluteURL(wikiURL("ko", "새 이름"))) {
		t.Fatalf("sitemap did not refresh after document move: %s", body)
	}
}

func TestInactiveEditionBlocksArticlesUntilAdminCreatesMainPage(t *testing.T) {
	store, err := storage.OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open document store: %v", err)
	}
	defer store.Close()
	accounts, err := auth.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open account store: %v", err)
	}
	defer accounts.Close()
	admin, err := accounts.Register(context.Background(), auth.Registration{
		Username: "language_admin", Password: "long-admin-password", Permissions: []auth.Permission{auth.PermissionAdmin},
	})
	if err != nil || !admin.Has(auth.PermissionAdmin) {
		t.Fatalf("register administrator: user=%#v err=%v", admin, err)
	}
	_, token, _, err := accounts.Login(context.Background(), "language_admin", "long-admin-password")
	if err != nil {
		t.Fatalf("login administrator: %v", err)
	}
	if _, err := store.ProposeEdition(context.Background(), edition.Edition{
		Tag: "fr", Name: "French", NativeName: "Français", Direction: "ltr", Status: edition.StatusPending,
	}); err != nil {
		t.Fatalf("propose French edition: %v", err)
	}
	srv := New(config.Config{PublicURL: "https://techpedia.example"}, store, accounts, techmark.NewParser())

	inactiveReq := httptest.NewRequest(http.MethodGet, "/api/pages/fr/Accueil", nil)
	inactiveRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(inactiveRec, inactiveReq)
	if inactiveRec.Code != http.StatusConflict {
		t.Fatalf("expected inactive edition conflict, got %d: %s", inactiveRec.Code, inactiveRec.Body.String())
	}
	approveReq := httptest.NewRequest(http.MethodPost, "/api/admin/languages/fr/approve", nil)
	approveReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	approveRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(approveRec, approveReq)
	if approveRec.Code != http.StatusOK {
		t.Fatalf("approve French edition: %d: %s", approveRec.Code, approveRec.Body.String())
	}

	payload := map[string]any{
		"main_page_name": "Accueil", "source": "# Accueil\n\nTechPedia en français.",
		"interface_messages": configuredMessages(),
		"navigation":         []edition.NavigationSection{{Title: "Explorer", Items: []edition.NavigationItem{{Label: "Accueil", Target: "Accueil"}}}},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode activation: %v", err)
	}
	body := string(encoded)
	activateReq := httptest.NewRequest(http.MethodPost, "/api/admin/languages/fr/activate", strings.NewReader(body))
	activateReq.Header.Set("Content-Type", "application/json")
	activateReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	activateRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(activateRec, activateReq)
	if activateRec.Code != http.StatusOK {
		t.Fatalf("activate French edition: %d: %s", activateRec.Code, activateRec.Body.String())
	}

	pageReq := httptest.NewRequest(http.MethodGet, "/api/pages/fr/Accueil", nil)
	pageRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(pageRec, pageReq)
	if pageRec.Code != http.StatusOK || !strings.Contains(pageRec.Body.String(), `"locale":"fr"`) {
		t.Fatalf("expected active French main page, got %d: %s", pageRec.Code, pageRec.Body.String())
	}
	adminPagesReq := httptest.NewRequest(http.MethodGet, "/api/admin/pages?locale=fr", nil)
	adminPagesReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	adminPagesRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(adminPagesRec, adminPagesReq)
	if adminPagesRec.Code != http.StatusOK {
		t.Fatalf("list French admin pages: %d: %s", adminPagesRec.Code, adminPagesRec.Body.String())
	}
	if !strings.Contains(adminPagesRec.Body.String(), `"name":"Accueil"`) || !strings.Contains(adminPagesRec.Body.String(), `"wid":"W`) {
		t.Fatalf("admin pages omitted stored main page: %s", adminPagesRec.Body.String())
	}
	for _, absent := range []string{"Recent Changes", "Random Page", "Help", "Copyright"} {
		if strings.Contains(adminPagesRec.Body.String(), absent) {
			t.Fatalf("admin pages synthesized %q: %s", absent, adminPagesRec.Body.String())
		}
	}
	if strings.Count(adminPagesRec.Body.String(), `"name":"Accueil"`) != 1 {
		t.Fatalf("admin pages duplicated the edition main page: %s", adminPagesRec.Body.String())
	}
	indexReq := httptest.NewRequest(http.MethodGet, "/fr/wiki/Accueil", nil)
	indexRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(indexRec, indexReq)
	if indexRec.Code != http.StatusOK || indexRec.Header().Get("Content-Language") != "fr" {
		t.Fatalf("expected French HTML response, got %d headers=%v", indexRec.Code, indexRec.Header())
	}
}

func TestAdminAPIRequiresToken(t *testing.T) {
	srv := New(config.Config{}, emptyStore{}, emptyAuth{}, techmark.NewParser())
	paths := []string{
		"/api/admin/techdata/items",
		"/api/admin/techdata/items/T1",
		"/api/admin/techdata/audit",
		"/api/admin/users",
		"/api/admin/dashboard",
		"/api/admin/pages",
		"/api/admin/discussions/topics",
		"/api/admin/bots/naming/suggestions",
		"/api/admin/links/report",
		"/api/admin/server",
		"/api/admin/support",
		"/api/editor/pages/ko/C",
	}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401 for %s, got %d: %s", path, rec.Code, rec.Body.String())
		}
	}
	removedReq := httptest.NewRequest(http.MethodGet, "/api/admin/bots/naming/rules", nil)
	removedRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(removedRec, removedReq)
	if removedRec.Code != http.StatusNotFound {
		t.Fatalf("expected removed naming rule API to return 404, got %d: %s", removedRec.Code, removedRec.Body.String())
	}
	policyReq := httptest.NewRequest(http.MethodPatch, "/api/admin/users/regular-permissions", strings.NewReader(`{"permissions":["edit"]}`))
	policyRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(policyRec, policyReq)
	if policyRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for regular user policy, got %d: %s", policyRec.Code, policyRec.Body.String())
	}
	portalReq := httptest.NewRequest(http.MethodPatch, "/api/admin/languages/ko/portal", strings.NewReader(`{"portal_order":1,"country_codes":["KR"]}`))
	portalRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(portalRec, portalReq)
	if portalRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for language portal metadata, got %d: %s", portalRec.Code, portalRec.Body.String())
	}
	techdataLinkReq := httptest.NewRequest(http.MethodPost, "/api/admin/pages/ko/C/techdata", strings.NewReader(`{}`))
	techdataLinkRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(techdataLinkRec, techdataLinkReq)
	if techdataLinkRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for document techdata link, got %d: %s", techdataLinkRec.Code, techdataLinkRec.Body.String())
	}
}

func TestResolveLinksDoesNotExposeStorageKeys(t *testing.T) {
	srv := New(config.Config{}, emptyStore{}, emptyAuth{}, techmark.NewParser())
	req := httptest.NewRequest(http.MethodGet, "/api/links/resolve?locale=ko&target=%EC%8B%9C%EB%B6%84%ED%95%A0_%EC%8B%9C%EC%8A%A4%ED%85%9C&target=Compatible+Time-Sharing+System", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "slug") || strings.Contains(rec.Body.String(), "revision") || strings.Contains(rec.Body.String(), "group") {
		t.Fatalf("link response leaked internal fields: %s", rec.Body.String())
	}
	var response publicLinkResolveResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode link response: %v", err)
	}
	if len(response.Links) != 2 {
		t.Fatalf("expected two resolved links, got %#v", response.Links)
	}
	want := []publicResolvedLink{
		{Locale: "ko", Target: "시분할 시스템", Name: "시분할 시스템", URL: "/ko/wiki/시분할_시스템", Exists: false},
		{Locale: "ko", Target: "Compatible Time-Sharing System", Name: "Compatible Time-Sharing System", URL: "/ko/wiki/Compatible_Time-Sharing_System", Exists: false},
	}
	for index := range want {
		if response.Links[index] != want[index] {
			t.Fatalf("link %d = %#v, want %#v", index, response.Links[index], want[index])
		}
	}
}

func TestResolveLinksKeepsEveryRequestedTargetInOrder(t *testing.T) {
	srv := New(config.Config{}, emptyStore{}, emptyAuth{}, techmark.NewParser())
	req := httptest.NewRequest(http.MethodGet, "/api/links/resolve?locale=ko&target=Pascal&target=pascal&target=C+%28%ED%94%84%EB%A1%9C%EA%B7%B8%EB%9E%98%EB%B0%8D+%EC%96%B8%EC%96%B4%29&target=Simula", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response publicLinkResolveResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode link response: %v", err)
	}
	wantTargets := []string{"Pascal", "pascal", "C (프로그래밍 언어)", "Simula"}
	wantNames := []string{"Pascal", "Pascal", "C (프로그래밍 언어)", "Simula"}
	wantURLs := []string{"/ko/wiki/Pascal", "/ko/wiki/Pascal", "/ko/wiki/C_(프로그래밍_언어)", "/ko/wiki/Simula"}
	if len(response.Links) != len(wantTargets) {
		t.Fatalf("links = %#v, want one result per target", response.Links)
	}
	for index, target := range wantTargets {
		if response.Links[index].Target != target || response.Links[index].Name != wantNames[index] || response.Links[index].URL != wantURLs[index] {
			t.Fatalf("link %d = %#v, want target %q, name %q and URL %q", index, response.Links[index], target, wantNames[index], wantURLs[index])
		}
	}
}

func TestWikiRouteRedirectsToStoredTitleCasing(t *testing.T) {
	doc := document.Document{Locale: "en", Slug: "programming-language", Title: "Programming language", Source: "# Programming language", Revision: "r1"}
	srv := New(config.Config{}, revisionStore{emptyStore: emptyStore{}, doc: doc}, emptyAuth{}, techmark.NewParser())

	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/en/wiki/Programming_Language", nil))
	if recorder.Code != http.StatusPermanentRedirect || recorder.Header().Get("Location") != "/en/wiki/Programming_language" {
		t.Fatalf("title casing redirect = %d location=%q", recorder.Code, recorder.Header().Get("Location"))
	}
}

func TestWikiRouteDoesNotRedirectEquivalentPathEscaping(t *testing.T) {
	doc := document.Document{Locale: "ko", Slug: "c-프로그래밍-언어", Title: "C (프로그래밍 언어)", Source: "# C (프로그래밍 언어)", Revision: "r1"}
	srv := New(config.Config{}, revisionStore{emptyStore: emptyStore{}, doc: doc}, emptyAuth{}, techmark.NewParser())

	paths := []string{
		"/ko/wiki/C_(%ED%94%84%EB%A1%9C%EA%B7%B8%EB%9E%98%EB%B0%8D_%EC%96%B8%EC%96%B4)",
		"/ko/wiki/C_%28%ed%94%84%eb%a1%9c%ea%b7%b8%eb%9e%98%eb%b0%8d_%ec%96%b8%ec%96%b4%29",
	}
	for _, path := range paths {
		recorder := httptest.NewRecorder()
		srv.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK || recorder.Header().Get("Location") != "" {
			t.Fatalf("equivalent escaped path %q redirected: status=%d location=%q", path, recorder.Code, recorder.Header().Get("Location"))
		}
	}
}

func TestPageTranslationsFollowTechdataWikiPages(t *testing.T) {
	store, err := storage.OpenBadgerDocumentStore(t.TempDir())
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
	if _, err := store.CreateItem(ctx, techdata.Item{
		Labels:    map[string]string{"ko": "라이선스", "en": "License"},
		WikiPages: map[string]document.WID{"ko": ko.ID, "en": en.ID},
	}); err != nil {
		t.Fatalf("create techdata item: %v", err)
	}
	srv := New(config.Config{}, store, emptyAuth{}, techmark.NewParser())
	translations, err := srv.translationsForPage(ctx, ko)
	if err != nil {
		t.Fatalf("load translations: %v", err)
	}
	if len(translations) != 2 || translations[0].Locale != "en" || translations[0].Title != "License" || translations[1].Locale != "ko" {
		t.Fatalf("unexpected Techdata translations: %#v", translations)
	}
	response := newPublicPageResponse(ko, translations)
	if len(response.Languages) != 2 || response.Languages[0].URL != "/en/wiki/License" {
		t.Fatalf("public language links did not use Techdata documents: %#v", response.Languages)
	}
}

func TestSignupCreatesRegularUserSessionWithoutInternalUserFields(t *testing.T) {
	accounts, err := auth.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	defer accounts.Close()
	srv := New(config.Config{PublicURL: "https://techpedia.example"}, emptyStore{}, accounts, techmark.NewParser())

	body := `{"username":"editor","display_name":"편집자","password":"long-enough-password"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/signup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"id"`) || strings.Contains(rec.Body.String(), "password") {
		t.Fatalf("signup response leaked internal account fields: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"permissions":[]`) {
		t.Fatalf("signup account did not use the default regular-user policy: %s", rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName || !cookies[0].HttpOnly || !cookies[0].Secure {
		t.Fatalf("expected HttpOnly Secure session cookie, got %#v", cookies)
	}

	sessionReq := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	sessionReq.AddCookie(cookies[0])
	sessionRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(sessionRec, sessionReq)
	if sessionRec.Code != http.StatusOK {
		t.Fatalf("expected authenticated session, got %d: %s", sessionRec.Code, sessionRec.Body.String())
	}
}

func TestAdminRegularUserPolicyUpdatesExistingAndFutureAccounts(t *testing.T) {
	accounts, err := auth.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	defer accounts.Close()
	ctx := context.Background()
	if err := accounts.EnsureBootstrapAdmin(ctx, "policy_owner", "long-owner-password"); err != nil {
		t.Fatalf("create bootstrap owner: %v", err)
	}
	var owner auth.User
	users, err := accounts.ListUsers(ctx)
	if err != nil || len(users) != 1 {
		t.Fatalf("list bootstrap owner: %#v, %v", users, err)
	}
	owner = users[0]
	admin, err := accounts.Register(ctx, auth.Registration{
		Username: "policy_admin", Password: "long-admin-password", Permissions: []auth.Permission{auth.PermissionAdmin},
	})
	if err != nil {
		t.Fatalf("register delegated administrator: %v", err)
	}
	regular, err := accounts.Register(ctx, auth.Registration{
		Username: "regular_user", Password: "long-regular-password", UseRegularPermissions: true,
	})
	if err != nil {
		t.Fatalf("register regular user: %v", err)
	}
	_, ownerToken, _, err := accounts.Login(ctx, owner.Username, "long-owner-password")
	if err != nil {
		t.Fatalf("login owner: %v", err)
	}
	srv := New(config.Config{}, emptyStore{}, accounts, techmark.NewParser())

	policyReq := httptest.NewRequest(http.MethodPatch, "/api/admin/users/regular-permissions", strings.NewReader(`{"permissions":["edit","techdata"]}`))
	policyReq.Header.Set("Content-Type", "application/json")
	policyReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: ownerToken})
	policyRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(policyRec, policyReq)
	if policyRec.Code != http.StatusOK || !strings.Contains(policyRec.Body.String(), `"updated_users":1`) {
		t.Fatalf("update regular user policy: %d: %s", policyRec.Code, policyRec.Body.String())
	}

	users, err = accounts.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	for _, user := range users {
		switch user.ID {
		case owner.ID:
			if !user.Has(auth.PermissionOwner) {
				t.Fatalf("regular policy changed owner: %#v", user)
			}
		case admin.ID:
			if len(user.Permissions) != 1 || user.Permissions[0] != auth.PermissionAdmin {
				t.Fatalf("regular policy changed administrator: %#v", user)
			}
		case regular.ID:
			if !user.Has(auth.PermissionEdit) || !user.Has(auth.PermissionTechdata) {
				t.Fatalf("regular policy did not update existing user: %#v", user)
			}
		}
	}

	signupReq := httptest.NewRequest(http.MethodPost, "/api/auth/signup", strings.NewReader(`{"username":"future_user","password":"long-future-password"}`))
	signupReq.Header.Set("Content-Type", "application/json")
	signupRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(signupRec, signupReq)
	if signupRec.Code != http.StatusCreated || !strings.Contains(signupRec.Body.String(), `"edit"`) || !strings.Contains(signupRec.Body.String(), `"techdata"`) {
		t.Fatalf("future signup did not inherit regular policy: %d: %s", signupRec.Code, signupRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	listReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: ownerToken})
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK || !strings.Contains(listRec.Body.String(), owner.Username) || !strings.Contains(listRec.Body.String(), admin.Username) || !strings.Contains(listRec.Body.String(), `"regular_permissions":["edit","techdata"]`) {
		t.Fatalf("admin user listing did not expose all users and regular policy: %d: %s", listRec.Code, listRec.Body.String())
	}

	promoteReq := httptest.NewRequest(http.MethodPatch, "/api/admin/users/"+regular.ID+"/permissions", strings.NewReader(`{"permissions":["admin"]}`))
	promoteReq.Header.Set("Content-Type", "application/json")
	promoteReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: ownerToken})
	promoteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(promoteRec, promoteReq)
	if promoteRec.Code != http.StatusOK || !strings.Contains(promoteRec.Body.String(), `"admin"`) {
		t.Fatalf("promote regular user: %d: %s", promoteRec.Code, promoteRec.Body.String())
	}

	demoteReq := httptest.NewRequest(http.MethodPatch, "/api/admin/users/"+regular.ID+"/permissions", strings.NewReader(`{"permissions":[]}`))
	demoteReq.Header.Set("Content-Type", "application/json")
	demoteReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: ownerToken})
	demoteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(demoteRec, demoteReq)
	if demoteRec.Code != http.StatusOK || strings.Contains(demoteRec.Body.String(), `"admin"`) || !strings.Contains(demoteRec.Body.String(), `"edit"`) {
		t.Fatalf("demote administrator to regular policy: %d: %s", demoteRec.Code, demoteRec.Body.String())
	}

	_, adminToken, _, err := accounts.Login(ctx, admin.Username, "long-admin-password")
	if err != nil {
		t.Fatalf("login delegated administrator: %v", err)
	}
	delegatedPromoteReq := httptest.NewRequest(http.MethodPatch, "/api/admin/users/"+regular.ID+"/permissions", strings.NewReader(`{"permissions":["admin"]}`))
	delegatedPromoteReq.Header.Set("Content-Type", "application/json")
	delegatedPromoteReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	delegatedPromoteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(delegatedPromoteRec, delegatedPromoteReq)
	if delegatedPromoteRec.Code != http.StatusOK {
		t.Fatalf("delegated administrator could not promote user: %d: %s", delegatedPromoteRec.Code, delegatedPromoteRec.Body.String())
	}

	selfDemoteReq := httptest.NewRequest(http.MethodPatch, "/api/admin/users/"+admin.ID+"/permissions", strings.NewReader(`{"permissions":[]}`))
	selfDemoteReq.Header.Set("Content-Type", "application/json")
	selfDemoteReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	selfDemoteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(selfDemoteRec, selfDemoteReq)
	if selfDemoteRec.Code != http.StatusConflict {
		t.Fatalf("administrator removed own role: %d: %s", selfDemoteRec.Code, selfDemoteRec.Body.String())
	}
}

func TestPublicUserProfileAndAuthenticatedProfileUpdate(t *testing.T) {
	accounts, err := auth.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	defer accounts.Close()
	ctx := context.Background()
	user, err := accounts.Register(ctx, auth.Registration{
		Username: "profile_user", DisplayName: "Profile User", Password: "long-profile-password", Permissions: []auth.Permission{auth.PermissionEdit},
	})
	if err != nil {
		t.Fatalf("register profile user: %v", err)
	}
	_, token, _, err := accounts.Login(ctx, user.Username, "long-profile-password")
	if err != nil {
		t.Fatalf("login profile user: %v", err)
	}
	srv := New(config.Config{PublicURL: "https://techpedia.example"}, emptyStore{}, accounts, techmark.NewParser())

	unauthorizedReq := httptest.NewRequest(http.MethodPatch, "/api/account/profile", strings.NewReader(`{"display_name":"Changed","bio":"No session"}`))
	unauthorizedReq.Header.Set("Content-Type", "application/json")
	unauthorizedRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unauthorizedRec, unauthorizedReq)
	if unauthorizedRec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated profile update: %d: %s", unauthorizedRec.Code, unauthorizedRec.Body.String())
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/account/profile", strings.NewReader(`{"display_name":"Compiler Editor","bio":"Writes about compilers.\nInterested in language design."}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	updateRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK || !strings.Contains(updateRec.Body.String(), `"display_name":"Compiler Editor"`) || !strings.Contains(updateRec.Body.String(), `"bio":"Writes about compilers.\nInterested in language design."`) {
		t.Fatalf("update own profile: %d: %s", updateRec.Code, updateRec.Body.String())
	}

	profileReq := httptest.NewRequest(http.MethodGet, "/api/users/profile_user", nil)
	profileRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(profileRec, profileReq)
	if profileRec.Code != http.StatusOK || !strings.Contains(profileRec.Body.String(), `"username":"profile_user"`) || strings.Contains(profileRec.Body.String(), `"permissions"`) || strings.Contains(profileRec.Body.String(), `"id"`) {
		t.Fatalf("public profile response: %d: %s", profileRec.Code, profileRec.Body.String())
	}

	canonicalReq := httptest.NewRequest(http.MethodGet, "/user/Profile_User", nil)
	canonicalRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(canonicalRec, canonicalReq)
	if canonicalRec.Code != http.StatusPermanentRedirect || canonicalRec.Header().Get("Location") != "/user/profile_user" {
		t.Fatalf("canonical profile redirect: %d: %s", canonicalRec.Code, canonicalRec.Header().Get("Location"))
	}
	pageReq := httptest.NewRequest(http.MethodGet, "/user/profile_user", nil)
	pageRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(pageRec, pageReq)
	if pageRec.Code != http.StatusOK || !strings.Contains(pageRec.Body.String(), `Compiler Editor (@profile_user)`) || pageRec.Header().Get("X-Robots-Tag") != "noindex,follow" {
		t.Fatalf("profile page shell: %d: %s", pageRec.Code, pageRec.Body.String())
	}
	missingReq := httptest.NewRequest(http.MethodGet, "/user/missing_user", nil)
	missingRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("missing profile page: %d: %s", missingRec.Code, missingRec.Body.String())
	}
}

func TestPublicRevisionResponseUsesWikiRevisionLabels(t *testing.T) {
	doc := document.Document{
		Locale:    "ko",
		Slug:      "c",
		Title:     "C",
		HTML:      "<h1>C</h1>",
		Author:    "editor",
		Summary:   "설명 보강",
		Revision:  "r2",
		UpdatedAt: time.Date(2026, 7, 14, 1, 2, 3, 0, time.UTC),
	}
	store := revisionStore{emptyStore: emptyStore{}, doc: doc}
	srv := New(config.Config{}, store, emptyAuth{}, techmark.NewParser())

	req := httptest.NewRequest(http.MethodGet, "/api/pages/ko/C/revisions", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"revision":"r2"`) {
		t.Fatalf("expected public r2 label: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "slug") || strings.Contains(rec.Body.String(), "group") {
		t.Fatalf("revision response leaked storage fields: %s", rec.Body.String())
	}
}

func TestTechnicalPunctuationSurvivesDocumentURLs(t *testing.T) {
	store, err := storage.OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open document store: %v", err)
	}
	defer store.Close()
	configureTestEdition(t, store, "ko", "대문")

	title := "C++/#* Toolkit"
	if _, err := store.Save(context.Background(), document.Document{
		Locale: "ko", Slug: techmark.Slugify(title), Title: title,
		Source: "# C++/#* Toolkit\n\n특수문자 문서입니다.", HTML: "<h1>C++/#* Toolkit</h1>",
	}); err != nil {
		t.Fatalf("save punctuation document: %v", err)
	}
	srv := New(config.Config{}, store, emptyAuth{}, techmark.NewParser())
	encodedName := "C++%2F%23*_Toolkit"

	apiReq := httptest.NewRequest(http.MethodGet, "/api/pages/ko/"+encodedName, nil)
	apiRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(apiRec, apiReq)
	if apiRec.Code != http.StatusOK || !strings.Contains(apiRec.Body.String(), `"name":"C++/#* Toolkit"`) || !strings.Contains(apiRec.Body.String(), `"url":"/ko/wiki/C++%2F%23*_Toolkit"`) {
		t.Fatalf("special document API failed: %d: %s", apiRec.Code, apiRec.Body.String())
	}

	historyReq := httptest.NewRequest(http.MethodGet, "/api/pages/ko/"+encodedName+"/revisions", nil)
	historyRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(historyRec, historyReq)
	if historyRec.Code != http.StatusOK {
		t.Fatalf("special document history failed: %d: %s", historyRec.Code, historyRec.Body.String())
	}

	pageReq := httptest.NewRequest(http.MethodGet, "/ko/wiki/"+encodedName, nil)
	pageRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(pageRec, pageReq)
	if pageRec.Code != http.StatusOK {
		t.Fatalf("special document page failed: %d: %s", pageRec.Code, pageRec.Body.String())
	}

	for _, route := range []string{"/edit/ko/" + encodedName, "/history/ko/" + encodedName} {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("special document shell route %s failed: %d: %s", route, rec.Code, rec.Body.String())
		}
	}
}

func configuredMessages() map[string]string {
	messages := make(map[string]string, len(edition.InterfaceMessageKeys()))
	for _, key := range edition.InterfaceMessageKeys() {
		messages[key] = "configured " + key
	}
	return messages
}

func configureTestEdition(t *testing.T, store *storage.BadgerDocumentStore, tag string, mainPageName string) {
	t.Helper()
	name, nativeName, direction, ok := edition.DescribeLanguage(tag)
	if !ok {
		t.Fatalf("describe test edition %s", tag)
	}
	if _, err := store.ProposeEdition(context.Background(), edition.Edition{Tag: tag, Name: name, NativeName: nativeName, Direction: direction, Status: edition.StatusPending}); err != nil {
		t.Fatalf("propose test edition: %v", err)
	}
	if _, err := store.ApproveEdition(context.Background(), tag, "test_administrator", time.Now()); err != nil {
		t.Fatalf("approve test edition: %v", err)
	}
	if _, err := store.UpdateInterfaceMessages(context.Background(), tag, configuredMessages()); err != nil {
		t.Fatalf("configure test interface: %v", err)
	}
	navigation := []edition.NavigationSection{{Title: "Navigation", Items: []edition.NavigationItem{{Label: mainPageName, Target: mainPageName}}}}
	if _, err := store.UpdateNavigation(context.Background(), tag, navigation); err != nil {
		t.Fatalf("configure test navigation: %v", err)
	}
	if _, err := store.ActivateEdition(context.Background(), tag, mainPageName, time.Now()); err != nil {
		t.Fatalf("activate test edition: %v", err)
	}
}

func TestStoredSourceIsRenderedWithCurrentLinkRules(t *testing.T) {
	doc := document.Document{
		Locale: "ko", Group: "c-list", Slug: "c-list", Title: "C 목록",
		Source: "# C 목록\n\n- [[C++]]", HTML: `<h1>C 목록</h1><a href="/ko/wiki/c-plus-plus">C++</a>`,
		Revision: "r1", UpdatedAt: time.Now().UTC(),
	}
	srv := New(config.Config{}, revisionStore{emptyStore: emptyStore{}, doc: doc}, emptyAuth{}, techmark.NewParser())
	req := httptest.NewRequest(http.MethodGet, "/api/pages/ko/C_목록", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `/ko/wiki/C++`) || strings.Contains(rec.Body.String(), "c-plus-plus") {
		t.Fatalf("expected current C++ link rendering, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestNativeDiscussionBoardUsesTechPediaAccounts(t *testing.T) {
	store, err := storage.OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open document store: %v", err)
	}
	defer store.Close()
	configureTestEdition(t, store, "ko", "대문")
	if _, err := store.Save(context.Background(), document.Document{Locale: "ko", Slug: techmark.Slugify("C"), Title: "C", Source: "# C"}); err != nil {
		t.Fatalf("save discussion document: %v", err)
	}

	accounts, err := auth.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open accounts: %v", err)
	}
	defer accounts.Close()
	if _, err := accounts.Register(context.Background(), auth.Registration{Username: "discuss_user", Password: "long-user-password"}); err != nil {
		t.Fatalf("register user: %v", err)
	}
	if _, err := accounts.Register(context.Background(), auth.Registration{Username: "discuss_admin", Password: "long-admin-password", Permissions: []auth.Permission{auth.PermissionAdmin}}); err != nil {
		t.Fatalf("register admin: %v", err)
	}
	_, userToken, _, _ := accounts.Login(context.Background(), "discuss_user", "long-user-password")
	_, adminToken, _, _ := accounts.Login(context.Background(), "discuss_admin", "long-admin-password")
	srv := New(config.Config{}, store, accounts, techmark.NewParser())

	publicReq := httptest.NewRequest(http.MethodGet, "/api/discussions/ko/C/topics", nil)
	publicRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(publicRec, publicReq)
	if publicRec.Code != http.StatusOK || publicRec.Body.String() != "{\"topics\":[]}\n" {
		t.Fatalf("public discussion list: %d: %s", publicRec.Code, publicRec.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/discussions/ko/C/topics", strings.NewReader(`{"subject":"표준 범위","body":"C17까지 다룹니까?"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: userToken})
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated || !strings.Contains(createRec.Body.String(), `"subject":"표준 범위"`) || strings.Contains(createRec.Body.String(), "author_id") {
		t.Fatalf("create discussion topic: %d: %s", createRec.Code, createRec.Body.String())
	}
	var created discussionTopicResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode topic: %v", err)
	}

	replyReq := httptest.NewRequest(http.MethodPost, "/api/discussions/topics/"+created.ID+"/replies", strings.NewReader(`{"body":"C23도 언급합시다."}`))
	replyReq.Header.Set("Content-Type", "application/json")
	replyReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: userToken})
	replyRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(replyRec, replyReq)
	if replyRec.Code != http.StatusCreated {
		t.Fatalf("create reply: %d: %s", replyRec.Code, replyRec.Body.String())
	}

	archiveReq := httptest.NewRequest(http.MethodPost, "/api/discussions/topics/"+created.ID+"/archive", nil)
	archiveReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: userToken})
	archiveRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(archiveRec, archiveReq)
	if archiveRec.Code != http.StatusOK || !strings.Contains(archiveRec.Body.String(), `"archived":true`) || !strings.Contains(archiveRec.Body.String(), `"can_archive":false`) {
		t.Fatalf("archive discussion: %d: %s", archiveRec.Code, archiveRec.Body.String())
	}
	if strings.Contains(archiveRec.Body.String(), "archived_by") {
		t.Fatalf("archive response leaked internal user id: %s", archiveRec.Body.String())
	}
	lateReplyReq := httptest.NewRequest(http.MethodPost, "/api/discussions/topics/"+created.ID+"/replies", strings.NewReader(`{"body":"late reply"}`))
	lateReplyReq.Header.Set("Content-Type", "application/json")
	lateReplyReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: userToken})
	lateReplyRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(lateReplyRec, lateReplyReq)
	if lateReplyRec.Code != http.StatusConflict {
		t.Fatalf("archived discussion accepted reply: %d: %s", lateReplyRec.Code, lateReplyRec.Body.String())
	}
	ownerDeleteReq := httptest.NewRequest(http.MethodDelete, "/api/discussions/topics/"+created.ID, nil)
	ownerDeleteReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: userToken})
	ownerDeleteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(ownerDeleteRec, ownerDeleteReq)
	if ownerDeleteRec.Code != http.StatusForbidden {
		t.Fatalf("topic owner deleted archived discussion: %d: %s", ownerDeleteRec.Code, ownerDeleteRec.Body.String())
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/api/admin/discussions/topics?q=C17", nil)
	adminReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	adminRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(adminRec, adminReq)
	if adminRec.Code != http.StatusOK || !strings.Contains(adminRec.Body.String(), `"count":1`) {
		t.Fatalf("admin discussion search: %d: %s", adminRec.Code, adminRec.Body.String())
	}
	adminDeleteReq := httptest.NewRequest(http.MethodDelete, "/api/admin/discussions/topics/"+created.ID, nil)
	adminDeleteReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	adminDeleteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(adminDeleteRec, adminDeleteReq)
	if adminDeleteRec.Code != http.StatusNoContent {
		t.Fatalf("administrator could not delete archived discussion: %d: %s", adminDeleteRec.Code, adminDeleteRec.Body.String())
	}
}

func TestAdministratorOnlyDocumentPolicyIsEnforced(t *testing.T) {
	store, err := storage.OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open document store: %v", err)
	}
	defer store.Close()
	configureTestEdition(t, store, "ko", "대문")
	configureTestEdition(t, store, "en", "Main Page")

	accounts, err := auth.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open account store: %v", err)
	}
	defer accounts.Close()
	if _, err := accounts.Register(context.Background(), auth.Registration{
		Username: "policy_admin", Password: "long-admin-password", Permissions: []auth.Permission{auth.PermissionAdmin},
	}); err != nil {
		t.Fatalf("register administrator: %v", err)
	}
	if _, err := accounts.Register(context.Background(), auth.Registration{
		Username: "policy_editor", Password: "long-editor-password", Permissions: []auth.Permission{auth.PermissionEdit},
	}); err != nil {
		t.Fatalf("register editor: %v", err)
	}
	_, adminToken, _, err := accounts.Login(context.Background(), "policy_admin", "long-admin-password")
	if err != nil {
		t.Fatalf("login administrator: %v", err)
	}
	_, editorToken, _, err := accounts.Login(context.Background(), "policy_editor", "long-editor-password")
	if err != nil {
		t.Fatalf("login editor: %v", err)
	}

	if _, err := store.Save(context.Background(), document.Document{
		Locale: "ko", Slug: techmark.Slugify("Protected"), Title: "Protected", Source: "# Protected", HTML: "<h1>Protected</h1>",
		EditPolicy: document.EditPolicyAdmins,
	}); err != nil {
		t.Fatalf("save protected document: %v", err)
	}
	srv := New(config.Config{}, store, accounts, techmark.NewParser())

	publicReq := httptest.NewRequest(http.MethodGet, "/api/pages/ko/Protected", nil)
	publicRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(publicRec, publicReq)
	if publicRec.Code != http.StatusOK || !strings.Contains(publicRec.Body.String(), `"edit_policy":"admins"`) {
		t.Fatalf("public response omitted edit policy: %d: %s", publicRec.Code, publicRec.Body.String())
	}

	editorGetReq := httptest.NewRequest(http.MethodGet, "/api/editor/pages/ko/Protected", nil)
	editorGetReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: editorToken})
	editorGetRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(editorGetRec, editorGetReq)
	if editorGetRec.Code != http.StatusForbidden {
		t.Fatalf("regular editor opened protected source: %d: %s", editorGetRec.Code, editorGetRec.Body.String())
	}

	editorSaveReq := httptest.NewRequest(http.MethodPut, "/api/editor/pages/ko/Protected", strings.NewReader(`{"name":"Protected","source":"# changed","edit_policy":"editors"}`))
	editorSaveReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: editorToken})
	editorSaveRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(editorSaveRec, editorSaveReq)
	if editorSaveRec.Code != http.StatusForbidden {
		t.Fatalf("regular editor changed protected document: %d: %s", editorSaveRec.Code, editorSaveRec.Body.String())
	}

	adminSaveReq := httptest.NewRequest(http.MethodPut, "/api/editor/pages/ko/Protected", strings.NewReader(`{"name":"Protected","source":"# Protected\n\nUpdated.","edit_policy":"editors","base_revision":"r1"}`))
	adminSaveReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	adminSaveRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(adminSaveRec, adminSaveReq)
	if adminSaveRec.Code != http.StatusOK || !strings.Contains(adminSaveRec.Body.String(), `"edit_policy":"editors"`) {
		t.Fatalf("administrator could not remove protection: %d: %s", adminSaveRec.Code, adminSaveRec.Body.String())
	}

	editorProtectReq := httptest.NewRequest(http.MethodPut, "/api/editor/pages/ko/Protected", strings.NewReader(`{"name":"Protected","source":"# Protected","edit_policy":"admins","base_revision":"r2"}`))
	editorProtectReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: editorToken})
	editorProtectRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(editorProtectRec, editorProtectReq)
	if editorProtectRec.Code != http.StatusForbidden {
		t.Fatalf("regular editor changed edit policy: %d: %s", editorProtectRec.Code, editorProtectRec.Body.String())
	}

	specialSource := "# Special\n\n<wiki-recent-changes limit=\"20\" empty=\"No changes\"></wiki-recent-changes>\n\n<wiki-random-page label=\"Open random page\"></wiki-random-page>"
	editorPreviewBody, _ := json.Marshal(map[string]string{"locale": "ko", "source": specialSource})
	editorPreviewReq := httptest.NewRequest(http.MethodPost, "/api/editor/preview", strings.NewReader(string(editorPreviewBody)))
	editorPreviewReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: editorToken})
	editorPreviewRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(editorPreviewRec, editorPreviewReq)
	if editorPreviewRec.Code != http.StatusForbidden {
		t.Fatalf("regular editor previewed special syntax: %d: %s", editorPreviewRec.Code, editorPreviewRec.Body.String())
	}

	codePreviewBody, _ := json.Marshal(map[string]string{
		"locale": "ko", "name": "Code example",
		"source": "```c id=\"hfvq7t\"\nint main(void) { return 0; }\n```",
	})
	codePreviewReq := httptest.NewRequest(http.MethodPost, "/api/editor/preview", strings.NewReader(string(codePreviewBody)))
	codePreviewReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: editorToken})
	codePreviewRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(codePreviewRec, codePreviewReq)
	var codePreview map[string]string
	if codePreviewRec.Code != http.StatusOK || json.NewDecoder(codePreviewRec.Body).Decode(&codePreview) != nil ||
		!strings.Contains(codePreview["html"], `<pre id="hfvq7t" class="code-block"><code class="syntax-chroma language-c">`) {
		t.Fatalf("code fence id preview failed: %d: %s", codePreviewRec.Code, codePreviewRec.Body.String())
	}

	editorSpecialBody, _ := json.Marshal(map[string]string{"name": "Special", "source": specialSource, "edit_policy": "editors"})
	editorSpecialReq := httptest.NewRequest(http.MethodPut, "/api/editor/pages/ko/Special", strings.NewReader(string(editorSpecialBody)))
	editorSpecialReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: editorToken})
	editorSpecialRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(editorSpecialRec, editorSpecialReq)
	if editorSpecialRec.Code != http.StatusForbidden {
		t.Fatalf("regular editor saved special syntax: %d: %s", editorSpecialRec.Code, editorSpecialRec.Body.String())
	}

	adminSpecialBody, _ := json.Marshal(map[string]string{"name": "Special", "source": specialSource, "edit_policy": "admins", "summary": "Add special page"})
	adminSpecialReq := httptest.NewRequest(http.MethodPut, "/api/editor/pages/ko/Special", strings.NewReader(string(adminSpecialBody)))
	adminSpecialReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	adminSpecialRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(adminSpecialRec, adminSpecialReq)
	if adminSpecialRec.Code != http.StatusOK {
		t.Fatalf("administrator could not save special page: %d: %s", adminSpecialRec.Code, adminSpecialRec.Body.String())
	}

	publicSpecialReq := httptest.NewRequest(http.MethodGet, "/api/pages/ko/Special", nil)
	publicSpecialRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(publicSpecialRec, publicSpecialReq)
	for _, expected := range []string{`special-change-list`, `techpedia-time`, `/ko/special/random`, `Open random page`} {
		if publicSpecialRec.Code != http.StatusOK || !strings.Contains(publicSpecialRec.Body.String(), expected) {
			t.Fatalf("special page did not render %q: %d: %s", expected, publicSpecialRec.Code, publicSpecialRec.Body.String())
		}
	}

	randomReq := httptest.NewRequest(http.MethodGet, "/ko/special/random", nil)
	randomRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(randomRec, randomReq)
	if randomRec.Code != http.StatusTemporaryRedirect || randomRec.Header().Get("Location") != "/ko/wiki/Protected" {
		t.Fatalf("random page did not exclude special documents: %d location=%q", randomRec.Code, randomRec.Header().Get("Location"))
	}

	unprotectSpecialBody, _ := json.Marshal(map[string]string{"name": "Special", "source": specialSource, "edit_policy": "editors"})
	unprotectSpecialReq := httptest.NewRequest(http.MethodPut, "/api/editor/pages/ko/Special", strings.NewReader(string(unprotectSpecialBody)))
	unprotectSpecialReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	unprotectSpecialRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unprotectSpecialRec, unprotectSpecialReq)
	if unprotectSpecialRec.Code != http.StatusBadRequest {
		t.Fatalf("special document protection was removed: %d: %s", unprotectSpecialRec.Code, unprotectSpecialRec.Body.String())
	}

	if _, err := store.Save(context.Background(), document.Document{
		Locale: "ko", Slug: techmark.Slugify("C"), Title: "C", Source: "# C\n\n[[대문]] [[없는 링크]]", EditPolicy: document.EditPolicyEditors,
	}); err != nil {
		t.Fatalf("save homepage candidate: %v", err)
	}
	for index := 0; index < 3; index++ {
		if err := store.RecordPageView(context.Background(), "ko", techmark.Slugify("C"), time.Now().UTC()); err != nil {
			t.Fatalf("record popular page view: %v", err)
		}
	}
	homeSource := "# 대문\n\n<wiki-home-grid>\n<wiki-featured-pages title=\"오늘의 문서\" limit=\"2\" empty=\"문서 없음\"></wiki-featured-pages>\n<wiki-popular-pages title=\"많이 본 문서\" period=\"7d\" limit=\"5\" empty=\"조회 없음\"></wiki-popular-pages>\n</wiki-home-grid>"
	homeBody, _ := json.Marshal(map[string]string{"name": "대문", "source": homeSource, "edit_policy": "admins"})
	homeReq := httptest.NewRequest(http.MethodPut, "/api/editor/pages/ko/대문", strings.NewReader(string(homeBody)))
	homeReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	homeRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(homeRec, homeReq)
	if homeRec.Code != http.StatusOK {
		t.Fatalf("administrator could not save homepage panels: %d: %s", homeRec.Code, homeRec.Body.String())
	}

	publicHomeReq := httptest.NewRequest(http.MethodGet, "/api/pages/ko/대문", nil)
	publicHomeRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(publicHomeRec, publicHomeReq)
	for _, expected := range []string{`wiki-home-grid`, `wiki-home-panel-featured`, `오늘의 문서`, `wiki-home-panel-popular`, `많이 본 문서`, `/ko/wiki/C`} {
		if publicHomeRec.Code != http.StatusOK || !strings.Contains(publicHomeRec.Body.String(), expected) {
			t.Fatalf("homepage panels did not render %q: %d: %s", expected, publicHomeRec.Code, publicHomeRec.Body.String())
		}
	}
	backlinksReq := httptest.NewRequest(http.MethodGet, "/api/pages/ko/대문/backlinks", nil)
	backlinksRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(backlinksRec, backlinksReq)
	if backlinksRec.Code != http.StatusOK || !strings.Contains(backlinksRec.Body.String(), `"name":"C"`) {
		t.Fatalf("public backlinks did not include C: %d: %s", backlinksRec.Code, backlinksRec.Body.String())
	}
	linkReportReq := httptest.NewRequest(http.MethodGet, "/api/admin/links/report?locale=ko", nil)
	linkReportReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	linkReportRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(linkReportRec, linkReportReq)
	if linkReportRec.Code != http.StatusOK || !strings.Contains(linkReportRec.Body.String(), `"name":"없는 링크"`) {
		t.Fatalf("administrator link report omitted a missing link: %d: %s", linkReportRec.Code, linkReportRec.Body.String())
	}
	serverInfoReq := httptest.NewRequest(http.MethodGet, "/api/admin/server", nil)
	serverInfoReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	serverInfoRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(serverInfoRec, serverInfoReq)
	if serverInfoRec.Code != http.StatusOK || !strings.Contains(serverInfoRec.Body.String(), `"engine_version":"`+buildinfo.Version+`"`) {
		t.Fatalf("administrator server information failed: %d: %s", serverInfoRec.Code, serverInfoRec.Body.String())
	}
	supportBody := `{"enabled":true,"title":"TechPedia 후원","description":"공식 암호화폐 주소입니다.","wallets":[{"asset":"BTC","network":"Bitcoin","address":"bc1qexampleaddress"}]}`
	supportSaveReq := httptest.NewRequest(http.MethodPut, "/api/admin/support", strings.NewReader(supportBody))
	supportSaveReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	supportSaveRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(supportSaveRec, supportSaveReq)
	if supportSaveRec.Code != http.StatusOK || !strings.Contains(supportSaveRec.Body.String(), `"enabled":true`) {
		t.Fatalf("administrator could not publish support page: %d: %s", supportSaveRec.Code, supportSaveRec.Body.String())
	}
	supportPageReq := httptest.NewRequest(http.MethodGet, "/support", nil)
	supportPageRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(supportPageRec, supportPageReq)
	if supportPageRec.Code != http.StatusOK || !strings.Contains(supportPageRec.Body.String(), "bc1qexampleaddress") {
		t.Fatalf("published support page is unavailable: %d: %s", supportPageRec.Code, supportPageRec.Body.String())
	}
	techdataAuditReq := httptest.NewRequest(http.MethodGet, "/api/admin/techdata/audit", nil)
	techdataAuditReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	techdataAuditRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(techdataAuditRec, techdataAuditReq)
	if techdataAuditRec.Code != http.StatusOK || !strings.Contains(techdataAuditRec.Body.String(), `"unlinked_documents"`) || !strings.Contains(techdataAuditRec.Body.String(), `"name":"C"`) {
		t.Fatalf("administrator techdata audit omitted an unlinked document: %d: %s", techdataAuditRec.Code, techdataAuditRec.Body.String())
	}
	publicPageSummaryReq := httptest.NewRequest(http.MethodGet, "/api/admin/pages/ko/C", nil)
	publicPageSummaryRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(publicPageSummaryRec, publicPageSummaryReq)
	if publicPageSummaryRec.Code != http.StatusUnauthorized || strings.Contains(publicPageSummaryRec.Body.String(), `"wid"`) {
		t.Fatalf("public request exposed the administrator document summary: %d: %s", publicPageSummaryRec.Code, publicPageSummaryRec.Body.String())
	}
	editorPageSummaryReq := httptest.NewRequest(http.MethodGet, "/api/admin/pages/ko/C", nil)
	editorPageSummaryReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: editorToken})
	editorPageSummaryRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(editorPageSummaryRec, editorPageSummaryReq)
	if editorPageSummaryRec.Code != http.StatusForbidden || strings.Contains(editorPageSummaryRec.Body.String(), `"wid"`) {
		t.Fatalf("editor request exposed the administrator document summary: %d: %s", editorPageSummaryRec.Code, editorPageSummaryRec.Body.String())
	}
	adminPageSummaryReq := httptest.NewRequest(http.MethodGet, "/api/admin/pages/ko/C", nil)
	adminPageSummaryReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	adminPageSummaryRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(adminPageSummaryRec, adminPageSummaryReq)
	if adminPageSummaryRec.Code != http.StatusOK || !strings.Contains(adminPageSummaryRec.Body.String(), `"name":"C"`) || strings.Contains(adminPageSummaryRec.Body.String(), `"tid"`) {
		t.Fatalf("administrator document summary did not report an unlinked page: %d: %s", adminPageSummaryRec.Code, adminPageSummaryRec.Body.String())
	}
	createTechdataReq := httptest.NewRequest(http.MethodPost, "/api/admin/pages/ko/C/techdata", strings.NewReader(`{}`))
	createTechdataReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	createTechdataRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createTechdataRec, createTechdataReq)
	if createTechdataRec.Code != http.StatusOK {
		t.Fatalf("administrator could not create and link techdata: %d: %s", createTechdataRec.Code, createTechdataRec.Body.String())
	}
	var linkedItem techdata.Item
	if err := json.Unmarshal(createTechdataRec.Body.Bytes(), &linkedItem); err != nil || linkedItem.ID == "" || linkedItem.WikiPages["ko"] == "" {
		t.Fatalf("invalid linked techdata item: %#v err=%v", linkedItem, err)
	}
	linkedPageSummaryReq := httptest.NewRequest(http.MethodGet, "/api/admin/pages/ko/C", nil)
	linkedPageSummaryReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	linkedPageSummaryRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(linkedPageSummaryRec, linkedPageSummaryReq)
	if linkedPageSummaryRec.Code != http.StatusOK || !strings.Contains(linkedPageSummaryRec.Body.String(), `"tid":"`+linkedItem.ID.String()+`"`) {
		t.Fatalf("administrator document summary omitted the Techdata link: %d: %s", linkedPageSummaryRec.Code, linkedPageSummaryRec.Body.String())
	}
	listTechdataReq := httptest.NewRequest(http.MethodGet, "/api/admin/techdata/items?q=C", nil)
	listTechdataReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	listTechdataRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listTechdataRec, listTechdataReq)
	if listTechdataRec.Code != http.StatusOK || !strings.Contains(listTechdataRec.Body.String(), `"id":"`+linkedItem.ID.String()+`"`) {
		t.Fatalf("techdata search omitted linked item: %d: %s", listTechdataRec.Code, listTechdataRec.Body.String())
	}
	unlinkTechdataReq := httptest.NewRequest(http.MethodDelete, "/api/admin/pages/ko/C/techdata", nil)
	unlinkTechdataReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	unlinkTechdataRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unlinkTechdataRec, unlinkTechdataReq)
	if unlinkTechdataRec.Code != http.StatusNoContent {
		t.Fatalf("administrator could not unlink techdata: %d: %s", unlinkTechdataRec.Code, unlinkTechdataRec.Body.String())
	}
	duplicateTechdataReq := httptest.NewRequest(http.MethodPost, "/api/admin/pages/ko/C/techdata", strings.NewReader(`{}`))
	duplicateTechdataReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	duplicateTechdataRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(duplicateTechdataRec, duplicateTechdataReq)
	if duplicateTechdataRec.Code != http.StatusConflict || !strings.Contains(duplicateTechdataRec.Body.String(), `"code":"techdata_duplicate_candidate"`) || !strings.Contains(duplicateTechdataRec.Body.String(), `"id":"`+linkedItem.ID.String()+`"`) {
		t.Fatalf("duplicate Techdata creation was not blocked with candidates: %d: %s", duplicateTechdataRec.Code, duplicateTechdataRec.Body.String())
	}
	confirmedDistinctReq := httptest.NewRequest(http.MethodPost, "/api/admin/pages/ko/C/techdata", strings.NewReader(`{"confirm_distinct":true}`))
	confirmedDistinctReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	confirmedDistinctRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(confirmedDistinctRec, confirmedDistinctReq)
	var distinctItem techdata.Item
	if err := json.Unmarshal(confirmedDistinctRec.Body.Bytes(), &distinctItem); confirmedDistinctRec.Code != http.StatusOK || err != nil || distinctItem.ID == "" || distinctItem.ID == linkedItem.ID {
		t.Fatalf("administrator could not confirm a distinct Techdata item: %d: %s", confirmedDistinctRec.Code, confirmedDistinctRec.Body.String())
	}
	unlinkDistinctReq := httptest.NewRequest(http.MethodDelete, "/api/admin/pages/ko/C/techdata", nil)
	unlinkDistinctReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	unlinkDistinctRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unlinkDistinctRec, unlinkDistinctReq)
	if unlinkDistinctRec.Code != http.StatusNoContent {
		t.Fatalf("administrator could not unlink confirmed distinct item: %d: %s", unlinkDistinctRec.Code, unlinkDistinctRec.Body.String())
	}
	relinkBody, _ := json.Marshal(map[string]string{"tid": linkedItem.ID.String()})
	relinkTechdataReq := httptest.NewRequest(http.MethodPost, "/api/admin/pages/ko/C/techdata", strings.NewReader(string(relinkBody)))
	relinkTechdataReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	relinkTechdataRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(relinkTechdataRec, relinkTechdataReq)
	if relinkTechdataRec.Code != http.StatusOK {
		t.Fatalf("administrator could not relink existing techdata: %d: %s", relinkTechdataRec.Code, relinkTechdataRec.Body.String())
	}
	translationBody, _ := json.Marshal(map[string]any{
		"name": "C (programming language)", "source": "# C (programming language)", "edit_policy": "editors",
		"translation_source": map[string]string{"locale": "ko", "name": "C"},
	})
	translationReq := httptest.NewRequest(http.MethodPut, "/api/editor/pages/en/C_(programming_language)", strings.NewReader(string(translationBody)))
	translationReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: editorToken})
	translationRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(translationRec, translationReq)
	if translationRec.Code != http.StatusOK {
		t.Fatalf("editor could not create linked translation: %d: %s", translationRec.Code, translationRec.Body.String())
	}
	translated, err := store.GetLatest(context.Background(), "en", techmark.Slugify("C (programming language)"))
	if err != nil {
		t.Fatalf("load translated document: %v", err)
	}
	translatedItem, err := store.GetItemByWikiPage(context.Background(), translated.ID)
	if err != nil || translatedItem.ID != linkedItem.ID || translatedItem.Labels["en"] != "C (programming language)" {
		t.Fatalf("translated document was not linked to source Techdata: %#v, %v", translatedItem, err)
	}
	translatedLanguagesReq := httptest.NewRequest(http.MethodGet, "/api/pages/ko/C", nil)
	translatedLanguagesRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(translatedLanguagesRec, translatedLanguagesReq)
	if translatedLanguagesRec.Code != http.StatusOK || !strings.Contains(translatedLanguagesRec.Body.String(), `"url":"/en/wiki/C_(programming_language)"`) {
		t.Fatalf("public source omitted translated language: %d: %s", translatedLanguagesRec.Code, translatedLanguagesRec.Body.String())
	}
	countryCatalogReq := httptest.NewRequest(http.MethodGet, "/api/country-catalog", nil)
	countryCatalogRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(countryCatalogRec, countryCatalogReq)
	if countryCatalogRec.Code != http.StatusOK || !strings.Contains(countryCatalogRec.Body.String(), `"code":"KR"`) {
		t.Fatalf("country catalog omitted KR: %d: %s", countryCatalogRec.Code, countryCatalogRec.Body.String())
	}
	portalMetadataReq := httptest.NewRequest(http.MethodPatch, "/api/admin/languages/ko/portal", strings.NewReader(`{"portal_order":10,"country_codes":["KR","US"]}`))
	portalMetadataReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	portalMetadataRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(portalMetadataRec, portalMetadataReq)
	if portalMetadataRec.Code != http.StatusOK || !strings.Contains(portalMetadataRec.Body.String(), `"portal_order":10`) || !strings.Contains(portalMetadataRec.Body.String(), `"country_codes":["KR","US"]`) {
		t.Fatalf("administrator could not update portal metadata: %d: %s", portalMetadataRec.Code, portalMetadataRec.Body.String())
	}

	wrongHomeBody, _ := json.Marshal(map[string]string{"name": "Not Main", "source": homeSource, "edit_policy": "admins"})
	wrongHomeReq := httptest.NewRequest(http.MethodPut, "/api/editor/pages/ko/Not_Main", strings.NewReader(string(wrongHomeBody)))
	wrongHomeReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	wrongHomeRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wrongHomeRec, wrongHomeReq)
	if wrongHomeRec.Code != http.StatusBadRequest {
		t.Fatalf("homepage syntax was accepted outside the main page: %d: %s", wrongHomeRec.Code, wrongHomeRec.Body.String())
	}

	canonicalBotDocument, err := store.Save(context.Background(), document.Document{
		Locale: "ko", Slug: techmark.Slugify("Rust"), Title: "Rust", Source: "# Rust", Author: "admin",
	})
	if err != nil {
		t.Fatalf("create canonical bot document: %v", err)
	}
	botScanDocument, err := store.Save(context.Background(), document.Document{
		Locale: "ko", Slug: techmark.Slugify("명명 봇 검사"), Title: "명명 봇 검사", Source: "# 명명 봇 검사\n\n[[러스트]]를 참고한다.", Author: "admin",
	})
	if err != nil {
		t.Fatalf("create bot scan document: %v", err)
	}
	if _, err := store.CreateItem(context.Background(), techdata.Item{
		Labels:    map[string]string{"ko": "Rust"},
		Aliases:   map[string][]string{"ko": {"러스트"}},
		WikiPages: map[string]document.WID{"ko": canonicalBotDocument.ID},
	}, techdata.CreateItemOptions{ConfirmDistinct: true}); err != nil {
		t.Fatalf("create Techdata naming source: %v", err)
	}
	scanReq := httptest.NewRequest(http.MethodPost, "/api/admin/bots/naming/scan?locale=ko", nil)
	scanReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	scanRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(scanRec, scanReq)
	if scanRec.Code != http.StatusOK || !strings.Contains(scanRec.Body.String(), `"created":1`) ||
		!strings.Contains(scanRec.Body.String(), `"links_discovered":`) ||
		!strings.Contains(scanRec.Body.String(), `"missing_links":`) ||
		!strings.Contains(scanRec.Body.String(), `"orphan_documents":`) {
		t.Fatalf("naming scan did not create a suggestion: %d: %s", scanRec.Code, scanRec.Body.String())
	}
	listSuggestionsReq := httptest.NewRequest(http.MethodGet, "/api/admin/bots/naming/suggestions?status=open", nil)
	listSuggestionsReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	listSuggestionsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listSuggestionsRec, listSuggestionsReq)
	var suggestionList struct {
		Suggestions []bot.NamingSuggestion `json:"suggestions"`
	}
	if listSuggestionsRec.Code != http.StatusOK || json.Unmarshal(listSuggestionsRec.Body.Bytes(), &suggestionList) != nil || len(suggestionList.Suggestions) != 1 {
		t.Fatalf("could not list naming suggestion: %d: %s", listSuggestionsRec.Code, listSuggestionsRec.Body.String())
	}
	suggestion := suggestionList.Suggestions[0]
	acceptBody, _ := json.Marshal(map[string]string{"proposed_name": suggestion.ProposedName, "proposed_source": suggestion.ProposedSource})
	acceptReq := httptest.NewRequest(http.MethodPost, "/api/admin/bots/naming/suggestions/"+suggestion.ID+"/accept", strings.NewReader(string(acceptBody)))
	acceptReq.Header.Set("Content-Type", "application/json")
	acceptReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	acceptRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(acceptRec, acceptReq)
	if acceptRec.Code != http.StatusOK || !strings.Contains(acceptRec.Body.String(), `"status":"accepted"`) {
		t.Fatalf("administrator could not accept naming suggestion: %d: %s", acceptRec.Code, acceptRec.Body.String())
	}
	if updated, err := store.GetLatest(context.Background(), "ko", botScanDocument.Slug); err != nil || !strings.Contains(updated.Source, "[[Rust|러스트]]") {
		t.Fatalf("accepted naming suggestion did not update the link target: %#v, %v", updated, err)
	}

	editorMoveReq := httptest.NewRequest(http.MethodPost, "/api/admin/pages/ko/Protected/move", strings.NewReader(`{"name":"Moved Protected"}`))
	editorMoveReq.Header.Set("Content-Type", "application/json")
	editorMoveReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: editorToken})
	editorMoveRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(editorMoveRec, editorMoveReq)
	if editorMoveRec.Code != http.StatusForbidden {
		t.Fatalf("regular editor moved a document: %d: %s", editorMoveRec.Code, editorMoveRec.Body.String())
	}

	adminMoveReq := httptest.NewRequest(http.MethodPost, "/api/admin/pages/ko/Protected/move", strings.NewReader(`{"name":"Moved Protected"}`))
	adminMoveReq.Header.Set("Content-Type", "application/json")
	adminMoveReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	adminMoveRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(adminMoveRec, adminMoveReq)
	if adminMoveRec.Code != http.StatusOK || !strings.Contains(adminMoveRec.Body.String(), `"name":"Moved Protected"`) {
		t.Fatalf("administrator could not move document: %d: %s", adminMoveRec.Code, adminMoveRec.Body.String())
	}
	titleMoveReq := httptest.NewRequest(http.MethodPost, "/api/admin/pages/ko/Moved_Protected/move", strings.NewReader(`{"name":"Moved protected"}`))
	titleMoveReq.Header.Set("Content-Type", "application/json")
	titleMoveReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	titleMoveRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(titleMoveRec, titleMoveReq)
	if titleMoveRec.Code != http.StatusOK || !strings.Contains(titleMoveRec.Body.String(), `"name":"Moved protected"`) {
		t.Fatalf("administrator could not change title casing: %d: %s", titleMoveRec.Code, titleMoveRec.Body.String())
	}
	oldPageReq := httptest.NewRequest(http.MethodGet, "/ko/wiki/Protected", nil)
	oldPageRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(oldPageRec, oldPageReq)
	if oldPageRec.Code != http.StatusPermanentRedirect || oldPageRec.Header().Get("Location") != "/ko/wiki/Moved_protected" {
		t.Fatalf("old document route did not redirect: %d location=%q", oldPageRec.Code, oldPageRec.Header().Get("Location"))
	}
	oldLinkReq := httptest.NewRequest(http.MethodGet, "/api/links/resolve?locale=ko&target=Protected", nil)
	oldLinkRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(oldLinkRec, oldLinkReq)
	if oldLinkRec.Code != http.StatusOK || !strings.Contains(oldLinkRec.Body.String(), `"name":"Moved protected"`) || !strings.Contains(oldLinkRec.Body.String(), `"exists":true`) {
		t.Fatalf("old internal link did not resolve through redirect: %d: %s", oldLinkRec.Code, oldLinkRec.Body.String())
	}
}

func TestV16EditingWorkflowAPIs(t *testing.T) {
	store, err := storage.OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	configureTestEdition(t, store, "ko", "대문")
	first, err := store.Save(context.Background(), document.Document{Locale: "ko", Slug: "workflow", Title: "Workflow", Source: "# Workflow\n\nfirst", Author: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	secondInput := first
	secondInput.Source = "# Workflow\n\nsecond"
	second, err := store.SaveIfRevision(context.Background(), secondInput, first.Revision)
	if err != nil {
		t.Fatal(err)
	}

	accounts, err := auth.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer accounts.Close()
	if err := accounts.EnsureBootstrapAdmin(context.Background(), "workflow_admin", "long-admin-password"); err != nil {
		t.Fatal(err)
	}
	_, token, _, err := accounts.Login(context.Background(), "workflow_admin", "long-admin-password")
	if err != nil {
		t.Fatal(err)
	}
	srv := New(config.Config{}, store, accounts, techmark.NewParser())
	do := func(method string, target string, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}

	conflict := do(http.MethodPut, "/api/editor/pages/ko/Workflow", `{"name":"Workflow","source":"stale","edit_policy":"editors","base_revision":"r1"}`)
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), `"revision":"r2"`) || !strings.Contains(conflict.Body.String(), "second") {
		t.Fatalf("expected structured edit conflict, got %d: %s", conflict.Code, conflict.Body.String())
	}

	draft := do(http.MethodPut, "/api/editor/pages/ko/Workflow/draft", `{"name":"Workflow","source":"draft source","summary":"draft","base_revision":"r2"}`)
	if draft.Code != http.StatusOK {
		t.Fatalf("save draft: %d: %s", draft.Code, draft.Body.String())
	}
	loadedDraft := do(http.MethodGet, "/api/editor/pages/ko/Workflow/draft", "")
	if loadedDraft.Code != http.StatusOK || !strings.Contains(loadedDraft.Body.String(), "draft source") || strings.Contains(loadedDraft.Body.String(), `"user_id":"U`) {
		t.Fatalf("load protected draft: %d: %s", loadedDraft.Code, loadedDraft.Body.String())
	}

	diff := do(http.MethodGet, "/api/pages/ko/Workflow/diff?from=r1&to=r2", "")
	if diff.Code != http.StatusOK || !strings.Contains(diff.Body.String(), `"kind":"removed"`) || !strings.Contains(diff.Body.String(), `"kind":"added"`) {
		t.Fatalf("revision diff: %d: %s", diff.Code, diff.Body.String())
	}
	restore := do(http.MethodPost, "/api/editor/pages/ko/Workflow/restore", `{"revision":"r1","base_revision":"r2","reason":"regression"}`)
	if restore.Code != http.StatusOK || !strings.Contains(restore.Body.String(), `"revision":"r3"`) {
		t.Fatalf("restore revision: %d: %s", restore.Code, restore.Body.String())
	}

	deleted := do(http.MethodDelete, "/api/admin/pages/ko/Workflow", `{"reason":"integration test"}`)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("soft delete: %d: %s", deleted.Code, deleted.Body.String())
	}
	trash := do(http.MethodGet, "/api/admin/trash", "")
	if trash.Code != http.StatusOK || !strings.Contains(trash.Body.String(), first.ID.String()) {
		t.Fatalf("list trash: %d: %s", trash.Code, trash.Body.String())
	}
	restored := do(http.MethodPost, "/api/admin/trash/"+first.ID.String()+"/restore", `{"reason":"integration restore"}`)
	if restored.Code != http.StatusOK {
		t.Fatalf("restore trash: %d: %s", restored.Code, restored.Body.String())
	}
	latest, err := store.GetLatest(context.Background(), "ko", "workflow")
	if err != nil || latest.ID != first.ID || latest.Revision != "r3" {
		t.Fatalf("restored identity mismatch: %#v, %v", latest, err)
	}
	audit := do(http.MethodGet, "/api/admin/audit", "")
	for _, action := range []string{"revision.restore", "document.delete", "document.restore"} {
		if audit.Code != http.StatusOK || !strings.Contains(audit.Body.String(), action) {
			t.Fatalf("audit log omitted %s: %d: %s", action, audit.Code, audit.Body.String())
		}
	}
	_ = second
}

func TestOwnerCanResetDocumentHistoryAndRevisionResponsesAreNotCached(t *testing.T) {
	store, err := storage.OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	configureTestEdition(t, store, "ko", "대문")
	first, err := store.Save(context.Background(), document.Document{Locale: "ko", Slug: "privacy", Title: "Privacy", Source: "old legal name", Author: "legal-name"})
	if err != nil {
		t.Fatal(err)
	}
	secondInput := first
	secondInput.Source = "safe nickname"
	secondInput.Author = "nickname"
	second, err := store.SaveIfRevision(context.Background(), secondInput, first.Revision)
	if err != nil {
		t.Fatal(err)
	}

	accounts, err := auth.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer accounts.Close()
	if err := accounts.EnsureBootstrapAdmin(context.Background(), "privacy_owner", "long-owner-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.Register(context.Background(), auth.Registration{Username: "privacy_admin", Password: "long-admin-password", Permissions: []auth.Permission{auth.PermissionAdmin}}); err != nil {
		t.Fatal(err)
	}
	_, ownerToken, _, err := accounts.Login(context.Background(), "privacy_owner", "long-owner-password")
	if err != nil {
		t.Fatal(err)
	}
	_, adminToken, _, err := accounts.Login(context.Background(), "privacy_admin", "long-admin-password")
	if err != nil {
		t.Fatal(err)
	}

	srv := New(config.Config{}, store, accounts, techmark.NewParser())
	srv.graph["ko:600"] = graphCacheEntry{body: []byte("old graph"), expiresAt: time.Now().Add(time.Minute)}
	do := func(token string, method string, target string, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
		}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}

	oldRevision := do("", http.MethodGet, "/api/pages/ko/Privacy/revisions/r1", "")
	if oldRevision.Code != http.StatusOK || !strings.Contains(oldRevision.Body.String(), "old legal name") {
		t.Fatalf("old revision precondition: %d: %s", oldRevision.Code, oldRevision.Body.String())
	}
	if oldRevision.Header().Get("Cache-Control") != "no-store" || oldRevision.Header().Get("X-Robots-Tag") != "noindex,noarchive" {
		t.Fatalf("revision response cache policy: %#v", oldRevision.Header())
	}
	history := do(ownerToken, http.MethodGet, "/api/pages/ko/Privacy/revisions", "")
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), `"can_reset_history":true`) || history.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("history list privacy policy: %d headers=%#v body=%s", history.Code, history.Header(), history.Body.String())
	}
	diff := do("", http.MethodGet, "/api/pages/ko/Privacy/diff?from=r1&to=r2", "")
	if diff.Code != http.StatusOK || diff.Header().Get("Cache-Control") != "no-store" || diff.Header().Get("X-Robots-Tag") != "noindex,noarchive" {
		t.Fatalf("revision diff privacy policy: %d headers=%#v body=%s", diff.Code, diff.Header(), diff.Body.String())
	}

	body := `{"base_revision":"r2","confirmation":"Privacy"}`
	adminReset := do(adminToken, http.MethodPost, "/api/admin/pages/ko/Privacy/history/reset", body)
	if adminReset.Code != http.StatusForbidden {
		t.Fatalf("administrator reset owner-only history: %d: %s", adminReset.Code, adminReset.Body.String())
	}
	wrongConfirmation := do(ownerToken, http.MethodPost, "/api/admin/pages/ko/Privacy/history/reset", `{"base_revision":"r2","confirmation":"Wrong"}`)
	if wrongConfirmation.Code != http.StatusBadRequest {
		t.Fatalf("history reset accepted wrong confirmation: %d: %s", wrongConfirmation.Code, wrongConfirmation.Body.String())
	}
	reset := do(ownerToken, http.MethodPost, "/api/admin/pages/ko/Privacy/history/reset", body)
	if reset.Code != http.StatusOK || !strings.Contains(reset.Body.String(), `"revision":"r1"`) || reset.Header().Get("Clear-Site-Data") != `"cache"` {
		t.Fatalf("owner history reset: %d headers=%#v body=%s", reset.Code, reset.Header(), reset.Body.String())
	}
	if len(srv.graph) != 0 {
		t.Fatalf("graph cache was not invalidated: %#v", srv.graph)
	}
	removed := do("", http.MethodGet, "/api/pages/ko/Privacy/revisions/r2", "")
	if removed.Code != http.StatusNotFound || strings.Contains(removed.Body.String(), "legal") {
		t.Fatalf("removed revision remains public: %d: %s", removed.Code, removed.Body.String())
	}
	current, err := store.GetLatest(context.Background(), "ko", "privacy")
	if err != nil || current.ID != first.ID || current.Revision != "r1" || current.Source != second.Source || current.Author != "privacy_owner" {
		t.Fatalf("unexpected reset document: %#v, %v", current, err)
	}
	audit, err := store.ListAudit(context.Background(), document.AuditFilter{Action: "document.history_reset", Limit: 10})
	if err != nil || len(audit) != 1 || strings.Contains(audit[0].Detail, "legal") {
		t.Fatalf("privacy reset audit is missing or leaks content: %#v, %v", audit, err)
	}
}

func TestProtectedRevisionDiffRequiresAdministrator(t *testing.T) {
	store, err := storage.OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	configureTestEdition(t, store, "ko", "대문")
	first, err := store.Save(context.Background(), document.Document{Locale: "ko", Slug: techmark.Slugify("Protected diff"), Title: "Protected diff", Source: "one", EditPolicy: document.EditPolicyAdmins})
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.Source = "two"
	if _, err := store.SaveIfRevision(context.Background(), second, first.Revision); err != nil {
		t.Fatal(err)
	}
	srv := New(config.Config{}, store, emptyAuth{}, techmark.NewParser())
	req := httptest.NewRequest(http.MethodGet, "/api/pages/ko/Protected_diff/diff?from=r1&to=r2", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "one") || strings.Contains(rec.Body.String(), "two") {
		t.Fatalf("protected source diff leaked: %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteTechdataItemRequiresOwnerAndRejectsItemsInUse(t *testing.T) {
	store, err := storage.OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	accounts, err := auth.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer accounts.Close()
	ctx := context.Background()
	if err := accounts.EnsureBootstrapAdmin(ctx, "techdata_owner", "long-owner-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.Register(ctx, auth.Registration{
		Username: "techdata_admin", Password: "long-admin-password", Permissions: []auth.Permission{auth.PermissionAdmin},
	}); err != nil {
		t.Fatal(err)
	}
	_, ownerToken, _, err := accounts.Login(ctx, "techdata_owner", "long-owner-password")
	if err != nil {
		t.Fatal(err)
	}
	_, adminToken, _, err := accounts.Login(ctx, "techdata_admin", "long-admin-password")
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := store.CreateItem(ctx, techdata.Item{Labels: map[string]string{"ko": "삭제 대상 개인 정보"}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := store.CreateItem(ctx, techdata.Item{WikiPages: map[string]document.WID{"ko": "W99"}})
	if err != nil {
		t.Fatal(err)
	}
	srv := New(config.Config{}, store, accounts, techmark.NewParser())

	adminReq := httptest.NewRequest(http.MethodDelete, "/api/admin/techdata/items/"+orphan.ID.String(), strings.NewReader(`{"confirmation":"`+orphan.ID.String()+`"}`))
	adminReq.Header.Set("Content-Type", "application/json")
	adminReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	adminRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(adminRec, adminReq)
	if adminRec.Code != http.StatusForbidden {
		t.Fatalf("administrator deleted an owner-only item: %d: %s", adminRec.Code, adminRec.Body.String())
	}

	wrongReq := httptest.NewRequest(http.MethodDelete, "/api/admin/techdata/items/"+orphan.ID.String(), strings.NewReader(`{"confirmation":"T999"}`))
	wrongReq.Header.Set("Content-Type", "application/json")
	wrongReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: ownerToken})
	wrongRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wrongRec, wrongReq)
	if wrongRec.Code != http.StatusBadRequest {
		t.Fatalf("wrong confirmation was accepted: %d: %s", wrongRec.Code, wrongRec.Body.String())
	}

	linkedReq := httptest.NewRequest(http.MethodDelete, "/api/admin/techdata/items/"+linked.ID.String(), strings.NewReader(`{"confirmation":"`+linked.ID.String()+`"}`))
	linkedReq.Header.Set("Content-Type", "application/json")
	linkedReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: ownerToken})
	linkedRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(linkedRec, linkedReq)
	if linkedRec.Code != http.StatusConflict || !strings.Contains(linkedRec.Body.String(), `"code":"techdata_item_in_use"`) {
		t.Fatalf("linked item deletion was not rejected: %d: %s", linkedRec.Code, linkedRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/admin/techdata/items/"+orphan.ID.String(), strings.NewReader(`{"confirmation":"`+orphan.ID.String()+`"}`))
	deleteReq.Header.Set("Content-Type", "application/json")
	deleteReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: ownerToken})
	deleteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent || deleteRec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("owner could not delete orphan item: %d: %s", deleteRec.Code, deleteRec.Body.String())
	}
	if _, err := store.GetItem(ctx, orphan.ID); !errors.Is(err, techdata.ErrItemNotFound) {
		t.Fatalf("deleted item remains: %v", err)
	}
	audit, err := store.ListAudit(ctx, document.AuditFilter{Action: "techdata.item_delete", Limit: 10})
	if err != nil || len(audit) < 2 {
		t.Fatalf("techdata deletion audit is missing: %#v, %v", audit, err)
	}
	for _, entry := range audit {
		if entry.Target != orphan.ID.String() && entry.Target != linked.ID.String() {
			t.Fatalf("unexpected deletion audit target: %#v", entry)
		}
		if strings.Contains(entry.Detail, "개인 정보") {
			t.Fatalf("deletion audit copied item metadata: %#v", entry)
		}
	}
}

type emptyStore struct{}

type countedEditionStore struct {
	emptyStore
}

type apiEditionStore struct {
	emptyStore
}

func (apiEditionStore) GetEdition(ctx context.Context, tag string) (edition.Edition, error) {
	if tag == "api" {
		return edition.Edition{Tag: "api", Name: "Apiaka", NativeName: "Apiaka", Direction: "ltr", Status: edition.StatusActive, MainPageName: "Main Page"}, nil
	}
	return emptyStore{}.GetEdition(ctx, tag)
}

func (countedEditionStore) ListEditions(context.Context) ([]edition.Edition, error) {
	return []edition.Edition{
		{Tag: "en", Name: "English", NativeName: "English", Direction: "ltr", Status: edition.StatusActive, MainPageName: "Main Page", PortalOrder: 1},
		{Tag: "es", Name: "Spanish", NativeName: "Espanol", Direction: "ltr", Status: edition.StatusActive, MainPageName: "Portada", PortalOrder: 2},
		{Tag: "ko", Name: "Korean", NativeName: "한국어", Direction: "ltr", Status: edition.StatusActive, MainPageName: "대문", PortalOrder: 100},
	}, nil
}

func (countedEditionStore) CountLatestByLocale(context.Context) (map[string]int, error) {
	return map[string]int{"ko": 8, "en": 3, "es": 1}, nil
}

type supportStore struct {
	emptyStore
	settings support.Settings
}

func (s supportStore) GetSupportSettings(context.Context) (support.Settings, error) {
	return support.NormalizeSettings(s.settings)
}

func (emptyStore) ListEditions(context.Context) ([]edition.Edition, error) {
	return []edition.Edition{
		{Tag: "ko", Name: "Korean", NativeName: "한국어", Direction: "ltr", Status: edition.StatusActive, MainPageName: "대문"},
		{Tag: "en", Name: "English", NativeName: "English", Direction: "ltr", Status: edition.StatusActive, MainPageName: "Main Page"},
	}, nil
}

func (emptyStore) GetEdition(_ context.Context, tag string) (edition.Edition, error) {
	for _, value := range []edition.Edition{
		{Tag: "ko", Name: "Korean", NativeName: "한국어", Direction: "ltr", Status: edition.StatusActive, MainPageName: "대문"},
		{Tag: "en", Name: "English", NativeName: "English", Direction: "ltr", Status: edition.StatusActive, MainPageName: "Main Page"},
	} {
		if value.Tag == tag {
			return value, nil
		}
	}
	return edition.Edition{}, edition.ErrNotFound
}

func (emptyStore) ProposeEdition(context.Context, edition.Edition) (edition.Edition, error) {
	return edition.Edition{}, errors.New("not implemented")
}

func (emptyStore) ApproveEdition(context.Context, string, string, time.Time) (edition.Edition, error) {
	return edition.Edition{}, errors.New("not implemented")
}

func (emptyStore) RejectEdition(context.Context, string) (edition.Edition, error) {
	return edition.Edition{}, errors.New("not implemented")
}

func (emptyStore) ActivateEdition(context.Context, string, string, time.Time) (edition.Edition, error) {
	return edition.Edition{}, errors.New("not implemented")
}

func (emptyStore) UpdateInterfaceMessages(context.Context, string, map[string]string) (edition.Edition, error) {
	return edition.Edition{}, errors.New("not implemented")
}

func (emptyStore) FillInterfaceMessages(context.Context, string, map[string]string) (edition.Edition, int, error) {
	return edition.Edition{}, 0, errors.New("not implemented")
}

func (emptyStore) UpdateNavigation(context.Context, string, []edition.NavigationSection) (edition.Edition, error) {
	return edition.Edition{}, errors.New("not implemented")
}

func (emptyStore) UpdatePortal(context.Context, string, int, []string) (edition.Edition, error) {
	return edition.Edition{}, errors.New("not implemented")
}

func (emptyStore) SaveMedia(context.Context, media.File) (media.File, error) {
	return media.File{}, errors.New("not implemented")
}

func (emptyStore) GetMedia(context.Context, string) (media.File, error) {
	return media.File{}, media.ErrNotFound
}

func (emptyStore) GetMediaByHash(context.Context, string) (media.File, error) {
	return media.File{}, media.ErrNotFound
}

func (emptyStore) SearchMedia(context.Context, string, int) ([]media.File, error) {
	return []media.File{}, nil
}

type revisionStore struct {
	emptyStore
	doc document.Document
}

type pageViewStore struct {
	revisionStore
	views  int
	locale string
	slug   string
}

func (s *pageViewStore) RecordPageView(_ context.Context, locale string, slug string, _ time.Time) error {
	s.views++
	s.locale = locale
	s.slug = slug
	return nil
}

func (s revisionStore) GetLatest(context.Context, string, string) (document.Document, error) {
	return s.doc, nil
}

func (s revisionStore) GetRevision(context.Context, string, string, string) (document.Document, error) {
	return s.doc, nil
}

func (s revisionStore) ListRevisions(context.Context, string, string, int) ([]document.RevisionSummary, error) {
	return []document.RevisionSummary{document.NewRevisionSummary(s.doc)}, nil
}

type emptyAuth struct{}

func (emptyAuth) Register(context.Context, auth.Registration) (auth.User, error) {
	return auth.User{}, errors.New("not implemented")
}

func (emptyAuth) Login(context.Context, string, string) (auth.User, string, time.Time, error) {
	return auth.User{}, "", time.Time{}, auth.ErrInvalidCredentials
}

func (emptyAuth) Authenticate(context.Context, string) (auth.User, error) {
	return auth.User{}, auth.ErrSessionNotFound
}

func (emptyAuth) Logout(context.Context, string) error { return nil }

func (emptyAuth) GetProfile(context.Context, string) (auth.User, error) {
	return auth.User{}, auth.ErrUserNotFound
}

func (emptyAuth) UpdateProfile(context.Context, string, auth.ProfileUpdate) (auth.User, error) {
	return auth.User{}, auth.ErrUserNotFound
}

func (emptyAuth) ListUsers(context.Context) ([]auth.User, error) {
	return []auth.User{}, nil
}

func (emptyAuth) SetPermissions(context.Context, string, []auth.Permission) (auth.User, error) {
	return auth.User{}, auth.ErrUserNotFound
}

func (emptyAuth) RegularUserPermissions(context.Context) ([]auth.Permission, error) {
	return []auth.Permission{}, nil
}

func (emptyAuth) SetRegularUserPermissions(context.Context, []auth.Permission) (int, error) {
	return 0, nil
}

func (emptyAuth) EnsureBootstrapAdmin(context.Context, string, string) error { return nil }
func (emptyAuth) Close() error                                               { return nil }

func (emptyStore) Save(context.Context, document.Document) (document.Document, error) {
	return document.Document{}, errors.New("not implemented")
}

func (emptyStore) SaveIfRevision(context.Context, document.Document, string) (document.Document, error) {
	return document.Document{}, errors.New("not implemented")
}

func (emptyStore) SaveTranslation(context.Context, document.Document, document.WID) (document.Document, error) {
	return document.Document{}, errors.New("not implemented")
}

func (emptyStore) MoveDocument(context.Context, string, string, string, string) (document.Document, error) {
	return document.Document{}, errors.New("not implemented")
}

func (emptyStore) MoveDocumentAs(context.Context, string, string, string, string, string) (document.Document, error) {
	return document.Document{}, errors.New("not implemented")
}

func (emptyStore) CreateDocumentRedirect(context.Context, string, string, string, string) (document.Redirect, error) {
	return document.Redirect{}, errors.New("not implemented")
}

func (emptyStore) DeleteDocumentRedirect(context.Context, string, string) (document.Redirect, error) {
	return document.Redirect{}, storage.ErrDocumentNotFound
}

func (emptyStore) ListDocumentRedirects(context.Context, string, int) ([]document.Redirect, error) {
	return []document.Redirect{}, nil
}

func (emptyStore) GetDocumentRedirect(context.Context, string, string) (string, error) {
	return "", storage.ErrDocumentNotFound
}

func (emptyStore) GetLatest(context.Context, string, string) (document.Document, error) {
	return document.Document{}, storage.ErrDocumentNotFound
}

func (emptyStore) GetLatestByWID(context.Context, document.WID) (document.Document, error) {
	return document.Document{}, storage.ErrDocumentNotFound
}

func (emptyStore) GetRevision(context.Context, string, string, string) (document.Document, error) {
	return document.Document{}, storage.ErrDocumentNotFound
}

func (emptyStore) ListRevisions(context.Context, string, string, int) ([]document.RevisionSummary, error) {
	return []document.RevisionSummary{}, nil
}

func (emptyStore) ListRecentRevisions(context.Context, string, int) ([]document.RevisionSummary, error) {
	return []document.RevisionSummary{}, nil
}

func (emptyStore) ListChanges(context.Context, document.ChangeFilter) ([]document.Change, error) {
	return []document.Change{}, nil
}

func (emptyStore) RestoreRevision(context.Context, string, string, string, string, string, string) (document.Document, error) {
	return document.Document{}, storage.ErrDocumentNotFound
}

func (emptyStore) ResetDocumentHistory(context.Context, string, string, string, string) (document.HistoryResetResult, error) {
	return document.HistoryResetResult{}, storage.ErrDocumentNotFound
}

func (emptyStore) SaveDraft(context.Context, document.Draft) (document.Draft, error) {
	return document.Draft{}, nil
}

func (emptyStore) GetDraft(context.Context, string, string, string) (document.Draft, error) {
	return document.Draft{}, storage.ErrDocumentNotFound
}

func (emptyStore) DeleteDraft(context.Context, string, string, string) error { return nil }

func (emptyStore) DeleteDocument(context.Context, string, string, string, string) (document.TrashEntry, error) {
	return document.TrashEntry{}, storage.ErrDocumentNotFound
}

func (emptyStore) ListTrash(context.Context, int) ([]document.TrashEntry, error) {
	return []document.TrashEntry{}, nil
}

func (emptyStore) RestoreDeletedDocument(context.Context, document.WID, string, string) (document.Document, error) {
	return document.Document{}, storage.ErrDocumentNotFound
}

func (emptyStore) AppendAudit(_ context.Context, entry document.AuditEntry) (document.AuditEntry, error) {
	return entry, nil
}

func (emptyStore) ListAudit(context.Context, document.AuditFilter) ([]document.AuditEntry, error) {
	return []document.AuditEntry{}, nil
}

func (emptyStore) GetTranslations(context.Context, string) ([]document.Document, error) {
	return []document.Document{}, nil
}

func (emptyStore) Search(context.Context, string, string, int) ([]document.SearchResult, error) {
	return []document.SearchResult{}, nil
}

func (emptyStore) Exists(context.Context, string, string) (document.Document, bool, error) {
	return document.Document{}, false, nil
}

func (emptyStore) ListLatest(context.Context, string, int) ([]document.Document, error) {
	return []document.Document{}, nil
}

func (emptyStore) ListLinkDocuments(context.Context, string) ([]document.LinkDocument, error) {
	return nil, nil
}

func (emptyStore) ListBacklinks(context.Context, string, string) ([]document.Document, error) {
	return nil, nil
}

func (emptyStore) ListAllLatest(context.Context, string) ([]document.Document, error) {
	return []document.Document{}, nil
}

func (emptyStore) CountLatestByLocale(context.Context) (map[string]int, error) {
	return map[string]int{}, nil
}

func (emptyStore) RandomLatest(context.Context, string) (document.Document, error) {
	return document.Document{}, storage.ErrDocumentNotFound
}

func (emptyStore) RecordPageView(context.Context, string, string, time.Time) error {
	return nil
}

func (emptyStore) ListPopular(context.Context, string, time.Time, int) ([]storage.PopularDocument, error) {
	return []storage.PopularDocument{}, nil
}

func (emptyStore) RecordAnalyticsVisit(context.Context, analytics.Visit) error {
	return nil
}

func (emptyStore) ListAnalyticsDaily(context.Context, time.Time, time.Time) ([]analytics.Daily, error) {
	return []analytics.Daily{}, nil
}

func (emptyStore) GetSupportSettings(context.Context) (support.Settings, error) {
	return support.DefaultSettings(), nil
}

func (emptyStore) SaveSupportSettings(context.Context, support.Settings) (support.Settings, error) {
	return support.Settings{}, errors.New("not implemented")
}

func (emptyStore) CreateDiscussionTopic(context.Context, discussion.Topic) (discussion.Topic, error) {
	return discussion.Topic{}, discussion.ErrInvalid
}

func (emptyStore) ListDiscussionTopics(context.Context, string, string, int) ([]discussion.Topic, error) {
	return []discussion.Topic{}, nil
}

func (emptyStore) ListAllDiscussionTopics(context.Context, string, int) ([]discussion.Topic, error) {
	return []discussion.Topic{}, nil
}

func (emptyStore) GetDiscussionThread(context.Context, string) (discussion.Thread, error) {
	return discussion.Thread{}, discussion.ErrNotFound
}

func (emptyStore) AddDiscussionReply(context.Context, discussion.Reply) (discussion.Reply, error) {
	return discussion.Reply{}, discussion.ErrInvalid
}

func (emptyStore) ArchiveDiscussionTopic(context.Context, string, string) (discussion.Topic, error) {
	return discussion.Topic{}, discussion.ErrNotFound
}

func (emptyStore) DeleteDiscussionTopic(context.Context, string) error {
	return discussion.ErrNotFound
}

func (emptyStore) DeleteDiscussionReply(context.Context, string, string) error {
	return discussion.ErrNotFound
}

func (emptyStore) Close() error {
	return nil
}

func (emptyStore) CreateItem(context.Context, techdata.Item, ...techdata.CreateItemOptions) (techdata.Item, error) {
	return techdata.Item{}, errors.New("not implemented")
}

func (emptyStore) ListItems(context.Context) ([]techdata.Item, error) {
	return []techdata.Item{}, nil
}

func (emptyStore) GetItem(context.Context, techdata.TID) (techdata.Item, error) {
	return techdata.Item{}, techdata.ErrItemNotFound
}

func (emptyStore) DeleteItem(context.Context, techdata.TID) (techdata.Item, error) {
	return techdata.Item{}, techdata.ErrItemNotFound
}

func (emptyStore) GetItemByWikiPage(context.Context, document.WID) (techdata.Item, error) {
	return techdata.Item{}, techdata.ErrItemNotFound
}

func (emptyStore) AttachWikiPage(context.Context, techdata.TID, string, document.WID, string) (techdata.Item, error) {
	return techdata.Item{}, errors.New("not implemented")
}

func (emptyStore) DetachWikiPage(context.Context, techdata.TID, string) (techdata.Item, error) {
	return techdata.Item{}, errors.New("not implemented")
}

func (emptyStore) CreateNamingSuggestion(context.Context, bot.NamingSuggestion) (bot.NamingSuggestion, bool, error) {
	return bot.NamingSuggestion{}, false, errors.New("not implemented")
}

func (emptyStore) GetNamingSuggestion(context.Context, string) (bot.NamingSuggestion, error) {
	return bot.NamingSuggestion{}, bot.ErrNotFound
}

func (emptyStore) ListNamingSuggestions(context.Context, bot.SuggestionStatus, int) ([]bot.NamingSuggestion, error) {
	return []bot.NamingSuggestion{}, nil
}

func (emptyStore) ReviewNamingSuggestion(context.Context, string, bot.SuggestionStatus, string, string, string) (bot.NamingSuggestion, error) {
	return bot.NamingSuggestion{}, errors.New("not implemented")
}
