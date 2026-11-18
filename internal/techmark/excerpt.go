package techmark

import (
	"html"
	"regexp"
	"strings"
)

var (
	excerptHeadingPattern      = regexp.MustCompile(`^!?#(?:#{0,5})(?:\s+|$)`)
	excerptListPattern         = regexp.MustCompile(`^(?:[-*]|\d+[.)])\s+`)
	excerptFootnotePattern     = regexp.MustCompile(`(?is)<wiki-footnote\s*>.*?</wiki-footnote\s*>`)
	excerptWikiLinkPattern     = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
	excerptMarkdownLinkPattern = regexp.MustCompile(`(?i)\[([^\]\n]+)\]\(https?://(?:[^\s()]|\([^\s()]*\))+\)`)
	excerptHTMLPattern         = regexp.MustCompile(`(?i)</?(?:br|code|em|li|ol|p|strong|table|tbody|td|th|thead|tr|ul|wiki-alert)(?:\s+[^<>]*)?>`)
	excerptSpacePattern        = regexp.MustCompile(`\s+`)
)

// Excerpt returns the first prose line in TechMark source. Structural markup,
// code examples, and a standalone line repeating the document title are skipped.
func Excerpt(source string, title string, maxRunes int) string {
	source = strings.ReplaceAll(source, "\r\n", "\n")
	source = strings.ReplaceAll(source, "\r", "\n")
	source = stripPatternOutsideCode(source, excerptFootnotePattern)
	title = normalizeExcerptComparison(cleanExcerptInline(title))
	inFence := false
	blockedWikiTag := ""

	for _, rawLine := range strings.Split(source, "\n") {
		line := strings.TrimSpace(rawLine)
		if codeFencePattern.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence || line == "" {
			continue
		}
		if blockedWikiTag != "" {
			if containsOutsideInlineCode(line, "</"+blockedWikiTag+">") {
				blockedWikiTag = ""
			}
			continue
		}
		if tag := excerptStructuralWikiTag(line); tag != "" {
			if !containsOutsideInlineCode(line, "</"+tag+">") && !(tag == "wiki-box" && containsOutsideInlineCode(line, "/>")) {
				blockedWikiTag = tag
			}
			continue
		}
		if excerptHeadingPattern.MatchString(line) || horizontalRulePattern.MatchString(line) || strings.HasPrefix(line, "|") || strings.HasPrefix(line, "[^") {
			continue
		}

		line = excerptListPattern.ReplaceAllString(line, "")
		line = cleanExcerptInline(line)
		if line == "" || normalizeExcerptComparison(line) == title {
			continue
		}
		return truncateExcerpt(line, maxRunes)
	}
	return ""
}

func excerptStructuralWikiTag(line string) string {
	for _, tag := range []string{"wiki-box", "wiki-infobox", "wiki-image", "wiki-navbox", "wiki-tree", "wiki-flow", "wiki-recent-changes", "wiki-random-page", "wiki-home-grid", "wiki-featured-pages", "wiki-popular-pages"} {
		if containsOutsideInlineCode(line, "<"+tag) {
			return tag
		}
	}
	return ""
}

func containsOutsideInlineCode(line string, needle string) bool {
	needle = strings.ToLower(needle)
	for _, segment := range nonCodeSegments(line) {
		if strings.Contains(strings.ToLower(segment), needle) {
			return true
		}
	}
	return false
}

func cleanExcerptInline(value string) string {
	value = excerptWikiLinkPattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := excerptWikiLinkPattern.FindStringSubmatch(match)
		if len(parts) > 2 && strings.TrimSpace(parts[2]) != "" {
			return parts[2]
		}
		if len(parts) > 1 {
			return parts[1]
		}
		return ""
	})
	value = excerptMarkdownLinkPattern.ReplaceAllString(value, "$1")
	value = excerptHTMLPattern.ReplaceAllString(value, "")
	value = strings.NewReplacer("**", "", "__", "", "`", "").Replace(value)
	value = html.UnescapeString(value)
	return strings.TrimSpace(excerptSpacePattern.ReplaceAllString(value, " "))
}

func normalizeExcerptComparison(value string) string {
	return strings.ToLower(strings.TrimSpace(excerptSpacePattern.ReplaceAllString(value, " ")))
}

func truncateExcerpt(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}
