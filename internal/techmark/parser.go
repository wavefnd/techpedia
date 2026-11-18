package techmark

import (
	"bytes"
	"fmt"
	"html"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"techpedia/internal/media"
)

type Parser struct {
	cache *renderCache
}

func NewParser() *Parser {
	return &Parser{cache: newRenderCache(256)}
}

// ClearCache drops rendered source kept in process memory. Privacy-sensitive
// history operations clear the whole bounded cache because entries are keyed by
// content hashes rather than document identity.
func (p *Parser) ClearCache() {
	if p.cache != nil {
		p.cache.clear()
	}
}

var (
	wikiTagPattern            = regexp.MustCompile(`(?s)</?wiki-[a-zA-Z0-9-]+(?:\s+[^<>]*)?>`)
	attrPattern               = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9_-]*)\s*=\s*"([^"]*)"`)
	boldPattern               = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	linkPattern               = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
	markdownLinkPattern       = regexp.MustCompile(`(?i)\[([^\]\n]+)\]\((https?://(?:[^\s()]|\([^\s()]*\))+)\)`)
	externalLinkTokenPattern  = regexp.MustCompile("\\x00techpedia-external-link:([0-9]+)\\x00")
	inlineFootnotePattern     = regexp.MustCompile(`(?is)<wiki-footnote\s*>(.*?)</wiki-footnote\s*>|\[\^([A-Za-z0-9_-]{1,64})\]`)
	footnoteDefinitionPattern = regexp.MustCompile(`^\[\^([A-Za-z0-9_-]{1,64})\]:\s*(.+)$`)
	codeFencePattern          = regexp.MustCompile("^```([a-zA-Z0-9_+#.-]*)(?:\\s+id=\"([a-zA-Z0-9][a-zA-Z0-9_.:-]{0,127})\")?\\s*$")
	legacyTOCPattern          = regexp.MustCompile(`(?i)</?wiki-toc(?:\s+[^<>]*)?>`)
	specialSyntaxPattern      = regexp.MustCompile(`(?i)</?wiki-(?:recent-changes|random-page|home-grid|featured-pages|popular-pages)(?:\s+[^<>]*)?>`)
	homeSyntaxPattern         = regexp.MustCompile(`(?i)</?wiki-(?:home-grid|featured-pages|popular-pages)(?:\s+[^<>]*)?>`)
	secondLevelSectionPattern = regexp.MustCompile(`(?s)<!--techmark-section:(open|closed)-->(<h2 id="section-[0-9]+">.*?</h2>)`)
	infoboxRowPattern         = regexp.MustCompile(`^\|\s*([^=|]+?)\s*=\s*(.*?)\s*$`)
	navboxColorPattern        = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	treeLinePattern           = regexp.MustCompile(`^((?:(?:│ {3})|(?: {4}))*)(?:├──|└──)[ \t]*(.+?)\s*$`)
	horizontalRulePattern     = regexp.MustCompile(`^-{3,}$`)
	unorderedListPattern      = regexp.MustCompile(`^[-*]\s+(.+)$`)
	orderedListPattern        = regexp.MustCompile(`^\d+[.)]\s+(.+)$`)
	htmlTagPattern            = regexp.MustCompile(`(?s)</?[a-zA-Z][a-zA-Z0-9-]*(?:\s+[^<>]*)?>`)
	allowedHTMLTags           = map[string]struct{}{
		"br":             {},
		"code":           {},
		"em":             {},
		"li":             {},
		"ol":             {},
		"p":              {},
		"strong":         {},
		"techpedia-time": {},
		"table":          {},
		"tbody":          {},
		"td":             {},
		"th":             {},
		"thead":          {},
		"tr":             {},
		"ul":             {},
	}
	allowedWikiTagAttributes = map[string]map[string]struct{}{
		"wiki-alert":          {"type": {}},
		"wiki-box":            {"name": {}},
		"wiki-featured-pages": {"empty": {}, "limit": {}, "title": {}},
		"wiki-flow":           {"accent": {}, "state": {}, "title": {}},
		"wiki-home-grid":      {},
		"wiki-infobox":        {"title": {}, "type": {}},
		"wiki-image":          {"alt": {}, "file": {}, "layout": {}, "src": {}},
		"wiki-navbox":         {"accent": {}, "state": {}, "title": {}},
		"wiki-popular-pages":  {"empty": {}, "limit": {}, "period": {}, "title": {}},
		"wiki-quote":          {"source": {}},
		"wiki-random-page":    {"label": {}},
		"wiki-recent-changes": {"empty": {}, "limit": {}},
		"wiki-tree":           {"accent": {}, "state": {}, "title": {}},
	}
	codeHighlightStyle = func() *chroma.Style {
		style := styles.Get("github")
		if style == nil {
			return styles.Fallback
		}
		return style
	}()
	codeHighlightFormatter = chromahtml.New(chromahtml.PreventSurroundingPre(true), chromahtml.WithClasses(true), chromahtml.ClassPrefix("syntax-"))
)

func ContainsSpecialSyntax(source string) bool {
	return containsPatternOutsideCode(source, specialSyntaxPattern)
}

func StripSpecialSyntax(source string) string {
	return stripPatternOutsideCode(source, specialSyntaxPattern)
}

func ContainsHomePageSyntax(source string) bool {
	return containsPatternOutsideCode(source, homeSyntaxPattern)
}

func StripHomePageSyntax(source string) string {
	return stripPatternOutsideCode(source, homeSyntaxPattern)
}

func (p *Parser) Render(source string) string {
	return p.renderCached(source, "/wiki/")
}

func (p *Parser) RenderWithLocale(source string, locale string) string {
	return p.renderCached(source, localizedWikiPrefix(locale))
}

func (p *Parser) render(source string, linkPrefix string) string {
	source = strings.ReplaceAll(source, "\r\n", "\n")
	source = strings.ReplaceAll(source, "\r", "\n")
	source = legacyTOCPattern.ReplaceAllString(source, "")
	source, notes := prepareFootnotes(source)
	source, protectedCodeTags := protectWikiTagsInCode(source)
	closings := indexWikiClosingTags(source)
	images := &imageRenderBudget{}

	var out bytes.Buffer
	position := 0

	for position < len(source) {
		if images.exceeded {
			return imageLimitHTML
		}
		match := wikiTagPattern.FindStringIndex(source[position:])
		if match == nil {
			out.WriteString(renderTextSegment(source[position:], linkPrefix))
			break
		}

		start := position + match[0]
		end := position + match[1]
		tag := source[start:end]

		out.WriteString(renderTextSegment(source[position:start], linkPrefix))

		name, closing := wikiTagName(tag)
		if name == "wiki-box" && !closing {
			if strings.HasSuffix(strings.TrimSpace(tag), "/>") {
				out.WriteString(renderBoxReference(tag))
				position = end
				continue
			}
			closeStart, closeEnd := closings.after(end, name)
			if closeStart >= 0 && strings.TrimSpace(source[end:closeStart]) == "" {
				out.WriteString(renderBoxReference(tag))
				position = closeEnd
				continue
			}
			position = end
			continue
		}
		if name == "wiki-infobox" && !closing {
			closeStart, closeEnd := closings.after(end, name)
			if closeStart >= 0 {
				out.WriteString(sanitizeWikiTag(tag))
				out.WriteString(renderInfoboxContent(source[end:closeStart], linkPrefix, images))
				out.WriteString("</wiki-infobox>")
				position = closeEnd
				continue
			}
		}
		if name == "wiki-navbox" && !closing {
			closeStart, closeEnd := closings.after(end, name)
			if closeStart >= 0 {
				out.WriteString(renderNavbox(tag, source[end:closeStart], linkPrefix))
				position = closeEnd
				continue
			}
		}
		if name == "wiki-image" && !closing {
			closeStart, closeEnd := closings.after(end, name)
			if closeStart >= 0 {
				out.WriteString(renderWikiImageInContext(tag, source[end:closeStart], linkPrefix, false, images))
				position = closeEnd
				continue
			}
		}
		if (name == "wiki-tree" || name == "wiki-flow") && !closing {
			closeStart, closeEnd := closings.after(end, name)
			if closeStart >= 0 {
				if name == "wiki-tree" {
					out.WriteString(renderWikiTree(tag, source[end:closeStart], linkPrefix))
				} else {
					out.WriteString(renderWikiFlow(tag, source[end:closeStart], linkPrefix))
				}
				position = closeEnd
				continue
			}
		}

		out.WriteString(sanitizeWikiTag(tag))
		position = end
	}

	if images.exceeded {
		return imageLimitHTML
	}
	rendered := replaceFootnoteReferences(out.String(), notes)
	rendered = restoreProtectedWikiTags(rendered, protectedCodeTags)
	rendered = addHeadingIDs(rendered)
	rendered = wrapSecondLevelSections(rendered)
	return rendered + renderFootnotes(notes, linkPrefix)
}

func renderWikiImage(openingTag string, caption string, linkPrefix string) string {
	return renderWikiImageInContext(openingTag, caption, linkPrefix, false, &imageRenderBudget{})
}

const MaxGalleryImages = 64
const MaxDocumentImages = 1024
const maxRenderedImageBytes = 8 << 20
const imageLimitHTML = `<p class="wiki-image-error">Image rendering limit exceeded</p>`

type imageRenderBudget struct {
	count, bytes int
	exceeded     bool
}

func renderWikiImageInContext(openingTag string, caption string, linkPrefix string, infobox bool, budget *imageRenderBudget) string {
	fileName := strings.TrimSpace(wikiTagAttribute(openingTag, "file"))
	sourceURL := strings.TrimSpace(wikiTagAttribute(openingTag, "src"))
	if (fileName == "") == (sourceURL == "") {
		return `<span class="wiki-image-error">Invalid image source</span>`
	}
	files := []string{""}
	if fileName != "" {
		var err error
		files, err = wikiImageFileNames(fileName)
		if err != nil {
			return `<span class="wiki-image-error">Invalid media filename</span>`
		}
	} else {
		parsed, err := url.Parse(sourceURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			return `<span class="wiki-image-error">Invalid external image URL</span>`
		}
		sourceURL = parsed.String()
	}
	budget.count += len(files)
	if budget.count > MaxDocumentImages {
		budget.exceeded = true
		return imageLimitHTML
	}
	layout := strings.ToLower(strings.TrimSpace(wikiTagAttribute(openingTag, "layout")))
	switch layout {
	case "side", "wide":
	default:
		layout = "frame"
	}
	if infobox {
		layout = "infobox"
	}
	alt := strings.TrimSpace(wikiTagAttribute(openingTag, "alt"))
	if len([]rune(alt)) > 500 {
		alt = string([]rune(alt)[:500])
	}

	var out bytes.Buffer
	out.WriteString(`<wiki-image class="wiki-image wiki-image-`)
	out.WriteString(layout)
	out.WriteString(`"><figure>`)
	if len(files) > 1 {
		out.WriteString(`<div class="wiki-image-gallery" dir="ltr">`)
	}
	for _, normalized := range files {
		out.WriteString(`<button type="button" class="wiki-image-stage wiki-image-open" aria-haspopup="dialog"><img alt="`)
		out.WriteString(html.EscapeString(alt))
		out.WriteString(`" loading="lazy" decoding="async"`)
		if fileName != "" {
			base := "/media/files/" + url.PathEscape(normalized) + "/"
			out.WriteString(` data-large-src="` + html.EscapeString(base+"large.webp") + `"`)
			out.WriteString(` src="` + html.EscapeString(base+"article.webp") + `"`)
			out.WriteString(` srcset="` + html.EscapeString(base+"thumb.webp") + ` 320w, ` + html.EscapeString(base+"article.webp") + ` 960w, ` + html.EscapeString(base+"large.webp") + ` 1920w"`)
			if infobox {
				out.WriteString(` sizes="(max-width: 720px) calc(100vw - 56px), 274px"`)
			} else if layout == "side" {
				out.WriteString(` sizes="(max-width: 720px) 100vw, 440px"`)
			} else {
				out.WriteString(` sizes="(max-width: 720px) 100vw, 960px"`)
			}
		} else {
			out.WriteString(` data-large-src="` + html.EscapeString(sourceURL) + `"`)
			out.WriteString(` src="` + html.EscapeString(sourceURL) + `" referrerpolicy="no-referrer"`)
		}
		out.WriteString(`></button>`)
	}
	if len(files) > 1 {
		out.WriteString(`</div>`)
		out.WriteString(`<div class="wiki-image-controls" dir="ltr"><button type="button" class="wiki-image-previous" aria-label="Previous image">‹</button><output class="wiki-image-position" aria-live="polite" aria-atomic="true">1 / ` + strconv.Itoa(len(files)) + `</output><button type="button" class="wiki-image-next" aria-label="Next image">›</button></div>`)
	}
	caption = strings.TrimSpace(caption)
	if caption != "" || sourceURL != "" {
		out.WriteString(`<figcaption>`)
		if caption != "" {
			out.WriteString(`<span class="wiki-image-caption">`)
			out.WriteString(renderInlineText(caption, linkPrefix))
			out.WriteString(`</span>`)
		}
		if sourceURL != "" {
			parsed, _ := url.Parse(sourceURL)
			out.WriteString(`<small class="wiki-image-source">from <a href="`)
			out.WriteString(html.EscapeString(parsed.String()))
			out.WriteString(`" target="_blank" rel="nofollow noopener noreferrer" referrerpolicy="no-referrer">`)
			out.WriteString(html.EscapeString(parsed.Hostname()))
			out.WriteString(`</a></small>`)
		}
		out.WriteString(`</figcaption>`)
	}
	out.WriteString(`</figure></wiki-image>` + "\n")
	budget.bytes += out.Len()
	if budget.bytes > maxRenderedImageBytes {
		budget.exceeded = true
		return imageLimitHTML
	}
	return out.String()
}

func wikiImageFileNames(value string) ([]string, error) {
	var files []string
	var name strings.Builder
	appendName := func() error {
		if len(files) >= MaxGalleryImages {
			return fmt.Errorf("too many gallery images")
		}
		normalized, err := media.NormalizeName(name.String())
		if err != nil {
			return err
		}
		files = append(files, normalized)
		name.Reset()
		return nil
	}
	for index := 0; index < len(value); index++ {
		switch value[index] {
		case '\\':
			if index+1 < len(value) && value[index+1] == ',' {
				name.WriteByte(',')
				index++
			} else {
				name.WriteByte(value[index])
			}
		case ',':
			if err := appendName(); err != nil {
				return nil, err
			}
		default:
			name.WriteByte(value[index])
		}
	}
	if err := appendName(); err != nil {
		return nil, err
	}
	return files, nil
}

type footnoteState struct {
	Definitions map[string]string
	Order       []string
	Numbers     map[string]int
	References  map[string]int
	InlineCount int
}

func prepareFootnotes(source string) (string, footnoteState) {
	state := footnoteState{
		Definitions: map[string]string{},
		Numbers:     map[string]int{},
		References:  map[string]int{},
	}
	lines := strings.Split(source, "\n")
	inFence := false
	for index, line := range lines {
		if codeFencePattern.MatchString(strings.TrimSpace(line)) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if match := footnoteDefinitionPattern.FindStringSubmatch(strings.TrimSpace(line)); len(match) == 3 {
			state.Definitions[match[1]] = strings.TrimSpace(match[2])
			lines[index] = ""
		}
	}

	return replaceFootnoteSyntax(strings.Join(lines, "\n"), &state), state
}

func registerFootnoteReference(state *footnoteState, key string) string {
	if state.Numbers[key] == 0 {
		state.Order = append(state.Order, key)
		state.Numbers[key] = len(state.Order)
	}
	state.References[key]++
	return "\x00techpedia-footnote-ref:" + key + ":" + strconv.Itoa(state.References[key]) + "\x00"
}

func replaceFootnoteSyntax(source string, state *footnoteState) string {
	return transformOutsideFencedCode(source, func(prose string) string {
		return replaceFootnotesInProse(prose, state)
	})
}

func transformOutsideFencedCode(source string, transform func(string) string) string {
	var out strings.Builder
	var prose strings.Builder
	inFence := false
	flushProse := func() {
		out.WriteString(transform(prose.String()))
		prose.Reset()
	}

	for _, line := range strings.SplitAfter(source, "\n") {
		lineWithoutNewline := strings.TrimSuffix(line, "\n")
		if codeFencePattern.MatchString(strings.TrimSpace(lineWithoutNewline)) {
			if !inFence {
				flushProse()
			}
			out.WriteString(line)
			inFence = !inFence
			continue
		}
		if inFence {
			out.WriteString(line)
			continue
		}
		prose.WriteString(line)
	}
	flushProse()
	return out.String()
}

func containsPatternOutsideCode(source string, pattern *regexp.Regexp) bool {
	if !pattern.MatchString(source) {
		return false
	}
	found := false
	transformOutsideFencedCode(source, func(prose string) string {
		if !found && patternMatchesOutsideInlineCode(prose, pattern) {
			found = true
		}
		return prose
	})
	return found
}

func stripPatternOutsideCode(source string, pattern *regexp.Regexp) string {
	if !pattern.MatchString(source) {
		return source
	}
	return transformOutsideFencedCode(source, func(prose string) string {
		return replacePatternOutsideInlineCode(prose, pattern, "")
	})
}

func patternMatchesOutsideInlineCode(text string, pattern *regexp.Regexp) bool {
	codeRanges := inlineCodeRangesInText(text)
	for _, match := range pattern.FindAllStringIndex(text, -1) {
		if !offsetInRanges(match[0], codeRanges) {
			return true
		}
	}
	return false
}

func replacePatternOutsideInlineCode(text string, pattern *regexp.Regexp, replacement string) string {
	matches := pattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}
	codeRanges := inlineCodeRangesInText(text)
	var out strings.Builder
	position := 0
	for _, match := range matches {
		if offsetInRanges(match[0], codeRanges) {
			continue
		}
		out.WriteString(text[position:match[0]])
		out.WriteString(replacement)
		position = match[1]
	}
	out.WriteString(text[position:])
	return out.String()
}

func replaceFootnotesInProse(prose string, state *footnoteState) string {
	matches := inlineFootnotePattern.FindAllStringSubmatchIndex(prose, -1)
	if len(matches) == 0 {
		return prose
	}
	codeRanges := inlineCodeRangesInText(prose)
	var out strings.Builder
	position := 0
	for _, match := range matches {
		start, end := match[0], match[1]
		if offsetInRanges(start, codeRanges) {
			continue
		}
		out.WriteString(prose[position:start])
		replacement := prose[start:end]
		if match[2] >= 0 {
			content := strings.TrimSpace(prose[match[2]:match[3]])
			if content != "" {
				state.InlineCount++
				key := "inline-" + strconv.Itoa(state.InlineCount)
				state.Definitions[key] = content
				replacement = registerFootnoteReference(state, key)
			}
		} else if match[4] >= 0 {
			key := prose[match[4]:match[5]]
			if _, exists := state.Definitions[key]; exists {
				replacement = registerFootnoteReference(state, key)
			}
		}
		out.WriteString(replacement)
		position = end
	}
	out.WriteString(prose[position:])
	return out.String()
}

func inlineCodeRangesInText(text string) [][2]int {
	ranges := make([][2]int, 0)
	offset := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		for _, current := range inlineCodeRanges(line) {
			ranges = append(ranges, [2]int{offset + current[0], offset + current[1]})
		}
		offset += len(line)
	}
	return ranges
}

func inlineCodeRanges(line string) [][2]int {
	ranges := make([][2]int, 0)
	position := 0
	for position < len(line) {
		start := strings.IndexByte(line[position:], '`')
		if start < 0 {
			break
		}
		start += position
		end := strings.IndexByte(line[start+1:], '`')
		if end < 0 {
			ranges = append(ranges, [2]int{start, len(line)})
			break
		}
		end += start + 2
		ranges = append(ranges, [2]int{start, end})
		position = end
	}
	return ranges
}

func offsetInRanges(offset int, ranges [][2]int) bool {
	for _, current := range ranges {
		if offset >= current[0] && offset < current[1] {
			return true
		}
	}
	return false
}

func protectWikiTagsInCode(source string) (string, map[string]string) {
	protected := make(map[string]string)
	lines := strings.Split(source, "\n")
	inFence := false
	sequence := 0
	for index, line := range lines {
		if codeFencePattern.MatchString(strings.TrimSpace(line)) {
			inFence = !inFence
			continue
		}
		codeRanges := inlineCodeRanges(line)
		matches := wikiTagPattern.FindAllStringIndex(line, -1)
		if len(matches) == 0 {
			continue
		}
		var out strings.Builder
		position := 0
		for _, match := range matches {
			if !inFence && !offsetInRanges(match[0], codeRanges) {
				continue
			}
			out.WriteString(line[position:match[0]])
			sequence++
			token := "\x00techpedia-code-wiki-tag-" + strconv.Itoa(sequence) + "\x00"
			protected[token] = html.EscapeString(line[match[0]:match[1]])
			out.WriteString(token)
			position = match[1]
		}
		if position > 0 {
			out.WriteString(line[position:])
			lines[index] = out.String()
		}
	}
	return strings.Join(lines, "\n"), protected
}

func restoreProtectedWikiTags(rendered string, protected map[string]string) string {
	for token, value := range protected {
		rendered = strings.ReplaceAll(rendered, token, value)
	}
	return rendered
}

func replaceFootnoteReferences(rendered string, state footnoteState) string {
	pattern := regexp.MustCompile("\\x00techpedia-footnote-ref:([A-Za-z0-9_-]+):([0-9]+)\\x00")
	return pattern.ReplaceAllStringFunc(rendered, func(match string) string {
		parts := pattern.FindStringSubmatch(match)
		key := parts[1]
		number := state.Numbers[key]
		return `<sup class="techmark-footnote-ref" id="footnote-ref-` + key + `-` + parts[2] + `"><a href="#footnote-` + key + `" role="doc-noteref">[` + strconv.Itoa(number) + `]</a></sup>`
	})
}

func renderFootnotes(state footnoteState, linkPrefix string) string {
	if len(state.Order) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString(`<section class="techmark-footnotes" role="doc-endnotes"><ol>`)
	for _, key := range state.Order {
		out.WriteString(`<li id="footnote-` + key + `">`)
		out.WriteString(renderInlineText(state.Definitions[key], linkPrefix))
		for ordinal := 1; ordinal <= state.References[key]; ordinal++ {
			out.WriteString(` <a class="techmark-footnote-back" href="#footnote-ref-` + key + `-` + strconv.Itoa(ordinal) + `" aria-label="[` + strconv.Itoa(state.Numbers[key]) + `]">↩</a>`)
		}
		out.WriteString(`</li>`)
	}
	out.WriteString(`</ol></section>`)
	return out.String()
}

func addHeadingIDs(rendered string) string {
	index := 0
	for _, level := range []string{"h2", "h3", "h4"} {
		pattern := regexp.MustCompile(`(?i)<` + level + `>`)
		rendered = pattern.ReplaceAllStringFunc(rendered, func(tag string) string {
			index++
			return `<` + level + ` id="section-` + strconv.Itoa(index) + `">`
		})
	}
	return renumberHeadingIDs(rendered)
}

func renumberHeadingIDs(rendered string) string {
	index := 0
	pattern := regexp.MustCompile(`(?i)<h([2-4]) id="section-[0-9]+">`)
	return pattern.ReplaceAllStringFunc(rendered, func(tag string) string {
		index++
		level := tag[2]
		return `<h` + string(level) + ` id="section-` + strconv.Itoa(index) + `">`
	})
}

func wrapSecondLevelSections(rendered string) string {
	matches := secondLevelSectionPattern.FindAllStringSubmatchIndex(rendered, -1)
	if len(matches) == 0 {
		return rendered
	}

	var out strings.Builder
	position := 0
	for index, match := range matches {
		out.WriteString(rendered[position:match[0]])
		state := rendered[match[2]:match[3]]
		heading := rendered[match[4]:match[5]]
		contentEnd := len(rendered)
		if index+1 < len(matches) {
			contentEnd = matches[index+1][0]
		}
		out.WriteString(`<div class="techmark-section-viewport"><details class="techmark-section"`)
		if state == "open" {
			out.WriteString(` open`)
		}
		out.WriteString(`><summary>`)
		out.WriteString(heading)
		out.WriteString(`</summary><div class="techmark-section-content">`)
		out.WriteString(rendered[match[1]:contentEnd])
		out.WriteString(`</div></details></div>`)
		position = contentEnd
	}
	return out.String()
}

func localizedWikiPrefix(locale string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		locale = "ko"
	}
	return "/" + locale + "/wiki/"
}

func renderInfoboxContent(segment string, linkPrefix string, images *imageRenderBudget) string {
	var out bytes.Buffer
	closings := indexWikiClosingTags(segment)
	position, cursor := 0, 0
	for cursor < len(segment) {
		if images.exceeded {
			return imageLimitHTML
		}
		match := wikiTagPattern.FindStringIndex(segment[cursor:])
		if match == nil {
			break
		}
		start, end := cursor+match[0], cursor+match[1]
		cursor = end
		name, closing := wikiTagName(segment[start:end])
		if name != "wiki-image" || closing {
			continue
		}
		closeStart, closeEnd := closings.after(end, name)
		if closeStart < 0 {
			break
		}
		out.WriteString(renderInfoboxTextContent(segment[position:start], linkPrefix))
		out.WriteString(renderWikiImageInContext(segment[start:end], segment[end:closeStart], linkPrefix, true, images))
		position, cursor = closeEnd, closeEnd
	}
	out.WriteString(renderInfoboxTextContent(segment[position:], linkPrefix))
	return out.String()
}

func renderInfoboxTextContent(segment string, linkPrefix string) string {
	lines := strings.Split(segment, "\n")
	var out bytes.Buffer
	var pendingText []string
	var pendingRows []infoboxRow

	flushText := func() {
		if len(pendingText) == 0 {
			return
		}
		out.WriteString(renderTextSegment(strings.Join(pendingText, "\n"), linkPrefix))
		pendingText = nil
	}

	flushRows := func() {
		if len(pendingRows) == 0 {
			return
		}
		out.WriteString(`<table class="wiki-infobox-rows">` + "\n<tbody>\n")
		for _, row := range pendingRows {
			out.WriteString(`<tr><th scope="row">`)
			out.WriteString(renderInlineText(row.Label, linkPrefix))
			out.WriteString(`</th><td>`)
			out.WriteString(renderInlineText(row.Value, linkPrefix))
			out.WriteString("</td></tr>\n")
		}
		out.WriteString("</tbody>\n</table>\n")
		pendingRows = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimRight(line, " \t"))
		if matches := infoboxRowPattern.FindStringSubmatch(trimmed); len(matches) == 3 {
			flushText()
			pendingRows = append(pendingRows, infoboxRow{
				Label: strings.TrimSpace(matches[1]),
				Value: strings.TrimSpace(matches[2]),
			})
			continue
		}

		flushRows()
		pendingText = append(pendingText, line)
	}

	flushText()
	flushRows()
	return out.String()
}

type infoboxRow struct {
	Label string
	Value string
}

type navboxRow struct {
	Level int
	Label string
	Value string
	Color string
	Note  string
}

type navboxGroup struct {
	Parent   navboxRow
	Children []navboxBranch
	Depth    int
	Nodes    []*navboxNode
}

type navboxNode struct {
	Row         navboxRow
	SourceLevel int
	Children    []*navboxNode
}

type navboxBranch struct {
	Child         navboxRow
	Grandchildren []navboxRow
}

func renderNavbox(openingTag string, segment string, linkPrefix string) string {
	title := wikiTagAttribute(openingTag, "title")
	state := strings.ToLower(strings.TrimSpace(wikiTagAttribute(openingTag, "state")))
	accent := normalizeNavboxColor(wikiTagAttribute(openingTag, "accent"))
	groups := groupNavboxRows(parseNavboxRows(segment))

	var out bytes.Buffer
	out.WriteString(`<wiki-navbox class="wiki-navbox"`)
	if accent != "" {
		out.WriteString(` style="--navbox-accent:`)
		out.WriteString(accent)
		out.WriteString(`;--navbox-accent-text:`)
		out.WriteString(navboxTextColor(accent))
		out.WriteByte('"')
	}
	out.WriteString(`><details class="wiki-navbox-details"`)
	if state != "collapsed" {
		out.WriteString(` open`)
	}
	out.WriteString(`><summary class="wiki-navbox-title"><span>`)
	out.WriteString(html.EscapeString(title))
	out.WriteString(`</span></summary><div class="wiki-navbox-scroll"><table class="wiki-navbox-table`)
	depth := navboxDepth(groups)
	if depth >= 3 {
		out.WriteString(` wiki-navbox-depth-3`)
	}
	if depth > 3 {
		out.WriteString(` wiki-navbox-deep`)
	}
	if depth > 3 {
		out.WriteString(`" style="--navbox-depth:` + strconv.Itoa(depth))
	}
	out.WriteString(`"><colgroup><col class="wiki-navbox-parent-column">`)
	if depth > 3 {
		for level := 2; level <= depth; level++ {
			out.WriteString(`<col class="wiki-navbox-descendant-column">`)
		}
	} else {
		out.WriteString(`<col class="wiki-navbox-child-column"><col class="wiki-navbox-grandchild-column">`)
	}
	out.WriteString(`<col></colgroup><tbody>` + "\n")
	for _, group := range groups {
		if depth > 3 {
			renderDeepNavboxGroup(&out, group, depth, linkPrefix)
		} else {
			renderNavboxGroup(&out, group, linkPrefix)
		}
	}
	out.WriteString(`</tbody></table></div></details></wiki-navbox>` + "\n")
	return out.String()
}

func parseNavboxRows(segment string) []navboxRow {
	rows := make([]navboxRow, 0)
	for _, line := range strings.Split(segment, "\n") {
		if row, ok := parseNavboxRow(line); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

func parseNavboxRow(line string) (navboxRow, bool) {
	trimmed := strings.TrimSpace(line)
	row := navboxRow{}
	for row.Level < len(trimmed) && trimmed[row.Level] == '-' {
		row.Level++
	}
	if row.Level == 0 || row.Level == len(trimmed) || (trimmed[row.Level] != ' ' && trimmed[row.Level] != '\t') {
		return navboxRow{}, false
	}
	trimmed = strings.TrimSpace(trimmed[row.Level:])

	delimiter := navboxDelimiter(trimmed)
	if delimiter < 0 {
		return navboxRow{}, false
	}
	label, color, note := parseNavboxLabel(strings.TrimSpace(trimmed[:delimiter]))
	if label == "" {
		return navboxRow{}, false
	}
	row.Label = label
	row.Value = strings.TrimSpace(trimmed[delimiter+1:])
	row.Color = color
	row.Note = note
	return row, true
}

func navboxDelimiter(value string) int {
	inCode := false
	inQuote := false
	wikiDepth := 0
	metadataDepth := 0
	for index := 0; index < len(value); index++ {
		char := value[index]
		if char == '`' && metadataDepth == 0 {
			inCode = !inCode
			continue
		}
		if inCode {
			continue
		}
		if index+1 < len(value) {
			switch value[index : index+2] {
			case "[[":
				wikiDepth++
				index++
				continue
			case "]]":
				if wikiDepth > 0 {
					wikiDepth--
				}
				index++
				continue
			}
		}
		if wikiDepth > 0 {
			continue
		}
		if char == '"' && metadataDepth > 0 {
			inQuote = !inQuote
			continue
		}
		if !inQuote {
			switch char {
			case '{':
				metadataDepth++
				continue
			case '}':
				if metadataDepth > 0 {
					metadataDepth--
				}
				continue
			case ':':
				if metadataDepth == 0 {
					return index
				}
			}
		}
	}
	return -1
}

func parseNavboxLabel(value string) (string, string, string) {
	label := strings.TrimSpace(value)
	color := ""
	note := ""
	if !strings.HasSuffix(label, "}") {
		return label, color, note
	}
	metadataStart := strings.LastIndex(label, "{")
	if metadataStart < 0 {
		return label, color, note
	}
	metadata := label[metadataStart+1 : len(label)-1]
	label = strings.TrimSpace(label[:metadataStart])
	for _, match := range attrPattern.FindAllStringSubmatch(metadata, -1) {
		switch strings.ToLower(match[1]) {
		case "color":
			color = normalizeNavboxColor(match[2])
		case "note":
			note = strings.TrimSpace(match[2])
			if len([]rune(note)) > 64 {
				note = string([]rune(note)[:64])
			}
		}
	}
	return label, color, note
}

func groupNavboxRows(rows []navboxRow) []navboxGroup {
	groups := make([]navboxGroup, 0)
	stack := make([]*navboxNode, 0)
	for _, row := range rows {
		// Missing intermediate markers attach to the nearest earlier shallower
		// row. Keep source depth separate so later siblings remain siblings.
		sourceLevel := row.Level
		for len(stack) > 0 && stack[len(stack)-1].SourceLevel >= sourceLevel {
			stack = stack[:len(stack)-1]
		}
		row.Level = len(stack) + 1
		node := &navboxNode{Row: row, SourceLevel: sourceLevel}
		if len(stack) == 0 {
			groups = append(groups, navboxGroup{Parent: row, Depth: 1})
		} else {
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, node)
		}
		group := &groups[len(groups)-1]
		group.Nodes = append(group.Nodes, node)
		group.Depth = max(group.Depth, row.Level)
		stack = append(stack, node)
	}
	for i := range groups {
		group := &groups[i]
		for _, child := range group.Nodes[0].Children {
			branch := navboxBranch{Child: child.Row}
			for _, grandchild := range child.Children {
				branch.Grandchildren = append(branch.Grandchildren, grandchild.Row)
			}
			group.Children = append(group.Children, branch)
		}
	}
	return groups
}

// Count descendant rows bottom-up, then emit merged lineage cells in preorder.
// Both passes are iterative so author-controlled depth never uses recursion.
func renderDeepNavboxGroup(out *bytes.Buffer, group navboxGroup, depth int, linkPrefix string) {
	rows := make(map[*navboxNode]int, len(group.Nodes))
	for i := len(group.Nodes) - 1; i >= 0; i-- {
		node := group.Nodes[i]
		if node.Row.Value != "" || len(node.Children) == 0 {
			rows[node] = 1
		}
		for _, child := range node.Children {
			rows[node] += rows[child]
		}
	}
	rendered := make(map[*navboxNode]bool, len(group.Nodes))
	path := make([]*navboxNode, 0, group.Depth)
	for _, node := range group.Nodes {
		path = append(path[:node.Row.Level-1], node)
		if node.Row.Value == "" && len(node.Children) > 0 {
			continue
		}
		out.WriteString(`<tr class="wiki-navbox-row">`)
		for _, ancestor := range path {
			if rendered[ancestor] {
				continue
			}
			className := "wiki-navbox-descendant"
			switch ancestor.Row.Level {
			case 1:
				className = "wiki-navbox-parent"
			case 2:
				className = "wiki-navbox-child"
			case 3:
				className = "wiki-navbox-grandchild"
			}
			renderNavboxLabelCell(out, ancestor.Row, className, rows[ancestor], rows[ancestor] > 1)
			rendered[ancestor] = true
		}
		out.WriteString(`<td class="wiki-navbox-content" colspan="` + strconv.Itoa(depth-node.Row.Level+1) + `">`)
		out.WriteString(renderInlineText(node.Row.Value, linkPrefix))
		out.WriteString("</td></tr>\n")
	}
}

func renderNavboxGroup(out *bytes.Buffer, group navboxGroup, linkPrefix string) {
	directRow := group.Parent.Value != "" || len(group.Children) == 0
	rowspan := 0
	if directRow {
		rowspan = 1
	}
	for _, branch := range group.Children {
		rowspan += navboxBranchRows(branch)
	}
	parentRendered := false

	if directRow {
		out.WriteString(`<tr class="wiki-navbox-row wiki-navbox-parent-row">`)
		renderNavboxLabelCell(out, group.Parent, "wiki-navbox-parent", rowspan, rowspan > 1)
		parentRendered = true
		out.WriteString(`<td class="wiki-navbox-content" colspan="3">`)
		out.WriteString(renderInlineText(group.Parent.Value, linkPrefix))
		out.WriteString("</td></tr>\n")
	}

	for _, branch := range group.Children {
		childRows := navboxBranchRows(branch)
		childDirectRow := branch.Child.Value != "" || len(branch.Grandchildren) == 0
		childRendered := false
		if childDirectRow {
			out.WriteString(`<tr class="wiki-navbox-row wiki-navbox-child-row">`)
			if !parentRendered {
				renderNavboxLabelCell(out, group.Parent, "wiki-navbox-parent", rowspan, rowspan > 1)
				parentRendered = true
			}
			renderNavboxLabelCell(out, branch.Child, "wiki-navbox-child", childRows, childRows > 1)
			childRendered = true
			out.WriteString(`<td class="wiki-navbox-content" colspan="2">`)
			out.WriteString(renderInlineText(branch.Child.Value, linkPrefix))
			out.WriteString("</td></tr>\n")
		}
		for _, grandchild := range branch.Grandchildren {
			out.WriteString(`<tr class="wiki-navbox-row wiki-navbox-grandchild-row">`)
			if !parentRendered {
				renderNavboxLabelCell(out, group.Parent, "wiki-navbox-parent", rowspan, rowspan > 1)
				parentRendered = true
			}
			if !childRendered {
				renderNavboxLabelCell(out, branch.Child, "wiki-navbox-child", childRows, childRows > 1)
				childRendered = true
			}
			renderNavboxLabelCell(out, grandchild, "wiki-navbox-grandchild", 1, false)
			out.WriteString(`<td class="wiki-navbox-content">`)
			out.WriteString(renderInlineText(grandchild.Value, linkPrefix))
			out.WriteString("</td></tr>\n")
		}
	}
}

func navboxBranchRows(branch navboxBranch) int {
	rows := len(branch.Grandchildren)
	if branch.Child.Value != "" || rows == 0 {
		rows++
	}
	return rows
}

func navboxDepth(groups []navboxGroup) int {
	depth := 1
	for _, group := range groups {
		if group.Depth > depth {
			depth = group.Depth
		}
	}
	return depth
}

func renderNavboxLabelCell(out *bytes.Buffer, row navboxRow, className string, rowspan int, rowGroup bool) {
	out.WriteString(`<th class="` + className + `" scope="`)
	if rowGroup {
		out.WriteString("rowgroup")
	} else {
		out.WriteString("row")
	}
	out.WriteByte('"')
	if rowspan > 1 {
		out.WriteString(` rowspan="` + strconv.Itoa(rowspan) + `"`)
	}
	if row.Color != "" {
		out.WriteString(` style="--navbox-label-color:` + row.Color + `;--navbox-label-text:` + navboxTextColor(row.Color) + `"`)
	}
	out.WriteString(`><span class="wiki-navbox-label">`)
	out.WriteString(html.EscapeString(row.Label))
	if row.Note != "" {
		out.WriteString(`<small>` + html.EscapeString(row.Note) + `</small>`)
	}
	out.WriteString(`</span></th>`)
}

func wikiTagAttribute(tag string, name string) string {
	for _, match := range attrPattern.FindAllStringSubmatch(tag, -1) {
		if strings.EqualFold(match[1], name) {
			return match[2]
		}
	}
	return ""
}

func normalizeNavboxColor(value string) string {
	value = strings.TrimSpace(value)
	if !navboxColorPattern.MatchString(value) {
		return ""
	}
	return strings.ToUpper(value)
}

func navboxTextColor(color string) string {
	red, _ := strconv.ParseInt(color[1:3], 16, 16)
	green, _ := strconv.ParseInt(color[3:5], 16, 16)
	blue, _ := strconv.ParseInt(color[5:7], 16, 16)
	luminance := 0.2126*navboxColorChannel(red) + 0.7152*navboxColorChannel(green) + 0.0722*navboxColorChannel(blue)
	darkLuminance := navboxColorChannel(0x11)
	darkContrast := (luminance + 0.05) / (darkLuminance + 0.05)
	lightContrast := 1.05 / (luminance + 0.05)
	if darkContrast >= lightContrast {
		return "#111111"
	}
	return "#FFFFFF"
}

func navboxColorChannel(value int64) float64 {
	channel := float64(value) / 255
	if channel <= 0.04045 {
		return channel / 12.92
	}
	return math.Pow((channel+0.055)/1.055, 2.4)
}

type visualTreeNode struct {
	Label    string
	Children []*visualTreeNode
}

type visualFlowStep struct {
	Label     string
	Connected bool
}

func renderWikiTree(openingTag string, segment string, linkPrefix string) string {
	roots := parseVisualTree(segment)
	var body bytes.Buffer
	body.WriteString(`<div class="wiki-tree-canvas"><ul class="wiki-tree-roots">` + "\n")
	for _, root := range roots {
		renderVisualTreeNode(&body, root, linkPrefix)
	}
	body.WriteString(`</ul></div>` + "\n")
	return renderVisualBlock("wiki-tree", openingTag, body.String())
}

func parseVisualTree(segment string) []*visualTreeNode {
	roots := make([]*visualTreeNode, 0)
	stack := make([]*visualTreeNode, 0)
	for _, sourceLine := range strings.Split(segment, "\n") {
		line := strings.ReplaceAll(strings.TrimRight(sourceLine, " \t"), "\t", "    ")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || isVisualTreeGuideLine(line) {
			continue
		}

		depth := 0
		label := trimmed
		if match := treeLinePattern.FindStringSubmatch(line); len(match) == 3 {
			depth = len([]rune(match[1]))/4 + 1
			label = strings.TrimSpace(match[2])
		}
		if label == "" {
			continue
		}

		node := &visualTreeNode{Label: label}
		if depth == 0 || len(stack) == 0 {
			roots = append(roots, node)
			stack = []*visualTreeNode{node}
			continue
		}
		if depth > len(stack) {
			depth = len(stack)
		}
		parent := stack[depth-1]
		parent.Children = append(parent.Children, node)
		stack = append(stack[:depth], node)
	}
	return roots
}

func isVisualTreeGuideLine(line string) bool {
	if !strings.ContainsRune(line, '│') {
		return false
	}
	return strings.TrimSpace(strings.ReplaceAll(line, "│", "")) == ""
}

func renderVisualTreeNode(out *bytes.Buffer, node *visualTreeNode, linkPrefix string) {
	out.WriteString(`<li class="wiki-tree-item"><div class="wiki-tree-node">`)
	out.WriteString(renderInlineText(node.Label, linkPrefix))
	out.WriteString(`</div>`)
	if len(node.Children) > 0 {
		out.WriteString("<ul>\n")
		for _, child := range node.Children {
			renderVisualTreeNode(out, child, linkPrefix)
		}
		out.WriteString("</ul>\n")
	}
	out.WriteString("</li>\n")
}

func renderWikiFlow(openingTag string, segment string, linkPrefix string) string {
	steps := parseVisualFlow(segment)
	var body bytes.Buffer
	body.WriteString(`<ol class="wiki-flow-list">` + "\n")
	for _, step := range steps {
		body.WriteString(`<li class="wiki-flow-step"><div class="wiki-flow-node">`)
		body.WriteString(renderInlineText(step.Label, linkPrefix))
		body.WriteString(`</div>`)
		if step.Connected {
			body.WriteString(`<span class="wiki-flow-connector" aria-hidden="true"><span>↓</span></span>`)
		}
		body.WriteString("</li>\n")
	}
	body.WriteString("</ol>\n")
	return renderVisualBlock("wiki-flow", openingTag, body.String())
}

func parseVisualFlow(segment string) []visualFlowStep {
	steps := make([]visualFlowStep, 0)
	for _, line := range strings.Split(segment, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if isVisualFlowArrow(trimmed) {
			if len(steps) > 0 {
				steps[len(steps)-1].Connected = true
			}
			continue
		}
		steps = append(steps, visualFlowStep{Label: trimmed})
	}
	return steps
}

func isVisualFlowArrow(line string) bool {
	switch line {
	case "↓", "⇣", "▼":
		return true
	default:
		return false
	}
}

func renderVisualBlock(tagName string, openingTag string, body string) string {
	title := wikiTagAttribute(openingTag, "title")
	state := strings.ToLower(strings.TrimSpace(wikiTagAttribute(openingTag, "state")))
	accent := normalizeNavboxColor(wikiTagAttribute(openingTag, "accent"))

	var out bytes.Buffer
	out.WriteByte('<')
	out.WriteString(tagName)
	out.WriteString(` class="wiki-visual ` + tagName + `"`)
	if accent != "" {
		out.WriteString(` style="--wiki-visual-accent:` + accent + `;--wiki-visual-accent-text:` + navboxTextColor(accent) + `"`)
	}
	out.WriteByte('>')
	if title == "" {
		out.WriteString(`<div class="wiki-visual-body wiki-visual-untitled">`)
		out.WriteString(body)
		out.WriteString(`</div></` + tagName + `>` + "\n")
		return out.String()
	}

	out.WriteString(`<details class="wiki-visual-details"`)
	if state != "collapsed" {
		out.WriteString(` open`)
	}
	out.WriteString(`><summary class="wiki-visual-title"><span>`)
	out.WriteString(html.EscapeString(title))
	out.WriteString(`</span></summary><div class="wiki-visual-body">`)
	out.WriteString(body)
	out.WriteString(`</div></details></` + tagName + `>` + "\n")
	return out.String()
}

func renderTextSegment(segment string, linkPrefix string) string {
	if segment == "" {
		return ""
	}

	lines := strings.Split(segment, "\n")
	var out bytes.Buffer

	for i := 0; i < len(lines); {
		trimmed := strings.TrimSpace(strings.TrimRight(lines[i], " \t"))

		if trimmed == "" {
			i++
			continue
		}

		switch {
		case codeFencePattern.MatchString(trimmed):
			next := renderCodeBlock(&out, lines, i)
			i = next
		case isTableStart(lines, i):
			next := renderTable(&out, lines, i, linkPrefix)
			i = next
		case horizontalRulePattern.MatchString(trimmed):
			out.WriteString(`<hr class="techmark-divider">` + "\n")
			i++
		case unorderedListPattern.MatchString(trimmed):
			next := renderList(&out, lines, i, false, linkPrefix)
			i = next
		case orderedListPattern.MatchString(trimmed):
			next := renderList(&out, lines, i, true, linkPrefix)
			i = next
		case strings.HasPrefix(trimmed, "> "):
			next := renderBlockquote(&out, lines, i, linkPrefix)
			i = next
		case startsWithAllowedHTMLTag(trimmed):
			out.WriteString(renderInlineText(trimmed, linkPrefix))
			out.WriteByte('\n')
			i++
		case strings.HasPrefix(trimmed, "!## "):
			renderSecondLevelSectionHeading(&out, strings.TrimSpace(strings.TrimPrefix(trimmed, "!## ")), linkPrefix, true)
			i++
		case strings.HasPrefix(trimmed, "#### "):
			renderHeading(&out, "h4", strings.TrimSpace(strings.TrimPrefix(trimmed, "#### ")), linkPrefix)
			i++
		case strings.HasPrefix(trimmed, "### "):
			renderHeading(&out, "h3", strings.TrimSpace(strings.TrimPrefix(trimmed, "### ")), linkPrefix)
			i++
		case strings.HasPrefix(trimmed, "## "):
			renderSecondLevelSectionHeading(&out, strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")), linkPrefix, false)
			i++
		case strings.HasPrefix(trimmed, "# "):
			renderHeading(&out, "h1", strings.TrimSpace(strings.TrimPrefix(trimmed, "# ")), linkPrefix)
			i++
		default:
			next := renderParagraph(&out, lines, i, linkPrefix)
			i = next
		}
	}

	return out.String()
}

func renderSecondLevelSectionHeading(out *bytes.Buffer, text string, linkPrefix string, collapsed bool) {
	state := "open"
	if collapsed {
		state = "closed"
	}
	out.WriteString("<!--techmark-section:" + state + "-->")
	renderHeading(out, "h2", text, linkPrefix)
}

func renderHeading(out *bytes.Buffer, tag string, text string, linkPrefix string) {
	out.WriteByte('<')
	out.WriteString(tag)
	out.WriteByte('>')
	out.WriteString(renderInlineText(text, linkPrefix))
	out.WriteString("</")
	out.WriteString(tag)
	out.WriteString(">\n")
}

func renderParagraph(out *bytes.Buffer, lines []string, start int, linkPrefix string) int {
	var parts []string
	i := start

	for i < len(lines) {
		trimmed := strings.TrimSpace(strings.TrimRight(lines[i], " \t"))
		if trimmed == "" || isBlockStart(lines, i) {
			break
		}
		parts = append(parts, trimmed)
		i++
	}

	out.WriteString("<p>")
	out.WriteString(renderInlineText(strings.Join(parts, " "), linkPrefix))
	out.WriteString("</p>\n")
	return i
}

func renderCodeBlock(out *bytes.Buffer, lines []string, start int) int {
	trimmed := strings.TrimSpace(lines[start])
	matches := codeFencePattern.FindStringSubmatch(trimmed)
	language := ""
	blockID := ""
	if len(matches) > 1 {
		language = matches[1]
	}
	if len(matches) > 2 {
		blockID = matches[2]
	}

	var code bytes.Buffer
	i := start + 1
	for i < len(lines) {
		if strings.TrimSpace(lines[i]) == "```" {
			i++
			break
		}
		if code.Len() > 0 {
			code.WriteByte('\n')
		}
		code.WriteString(lines[i])
		i++
	}

	out.WriteString(`<pre`)
	if blockID != "" {
		out.WriteString(` id="`)
		out.WriteString(html.EscapeString(blockID))
		out.WriteByte('"')
	}
	out.WriteString(` class="code-block"><code class="syntax-chroma`)
	if language != "" {
		out.WriteString(` language-`)
		out.WriteString(html.EscapeString(language))
	}
	out.WriteString(`">`)
	out.WriteString(highlightCode(code.String(), language))
	out.WriteString("</code></pre>\n")
	return i
}

func highlightCode(source string, language string) string {
	if language == "" {
		return html.EscapeString(source)
	}

	lexer := lexers.Get(codeLexerLanguage(language))
	if lexer == nil {
		return html.EscapeString(source)
	}
	iterator, err := chroma.Coalesce(lexer).Tokenise(nil, source)
	if err != nil {
		return html.EscapeString(source)
	}

	var highlighted bytes.Buffer
	// These tokens inherit the code block's color in wiki.css. Emitting a span
	// for every identifier and punctuation mark can dwarf the actual source.
	compact := func() chroma.Token {
		token := iterator()
		switch token.Type {
		case chroma.Name, chroma.Punctuation, chroma.TextWhitespace:
			token.Type = chroma.Text
		}
		return token
	}
	if err := codeHighlightFormatter.Format(&highlighted, codeHighlightStyle, compact); err != nil {
		return html.EscapeString(source)
	}
	return highlighted.String()
}

func codeLexerLanguage(language string) string {
	switch strings.ToLower(language) {
	case "objective-c++", "objc++", "obj-c++":
		return "objective-c"
	default:
		return language
	}
}

func renderList(out *bytes.Buffer, lines []string, start int, ordered bool, linkPrefix string) int {
	itemPattern := unorderedListPattern
	tag := "ul"
	if ordered {
		itemPattern = orderedListPattern
		tag = "ol"
	}

	out.WriteByte('<')
	out.WriteString(tag)
	out.WriteString(">\n")

	i := start
	for i < len(lines) {
		trimmed := strings.TrimSpace(strings.TrimRight(lines[i], " \t"))
		matches := itemPattern.FindStringSubmatch(trimmed)
		if len(matches) == 0 {
			break
		}

		out.WriteString("<li>")
		out.WriteString(renderInlineText(strings.TrimSpace(matches[1]), linkPrefix))
		out.WriteString("</li>\n")
		i++
	}

	out.WriteString("</")
	out.WriteString(tag)
	out.WriteString(">\n")
	return i
}

func renderBlockquote(out *bytes.Buffer, lines []string, start int, linkPrefix string) int {
	var parts []string
	i := start
	for i < len(lines) {
		trimmed := strings.TrimSpace(strings.TrimRight(lines[i], " \t"))
		if !strings.HasPrefix(trimmed, "> ") {
			break
		}
		parts = append(parts, strings.TrimSpace(strings.TrimPrefix(trimmed, "> ")))
		i++
	}

	out.WriteString("<blockquote>")
	out.WriteString(renderInlineText(strings.Join(parts, " "), linkPrefix))
	out.WriteString("</blockquote>\n")
	return i
}

func renderTable(out *bytes.Buffer, lines []string, start int, linkPrefix string) int {
	header := splitTableRow(lines[start])
	out.WriteString(`<div class="wiki-table-scroll"><table>` + "\n<thead><tr>")
	for _, cell := range header {
		out.WriteString("<th>")
		out.WriteString(renderInlineText(cell, linkPrefix))
		out.WriteString("</th>")
	}
	out.WriteString("</tr></thead>\n<tbody>\n")

	i := start + 2
	for i < len(lines) && isTableRow(lines[i]) {
		cells := splitTableRow(lines[i])
		out.WriteString("<tr>")
		for _, cell := range cells {
			out.WriteString("<td>")
			out.WriteString(renderInlineText(cell, linkPrefix))
			out.WriteString("</td>")
		}
		out.WriteString("</tr>\n")
		i++
	}

	out.WriteString("</tbody>\n</table></div>\n")
	return i
}

func isBlockStart(lines []string, index int) bool {
	trimmed := strings.TrimSpace(strings.TrimRight(lines[index], " \t"))
	return codeFencePattern.MatchString(trimmed) ||
		isTableStart(lines, index) ||
		horizontalRulePattern.MatchString(trimmed) ||
		unorderedListPattern.MatchString(trimmed) ||
		orderedListPattern.MatchString(trimmed) ||
		strings.HasPrefix(trimmed, "> ") ||
		startsWithAllowedHTMLTag(trimmed) ||
		strings.HasPrefix(trimmed, "!## ") ||
		strings.HasPrefix(trimmed, "# ") ||
		strings.HasPrefix(trimmed, "## ") ||
		strings.HasPrefix(trimmed, "### ") ||
		strings.HasPrefix(trimmed, "#### ")
}

func isTableStart(lines []string, index int) bool {
	if index+1 >= len(lines) {
		return false
	}
	if !isTableRow(lines[index]) {
		return false
	}

	cells := splitTableRow(lines[index])
	if len(cells) < 2 {
		return false
	}

	return isTableSeparator(lines[index+1], len(cells))
}

func isTableRow(line string) bool {
	return len(splitTableRow(line)) >= 2
}

func isTableSeparator(line string, expectedCells int) bool {
	cells := splitTableRow(line)
	if len(cells) != expectedCells {
		return false
	}

	for _, cell := range cells {
		value := strings.TrimSpace(cell)
		value = strings.Trim(value, ":")
		if len(value) < 3 {
			return false
		}
		for _, r := range value {
			if r != '-' {
				return false
			}
		}
	}

	return true
}

func splitTableRow(line string) []string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "|") {
		trimmed = strings.TrimPrefix(trimmed, "|")
	}
	if strings.HasSuffix(trimmed, "|") {
		trimmed = strings.TrimSuffix(trimmed, "|")
	}

	cells := make([]string, 0, strings.Count(trimmed, "|")+1)
	var cell strings.Builder
	inCode := false
	wikiLinkDepth := 0
	for index := 0; index < len(trimmed); index++ {
		char := trimmed[index]
		if char == '`' {
			inCode = !inCode
			cell.WriteByte(char)
			continue
		}
		if !inCode && index+1 < len(trimmed) {
			pair := trimmed[index : index+2]
			switch pair {
			case "[[":
				wikiLinkDepth++
				cell.WriteString(pair)
				index++
				continue
			case "]]":
				if wikiLinkDepth > 0 {
					wikiLinkDepth--
				}
				cell.WriteString(pair)
				index++
				continue
			}
		}
		if char == '|' && !inCode && wikiLinkDepth == 0 {
			cells = append(cells, strings.TrimSpace(cell.String()))
			cell.Reset()
			continue
		}
		cell.WriteByte(char)
	}
	cells = append(cells, strings.TrimSpace(cell.String()))
	return cells
}

func renderInlineText(text string, linkPrefix string) string {
	var out bytes.Buffer
	position := 0
	for _, current := range inlineCodeRanges(text) {
		out.WriteString(renderInline(sanitizeInlineHTML(text[position:current[0]]), linkPrefix))
		closed := current[1]-current[0] >= 2 && text[current[1]-1] == '`'
		if !closed {
			out.WriteString(html.EscapeString(text[current[0]:current[1]]))
			position = current[1]
			continue
		}
		out.WriteString("<code>")
		out.WriteString(html.EscapeString(text[current[0]+1 : current[1]-1]))
		out.WriteString("</code>")
		position = current[1]
	}
	out.WriteString(renderInline(sanitizeInlineHTML(text[position:]), linkPrefix))
	return out.String()
}

func sanitizeInlineHTML(text string) string {
	var out bytes.Buffer
	last := 0

	for _, match := range htmlTagPattern.FindAllStringIndex(text, -1) {
		out.WriteString(html.EscapeString(text[last:match[0]]))
		out.WriteString(sanitizeHTMLTag(text[match[0]:match[1]]))
		last = match[1]
	}

	out.WriteString(html.EscapeString(text[last:]))
	return out.String()
}

func sanitizeHTMLTag(tag string) string {
	trimmed := strings.TrimSpace(tag)
	closing := strings.HasPrefix(trimmed, "</")

	name := strings.TrimPrefix(trimmed, "</")
	name = strings.TrimPrefix(name, "<")
	name = strings.TrimSuffix(name, ">")
	name = strings.TrimSuffix(name, "/")
	name = strings.TrimSpace(name)
	if fields := strings.Fields(name); len(fields) > 0 {
		name = fields[0]
	}
	name = strings.ToLower(strings.Trim(name, "/"))

	if _, ok := allowedHTMLTags[name]; !ok {
		return html.EscapeString(tag)
	}

	if closing {
		return "</" + name + ">"
	}
	if name == "br" {
		return "<br>"
	}
	if name == "techpedia-time" {
		matches := attrPattern.FindAllStringSubmatch(trimmed, -1)
		for _, match := range matches {
			if strings.EqualFold(match[1], "datetime") {
				return `<techpedia-time datetime="` + html.EscapeString(match[2]) + `">`
			}
		}
	}
	return "<" + name + ">"
}

func startsWithAllowedHTMLTag(text string) bool {
	match := htmlTagPattern.FindStringIndex(text)
	if match == nil || match[0] != 0 {
		return false
	}

	tag := text[match[0]:match[1]]
	return sanitizeHTMLTag(tag) != html.EscapeString(tag)
}

func renderInline(text string, linkPrefix string) string {
	text = boldPattern.ReplaceAllString(text, "<strong>$1</strong>")
	externalLinks := make([]string, 0)
	text = markdownLinkPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := markdownLinkPattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		externalLinks = append(externalLinks, `<a class="external-link" href="`+html.EscapeString(html.UnescapeString(parts[2]))+`" target="_blank" rel="nofollow noopener noreferrer">`+parts[1]+`</a>`)
		return "\x00techpedia-external-link:" + strconv.Itoa(len(externalLinks)-1) + "\x00"
	})
	text = linkPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := linkPattern.FindStringSubmatch(match)
		if len(parts) == 0 {
			return match
		}

		rawTarget := html.UnescapeString(strings.TrimSpace(parts[1]))
		target := WikiLinkTargetName(rawTarget)
		label := rawTarget
		if len(parts) > 2 && strings.TrimSpace(parts[2]) != "" {
			label = html.UnescapeString(strings.TrimSpace(parts[2]))
		}

		return `<a class="wiki-link" href="` + html.EscapeString(linkPrefix) + html.EscapeString(URLName(target)) + `">` + html.EscapeString(label) + `</a>`
	})
	return externalLinkTokenPattern.ReplaceAllStringFunc(text, func(token string) string {
		parts := externalLinkTokenPattern.FindStringSubmatch(token)
		if len(parts) != 2 {
			return token
		}
		index, err := strconv.Atoi(parts[1])
		if err != nil || index < 0 || index >= len(externalLinks) {
			return token
		}
		return externalLinks[index]
	})
}

func sanitizeWikiTag(tag string) string {
	trimmed := strings.TrimSpace(tag)
	if strings.HasPrefix(trimmed, "</") {
		name := strings.TrimPrefix(strings.TrimSuffix(trimmed, ">"), "</")
		name = strings.ToLower(strings.TrimSpace(name))
		if !isAllowedWikiTagName(name) {
			return html.EscapeString(tag)
		}
		return "</" + name + ">"
	}

	inner := strings.TrimPrefix(strings.TrimSuffix(trimmed, ">"), "<")
	fields := strings.Fields(inner)
	if len(fields) == 0 {
		return html.EscapeString(tag)
	}

	name := strings.ToLower(fields[0])
	if !isAllowedWikiTagName(name) {
		return html.EscapeString(tag)
	}

	var out bytes.Buffer
	out.WriteByte('<')
	out.WriteString(name)

	seen := make(map[string]struct{})
	for _, match := range attrPattern.FindAllStringSubmatch(inner, -1) {
		attrName := strings.ToLower(match[1])
		if !isAllowedWikiTagAttribute(name, attrName) {
			continue
		}
		if _, exists := seen[attrName]; exists {
			continue
		}
		seen[attrName] = struct{}{}
		attrValue := html.EscapeString(match[2])
		out.WriteByte(' ')
		out.WriteString(attrName)
		out.WriteString(`="`)
		out.WriteString(attrValue)
		out.WriteByte('"')
	}

	out.WriteByte('>')
	return out.String()
}

func wikiTagName(tag string) (string, bool) {
	trimmed := strings.TrimSpace(tag)
	closing := strings.HasPrefix(trimmed, "</")

	name := strings.TrimPrefix(trimmed, "</")
	name = strings.TrimPrefix(name, "<")
	name = strings.TrimSuffix(name, ">")
	name = strings.TrimSuffix(name, "/")
	name = strings.TrimSpace(name)
	if fields := strings.Fields(name); len(fields) > 0 {
		name = fields[0]
	}

	return strings.ToLower(strings.Trim(name, "/")), closing
}

type wikiTagEnd struct{ start, end int }
type wikiClosingTags map[string][]wikiTagEnd

func indexWikiClosingTags(source string) wikiClosingTags {
	result := wikiClosingTags{}
	position := 0
	for position < len(source) {
		match := wikiTagPattern.FindStringIndex(source[position:])
		if match == nil {
			break
		}

		tagStart := position + match[0]
		tagEnd := position + match[1]
		tagName, closing := wikiTagName(source[tagStart:tagEnd])
		if closing {
			result[tagName] = append(result[tagName], wikiTagEnd{tagStart, tagEnd})
		}

		position = tagEnd
	}

	return result
}

func (index wikiClosingTags) after(start int, name string) (int, int) {
	tags := index[name]
	i := sort.Search(len(tags), func(i int) bool { return tags[i].start >= start })
	if i == len(tags) {
		return -1, -1
	}
	return tags[i].start, tags[i].end
}

func isAllowedWikiTagName(name string) bool {
	_, ok := allowedWikiTagAttributes[name]
	return ok
}

func isAllowedWikiTagAttribute(name string, attribute string) bool {
	attributes, ok := allowedWikiTagAttributes[name]
	if !ok {
		return false
	}
	_, ok = attributes[attribute]
	return ok
}
