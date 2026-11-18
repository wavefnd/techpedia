package wiki

import (
	"context"

	"techpedia/internal/changeproposal"
	"techpedia/internal/oauth"
)

type connectionKey struct{}

// WithConnection is only called by the HTTP OAuth gate after verifying the bearer
// and the current immutable administrator identity. No tool argument can set it.
func WithConnection(ctx context.Context, grant oauth.Grant) context.Context {
	return context.WithValue(ctx, connectionKey{}, grant)
}
func Connection(ctx context.Context) (oauth.Grant, bool) {
	g, ok := ctx.Value(connectionKey{}).(oauth.Grant)
	return g, ok
}

type Proposer struct{ store changeproposal.Store }

func NewProposer(store changeproposal.Store) *Proposer { return &Proposer{store: store} }
func (p *Proposer) Submit(ctx context.Context, input changeproposal.Input) (changeproposal.Proposal, error) {
	g, ok := Connection(ctx)
	if !ok || !oauth.CanPropose(g.Scope) {
		return changeproposal.Proposal{}, changeproposal.ErrUnavailable
	}
	return p.store.SubmitChangeProposal(ctx, input, g.ID, g.UserID, g.Username)
}
func (p *Proposer) Amend(ctx context.Context, input changeproposal.Amendment) (changeproposal.Proposal, error) {
	g, ok := Connection(ctx)
	if !ok || !oauth.CanPropose(g.Scope) {
		return changeproposal.Proposal{}, changeproposal.ErrUnavailable
	}
	return p.store.AmendChangeProposal(ctx, input, g.ID, g.UserID, g.Username)
}
func (p *Proposer) Status(ctx context.Context, id string) (changeproposal.Proposal, error) {
	g, ok := Connection(ctx)
	if !ok || !oauth.CanPropose(g.Scope) {
		return changeproposal.Proposal{}, changeproposal.ErrUnavailable
	}
	v, err := p.store.GetChangeProposal(ctx, id)
	if err != nil {
		return v, err
	}
	if v.GrantID != g.ID || v.UserID != g.UserID || v.Username != g.Username {
		return changeproposal.Proposal{}, changeproposal.ErrNotFound
	}
	return v, nil
}
