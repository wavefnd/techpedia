package techmark

import (
	"fmt"
	"strings"
	"unicode"
)

func Slugify(input string) string {
	input = strings.TrimSpace(strings.ToLower(input))
	var out strings.Builder
	lastDash := false

	for _, r := range input {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			out.WriteRune(r)
			lastDash = false
		case r == '+':
			writeSlugWord(&out, &lastDash, "plus")
		case r == '#':
			writeSlugWord(&out, &lastDash, "sharp")
		case r == '*':
			writeSlugWord(&out, &lastDash, "star")
		case r == '/':
			writeSlugWord(&out, &lastDash, "slash")
		case r == '-' || r == '_' || r == '.' || unicode.IsSpace(r):
			if !lastDash && out.Len() > 0 {
				out.WriteByte('-')
				lastDash = true
			}
		}
	}

	result := strings.Trim(out.String(), "-")
	if result == "" {
		return "untitled"
	}
	return result
}

// URLName keeps the document title in the public URL while escaping path delimiters.
func URLName(input string) string {
	name := strings.ReplaceAll(strings.TrimSpace(input), " ", "_")
	var out strings.Builder
	for _, r := range name {
		if r != '/' && r != '#' && r != '?' && r != '%' && r != '\\' && !unicode.IsControl(r) {
			out.WriteRune(r)
			continue
		}
		for _, value := range []byte(string(r)) {
			_, _ = fmt.Fprintf(&out, "%%%02X", value)
		}
	}
	return out.String()
}

// WikiLinkTargetName applies the first-letter convention to a link target
// without changing the label written in the document source.
func WikiLinkTargetName(input string) string {
	name := strings.TrimSpace(input)
	runes := []rune(name)
	if len(runes) > 0 && unicode.In(runes[0], unicode.Latin) {
		runes[0] = unicode.ToUpper(runes[0])
	}
	return string(runes)
}

func writeSlugWord(out *strings.Builder, lastDash *bool, word string) {
	if out.Len() > 0 && !*lastDash {
		out.WriteByte('-')
	}
	out.WriteString(word)
	out.WriteByte('-')
	*lastDash = true
}
