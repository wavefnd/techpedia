package bot

import (
	"fmt"
	"testing"

	"techpedia/internal/document"
	"techpedia/internal/techdata"
)

func TestNameIndexBuildsGeneratedAndTechdataNames(t *testing.T) {
	documents := []document.Document{
		{ID: "W1", Locale: "ko", Slug: "c-language", Title: "C (프로그래밍 언어)"},
		{ID: "W2", Locale: "ko", Slug: "rust", Title: "Rust"},
	}
	items := []techdata.Item{{
		ID:        "T1",
		Labels:    map[string]string{"ko": "C (프로그래밍 언어)"},
		Aliases:   map[string][]string{"ko": {"C언어", "C 언어"}},
		WikiPages: map[string]document.WID{"ko": "W1"},
	}}
	index := BuildNameIndex(documents, items)

	for _, query := range []string{"C", "C 프로그래밍 언어", "C프로그래밍언어", "C언어"} {
		match := index.Match("ko", query)
		if !match.Actionable() || match.Candidate.CanonicalName != "C (프로그래밍 언어)" {
			t.Fatalf("Match(%q) = %#v", query, match)
		}
	}
	stats := index.Stats()
	if stats.GeneratorVersion != NameIndexGeneratorVersion || stats.Documents != 2 || stats.Terms < 5 || stats.GramKeys == 0 || stats.GramPostings == 0 {
		t.Fatalf("unexpected index stats: %#v", stats)
	}
}

func TestNameIndexPrefersAnExistingExactTitleOverGeneratedBase(t *testing.T) {
	index := BuildNameIndex([]document.Document{
		{ID: "W1", Locale: "en", Slug: "c-language", Title: "C (programming language)"},
		{ID: "W2", Locale: "en", Slug: "c-letter", Title: "C"},
	}, nil)

	match := index.Match("en", "C")
	if match.Kind != NameMatchExact || match.Candidate.Slug != "c-letter" || match.Candidate.CanonicalName != "C" {
		t.Fatalf("exact title did not win: %#v", match)
	}
}

func TestNameIndexUsesTheOutermostParentheticalBase(t *testing.T) {
	index := BuildNameIndex([]document.Document{{
		ID: "W1", Locale: "en", Slug: "nested", Title: "A (B (technology))",
	}}, nil)
	if match := index.Match("en", "A"); !match.Actionable() || match.Candidate.CanonicalName != "A (B (technology))" {
		t.Fatalf("outer parenthetical base did not resolve: %#v", match)
	}
	if match := index.Match("en", "A (B"); match.Actionable() {
		t.Fatalf("inner parenthetical fragment was indexed: %#v", match)
	}
}

func TestNameIndexReportsAmbiguousAliases(t *testing.T) {
	documents := []document.Document{
		{ID: "W1", Locale: "en", Slug: "alpha", Title: "Alpha System"},
		{ID: "W2", Locale: "en", Slug: "beta", Title: "Beta System"},
	}
	items := []techdata.Item{
		{ID: "T1", Labels: map[string]string{"en": "Alpha System"}, Aliases: map[string][]string{"en": {"Common"}}, WikiPages: map[string]document.WID{"en": "W1"}},
		{ID: "T2", Labels: map[string]string{"en": "Beta System"}, Aliases: map[string][]string{"en": {"Common"}}, WikiPages: map[string]document.WID{"en": "W2"}},
	}
	match := BuildNameIndex(documents, items).Match("en", "Common")
	if match.Kind != NameMatchAmbiguous || len(match.Alternatives) != 2 {
		t.Fatalf("expected an ambiguous match: %#v", match)
	}
}

func TestNameIndexUsesFuzzyCandidateShortlist(t *testing.T) {
	index := BuildNameIndex([]document.Document{
		{ID: "W1", Locale: "en", Slug: "programming-language", Title: "Programming language"},
		{ID: "W2", Locale: "en", Slug: "operating-system", Title: "Operating system"},
	}, nil)
	match := index.Match("en", "Programing language")
	if match.Kind != NameMatchFuzzy || match.Candidate.CanonicalName != "Programming language" {
		t.Fatalf("unexpected fuzzy match: %#v", match)
	}
}

func TestNameIndexNormalizesUnicodeButPreservesTechnicalPunctuation(t *testing.T) {
	index := BuildNameIndex([]document.Document{
		{ID: "W1", Locale: "ko", Slug: "c-sharp", Title: "C#"},
		{ID: "W2", Locale: "ko", Slug: "os-slash-2", Title: "OS/2"},
		{ID: "W3", Locale: "ko", Slug: "os2", Title: "OS2"},
	}, nil)

	if match := index.Match("ko", "C＃"); match.Kind != NameMatchExact || match.Candidate.CanonicalName != "C#" {
		t.Fatalf("full-width compatibility form did not resolve: %#v", match)
	}
	if match := index.Match("ko", "OS/2"); match.Kind != NameMatchExact || match.Candidate.Slug != "os-slash-2" {
		t.Fatalf("OS/2 did not preserve slash identity: %#v", match)
	}
	if match := index.Match("ko", "OS2"); match.Kind != NameMatchExact || match.Candidate.Slug != "os2" {
		t.Fatalf("OS2 collided with OS/2: %#v", match)
	}
}

func TestNameIndexKeepsLocalesIsolated(t *testing.T) {
	documents := []document.Document{
		{ID: "W1", Locale: "ko", Slug: "korean", Title: "한국어 이름"},
		{ID: "W2", Locale: "en", Slug: "english", Title: "English Name"},
	}
	items := []techdata.Item{
		{ID: "T1", Labels: map[string]string{"ko": "한국어 이름"}, Aliases: map[string][]string{"ko": {"Shared"}}, WikiPages: map[string]document.WID{"ko": "W1"}},
		{ID: "T2", Labels: map[string]string{"en": "English Name"}, Aliases: map[string][]string{"en": {"Shared"}}, WikiPages: map[string]document.WID{"en": "W2"}},
	}
	index := BuildNameIndex(documents, items)
	if match := index.Match("ko", "Shared"); match.Candidate.CanonicalName != "한국어 이름" {
		t.Fatalf("Korean match crossed locale boundary: %#v", match)
	}
	if match := index.Match("en", "Shared"); match.Candidate.CanonicalName != "English Name" {
		t.Fatalf("English match crossed locale boundary: %#v", match)
	}
}

func TestDamerauLevenshteinCountsAdjacentTranspositionOnce(t *testing.T) {
	if got := damerauLevenshtein([]rune("langauge"), []rune("language")); got != 1 {
		t.Fatalf("distance = %d", got)
	}
}

func benchmarkNameIndexDocuments() []document.Document {
	documents := make([]document.Document, 10_000)
	for index := range documents {
		documents[index] = document.Document{
			ID:     document.WID(fmt.Sprintf("W%d", index+1)),
			Locale: "en",
			Slug:   fmt.Sprintf("technology-%05d", index),
			Title:  fmt.Sprintf("Technology %05d", index),
		}
	}
	return documents
}

func BenchmarkNameIndexBuild10000(b *testing.B) {
	documents := benchmarkNameIndexDocuments()
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		_ = BuildNameIndex(documents, nil)
	}
}

func BenchmarkNameIndexMatch10000(b *testing.B) {
	index := BuildNameIndex(benchmarkNameIndexDocuments(), nil)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		_ = index.Match("en", "Technlogy 09999")
	}
}

func BenchmarkNameIndexExactMatch10000(b *testing.B) {
	index := BuildNameIndex(benchmarkNameIndexDocuments(), nil)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		_ = index.Match("en", "Technology 09999")
	}
}
