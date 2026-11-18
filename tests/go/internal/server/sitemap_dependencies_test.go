package server

import (
	"context"
	"encoding/xml"
	"net/http/httptest"
	"testing"

	"techpedia/internal/document"
	"techpedia/internal/storage"
	"techpedia/internal/techmark"
)

func TestSitemapDoesNotAdvertiseRevisionDateForLiveContent(t *testing.T) {
	s, _ := oauthTestServer(t)
	db := s.store.(*storage.BadgerDocumentStore)
	configureTestEdition(t, db, "ko", "Main")
	for _, tc := range []struct {
		name, source string
		dated        bool
	}{
		{"Plain", "Article prose.", true},
		{"Recent", "<wiki-recent-changes></wiki-recent-changes>", false},
		{"Box", `<wiki-box name="Common" />`, false},
		{"Literal", "`<wiki-box name=\"Common\" />`", true},
		{"Example", "```techmark\n<wiki-recent-changes></wiki-recent-changes>\n```", true},
	} {
		if _, err := db.Save(context.Background(), document.Document{Locale: "ko", Title: tc.name, Slug: techmark.Slugify(tc.name), Source: tc.source}); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/sitemap.xml", nil))
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		var entries sitemapURLSet
		if err := xml.Unmarshal(w.Body.Bytes(), &entries); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, entry := range entries.URLs {
			if entry.Location == s.absoluteURL(wikiURL("ko", tc.name)) {
				found = true
				if (entry.LastMod != "") != tc.dated {
					t.Fatalf("%s: incorrect lastmod %q", tc.name, entry.LastMod)
				}
			}
		}
		if !found {
			t.Fatalf("document disappeared from sitemap: %s", tc.name)
		}
	}
}
