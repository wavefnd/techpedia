package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"techpedia/internal/auth"
	"techpedia/internal/document"
	"techpedia/internal/oauth"
	"techpedia/internal/wiki"
)

func oauthSafeOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	ip := net.ParseIP(u.Hostname())
	return u.Scheme == "http" && (u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback()))
}

func redactDiagnosticURL(value string) string {
	u, err := url.Parse(value)
	if err != nil {
		return "invalid-url"
	}
	if u.Scheme != "" && u.Scheme != "http" && u.Scheme != "https" {
		return u.Scheme + ":"
	}
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	u.RawFragment = ""
	u.User = nil
	return u.String()
}

func (s *Server) oauthRoutes() {
	s.proposalRoutes()
	s.mux.HandleFunc("GET /admin/connections", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		s.handleIndex(w, r)
	})
	s.mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.oauthResource)
	s.mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", s.oauthResource)
	s.mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.oauthMetadata)
	s.mux.HandleFunc("GET /oauth/authorize", s.oauthAuthorize)
	s.mux.HandleFunc("POST /oauth/token", s.oauthToken)
	s.mux.HandleFunc("GET /api/admin/oauth", s.oauthAdmin)
	s.mux.HandleFunc("POST /api/admin/oauth/clients", s.oauthRegister)
	s.mux.HandleFunc("POST /api/admin/oauth/clients/{id}/disable", s.oauthDisable)
	s.mux.HandleFunc("POST /api/admin/oauth/requests", s.oauthPrepare)
	s.mux.HandleFunc("POST /api/admin/oauth/requests/{id}/decide", s.oauthDecide)
	s.mux.HandleFunc("POST /api/admin/oauth/grants/{id}/revoke", s.oauthRevoke)
}
func (s *Server) oauthEnabled(w http.ResponseWriter) bool {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if s.oauth == nil {
		http.Error(w, "ChatGPT 연결이 비활성화되어 있습니다.", http.StatusNotFound)
		return false
	}
	return true
}
func (s *Server) oauthResource(w http.ResponseWriter, r *http.Request) {
	if !s.oauthEnabled(w) {
		return
	}
	writeJSON(w, 200, map[string]any{"resource": s.publicURL + "/mcp", "authorization_servers": []string{s.publicURL}, "scopes_supported": []string{oauth.Scope, oauth.ProposeScope}, "bearer_methods_supported": []string{"header"}})
}
func (s *Server) oauthMetadata(w http.ResponseWriter, r *http.Request) {
	if !s.oauthEnabled(w) {
		return
	}
	writeJSON(w, 200, map[string]any{"issuer": s.publicURL, "authorization_endpoint": s.publicURL + "/oauth/authorize", "token_endpoint": s.publicURL + "/oauth/token", "response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"}, "token_endpoint_auth_methods_supported": []string{"none"}, "code_challenge_methods_supported": []string{"S256"}, "scopes_supported": []string{oauth.Scope, oauth.ProposeScope}, "authorization_response_iss_parameter_supported": true})
}
func authorizationFrom(q url.Values) oauth.Authorization {
	return oauth.Authorization{ClientID: q.Get("client_id"), RedirectURI: q.Get("redirect_uri"), ResponseType: q.Get("response_type"), Challenge: q.Get("code_challenge"), ChallengeMethod: q.Get("code_challenge_method"), Scope: q.Get("scope"), Resource: q.Get("resource"), State: q.Get("state")}
}
func (s *Server) oauthAuthorize(w http.ResponseWriter, r *http.Request) {
	if !s.oauthEnabled(w) {
		return
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 4096 || hasDuplicateOAuthParameters(q) {
		oauthFailure(w)
		return
	}
	if _, err = s.oauth.Validate(r.Context(), authorizationFrom(q)); err != nil {
		oauthFailure(w)
		return
	}
	// No grant is issued on GET. Login and explicit approval stay in the admin UI.
	http.Redirect(w, r, "/admin/connections?"+q.Encode(), http.StatusSeeOther)
}
func hasDuplicateOAuthParameters(q url.Values) bool {
	for _, v := range q {
		if len(v) != 1 {
			return true
		}
	}
	return false
}
func oauthFailure(w http.ResponseWriter) {
	writeJSON(w, 400, map[string]string{"error": "invalid_request", "message": "연결 요청 또는 인증 정보를 확인하세요."})
}
func (s *Server) grantAdministrator(ctx context.Context, g oauth.Grant) error {
	u, err := s.accounts.GetProfile(ctx, g.Username)
	if err != nil || u.ID != g.UserID || !u.Has(auth.PermissionAdmin) {
		return oauth.ErrInvalid
	}
	return nil
}
func (s *Server) oauthToken(w http.ResponseWriter, r *http.Request) {
	if !s.oauthEnabled(w) {
		return
	}
	if !s.allowOAuthRequest() {
		w.Header().Set("Retry-After", "60")
		writeJSON(w, 429, map[string]string{"error": "temporarily_unavailable"})
		return
	}
	if r.URL.RawQuery != "" || strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/x-www-form-urlencoded" {
		oauthFailure(w)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := r.ParseForm(); err != nil || hasDuplicateOAuthParameters(r.PostForm) {
		oauthFailure(w)
		return
	}
	f := r.PostForm
	token := f.Get("code")
	if f.Get("grant_type") == "refresh_token" {
		token = f.Get("refresh_token")
	}
	tokens, err := s.oauth.Exchange(r.Context(), f.Get("grant_type"), token, f.Get("client_id"), f.Get("redirect_uri"), f.Get("code_verifier"), f.Get("resource"), f.Get("scope"), func(g oauth.Grant) error { return s.grantAdministrator(r.Context(), g) })
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
		return
	}
	writeJSON(w, 200, tokens)
}

// Global budget avoids persisting visitors' addresses or trusting proxy IP headers.
func (s *Server) allowOAuthRequest() bool {
	s.oauthLimitMu.Lock()
	defer s.oauthLimitMu.Unlock()
	if time.Since(s.oauthLimitAt) >= time.Minute {
		s.oauthLimitAt = time.Now()
		s.oauthLimitCount = 0
	}
	if s.oauthLimitCount >= 120 {
		return false
	}
	s.oauthLimitCount++
	return true
}
func (s *Server) authorizeMCP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		parts := strings.Fields(r.Header.Get("Authorization"))
		var token string
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			token = parts[1]
		}
		grant, err := s.oauth.Access(r.Context(), token)
		if err != nil || s.grantAdministrator(r.Context(), grant) != nil {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+s.publicURL+`/.well-known/oauth-protected-resource", scope="`+oauth.Scope+`"`)
			writeJSON(w, 401, map[string]string{"error": "invalid_token"})
			return
		}
		next.ServeHTTP(w, r.WithContext(wiki.WithConnection(r.Context(), grant)))
	})
}
func (s *Server) oauthAdministrator(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	if !s.oauthEnabled(w) {
		return auth.User{}, false
	}
	u, ok := s.requirePermission(w, r, auth.PermissionAdmin)
	if !ok {
		return u, false
	}
	if r.Method != "GET" {
		if r.Header.Get("Origin") != s.publicURL || strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
			writeJSON(w, 403, map[string]string{"error": "관리자 화면에서만 연결을 변경할 수 있습니다."})
			return auth.User{}, false
		}
	}
	return u, true
}
func oauthDecode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		oauthFailure(w)
		return false
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		oauthFailure(w)
		return false
	}
	return true
}
func (s *Server) oauthAudit(r *http.Request, u auth.User, action, target, result string) error {
	_, err := s.store.AppendAudit(r.Context(), document.AuditEntry{Actor: u.Username, Action: "oauth." + action, TargetType: "oauth", Target: target, Result: result})
	return err
}
func (s *Server) oauthAdmin(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requirePermission(w, r, auth.PermissionAdmin)
	if !ok {
		return
	}
	if s.oauth == nil {
		writeJSON(w, 200, map[string]any{"enabled": false, "clients": []oauth.Client{}, "grants": []oauth.Grant{}, "resource": s.publicURL + "/mcp", "scope": oauth.Scope, "proposal_scope": oauth.ProposalScopes})
		return
	}
	clients, err := s.oauth.Clients(r.Context())
	if err != nil {
		http.Error(w, "연결 목록을 불러오지 못했습니다.", 500)
		return
	}
	grants, err := s.oauth.Grants(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "연결 목록을 불러오지 못했습니다.", 500)
		return
	}
	writeJSON(w, 200, map[string]any{"enabled": true, "clients": clients, "grants": grants, "resource": s.publicURL + "/mcp", "scope": oauth.Scope, "proposal_scope": oauth.ProposalScopes})
}
func (s *Server) oauthRegister(w http.ResponseWriter, r *http.Request) {
	u, ok := s.oauthAdministrator(w, r)
	if !ok {
		return
	}
	var req struct {
		Name     string `json:"name"`
		Redirect string `json:"redirect_uri"`
	}
	if !oauthDecode(w, r, &req) {
		return
	}
	if s.oauthAudit(r, u, "client_register", "client", "attempt") != nil {
		http.Error(w, "감사 로그를 기록하지 못했습니다.", 503)
		return
	}
	c, err := s.oauth.Register(r.Context(), strings.TrimSpace(req.Name), req.Redirect, oauth.Actor{ID: u.ID, Username: u.Username})
	if err != nil {
		_ = s.oauthAudit(r, u, "client_register", "client", "failure")
		oauthFailure(w)
		return
	}
	writeJSON(w, 201, c)
}
func (s *Server) oauthDisable(w http.ResponseWriter, r *http.Request) {
	u, ok := s.oauthAdministrator(w, r)
	if !ok {
		return
	}
	var req struct {
		Confirmation string `json:"confirmation"`
	}
	if !oauthDecode(w, r, &req) {
		return
	}
	id := r.PathValue("id")
	if req.Confirmation != id {
		oauthFailure(w)
		return
	}
	s.oauthMutation(w, r, u, "client_disable", id, func() error { return s.oauth.Disable(r.Context(), id, oauth.Actor{ID: u.ID, Username: u.Username}) })
}
func (s *Server) oauthPrepare(w http.ResponseWriter, r *http.Request) {
	u, ok := s.oauthAdministrator(w, r)
	if !ok {
		return
	}
	var req oauth.Authorization
	if !oauthDecode(w, r, &req) {
		return
	}
	p, c, err := s.oauth.Prepare(r.Context(), req, oauth.Actor{ID: u.ID, Username: u.Username})
	if err != nil {
		oauthFailure(w)
		return
	}
	writeJSON(w, 201, map[string]any{"request": p, "client": c})
}
func (s *Server) oauthDecide(w http.ResponseWriter, r *http.Request) {
	u, ok := s.oauthAdministrator(w, r)
	if !ok {
		return
	}
	var req struct {
		Allow bool `json:"allow"`
	}
	if !oauthDecode(w, r, &req) {
		return
	}
	id := r.PathValue("id")
	action := "approve"
	if !req.Allow {
		action = "deny"
	}
	if s.oauthAudit(r, u, action, "connection", "attempt") != nil {
		http.Error(w, "감사 로그를 기록하지 못했습니다.", 503)
		return
	}
	redirect, err := s.oauth.Decide(r.Context(), id, oauth.Actor{ID: u.ID, Username: u.Username}, req.Allow)
	if err != nil {
		_ = s.oauthAudit(r, u, action, "connection", "failure")
		oauthFailure(w)
		return
	}
	writeJSON(w, 200, map[string]string{"redirect_url": redirect})
}
func (s *Server) oauthRevoke(w http.ResponseWriter, r *http.Request) {
	u, ok := s.oauthAdministrator(w, r)
	if !ok {
		return
	}
	var req struct {
		Confirmation string `json:"confirmation"`
	}
	if !oauthDecode(w, r, &req) {
		return
	}
	id := r.PathValue("id")
	if req.Confirmation != id {
		oauthFailure(w)
		return
	}
	s.oauthMutation(w, r, u, "revoke", id, func() error { return s.oauth.Revoke(r.Context(), id, oauth.Actor{ID: u.ID, Username: u.Username}) })
}
func (s *Server) oauthMutation(w http.ResponseWriter, r *http.Request, u auth.User, action, id string, fn func() error) {
	// Never copy user input, callback query, state, authorization codes or tokens into audit logs.
	if s.oauthAudit(r, u, action, "connection", "attempt") != nil {
		http.Error(w, "감사 로그를 기록하지 못했습니다.", 503)
		return
	}
	if err := fn(); err != nil {
		_ = s.oauthAudit(r, u, action, "connection", "failure")
		oauthFailure(w)
		return
	}
	w.WriteHeader(204)
}
