package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"techpedia/internal/auth"
	"techpedia/internal/config"
	"techpedia/internal/techmark"
)

type originTestAccounts struct {
	emptyAuth
	calls    int
	loginErr error
}

func (a *originTestAccounts) Register(context.Context, auth.Registration) (auth.User, error) {
	a.calls++
	return auth.User{Username: "test"}, nil
}

func (a *originTestAccounts) Login(context.Context, string, string) (auth.User, string, time.Time, error) {
	a.calls++
	if a.loginErr != nil {
		return auth.User{}, "", time.Time{}, a.loginErr
	}
	return auth.User{Username: "test"}, "test-session", time.Now().Add(time.Hour), nil
}

func TestSignupBusyAfterRegistrationExplainsAccountWasCreated(t *testing.T) {
	a := &originTestAccounts{loginErr: auth.ErrBusy}
	s := New(config.Config{PublicURL: "https://wiki.example"}, emptyStore{}, a, techmark.NewParser())
	r := httptest.NewRequest("POST", "/api/auth/signup", strings.NewReader(`{"username":"test","password":"test-password"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusTooManyRequests || a.calls != 2 || w.Header().Get("Retry-After") != "1" || !strings.Contains(w.Body.String(), "account created") || len(w.Result().Cookies()) != 0 {
		t.Fatalf("incorrect partial signup response: %d %s", w.Code, w.Body.String())
	}
}

func TestAuthenticationRejectsCrossOriginAndPlainTextBeforeBackend(t *testing.T) {
	for _, endpoint := range []string{"login", "signup"} {
		for _, tc := range []struct {
			name, origin, mediaType, site string
			status                        int
		}{
			{"browser", "https://wiki.example", "application/json", "same-origin", 200},
			{"charset", "https://wiki.example", "application/json; charset=utf-8", "", 200},
			{"nonbrowser", "", "application/json", "", 200},
			{"foreign", "https://attacker.example", "application/json", "", 403},
			{"null", "null", "application/json", "", 403},
			{"fetch-metadata", "", "application/json", "cross-site", 403},
			{"plain-form", "", "text/plain", "", 415},
		} {
			t.Run(endpoint+"/"+tc.name, func(t *testing.T) {
				a := &originTestAccounts{}
				s := New(config.Config{PublicURL: "https://wiki.example"}, emptyStore{}, a, techmark.NewParser())
				r := httptest.NewRequest("POST", "/api/auth/"+endpoint, strings.NewReader(`{"username":"test","password":"test-password","padding":"="}`))
				r.Header.Set("Content-Type", tc.mediaType)
				r.Header.Set("Origin", tc.origin)
				r.Header.Set("Sec-Fetch-Site", tc.site)
				w := httptest.NewRecorder()
				s.Handler().ServeHTTP(w, r)
				want := tc.status
				if want == 200 && endpoint == "signup" {
					want = 201
				}
				if w.Code != want {
					t.Fatalf("status %d, want %d: %s", w.Code, want, w.Body.String())
				}
				if want >= 400 && (a.calls != 0 || len(w.Result().Cookies()) != 0) {
					t.Fatal("rejected request reached credentials or changed session")
				}
				if want < 400 && len(w.Result().Cookies()) != 1 {
					t.Fatal("legitimate authentication lost session")
				}
			})
		}
	}
}

func TestLogoutRejectsForeignOrigin(t *testing.T) {
	s := New(config.Config{PublicURL: "https://wiki.example"}, emptyStore{}, emptyAuth{}, techmark.NewParser())
	r := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	r.Header.Set("Origin", "https://attacker.example")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 || len(w.Result().Cookies()) != 0 {
		t.Fatal("foreign logout altered session", w.Code)
	}
}
