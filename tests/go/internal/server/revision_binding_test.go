package server

import (
	"context"
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

type revisionBindingAccounts struct{ emptyAuth }

func (revisionBindingAccounts) Authenticate(context.Context, string) (auth.User, error) {
	return auth.User{Username: "editor", Permissions: []auth.Permission{auth.PermissionEdit}}, nil
}

type revisionBindingStore struct {
	emptyStore
	writes int
}

func (s *revisionBindingStore) GetLatest(context.Context, string, string) (document.Document, error) {
	return document.Document{Locale: "ko", Slug: "race", Title: "Race", Revision: "r1", Source: "visible source", EditPolicy: document.EditPolicyEditors}, nil
}
func (s *revisionBindingStore) SaveIfRevision(_ context.Context, d document.Document, base string) (document.Document, error) {
	s.writes++
	if base != "r2" {
		return document.Document{}, storage.ErrRevisionConflict
	}
	d.Revision = "r3"
	return d, nil
}
func (s *revisionBindingStore) RestoreRevision(_ context.Context, _, _, _, base, _, _ string) (document.Document, error) {
	s.writes++
	if base != "r2" {
		return document.Document{}, storage.ErrRevisionConflict
	}
	return document.Document{Locale: "ko", Title: "Race", Revision: "r3"}, nil
}

func TestFutureBaseCannotCrossDocumentAuthorizationSnapshot(t *testing.T) {
	for _, endpoint := range []struct{ method, path, body string }{
		{"PUT", "/api/editor/pages/ko/Race", `{"name":"Race","source":"changed","base_revision":"r2"}`},
		{"POST", "/api/editor/pages/ko/Race/restore", `{"revision":"r1","base_revision":"r2"}`},
	} {
		for _, base := range []string{"r2", " r2 ", "r999"} {
			s := &revisionBindingStore{}
			srv := New(config.Config{}, s, revisionBindingAccounts{}, techmark.NewParser())
			r := httptest.NewRequest(endpoint.method, endpoint.path, strings.NewReader(strings.Replace(endpoint.body, "r2", base, 1)))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, r)
			if w.Code != http.StatusConflict || s.writes != 0 {
				t.Fatalf("future base reached write: status=%d calls=%d body=%s", w.Code, s.writes, w.Body.String())
			}
		}
	}
}
