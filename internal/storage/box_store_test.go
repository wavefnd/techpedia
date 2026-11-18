package storage_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"techpedia/internal/box"
	"techpedia/internal/document"
	"techpedia/internal/edition"
	"techpedia/internal/media"
	"techpedia/internal/storage"
)

func boxFixture(t *testing.T) *storage.BadgerDocumentStore {
	t.Helper()
	db, err := storage.OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, locale := range []string{"ko", "en", "ar", "fr", "sr", "bs"} {
		if _, err := db.ProposeEdition(context.Background(), edition.Edition{Tag: locale, Direction: "ltr", Status: edition.StatusPending}); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ApproveEdition(context.Background(), locale, "admin", time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestBoxLanguagesHistoryConflictAuditAndPersistence(t *testing.T) {
	db := boxFixture(t)
	ctx := context.Background()
	var store storage.BoxStore = db
	in := box.Save{Name: "리눅스 박스", Locale: "ko", Source: `<wiki-navbox title="비공개 원문 검사">` + "\n- 분류: [[Linux]]\n</wiki-navbox>"}
	first, err := store.SaveBox(ctx, in, "admin")
	if err != nil || first.Version != 1 {
		t.Fatal(first, err)
	}
	in.Locale = "en"
	in.Source = "<wiki-alert>English</wiki-alert>"
	in.BaseVersion = 1
	in.FallbackLocale = "ko"
	second, err := store.SaveBox(ctx, in, "admin")
	if err != nil || second.Version != 2 || len(second.Locales) != 2 {
		t.Fatal(second, err)
	}
	if _, err := store.SaveBox(ctx, in, "admin"); !errors.Is(err, box.ErrConflict) {
		t.Fatal(err)
	}
	old, err := store.GetBoxRevision(ctx, in.Name, 1)
	if err != nil || old.Content.Source != first.Content.Source || old.Version != 1 {
		t.Fatal(old, err)
	}
	korean, err := store.GetBoxContent(ctx, in.Name, "ko")
	if err != nil || korean.Source != first.Content.Source {
		t.Fatal(korean, err)
	}
	if _, err := store.GetBoxContent(ctx, in.Name, "ar"); !errors.Is(err, box.ErrNotFound) {
		t.Fatal(err)
	}
	audit, err := db.ListAudit(ctx, document.AuditFilter{Action: "box.save", Limit: 20})
	if err != nil || len(audit) != 2 {
		t.Fatal(audit, err)
	}
	for _, a := range audit {
		if strings.Contains(a.Detail, "원문") || strings.Contains(a.Detail, "English") {
			t.Fatal("source leaked to audit", a)
		}
	}
	pages, err := store.ListBoxes(ctx, "리눅스", "", 1)
	if err != nil || len(pages.Boxes) != 1 {
		t.Fatal(pages, err)
	}
}

func TestBoxValidationRejectsUnacceptedLanguagesAndNestedSystemSyntax(t *testing.T) {
	db := boxFixture(t)
	ctx := context.Background()
	for _, locale := range []string{"zh-Hans", "pt-BR", "EN", "und", "xx", "es"} {
		if _, err := db.SaveBox(ctx, box.Save{Name: "Box", Locale: locale, Source: "text"}, "admin"); !errors.Is(err, box.ErrInvalid) {
			t.Fatal(locale, err)
		}
	}
	for _, source := range []string{"", "# heading", `<wiki-box name="x" />`, `<wiki-recent-changes></wiki-recent-changes>`, strings.Repeat("x", box.MaxSourceBytes+1)} {
		if _, err := db.SaveBox(ctx, box.Save{Name: "Box", Locale: "ko", Source: source}, "admin"); !errors.Is(err, box.ErrInvalid) {
			t.Fatal("invalid source accepted", err)
		}
	}
	if _, err := db.SaveBox(ctx, box.Save{Name: "Box", Locale: "ko", Source: "text", FallbackLocale: "ar"}, "admin"); !errors.Is(err, box.ErrInvalid) {
		t.Fatal(err)
	}
	for _, locale := range []string{"sr", "bs"} {
		if _, err := db.SaveBox(ctx, box.Save{Name: locale, Locale: locale, Source: "Unicode العربية 한글 🙂"}, "admin"); err != nil {
			t.Fatal(locale, err)
		}
	}
}

func TestBoxPaginationAndConcurrentUpdates(t *testing.T) {
	db := boxFixture(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if _, err := db.SaveBox(ctx, box.Save{Name: fmt.Sprintf("Box %d", i), Locale: "ko", Source: "text"}, "admin"); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	after := ""
	for {
		p, err := db.ListBoxes(ctx, "Box", after, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range p.Boxes {
			if seen[b.Name] {
				t.Fatal("duplicate page", b.Name)
			}
			seen[b.Name] = true
		}
		if p.Next == "" {
			break
		}
		after = p.Next
	}
	if len(seen) != 5 {
		t.Fatal(seen)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := db.SaveBox(ctx, box.Save{Name: "Box 0", Locale: "en", Source: "English", BaseVersion: 1}, "admin")
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	conflict := 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, box.ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal(success, conflict)
	}
}

func TestBoxMediaReferencesPreventDeletionButHistoryAndCodeDoNot(t *testing.T) {
	db := boxFixture(t)
	ctx := context.Background()
	f := media.File{Name: "Box.webp", Hash: "box-photo", Width: 80, Height: 40, SizeBytes: 3, Variants: map[string]media.Variant{}}
	for _, v := range []string{"thumb", "article", "large"} {
		f.Variants[v] = media.Variant{Path: "aa/box/" + v + ".webp", Width: 80, Height: 40, SizeBytes: 1}
	}
	if _, err := db.SaveMedia(ctx, f); err != nil {
		t.Fatal(err)
	}
	in := box.Save{Name: "Picture", Locale: "ko", Source: `<wiki-infobox><wiki-image file="Box.webp"></wiki-image></wiki-infobox>`}
	if _, err := db.SaveBox(ctx, in, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DeleteMedia(ctx, f.Name, f.Hash, "owner"); !errors.Is(err, media.ErrInUse) {
		t.Fatal(err)
	}
	in.Source = "`<wiki-image file=\"Box.webp\"></wiki-image>`"
	in.BaseVersion = 1
	if _, err := db.SaveBox(ctx, in, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DeleteMedia(ctx, f.Name, f.Hash, "owner"); err != nil {
		t.Fatal(err)
	}
}

func TestBoxResourcesSurviveDatabaseReopen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.OpenBadgerDocumentStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ProposeEdition(ctx, edition.Edition{Tag: "ko", Status: edition.StatusPending}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ApproveEdition(ctx, "ko", "admin", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveBox(ctx, box.Save{Name: "Persistent", Locale: "ko", Source: "저장된 원문"}, "admin"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = storage.OpenBadgerDocumentStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c, err := db.GetBoxContent(ctx, "Persistent", "ko")
	if err != nil || c.Source != "저장된 원문" {
		t.Fatal(c, err)
	}
	history, err := db.GetBoxRevision(ctx, "Persistent", 1)
	if err != nil || history.Content.Source != c.Source {
		t.Fatal(history, err)
	}
}
