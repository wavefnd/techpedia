package storage

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"techpedia/internal/document"
)

func BenchmarkRevisionSummaryRead(b *testing.B) {
	for _, count := range []int{10, 500} {
		b.Run(fmt.Sprintf("revisions_%d", count), func(b *testing.B) {
			store, err := OpenBadgerDocumentStore(b.TempDir())
			if err != nil {
				b.Fatal(err)
			}
			defer func() { store.Close() }()
			ctx := context.Background()
			source := strings.Repeat("Audit revision text.\n", 800)
			for range count {
				if _, err := store.Save(ctx, document.Document{Locale: "en", Slug: "audit", Title: "Audit", Source: source}); err != nil {
					b.Fatal(err)
				}
			}
			path := store.db.Opts().Dir
			if err := store.Close(); err != nil {
				b.Fatal(err)
			}
			store, err = OpenBadgerDocumentStore(path)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := store.ListRevisions(ctx, "en", "audit", 8); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := store.ListRevisions(ctx, "en", "audit", 8); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkWarmDocumentSearch(b *testing.B) {
	for _, count := range []int{100, 500} {
		b.Run(fmt.Sprintf("matching_documents_%d", count), func(b *testing.B) {
			store, err := OpenBadgerDocumentStore(b.TempDir())
			if err != nil {
				b.Fatal(err)
			}
			defer func() { store.Close() }()
			ctx := context.Background()
			source := strings.Repeat("Audit searchable body with [[Target]].\n", 400)
			for i := range count {
				if _, err := store.Save(ctx, document.Document{Locale: "en", Slug: fmt.Sprintf("audit-%d", i), Title: fmt.Sprintf("Audit %d", i), Source: source}); err != nil {
					b.Fatal(err)
				}
			}
			path := store.db.Opts().Dir
			if err := store.Close(); err != nil {
				b.Fatal(err)
			}
			store, err = OpenBadgerDocumentStore(path)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := store.Search(ctx, "en", "Audit", 8); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := store.Search(ctx, "en", "Audit", 8); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
