package techmark

import (
	"fmt"
	"strings"
	"testing"
)

func TestNavboxSixAndMoreLevelsKeepMergedLineage(t *testing.T) {
	source := `<wiki-navbox title="深い 분류">
- Root:
-- Branch:
--- Third:
---- Fourth:
----- Fifth:
------ Sixth {color="#87CF3E" note="6"}: [[FreeBSD]]
------- Seventh: [[OpenBSD]]
------ Sibling: [[NetBSD]]
-- Other: [[Debian]]
- Separate: [[Ubuntu]]
</wiki-navbox>`
	for _, locale := range []string{"ko", "en", "fr", "ar"} {
		t.Run(locale, func(t *testing.T) {
			output := NewParser().RenderWithLocale(source, locale)
			assertContains(t, output, `wiki-navbox-depth-3 wiki-navbox-deep`)
			assertContains(t, output, `style="--navbox-depth:7"`)
			assertContains(t, output, `<th class="wiki-navbox-descendant" scope="rowgroup" rowspan="2" style="--navbox-label-color:#87CF3E;--navbox-label-text:#111111"><span class="wiki-navbox-label">Sixth<small>6</small>`)
			assertContains(t, output, `<span class="wiki-navbox-label">Seventh</span>`)
			assertContains(t, output, `href="/`+locale+`/wiki/OpenBSD"`)
			assertContains(t, output, `<th class="wiki-navbox-parent" scope="rowgroup" rowspan="4"`)
			assertContains(t, output, `<th class="wiki-navbox-child" scope="rowgroup" rowspan="3"`)
			assertNotContains(t, output, "wiki-navbox-level")
			assertNotContains(t, output, "wiki-navbox-nested")
			if strings.Count(output, `<col `) != 7 || strings.Count(output, `<tr class="wiki-navbox-row`) != 5 {
				t.Fatalf("lineage was flattened or empty parent rows were added:\n%s", output)
			}
		})
	}
}

func TestNavboxMissingLevelsAndReturningSiblings(t *testing.T) {
	groups := groupNavboxRows(parseNavboxRows("- Root:\n------ A:\n------- B: b\n------ C: c\n-- D: d\n- Other: other"))
	if len(groups) != 2 || groups[0].Depth != 3 || len(groups[0].Children) != 3 {
		t.Fatalf("wrong tree: %+v", groups)
	}
	children := groups[0].Children
	if children[0].Child.Label != "A" || children[0].Child.Level != 2 || len(children[0].Grandchildren) != 1 || children[1].Child.Label != "C" || children[2].Child.Label != "D" {
		t.Fatal("skipped markers or shallower siblings were attached incorrectly")
	}
	orphan := groupNavboxRows(parseNavboxRows("------ Orphan: [[C]]"))
	if len(orphan) != 1 || orphan[0].Parent.Level != 1 {
		t.Fatal("orphan row was discarded")
	}
	for _, line := range []string{"----No space: x", "------", "--- : x", "plain: x"} {
		if _, ok := parseNavboxRow(line); ok {
			t.Fatal("accepted invalid row", line)
		}
	}
	if row, ok := parseNavboxRow("  ------\tTab: [[C]]"); !ok || row.Level != 6 {
		t.Fatal("tab/indent support")
	}
}

func TestNavboxDeepGroupsKeepOwnValuesAndSafeMetadata(t *testing.T) {
	output := NewParser().Render(`<wiki-navbox title="Safe">
- Root: [[C]]
-- Branch: [[Go]]
--- Third: [[Rust]]
---- &lt;script&gt; {color="red" note="&lt;unsafe&gt;" style="display:none"}: [[D]]
</wiki-navbox>`)
	for _, name := range []string{"C", "Go", "Rust", "D"} {
		assertContains(t, output, `href="/wiki/`+name+`"`)
	}
	assertContains(t, output, `rowspan="4"`)
	assertContains(t, output, `>&amp;lt;script&amp;gt;<small>&amp;lt;unsafe&amp;gt;</small>`)
	assertNotContains(t, output, "display:none")
	assertNotContains(t, output, "<script>")
	assertNotContains(t, output, "--navbox-label-color:red")
}

func TestNavboxDepthIsNotLimitedToSixAndDoesNotRecurse(t *testing.T) {
	var source strings.Builder
	source.WriteString(`<wiki-navbox title="Deep">` + "\n")
	for level := 1; level <= 128; level++ {
		fmt.Fprintf(&source, "%s Level %d: [[C]]\n", strings.Repeat("-", level), level)
	}
	source.WriteString(`</wiki-navbox>`)
	output := NewParser().Render(source.String())
	assertContains(t, output, `style="--navbox-depth:128"`)
	assertContains(t, output, `rowspan="128"`)
	assertNotContains(t, output, "wiki-navbox-level")
	if strings.Count(output, `<tr class="wiki-navbox-row`) != 128 || strings.Count(output, `href="/wiki/C"`) != 128 {
		t.Fatal("deep rows were lost")
	}
}
