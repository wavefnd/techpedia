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
	"techpedia/internal/storage"
	"techpedia/internal/techmark"
)

func TestAnonymousAnalyticsCollectionAndAdminSummary(t *testing.T) {
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
	if _, err := accounts.Register(context.Background(), auth.Registration{
		Username: "analytics_admin", Password: "long-admin-password", Permissions: []auth.Permission{auth.PermissionAdmin},
	}); err != nil {
		t.Fatal(err)
	}
	_, token, _, err := accounts.Login(context.Background(), "analytics_admin", "long-admin-password")
	if err != nil {
		t.Fatal(err)
	}

	srv := New(config.Config{AnalyticsEnabled: true, AnalyticsCountryHeader: "CF-IPCountry"}, store, accounts, techmark.NewParser())
	ping := httptest.NewRecorder()
	srv.Handler().ServeHTTP(ping, httptest.NewRequest(http.MethodGet, "/api/analytics/ping", nil))
	if ping.Code != http.StatusNoContent || ping.Header().Get(analyticsEnabledHeader) != "enabled" || ping.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected analytics ping: %d %#v", ping.Code, ping.Header())
	}

	record := func(body string, country string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/analytics/visits", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("CF-IPCountry", country)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}
	if rec := record(`{"route":"article","locale":"ko","rtt_ms":84}`, "KR"); rec.Code != http.StatusNoContent {
		t.Fatalf("record article visit: %d: %s", rec.Code, rec.Body.String())
	}
	if rec := record(`{"route":"portal","rtt_ms":126}`, "US"); rec.Code != http.StatusNoContent {
		t.Fatalf("record portal visit: %d: %s", rec.Code, rec.Body.String())
	}
	if rec := record(`{"route":"/ko/wiki/secret","locale":"ko"}`, "KR"); rec.Code != http.StatusBadRequest {
		t.Fatalf("exact path was accepted as route kind: %d", rec.Code)
	}

	unauthorized := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/admin/analytics", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("public analytics summary: %d", unauthorized.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics?days=7", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("admin analytics summary: %d: %s", rec.Code, rec.Body.String())
	}
	var response adminAnalyticsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.TotalViews != 2 || response.AverageRTTMS != 105 || response.RTTSamples != 2 {
		t.Fatalf("unexpected analytics response: %#v", response)
	}
	if len(response.Countries) != 2 || response.Countries[0].Views != 1 || len(response.Daily) != 7 {
		t.Fatalf("unexpected analytics breakdown: %#v", response)
	}
}

func TestAnalyticsCanBeDisabled(t *testing.T) {
	srv := New(config.Config{}, emptyStore{}, emptyAuth{}, techmark.NewParser())
	ping := httptest.NewRecorder()
	srv.Handler().ServeHTTP(ping, httptest.NewRequest(http.MethodGet, "/api/analytics/ping", nil))
	if ping.Code != http.StatusNoContent || ping.Header().Get(analyticsEnabledHeader) != "" {
		t.Fatalf("disabled analytics ping: %d %#v", ping.Code, ping.Header())
	}
	req := httptest.NewRequest(http.MethodPost, "/api/analytics/visits", strings.NewReader(`{"route":"article","locale":"ko","rtt_ms":50}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("disabled analytics submission: %d", rec.Code)
	}
}
