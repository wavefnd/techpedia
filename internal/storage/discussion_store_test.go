package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"techpedia/internal/discussion"
	"techpedia/internal/document"
)

func TestDiscussionBoardLifecycle(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	topic, err := store.CreateDiscussionTopic(ctx, discussion.Topic{
		ID: "dt_test", Locale: "ko", Slug: "c", PageTitle: "C",
		Subject: "표준 문서 범위", Body: "어디까지 설명할까요?", AuthorID: "U1", Author: "작성자",
	})
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}
	if topic.ReplyCount != 0 || topic.CreatedAt.IsZero() {
		t.Fatalf("unexpected topic: %#v", topic)
	}

	listed, err := store.ListDiscussionTopics(ctx, "ko", "c", 10)
	if err != nil || len(listed) != 1 || listed[0].ID != topic.ID {
		t.Fatalf("list topics: %#v, %v", listed, err)
	}
	reply, err := store.AddDiscussionReply(ctx, discussion.Reply{
		ID: "dr_test", TopicID: topic.ID, Body: "C17까지 포함합시다.", AuthorID: "U2", Author: "답변자",
	})
	if err != nil {
		t.Fatalf("add reply: %v", err)
	}
	thread, err := store.GetDiscussionThread(ctx, topic.ID)
	if err != nil || thread.Topic.ReplyCount != 1 || len(thread.Replies) != 1 || thread.Replies[0].ID != reply.ID {
		t.Fatalf("get thread: %#v, %v", thread, err)
	}
	archived, err := store.ArchiveDiscussionTopic(ctx, topic.ID, "U1")
	if err != nil || archived.ArchivedAt.IsZero() || archived.ArchivedBy != "U1" {
		t.Fatalf("archive topic: %#v, %v", archived, err)
	}
	if _, err := store.AddDiscussionReply(ctx, discussion.Reply{ID: "dr_late", TopicID: topic.ID, Body: "late", AuthorID: "U2", Author: "답변자"}); !errors.Is(err, discussion.ErrArchived) {
		t.Fatalf("expected archived topic to reject replies, got %v", err)
	}

	if err := store.DeleteDiscussionReply(ctx, topic.ID, reply.ID); err != nil {
		t.Fatalf("delete reply: %v", err)
	}
	if err := store.DeleteDiscussionTopic(ctx, topic.ID); err != nil {
		t.Fatalf("delete topic: %v", err)
	}
	if _, err := store.GetDiscussionThread(ctx, topic.ID); !errors.Is(err, discussion.ErrNotFound) {
		t.Fatalf("expected deleted topic to be absent, got %v", err)
	}
}

func TestMoveDocumentPreservesHistoryMetricsAndDiscussion(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	for _, source := range []string{"# Old\n\nfirst", "# Old\n\nsecond"} {
		if _, err := store.Save(ctx, document.Document{Locale: "en", Slug: "Old", Title: "Old", Source: source}); err != nil {
			t.Fatalf("save revision: %v", err)
		}
	}
	if err := store.RecordPageView(ctx, "en", "Old", time.Now()); err != nil {
		t.Fatalf("record view: %v", err)
	}
	if _, err := store.CreateDiscussionTopic(ctx, discussion.Topic{
		ID: "dt_move", Locale: "en", Slug: "Old", PageTitle: "Old", Subject: "Rename", Body: "Keep this thread.", AuthorID: "U1", Author: "Editor",
	}); err != nil {
		t.Fatalf("create topic: %v", err)
	}

	moved, err := store.MoveDocument(ctx, "en", "Old", "New_Name", "New Name")
	if err != nil {
		t.Fatalf("move document: %v", err)
	}
	if moved.Title != "New Name" || moved.Revision != "r2" {
		t.Fatalf("unexpected moved document: %#v", moved)
	}
	if _, err := store.GetLatest(ctx, "en", "Old"); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("old document should be absent, got %v", err)
	}
	if target, err := store.GetDocumentRedirect(ctx, "en", "Old"); err != nil || target != "New_Name" {
		t.Fatalf("redirect target: %q, %v", target, err)
	}
	revisions, err := store.ListRevisions(ctx, "en", "New_Name", 10)
	if err != nil || len(revisions) != 2 || revisions[0].Title != "New Name" {
		t.Fatalf("moved revisions: %#v, %v", revisions, err)
	}
	topics, err := store.ListDiscussionTopics(ctx, "en", "New_Name", 10)
	if err != nil || len(topics) != 1 || topics[0].PageTitle != "New Name" {
		t.Fatalf("moved discussions: %#v, %v", topics, err)
	}
	popular, err := store.ListPopular(ctx, "en", time.Now().Add(-24*time.Hour), 10)
	if err != nil || len(popular) != 1 || popular[0].Document.Title != "New Name" || popular[0].Views != 1 {
		t.Fatalf("moved metrics: %#v, %v", popular, err)
	}

	retitled, err := store.MoveDocument(ctx, "en", "New_Name", "New_Name", "New name")
	if err != nil || retitled.ID != moved.ID || retitled.Title != "New name" || retitled.Slug != "New_Name" {
		t.Fatalf("title-only move: %#v, %v", retitled, err)
	}
	revisions, err = store.ListRevisions(ctx, "en", "New_Name", 10)
	if err != nil || len(revisions) != 2 || revisions[0].Title != "New name" {
		t.Fatalf("title-only move did not preserve revisions: %#v, %v", revisions, err)
	}
	topics, err = store.ListDiscussionTopics(ctx, "en", "New_Name", 10)
	if err != nil || len(topics) != 1 || topics[0].PageTitle != "New name" {
		t.Fatalf("title-only move did not update discussions: %#v, %v", topics, err)
	}
	popular, err = store.ListPopular(ctx, "en", time.Now().Add(-24*time.Hour), 10)
	if err != nil || len(popular) != 1 || popular[0].Document.Title != "New name" || popular[0].Views != 1 {
		t.Fatalf("title-only move did not preserve metrics: %#v, %v", popular, err)
	}
}
