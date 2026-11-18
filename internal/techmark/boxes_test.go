package techmark

import (
	"strings"
	"testing"
)

func TestBoxReferencesAreSafeAndCodeExamplesDoNotExpand(t *testing.T) {
	p := NewParser()
	for _, source := range []string{`<wiki-box name="A &amp; B" />`, `<wiki-box name="A &amp; B"></wiki-box>`} {
		got := p.RenderWithLocale(source, "ko")
		if strings.Count(got, `<wiki-box name="A &amp; B"></wiki-box>`) != 1 {
			t.Fatal(got)
		}
	}
	for _, source := range []string{"`<wiki-box name=\"X\" />`", "```techmark\n<wiki-box name=\"X\" />\n```"} {
		got := p.Render(source)
		if strings.Contains(got, "<wiki-box") || !strings.Contains(got, "&lt;wiki-box") {
			t.Fatal(got)
		}
	}
	got := p.Render(`<wiki-box name="&quot; onclick=&quot;alert(1)" onclick="evil()" />`)
	if strings.Contains(got, ` onclick="`) || strings.Contains(got, `<script`) {
		t.Fatal(got)
	}
}

func TestBoxFragmentValidationAndExcerpt(t *testing.T) {
	for _, source := range []string{"# heading", "!## Heading", "x\n<wiki-box name=\"nested\" />", "[^a]: note", "<wiki-footnote>note</wiki-footnote>"} {
		if ValidBoxSource(source) {
			t.Fatalf("accepted %s", source)
		}
	}
	for _, source := range []string{`<wiki-navbox title="Hi">` + "\n- Group: [[Article]]\n</wiki-navbox>", "`<wiki-box name=\"X\" />`", "```\n# sample\n<wiki-box name=\"X\" />\n```"} {
		if !ValidBoxSource(source) {
			t.Fatalf("rejected %s", source)
		}
	}
	if got := Excerpt("<wiki-box name=\"X\" />\n\nAn article description.", "Title", 100); got != "An article description." {
		t.Fatal(got)
	}
}
