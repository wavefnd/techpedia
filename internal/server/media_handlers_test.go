package server

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"techpedia/internal/auth"
	"techpedia/internal/config"
	"techpedia/internal/document"
	"techpedia/internal/media"
	"techpedia/internal/storage"
	"techpedia/internal/techmark"
)

func TestMediaDeletionIsOwnerOnlyConfirmedAndBlocksCurrentUse(t *testing.T) {
	s, accounts := oauthTestServer(t)
	ctx := context.Background()
	db := s.store.(*storage.BadgerDocumentStore)
	s.media = media.NewManager(t.TempDir(), db, media.Limits{})
	body, contentType := mediaUploadBody(t)
	r := httptest.NewRequest("POST", "https://wiki.example/api/editor/media", bytes.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "admin-session"})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var file mediaFileResponse
	if err := json.Unmarshal(w.Body.Bytes(), &file); err != nil {
		t.Fatal(err)
	}
	stored, err := db.GetMedia(ctx, file.Name)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/admin/media/" + file.Name
	confirmation := `{"confirmation":"` + file.Name + `"}`
	if w := oauthRequest(s, "DELETE", path, confirmation, "", "https://wiki.example"); w.Code != 401 {
		t.Fatal(w.Code)
	}
	for _, permission := range []auth.Permission{auth.PermissionEdit, auth.PermissionAdmin} {
		accounts.user.Permissions = []auth.Permission{permission}
		if w := oauthRequest(s, "DELETE", path, confirmation, "admin-session", "https://wiki.example"); w.Code != 403 {
			t.Fatal("not owner", permission, w.Code)
		}
	}
	accounts.user.Permissions = []auth.Permission{auth.PermissionOwner}
	if w := oauthRequest(s, "DELETE", path, confirmation, "admin-session", "https://evil.example"); w.Code != 403 {
		t.Fatal("bad origin", w.Code)
	}
	if w := oauthRequest(s, "DELETE", path, `{"confirmation":"wrong"}`, "admin-session", "https://wiki.example"); w.Code != 400 {
		t.Fatal(w.Code)
	}
	doc, err := db.Save(ctx, document.Document{Locale: "ko", Slug: "media", Title: "Media", Source: `<wiki-image file="Server_diagram.webp"></wiki-image>`})
	if err != nil {
		t.Fatal(err)
	}
	if w := oauthRequest(s, "DELETE", path, confirmation, "admin-session", "https://wiki.example"); w.Code != 409 {
		t.Fatal("used file deleted", w.Code, w.Body.String())
	}
	if w := oauthRequest(s, "GET", file.ThumbnailURL, "", "", ""); w.Code != 200 {
		t.Fatal("blocked delete removed bytes", w.Code)
	}
	doc.Source = "No current image"
	if _, err := db.Save(ctx, doc); err != nil {
		t.Fatal(err)
	}
	w = oauthRequest(s, "DELETE", path, confirmation, "admin-session", "https://wiki.example")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"cleanup_pending":false`) {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, url := range []string{file.ThumbnailURL, file.PreviewURL, file.OriginalURL} {
		if w := oauthRequest(s, "GET", url, "", "", ""); w.Code != 404 {
			t.Fatal("deleted variant still public", url, w.Code)
		}
	}
	if _, err := db.GetMediaByHash(ctx, stored.Hash); err != media.ErrNotFound {
		t.Fatal("hash index survived", err)
	}
	if usage, err := s.media.Usage(); err != nil || usage != 0 {
		t.Fatal(usage, err)
	}
	if w := oauthRequest(s, "DELETE", path, confirmation, "admin-session", "https://wiki.example"); w.Code != 404 {
		t.Fatal("replayed deletion", w.Code)
	}
	audits, err := db.ListAudit(ctx, document.AuditFilter{Action: "media.delete"})
	if err != nil {
		t.Fatal(err)
	}
	success, failure := 0, 0
	for _, audit := range audits {
		if audit.Detail != "" {
			t.Fatal("audit copied content")
		}
		if audit.Result == "success" {
			success++
		} else {
			failure++
		}
	}
	if success != 1 || failure < 3 {
		t.Fatal("missing delete audit", success, failure)
	}
}

func TestEditorMediaUploadSearchAndPublicDelivery(t *testing.T) {
	dataDir := t.TempDir()
	store, err := storage.OpenBadgerDocumentStore(dataDir + "/badger")
	if err != nil {
		t.Fatalf("open document store: %v", err)
	}
	defer store.Close()
	accounts, err := auth.Open(dataDir + "/auth")
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	defer accounts.Close()
	ctx := context.Background()
	_, err = accounts.Register(ctx, auth.Registration{
		Username: "media_editor", Password: "long-media-password", Permissions: []auth.Permission{auth.PermissionEdit},
	})
	if err != nil {
		t.Fatalf("register editor: %v", err)
	}
	_, token, _, err := accounts.Login(ctx, "media_editor", "long-media-password")
	if err != nil {
		t.Fatalf("login editor: %v", err)
	}
	srv := New(config.Config{DataDir: dataDir, MediaStorageLimitBytes: 20 << 20, MediaUploadLimitBytes: 2 << 20}, store, accounts, techmark.NewParser())

	body, contentType := mediaUploadBody(t)
	unauthorized := httptest.NewRequest(http.MethodPost, "/api/editor/media", bytes.NewReader(body))
	unauthorized.Header.Set("Content-Type", contentType)
	unauthorizedRecorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unauthorizedRecorder, unauthorized)
	if unauthorizedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected upload to require edit permission, got %d", unauthorizedRecorder.Code)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/editor/media", bytes.NewReader(body))
	request.Header.Set("Content-Type", contentType)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("upload media: %d: %s", recorder.Code, recorder.Body.String())
	}
	if bytes.Contains(recorder.Body.Bytes(), []byte(`"hash"`)) || bytes.Contains(recorder.Body.Bytes(), []byte(`"path"`)) {
		t.Fatalf("editor media response leaked internal storage details: %s", recorder.Body.String())
	}
	var uploaded mediaFileResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &uploaded); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if uploaded.Name != "Server_diagram.webp" || uploaded.ThumbnailURL == "" {
		t.Fatalf("unexpected upload response: %#v", uploaded)
	}

	search := httptest.NewRequest(http.MethodGet, "/api/editor/media?q=server", nil)
	search.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	searchRecorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(searchRecorder, search)
	if searchRecorder.Code != http.StatusOK || !bytes.Contains(searchRecorder.Body.Bytes(), []byte(`"Server_diagram.webp"`)) {
		t.Fatalf("search media: %d: %s", searchRecorder.Code, searchRecorder.Body.String())
	}

	imageRequest := httptest.NewRequest(http.MethodGet, uploaded.ThumbnailURL, nil)
	imageRecorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(imageRecorder, imageRequest)
	result := imageRecorder.Result()
	if imageRecorder.Code != http.StatusOK || result.Header.Get("Content-Type") != "image/webp" {
		t.Fatalf("serve media: %d %q", imageRecorder.Code, result.Header.Get("Content-Type"))
	}
	data := imageRecorder.Body.Bytes()
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		t.Fatal("served media is not WebP")
	}
}

func mediaUploadBody(t *testing.T) ([]byte, string) {
	t.Helper()
	value := image.NewNRGBA(image.Rect(0, 0, 128, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 128; x++ {
			value.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y * 2), B: 120, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, value); err != nil {
		t.Fatalf("encode upload fixture: %v", err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "diagram.png")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write(encoded.Bytes()); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	_ = writer.WriteField("name", "Server diagram.png")
	_ = writer.WriteField("license", "CC BY-SA 4.0")
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart body: %v", err)
	}
	return body.Bytes(), writer.FormDataContentType()
}
