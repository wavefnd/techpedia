package techmark

import (
	"net/url"
	"regexp"
	"strings"
)

// This scans our own escaped renderer output, not arbitrary source HTML.
var renderedMediaImages = regexp.MustCompile(`<img\b[^>]*>`)

// UsesMediaFile follows rendered images, so code examples and captions do not
// falsely prevent deletion. Rendering also handles galleries and infoboxes.
func (p *Parser) UsesMediaFile(source, name string) bool {
	for _, tag := range renderedMediaImages.FindAllString(p.Render(source), -1) {
		for _, key := range []string{"src", "data-large-src"} {
			u, err := url.Parse(wikiTagAttribute(tag, key))
			if err != nil || !strings.HasPrefix(u.Path, "/media/files/") {
				continue
			}
			parts := strings.Split(strings.TrimPrefix(u.Path, "/media/files/"), "/")
			if len(parts) == 2 && strings.EqualFold(parts[0], name) {
				return true
			}
		}
	}
	return false
}
