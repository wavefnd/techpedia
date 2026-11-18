package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTMLValidationIncludesEveryRenderedDependency(t *testing.T) {
	changes := map[string]func(*indexMetadata){
		"recent changes": func(m *indexMetadata) { m.InitialHTML = "<main>new recent change</main>" },
		"featured pages": func(m *indexMetadata) { m.InitialHTML = "<main>updated featured article</main>" },
		"Techdata language link": func(m *indexMetadata) {
			m.Alternates = []indexAlternate{{Locale: "en", URL: "https://example.test/en/wiki/Test"}}
		},
		"interface messages": func(m *indexMetadata) { m.InitialData = `{"label":"updated"}` },
		"template":           func(m *indexMetadata) { m.Title = "New template title" },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			s := &Server{}
			before := indexMetadata{Lang: "ko", Title: "Test", Robots: "index,follow", UpdatedAt: "2026-07-15T02:41:47Z", InitialHTML: "<main>old</main>"}
			get := func(method string, metadata indexMetadata, headers map[string]string) *httptest.ResponseRecorder {
				r := httptest.NewRequest(method, "/ko/wiki/Test", nil)
				for key, value := range headers {
					r.Header.Set(key, value)
				}
				w := httptest.NewRecorder()
				s.writeIndex(w, r, http.StatusOK, metadata)
				return w
			}
			old := get("GET", before, nil)
			after := before
			change(&after)
			fresh := get("GET", after, nil)
			if old.Header().Get("ETag") == fresh.Header().Get("ETag") {
				t.Fatal("changed representation must change the ETag")
			}
			for _, metadata := range []indexMetadata{before, after} {
				w := get("GET", metadata, map[string]string{"If-Modified-Since": "Mon, 03 Aug 2026 00:00:00 GMT"})
				if w.Code != 200 || w.Body.Len() == 0 || w.Header().Get("Last-Modified") != "" {
					t.Fatalf("date-only request must receive the complete HTML: %d %v", w.Code, w.Header())
				}
			}
			w := get("GET", after, map[string]string{"If-None-Match": old.Header().Get("ETag"), "If-Modified-Since": "Mon, 03 Aug 2026 00:00:00 GMT"})
			if w.Code != 200 || w.Body.String() != fresh.Body.String() {
				t.Fatal("old ETag must receive the fresh representation")
			}
			for _, method := range []string{"GET", "HEAD"} {
				w := get(method, after, map[string]string{"If-None-Match": "W/" + fresh.Header().Get("ETag")})
				if w.Code != 304 || w.Body.Len() != 0 {
					t.Fatalf("matching ETag: %s status=%d body=%d", method, w.Code, w.Body.Len())
				}
			}
			w = get("HEAD", after, nil)
			if w.Code != 200 || w.Body.Len() != 0 || w.Header().Get("ETag") != fresh.Header().Get("ETag") || w.Header().Get("Content-Length") != fresh.Header().Get("Content-Length") {
				t.Fatal("HEAD must describe the same current representation without a body")
			}
		})
	}
}
