package techmark

import (
	"strings"
	"testing"
)

func TestGalleryRejectsExcessEntriesWithoutPartialImages(t *testing.T) {
	for _, opening := range []string{"", "<wiki-infobox>"} {
		source := opening + `<wiki-image file="` + strings.Repeat("a,", 64) + `a" alt="safe">caption</wiki-image>`
		if opening != "" {
			source += "</wiki-infobox>"
		}
		html := NewParser().Render(source)
		if strings.Contains(html, "<img") || !strings.Contains(html, "Invalid media filename") {
			t.Fatal("oversized gallery was rendered")
		}
	}
}

func TestImagesShareWholeDocumentBudgetAcrossBlocksAndInfoboxes(t *testing.T) {
	image := `<wiki-image file="a">caption</wiki-image>`
	for _, source := range []string{strings.Repeat(image, 1025), "<wiki-infobox>" + strings.Repeat(image, 1025) + "</wiki-infobox>"} {
		html := NewParser().Render(source)
		if strings.Contains(html, "<img") || !strings.Contains(html, "Image rendering limit exceeded") {
			t.Fatal("whole-document image budget was bypassed")
		}
	}
	control := NewParser().Render(strings.Repeat(image, 10))
	if strings.Count(control, "<img") != 10 {
		t.Fatal("ordinary image blocks were rejected")
	}
}

func BenchmarkUnmatchedWikiTags(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(strings.Repeat("x", count/100), func(b *testing.B) {
			source := strings.Repeat("<wiki-infobox>", count)
			p := NewParser()
			p.cache = nil
			b.ReportAllocs()
			for range b.N {
				p.Render(source)
			}
		})
	}
}

func TestClosingTagIndexRetainsFirstFollowingCaseInsensitiveSemantics(t *testing.T) {
	source := `<wiki-infobox><wiki-infobox>nested</wiki-INFOBOX></wiki-infobox>`
	index := indexWikiClosingTags(source)
	start, end := index.after(len("<wiki-infobox>"), "wiki-infobox")
	if source[start:end] != "</wiki-INFOBOX>" {
		t.Fatal("closing semantics changed")
	}
	if a, b := index.after(len(source), "wiki-image"); a != -1 || b != -1 {
		t.Fatal("missing close accepted")
	}
	for _, tag := range []string{"wiki-box", "wiki-infobox", "wiki-navbox", "wiki-image", "wiki-tree", "wiki-flow"} {
		NewParser().Render(strings.Repeat("<"+tag+">", 2000))
	}
}

func TestImageByteBudgetRejectsBeforeCopyingFragmentIntoDocument(t *testing.T) {
	budget := &imageRenderBudget{bytes: maxRenderedImageBytes - 1}
	output := renderWikiImageInContext(`<wiki-image file="a,b">`, "caption", "/en/wiki/", false, budget)
	if !budget.exceeded || strings.Contains(output, "<img") || !strings.Contains(output, "Image rendering limit exceeded") {
		t.Fatal("byte budget did not reject fragment")
	}
}
