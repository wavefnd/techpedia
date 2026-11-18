package server

import (
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"techpedia/internal/document"
	"techpedia/internal/edition"
	"techpedia/internal/techmark"
)

const defaultSiteDescription = "TechPedia는 컴퓨터 과학과 소프트웨어 공학 지식을 다루는 기술 위키입니다."

type indexMetadata struct {
	Lang          string
	Direction     string
	Title         string
	Description   string
	Canonical     string
	Robots        string
	OpenGraph     bool
	UpdatedAt     string
	Alternates    []indexAlternate
	JSONLD        string
	InitialHTML   string
	InitialData   string
	InitialDataID string
	PortalAssets  bool
}

type indexAlternate struct {
	Locale string
	URL    string
}

func defaultIndexMetadata() indexMetadata {
	return indexMetadata{
		Lang:        document.DefaultLocale,
		Direction:   "ltr",
		Title:       "TechPedia",
		Description: defaultSiteDescription,
		Robots:      "noindex,nofollow",
	}
}

func (s *Server) handleArticleIndex(w http.ResponseWriter, r *http.Request, locale string) {
	name := strings.TrimSpace(strings.ReplaceAll(r.PathValue("name"), "_", " "))
	slug := techmark.Slugify(name)

	var doc document.Document
	var err error
	doc, err = s.store.GetLatest(r.Context(), locale, slug)
	if isDocumentNotFound(err) {
		if target, redirectErr := s.store.GetDocumentRedirect(r.Context(), locale, slug); redirectErr == nil {
			if moved, targetErr := s.store.GetLatest(r.Context(), locale, target); targetErr == nil {
				http.Redirect(w, r, wikiURL(locale, moved.Title), http.StatusPermanentRedirect)
				return
			}
		}
		s.writeIndex(w, r, http.StatusNotFound, s.missingIndexMetadata(locale, name, r.URL.Path))
		return
	}
	if err != nil {
		http.Error(w, "failed to load document", http.StatusInternalServerError)
		return
	}
	canonicalPath := wikiURL(locale, doc.Title)
	if canonical, parseErr := url.Parse(canonicalPath); parseErr == nil && r.URL.Path != canonical.Path {
		http.Redirect(w, r, canonicalPath, http.StatusPermanentRedirect)
		return
	}

	translations, err := s.translationsForPage(r.Context(), doc)
	if err != nil {
		http.Error(w, "failed to load document languages", http.StatusInternalServerError)
		return
	}
	rendered, err := s.renderDocument(r.Context(), doc)
	if err != nil {
		http.Error(w, "failed to render document", http.StatusInternalServerError)
		return
	}
	metadata := s.articleIndexMetadata(doc, translations)
	metadata.InitialHTML = articleInitialHTML(doc, rendered)
	doc.HTML = rendered
	var currentEdition *edition.Edition
	if value, err := s.store.GetEdition(r.Context(), locale); err == nil {
		metadata.Direction = value.Direction
		currentEdition = &value
	}
	metadata.InitialData = articleInitialData(doc, translations, currentEdition)
	s.writeIndex(w, r, http.StatusOK, metadata)
}

func (s *Server) missingIndexMetadata(locale string, name string, path string) indexMetadata {
	title := name + " - 문서 없음 - TechPedia"
	description := name + " 문서는 아직 작성되지 않았습니다."
	if !strings.HasPrefix(strings.ToLower(locale), "ko") {
		title = name + " - Page not found - TechPedia"
		description = name + " has not been written yet."
	}
	return indexMetadata{
		Lang:        locale,
		Direction:   "ltr",
		Title:       title,
		Description: description,
		Canonical:   s.absoluteURL(path),
		Robots:      "noindex,follow,noarchive",
	}
}

func (s *Server) inactiveEditionMetadata(locale string, path string) indexMetadata {
	title := "비활성 언어판 - TechPedia"
	description := "이 언어판은 메인 페이지가 아직 생성되지 않았습니다. 관리자에게 언어판 활성화를 요청하세요."
	if locale != "ko" {
		title = "Inactive language edition - TechPedia"
		description = "This language edition has no main page yet. Ask an administrator to activate it."
	}
	return indexMetadata{
		Lang:        locale,
		Direction:   "ltr",
		Title:       title,
		Description: description,
		Canonical:   s.absoluteURL(path),
		Robots:      "noindex,follow,noarchive",
	}
}

func (s *Server) articleIndexMetadata(doc document.Document, translations []document.Document) indexMetadata {
	canonical := s.absoluteURL(wikiURL(doc.Locale, doc.Title))
	metadata := indexMetadata{
		Lang:        doc.Locale,
		Direction:   "ltr",
		Title:       doc.Title + " - TechPedia",
		Description: documentDescription(doc.Source, doc.Title),
		Canonical:   canonical,
		Robots:      "index,follow,max-image-preview:large,max-snippet:-1,max-video-preview:-1",
		OpenGraph:   true,
	}
	if !doc.UpdatedAt.IsZero() {
		metadata.UpdatedAt = doc.UpdatedAt.UTC().Format(time.RFC3339)
	}

	seen := make(map[string]struct{}, len(translations))
	for _, translation := range translations {
		translation = document.NormalizeDocument(translation)
		if _, ok := seen[translation.Locale]; ok {
			continue
		}
		seen[translation.Locale] = struct{}{}
		metadata.Alternates = append(metadata.Alternates, indexAlternate{
			Locale: translation.Locale,
			URL:    s.absoluteURL(wikiURL(translation.Locale, translation.Title)),
		})
	}
	sort.Slice(metadata.Alternates, func(i, j int) bool {
		return metadata.Alternates[i].Locale < metadata.Alternates[j].Locale
	})
	metadata.JSONLD = s.techArticleJSONLD(doc, canonical)
	return metadata
}

func (s *Server) techArticleJSONLD(doc document.Document, canonical string) string {
	organizationID := s.publicURL + "/#organization"
	websiteID := s.publicURL + "/#website"
	article := map[string]any{
		"@type":            []string{"Article", "TechArticle"},
		"@id":              canonical + "#article",
		"url":              canonical,
		"headline":         doc.Title,
		"description":      documentDescription(doc.Source, doc.Title),
		"inLanguage":       doc.Locale,
		"mainEntityOfPage": map[string]any{"@type": "WebPage", "@id": canonical},
		"author":           map[string]any{"@type": "Organization", "name": "TechPedia contributors", "url": s.publicURL + "/"},
		"publisher":        map[string]any{"@id": organizationID},
		"isPartOf":         map[string]any{"@id": websiteID},
	}
	if doc.Revision != "system" {
		if !doc.CreatedAt.IsZero() {
			article["datePublished"] = doc.CreatedAt.UTC().Format(time.RFC3339)
		}
		if !doc.UpdatedAt.IsZero() {
			article["dateModified"] = doc.UpdatedAt.UTC().Format(time.RFC3339)
		}
	}
	entities := s.websiteJSONLDEntities()
	entities = append(entities, article)
	graph := map[string]any{
		"@context": "https://schema.org",
		"@graph":   entities,
	}
	return marshalJSONLD(graph)
}

func (s *Server) websiteJSONLD() string {
	return marshalJSONLD(map[string]any{
		"@context": "https://schema.org",
		"@graph":   s.websiteJSONLDEntities(),
	})
}

func (s *Server) websiteJSONLDEntities() []any {
	organizationID := s.publicURL + "/#organization"
	return []any{
		map[string]any{
			"@type": "Organization",
			"@id":   organizationID,
			"name":  "TechPedia",
			"url":   s.publicURL + "/",
		},
		map[string]any{
			"@type":     "WebSite",
			"@id":       s.publicURL + "/#website",
			"name":      "TechPedia",
			"url":       s.publicURL + "/",
			"publisher": map[string]any{"@id": organizationID},
		},
	}
}

func marshalJSONLD(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

func contentETag(data []byte) string {
	digest := sha256.Sum256(data)
	return fmt.Sprintf(`"%x"`, digest[:16])
}

func requestMatchesValidator(r *http.Request, etag string, modifiedAt time.Time) bool {
	ifNoneMatch := strings.TrimSpace(r.Header.Get("If-None-Match"))
	if ifNoneMatch != "" {
		for _, candidate := range strings.Split(ifNoneMatch, ",") {
			candidate = strings.TrimSpace(candidate)
			if candidate == "*" || strings.TrimPrefix(candidate, "W/") == strings.TrimPrefix(etag, "W/") {
				return true
			}
		}
		return false
	}
	if modifiedAt.IsZero() {
		return false
	}
	ifModifiedSince, err := http.ParseTime(r.Header.Get("If-Modified-Since"))
	if err != nil {
		return false
	}
	return !modifiedAt.UTC().Truncate(time.Second).After(ifModifiedSince.UTC())
}

func setRevalidationHeaders(w http.ResponseWriter, r *http.Request, data []byte, modifiedAt time.Time) bool {
	etag := contentETag(data)
	w.Header().Set("ETag", etag)
	if !modifiedAt.IsZero() {
		w.Header().Set("Last-Modified", modifiedAt.UTC().Format(http.TimeFormat))
	}
	return requestMatchesValidator(r, etag, modifiedAt)
}

func articleInitialHTML(doc document.Document, rendered string) string {
	var out strings.Builder
	out.WriteString(`<main class="article-view seo-initial-content"><article class="article-body"><header class="article-header"><div><h1>`)
	out.WriteString(html.EscapeString(doc.Title))
	out.WriteString(`</h1><p>`)
	out.WriteString(html.EscapeString(documentDescription(doc.Source, doc.Title)))
	out.WriteString(`</p></div></header><div class="techmark-content">`)
	out.WriteString(rendered)
	out.WriteString(`</div></article></main>`)
	return out.String()
}

func articleInitialData(doc document.Document, translations []document.Document, currentEdition *edition.Edition) string {
	response := newPublicPageResponse(doc, translations)
	// The rendered body is already present in the initial HTML. The browser reads
	// it from the DOM to avoid sending the potentially large article twice.
	response.HTML = ""
	initial := struct {
		publicPageResponse
		Edition *editionResponse `json:"edition,omitempty"`
	}{publicPageResponse: response}
	if currentEdition != nil && currentEdition.Active() && currentEdition.Tag == doc.Locale {
		value := newEditionResponse(*currentEdition)
		initial.Edition = &value
	}
	data, err := json.Marshal(initial)
	if err != nil {
		return ""
	}
	return string(data)
}

func activePortalEditions(editions []edition.Edition, documentCounts map[string]int) []edition.Edition {
	active := make([]edition.Edition, 0, len(editions))
	for _, value := range editions {
		if value.Active() {
			active = append(active, value)
		}
	}
	sort.SliceStable(active, func(i, j int) bool {
		leftCount, rightCount := documentCounts[active[i].Tag], documentCounts[active[j].Tag]
		if leftCount != rightCount {
			return leftCount > rightCount
		}
		return active[i].Tag < active[j].Tag
	})
	return active
}

type languagePortalData struct {
	Editions []languagePortalEdition `json:"editions"`
	Support  supportSettingsResponse `json:"support"`
}

type languagePortalEdition struct {
	Tag           string         `json:"tag"`
	Name          string         `json:"name"`
	NativeName    string         `json:"native_name"`
	Direction     string         `json:"direction"`
	Status        edition.Status `json:"status"`
	MainPageName  string         `json:"main_page_name"`
	MainPageURL   string         `json:"main_page_url"`
	PortalOrder   int            `json:"portal_order"`
	CountryCodes  []string       `json:"country_codes"`
	DocumentCount int            `json:"document_count"`
}

func languagePortalInitialData(editions []edition.Edition, documentCounts map[string]int, supportSettings supportSettingsResponse) string {
	active := activePortalEditions(editions, documentCounts)
	responses := make([]languagePortalEdition, 0, len(active))
	for _, value := range active {
		countryCodes := value.CountryCodes
		if countryCodes == nil {
			countryCodes = []string{}
		}
		responses = append(responses, languagePortalEdition{
			Tag: value.Tag, Name: value.Name, NativeName: value.NativeName,
			Direction: value.Direction, Status: value.Status,
			MainPageName: value.MainPageName, MainPageURL: wikiURL(value.Tag, value.MainPageName),
			PortalOrder: value.PortalOrder, CountryCodes: countryCodes,
			DocumentCount: documentCounts[value.Tag],
		})
	}
	data, err := json.Marshal(languagePortalData{Editions: responses, Support: supportSettings})
	if err != nil {
		return ""
	}
	return string(data)
}

func languagePortalInitialHTML(editions []edition.Edition, documentCounts map[string]int, supportSettings supportSettingsResponse) string {
	active := activePortalEditions(editions, documentCounts)

	var out strings.Builder
	out.WriteString(`<main class="language-portal"><button class="portal-theme-toggle theme-toggle icon-button" type="button" title="Theme" aria-label="Theme" aria-pressed="false"><span class="portal-control-icon" aria-hidden="true"></span></button><header class="portal-header"><div class="portal-mark" aria-hidden="true">T</div><h1>TechPedia</h1><p>Technical knowledge wiki</p></header><section class="portal-language-panel" aria-labelledby="portal-languages"><div class="portal-section-head"><h2 id="portal-languages">Languages</h2><span>`)
	fmt.Fprintf(&out, "%d / %d", len(active), len(active))
	out.WriteString(`</span></div><div class="portal-query-row"><label class="portal-search"><span class="portal-control-icon" aria-hidden="true"></span><span class="sr-only">Find a language</span><input type="search" placeholder="Find a language"></label><div class="portal-country-filter"><div class="portal-country-search"><span class="portal-control-icon" aria-hidden="true"></span><label class="sr-only" for="portal-country-query">Country or region</label><input id="portal-country-query" type="search" role="combobox" autocomplete="off" placeholder="Country or region" aria-autocomplete="list" aria-controls="portal-country-options" aria-expanded="false"><span class="portal-control-icon portal-control-icon-small" aria-hidden="true"></span></div></div><button class="portal-sort-button" type="button" title="Sort by fewest documents first" aria-label="Sort by fewest documents first" aria-pressed="false"><span class="portal-control-icon" aria-hidden="true"></span></button></div><div class="portal-language-grid">`)
	for _, value := range active {
		nativeName := strings.TrimSpace(value.NativeName)
		if nativeName == "" {
			nativeName = value.Name
		}
		displayName := strings.TrimSpace(value.Name)
		if displayName == "" {
			displayName = value.Tag
		}
		direction := value.Direction
		if direction != "rtl" {
			direction = "ltr"
		}
		count := documentCounts[value.Tag]
		articleLabel := "articles"
		if count == 1 {
			articleLabel = "article"
		}
		out.WriteString(`<a hreflang="`)
		out.WriteString(html.EscapeString(value.Tag))
		out.WriteString(`" lang="`)
		out.WriteString(html.EscapeString(value.Tag))
		out.WriteString(`" dir="`)
		out.WriteString(direction)
		out.WriteString(`" href="`)
		out.WriteString(html.EscapeString(wikiURL(value.Tag, value.MainPageName)))
		out.WriteString(`"><strong>`)
		out.WriteString(html.EscapeString(nativeName))
		out.WriteString(`</strong><span>`)
		out.WriteString(html.EscapeString(displayName))
		out.WriteString(`</span><small>`)
		fmt.Fprintf(&out, "%d %s", count, articleLabel)
		out.WriteString(`</small><code>`)
		out.WriteString(html.EscapeString(value.Tag))
		out.WriteString(`</code></a>`)
	}
	out.WriteString(`</div></section><footer class="portal-footer"><span>Computer science and software engineering</span><nav aria-label="Account">`)
	if supportSettings.Enabled && strings.TrimSpace(supportSettings.Title) != "" {
		out.WriteString(`<a href="/support">`)
		out.WriteString(html.EscapeString(supportSettings.Title))
		out.WriteString(`</a>`)
	}
	out.WriteString(`<a href="/login">Log in</a><a href="/signup">Create account</a></nav></footer></main>`)
	return out.String()
}

func notFoundInitialHTML() string {
	return `<main class="not-found-page"><a class="not-found-brand" href="/"><span aria-hidden="true">T</span><strong>TechPedia</strong></a><section class="not-found-content"><p class="not-found-code">404</p><h1>Page not found</h1><p>The requested page does not exist. Check the address or continue from the language portal.</p><a class="not-found-portal-link" href="/">Go to language portal</a></section></main>`
}

func renderIndex(data []byte, metadata indexMetadata) []byte {
	page := string(data)
	if metadata.PortalAssets {
		page = usePortalStylesheet(page)
	}
	lang := metadata.Lang
	if !document.IsSupportedLocale(lang) {
		lang = document.DefaultLocale
	}
	direction := metadata.Direction
	if direction != "rtl" {
		direction = "ltr"
	}
	writingSystem := "Latn"
	if profile, ok := edition.LanguageProfileFor(lang); ok {
		writingSystem = profile.WritingSystem
	}
	page = strings.Replace(page, `<html lang="ko">`, `<html lang="`+html.EscapeString(lang)+`" dir="`+direction+`" data-writing-system="`+html.EscapeString(writingSystem)+`">`, 1)
	page = strings.Replace(page, `<meta name="description" content="`+defaultSiteDescription+`">`, "", 1)
	page = strings.Replace(page, `<title>TechPedia</title>`, `<title>`+html.EscapeString(metadata.Title)+`</title>`, 1)

	var head strings.Builder
	writeMeta(&head, "name", "description", metadata.Description)
	writeMeta(&head, "name", "robots", metadata.Robots)
	writeMeta(
		&head,
		"name",
		"naver-site-verification",
		"b4cc6f39959e6ffdb45b3399c809962f63b22132",
	)
	if metadata.Canonical != "" {
		fmt.Fprintf(&head, `<link rel="canonical" href="%s">`, html.EscapeString(metadata.Canonical))
	}
	for _, alternate := range metadata.Alternates {
		fmt.Fprintf(&head, `<link rel="alternate" hreflang="%s" href="%s">`, html.EscapeString(alternate.Locale), html.EscapeString(alternate.URL))
	}
	if len(metadata.Alternates) > 0 {
		defaultURL := metadata.Canonical
		for _, alternate := range metadata.Alternates {
			if alternate.Locale == document.DefaultLocale {
				defaultURL = alternate.URL
				break
			}
		}
		fmt.Fprintf(&head, `<link rel="alternate" hreflang="x-default" href="%s">`, html.EscapeString(defaultURL))
	}
	if metadata.OpenGraph {
		writeMeta(&head, "property", "og:type", "article")
		writeMeta(&head, "property", "og:site_name", "TechPedia")
		writeMeta(&head, "property", "og:title", metadata.Title)
		writeMeta(&head, "property", "og:description", metadata.Description)
		writeMeta(&head, "property", "og:url", metadata.Canonical)
		writeMeta(&head, "property", "og:locale", openGraphLocale(metadata.Lang))
		for _, alternate := range metadata.Alternates {
			if alternate.Locale != metadata.Lang {
				writeMeta(&head, "property", "og:locale:alternate", openGraphLocale(alternate.Locale))
			}
		}
		writeMeta(&head, "name", "twitter:card", "summary")
		writeMeta(&head, "name", "twitter:title", metadata.Title)
		writeMeta(&head, "name", "twitter:description", metadata.Description)
		if metadata.UpdatedAt != "" {
			writeMeta(&head, "property", "article:modified_time", metadata.UpdatedAt)
		}
	}
	if metadata.JSONLD != "" {
		head.WriteString(`<script type="application/ld+json">`)
		head.WriteString(metadata.JSONLD)
		head.WriteString(`</script>`)
	}
	if metadata.InitialData != "" {
		initialDataID := metadata.InitialDataID
		if initialDataID == "" {
			initialDataID = "techpedia-page-data"
		}
		head.WriteString(`<script id="`)
		head.WriteString(html.EscapeString(initialDataID))
		head.WriteString(`" type="application/json">`)
		head.WriteString(metadata.InitialData)
		head.WriteString(`</script>`)
	}
	page = strings.Replace(page, "</head>", head.String()+"</head>", 1)
	if metadata.InitialHTML != "" {
		page = strings.Replace(page, `<div id="app"></div>`, `<div id="app">`+metadata.InitialHTML+`</div>`, 1)
	}
	return []byte(page)
}

type viteManifestEntry struct {
	CSS []string `json:"css"`
}

var (
	viteManifestOnce sync.Once
	viteManifest     map[string]viteManifestEntry
)

func loadViteManifest() map[string]viteManifestEntry {
	viteManifestOnce.Do(func() {
		data, err := staticFS.ReadFile("static/dist/manifest.json")
		if err != nil {
			return
		}
		_ = json.Unmarshal(data, &viteManifest)
	})
	return viteManifest
}

func usePortalStylesheet(page string) string {
	manifest := loadViteManifest()
	entryCSS := manifest["index.html"].CSS
	portalCSS := manifest["src/views/LanguagePortalView.vue"].CSS
	if len(entryCSS) == 0 || len(portalCSS) == 0 {
		return page
	}
	for _, path := range entryCSS {
		page = strings.ReplaceAll(page, `<link rel="stylesheet" crossorigin href="/`+path+`">`, "")
	}
	var links strings.Builder
	for _, path := range portalCSS {
		links.WriteString(`<link rel="stylesheet" crossorigin href="/`)
		links.WriteString(html.EscapeString(path))
		links.WriteString(`">`)
	}
	return strings.Replace(page, "</head>", links.String()+"</head>", 1)
}

func writeMeta(head *strings.Builder, attribute string, key string, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(head, `<meta %s="%s" content="%s">`, attribute, html.EscapeString(key), html.EscapeString(value))
}

func openGraphLocale(locale string) string {
	if locale == "en" {
		return "en_US"
	}
	if locale == "ko" {
		return "ko_KR"
	}
	parts := strings.Split(locale, "-")
	for _, part := range parts[1:] {
		if len(part) == 2 || (len(part) == 3 && part[0] >= '0' && part[0] <= '9') {
			return strings.ToLower(parts[0]) + "_" + strings.ToUpper(part)
		}
	}
	return ""
}

func (s *Server) absoluteURL(path string) string {
	base, err := url.Parse(s.publicURL + "/")
	if err != nil {
		return s.publicURL + path
	}
	reference, err := url.Parse(path)
	if err != nil {
		return s.publicURL + path
	}
	return base.ResolveReference(reference).String()
}

func (s *Server) handleRobots(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = fmt.Fprintf(w, "User-agent: *\nAllow: /\nDisallow: /api/\nDisallow: /edit/\nDisallow: /admin/\nDisallow: /account\nDisallow: /login\nDisallow: /signup\nSitemap: %s/sitemap.xml\n", s.publicURL)
}

const securityTextExpires = "2027-06-30T00:00:00Z"

func (s *Server) handleSecurityText(w http.ResponseWriter, r *http.Request) {
	lines := []string{
		"Contact: mailto:luna@lunastev.org",
		"Expires: " + securityTextExpires,
		"Preferred-Languages: ko, en",
	}
	if strings.HasPrefix(s.publicURL, "https://") {
		lines = append(lines, "Canonical: "+s.absoluteURL("/.well-known/security.txt"))
	}
	body := []byte(strings.Join(lines, "\n") + "\n")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400, must-revalidate")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if setRevalidationHeaders(w, r, body, time.Time{}) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	XMLNS   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Location string `xml:"loc"`
	LastMod  string `xml:"lastmod,omitempty"`
}

func sitemapLastMod(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Truncate(time.Second).Format(time.RFC3339)
}

func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	root := s.absoluteURL("/")
	entries := map[string]sitemapURL{root: {Location: root}}
	changelog := s.absoluteURL("/changelog")
	entries[changelog] = sitemapURL{Location: changelog}
	settings, err := s.store.GetSupportSettings(r.Context())
	if err != nil {
		http.Error(w, "failed to build sitemap", http.StatusInternalServerError)
		return
	}
	if settings.Enabled {
		location := s.absoluteURL("/support")
		entry := sitemapURL{Location: location}
		if !settings.UpdatedAt.IsZero() {
			entry.LastMod = sitemapLastMod(settings.UpdatedAt)
		}
		entries[location] = entry
	}
	editions, err := s.store.ListEditions(r.Context())
	if err != nil {
		http.Error(w, "failed to build sitemap", http.StatusInternalServerError)
		return
	}
	for _, value := range editions {
		if !value.Active() {
			continue
		}
		locale := value.Tag
		docs, err := s.store.ListAllLatest(r.Context(), locale)
		if err != nil && !isDocumentNotFound(err) {
			http.Error(w, "failed to build sitemap", http.StatusInternalServerError)
			return
		}
		for _, doc := range docs {
			location := s.absoluteURL(wikiURL(doc.Locale, doc.Title))
			entry := sitemapURL{Location: location}
			// Live lists and shared resources have no single document timestamp
			// for their rendered content. Omit the optional date rather than
			// stamping every sitemap request with "now".
			if !techmark.ContainsSpecialSyntax(doc.Source) && !techmark.ContainsBoxSyntax(doc.Source) {
				entry.LastMod = sitemapLastMod(doc.UpdatedAt)
			}
			entries[location] = entry
		}
	}

	locations := make([]string, 0, len(entries))
	for location := range entries {
		locations = append(locations, location)
	}
	sort.Strings(locations)
	urls := make([]sitemapURL, 0, len(locations))
	for _, location := range locations {
		urls = append(urls, entries[location])
	}
	data, err := xml.MarshalIndent(sitemapURLSet{
		XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  urls,
	}, "", "  ")
	if err != nil {
		http.Error(w, "failed to build sitemap", http.StatusInternalServerError)
		return
	}
	body := append([]byte(xml.Header), data...)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300, stale-while-revalidate=60")
	if setRevalidationHeaders(w, r, body, time.Time{}) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(body)
}
