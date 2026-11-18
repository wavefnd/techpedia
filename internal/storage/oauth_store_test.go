package storage_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"

	"techpedia/internal/oauth"
	"techpedia/internal/storage"
)

type failingAuditTx struct{ oauth.Tx }

func (f failingAuditTx) Audit(oauth.Event) error { return errors.New("audit unavailable") }

type failingAuditStore struct{ oauth.Store }

func (f failingAuditStore) OAuthUpdate(ctx context.Context, fn func(oauth.Tx) error) error {
	return f.Store.OAuthUpdate(ctx, func(tx oauth.Tx) error { return fn(failingAuditTx{tx}) })
}

func TestOAuthAuditFailureRollsBackClient(t *testing.T) {
	db, err := storage.OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := oauth.New(failingAuditStore{db}, "https://wiki.example")
	if _, err := s.Register(context.Background(), "ChatGPT", "https://chatgpt.example/callback", oauth.Actor{ID: "admin", Username: "admin"}); err == nil {
		t.Fatal("client saved without audit")
	}
	clients, err := s.Clients(context.Background())
	if err != nil || len(clients) != 0 {
		t.Fatalf("rollback: %+v %v", clients, err)
	}
}
func TestOAuthBadgerRestartAndConcurrentRefresh(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir()
	db, err := storage.OpenBadgerDocumentStore(path)
	if err != nil {
		t.Fatal(err)
	}
	s := oauth.New(db, "https://wiki.example")
	actor := oauth.Actor{ID: "admin", Username: "admin"}
	client, err := s.Register(ctx, "ChatGPT", "https://chatgpt.example/callback", actor)
	if err != nil {
		t.Fatal(err)
	}
	verifier := strings.Repeat("v", 43)
	sum := sha256.Sum256([]byte(verifier))
	a := oauth.Authorization{ClientID: client.ID, RedirectURI: client.RedirectURI, ResponseType: "code", Challenge: base64.RawURLEncoding.EncodeToString(sum[:]), ChallengeMethod: "S256", Scope: oauth.Scope, Resource: "https://wiki.example/mcp"}
	p, _, err := s.Prepare(ctx, a, actor)
	if err != nil {
		t.Fatal(err)
	}
	callback, err := s.Decide(ctx, p.ID, actor, true)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(callback)
	check := func(oauth.Grant) error { return nil }
	tokens, err := s.Exchange(ctx, "authorization_code", u.Query().Get("code"), client.ID, client.RedirectURI, verifier, a.Resource, "", check)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = storage.OpenBadgerDocumentStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s = oauth.New(db, "https://wiki.example")
	if _, err := s.Access(ctx, tokens.AccessToken); err != nil {
		t.Fatal("restart lost token", err)
	}
	var wg sync.WaitGroup
	success := make(chan bool, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Exchange(ctx, "refresh_token", tokens.RefreshToken, client.ID, "", "", a.Resource, "", check)
			success <- err == nil
		}()
	}
	wg.Wait()
	close(success)
	count := 0
	for ok := range success {
		if ok {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("refresh issued %d times", count)
	}
}
