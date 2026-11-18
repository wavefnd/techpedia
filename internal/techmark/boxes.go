package techmark

import (
	"html"
	"regexp"
	"strings"
)

var (
	boxForbiddenPattern = regexp.MustCompile(`(?im)</?wiki-(?:box|footnote)\b|^\s*!?#{1,6}\s|\[\^[^\]]+\]`)
	boxReferencePattern = regexp.MustCompile(`(?i)</?wiki-box\b`)
)

func ContainsBoxSyntax(source string) bool {
	return containsPatternOutsideCode(source, boxReferencePattern)
}

// Boxes are non-recursive fragments, not documents with their own heading or
// footnote namespace. Code examples are excluded using the parser's boundary.
func ValidBoxSource(source string) bool {
	return !containsPatternOutsideCode(source, boxForbiddenPattern)
}

func renderBoxReference(tag string) string {
	name := strings.TrimSpace(html.UnescapeString(wikiTagAttribute(tag, "name")))
	if name == "" || len(name) > 512 {
		return ""
	}
	return `<wiki-box name="` + html.EscapeString(name) + `"></wiki-box>`
}
