package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"techpedia/internal/auth"
	"techpedia/internal/changeproposal"
	"techpedia/internal/document"
	"techpedia/internal/oauth"
	"techpedia/internal/storage"
	"techpedia/internal/techmark"
)

func TestMCPPartialEditsAndAmendmentsRequireFreshAdministratorApproval(t *testing.T) {
	s, _ := oauthTestServer(t)
	ctx := context.Background()
	db := s.store.(*storage.BadgerDocumentStore)
	configureTestEdition(t, db, "en", "Main")
	db.Save(ctx, document.Document{Locale: "en", Slug: techmark.Slugify("Main"), Title: "Main", Source: "main"})
	doc, err := db.Save(ctx, document.Document{Locale: "en", Slug: techmark.Slugify("Article"), Title: "Article", Source: "original 🙂\nuntouched"})
	if err != nil {
		t.Fatal(err)
	}
	write := proposalToken(t, s, oauth.ProposalScopes)
	read := proposalToken(t, s, oauth.Scope)
	other := proposalToken(t, s, oauth.ProposalScopes)
	in := changeproposal.Input{Kind: "edit", Locale: "en", Name: "Article", BaseRevision: doc.Revision, Summary: "partial", Edits: []changeproposal.TextEdit{{Before: "original 🙂", After: "proposed 🙂"}}}
	result := mcpCall(t, s, write.AccessToken, "propose_change", in)
	if string(result["isError"]) == "true" {
		t.Fatal(result)
	}
	var first struct {
		ID string `json:"proposal_id"`
	}
	if err := json.Unmarshal(result["structuredContent"], &first); err != nil || first.ID == "" {
		t.Fatal(result, err)
	}
	p, err := db.GetChangeProposal(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"read_proposal", "amend_proposal"} {
		args := map[string]any{"proposal_id": p.ID, "source": "corrected", "summary": "amend"}
		if tool == "read_proposal" {
			args = map[string]any{"proposal_id": p.ID}
		}
		if denied := mcpCall(t, s, read.AccessToken, tool, args); string(denied["isError"]) != "true" || !strings.Contains(string(denied["_meta"]), "insufficient_scope") {
			t.Fatal("read grant accepted", tool, denied)
		}
		if denied := mcpCall(t, s, other.AccessToken, tool, args); string(denied["isError"]) != "true" {
			t.Fatal("foreign connection accepted", tool, denied)
		}
	}
	readResult := mcpCall(t, s, write.AccessToken, "read_proposal", map[string]any{"proposal_id": p.ID})
	if !strings.Contains(string(readResult["structuredContent"]), "proposed 🙂") || strings.Contains(string(readResult["structuredContent"]), doc.ID.String()) || strings.Contains(string(readResult["structuredContent"]), "review_digest") {
		t.Fatal(readResult)
	}
	corrected := mcpCall(t, s, write.AccessToken, "amend_proposal", changeproposal.Amendment{ProposalID: p.ID, Summary: "correct typo", Edits: []changeproposal.TextEdit{{Before: "proposed 🙂", After: "corrected 🙂"}}})
	var next struct {
		ID         string `json:"proposal_id"`
		Supersedes string `json:"supersedes"`
	}
	if err := json.Unmarshal(corrected["structuredContent"], &next); err != nil || next.ID == "" || next.Supersedes != p.ID {
		t.Fatal(corrected, err)
	}
	current, _ := db.GetLatest(ctx, "en", techmark.Slugify("Article"))
	if current.Source != doc.Source {
		t.Fatal("MCP directly changed wiki")
	}
	oldDecision, _ := json.Marshal(map[string]any{"allow": true, "confirmation": p.ID, "review_digest": p.Digest()})
	if w := oauthRequest(s, "POST", "/api/admin/mcp/proposals/"+p.ID+"/decide", string(oldDecision), "admin-session", "https://wiki.example"); w.Code != 409 {
		t.Fatal("old review applied", w.Code, w.Body.String())
	}
	batch, _ := json.Marshal(map[string]any{"allow": true, "confirmation": "approve:1", "proposals": []map[string]string{{"id": p.ID, "review_digest": p.Digest()}}})
	if w := oauthRequest(s, "POST", "/api/admin/mcp/proposals/decide-batch", string(batch), "admin-session", "https://wiki.example"); w.Code != 200 || !strings.Contains(w.Body.String(), `"error"`) {
		t.Fatal("old batch applied", w.Code, w.Body.String())
	}
	for _, id := range []string{p.ID, next.ID} {
		w := oauthRequest(s, "GET", "/api/admin/mcp/proposals/"+id, "", "admin-session", "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), next.ID) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	nextStored, _ := db.GetChangeProposal(ctx, next.ID)
	decision, _ := json.Marshal(map[string]any{"allow": true, "confirmation": next.ID, "review_digest": nextStored.Digest()})
	if w := oauthRequest(s, "POST", "/api/admin/mcp/proposals/"+next.ID+"/decide", string(decision), "admin-session", "https://wiki.example"); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	current, _ = db.GetLatest(ctx, "en", techmark.Slugify("Article"))
	if current.Source != "corrected 🙂\nuntouched" || current.ID != doc.ID || current.Revision == doc.Revision {
		t.Fatal(current)
	}
	status := mcpCall(t, s, write.AccessToken, "get_proposal_status", map[string]any{"proposal_id": p.ID})
	if !strings.Contains(string(status["structuredContent"]), `"superseded_by":"`+next.ID+`"`) {
		t.Fatal(status)
	}
}

func TestMCPProposalSourceChunksAreUTF8SafeAndCurrentPermissionsStillRequired(t *testing.T) {
	s, accounts := oauthTestServer(t)
	ctx := context.Background()
	db := s.store.(*storage.BadgerDocumentStore)
	configureTestEdition(t, db, "en", "Main")
	db.Save(ctx, document.Document{Locale: "en", Slug: techmark.Slugify("Main"), Title: "Main", Source: "main"})
	tokens := proposalToken(t, s, oauth.ProposalScopes)
	g, _ := s.oauth.Access(ctx, tokens.AccessToken)
	source := strings.Repeat("🙂한글", 10000)
	p, err := db.SubmitChangeProposal(ctx, changeproposal.Input{Kind: "create", Locale: "en", Name: "New", Source: source, Summary: "create"}, g.ID, g.UserID, g.Username)
	if err != nil {
		t.Fatal(err)
	}
	offset := 0
	var rebuilt strings.Builder
	for {
		result := mcpCall(t, s, tokens.AccessToken, "read_proposal", map[string]any{"proposal_id": p.ID, "offset": offset})
		var chunk struct {
			Source string `json:"source"`
			Next   int    `json:"next_offset"`
			EOF    bool   `json:"eof"`
		}
		if err := json.Unmarshal(result["structuredContent"], &chunk); err != nil || len(chunk.Source) > 32<<10 {
			t.Fatal(result, err)
		}
		rebuilt.WriteString(chunk.Source)
		if chunk.EOF {
			break
		}
		if chunk.Next <= offset {
			t.Fatal("read did not advance")
		}
		offset = chunk.Next
	}
	if rebuilt.String() != source {
		t.Fatal("proposal chunks changed source")
	}
	if bad := mcpCall(t, s, tokens.AccessToken, "read_proposal", map[string]any{"proposal_id": p.ID, "offset": 1}); string(bad["isError"]) != "true" {
		t.Fatal("invalid UTF-8 offset accepted")
	}
	accounts.user.Permissions = []auth.Permission{auth.PermissionEdit}
	raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "amend_proposal", "arguments": map[string]any{"proposal_id": p.ID, "source": "changed", "summary": "changed"}}})
	// The normal OAuth gate rejects the downgraded account before tool execution.
	r := httptest.NewRequest("POST", "https://wiki.example/mcp", strings.NewReader(string(raw)))
	r.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("MCP-Protocol-Version", "2025-11-25")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 && w.Code != 401 {
		t.Fatal("downgraded account amended", w.Code, w.Body.String())
	}
}
