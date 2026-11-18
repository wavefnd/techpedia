package server

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"errors"
	"html"
	"math/big"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"techpedia/internal/document"
	"techpedia/internal/edition"
	"techpedia/internal/storage"
	"techpedia/internal/techmark"
)

var (
	recentChangesElementPattern = regexp.MustCompile(`(?is)<wiki-recent-changes(?:\s+[^<>]*)?>\s*</wiki-recent-changes>`)
	randomPageElementPattern    = regexp.MustCompile(`(?is)<wiki-random-page(?:\s+[^<>]*)?>\s*</wiki-random-page>`)
	featuredPagesElementPattern = regexp.MustCompile(`(?is)<wiki-featured-pages(?:\s+[^<>]*)?>\s*</wiki-featured-pages>`)
	popularPagesElementPattern  = regexp.MustCompile(`(?is)<wiki-popular-pages(?:\s+[^<>]*)?>\s*</wiki-popular-pages>`)
	specialElementAttrPattern   = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9_-]*)="([^"]*)"`)
)

func (s *Server) renderDocument(ctx context.Context, doc document.Document) (string, error) {
	if doc.Source == "" {
		return doc.HTML, nil
	}

	source := doc.Source
	if techmark.ContainsSpecialSyntax(source) && document.NormalizeEditPolicy(doc.EditPolicy) != document.EditPolicyAdmins {
		source = techmark.StripSpecialSyntax(source)
	}
	if techmark.ContainsHomePageSyntax(source) {
		isMainPage, err := s.isEditionMainPage(ctx, doc.Locale, doc.Slug)
		if err != nil {
			return "", err
		}
		if !isMainPage {
			source = techmark.StripHomePageSyntax(source)
		}
	}
	rendered := s.parser.RenderWithLocale(source, doc.Locale)
	var boxErr error
	rendered, boxErr = s.renderBoxes(ctx, rendered, doc.Locale)
	if boxErr != nil {
		return "", boxErr
	}
	if !techmark.ContainsSpecialSyntax(source) {
		return rendered, nil
	}

	var err error
	rendered, err = replaceSpecialElements(rendered, recentChangesElementPattern, func(element string) (string, error) {
		return s.renderRecentChanges(ctx, doc.Locale, specialLimit(element), specialAttribute(element, "empty"))
	})
	if err != nil {
		return "", err
	}
	rendered, err = replaceSpecialElements(rendered, randomPageElementPattern, func(element string) (string, error) {
		label := strings.TrimSpace(specialAttribute(element, "label"))
		if label == "" {
			label = doc.Title
		}
		return renderRandomPageLink(doc.Locale, label), nil
	})
	if err != nil {
		return "", err
	}
	rendered, err = replaceSpecialElements(rendered, featuredPagesElementPattern, func(element string) (string, error) {
		return s.renderFeaturedPages(ctx, doc, element)
	})
	if err != nil {
		return "", err
	}
	return replaceSpecialElements(rendered, popularPagesElementPattern, func(element string) (string, error) {
		return s.renderPopularPages(ctx, doc, element)
	})
}

func (s *Server) isEditionMainPage(ctx context.Context, locale string, slug string) (bool, error) {
	value, err := s.store.GetEdition(ctx, locale)
	if err != nil {
		return false, err
	}
	return techmark.Slugify(value.MainPageName) == slug, nil
}

func (s *Server) renderFeaturedPages(ctx context.Context, current document.Document, element string) (string, error) {
	pages, err := s.store.ListLinkDocuments(ctx, current.Locale)
	if err != nil {
		return "", err
	}

	type dailyDocument struct {
		doc  document.Document
		hash [32]byte
	}
	day := time.Now().UTC().Format("2006-01-02")
	candidates := make([]dailyDocument, 0, len(pages))
	for _, page := range pages {
		doc := page.Document
		if doc.Slug == current.Slug {
			continue
		}
		seed := day + "\x00" + current.Locale + "\x00" + doc.Slug
		candidates = append(candidates, dailyDocument{doc: doc, hash: sha256.Sum256([]byte(seed))})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		comparison := bytes.Compare(candidates[i].hash[:], candidates[j].hash[:])
		if comparison == 0 {
			return candidates[i].doc.Title < candidates[j].doc.Title
		}
		return comparison < 0
	})

	limit := homePanelLimit(element, 3)
	selected := make([]document.Document, 0, limit)
	for _, candidate := range candidates {
		doc, err := s.store.GetLatest(ctx, current.Locale, candidate.doc.Slug)
		if errors.Is(err, storage.ErrDocumentNotFound) {
			continue
		}
		if err != nil {
			return "", err
		}
		if techmark.ContainsSpecialSyntax(doc.Source) {
			continue
		}
		selected = append(selected, doc)
		if len(selected) == limit {
			break
		}
	}
	return renderHomeDocumentList(current.Locale, "featured", specialAttribute(element, "title"), specialAttribute(element, "empty"), selected, false), nil
}

func (s *Server) renderPopularPages(ctx context.Context, current document.Document, element string) (string, error) {
	days := popularPeriodDays(specialAttribute(element, "period"))
	popular, err := s.store.ListPopular(ctx, current.Locale, time.Now().UTC().AddDate(0, 0, 1-days), 50)
	if err != nil {
		return "", err
	}

	limit := homePanelLimit(element, 10)
	selected := make([]document.Document, 0, limit)
	for _, result := range popular {
		if result.Document.Slug == current.Slug || techmark.ContainsSpecialSyntax(result.Document.Source) {
			continue
		}
		selected = append(selected, result.Document)
		if len(selected) == limit {
			break
		}
	}
	return renderHomeDocumentList(current.Locale, "popular", specialAttribute(element, "title"), specialAttribute(element, "empty"), selected, true), nil
}

func renderHomeDocumentList(locale string, kind string, title string, emptyLabel string, docs []document.Document, ordered bool) string {
	var out bytes.Buffer
	out.WriteString(`<section class="wiki-home-panel wiki-home-panel-`)
	out.WriteString(kind)
	out.WriteString(`">`)
	if strings.TrimSpace(title) != "" {
		out.WriteString(`<h2>`)
		out.WriteString(html.EscapeString(title))
		out.WriteString(`</h2>`)
	}
	if len(docs) == 0 {
		if strings.TrimSpace(emptyLabel) != "" {
			out.WriteString(`<p class="wiki-special-empty">`)
			out.WriteString(html.EscapeString(emptyLabel))
			out.WriteString(`</p>`)
		}
		out.WriteString(`</section>`)
		return out.String()
	}
	listTag := "ul"
	if ordered {
		listTag = "ol"
	}
	out.WriteByte('<')
	out.WriteString(listTag)
	out.WriteString(` class="wiki-home-document-list">`)
	for _, doc := range docs {
		out.WriteString(`<li><a href="`)
		out.WriteString(html.EscapeString(wikiURL(locale, doc.Title)))
		out.WriteString(`">`)
		out.WriteString(html.EscapeString(doc.Title))
		out.WriteString(`</a></li>`)
	}
	out.WriteString(`</`)
	out.WriteString(listTag)
	out.WriteString(`></section>`)
	return out.String()
}

func homePanelLimit(element string, fallback int) int {
	value, err := strconv.Atoi(specialAttribute(element, "limit"))
	if err != nil || value <= 0 {
		return fallback
	}
	if value > 12 {
		return 12
	}
	return value
}

func popularPeriodDays(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1d":
		return 1
	case "30d":
		return 30
	default:
		return 7
	}
}

func replaceSpecialElements(input string, pattern *regexp.Regexp, render func(string) (string, error)) (string, error) {
	matches := pattern.FindAllStringIndex(input, -1)
	if len(matches) == 0 {
		return input, nil
	}

	var out bytes.Buffer
	position := 0
	for _, match := range matches {
		out.WriteString(input[position:match[0]])
		replacement, err := render(input[match[0]:match[1]])
		if err != nil {
			return "", err
		}
		out.WriteString(replacement)
		position = match[1]
	}
	out.WriteString(input[position:])
	return out.String(), nil
}

func (s *Server) renderRecentChanges(ctx context.Context, locale string, limit int, emptyLabel string) (string, error) {
	changes, err := s.store.ListChanges(ctx, document.ChangeFilter{Locale: locale, Limit: limit})
	if err != nil {
		return "", err
	}
	if len(changes) == 0 {
		if strings.TrimSpace(emptyLabel) == "" {
			return `<div class="wiki-special-empty" aria-hidden="true"></div>`, nil
		}
		return `<p class="wiki-special-empty">` + html.EscapeString(emptyLabel) + `</p>`, nil
	}

	var out bytes.Buffer
	out.WriteString(`<section class="wiki-special wiki-special-recent"><ol class="special-change-list">`)
	for _, change := range changes {
		out.WriteString(`<li><div class="special-change-primary"><a href="`)
		out.WriteString(html.EscapeString(wikiURL(change.Locale, change.Title)))
		out.WriteString(`">`)
		out.WriteString(html.EscapeString(change.Title))
		out.WriteString(`</a><span class="special-change-action">`)
		out.WriteString(html.EscapeString(string(change.Action)))
		out.WriteString(`</span>`)
		if change.Revision != "" {
			out.WriteString(`<a class="special-revision" href="`)
			out.WriteString(html.EscapeString(historyURL(change.Locale, change.Title)))
			out.WriteString(`">`)
			out.WriteString(html.EscapeString(change.Revision))
			out.WriteString(`</a>`)
		}
		out.WriteString(`</div><div class="special-change-meta"><span>`)
		out.WriteString(html.EscapeString(change.Author))
		out.WriteString(`</span>`)
		if strings.TrimSpace(change.Summary) != "" {
			out.WriteString(`<span>`)
			out.WriteString(html.EscapeString(change.Summary))
			out.WriteString(`</span>`)
		}
		out.WriteString(`<techpedia-time datetime="`)
		out.WriteString(html.EscapeString(change.CreatedAt.Format(time.RFC3339)))
		out.WriteString(`"></techpedia-time></div></li>`)
	}
	out.WriteString(`</ol></section>`)
	return out.String(), nil
}

func renderRandomPageLink(locale string, label string) string {
	return `<p class="wiki-special wiki-special-random"><a href="/` + html.EscapeString(locale) + `/special/random">` + html.EscapeString(label) + `</a></p>`
}

func specialLimit(element string) int {
	value, err := strconv.Atoi(specialAttribute(element, "limit"))
	if err != nil || value <= 0 {
		return 50
	}
	if value > 100 {
		return 100
	}
	return value
}

func specialAttribute(element string, name string) string {
	for _, match := range specialElementAttrPattern.FindAllStringSubmatch(element, -1) {
		if strings.EqualFold(match[1], name) {
			return html.UnescapeString(match[2])
		}
	}
	return ""
}

func historyURL(locale string, name string) string {
	return "/history/" + locale + "/" + techmark.URLName(name)
}

func (s *Server) handleRandomPage(w http.ResponseWriter, r *http.Request) {
	locale, ok := requestLocale(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !s.requireActiveEdition(w, r, locale) {
		return
	}

	docs, err := s.store.ListAllLatest(r.Context(), locale)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	candidates := make([]document.Document, 0, len(docs))
	for _, doc := range docs {
		if !techmark.ContainsSpecialSyntax(doc.Source) {
			candidates = append(candidates, doc)
		}
	}
	if len(candidates) == 0 {
		value, editionErr := s.store.GetEdition(r.Context(), locale)
		if editionErr != nil || !value.Active() {
			if editionErr != nil && editionErr != edition.ErrNotFound {
				http.Error(w, editionErr.Error(), http.StatusInternalServerError)
				return
			}
			http.Error(w, storage.ErrDocumentNotFound.Error(), http.StatusNotFound)
			return
		}
		http.Redirect(w, r, wikiURL(locale, value.MainPageName), http.StatusTemporaryRedirect)
		return
	}

	index, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(len(candidates))))
	if err != nil {
		http.Error(w, "unable to select a random page", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, wikiURL(locale, candidates[index.Int64()].Title), http.StatusTemporaryRedirect)
}
