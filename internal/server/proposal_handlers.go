package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"techpedia/internal/auth"
	"techpedia/internal/changeproposal"
	"techpedia/internal/oauth"
	"techpedia/internal/storage"
)

func (s *Server) proposalRoutes() {
	s.mux.HandleFunc("GET /api/admin/mcp/proposals", s.handleProposalList)
	s.mux.HandleFunc("GET /api/admin/mcp/proposal-policy", s.handleProposalPolicy)
	s.mux.HandleFunc("PUT /api/admin/mcp/proposal-policy", s.handleProposalPolicy)
	s.mux.HandleFunc("POST /api/admin/mcp/proposals/decide-batch", s.handleProposalBatchDecision)
	s.mux.HandleFunc("GET /api/admin/mcp/proposals/{id}", s.handleProposalDetail)
	s.mux.HandleFunc("GET /api/admin/mcp/proposals/{id}/diff", s.handleProposalDiff)
	s.mux.HandleFunc("POST /api/admin/mcp/proposals/{id}/decide", s.handleProposalDecision)
}

func (s *Server) handleProposalPolicy(w http.ResponseWriter, r *http.Request) {
	u, ok := s.proposalAdministrator(w, r)
	if !ok {
		return
	}
	if r.Method == "PUT" {
		var policy changeproposal.Policy
		if !oauthDecode(w, r, &policy) {
			return
		}
		if err := s.proposals.SetProposalPolicy(r.Context(), policy, u.Username); err != nil {
			s.recordAudit(r.Context(), u.Username, "mcp.proposal_policy", "mcp_proposal", "", "", "failed")
			proposalFailure(w, err)
			return
		}
	}
	policy, err := s.proposals.GetProposalPolicy(r.Context())
	if err != nil {
		proposalFailure(w, err)
		return
	}
	writeJSON(w, 200, policy)
}

// Each entry is a separate transaction. A stale entry cannot authorize another
// entry, and a partial failure never rolls back successfully reviewed changes.
func (s *Server) handleProposalBatchDecision(w http.ResponseWriter, r *http.Request) {
	u, ok := s.proposalAdministrator(w, r)
	if !ok {
		return
	}
	var req struct {
		Allow        *bool  `json:"allow"`
		Confirmation string `json:"confirmation"`
		Proposals    []struct {
			ID           string `json:"id"`
			ReviewDigest string `json:"review_digest"`
		} `json:"proposals"`
	}
	if !oauthDecode(w, r, &req) {
		return
	}
	action := "reject:"
	if req.Allow != nil && *req.Allow {
		action = "approve:"
	}
	if req.Allow == nil || len(req.Proposals) == 0 || len(req.Proposals) > 100 || req.Confirmation != action+strconv.Itoa(len(req.Proposals)) {
		proposalFailure(w, changeproposal.ErrInvalid)
		return
	}
	seen := map[string]bool{}
	for _, entry := range req.Proposals {
		if entry.ID == "" || len(entry.ID) > 80 || len(entry.ReviewDigest) != 64 || seen[entry.ID] {
			proposalFailure(w, changeproposal.ErrInvalid)
			return
		}
		seen[entry.ID] = true
	}
	type result struct {
		ID       string           `json:"id"`
		Proposal *proposalSummary `json:"proposal,omitempty"`
		Error    string           `json:"error,omitempty"`
	}
	results := make([]result, 0, len(req.Proposals))
	for _, entry := range req.Proposals {
		p, err := s.decideReviewedProposal(r.Context(), u, entry.ID, entry.ReviewDigest, *req.Allow)
		if err != nil {
			s.recordAudit(r.Context(), u.Username, "mcp.proposal_decide", "mcp_proposal", entry.ID, "", "failed")
			_, message := proposalError(err)
			results = append(results, result{ID: entry.ID, Error: message})
		} else {
			summary := summarizeProposal(p)
			results = append(results, result{ID: entry.ID, Proposal: &summary})
		}
	}
	writeJSON(w, 200, map[string]any{"results": results})
}

type proposalSummary struct {
	ID             string               `json:"id"`
	Kind           string               `json:"kind"`
	Page           changeproposal.Page  `json:"page"`
	Target         *changeproposal.Page `json:"target,omitempty"`
	Summary        string               `json:"summary"`
	Status         string               `json:"status"`
	SubmittedBy    string               `json:"submitted_by"`
	CreatedAt      time.Time            `json:"created_at"`
	ExpiresAt      time.Time            `json:"expires_at"`
	DecidedBy      string               `json:"decided_by,omitempty"`
	ResultRevision string               `json:"result_revision,omitempty"`
	Supersedes     string               `json:"supersedes,omitempty"`
	SupersededBy   string               `json:"superseded_by,omitempty"`
	AmendedAt      time.Time            `json:"amended_at,omitempty"`
}

func summarizeProposal(p changeproposal.Proposal) proposalSummary {
	return proposalSummary{ID: p.ID, Kind: p.Input.Kind, Page: changeproposal.Page{Locale: p.Input.Locale, Name: p.Input.Name, Revision: p.Input.BaseRevision}, Target: p.Input.Target, Summary: p.Input.Summary, Status: p.Status, SubmittedBy: p.Username, CreatedAt: p.CreatedAt, ExpiresAt: p.ExpiresAt, DecidedBy: p.DecidedBy, ResultRevision: p.ResultRevision, Supersedes: p.Supersedes, SupersededBy: p.SupersededBy, AmendedAt: p.AmendedAt}
}
func proposalError(err error) (int, string) {
	code, message := 500, "제안을 처리하지 못했습니다. 다시 시도하세요."
	switch {
	case errors.Is(err, changeproposal.ErrNotFound):
		code, message = 404, "제안이 없거나 보관 기간이 만료되었습니다."
	case errors.Is(err, changeproposal.ErrConflict), errors.Is(err, storage.ErrDocumentExists):
		code, message = 409, "제안·문서 또는 언어 연결이 변경되었습니다. 최신 제안을 다시 검토하세요."
	case errors.Is(err, changeproposal.ErrUnavailable):
		code, message = 409, "수정본으로 대체되었거나 이미 처리·만료·연결 해제된 제안입니다."
	case errors.Is(err, changeproposal.ErrInvalid):
		code, message = 400, "잘못된 제안 또는 승인 요청입니다."
	case errors.Is(err, changeproposal.ErrSourceTooLarge):
		code, message = 413, "TechMark 원문이 서버 보호용 2 MiB 용량을 초과했습니다."
	}
	return code, message
}
func proposalFailure(w http.ResponseWriter, err error) {
	code, message := proposalError(err)
	writeJSON(w, code, map[string]string{"error": message})
}
func (s *Server) proposalAdministrator(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	setRevisionPrivacyHeaders(w)
	u, ok := s.requirePermission(w, r, auth.PermissionAdmin)
	if !ok {
		return u, false
	}
	if s.proposals == nil {
		http.NotFound(w, r)
		return u, false
	}
	if r.Method != "GET" && (r.Header.Get("Origin") != s.publicURL || r.Header.Get("Content-Type") != "application/json") {
		writeJSON(w, 403, map[string]string{"error": "관리자 화면에서만 제안을 승인하거나 거절할 수 있습니다."})
		return u, false
	}
	return u, true
}
func (s *Server) handleProposalList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.proposalAdministrator(w, r); !ok {
		return
	}
	values, err := s.proposals.ListChangeProposals(r.Context())
	if err != nil {
		proposalFailure(w, err)
		return
	}
	out := []proposalSummary{}
	for i := len(values) - 1; i >= 0; i-- {
		out = append(out, summarizeProposal(values[i]))
	}
	writeJSON(w, 200, out)
}
func (s *Server) handleProposalDetail(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.proposalAdministrator(w, r); !ok {
		return
	}
	preview := false
	if r.URL.Query().Has("preview") {
		var err error
		preview, err = strconv.ParseBool(r.URL.Query().Get("preview"))
		if err != nil {
			proposalFailure(w, changeproposal.ErrInvalid)
			return
		}
	}
	p, err := s.proposals.GetChangeProposal(r.Context(), r.PathValue("id"))
	if err != nil {
		proposalFailure(w, err)
		return
	}
	limit := 500
	if preview {
		limit = 50
	}
	diff := proposalDiffPage(p.Before, p.Input.Source, 0, limit, true)
	out := map[string]any{"proposal": summarizeProposal(p), "lines": diff.Lines, "diff_truncated": false, "review_digest": p.Digest(), "diff_offset": diff.Offset, "diff_limit": diff.Limit, "diff_total": diff.Total, "changes_only": diff.ChangesOnly, "added_lines": diff.Added, "removed_lines": diff.Removed}
	if !preview {
		out["before"], out["after"] = p.Before, p.Input.Source
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleProposalDiff(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.proposalAdministrator(w, r); !ok {
		return
	}
	offset, limit, changesOnly := 0, 500, true
	query := r.URL.Query()
	var err error
	if query.Has("offset") {
		offset, err = strconv.Atoi(query.Get("offset"))
	}
	if err != nil || offset < 0 || offset > 4*changeproposal.MaxSourceBytes {
		proposalFailure(w, changeproposal.ErrInvalid)
		return
	}
	if query.Has("limit") {
		limit, err = strconv.Atoi(query.Get("limit"))
	}
	if err != nil || limit < 1 || limit > 500 {
		proposalFailure(w, changeproposal.ErrInvalid)
		return
	}
	if query.Has("changes_only") {
		changesOnly, err = strconv.ParseBool(query.Get("changes_only"))
	}
	if err != nil {
		proposalFailure(w, changeproposal.ErrInvalid)
		return
	}
	p, err := s.proposals.GetChangeProposal(r.Context(), r.PathValue("id"))
	if err != nil {
		proposalFailure(w, err)
		return
	}
	writeJSON(w, 200, proposalDiffPage(p.Before, p.Input.Source, offset, limit, changesOnly))
}
func (s *Server) handleProposalDecision(w http.ResponseWriter, r *http.Request) {
	u, ok := s.proposalAdministrator(w, r)
	if !ok {
		return
	}
	var req struct {
		Allow        *bool  `json:"allow"`
		ReviewDigest string `json:"review_digest"`
		Confirmation string `json:"confirmation"`
	}
	if !oauthDecode(w, r, &req) {
		return
	}
	id := r.PathValue("id")
	if req.Allow == nil || req.Confirmation != id || len(req.ReviewDigest) != 64 {
		proposalFailure(w, changeproposal.ErrInvalid)
		return
	}
	result, err := s.decideReviewedProposal(r.Context(), u, id, req.ReviewDigest, *req.Allow)
	if err != nil {
		s.recordAudit(r.Context(), u.Username, "mcp.proposal_decide", "mcp_proposal", id, "", "failed")
		proposalFailure(w, err)
		return
	}
	writeJSON(w, 200, summarizeProposal(result))
}

func (s *Server) decideReviewedProposal(ctx context.Context, u auth.User, id, digest string, allow bool) (changeproposal.Proposal, error) {
	p, err := s.proposals.GetChangeProposal(ctx, id)
	if err != nil {
		return p, err
	}
	if digest != p.Digest() {
		return p, changeproposal.ErrConflict
	}
	if allow {
		// Recheck the proposer as well as the reviewer. A revoked/disabled grant
		// is checked again inside the same transaction as the actual mutation.
		if s.oauth == nil || s.grantAdministrator(ctx, oauth.Grant{UserID: p.UserID, Username: p.Username}) != nil {
			return p, changeproposal.ErrUnavailable
		}
		g, err := s.oauth.ProposalGrant(ctx, p.GrantID)
		if err != nil || g.UserID != p.UserID || g.Username != p.Username {
			return p, changeproposal.ErrUnavailable
		}
	}
	html := ""
	if allow && p.Input.Kind != "link" {
		html = s.parser.RenderWithLocale(p.Input.Source, p.Input.Locale)
	}
	if s.grantAdministrator(ctx, oauth.Grant{UserID: u.ID, Username: u.Username}) != nil {
		return p, changeproposal.ErrUnavailable
	}
	if allow && s.grantAdministrator(ctx, oauth.Grant{UserID: p.UserID, Username: p.Username}) != nil {
		return p, changeproposal.ErrUnavailable
	}
	result, err := s.proposals.DecideChangeProposal(ctx, id, digest, allow, u.Username, html)
	if err == nil && allow {
		s.invalidateGraphCache()
	}
	return result, err
}
