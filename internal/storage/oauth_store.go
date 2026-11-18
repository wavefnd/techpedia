package storage

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
	"techpedia/internal/document"
	"techpedia/internal/oauth"
)

type oauthTx struct {
	ctx context.Context
	txn *badger.Txn
}

const oauthPrefix = "oauth:v1:"

func (t oauthTx) Audit(event oauth.Event) error {
	if err := t.ctx.Err(); err != nil {
		return err
	}
	entry := document.AuditEntry{ID: orderedWorkflowID(time.Now().UTC()), Actor: event.Actor, Action: event.Action, TargetType: "oauth", Target: event.Target, Result: event.Result, CreatedAt: time.Now().UTC()}
	if strings.HasPrefix(event.Action, "mcp.") {
		entry.TargetType = "mcp_proposal"
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return t.txn.Set([]byte(auditPrefix+entry.ID), data)
}

func (t oauthTx) Get(key string, value any) error {
	if err := t.ctx.Err(); err != nil {
		return err
	}
	item, err := t.txn.Get([]byte(oauthPrefix + key))
	if err == badger.ErrKeyNotFound {
		return oauth.ErrMissing
	}
	if err != nil {
		return err
	}
	return item.Value(func(data []byte) error { return json.Unmarshal(data, value) })
}
func (t oauthTx) Put(key string, value any, expires time.Time) error {
	if err := t.ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	entry := badger.NewEntry([]byte(oauthPrefix+key), data)
	if !expires.IsZero() {
		entry.ExpiresAt = uint64(expires.Unix())
	}
	return t.txn.SetEntry(entry)
}
func (t oauthTx) List(prefix string) ([]json.RawMessage, error) {
	if err := t.ctx.Err(); err != nil {
		return nil, err
	}
	options := badger.DefaultIteratorOptions
	options.Prefix = []byte(oauthPrefix + prefix)
	iter := t.txn.NewIterator(options)
	defer iter.Close()
	out := []json.RawMessage{}
	for iter.Rewind(); iter.Valid(); iter.Next() {
		if err := t.ctx.Err(); err != nil {
			return nil, err
		}
		item := iter.Item()
		if !strings.HasPrefix(string(item.Key()), oauthPrefix+prefix) {
			break
		}
		data, err := item.ValueCopy(nil)
		if err != nil {
			return nil, err
		}
		out = append(out, json.RawMessage(data))
	}
	return out, nil
}
func (s *BadgerDocumentStore) OAuthView(ctx context.Context, fn func(oauth.Tx) error) error {
	return s.db.View(func(tx *badger.Txn) error { return fn(oauthTx{ctx, tx}) })
}
func (s *BadgerDocumentStore) OAuthUpdate(ctx context.Context, fn func(oauth.Tx) error) error {
	return s.db.Update(func(tx *badger.Txn) error { return fn(oauthTx{ctx, tx}) })
}
