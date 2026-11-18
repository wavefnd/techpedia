package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"techpedia/internal/auth"
	"techpedia/internal/changeproposal"
	"techpedia/internal/document"
	"techpedia/internal/oauth"
	"techpedia/internal/storage"
	"techpedia/internal/techmark"
)

func TestProposalPolicyUsesAdminSessionAndShortRetention(t *testing.T) {
	s, accounts := oauthTestServer(t)
	path := "/api/admin/mcp/proposal-policy"
	if w := oauthRequest(s, "GET", path, "", "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	accounts.user.Permissions = []auth.Permission{auth.PermissionEdit}
	if w := oauthRequest(s, "GET", path, "", "admin-session", ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	accounts.user.Permissions = []auth.Permission{auth.PermissionAdmin}
	for _, test := range []struct {
		body, origin string
		code         int
	}{
		{`{"retention_days":1}`, "https://evil.example", 403},
		{`{"retention_days":7}`, "https://wiki.example", 400},
		{`{"retention_days":0}`, "https://wiki.example", 400},
		{`{"retention_days":1}`, "https://wiki.example", 200},
		{`{"retention_days":3}`, "https://wiki.example", 200},
	} {
		w := oauthRequest(s, "PUT", path, test.body, "admin-session", test.origin)
		if w.Code != test.code {
			t.Fatal(test, w.Code, w.Body.String())
		}
	}
	s.oauth = nil
	w := oauthRequest(s, "GET", path, "", "admin-session", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"retention_days":3`) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestBatchProposalReviewIsExplicitAndChecksEachSnapshot(t *testing.T) {
	s, _ := oauthTestServer(t)
	ctx := context.Background()
	db := s.store.(*storage.BadgerDocumentStore)
	for _, locale := range []string{"ko", "en"} {
		configureTestEdition(t, db, locale, "Main")
		if _, err := db.Save(ctx, document.Document{Locale: locale, Slug: techmark.Slugify("Main"), Title: "Main", Source: "main"}); err != nil {
			t.Fatal(err)
		}
	}
	tokens := proposalToken(t, s, oauth.ProposalScopes)
	g, err := s.oauth.Access(ctx, tokens.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	submit := func(in changeproposal.Input) changeproposal.Proposal {
		t.Helper()
		p, err := db.SubmitChangeProposal(ctx, in, g.ID, g.UserID, g.Username)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	create := submit(changeproposal.Input{Kind: "create", Locale: "ko", Name: "정상 생성", Source: "new source", Summary: "create"})
	tampered := submit(changeproposal.Input{Kind: "create", Locale: "ko", Name: "변조 확인", Source: "not approved", Summary: "create"})
	base, err := db.Save(ctx, document.Document{Locale: "ko", Slug: "기존", Title: "기존", Source: "before"})
	if err != nil {
		t.Fatal(err)
	}
	stale := submit(changeproposal.Input{Kind: "edit", Locale: "ko", Name: base.Title, BaseRevision: base.Revision, Source: "proposed", Summary: "edit"})
	base.Source = "concurrent edit"
	if _, err := db.Save(ctx, base); err != nil {
		t.Fatal(err)
	}
	target, err := db.Save(ctx, document.Document{Locale: "en", Slug: techmark.Slugify("Target"), Title: "Target", Source: "en"})
	if err != nil {
		t.Fatal(err)
	}
	link := submit(changeproposal.Input{Kind: "link", Locale: "ko", Name: "Main", BaseRevision: "r1", Target: &changeproposal.Page{Locale: "en", Name: target.Title, Revision: target.Revision}, Summary: "link"})
	entries := []map[string]string{}
	for _, p := range []changeproposal.Proposal{create, tampered, stale, link} {
		digest := p.Digest()
		if p.ID == tampered.ID {
			digest = strings.Repeat("0", 64)
		}
		entries = append(entries, map[string]string{"id": p.ID, "review_digest": digest})
	}
	body, _ := json.Marshal(map[string]any{"allow": true, "confirmation": "approve:4", "proposals": entries})
	path := "/api/admin/mcp/proposals/decide-batch"
	if w := oauthRequest(s, "POST", path, string(body), "", "https://wiki.example"); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := oauthRequest(s, "POST", path, string(body), "admin-session", "https://evil.example"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	for _, invalid := range []any{
		map[string]any{"allow": true, "confirmation": "approve:5", "proposals": entries},
		map[string]any{"allow": true, "confirmation": "approve:2", "proposals": []map[string]string{entries[0], entries[0]}},
		map[string]any{"allow": true, "confirmation": "approve:0", "proposals": []any{}},
		map[string]any{"allow": true, "confirmation": "approve:1", "proposals": []map[string]string{{"id": create.ID}}},
	} {
		raw, _ := json.Marshal(invalid)
		if w := oauthRequest(s, "POST", path, string(raw), "admin-session", "https://wiki.example"); w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	w := oauthRequest(s, "POST", path, string(body), "admin-session", "https://wiki.example")
	var out struct {
		Results []struct {
			ID       string
			Proposal *proposalSummary
			Error    string
		}
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out.Results) != 4 {
		t.Fatal(w.Code, w.Body.String())
	}
	for i, result := range out.Results {
		if i == 0 || i == 3 {
			if result.Proposal == nil || result.Proposal.Status != "applied" || result.Error != "" {
				t.Fatal(result)
			}
		} else if result.Proposal != nil || result.Error == "" {
			t.Fatal("bad snapshot applied", result)
		}
	}
	if _, err := db.GetLatest(ctx, "ko", techmark.Slugify(tampered.Input.Name)); err != storage.ErrDocumentNotFound {
		t.Fatal("tampered proposal applied", err)
	}
	latest, _ := db.GetLatest(ctx, "ko", base.Slug)
	if latest.Source != "concurrent edit" {
		t.Fatal("stale proposal applied")
	}
	linked, err := db.GetItemByWikiPage(ctx, target.ID)
	if err != nil || linked.WikiPages["ko"] == "" {
		t.Fatal("batch language link failed", err)
	}
	// Failure entries remain rejectable, with their separately reviewed digests.
	reject, _ := json.Marshal(map[string]any{"allow": false, "confirmation": "reject:2", "proposals": []map[string]string{{"id": tampered.ID, "review_digest": tampered.Digest()}, {"id": stale.ID, "review_digest": stale.Digest()}}})
	w = oauthRequest(s, "POST", path, string(reject), "admin-session", "https://wiki.example")
	if w.Code != 200 || strings.Count(w.Body.String(), `"status":"rejected"`) != 2 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err := db.GetLatest(ctx, "ko", techmark.Slugify(create.Input.Name)); err != nil {
		t.Fatal("other failures rolled back approved document", err)
	}
}

func TestBatchProposalRejectsOversizedBatchBeforeMutation(t *testing.T) {
	s, _ := oauthTestServer(t)
	entries := make([]map[string]string, 101)
	for i := range entries {
		entries[i] = map[string]string{"id": fmt.Sprint(i), "review_digest": strings.Repeat("a", 64)}
	}
	body, _ := json.Marshal(map[string]any{"allow": true, "confirmation": "approve:101", "proposals": entries})
	w := oauthRequest(s, "POST", "/api/admin/mcp/proposals/decide-batch", string(body), "admin-session", "https://wiki.example")
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}

func proposalToken(t *testing.T, s *Server, scope string) oauth.Tokens {
	t.Helper()
	ctx := context.Background()
	c, a, verifier := testAuthorization(t, s)
	a.Scope = scope
	u, err := s.accounts.GetProfile(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := s.oauth.Prepare(ctx, a, oauth.Actor{ID: u.ID, Username: u.Username})
	if err != nil {
		t.Fatal(err)
	}
	callback, err := s.oauth.Decide(ctx, p.ID, oauth.Actor{ID: u.ID, Username: u.Username}, true)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(callback)
	tokens, err := s.oauth.Exchange(ctx, "authorization_code", parsed.Query().Get("code"), c.ID, c.RedirectURI, verifier, a.Resource, "", func(g oauth.Grant) error { return s.grantAdministrator(ctx, g) })
	if err != nil {
		t.Fatal(err)
	}
	return tokens
}
func mcpCall(t *testing.T, s *Server, token, tool string, args any) map[string]json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "https://wiki.example/mcp", strings.NewReader(string(raw)))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("MCP-Protocol-Version", "2025-11-25")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	var out struct {
		Result map[string]json.RawMessage `json:"result"`
		Error  json.RawMessage            `json:"error"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
		t.Fatalf("MCP %d %s", w.Code, w.Body.String())
	}
	if out.Error != nil {
		return map[string]json.RawMessage{"isError": json.RawMessage(`true`)}
	}
	return out.Result
}

func TestMCPLargeTextAndPagedAdministratorReview(t *testing.T) {
	s, _ := oauthTestServer(t)
	ctx := context.Background()
	db := s.store.(*storage.BadgerDocumentStore)
	configureTestEdition(t, db, "ko", "대문")
	if _, err := db.Save(ctx, document.Document{Locale: "ko", Slug: techmark.Slugify("대문"), Title: "대문", Source: "main"}); err != nil {
		t.Fatal(err)
	}
	before := strings.Repeat("같은 줄 "+strings.Repeat("<>&", 32)+"\n", 6000) + "old end"
	after := strings.TrimSuffix(before, "old end") + "new end"
	doc, err := db.Save(ctx, document.Document{Locale: "ko", Slug: techmark.Slugify("긴 문서"), Title: "긴 문서", Source: before})
	if err != nil {
		t.Fatal(err)
	}
	token := proposalToken(t, s, oauth.ProposalScopes)
	in := changeproposal.Input{Kind: "edit", Locale: "ko", Name: "긴 문서", Source: after, BaseRevision: doc.Revision, Summary: "tail edit"}
	encoded, _ := json.Marshal(in)
	if len(encoded) <= 2<<20 {
		t.Fatal("fixture must exceed the old encoded request limit")
	}
	result := mcpCall(t, s, token.AccessToken, "propose_change", in)
	if string(result["isError"]) == "true" {
		t.Fatal("long UTF-8 request was rejected", result)
	}
	var submitted struct {
		ID string `json:"proposal_id"`
	}
	if err := json.Unmarshal(result["structuredContent"], &submitted); err != nil {
		t.Fatal(err)
	}
	path := "/api/admin/mcp/proposals/" + submitted.ID
	w := oauthRequest(s, "GET", path, "", "admin-session", "")
	var detail struct {
		Before    string             `json:"before"`
		After     string             `json:"after"`
		Truncated bool               `json:"diff_truncated"`
		Lines     []revisionDiffLine `json:"lines"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &detail) != nil || detail.Before != before || detail.After != after || detail.Truncated {
		t.Fatal("incomplete review source")
	}
	found := false
	for _, line := range detail.Lines {
		if line.Kind == "added" && line.NewLine == 6001 {
			found = true
		}
	}
	if !found {
		t.Fatal("tail edit hidden in initial review")
	}
	preview := oauthRequest(s, "GET", path+"?preview=true", "", "admin-session", "")
	var compact struct {
		Digest string             `json:"review_digest"`
		Limit  int                `json:"diff_limit"`
		Lines  []revisionDiffLine `json:"lines"`
	}
	stored, err := db.GetChangeProposal(ctx, submitted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Code != 200 || json.Unmarshal(preview.Body.Bytes(), &compact) != nil || compact.Digest != stored.Digest() || compact.Limit != 50 || len(compact.Lines) > 50 {
		t.Fatal("batch review preview mismatch", preview.Code, compact)
	}
	if strings.Contains(preview.Body.String(), `"before":`) || strings.Contains(preview.Body.String(), `"after":`) || preview.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("batch preview copied complete sources or became cacheable")
	}
	if w := oauthRequest(s, "GET", path+"?preview=true", "", "", ""); w.Code != 401 {
		t.Fatal("preview requires admin session", w.Code)
	}
	if w := oauthRequest(s, "GET", path+"?preview=invalid", "", "admin-session", ""); w.Code != 400 {
		t.Fatal("invalid preview accepted", w.Code)
	}
	if w := oauthRequest(s, "GET", path+"/diff", "", "", ""); w.Code != 401 {
		t.Fatal("diff must require admin session")
	}
	for _, query := range []string{"?limit=501", "?limit=0", "?offset=-1", "?changes_only=unknown"} {
		if w := oauthRequest(s, "GET", path+"/diff"+query, "", "admin-session", ""); w.Code != 400 {
			t.Fatal(query, w.Code)
		}
	}
	w = oauthRequest(s, "GET", path+"/diff?offset=6000&limit=500&changes_only=false", "", "admin-session", "")
	var page proposalDiff
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Lines) != 2 || page.Lines[1].Text != "new end" || page.Added != 1 || page.Removed != 1 {
		t.Fatal("paged tail mismatch", w.Code)
	}
	if strings.Contains(w.Body.String(), `"before"`) || strings.Contains(w.Body.String(), `"after"`) || strings.Contains(w.Body.String(), "review_digest") {
		t.Fatal("diff page should not resend source or approval digest")
	}
	current, err := db.GetLatest(ctx, "ko", techmark.Slugify(in.Name))
	if err != nil || current.Source != before || current.Revision != doc.Revision {
		t.Fatal("review mutated a document", err)
	}
}
func TestMCPProposalRequiresSeparateScopeAndAdministratorDiffApproval(t *testing.T) {
	s, accounts := oauthTestServer(t)
	ctx := context.Background()
	db := s.store.(*storage.BadgerDocumentStore)
	configureTestEdition(t, db, "ko", "대문")
	if _, err := db.Save(ctx, document.Document{Locale: "ko", Slug: techmark.Slugify("대문"), Title: "대문", Source: "main"}); err != nil {
		t.Fatal(err)
	}
	read := proposalToken(t, s, oauth.Scope)
	write := proposalToken(t, s, oauth.ProposalScopes)
	in := changeproposal.Input{Kind: "create", Locale: "ko", Name: "새 문서", Source: "# 새 문서\n<script>alert(1)</script>", Summary: "new"}
	result := mcpCall(t, s, read.AccessToken, "propose_change", in)
	if string(result["isError"]) != "true" || !strings.Contains(string(result["_meta"]), "insufficient_scope") || !strings.Contains(string(result["_meta"]), oauth.ProposalScopes) {
		t.Fatal("missing scope gate/reauthorization challenge", result)
	}
	result = mcpCall(t, s, write.AccessToken, "propose_change", in)
	if string(result["isError"]) == "true" {
		t.Fatal(result)
	}
	var output struct {
		ID     string `json:"proposal_id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(result["structuredContent"], &output); err != nil {
		t.Fatal(err)
	}
	if output.ID == "" || output.Status != "pending" {
		t.Fatal(output)
	}
	if _, err := db.GetLatest(ctx, "ko", techmark.Slugify(in.Name)); err != storage.ErrDocumentNotFound {
		t.Fatal("MCP mutated directly", err)
	}
	path := "/api/admin/mcp/proposals/" + output.ID
	if w := oauthRequest(s, "GET", path, "", "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	accounts.user.Permissions = []auth.Permission{auth.PermissionEdit}
	if w := oauthRequest(s, "GET", path, "", "admin-session", ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	accounts.user.Permissions = []auth.Permission{auth.PermissionAdmin}
	w := oauthRequest(s, "GET", path, "", "admin-session", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"kind":"added"`) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("diff %d %s", w.Code, w.Body.String())
	}
	var detail struct {
		Digest string `json:"review_digest"`
	}
	json.Unmarshal(w.Body.Bytes(), &detail)
	body, _ := json.Marshal(map[string]any{"allow": true, "review_digest": detail.Digest, "confirmation": output.ID})
	if w := oauthRequest(s, "POST", path+"/decide", string(body), "admin-session", "https://evil.example"); w.Code != 403 {
		t.Fatal("CSRF", w.Code)
	}
	if w := oauthRequest(s, "POST", path+"/decide", string(body), "", "https://wiki.example"); w.Code != 401 {
		t.Fatal("cookie gate", w.Code)
	}
	if w := oauthRequest(s, "POST", path+"/decide", string(body), "admin-session", "https://wiki.example"); w.Code != 200 {
		t.Fatalf("approve %d %s", w.Code, w.Body.String())
	}
	doc, err := db.GetLatest(ctx, "ko", techmark.Slugify(in.Name))
	if err != nil || doc.Source != in.Source || strings.Contains(doc.HTML, "<script>") {
		t.Fatal("render safety", doc, err)
	}
	if w := oauthRequest(s, "POST", path+"/decide", string(body), "admin-session", "https://wiki.example"); w.Code != 409 {
		t.Fatal("approval replay", w.Code)
	}
	status := mcpCall(t, s, write.AccessToken, "get_proposal_status", map[string]any{"proposal_id": output.ID})
	if !strings.Contains(string(status["structuredContent"]), `"status":"applied"`) {
		t.Fatal(status)
	}
	for _, forbidden := range []string{"delete_page", "approve_proposal", "commit_change", "unlink_pages"} {
		if r := mcpCall(t, s, write.AccessToken, forbidden, map[string]any{"confirmed": true}); string(r["isError"]) != "true" {
			t.Fatal("forbidden tool", forbidden)
		}
	}
}
func TestMCPHistoryReadsOriginalSourcesWithoutInternalIDs(t *testing.T) {
	s, _ := oauthTestServer(t)
	db := s.store.(*storage.BadgerDocumentStore)
	ctx := context.Background()
	configureTestEdition(t, db, "en", "Main")
	db.Save(ctx, document.Document{Locale: "en", Slug: techmark.Slugify("Main"), Title: "Main", Source: "main"})
	doc, err := db.Save(ctx, document.Document{Locale: "en", Slug: techmark.Slugify("Article"), Title: "Article", Source: "old TechMark 🙂", Author: "private-author"})
	if err != nil {
		t.Fatal(err)
	}
	doc.Source = "latest"
	db.Save(ctx, doc)
	token := proposalToken(t, s, oauth.Scope)
	for _, tool := range []string{"list_revisions", "read_revision", "get_language_links"} {
		args := map[string]any{"locale": "en", "name": "Article"}
		if tool == "read_revision" {
			args["revision"] = "r1"
		}
		r := mcpCall(t, s, token.AccessToken, tool, args)
		raw := string(r["structuredContent"])
		if string(r["isError"]) == "true" || strings.Contains(raw, doc.ID.String()) || strings.Contains(raw, "private-author") {
			t.Fatal(tool, r)
		}
		if tool == "read_revision" && !strings.Contains(raw, "old TechMark") {
			t.Fatal(raw)
		}
	}
	doc.EditPolicy = document.EditPolicyAdmins
	db.Save(ctx, doc)
	if r := mcpCall(t, s, token.AccessToken, "read_revision", map[string]any{"locale": "en", "name": "Article", "revision": "r1"}); string(r["isError"]) != "true" {
		t.Fatal("protected source leaked")
	}
}
func TestDocumentDeletionIsOwnerOnlyEvenWithAdminOrMCPTokens(t *testing.T) {
	s, a := oauthTestServer(t)
	ctx := context.Background()
	db := s.store.(*storage.BadgerDocumentStore)
	configureTestEdition(t, db, "en", "Main")
	db.Save(ctx, document.Document{Locale: "en", Slug: techmark.Slugify("Main"), Title: "Main", Source: "main"})
	db.Save(ctx, document.Document{Locale: "en", Slug: techmark.Slugify("Delete me"), Title: "Delete me", Source: "source"})
	path := "/api/admin/pages/en/Delete_me"
	if w := oauthRequest(s, "DELETE", path, `{"reason":"test"}`, "admin-session", ""); w.Code != 403 {
		t.Fatal("admin deleted", w.Code)
	}
	a.user.Permissions = []auth.Permission{auth.PermissionOwner}
	if w := oauthRequest(s, "DELETE", path, `{"reason":"test"}`, "admin-session", ""); w.Code != 204 {
		t.Fatalf("owner %d %s", w.Code, w.Body.String())
	}
}

func TestProposalApprovalRechecksIdentityGrantAndDisableWhileRejectionRemainsAvailable(t *testing.T) {
	for _, mode := range []string{"revoked", "client-disabled", "mcp-disabled", "identity-replaced", "not-admin", "resource-changed"} {
		t.Run(mode, func(t *testing.T) {
			s, a := oauthTestServer(t)
			ctx := context.Background()
			db := s.store.(*storage.BadgerDocumentStore)
			configureTestEdition(t, db, "en", "Main")
			if _, err := db.Save(ctx, document.Document{Locale: "en", Slug: techmark.Slugify("Main"), Title: "Main", Source: "main"}); err != nil {
				t.Fatal(err)
			}
			tokens := proposalToken(t, s, oauth.ProposalScopes)
			g, err := s.oauth.Access(ctx, tokens.AccessToken)
			if err != nil {
				t.Fatal(err)
			}
			p, err := db.SubmitChangeProposal(ctx, changeproposal.Input{Kind: "create", Locale: "en", Name: "Proposal", Source: "source", Summary: "create"}, g.ID, g.UserID, g.Username)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "revoked":
				if err := s.oauth.Revoke(ctx, g.ID, oauth.Actor{ID: g.UserID, Username: g.Username}); err != nil {
					t.Fatal(err)
				}
			case "client-disabled":
				if err := s.oauth.Disable(ctx, g.ClientID, oauth.Actor{ID: g.UserID, Username: g.Username}); err != nil {
					t.Fatal(err)
				}
			case "mcp-disabled":
				s.oauth = nil
			case "resource-changed":
				s.oauth = oauth.New(db, "https://new.example")
			case "identity-replaced":
				a.user.ID = "replacement-id"
			case "not-admin":
				a.user.Permissions = []auth.Permission{auth.PermissionEdit}
			}
			body, _ := json.Marshal(map[string]any{"allow": true, "confirmation": p.ID, "review_digest": p.Digest()})
			w := oauthRequest(s, "POST", "/api/admin/mcp/proposals/"+p.ID+"/decide", string(body), "admin-session", "https://wiki.example")
			if w.Code != 409 && w.Code != 403 {
				t.Fatalf("%s accepted %d %s", mode, w.Code, w.Body.String())
			}
			if _, err := db.GetLatest(ctx, "en", techmark.Slugify("Proposal")); err != storage.ErrDocumentNotFound {
				t.Fatal("unauthorized mutation", err)
			}
			if mode != "not-admin" {
				body, _ = json.Marshal(map[string]any{"allow": false, "confirmation": p.ID, "review_digest": p.Digest()})
				w := oauthRequest(s, "POST", "/api/admin/mcp/proposals/"+p.ID+"/decide", string(body), "admin-session", "https://wiki.example")
				if w.Code != 200 {
					t.Fatalf("reject unavailable %d %s", w.Code, w.Body.String())
				}
			}
		})
	}
}
