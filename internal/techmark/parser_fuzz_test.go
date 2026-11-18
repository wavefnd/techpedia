package techmark

import "testing"

func FuzzParserPublicFunctions(f *testing.F) {
	for _, seed := range []string{
		"plain text",
		"`unclosed [[link]] <wiki-alert onclick=\"x\">",
		"```objective-c++\n#import <Foundation/Foundation.h>\n```",
		"<wiki-infobox title=\"C\">\n| kind = [[Language]]\n</wiki-infobox>",
		"text<wiki-footnote>\n[source](https://example.com)\n</wiki-footnote>",
		"| a | b |\n| --- | --- |\n| `x | y` | [[C|C language]] |",
		"<wiki-tree title=\"Tree\">\nRoot\n└── Child\n</wiki-tree>",
	} {
		f.Add(seed)
	}

	parser := NewParser()
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 1<<20 {
			t.Skip()
		}
		_ = parser.RenderWithLocale(source, "ko")
		_ = Excerpt(source, "Title", 160)
		_ = InternalLinkTargets(source)
	})
}
