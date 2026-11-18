package bot

import (
	"strings"
	"testing"

	"techpedia/internal/document"
	"techpedia/internal/techdata"
)

func TestScanDocumentRewritesOnlyCanonicalLinkTargets(t *testing.T) {
	documents := []document.Document{
		{ID: "W1", Locale: "ko", Slug: "c-language", Title: "C (프로그래밍 언어)"},
		{ID: "W2", Locale: "ko", Slug: "article", Title: "문서"},
	}
	items := []techdata.Item{{
		ID:        "T1",
		Labels:    map[string]string{"ko": "C (프로그래밍 언어)"},
		Aliases:   map[string][]string{"ko": {"C언어"}},
		WikiPages: map[string]document.WID{"ko": "W1"},
	}}
	doc := document.Document{
		ID: "W2", Locale: "ko", Slug: "article", Title: "문서", Revision: "r3",
		Source: "[[C]]와 [[C언어|C 언어]]를 참고한다. `[[C]]`\n\n```techmark\n[[C언어]]\n```\n",
	}

	suggestion, ok := ScanDocument(doc, BuildNameIndex(documents, items))
	if !ok {
		t.Fatal("expected a naming suggestion")
	}
	wantParts := []string{
		"[[C (프로그래밍 언어)|C]]",
		"[[C (프로그래밍 언어)|C 언어]]",
		"`[[C]]`",
		"```techmark\n[[C언어]]\n```",
	}
	for _, part := range wantParts {
		if !strings.Contains(suggestion.ProposedSource, part) {
			t.Fatalf("proposed source missing %q:\n%s", part, suggestion.ProposedSource)
		}
	}
	if suggestion.ProposedName != doc.Title || suggestion.ID == "" || len(suggestion.Findings) != 2 {
		t.Fatalf("incomplete suggestion: %#v", suggestion)
	}
	for _, finding := range suggestion.Findings {
		if finding.CheckerID != "name-index:v1" || finding.Area != "link" {
			t.Fatalf("unexpected finding: %#v", finding)
		}
	}
}

func TestScanDocumentDoesNotOverrideAnExistingExactTitle(t *testing.T) {
	documents := []document.Document{
		{ID: "W1", Locale: "en", Slug: "c-language", Title: "C (programming language)"},
		{ID: "W2", Locale: "en", Slug: "c-letter", Title: "C"},
		{ID: "W3", Locale: "en", Slug: "article", Title: "Article"},
	}
	doc := document.Document{ID: "W3", Locale: "en", Slug: "article", Title: "Article", Revision: "r1", Source: "[[C]]"}
	if _, ok := ScanDocument(doc, BuildNameIndex(documents, nil)); ok {
		t.Fatal("existing exact title was rewritten to a generated parenthetical candidate")
	}
}
