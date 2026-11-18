package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"techpedia/internal/auth"
	"techpedia/internal/config"
	"techpedia/internal/document"
	"techpedia/internal/oauth"
	"techpedia/internal/storage"
	"techpedia/internal/techmark"
)

type oauthTestAccounts struct {
	emptyAuth
	user auth.User
}

func (a *oauthTestAccounts) Authenticate(_ context.Context, token string) (auth.User, error) {
	if token != "admin-session" {
		return auth.User{}, auth.ErrSessionNotFound
	}
	return a.user, nil
}
func (a *oauthTestAccounts) GetProfile(_ context.Context, name string) (auth.User, error) {
	if name != a.user.Username {
		return auth.User{}, auth.ErrUserNotFound
	}
	return a.user, nil
}
func oauthTestServer(t *testing.T) (*Server, *oauthTestAccounts) {
	t.Helper()
	db, err := storage.OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	a := &oauthTestAccounts{user: auth.User{ID: "admin-id", Username: "admin", Permissions: []auth.Permission{auth.PermissionAdmin}}}
	return New(config.Config{MCPEnabled: true, PublicURL: "https://wiki.example"}, db, a, techmark.NewParser()), a
}

func TestOAuthTransportAndTokenBudget(t *testing.T) {
	for origin, want := range map[string]bool{
		"https://wiki.example": true, "http://wiki.example": false,
		"http://localhost:18081": true, "http://127.0.0.1:18081": true,
		"http://[::1]:18081": true, "http://192.0.2.1": false,
		"ftp://wiki.example": false,
	} {
		if got := oauthSafeOrigin(origin); got != want {
			t.Errorf("origin %q: got %v, want %v", origin, got, want)
		}
	}
	s, _ := oauthTestServer(t)
	s.oauthLimitAt = time.Now()
	for i := 0; i < 120; i++ {
		if !s.allowOAuthRequest() {
			t.Fatalf("budget rejected request %d", i)
		}
	}
	w := oauthRequest(s, "POST", "/oauth/token", "", "", "")
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "60" {
		t.Fatalf("missing token budget response: %d", w.Code)
	}
	s.oauthLimitAt = time.Now().Add(-2 * time.Minute)
	if !s.allowOAuthRequest() {
		t.Fatal("budget did not recover")
	}
}

func TestDisabledOAuthStatusIsAdministratorOnly(t *testing.T) {
	s, accounts := oauthTestServer(t)
	s.oauth = nil
	s.mcpHandler = nil
	if w := oauthRequest(s, "GET", "/api/admin/oauth", "", "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	accounts.user.Permissions = []auth.Permission{auth.PermissionEdit}
	if w := oauthRequest(s, "GET", "/api/admin/oauth", "", "admin-session", ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	accounts.user.Permissions = []auth.Permission{auth.PermissionAdmin}
	w := oauthRequest(s, "GET", "/api/admin/oauth", "", "admin-session", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"enabled":false`) || !strings.Contains(w.Body.String(), `"clients":[]`) {
		t.Fatalf("status: %d %s", w.Code, w.Body.String())
	}
	for _, path := range []string{"/mcp", "/.well-known/oauth-authorization-server"} {
		if w := oauthRequest(s, "GET", path, "", "", ""); w.Code != 404 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	if w := oauthRequest(s, "POST", "/api/admin/oauth/clients", `{}`, "admin-session", "https://wiki.example"); w.Code != 404 {
		t.Fatal(w.Code)
	}
}
func oauthRequest(s *Server, method, path, body, session, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://wiki.example"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if session != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session})
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func testAuthorization(t *testing.T, s *Server) (oauth.Client, oauth.Authorization, string) {
	t.Helper()
	client, err := s.oauth.Register(context.Background(), "Test ChatGPT", "https://chatgpt.example/callback", oauth.Actor{ID: "admin-id", Username: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	verifier := strings.Repeat("v", 43)
	sum := sha256.Sum256([]byte(verifier))
	return client, oauth.Authorization{ClientID: client.ID, RedirectURI: client.RedirectURI, ResponseType: "code", Challenge: base64.RawURLEncoding.EncodeToString(sum[:]), ChallengeMethod: "S256", Scope: oauth.Scope, Resource: "https://wiki.example/mcp", State: "opaque-state"}, verifier
}
func testOAuthTokens(t *testing.T, s *Server) oauth.Tokens {
	t.Helper()
	client, a, verifier := testAuthorization(t, s)
	u, _ := s.accounts.GetProfile(context.Background(), "admin")
	p, _, err := s.oauth.Prepare(context.Background(), a, oauth.Actor{ID: u.ID, Username: u.Username})
	if err != nil {
		t.Fatal(err)
	}
	redirect, err := s.oauth.Decide(context.Background(), p.ID, oauth.Actor{ID: u.ID, Username: u.Username}, true)
	if err != nil {
		t.Fatal(err)
	}
	callback, _ := url.Parse(redirect)
	out, err := s.oauth.Exchange(context.Background(), "authorization_code", callback.Query().Get("code"), client.ID, client.RedirectURI, verifier, a.Resource, "", func(g oauth.Grant) error { return s.grantAdministrator(context.Background(), g) })
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestOAuthAdministratorPermissionAndCSRF(t *testing.T) {
	s, accounts := oauthTestServer(t)
	for _, permission := range []auth.Permission{auth.PermissionEdit, auth.PermissionTechdata, auth.PermissionAdmin, auth.PermissionOwner} {
		accounts.user.Permissions = []auth.Permission{permission}
		w := oauthRequest(s, "GET", "/api/admin/oauth", "", "admin-session", "")
		want := 403
		if permission == auth.PermissionAdmin || permission == auth.PermissionOwner {
			want = 200
		}
		if w.Code != want {
			t.Fatalf("%s: %d %s", permission, w.Code, w.Body.String())
		}
		if want == 200 && !strings.Contains(w.Body.String(), `"enabled":true`) {
			t.Fatal("enabled OAuth status is missing")
		}
	}
	accounts.user.Permissions = []auth.Permission{auth.PermissionAdmin}
	for _, origin := range []string{"", "https://attacker.example"} {
		w := oauthRequest(s, "POST", "/api/admin/oauth/clients", `{"name":"Test","redirect_uri":"https://chatgpt.example/callback"}`, "admin-session", origin)
		if w.Code != 403 {
			t.Fatalf("CSRF %q: %d", origin, w.Code)
		}
	}
	w := oauthRequest(s, "POST", "/api/admin/oauth/clients", `{"name":"Test","redirect_uri":"https://chatgpt.example/callback"}`, "admin-session", "https://wiki.example")
	if w.Code != 201 {
		t.Fatalf("registration: %d %s", w.Code, w.Body.String())
	}
	if w := oauthRequest(s, "GET", "/api/admin/oauth", "", "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
}
func TestMCPRequiresBearerAndCurrentAdministrator(t *testing.T) {
	s, accounts := oauthTestServer(t)
	tokens := testOAuthTokens(t, s)
	call := func(token, cookie string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "https://wiki.example/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
		r.Header.Set("Accept", "application/json, text/event-stream")
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	for _, token := range []string{"", "invalid", tokens.RefreshToken} {
		w := call(token, "admin-session")
		if w.Code != 401 || !strings.Contains(w.Header().Get("WWW-Authenticate"), "oauth-protected-resource") {
			t.Fatalf("unauthorized: %d %s", w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("GET", "https://wiki.example/api/admin/oauth", nil)
	r.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("MCP token authorized admin cookie API")
	}
	if w := call(tokens.AccessToken, ""); w.Code != 200 || !strings.Contains(w.Body.String(), "oauth2") {
		t.Fatalf("admin token: %d %s", w.Code, w.Body.String())
	}
	accounts.user.Permissions = []auth.Permission{auth.PermissionEdit}
	if w := call(tokens.AccessToken, ""); w.Code != 401 {
		t.Fatalf("permission removed: %d", w.Code)
	}
	accounts.user.Permissions = []auth.Permission{auth.PermissionAdmin}
	accounts.user.ID = "different-account"
	if w := call(tokens.AccessToken, ""); w.Code != 401 {
		t.Fatalf("account replacement: %d", w.Code)
	}
}

func TestOAuthParametersExcludedFromHTTPAndCSPLogs(t *testing.T) {
	s, _ := oauthTestServer(t)
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	for _, path := range []string{"/oauth/authorize?state=secret-state", "/login?redirect=secret-state", "/admin/connections?state=secret-state", "/mcp?access_token=secret-state"} {
		oauthRequest(s, "GET", path, "", "", "")
	}
	if strings.Contains(output.String(), "secret-state") {
		t.Fatal("OAuth query logged")
	}
	for input, want := range map[string]string{"https://user:password@wiki.example/oauth/authorize?state=secret#fragment": "https://wiki.example/oauth/authorize", "data:text/html,secret": "data:", "inline": "inline"} {
		if got := redactDiagnosticURL(input); got != want {
			t.Fatalf("redact %q: %q", input, got)
		}
	}
}
func TestOAuthDiscoveryAndConsentHTTPFlow(t *testing.T) {
	s, _ := oauthTestServer(t)
	client, a, verifier := testAuthorization(t, s)
	for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-authorization-server"} {
		w := oauthRequest(s, "GET", path, "", "", "")
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("metadata %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	q := url.Values{"client_id": {client.ID}, "redirect_uri": {client.RedirectURI}, "response_type": {"code"}, "code_challenge": {a.Challenge}, "code_challenge_method": {"S256"}, "scope": {oauth.Scope}, "resource": {a.Resource}, "state": {a.State}}
	w := oauthRequest(s, "GET", "/oauth/authorize?"+q.Encode(), "", "", "")
	if w.Code != 303 || !strings.HasPrefix(w.Header().Get("Location"), "/admin/connections?") {
		t.Fatalf("consent redirect: %d %s", w.Code, w.Body.String())
	}
	grants, _ := s.oauth.Grants(context.Background(), "admin-id")
	if len(grants) != 0 {
		t.Fatal("GET issued a grant")
	}
	q.Set("redirect_uri", "https://attacker.example/callback")
	w = oauthRequest(s, "GET", "/oauth/authorize?"+q.Encode(), "", "", "")
	if w.Code != 400 || w.Header().Get("Location") != "" {
		t.Fatal("unregistered redirect accepted")
	}
	body, _ := json.Marshal(a)
	w = oauthRequest(s, "POST", "/api/admin/oauth/requests", string(body), "admin-session", "https://wiki.example")
	var prepared struct{ Request oauth.Approval }
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &prepared) != nil {
		t.Fatalf("prepare: %d %s", w.Code, w.Body.String())
	}
	w = oauthRequest(s, "POST", "/api/admin/oauth/requests/"+prepared.Request.ID+"/decide", `{"allow":true}`, "admin-session", "https://wiki.example")
	var redirect struct {
		URL string `json:"redirect_url"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &redirect) != nil {
		t.Fatalf("approve: %d %s", w.Code, w.Body.String())
	}
	u, _ := url.Parse(redirect.URL)
	if u.Query().Get("iss") != "https://wiki.example" || u.Query().Get("state") != a.State {
		t.Fatal("callback state/issuer missing")
	}
	f := url.Values{"grant_type": {"authorization_code"}, "client_id": {client.ID}, "code": {u.Query().Get("code")}, "redirect_uri": {client.RedirectURI}, "code_verifier": {verifier}, "resource": {a.Resource}}
	r := httptest.NewRequest("POST", "https://wiki.example/oauth/token", strings.NewReader(f.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	var tokens oauth.Tokens
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &tokens) != nil || tokens.AccessToken == "" {
		t.Fatalf("exchange: %d %s", w.Code, w.Body.String())
	}
	grant, err := s.oauth.Access(context.Background(), tokens.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	w = oauthRequest(s, "POST", "/api/admin/oauth/grants/"+grant.ID+"/revoke", `{"confirmation":"wrong"}`, "admin-session", "https://wiki.example")
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	w = oauthRequest(s, "POST", "/api/admin/oauth/grants/"+grant.ID+"/revoke", `{"confirmation":"`+grant.ID+`"}`, "admin-session", "https://wiki.example")
	if w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err := s.oauth.Access(context.Background(), tokens.AccessToken); err == nil {
		t.Fatal("revoked access accepted")
	}
	entries, err := s.store.ListAudit(context.Background(), document.AuditFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(entries)
	for _, secret := range []string{a.State, verifier, u.Query().Get("code"), tokens.AccessToken, tokens.RefreshToken, client.RedirectURI} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("audit leaked OAuth data")
		}
	}
}
