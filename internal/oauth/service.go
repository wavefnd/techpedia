// Package oauth implements administrator-approved MCP connections.
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid OAuth request or credential")
var ErrMissing = errors.New("OAuth record not found")

const Scope = "wiki:read"
const ProposeScope = "wiki:propose"
const ProposalScopes = Scope + " " + ProposeScope

func ValidScope(scope string) bool {
	return scope == Scope || scope == ProposalScopes || scope == ProposeScope+" "+Scope
}
func CanPropose(scope string) bool { return ValidScope(scope) && scope != Scope }

// Transactions are restricted to versioned OAuth JSON, never document data.
type Tx interface {
	Get(string, any) error
	Put(string, any, time.Time) error
	List(string) ([]json.RawMessage, error)
	Audit(Event) error
}
type Event struct{ Actor, Action, Target, Result string }
type Store interface {
	OAuthView(context.Context, func(Tx) error) error
	OAuthUpdate(context.Context, func(Tx) error) error
}
type Client struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	RedirectURI string    `json:"redirect_uri"`
	Disabled    bool      `json:"disabled"`
	CreatedAt   time.Time `json:"created_at"`
}
type Actor struct{ ID, Username string }
type Grant struct {
	ID        string    `json:"id"`
	ClientID  string    `json:"client_id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Scope     string    `json:"scope"`
	Resource  string    `json:"resource"`
	Revoked   bool      `json:"revoked"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Rotations int       `json:"rotations"`
}
type Authorization struct {
	ClientID        string `json:"client_id"`
	RedirectURI     string `json:"redirect_uri"`
	ResponseType    string `json:"response_type"`
	Challenge       string `json:"code_challenge"`
	ChallengeMethod string `json:"code_challenge_method"`
	Scope           string `json:"scope"`
	Resource        string `json:"resource"`
	State           string `json:"state"`
}
type Approval struct {
	ID            string        `json:"id"`
	UserID        string        `json:"user_id"`
	Authorization Authorization `json:"authorization"`
	ExpiresAt     time.Time     `json:"expires_at"`
}
type credential struct {
	Kind        string    `json:"kind"`
	GrantID     string    `json:"grant_id"`
	Challenge   string    `json:"challenge,omitempty"`
	RedirectURI string    `json:"redirect_uri,omitempty"`
	Used        bool      `json:"used"`
	ExpiresAt   time.Time `json:"expires_at"`
}
type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}
type Service struct {
	store            Store
	issuer, resource string
	now              func() time.Time
}

func New(store Store, issuer string) *Service {
	return &Service{store: store, issuer: issuer, resource: issuer + "/mcp", now: time.Now}
}
func random() string {
	var value [32]byte
	// Go's crypto/rand.Read fills the slice or terminates on entropy failure.
	rand.Read(value[:])
	return base64.RawURLEncoding.EncodeToString(value[:])
}
func digest(value string) string {
	hash := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}
func validID(value string) bool {
	if len(value) != 43 {
		return false
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(data) == 32 && base64.RawURLEncoding.EncodeToString(data) == value
}
func (s *Service) Register(ctx context.Context, name, redirect string, actor Actor) (Client, error) {
	u, err := url.Parse(redirect)
	if err != nil || u == nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || len(redirect) > 2048 || len([]rune(name)) < 1 || len([]rune(name)) > 80 {
		return Client{}, ErrInvalid
	}
	if u.Scheme != "https" {
		return Client{}, ErrInvalid
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return Client{}, ErrInvalid
	}
	for _, key := range []string{"code", "state", "iss", "error", "error_description"} {
		if q.Has(key) {
			return Client{}, ErrInvalid
		}
	}
	client := Client{ID: random(), Name: name, RedirectURI: redirect, CreatedAt: s.now().UTC()}
	err = s.store.OAuthUpdate(ctx, func(tx Tx) error {
		values, err := tx.List("client:")
		if err != nil {
			return err
		}
		if len(values) >= 32 {
			return ErrInvalid
		}
		if err := tx.Put("client:"+client.ID, client, time.Time{}); err != nil {
			return err
		}
		return tx.Audit(Event{actor.Username, "oauth.client_register", client.ID, "success"})
	})
	return client, err
}
func clientIn(tx Tx, id string) (Client, error) {
	var client Client
	if !validID(id) {
		return client, ErrInvalid
	}
	if err := tx.Get("client:"+id, &client); err != nil {
		return client, err
	}
	if client.Disabled {
		return client, ErrInvalid
	}
	return client, nil
}
func (s *Service) Clients(ctx context.Context) ([]Client, error) {
	result := []Client{}
	err := s.store.OAuthView(ctx, func(tx Tx) error {
		values, err := tx.List("client:")
		if err != nil {
			return err
		}
		for _, raw := range values {
			var c Client
			if err := json.Unmarshal(raw, &c); err != nil {
				return err
			}
			result = append(result, c)
		}
		return nil
	})
	return result, err
}
func (s *Service) Disable(ctx context.Context, id string, actor Actor) error {
	if !validID(id) {
		return ErrInvalid
	}
	return s.store.OAuthUpdate(ctx, func(tx Tx) error {
		var c Client
		if err := tx.Get("client:"+id, &c); err != nil {
			return err
		}
		c.Disabled = true
		if err := tx.Put("client:"+id, c, time.Time{}); err != nil {
			return err
		}
		return tx.Audit(Event{actor.Username, "oauth.client_disable", id, "success"})
	})
}
func (s *Service) validate(tx Tx, a Authorization) (Client, error) {
	c, err := clientIn(tx, a.ClientID)
	if err != nil {
		return Client{}, err
	}
	if a.RedirectURI != c.RedirectURI || a.ResponseType != "code" || a.ChallengeMethod != "S256" || !validID(a.Challenge) || !ValidScope(a.Scope) || a.Resource != s.resource || len(a.State) > 512 {
		return Client{}, ErrInvalid
	}
	return c, nil
}
func (s *Service) Validate(ctx context.Context, a Authorization) (Client, error) {
	var c Client
	err := s.store.OAuthView(ctx, func(tx Tx) error { var err error; c, err = s.validate(tx, a); return err })
	return c, err
}
func (s *Service) Prepare(ctx context.Context, a Authorization, actor Actor) (Approval, Client, error) {
	if actor.ID == "" || actor.Username == "" {
		return Approval{}, Client{}, ErrInvalid
	}
	p := Approval{ID: random(), UserID: actor.ID, Authorization: a, ExpiresAt: s.now().UTC().Add(5 * time.Minute)}
	var c Client
	err := s.store.OAuthUpdate(ctx, func(tx Tx) error {
		var err error
		c, err = s.validate(tx, a)
		if err != nil {
			return err
		}
		values, err := tx.List("approval:")
		if err != nil {
			return err
		}
		if len(values) >= 128 {
			return ErrInvalid
		}
		return tx.Put("approval:"+p.ID, p, p.ExpiresAt)
	})
	return p, c, err
}
func (s *Service) Decide(ctx context.Context, id string, actor Actor, allow bool) (string, error) {
	if !validID(id) {
		return "", ErrInvalid
	}
	var redirect string
	err := s.store.OAuthUpdate(ctx, func(tx Tx) error {
		var p Approval
		if err := tx.Get("approval:"+id, &p); err != nil {
			return err
		}
		if p.UserID != actor.ID || !s.now().Before(p.ExpiresAt) {
			return ErrInvalid
		}
		if _, err := s.validate(tx, p.Authorization); err != nil {
			return err
		}
		// Expire approval atomically with issuance; repeated approvals fail even before TTL cleanup.
		p.ExpiresAt = s.now()
		if err := tx.Put("approval:"+id, p, p.ExpiresAt.Add(time.Minute)); err != nil {
			return err
		}
		u, _ := url.Parse(p.Authorization.RedirectURI)
		q := u.Query()
		q.Set("state", p.Authorization.State)
		q.Set("iss", s.issuer)
		if !allow {
			q.Set("error", "access_denied")
		} else {
			values, err := tx.List("grant:")
			if err != nil {
				return err
			}
			if len(values) >= 256 {
				return ErrInvalid
			}
			g := Grant{ID: random(), ClientID: p.Authorization.ClientID, UserID: actor.ID, Username: actor.Username, Scope: p.Authorization.Scope, Resource: s.resource, CreatedAt: s.now().UTC(), ExpiresAt: s.now().UTC().Add(30 * 24 * time.Hour)}
			code := random()
			c := credential{Kind: "code", GrantID: g.ID, Challenge: p.Authorization.Challenge, RedirectURI: p.Authorization.RedirectURI, ExpiresAt: s.now().Add(5 * time.Minute)}
			if err := tx.Put("grant:"+g.ID, g, g.ExpiresAt); err != nil {
				return err
			}
			if err := tx.Put("credential:"+digest(code), c, c.ExpiresAt); err != nil {
				return err
			}
			q.Set("code", code)
		}
		u.RawQuery = q.Encode()
		redirect = u.String()
		action := "oauth.approve"
		if !allow {
			action = "oauth.deny"
		}
		return tx.Audit(Event{actor.Username, action, "connection", "success"})
	})
	return redirect, err
}
func validVerifier(v string) bool {
	if len(v) < 43 || len(v) > 128 {
		return false
	}
	for _, r := range v {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~", r) {
			return false
		}
	}
	return true
}

// Exchange rotates refresh tokens atomically. Reuse revokes the entire grant,
// including access tokens already issued from it; the revocation must commit.
func (s *Service) Exchange(ctx context.Context, kind, token, clientID, redirect, verifier, resource, scope string, check func(Grant) error) (Tokens, error) {
	if check == nil || !validID(token) || !validID(clientID) || resource != s.resource || (scope != "" && !ValidScope(scope)) {
		return Tokens{}, ErrInvalid
	}
	var out Tokens
	var rejected bool
	err := s.store.OAuthUpdate(ctx, func(tx Tx) error {
		var c credential
		if err := tx.Get("credential:"+digest(token), &c); err != nil {
			return err
		}
		var g Grant
		if err := tx.Get("grant:"+c.GrantID, &g); err != nil {
			return err
		}
		if g.ClientID != clientID || !ValidScope(g.Scope) || (scope != "" && CanPropose(scope) != CanPropose(g.Scope)) || g.Resource != resource || g.Revoked || !s.now().Before(g.ExpiresAt) || !s.now().Before(c.ExpiresAt) {
			return ErrInvalid
		}
		if _, err := clientIn(tx, clientID); err != nil {
			return err
		}
		if err := check(g); err != nil {
			return ErrInvalid
		}
		if kind == "authorization_code" {
			if c.Kind != "code" || !validVerifier(verifier) || redirect != c.RedirectURI || subtle.ConstantTimeCompare([]byte(digest(verifier)), []byte(c.Challenge)) != 1 {
				return ErrInvalid
			}
		} else if kind == "refresh_token" {
			if c.Kind != "refresh" {
				return ErrInvalid
			}
		} else {
			return ErrInvalid
		}
		if c.Used {
			g.Revoked = true
			rejected = true
			if err := tx.Put("grant:"+g.ID, g, g.ExpiresAt); err != nil {
				return err
			}
			return tx.Audit(Event{g.Username, "oauth.reuse_revoke", g.ID, "success"})
		}
		if g.Rotations >= 1024 {
			return ErrInvalid
		}
		g.Rotations++
		if err := tx.Put("grant:"+g.ID, g, g.ExpiresAt); err != nil {
			return err
		}
		c.Used = true
		if err := tx.Put("credential:"+digest(token), c, g.ExpiresAt); err != nil {
			return err
		}
		out = Tokens{AccessToken: random(), RefreshToken: random(), TokenType: "Bearer", ExpiresIn: 900, Scope: g.Scope}
		accessExpiry := s.now().Add(15 * time.Minute)
		if accessExpiry.After(g.ExpiresAt) {
			accessExpiry = g.ExpiresAt
		}
		out.ExpiresIn = int(accessExpiry.Sub(s.now()).Seconds())
		if err := tx.Put("credential:"+digest(out.AccessToken), credential{Kind: "access", GrantID: g.ID, ExpiresAt: accessExpiry}, accessExpiry); err != nil {
			return err
		}
		return tx.Put("credential:"+digest(out.RefreshToken), credential{Kind: "refresh", GrantID: g.ID, ExpiresAt: g.ExpiresAt}, g.ExpiresAt)
	})
	if err != nil {
		return Tokens{}, err
	}
	if rejected {
		return Tokens{}, ErrInvalid
	}
	return out, err
}
func (s *Service) Access(ctx context.Context, token string) (Grant, error) {
	var g Grant
	if !validID(token) {
		return g, ErrInvalid
	}
	err := s.store.OAuthView(ctx, func(tx Tx) error {
		var c credential
		if err := tx.Get("credential:"+digest(token), &c); err != nil {
			return err
		}
		if c.Kind != "access" || !s.now().Before(c.ExpiresAt) {
			return ErrInvalid
		}
		if err := tx.Get("grant:"+c.GrantID, &g); err != nil {
			return err
		}
		if g.Revoked || !s.now().Before(g.ExpiresAt) || !ValidScope(g.Scope) || g.Resource != s.resource {
			return ErrInvalid
		}
		_, err := clientIn(tx, g.ClientID)
		return err
	})
	return g, err
}

// ProposalGrant validates a retained proposal's connection without exposing or
// minting a bearer token. The storage commit separately checks revocation again.
func (s *Service) ProposalGrant(ctx context.Context, id string) (Grant, error) {
	var g Grant
	if !validID(id) {
		return g, ErrInvalid
	}
	err := s.store.OAuthView(ctx, func(tx Tx) error {
		if err := tx.Get("grant:"+id, &g); err != nil {
			return err
		}
		if g.Revoked || !s.now().Before(g.ExpiresAt) || !CanPropose(g.Scope) || g.Resource != s.resource {
			return ErrInvalid
		}
		_, err := clientIn(tx, g.ClientID)
		return err
	})
	return g, err
}
func (s *Service) Grants(ctx context.Context, userID string) ([]Grant, error) {
	result := []Grant{}
	err := s.store.OAuthView(ctx, func(tx Tx) error {
		values, err := tx.List("grant:")
		if err != nil {
			return err
		}
		for _, raw := range values {
			var g Grant
			if err := json.Unmarshal(raw, &g); err != nil {
				return err
			}
			if g.UserID == userID {
				result = append(result, g)
			}
		}
		return nil
	})
	return result, err
}
func (s *Service) Revoke(ctx context.Context, id string, actor Actor) error {
	if !validID(id) {
		return ErrInvalid
	}
	return s.store.OAuthUpdate(ctx, func(tx Tx) error {
		var g Grant
		if err := tx.Get("grant:"+id, &g); err != nil {
			return err
		}
		if g.UserID != actor.ID {
			return ErrInvalid
		}
		g.Revoked = true
		if err := tx.Put("grant:"+id, g, g.ExpiresAt); err != nil {
			return err
		}
		return tx.Audit(Event{actor.Username, "oauth.revoke", id, "success"})
	})
}
