package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"techpedia/internal/document"
	"techpedia/internal/storage"
)

func TestArticleBootstrapIncludesOnlyCurrentPublicEdition(t *testing.T) {
	s, _ := oauthTestServer(t)
	db := s.store.(*storage.BadgerDocumentStore)
	ctx := context.Background()
	for _, locale := range []string{"ko", "en", "ar"} {
		configureTestEdition(t, db, locale, "Main")
	}
	for _, locale := range []string{"ko", "en", "ar"} {
		t.Run(locale, func(t *testing.T) {
			messages := configuredMessages()
			messages["searchLabel"] = locale + ` </script><script>alert("x")</script>`
			if _, err := db.UpdateInterfaceMessages(ctx, locale, messages); err != nil {
				t.Fatal(err)
			}
			doc, err := db.Save(ctx, document.Document{Locale: locale, Title: "Test", Slug: "test", Source: strings.Repeat("Readable text 한글 العربية. ", 20)})
			if err != nil {
				t.Fatal(err)
			}
			read := func(headers map[string]string) *httptest.ResponseRecorder {
				r := httptest.NewRequest("GET", "/"+locale+"/wiki/Test", nil)
				for key, value := range headers {
					r.Header.Set(key, value)
				}
				w := httptest.NewRecorder()
				s.Handler().ServeHTTP(w, r)
				return w
			}
			before := read(nil)
			if before.Code != 200 {
				t.Fatal(before.Code, before.Body.String())
			}
			parts := strings.SplitN(before.Body.String(), `<script id="techpedia-page-data" type="application/json">`, 2)
			if len(parts) != 2 {
				t.Fatal("missing page bootstrap")
			}
			data := strings.SplitN(parts[1], "</script>", 2)[0]
			var initial struct {
				publicPageResponse
				Edition editionResponse `json:"edition"`
			}
			if err := json.Unmarshal([]byte(data), &initial); err != nil {
				t.Fatal(err)
			}
			if initial.Edition.Tag != locale || initial.Edition.InterfaceMessages["searchLabel"] != messages["searchLabel"] || len(initial.Edition.Navigation) == 0 {
				t.Fatal("current edition messages and navigation must survive without API access")
			}
			if initial.HTML != "" || initial.MetaDescription != documentDescription(doc.Source, doc.Title) || initial.MetaDescription == initial.Description {
				t.Fatal("bootstrap must omit duplicate HTML and preserve the longer SEO description")
			}
			for _, private := range []string{"requested_by", "approved_by", "requested_at", "test_administrator", "<script>alert"} {
				if strings.Contains(data, private) {
					t.Fatalf("unsafe or private bootstrap data: %s", private)
				}
			}
			messages["searchLabel"] = "Updated search"
			if _, err := db.UpdateInterfaceMessages(ctx, locale, messages); err != nil {
				t.Fatal(err)
			}
			after := read(map[string]string{"If-None-Match": before.Header().Get("ETag")})
			if after.Code != 200 || !strings.Contains(after.Body.String(), "Updated search") || before.Header().Get("ETag") == after.Header().Get("ETag") {
				t.Fatal("an interface edit must invalidate cached HTML without editing the article")
			}
			current, err := db.GetLatestByWID(ctx, doc.ID)
			if err != nil || current.Revision != doc.Revision {
				t.Fatal("interface rendering changed the document revision")
			}
		})
	}
}
