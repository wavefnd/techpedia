package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryStore struct {
	mu   sync.Mutex
	data map[string]json.RawMessage
}
type memoryTx map[string]json.RawMessage

func (tx memoryTx) Audit(event Event) error { return tx.Put("audit:"+random(), event, time.Time{}) }

func (tx memoryTx) Get(key string, v any) error {
	data, ok := tx[key]
	if !ok {
		return ErrMissing
	}
	return json.Unmarshal(data, v)
}
func (tx memoryTx) Put(key string, v any, _ time.Time) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tx[key] = data
	return nil
}
func (tx memoryTx) List(prefix string) ([]json.RawMessage, error) {
	out := []json.RawMessage{}
	for k, v := range tx {
		if strings.HasPrefix(k, prefix) {
			out = append(out, v)
		}
	}
	return out, nil
}
func (m *memoryStore) OAuthView(ctx context.Context, fn func(Tx) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(memoryTx(m.data))
}
func (m *memoryStore) OAuthUpdate(ctx context.Context, fn func(Tx) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	next := memoryTx{}
	for k, v := range m.data {
		next[k] = v
	}
	if err := fn(next); err != nil {
		return err
	}
	m.data = next
	return nil
}

type fixture struct {
	s        *Service
	client   Client
	a        Authorization
	actor    Actor
	verifier string
	clock    time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{s: New(&memoryStore{data: map[string]json.RawMessage{}}, "https://wiki.example"), actor: Actor{ID: "actor", Username: "admin"}, verifier: strings.Repeat("v", 43), clock: time.Now().UTC()}
	f.s.now = func() time.Time { return f.clock }
	var err error
	f.client, err = f.s.Register(context.Background(), "ChatGPT", "https://chatgpt.example/callback", f.actor)
	if err != nil {
		t.Fatal(err)
	}
	f.a = Authorization{ClientID: f.client.ID, RedirectURI: f.client.RedirectURI, ResponseType: "code", Challenge: digest(f.verifier), ChallengeMethod: "S256", Scope: Scope, Resource: "https://wiki.example/mcp", State: "state"}
	return f
}
func (f *fixture) code(t *testing.T) string {
	t.Helper()
	p, _, err := f.s.Prepare(context.Background(), f.a, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	redirect, err := f.s.Decide(context.Background(), p.ID, f.actor, true)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(redirect)
	return u.Query().Get("code")
}
func (f *fixture) exchange(code string) (Tokens, error) {
	return f.s.Exchange(context.Background(), "authorization_code", code, f.client.ID, f.client.RedirectURI, f.verifier, f.a.Resource, "", allowGrant)
}
func allowGrant(Grant) error { return nil }
func TestSeparateProposalConsentAndRefreshCannotUpgradeReadGrant(t *testing.T) {
	for _, scope := range []string{Scope, ProposalScopes, ProposeScope + " " + Scope} {
		t.Run(scope, func(t *testing.T) {
			f := newFixture(t)
			f.a.Scope = scope
			tokens, err := f.exchange(f.code(t))
			if err != nil || tokens.Scope != scope {
				t.Fatal(tokens, err)
			}
			if scope == Scope {
				if _, err := f.s.Exchange(context.Background(), "refresh_token", tokens.RefreshToken, f.client.ID, "", "", f.a.Resource, ProposalScopes, allowGrant); err == nil {
					t.Fatal("read token upgraded")
				}
			}
			next, err := f.s.Exchange(context.Background(), "refresh_token", tokens.RefreshToken, f.client.ID, "", "", f.a.Resource, "", allowGrant)
			if err != nil || next.Scope != scope {
				t.Fatal(next, err)
			}
		})
	}
	for _, scope := range []string{ProposeScope, "wiki:write", Scope + " " + Scope, ProposalScopes + " wiki:delete"} {
		f := newFixture(t)
		f.a.Scope = scope
		if _, err := f.s.Validate(context.Background(), f.a); err == nil {
			t.Fatal("invalid scope", scope)
		}
	}
}
func TestExactClientRedirectResourceScopeAndS256(t *testing.T) {
	f := newFixture(t)
	mutations := []func(*Authorization){
		func(a *Authorization) { a.RedirectURI += "?extra=1" }, func(a *Authorization) { a.Resource = "https://attacker.example/mcp" },
		func(a *Authorization) { a.Scope = "wiki:read wiki:write" }, func(a *Authorization) { a.ChallengeMethod = "plain" }, func(a *Authorization) { a.Challenge = "invalid" }, func(a *Authorization) { a.ResponseType = "token" }, func(a *Authorization) { a.ClientID = random() },
	}
	for i, change := range mutations {
		a := f.a
		change(&a)
		if _, err := f.s.Validate(context.Background(), a); err == nil {
			t.Fatalf("invalid authorization %d accepted", i)
		}
	}
	for _, redirect := range []string{"http://chatgpt.example/callback", "https://user:pass@chatgpt.example/callback", "https://chatgpt.example/callback#fragment", "javascript:alert(1)"} {
		if _, err := f.s.Register(context.Background(), "client", redirect, f.actor); err == nil {
			t.Fatalf("invalid redirect accepted: %q", redirect)
		}
	}
	code := f.code(t)
	for _, change := range []string{"verifier", "redirect", "resource", "client", "scope"} {
		verifier, redirect, resource, id, scope := f.verifier, f.client.RedirectURI, f.a.Resource, f.client.ID, ""
		switch change {
		case "verifier":
			verifier = strings.Repeat("w", 43)
		case "redirect":
			redirect += "?x=1"
		case "resource":
			resource += "/other"
		case "client":
			id = random()
		case "scope":
			scope = "wiki:write"
		}
		if _, err := f.s.Exchange(context.Background(), "authorization_code", code, id, redirect, verifier, resource, scope, allowGrant); err == nil {
			t.Fatalf("invalid exchange %s accepted", change)
		}
	}
	if _, err := f.exchange(code); err != nil {
		t.Fatal("failed exchange consumed code", err)
	}
}
func TestApprovalBoundToActorExpiresAndIsSingleUse(t *testing.T) {
	f := newFixture(t)
	p, _, err := f.s.Prepare(context.Background(), f.a, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Decide(context.Background(), p.ID, Actor{ID: "another-admin"}, true); err == nil {
		t.Fatal("another actor approved request")
	}
	if _, err := f.s.Decide(context.Background(), p.ID, f.actor, true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Decide(context.Background(), p.ID, f.actor, true); err == nil {
		t.Fatal("approval reused")
	}
	p, _, _ = f.s.Prepare(context.Background(), f.a, f.actor)
	f.clock = f.clock.Add(6 * time.Minute)
	if _, err := f.s.Decide(context.Background(), p.ID, f.actor, true); err == nil {
		t.Fatal("expired approval accepted")
	}
}
func TestDenialPreservesStateAndIssuerWithoutGrant(t *testing.T) {
	f := newFixture(t)
	p, _, _ := f.s.Prepare(context.Background(), f.a, f.actor)
	redirect, err := f.s.Decide(context.Background(), p.ID, f.actor, false)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(redirect)
	if u.Query().Get("error") != "access_denied" || u.Query().Get("state") != f.a.State || u.Query().Get("iss") != "https://wiki.example" || u.Query().Get("code") != "" {
		t.Fatal("denial callback invalid")
	}
	grants, _ := f.s.Grants(context.Background(), f.actor.ID)
	if len(grants) != 0 {
		t.Fatal("denial issued grant")
	}
}
func TestRotationReplayRevokesAllAccessAndRefreshTokens(t *testing.T) {
	f := newFixture(t)
	initial, err := f.exchange(f.code(t))
	if err != nil {
		t.Fatal(err)
	}
	next, err := f.s.Exchange(context.Background(), "refresh_token", initial.RefreshToken, f.client.ID, "", "", f.a.Resource, "", allowGrant)
	if err != nil {
		t.Fatal(err)
	}
	if initial.AccessToken == next.AccessToken || initial.RefreshToken == next.RefreshToken {
		t.Fatal("tokens not rotated")
	}
	if _, err := f.s.Access(context.Background(), next.AccessToken); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Exchange(context.Background(), "refresh_token", initial.RefreshToken, f.client.ID, "", "", f.a.Resource, "", allowGrant); err == nil {
		t.Fatal("refresh reused")
	}
	for _, token := range []string{initial.AccessToken, next.AccessToken} {
		if _, err := f.s.Access(context.Background(), token); err == nil {
			t.Fatal("replay did not revoke access")
		}
	}
	if _, err := f.s.Exchange(context.Background(), "refresh_token", next.RefreshToken, f.client.ID, "", "", f.a.Resource, "", allowGrant); err == nil {
		t.Fatal("replay did not revoke refresh")
	}
}
func TestExpiryRevocationDisabledClientAndLiveAccountCheck(t *testing.T) {
	t.Run("access expiry", func(t *testing.T) {
		f := newFixture(t)
		tokens, _ := f.exchange(f.code(t))
		f.clock = f.clock.Add(16 * time.Minute)
		if _, err := f.s.Access(context.Background(), tokens.AccessToken); err == nil {
			t.Fatal("expired access")
		}
		next, err := f.s.Exchange(context.Background(), "refresh_token", tokens.RefreshToken, f.client.ID, "", "", f.a.Resource, "", allowGrant)
		if err != nil || next.AccessToken == "" {
			t.Fatal(err)
		}
	})
	t.Run("grant expiry", func(t *testing.T) {
		f := newFixture(t)
		tokens, _ := f.exchange(f.code(t))
		f.clock = f.clock.Add(31 * 24 * time.Hour)
		if _, err := f.s.Exchange(context.Background(), "refresh_token", tokens.RefreshToken, f.client.ID, "", "", f.a.Resource, "", allowGrant); err == nil {
			t.Fatal("expired refresh")
		}
	})
	t.Run("code expiry", func(t *testing.T) {
		f := newFixture(t)
		code := f.code(t)
		f.clock = f.clock.Add(6 * time.Minute)
		if _, err := f.exchange(code); err == nil {
			t.Fatal("expired code")
		}
	})
	t.Run("revoke ownership", func(t *testing.T) {
		f := newFixture(t)
		tokens, _ := f.exchange(f.code(t))
		grant, _ := f.s.Access(context.Background(), tokens.AccessToken)
		if err := f.s.Revoke(context.Background(), grant.ID, Actor{ID: "another-admin"}); err == nil {
			t.Fatal("another admin revoked")
		}
		if err := f.s.Revoke(context.Background(), grant.ID, f.actor); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.Access(context.Background(), tokens.AccessToken); err == nil {
			t.Fatal("revoked access")
		}
	})
	t.Run("disabled client", func(t *testing.T) {
		f := newFixture(t)
		tokens, _ := f.exchange(f.code(t))
		if err := f.s.Disable(context.Background(), f.client.ID, f.actor); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.Access(context.Background(), tokens.AccessToken); err == nil {
			t.Fatal("disabled client access")
		}
	})
	t.Run("account check on refresh", func(t *testing.T) {
		f := newFixture(t)
		tokens, _ := f.exchange(f.code(t))
		if _, err := f.s.Exchange(context.Background(), "refresh_token", tokens.RefreshToken, f.client.ID, "", "", f.a.Resource, "", func(Grant) error { return errors.New("no longer admin") }); err == nil {
			t.Fatal("removed admin refreshed")
		}
	})
}
func TestCredentialsStoredOnlyAsHashesAndConcurrentExchange(t *testing.T) {
	f := newFixture(t)
	code := f.code(t)
	tokens, err := f.exchange(code)
	if err != nil {
		t.Fatal(err)
	}
	m := f.s.store.(*memoryStore)
	raw, _ := json.Marshal(m.data)
	for _, secret := range []string{code, tokens.AccessToken, tokens.RefreshToken, f.verifier} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("raw credential persisted")
		}
	}
	f = newFixture(t)
	code = f.code(t)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.exchange(code); results <- err }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("code exchanged %d times", success)
	}
}
