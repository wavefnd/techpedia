package storage

import (
	"context"
	"errors"
	"techpedia/internal/document"
	"techpedia/internal/techmark"
	"testing"
	"time"

	"techpedia/internal/media"
)

func TestMediaMetadataUsesGlobalNamesAndSearch(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	file := media.File{
		Name: "Linux_tree.webp", Hash: "abc123", MIME: "image/webp", Width: 960, Height: 540, SizeBytes: 30,
		OriginalName: "Linux tree.png", OriginalMIME: "image/png", UploadedBy: "editor", CreatedAt: time.Now().UTC(),
		Variants: map[string]media.Variant{
			"thumb":   {Path: "ab/cd/hash/thumb.webp", Width: 320, Height: 180, SizeBytes: 10},
			"article": {Path: "ab/cd/hash/article.webp", Width: 960, Height: 540, SizeBytes: 10},
			"large":   {Path: "ab/cd/hash/large.webp", Width: 960, Height: 540, SizeBytes: 10},
		},
	}
	saved, err := store.SaveMedia(context.Background(), file)
	if err != nil || saved.SearchName != "linux tree" {
		t.Fatalf("save media: %#v, %v", saved, err)
	}
	loaded, err := store.GetMedia(context.Background(), "linux_TREE.webp")
	if err != nil || loaded.Name != file.Name {
		t.Fatalf("case-insensitive global lookup: %#v, %v", loaded, err)
	}
	byHash, err := store.GetMediaByHash(context.Background(), file.Hash)
	if err != nil || byHash.Name != file.Name {
		t.Fatalf("hash lookup: %#v, %v", byHash, err)
	}
	results, err := store.SearchMedia(context.Background(), "linux tree", 10)
	if err != nil || len(results) != 1 || results[0].Name != file.Name {
		t.Fatalf("search media: %#v, %v", results, err)
	}
	file.Name = "LINUX_TREE.webp"
	file.Hash = "different"
	if _, err := store.SaveMedia(context.Background(), file); !errors.Is(err, media.ErrAlreadyExists) {
		t.Fatalf("expected case-insensitive filename conflict, got %v", err)
	}
}

func TestDeleteMediaChecksCurrentGalleryAndReservesOldURL(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	file := media.File{Name: "Photo.webp", Hash: "abc123", Width: 80, Height: 40, SizeBytes: 3, Variants: map[string]media.Variant{}}
	for _, v := range []string{"thumb", "article", "large"} {
		file.Variants[v] = media.Variant{Path: "ab/hash/" + v + ".webp", Width: 80, Height: 40, SizeBytes: 1}
	}
	if _, err := store.SaveMedia(ctx, file); err != nil {
		t.Fatal(err)
	}
	doc, err := store.Save(ctx, document.Document{Locale: "ko", Title: "Media", Slug: techmark.Slugify("Media"), Source: `<wiki-infobox><wiki-image file="Other.webp,Photo.webp"></wiki-image></wiki-infobox>`})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeleteMedia(ctx, file.Name, file.Hash, "owner"); !errors.Is(err, media.ErrInUse) {
		t.Fatal("used gallery file deleted", err)
	}
	if _, err := store.GetMediaByHash(ctx, file.Hash); err != nil {
		t.Fatal("failed deletion changed hash index", err)
	}
	doc.Source = "`<wiki-image file=\"Photo.webp\"></wiki-image>`"
	if _, err := store.Save(ctx, doc); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeleteMedia(ctx, file.Name, "wrong", "owner"); !errors.Is(err, media.ErrInvalidName) {
		t.Fatal("stale hash accepted", err)
	}
	if _, err := store.DeleteMedia(ctx, file.Name, file.Hash, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetMedia(ctx, file.Name); !errors.Is(err, media.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := store.GetMediaByHash(ctx, file.Hash); !errors.Is(err, media.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := store.SaveMedia(ctx, file); !errors.Is(err, media.ErrAlreadyExists) {
		t.Fatal("old cached URL can be reused", err)
	}
	listed, err := store.SearchMedia(ctx, "", 100)
	if err != nil || len(listed) != 0 {
		t.Fatal(listed, err)
	}
	audits, err := store.ListAudit(ctx, document.AuditFilter{})
	if err != nil || len(audits) != 1 || audits[0].Action != "media.delete" || audits[0].Detail != "" {
		t.Fatal(audits, err)
	}
}
