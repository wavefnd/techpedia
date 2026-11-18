package storage

import (
	"context"
	"errors"
	"testing"

	"techpedia/internal/document"
	"techpedia/internal/techmark"
)

func TestManagedDocumentRedirectFollowsTargetWID(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	canonical, err := store.Save(ctx, document.Document{
		Locale: "en", Slug: techmark.Slugify("Berkeley Software Distribution"), Title: "Berkeley Software Distribution", Source: "# Berkeley Software Distribution",
	})
	if err != nil {
		t.Fatal(err)
	}

	redirect, err := store.CreateDocumentRedirect(ctx, "en", "BSD", canonical.Slug, "administrator")
	if err != nil {
		t.Fatal(err)
	}
	if redirect.TargetWID != canonical.ID || redirect.TargetName != canonical.Title || !redirect.TargetExists {
		t.Fatalf("unexpected redirect: %#v", redirect)
	}
	if target, err := store.GetDocumentRedirect(ctx, "en", techmark.Slugify("BSD")); err != nil || target != canonical.Slug {
		t.Fatalf("resolve managed redirect: %q, %v", target, err)
	}
	if _, err := store.Save(ctx, document.Document{Locale: "en", Slug: techmark.Slugify("BSD"), Title: "BSD", Source: "# BSD"}); !errors.Is(err, ErrDocumentExists) {
		t.Fatalf("document shadowed redirect: %v", err)
	}

	moved, err := store.MoveDocument(ctx, "en", canonical.Slug, techmark.Slugify("Berkeley Software Distribution family"), "Berkeley Software Distribution family")
	if err != nil {
		t.Fatal(err)
	}
	if moved.ID != canonical.ID {
		t.Fatalf("move changed WID: %s -> %s", canonical.ID, moved.ID)
	}
	if target, err := store.GetDocumentRedirect(ctx, "en", techmark.Slugify("BSD")); err != nil || target != moved.Slug {
		t.Fatalf("managed redirect did not follow move: %q, %v", target, err)
	}
	if target, err := store.GetDocumentRedirect(ctx, "en", canonical.Slug); err != nil || target != moved.Slug {
		t.Fatalf("move redirect did not resolve final document: %q, %v", target, err)
	}
	redirects, err := store.ListDocumentRedirects(ctx, "en", 300)
	if err != nil || len(redirects) != 1 || redirects[0].TargetName != moved.Title {
		t.Fatalf("list managed redirects: %#v, %v", redirects, err)
	}

	deleted, err := store.DeleteDocumentRedirect(ctx, "en", techmark.Slugify("BSD"))
	if err != nil || deleted.Name != "BSD" {
		t.Fatalf("delete managed redirect: %#v, %v", deleted, err)
	}
	if _, err := store.GetDocumentRedirect(ctx, "en", techmark.Slugify("BSD")); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("deleted redirect still resolves: %v", err)
	}
	if _, err := store.Save(ctx, document.Document{Locale: "en", Slug: techmark.Slugify("BSD"), Title: "BSD", Source: "# BSD"}); err != nil {
		t.Fatalf("deleted redirect name was not released: %v", err)
	}
}

func TestManagedDocumentRedirectRejectsConflictsAndProtectsRestore(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	target, err := store.Save(ctx, document.Document{Locale: "en", Slug: "target", Title: "Target", Source: "# Target"})
	if err != nil {
		t.Fatal(err)
	}
	shadow, err := store.Save(ctx, document.Document{Locale: "en", Slug: "bsd", Title: "BSD", Source: "# BSD"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeleteDocument(ctx, "en", shadow.Slug, "administrator", "test redirect conflict"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateDocumentRedirect(ctx, "en", "BSD", target.Slug, "administrator"); err != nil {
		t.Fatal(err)
	}
	moving, err := store.Save(ctx, document.Document{Locale: "en", Slug: "moving", Title: "Moving", Source: "# Moving"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MoveDocument(ctx, "en", moving.Slug, "bsd", "BSD"); !errors.Is(err, ErrDocumentExists) {
		t.Fatalf("move shadowed redirect: %v", err)
	}
	if _, err := store.RestoreDeletedDocument(ctx, shadow.ID, "administrator", "restore"); !errors.Is(err, ErrDocumentExists) {
		t.Fatalf("restore shadowed redirect: %v", err)
	}
	if _, err := store.CreateDocumentRedirect(ctx, "en", "BSD", target.Slug, "administrator"); !errors.Is(err, ErrDocumentExists) {
		t.Fatalf("duplicate redirect: %v", err)
	}
	if _, err := store.CreateDocumentRedirect(ctx, "en", "Missing", "not-found", "administrator"); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("redirect accepted missing target: %v", err)
	}
	if _, err := store.CreateDocumentRedirect(ctx, "en", "Target", target.Slug, "administrator"); !errors.Is(err, ErrDocumentExists) {
		t.Fatalf("redirect replaced canonical document: %v", err)
	}
	if _, err := store.CreateDocumentRedirect(ctx, "zh-Hans", "Alias", target.Slug, "administrator"); !errors.Is(err, ErrInvalidLocale) {
		t.Fatalf("redirect accepted invalid edition locale: %v", err)
	}
}
