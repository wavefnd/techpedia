package storage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	badger "github.com/dgraph-io/badger/v4"

	"techpedia/internal/bot"
	"techpedia/internal/document"
	"techpedia/internal/techdata"
)

func TestResetDocumentHistoryKeepsCurrentIdentityAndPurgesRelatedContent(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	first, err := store.Save(ctx, document.Document{
		Locale: "ko", Slug: "private-project", Title: "Private project",
		Source: "old legal name", Author: "legal-name", Summary: "initial legal-name",
	})
	if err != nil {
		t.Fatal(err)
	}
	secondInput := first
	secondInput.Source = "safe nickname"
	secondInput.Author = "nickname"
	secondInput.Summary = "remove personal information"
	second, err := store.SaveIfRevision(ctx, secondInput, first.Revision)
	if err != nil {
		t.Fatal(err)
	}
	item, err := store.CreateItem(ctx, techdata.Item{
		Labels:    map[string]string{"ko": second.Title},
		WikiPages: map[string]document.WID{"ko": second.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, userID := range []string{"U1", "U2"} {
		if _, err := store.SaveDraft(ctx, document.Draft{UserID: userID, Locale: "ko", Slug: second.Slug, Name: second.Title, Source: "draft legal-name"}); err != nil {
			t.Fatal(err)
		}
	}
	suggestion := bot.NamingSuggestion{
		SchemaVersion: bot.NamingSuggestionSchemaVersion, ID: "ns2_privacytest", Locale: "ko", Slug: second.Slug,
		PageName: second.Title, BaseRevision: second.Revision, OriginalSource: "old legal-name",
		ProposedName: second.Title, ProposedSource: "safe nickname", Findings: []bot.Finding{{CheckerID: "test", Area: "link"}},
	}
	if _, _, err := store.CreateNamingSuggestion(ctx, suggestion); err != nil {
		t.Fatal(err)
	}
	legacySuggestion, _ := json.Marshal(map[string]any{"locale": "ko", "slug": second.Slug, "original_source": "old legal-name"})
	legacyDocument := second
	legacyDocument.Source = "legacy latest legal-name"
	legacyBlob, err := marshalDocumentXML(legacyDocument)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.db.Update(func(txn *badger.Txn) error {
		if err := txn.Set([]byte(legacyNamingSuggestionPrefix+"legacy-privacy"), legacySuggestion); err != nil {
			return err
		}
		return txn.Set([]byte(legacyLatestDocumentKey(second.Slug)), legacyBlob)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Search(ctx, "ko", "safe nickname", 10); err != nil {
		t.Fatal(err)
	}
	searchGeneration := store.searchGeneration["ko"]

	if _, err := store.ResetDocumentHistory(ctx, "ko", second.Slug, first.Revision, "owner"); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale history reset = %v, want revision conflict", err)
	}
	result, err := store.ResetDocumentHistory(ctx, "ko", second.Slug, second.Revision, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if result.RemovedRevisions != 2 || result.RemovedChanges != 2 || result.RemovedDrafts != 2 || result.RemovedSuggestions != 2 {
		t.Fatalf("unexpected reset counts: %#v", result)
	}
	if result.Document.HTML != "" {
		t.Fatalf("derived HTML was retained after reset: %q", result.Document.HTML)
	}
	if store.searchGeneration["ko"] <= searchGeneration {
		t.Fatalf("search generation was not invalidated: %d -> %d", searchGeneration, store.searchGeneration["ko"])
	}
	if err := store.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get([]byte(legacyLatestDocumentKey(second.Slug)))
		return err
	}); !errors.Is(err, badger.ErrKeyNotFound) {
		t.Fatalf("legacy latest document remains: %v", err)
	}
	if result.Document.ID != first.ID || result.Document.Revision != "r1" || result.Document.Source != "safe nickname" || result.Document.Author != "owner" || result.Document.Summary != "" {
		t.Fatalf("history reset changed current identity or retained metadata: %#v", result.Document)
	}
	if !result.Document.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("created time changed: %v -> %v", first.CreatedAt, result.Document.CreatedAt)
	}
	revisions, err := store.ListRevisions(ctx, "ko", second.Slug, 100)
	if err != nil || len(revisions) != 1 || revisions[0].Revision != "r1" || revisions[0].Author != "owner" {
		t.Fatalf("unexpected reset history: %#v, %v", revisions, err)
	}
	if _, err := store.GetRevision(ctx, "ko", second.Slug, "r2"); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("old revision remains readable: %v", err)
	}
	for _, userID := range []string{"U1", "U2"} {
		if _, err := store.GetDraft(ctx, userID, "ko", second.Slug); !errors.Is(err, ErrDocumentNotFound) {
			t.Fatalf("draft %s remains readable: %v", userID, err)
		}
	}
	if suggestions, err := store.ListNamingSuggestions(ctx, "", 100); err != nil || len(suggestions) != 0 {
		t.Fatalf("suggestions remain after reset: %#v, %v", suggestions, err)
	}
	changes, err := store.ListChanges(ctx, document.ChangeFilter{Locale: "ko", Limit: 20})
	if err != nil || len(changes) != 1 || changes[0].Revision != "r1" || changes[0].Author != "owner" || changes[0].Summary != "" {
		t.Fatalf("old recent changes remain after reset: %#v, %v", changes, err)
	}
	if linked, err := store.GetItemByWikiPage(ctx, first.ID); err != nil || linked.ID != item.ID {
		t.Fatalf("Techdata membership changed: %#v, %v", linked, err)
	}
	if err := store.db.View(func(txn *badger.Txn) error {
		for _, prefix := range []string{"wiki:v1:history:", "wiki:v1:recent-revision:", "wiki:v1:links:", "wiki:v1:backlink:", "search:v1:document:"} {
			if err := collectJSONValues(ctx, txn, prefix, func(value []byte) error {
				if strings.Contains(string(value), "legal-name") || strings.Contains(string(value), "old legal name") {
					t.Errorf("personal text retained in %s", prefix)
				}
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	nextInput := result.Document
	nextInput.Source = "safe follow-up"
	next, err := store.SaveIfRevision(ctx, nextInput, "r1")
	if err != nil || next.Revision != "r2" {
		t.Fatalf("revision sequence did not restart at r1: %#v, %v", next, err)
	}
}

func TestConditionalSaveRejectsStaleRevision(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	first, err := store.SaveIfRevision(ctx, document.Document{Locale: "ko", Slug: "c", Title: "C", Source: "first", Author: "one"}, "")
	if err != nil {
		t.Fatalf("create document: %v", err)
	}
	secondInput := first
	secondInput.Source = "second"
	secondInput.Author = "two"
	second, err := store.SaveIfRevision(ctx, secondInput, first.Revision)
	if err != nil {
		t.Fatalf("save second revision: %v", err)
	}
	stale := first
	stale.Source = "stale overwrite"
	if _, err := store.SaveIfRevision(ctx, stale, first.Revision); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
	latest, err := store.GetLatest(ctx, "ko", "c")
	if err != nil || latest.Revision != second.Revision || latest.Source != "second" {
		t.Fatalf("stale save changed latest document: %#v, %v", latest, err)
	}
}

func TestDraftsAreSeparatedByUserAndDocument(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	if _, err := store.SaveDraft(ctx, document.Draft{UserID: "U1", Locale: "ko", Slug: "c", Name: "C", Source: "one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveDraft(ctx, document.Draft{UserID: "U2", Locale: "ko", Slug: "c", Name: "C", Source: "two"}); err != nil {
		t.Fatal(err)
	}
	first, _ := store.GetDraft(ctx, "U1", "ko", "c")
	second, _ := store.GetDraft(ctx, "U2", "ko", "c")
	if first.Source != "one" || second.Source != "two" {
		t.Fatalf("draft ownership leaked: %#v %#v", first, second)
	}
	if err := store.DeleteDraft(ctx, "U1", "ko", "c"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetDraft(ctx, "U1", "ko", "c"); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("deleted draft remains readable: %v", err)
	}
}

func TestRevisionRestoreAndTrashPreserveIdentity(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	first, err := store.Save(ctx, document.Document{Locale: "ko", Slug: "operating-system", Title: "운영체제", Source: "first", Author: "editor", EditPolicy: document.EditPolicyAdmins})
	if err != nil {
		t.Fatal(err)
	}
	secondInput := first
	secondInput.Source = "second"
	secondInput.EditPolicy = document.EditPolicyEditors
	second, err := store.SaveIfRevision(ctx, secondInput, first.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateItem(ctx, techdata.Item{Labels: map[string]string{"ko": first.Title}, WikiPages: map[string]document.WID{"ko": first.ID}}); err != nil {
		t.Fatalf("create Techdata item: %v", err)
	}
	restoredRevision, err := store.RestoreRevision(ctx, "ko", first.Slug, first.Revision, second.Revision, "admin", "잘못된 변경 취소")
	if err != nil || restoredRevision.Revision != "r3" || restoredRevision.Source != first.Source || restoredRevision.EditPolicy != document.EditPolicyEditors {
		t.Fatalf("restore revision: %#v, %v", restoredRevision, err)
	}
	entry, err := store.DeleteDocument(ctx, "ko", first.Slug, "admin", "정리")
	if err != nil || entry.Document.ID != first.ID {
		t.Fatalf("soft delete: %#v, %v", entry, err)
	}
	if _, err := store.GetLatest(ctx, "ko", first.Slug); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("deleted latest remains public: %v", err)
	}
	if revision, err := store.GetRevision(ctx, "ko", first.Slug, "r3"); err != nil || revision.ID != first.ID {
		t.Fatalf("soft delete removed revisions: %#v, %v", revision, err)
	}
	if item, err := store.GetItemByWikiPage(ctx, first.ID); err != nil || item.WikiPages["ko"] != first.ID {
		t.Fatalf("soft delete removed Techdata membership: %#v, %v", item, err)
	}
	restored, err := store.RestoreDeletedDocument(ctx, first.ID, "admin", "복구")
	if err != nil || restored.ID != first.ID || restored.Revision != "r3" {
		t.Fatalf("restore deleted document: %#v, %v", restored, err)
	}
	if byID, err := store.GetLatestByWID(ctx, first.ID); err != nil || byID.Slug != first.Slug {
		t.Fatalf("WID index was not restored: %#v, %v", byID, err)
	}
	changes, err := store.ListChanges(ctx, document.ChangeFilter{Locale: "ko", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	foundRestore, foundDelete := false, false
	for _, change := range changes {
		foundRestore = foundRestore || change.Action == document.ChangeRestore
		foundDelete = foundDelete || change.Action == document.ChangeDelete
	}
	if !foundRestore || !foundDelete {
		t.Fatalf("workflow changes missing: %#v", changes)
	}
	edits, err := store.ListChanges(ctx, document.ChangeFilter{Locale: "ko", Action: document.ChangeEdit, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range edits {
		if change.Revision == restoredRevision.Revision {
			t.Fatalf("restored revision was incorrectly inferred as an edit: %#v", edits)
		}
	}
}

func TestAuditLogFiltersEntries(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.AppendAudit(ctx, document.AuditEntry{Actor: "admin", Action: "document.move", TargetType: "document", Target: "ko:C", Result: "success"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendAudit(ctx, document.AuditEntry{Actor: "other", Action: "edition.portal", TargetType: "edition", Target: "en", Result: "success"}); err != nil {
		t.Fatal(err)
	}
	entries, err := store.ListAudit(ctx, document.AuditFilter{Actor: "admin", Action: "document.move", Limit: 10})
	if err != nil || len(entries) != 1 || entries[0].Target != "ko:C" {
		t.Fatalf("unexpected audit result: %#v, %v", entries, err)
	}
}

func TestWorkflowChangeBackfillRunsOnceAcrossDocumentMove(t *testing.T) {
	path := t.TempDir()
	store, err := OpenBadgerDocumentStore(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, err := store.Save(ctx, document.Document{Locale: "ko", Slug: "old-name", Title: "Old name", Source: "one"})
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.Source = "two"
	if _, err := store.SaveIfRevision(ctx, second, first.Revision); err != nil {
		t.Fatal(err)
	}
	if err := store.db.Update(func(txn *badger.Txn) error {
		if err := deleteKeysByPrefix(txn, changePrefix); err != nil {
			return err
		}
		if err := deleteKeysByPrefix(txn, "migration:v1:"+revisionSummaryMigration+":"); err != nil {
			return err
		}
		return deleteKeysByPrefix(txn, changeRevisionMarkerPrefix)
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = OpenBadgerDocumentStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MoveDocumentAs(ctx, "ko", "old-name", "new-name", "New name", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenBadgerDocumentStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	changes, err := store.ListChanges(ctx, document.ChangeFilter{Locale: "ko", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 {
		t.Fatalf("expected one create, edit and move after repeated startup, got %#v", changes)
	}
}
