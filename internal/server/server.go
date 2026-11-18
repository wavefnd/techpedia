package server

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"techpedia/internal/analytics"
	"techpedia/internal/auth"
	"techpedia/internal/bot"
	"techpedia/internal/changeproposal"
	"techpedia/internal/config"
	"techpedia/internal/discussion"
	"techpedia/internal/document"
	"techpedia/internal/edition"
	"techpedia/internal/mcpwiki"
	"techpedia/internal/media"
	"techpedia/internal/oauth"
	"techpedia/internal/storage"
	"techpedia/internal/support"
	"techpedia/internal/techdata"
	"techpedia/internal/techmark"
	"techpedia/internal/translation"
	"techpedia/internal/wiki"
)

//go:embed static/*
var staticFS embed.FS

type Server struct {
	store                  Store
	accounts               auth.Backend
	parser                 *techmark.Parser
	publicURL              string
	cspMode                string
	dataDir                string
	location               string
	startedAt              time.Time
	mux                    *http.ServeMux
	graphMu                sync.Mutex
	graphBuildMu           sync.Mutex
	graph                  map[string]graphCacheEntry
	graphGeneration        uint64
	media                  *media.Manager
	cspReportMu            sync.Mutex
	cspReportAt            time.Time
	cspReports             int
	interfaceTranslator    translation.InterfaceTranslator
	analyticsEnabled       bool
	analyticsCountryHeader string
	reader                 *wiki.Reader
	mcpHandler             http.Handler
	oauth                  *oauth.Service
	proposals              changeproposal.Store
	boxes                  storage.BoxStore
	oauthLimitMu           sync.Mutex
	oauthLimitAt           time.Time
	oauthLimitCount        int
}

type Store interface {
	storage.DocumentStore
	discussion.Store
	techdata.Store
	edition.Store
	analytics.Store
	bot.Store
	support.Store
	media.Store
}

func New(cfg config.Config, store Store, accounts auth.Backend, parser *techmark.Parser) *Server {
	publicURL := strings.TrimRight(strings.TrimSpace(cfg.PublicURL), "/")
	if publicURL == "" {
		publicURL = "http://localhost:8080"
	}
	cspMode := "report-only"
	if strings.EqualFold(strings.TrimSpace(cfg.CSPMode), "enforce") {
		cspMode = "enforce"
	}
	mediaRoot := ""
	if strings.TrimSpace(cfg.DataDir) != "" {
		mediaRoot = cfg.MediaDir()
	}
	s := &Server{
		store:     store,
		accounts:  accounts,
		parser:    parser,
		publicURL: publicURL,
		cspMode:   cspMode,
		dataDir:   cfg.DataDir,
		location:  cfg.ServerLocation,
		startedAt: time.Now().UTC(),
		mux:       http.NewServeMux(),
		graph:     make(map[string]graphCacheEntry),
		media: media.NewManager(mediaRoot, store, media.Limits{
			StorageBytes: cfg.MediaStorageLimitBytes,
			UploadBytes:  cfg.MediaUploadLimitBytes,
		}),
		analyticsEnabled:       cfg.AnalyticsEnabled,
		analyticsCountryHeader: strings.TrimSpace(cfg.AnalyticsCountryHeader),
	}
	if cfg.InterfaceAutoTranslation {
		s.interfaceTranslator = translation.NewMyMemory(cfg.InterfaceTranslationURL, cfg.InterfaceTranslationEmail, nil)
	}
	s.reader = wiki.NewReader(store)
	s.boxes, _ = store.(storage.BoxStore)
	s.proposals, _ = store.(changeproposal.Store)
	if cfg.MCPEnabled {
		var proposer *wiki.Proposer
		if s.proposals != nil {
			proposer = wiki.NewProposer(s.proposals)
		}
		handler, err := mcpwiki.NewHandler(s.reader, publicURL, proposer)
		if err != nil {
			log.Print("MCP disabled: invalid public origin configuration")
		} else {
			if oauthStore, ok := store.(oauth.Store); ok && oauthSafeOrigin(publicURL) {
				s.oauth = oauth.New(oauthStore, publicURL)
				s.mcpHandler = s.authorizeMCP(handler)
			}
		}
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return logRequests(securityHeaders(s.cspMode, s.publicURL, http.HandlerFunc(s.dispatch)))
}

func (s *Server) dispatch(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/mcp" {
		if s.mcpHandler == nil {
			http.NotFound(w, r)
			return
		}
		s.mcpHandler.ServeHTTP(w, r)
		return
	}
	if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
		s.mux.ServeHTTP(w, r)
		return
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		path := strings.Trim(r.URL.Path, "/")
		parts := strings.SplitN(path, "/", 3)
		if len(parts) == 3 && parts[1] == "special" && parts[2] == "random" {
			if locale, ok := document.CanonicalLocale(parts[0]); ok {
				r.SetPathValue("locale", locale)
				s.handleRandomPage(w, r)
				return
			}
		}
		if len(parts) == 2 && parts[1] == "graph" {
			if locale, ok := document.CanonicalLocale(parts[0]); ok {
				if value, err := s.store.GetEdition(r.Context(), locale); err == nil && value.Active() {
					s.handleIndex(w, r)
					return
				}
			}
		}
		if len(parts) == 2 && parts[1] == "search" {
			if locale, ok := document.CanonicalLocale(parts[0]); ok {
				if value, err := s.store.GetEdition(r.Context(), locale); err == nil && value.Active() {
					s.handleSearchIndex(w, r, locale)
					return
				}
			}
		}
		if len(parts) >= 2 && parts[1] == "wiki" {
			if locale, ok := document.CanonicalLocale(parts[0]); ok {
				name := ""
				if len(parts) == 3 {
					name = parts[2]
				}
				r.SetPathValue("locale", locale)
				r.SetPathValue("name", name)
				s.handleLocalizedIndex(w, r)
				return
			}
		}
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.oauthRoutes()
	s.boxRoutes()
	if dist, err := fs.Sub(staticFS, "static/dist"); err == nil {
		assets := http.FileServer(http.FS(dist))
		s.mux.Handle("GET /assets/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			name := strings.TrimPrefix(r.URL.Path, "/")
			info, statErr := fs.Stat(dist, name)
			if statErr != nil || info.IsDir() {
				w.Header().Set("Cache-Control", "no-store")
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			w.Header().Set("X-Robots-Tag", "noindex,noarchive")
			assets.ServeHTTP(w, r)
		}))
		s.mux.Handle("GET /fonts/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			name := strings.TrimPrefix(r.URL.Path, "/")
			info, statErr := fs.Stat(dist, name)
			if statErr != nil || info.IsDir() {
				w.Header().Set("Cache-Control", "no-store")
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			w.Header().Set("X-Robots-Tag", "noindex,noarchive")
			assets.ServeHTTP(w, r)
		}))
		s.mux.Handle("GET /favicon.svg", http.FileServer(http.FS(dist)))
	}

	s.mux.HandleFunc("GET /{$}", s.handleLanguagePortal)
	s.mux.HandleFunc("GET /support", s.handleSupportIndex)
	s.mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/favicon.svg", http.StatusPermanentRedirect)
	})
	s.mux.HandleFunc("GET /robots.txt", s.handleRobots)
	s.mux.HandleFunc("GET /sitemap.xml", s.handleSitemap)
	s.mux.HandleFunc("GET /.well-known/security.txt", s.handleSecurityText)
	s.mux.HandleFunc("GET /security.txt", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/.well-known/security.txt", http.StatusPermanentRedirect)
	})
	s.mux.HandleFunc("GET /login", s.handleIndex)
	s.mux.HandleFunc("GET /signup", s.handleIndex)
	s.mux.HandleFunc("GET /account", s.handleIndex)
	s.mux.HandleFunc("GET /user/{username}", s.handleUserProfileIndex)
	s.mux.HandleFunc("GET /changelog", s.handleChangelogIndex)
	s.mux.HandleFunc("GET /graph", s.handleIndex)
	s.mux.HandleFunc("GET /history/{locale}/{name}", s.handleIndex)
	s.mux.HandleFunc("GET /edit/{locale}/{name}", s.handleIndex)
	s.mux.HandleFunc("GET /admin/techdata/{tid}", s.handleIndex)
	s.mux.HandleFunc("GET /admin/techdata-audit", s.handleIndex)
	s.mux.HandleFunc("GET /admin/trash", s.handleIndex)
	s.mux.HandleFunc("GET /admin/audit", s.handleIndex)
	s.mux.HandleFunc("GET /admin/support", s.handleIndex)
	s.mux.HandleFunc("GET /admin/users", s.handleIndex)
	s.mux.HandleFunc("GET /admin/pages", s.handleIndex)
	s.mux.HandleFunc("GET /admin/discussions", s.handleIndex)
	s.mux.HandleFunc("GET /admin/bots", s.handleIndex)
	s.mux.HandleFunc("GET /admin/server", s.handleIndex)
	s.mux.HandleFunc("GET /admin/analytics", s.handleIndex)
	s.mux.HandleFunc("GET /languages", s.handleIndex)
	s.mux.HandleFunc("GET /admin/languages", s.handleIndex)
	s.mux.HandleFunc("GET /admin", s.handleIndex)

	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/config", s.handlePublicConfig)
	s.mux.HandleFunc("POST /api/csp-reports", s.handleCSPReport)
	s.mux.HandleFunc("GET /api/analytics/ping", s.handleAnalyticsPing)
	s.mux.HandleFunc("POST /api/analytics/visits", s.handleRecordAnalyticsVisit)
	s.mux.HandleFunc("GET /api/support", s.handleGetSupportSettings)
	s.mux.HandleFunc("GET /api/search", s.handleSearch)
	s.mux.HandleFunc("GET /api/languages", s.handleListEditions)
	s.mux.HandleFunc("GET /api/language-catalog", s.handleLanguageCatalog)
	s.mux.HandleFunc("GET /api/country-catalog", s.handleCountryCatalog)
	s.mux.HandleFunc("GET /api/links/resolve", s.handleResolveLinks)
	s.mux.HandleFunc("GET /api/graph", s.handleGraph)
	s.mux.HandleFunc("GET /api/pages/{locale}/{name}/backlinks", s.handlePageBacklinks)
	s.mux.HandleFunc("POST /api/pages/{locale}/{name}/views", s.handleRecordPageView)
	s.mux.HandleFunc("GET /api/pages/{locale}/{name}/revisions/{revision}", s.handleGetRevision)
	s.mux.HandleFunc("GET /api/pages/{locale}/{name}/revisions", s.handleListRevisions)
	s.mux.HandleFunc("GET /api/pages/{locale}/{name}/diff", s.handleRevisionDiff)
	s.mux.HandleFunc("GET /api/pages/{locale}/{name}", s.handleGetPage)
	s.mux.HandleFunc("GET /api/recent-changes", s.handleRecentChanges)
	s.mux.HandleFunc("GET /api/account/contributions", s.handleAccountContributions)
	s.mux.HandleFunc("PATCH /api/account/profile", s.handleUpdateProfile)
	s.mux.HandleFunc("GET /api/users/{username}", s.handleGetUserProfile)
	s.mux.HandleFunc("GET /media/files/{name}/{variant}", s.handleMediaFile)
	s.mux.HandleFunc("POST /api/auth/signup", s.handleSignup)
	s.mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	s.mux.HandleFunc("GET /api/auth/session", s.handleSession)
	s.mux.HandleFunc("GET /api/discussions/{locale}/{name}/topics", s.handleListDiscussionTopics)
	s.mux.HandleFunc("POST /api/discussions/{locale}/{name}/topics", s.handleCreateDiscussionTopic)
	s.mux.HandleFunc("GET /api/discussions/topics/{topicID}", s.handleGetDiscussionThread)
	s.mux.HandleFunc("POST /api/discussions/topics/{topicID}/replies", s.handleCreateDiscussionReply)
	s.mux.HandleFunc("POST /api/discussions/topics/{topicID}/archive", s.handleArchiveDiscussionTopic)
	s.mux.HandleFunc("DELETE /api/discussions/topics/{topicID}", s.handleDeleteDiscussionTopic)
	s.mux.HandleFunc("DELETE /api/discussions/topics/{topicID}/replies/{replyID}", s.handleDeleteDiscussionReply)
	s.mux.HandleFunc("GET /api/editor/pages/{locale}/{name}", s.handleGetAdminPage)
	s.mux.HandleFunc("PUT /api/editor/pages/{locale}/{name}", s.handleSavePage)
	s.mux.HandleFunc("GET /api/editor/pages/{locale}/{name}/draft", s.handleGetDraft)
	s.mux.HandleFunc("PUT /api/editor/pages/{locale}/{name}/draft", s.handleSaveDraft)
	s.mux.HandleFunc("DELETE /api/editor/pages/{locale}/{name}/draft", s.handleDeleteDraft)
	s.mux.HandleFunc("POST /api/editor/pages/{locale}/{name}/restore", s.handleRestoreRevision)
	s.mux.HandleFunc("POST /api/editor/preview", s.handlePreview)
	s.mux.HandleFunc("GET /api/editor/media", s.handleSearchMedia)
	s.mux.HandleFunc("POST /api/editor/media", s.handleUploadMedia)
	s.mux.HandleFunc("DELETE /api/admin/media/{name}", s.handleDeleteMedia)
	s.mux.HandleFunc("POST /api/editor/languages", s.handleProposeEdition)
	s.mux.HandleFunc("GET /api/admin/techdata/items/{tid}", s.handleGetTechdataItem)
	s.mux.HandleFunc("DELETE /api/admin/techdata/items/{tid}", s.handleDeleteTechdataItem)
	s.mux.HandleFunc("GET /api/admin/techdata/items", s.handleListTechdataItems)
	s.mux.HandleFunc("GET /api/admin/techdata/audit", s.handleAdminTechdataAudit)
	s.mux.HandleFunc("GET /api/admin/support", s.handleGetAdminSupportSettings)
	s.mux.HandleFunc("PUT /api/admin/support", s.handleSaveAdminSupportSettings)
	s.mux.HandleFunc("GET /api/admin/users", s.handleListUsers)
	s.mux.HandleFunc("PATCH /api/admin/users/regular-permissions", s.handleSetRegularUserPermissions)
	s.mux.HandleFunc("GET /api/admin/dashboard", s.handleAdminDashboard)
	s.mux.HandleFunc("GET /api/admin/server", s.handleAdminServerInfo)
	s.mux.HandleFunc("GET /api/admin/analytics", s.handleAdminAnalytics)
	s.mux.HandleFunc("GET /api/admin/pages", s.handleAdminPages)
	s.mux.HandleFunc("GET /api/admin/redirects", s.handleListDocumentRedirects)
	s.mux.HandleFunc("POST /api/admin/redirects", s.handleCreateDocumentRedirect)
	s.mux.HandleFunc("DELETE /api/admin/redirects/{locale}/{name}", s.handleDeleteDocumentRedirect)
	s.mux.HandleFunc("GET /api/admin/pages/{locale}/{name}", s.handleAdminPage)
	s.mux.HandleFunc("DELETE /api/admin/pages/{locale}/{name}", s.handleDeletePage)
	s.mux.HandleFunc("GET /api/admin/trash", s.handleListTrash)
	s.mux.HandleFunc("POST /api/admin/trash/{wid}/restore", s.handleRestoreDeletedPage)
	s.mux.HandleFunc("GET /api/admin/audit", s.handleListAudit)
	s.mux.HandleFunc("GET /api/admin/links/report", s.handleAdminLinkReport)
	s.mux.HandleFunc("POST /api/admin/pages/{locale}/{name}/move", s.handleMovePage)
	s.mux.HandleFunc("POST /api/admin/pages/{locale}/{name}/history/reset", s.handleResetDocumentHistory)
	s.mux.HandleFunc("POST /api/admin/pages/{locale}/{name}/techdata", s.handleLinkPageTechdata)
	s.mux.HandleFunc("DELETE /api/admin/pages/{locale}/{name}/techdata", s.handleUnlinkPageTechdata)
	s.mux.HandleFunc("GET /api/admin/discussions/topics", s.handleAdminDiscussionTopics)
	s.mux.HandleFunc("DELETE /api/admin/discussions/topics/{topicID}", s.handleAdminDeleteDiscussionTopic)
	s.mux.HandleFunc("POST /api/admin/bots/naming/scan", s.handleScanNamingIndex)
	s.mux.HandleFunc("GET /api/admin/bots/naming/suggestions", s.handleListNamingSuggestions)
	s.mux.HandleFunc("POST /api/admin/bots/naming/suggestions/{suggestionID}/accept", s.handleAcceptNamingSuggestion)
	s.mux.HandleFunc("POST /api/admin/bots/naming/suggestions/{suggestionID}/reject", s.handleRejectNamingSuggestion)
	s.mux.HandleFunc("PATCH /api/admin/users/{userID}/permissions", s.handleSetUserPermissions)
	s.mux.HandleFunc("POST /api/admin/languages/{locale}/approve", s.handleApproveEdition)
	s.mux.HandleFunc("DELETE /api/admin/languages/{locale}", s.handleRejectEdition)
	s.mux.HandleFunc("POST /api/admin/languages/{locale}/activate", s.handleActivateEdition)
	s.mux.HandleFunc("PATCH /api/admin/languages/{locale}/interface", s.handleUpdateEditionInterface)
	s.mux.HandleFunc("POST /api/admin/languages/{locale}/interface/translate", s.handleTranslateEditionInterface)
	s.mux.HandleFunc("PATCH /api/admin/languages/{locale}/navigation", s.handleUpdateEditionNavigation)
	s.mux.HandleFunc("PATCH /api/admin/languages/{locale}/portal", s.handleUpdateEditionPortal)

	s.mux.HandleFunc("GET /", s.handleNotFoundIndex)
}

func (s *Server) handleLocaleHomeRedirect(w http.ResponseWriter, r *http.Request) {
	locale, ok := requestLocale(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	value, err := s.store.GetEdition(r.Context(), locale)
	if err != nil || !value.Active() {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, wikiURL(locale, value.MainPageName), http.StatusTemporaryRedirect)
}

func (s *Server) handleLocalizedIndex(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.PathValue("name")) == "" {
		s.handleLocaleHomeRedirect(w, r)
		return
	}
	locale, ok := requestLocale(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	value, err := s.store.GetEdition(r.Context(), locale)
	if err != nil || !value.Active() {
		metadata := s.inactiveEditionMetadata(locale, r.URL.Path)
		if err == nil {
			metadata.Direction = value.Direction
		}
		s.writeIndex(w, r, http.StatusNotFound, metadata)
		return
	}
	s.handleArticleIndex(w, r, locale)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	s.writeIndex(w, r, http.StatusOK, defaultIndexMetadata())
}

func (s *Server) handleChangelogIndex(w http.ResponseWriter, r *http.Request) {
	s.writeIndex(w, r, http.StatusOK, indexMetadata{
		Lang:        "ko",
		Direction:   "ltr",
		Title:       "TechPedia 변경 기록",
		Description: "TechPedia Engine의 버전별 변경 내역입니다.",
		Canonical:   s.absoluteURL("/changelog"),
		Robots:      "index,follow",
	})
}

func (s *Server) handleSearchIndex(w http.ResponseWriter, r *http.Request, locale string) {
	value, err := s.store.GetEdition(r.Context(), locale)
	if err != nil || !value.Active() {
		s.handleNotFoundIndex(w, r)
		return
	}
	title := strings.TrimSpace(value.InterfaceMessages["search"])
	if title == "" {
		title = "Search"
	}
	description := strings.TrimSpace(value.InterfaceMessages["sidebarNote"])
	if description == "" {
		description = defaultSiteDescription
	}
	s.writeIndex(w, r, http.StatusOK, indexMetadata{
		Lang:        locale,
		Direction:   value.Direction,
		Title:       title + " - TechPedia",
		Description: description,
		Robots:      "noindex,follow",
	})
}

func (s *Server) handleNotFoundIndex(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/assets/") {
		http.NotFound(w, r)
		return
	}
	s.writeIndex(w, r, http.StatusNotFound, indexMetadata{
		Lang:        "en",
		Direction:   "ltr",
		Title:       "Page not found - TechPedia",
		Description: "The requested TechPedia page does not exist or may have moved.",
		Robots:      "noindex,follow,noarchive",
		InitialHTML: notFoundInitialHTML(),
	})
}

func (s *Server) handleLanguagePortal(w http.ResponseWriter, r *http.Request) {
	editions, err := s.store.ListEditions(r.Context())
	if err != nil {
		http.Error(w, "failed to load language editions", http.StatusInternalServerError)
		return
	}
	counts, err := s.store.CountLatestByLocale(r.Context())
	if err != nil {
		http.Error(w, "failed to count language edition documents", http.StatusInternalServerError)
		return
	}
	supportSettings, err := s.store.GetSupportSettings(r.Context())
	if err != nil {
		http.Error(w, "failed to load support settings", http.StatusInternalServerError)
		return
	}
	publicSupport := publicSupportSettingsResponse(supportSettings)
	s.writeIndex(w, r, http.StatusOK, indexMetadata{
		Lang:          "en",
		Direction:     "ltr",
		Title:         "TechPedia - Technical knowledge wiki",
		Description:   "TechPedia is a multilingual technical wiki for computer science and software engineering.",
		Canonical:     s.absoluteURL("/"),
		Robots:        "index,follow",
		JSONLD:        s.websiteJSONLD(),
		InitialHTML:   languagePortalInitialHTML(editions, counts, publicSupport),
		InitialData:   languagePortalInitialData(editions, counts, publicSupport),
		InitialDataID: "techpedia-portal-data",
		PortalAssets:  true,
	})
}

func (s *Server) writeIndex(w http.ResponseWriter, r *http.Request, status int, metadata indexMetadata) {
	data, err := staticFS.ReadFile("static/dist/index.html")
	if err != nil {
		data, err = staticFS.ReadFile("static/index.html")
	}
	if err != nil {
		http.Error(w, "index not found", http.StatusInternalServerError)
		return
	}

	body := renderIndex(data, metadata)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if document.IsSupportedLocale(metadata.Lang) {
		w.Header().Set("Content-Language", metadata.Lang)
	}
	indexable := status == http.StatusOK && strings.HasPrefix(metadata.Robots, "index")
	if !indexable {
		w.Header().Set("Cache-Control", "no-store")
		if metadata.Robots != "" {
			w.Header().Set("X-Robots-Tag", metadata.Robots)
		}
	} else {
		w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
		// Article dates do not cover changes to templates, navigation, Techdata
		// or live lists. Only the complete representation can validate HTML.
		if setRevalidationHeaders(w, r, body, time.Time{}) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	if !s.allowSessionRequest(w, r, true) {
		return
	}
	var req signupRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	_, err := s.accounts.Register(r.Context(), auth.Registration{
		Username:              req.Username,
		DisplayName:           req.DisplayName,
		Password:              req.Password,
		UseRegularPermissions: true,
	})
	if errors.Is(err, auth.ErrBusy) {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "authentication temporarily busy", http.StatusTooManyRequests)
		return
	}
	if errors.Is(err, auth.ErrUsernameTaken) {
		http.Error(w, "username already exists", http.StatusConflict)
		return
	}
	if errors.Is(err, auth.ErrInvalidRegistration) {
		http.Error(w, "username or password does not meet the requirements", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	user, token, expiresAt, err := s.accounts.Login(r.Context(), req.Username, req.Password)
	if errors.Is(err, auth.ErrBusy) {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "account created; authentication temporarily busy, sign in after retrying", http.StatusTooManyRequests)
		return
	}
	if err != nil {
		http.Error(w, "account created but login failed", http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, r, token, expiresAt, s.publicURL)
	writeJSON(w, http.StatusCreated, newAccountResponse(user))
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.allowSessionRequest(w, r, true) {
		return
	}
	var req loginRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	user, token, expiresAt, err := s.accounts.Login(r.Context(), req.Username, req.Password)
	if errors.Is(err, auth.ErrBusy) {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "authentication temporarily busy", http.StatusTooManyRequests)
		return
	}
	if errors.Is(err, auth.ErrInvalidCredentials) {
		http.Error(w, "invalid username or password", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, r, token, expiresAt, s.publicURL)
	writeJSON(w, http.StatusOK, newAccountResponse(user))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if !s.allowSessionRequest(w, r, false) {
		return
	}
	token := sessionToken(r)
	if err := s.accounts.Logout(r.Context(), token); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	clearSessionCookie(w, r, s.publicURL)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "login required", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, newAccountResponse(user))
}

func (s *Server) handleUserProfileIndex(w http.ResponseWriter, r *http.Request) {
	profile, err := s.accounts.GetProfile(r.Context(), r.PathValue("username"))
	if errors.Is(err, auth.ErrUserNotFound) {
		s.handleNotFoundIndex(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if r.PathValue("username") != profile.Username {
		http.Redirect(w, r, "/user/"+profile.Username, http.StatusPermanentRedirect)
		return
	}
	s.writeIndex(w, r, http.StatusOK, indexMetadata{
		Lang:        "en",
		Direction:   "ltr",
		Title:       profile.DisplayName + " (@" + profile.Username + ") - TechPedia",
		Description: "TechPedia user profile for @" + profile.Username + ".",
		Canonical:   s.absoluteURL("/user/" + profile.Username),
		Robots:      "noindex,follow",
	})
}

func (s *Server) handleGetUserProfile(w http.ResponseWriter, r *http.Request) {
	profile, err := s.accounts.GetProfile(r.Context(), r.PathValue("username"))
	if errors.Is(err, auth.ErrUserNotFound) {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	changes, err := s.store.ListChanges(r.Context(), document.ChangeFilter{Author: profile.Username, Limit: 100})
	if err != nil {
		http.Error(w, "failed to load user contributions", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, publicProfileResponse{
		Username:      profile.Username,
		DisplayName:   profile.DisplayName,
		Bio:           profile.Bio,
		CreatedAt:     profile.CreatedAt.Format(time.RFC3339),
		Contributions: publicChanges(changes),
	})
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	current, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "login required", http.StatusUnauthorized)
		return
	}
	var req profileUpdateRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	updated, err := s.accounts.UpdateProfile(r.Context(), current.ID, auth.ProfileUpdate{
		DisplayName: req.DisplayName,
		Bio:         req.Bio,
	})
	if errors.Is(err, auth.ErrInvalidProfile) {
		http.Error(w, "display name or bio does not meet the profile requirements", http.StatusBadRequest)
		return
	}
	if errors.Is(err, auth.ErrUserNotFound) {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, newAccountResponse(updated))
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, auth.PermissionAdmin); !ok {
		return
	}
	users, err := s.accounts.ListUsers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	permissions, err := s.accounts.RegularUserPermissions(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"users":               users,
		"regular_permissions": permissions,
	})
}

func (s *Server) handleSetUserPermissions(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, auth.PermissionAdmin)
	if !ok {
		return
	}
	var req permissionUpdateRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !validPermissions(req.Permissions) || containsPermission(req.Permissions, auth.PermissionOwner) {
		http.Error(w, "invalid permission", http.StatusBadRequest)
		return
	}
	users, err := s.accounts.ListUsers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var target auth.User
	targetFound := false
	for _, user := range users {
		if user.ID == r.PathValue("userID") {
			target = user
			targetFound = true
			break
		}
	}
	if !targetFound {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	if target.Has(auth.PermissionOwner) {
		http.Error(w, "owner permissions cannot be changed", http.StatusConflict)
		return
	}
	wantsAdmin := containsPermission(req.Permissions, auth.PermissionAdmin)
	isAdmin := target.Has(auth.PermissionAdmin)
	if target.ID == actor.ID && isAdmin && !wantsAdmin {
		http.Error(w, "administrators cannot remove their own administrator permission", http.StatusConflict)
		return
	}
	if !wantsAdmin && !isAdmin {
		http.Error(w, "regular user permissions are managed by the global policy", http.StatusConflict)
		return
	}
	permissions, err := s.accounts.RegularUserPermissions(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if wantsAdmin {
		permissions = append(permissions, auth.PermissionAdmin)
	}
	user, err := s.accounts.SetPermissions(r.Context(), r.PathValue("userID"), permissions)
	if errors.Is(err, auth.ErrUserNotFound) {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	if errors.Is(err, auth.ErrOwnerImmutable) {
		http.Error(w, "owner permissions cannot be changed", http.StatusConflict)
		return
	}
	if err != nil {
		s.recordAudit(r.Context(), actor.Username, "user.permissions", "user", r.PathValue("userID"), err.Error(), "failed")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.recordAudit(r.Context(), actor.Username, "user.permissions", "user", user.ID, permissionsDetail(user.Permissions), "success")
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleSetRegularUserPermissions(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requirePermission(w, r, auth.PermissionAdmin)
	if !ok {
		return
	}
	var req permissionUpdateRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !validPermissions(req.Permissions) || containsPermission(req.Permissions, auth.PermissionAdmin) || containsPermission(req.Permissions, auth.PermissionOwner) {
		http.Error(w, "regular user policy supports edit and techdata permissions only", http.StatusBadRequest)
		return
	}
	updated, err := s.accounts.SetRegularUserPermissions(r.Context(), req.Permissions)
	if errors.Is(err, auth.ErrInvalidPermissions) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err != nil {
		s.recordAudit(r.Context(), actor.Username, "user.regular_permissions", "user_policy", "regular-users", err.Error(), "failed")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	permissions, err := s.accounts.RegularUserPermissions(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.recordAudit(r.Context(), actor.Username, "user.regular_permissions", "user_policy", "regular-users", permissionsDetail(permissions), "success")
	writeJSON(w, http.StatusOK, regularPermissionUpdateResponse{Permissions: permissions, UpdatedUsers: updated})
}

func permissionsDetail(permissions []auth.Permission) string {
	values := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		values = append(values, string(permission))
	}
	return strings.Join(values, ",")
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	locale, ok := queryLocale(r)
	if !ok {
		http.Error(w, "invalid language tag", http.StatusBadRequest)
		return
	}
	limit := parseSearchLimit(r.URL.Query().Get("limit"))
	result, err := s.reader.Search(r.Context(), locale, query, limit)
	if err != nil {
		writeReadError(w, err)
		return
	}
	publicResults := make([]publicSearchResult, 0, len(result.Results))
	for _, entry := range result.Results {
		publicResults = append(publicResults, newPublicSearchResult(entry))
	}

	response := publicSearchResponse{
		Query:   query,
		Results: publicResults,
	}
	if query != "" && !result.ExactMatch {
		response.Create = &publicCreateLink{
			Locale: locale,
			Name:   query,
			URL:    editorURL(locale, query),
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleResolveLinks(w http.ResponseWriter, r *http.Request) {
	locale, ok := queryLocale(r)
	if !ok {
		http.Error(w, "invalid language tag", http.StatusBadRequest)
		return
	}
	if !s.requireActiveEdition(w, r, locale) {
		return
	}
	rawTargets := r.URL.Query()["target"]
	if len(rawTargets) == 0 {
		rawTargets = strings.Split(r.URL.Query().Get("targets"), ",")
	}

	links := make([]publicResolvedLink, 0, len(rawTargets))
	for _, raw := range rawTargets {
		requestedName := strings.TrimSpace(strings.ReplaceAll(raw, "_", " "))
		name := techmark.WikiLinkTargetName(requestedName)
		slug := techmark.Slugify(name)
		if slug == "" {
			continue
		}

		resolved, err := s.resolveLink(r.Context(), locale, slug, name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resolved.Target = requestedName
		links = append(links, resolved)
	}

	writeJSON(w, http.StatusOK, publicLinkResolveResponse{
		Locale: locale,
		Links:  links,
	})
}

func (s *Server) handleGetTranslations(w http.ResponseWriter, r *http.Request) {
	group := strings.TrimSpace(r.PathValue("group"))
	if group == "" {
		http.Error(w, "translation group required", http.StatusBadRequest)
		return
	}

	translations, err := s.translationsForGroup(r.Context(), group)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, document.NewTranslationResponse(group, translations))
}

func (s *Server) handleSavePage(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionEdit)
	if !ok {
		return
	}

	locale, slug, ok := pageLocaleSlug(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !s.requireActiveEdition(w, r, locale) {
		return
	}
	if slug == "" {
		http.Error(w, "invalid slug", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var req adminSavePageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(req.Name)
	if title == "" {
		title = strings.ReplaceAll(r.PathValue("name"), "_", " ")
	}

	var created document.Document
	existing := false
	old, err := s.store.GetLatest(r.Context(), locale, slug)
	if err == nil {
		created = old
		existing = true
	} else if !errors.Is(err, storage.ErrDocumentNotFound) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var translationSource document.Document
	if req.TranslationSource != nil {
		sourceLocale, valid := document.CanonicalLocale(req.TranslationSource.Locale)
		sourceName := strings.TrimSpace(req.TranslationSource.Name)
		if !valid || sourceName == "" || sourceLocale == locale {
			http.Error(w, "invalid translation source", http.StatusBadRequest)
			return
		}
		if existing {
			http.Error(w, "translation target document already exists", http.StatusConflict)
			return
		}
		translationSource, err = s.store.GetLatest(r.Context(), sourceLocale, techmark.Slugify(sourceName))
		if errors.Is(err, storage.ErrDocumentNotFound) {
			http.Error(w, "translation source document not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "failed to load translation source", http.StatusInternalServerError)
			return
		}
		if translationSource.ID == "" {
			http.Error(w, "translation source has no document identity", http.StatusConflict)
			return
		}
	}

	requestedPolicy := req.EditPolicy
	if requestedPolicy != "" && !document.IsValidEditPolicy(requestedPolicy) {
		http.Error(w, "invalid edit policy", http.StatusBadRequest)
		return
	}
	editPolicy := document.NormalizeEditPolicy(created.EditPolicy)
	if existing && editPolicy == document.EditPolicyAdmins && !user.Has(auth.PermissionAdmin) {
		http.Error(w, "administrator permission required", http.StatusForbidden)
		return
	}
	if requestedPolicy != "" && requestedPolicy != editPolicy {
		if !user.Has(auth.PermissionAdmin) {
			http.Error(w, "administrator permission required to change edit policy", http.StatusForbidden)
			return
		}
		editPolicy = requestedPolicy
	}
	if techmark.ContainsSpecialSyntax(req.Source) {
		if !user.Has(auth.PermissionAdmin) {
			http.Error(w, "administrator permission required for special TechMark syntax", http.StatusForbidden)
			return
		}
		if editPolicy != document.EditPolicyAdmins {
			http.Error(w, "special TechMark syntax requires an administrator-only document", http.StatusBadRequest)
			return
		}
	}
	if techmark.ContainsHomePageSyntax(req.Source) {
		isMainPage, err := s.isEditionMainPage(r.Context(), locale, slug)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !isMainPage {
			http.Error(w, "homepage TechMark syntax is only allowed on the edition main page", http.StatusBadRequest)
			return
		}
	}

	// Bind authorization to the observed snapshot, not a client-predicted revision.
	if strings.TrimSpace(req.BaseRevision) != created.Revision {
		if !existing {
			http.Error(w, "document changed while editing", http.StatusConflict)
		} else {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "document changed while editing", "current": adminPageResponse{Locale: created.Locale, Name: created.Title, Source: created.Source, EditPolicy: created.EditPolicy, Revision: created.Revision, UpdatedAt: created.UpdatedAt.UTC().Format(time.RFC3339)}})
		}
		return
	}
	group := created.Group
	if group == "" {
		group = slug
	}

	doc := document.Document{
		ID:         created.ID,
		Locale:     locale,
		Group:      group,
		Slug:       slug,
		Title:      title,
		Source:     req.Source,
		HTML:       s.parser.RenderWithLocale(req.Source, locale),
		Author:     user.Username,
		Summary:    strings.TrimSpace(req.Summary),
		EditPolicy: editPolicy,
		CreatedAt:  created.CreatedAt,
	}

	var saved document.Document
	if req.TranslationSource != nil {
		saved, err = s.store.SaveTranslation(r.Context(), doc, translationSource.ID)
	} else {
		saved, err = s.store.SaveIfRevision(r.Context(), doc, created.Revision)
	}
	if errors.Is(err, storage.ErrRevisionConflict) {
		latest, latestErr := s.store.GetLatest(r.Context(), locale, slug)
		if latestErr != nil {
			http.Error(w, "document changed while editing", http.StatusConflict)
			return
		}
		if latest.EditPolicy == document.EditPolicyAdmins && !user.Has(auth.PermissionAdmin) {
			http.Error(w, "administrator permission required", http.StatusForbidden)
			return
		}
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   "document changed while editing",
			"current": adminPageResponse{Locale: latest.Locale, Name: latest.Title, Source: latest.Source, EditPolicy: latest.EditPolicy, Revision: latest.Revision, UpdatedAt: latest.UpdatedAt.UTC().Format(time.RFC3339)},
		})
		return
	}
	if errors.Is(err, storage.ErrDocumentExists) {
		http.Error(w, "translation target document already exists", http.StatusConflict)
		return
	}
	if errors.Is(err, techdata.ErrLocaleOccupied) || errors.Is(err, techdata.ErrWikiPageLinked) {
		http.Error(w, "translation language is already linked", http.StatusConflict)
		return
	}
	if writeTechdataDuplicateConflict(w, err) {
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, adminPageResponse{
		Locale:     saved.Locale,
		Name:       saved.Title,
		Source:     saved.Source,
		EditPolicy: saved.EditPolicy,
		Revision:   saved.Revision,
		UpdatedAt:  saved.UpdatedAt.UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleGetAdminPage(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionEdit)
	if !ok {
		return
	}
	locale, slug, ok := pageLocaleSlug(r)
	if !ok {
		http.Error(w, "invalid locale", http.StatusBadRequest)
		return
	}
	if !s.requireActiveEdition(w, r, locale) {
		return
	}

	doc, err := s.store.GetLatest(r.Context(), locale, slug)
	if errors.Is(err, storage.ErrDocumentNotFound) {
		if systemDoc, exists, systemErr := s.systemPage(r.Context(), locale, slug); exists {
			if systemErr != nil {
				http.Error(w, systemErr.Error(), http.StatusInternalServerError)
				return
			}
			doc = systemDoc
			err = nil
		}
	}
	if errors.Is(err, storage.ErrDocumentNotFound) {
		name := strings.TrimSpace(strings.ReplaceAll(r.PathValue("name"), "_", " "))
		writeJSON(w, http.StatusOK, adminPageResponse{Locale: locale, Name: name, Source: "", EditPolicy: document.EditPolicyEditors})
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	doc = document.NormalizeDocument(doc)
	if doc.EditPolicy == document.EditPolicyAdmins && !user.Has(auth.PermissionAdmin) {
		http.Error(w, "administrator permission required", http.StatusForbidden)
		return
	}
	revision := doc.Revision
	if revision == "system" {
		revision = ""
	}
	writeJSON(w, http.StatusOK, adminPageResponse{
		Locale:     doc.Locale,
		Name:       doc.Title,
		Source:     doc.Source,
		EditPolicy: doc.EditPolicy,
		Revision:   revision,
		UpdatedAt:  doc.UpdatedAt.UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleMovePage(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionAdmin)
	if !ok {
		return
	}
	locale, oldSlug, ok := pageLocaleSlug(r)
	if !ok || oldSlug == "" {
		http.Error(w, "invalid document name", http.StatusBadRequest)
		return
	}
	if !s.requireActiveEdition(w, r, locale) {
		return
	}
	isMainPage, err := s.isEditionMainPage(r.Context(), locale, oldSlug)
	if err != nil {
		http.Error(w, "failed to inspect language edition", http.StatusInternalServerError)
		return
	}
	if isMainPage {
		http.Error(w, "language edition main page cannot be moved", http.StatusConflict)
		return
	}
	var req adminMovePageRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	newTitle := strings.TrimSpace(req.Name)
	newSlug := techmark.Slugify(newTitle)
	if newTitle == "" || newSlug == "" {
		http.Error(w, "new document name must be different", http.StatusBadRequest)
		return
	}
	moved, err := s.store.MoveDocumentAs(r.Context(), locale, oldSlug, newSlug, newTitle, user.Username)
	if errors.Is(err, storage.ErrDocumentNotFound) {
		http.Error(w, "document not found", http.StatusNotFound)
		return
	}
	if errors.Is(err, storage.ErrDocumentExists) {
		http.Error(w, "target document already exists", http.StatusConflict)
		return
	}
	if err != nil {
		s.recordAudit(r.Context(), user.Username, "document.move", "document", locale+":"+oldSlug, newTitle, "failed")
		http.Error(w, "failed to move document", http.StatusInternalServerError)
		return
	}
	s.recordAudit(r.Context(), user.Username, "document.move", "document", locale+":"+oldSlug, newTitle, "success")
	writeJSON(w, http.StatusOK, adminPageResponse{
		Locale: moved.Locale, Name: moved.Title, Source: moved.Source,
		EditPolicy: moved.EditPolicy, Revision: moved.Revision, UpdatedAt: moved.UpdatedAt.UTC().Format(time.RFC3339),
	})
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionEdit)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var req adminPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if !document.IsSupportedLocale(req.Locale) {
		http.Error(w, "unsupported locale", http.StatusBadRequest)
		return
	}
	if !s.requireActiveEdition(w, r, document.NormalizeLocale(req.Locale)) {
		return
	}
	if techmark.ContainsSpecialSyntax(req.Source) && !user.Has(auth.PermissionAdmin) {
		http.Error(w, "administrator permission required for special TechMark syntax", http.StatusForbidden)
		return
	}
	previewSlug := techmark.Slugify(strings.TrimSpace(req.Name))
	if techmark.ContainsHomePageSyntax(req.Source) {
		isMainPage, err := s.isEditionMainPage(r.Context(), req.Locale, previewSlug)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !isMainPage {
			http.Error(w, "homepage TechMark syntax is only allowed on the edition main page", http.StatusBadRequest)
			return
		}
	}
	rendered, err := s.renderDocument(r.Context(), document.Document{
		Locale:     req.Locale,
		Slug:       previewSlug,
		Title:      strings.TrimSpace(req.Name),
		Source:     req.Source,
		EditPolicy: document.EditPolicyAdmins,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"html": rendered,
	})
}

func (s *Server) handleGetTechdataItem(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, auth.PermissionTechdata); !ok {
		return
	}
	id, err := techdata.ParseTID(r.PathValue("tid"))
	if err != nil {
		http.Error(w, "invalid TID", http.StatusBadRequest)
		return
	}
	item, err := s.store.GetItem(r.Context(), id)
	if errors.Is(err, techdata.ErrItemNotFound) {
		http.Error(w, "techdata item not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleGetPage(w http.ResponseWriter, r *http.Request) {
	locale, slug, ok := pageLocaleSlug(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	doc, err := s.reader.Page(r.Context(), locale, slug)
	if err != nil {
		writeReadError(w, err)
		return
	}

	s.writePageResponse(w, r, doc)
}

func writeReadError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "failed to read wiki data"
	switch {
	case errors.Is(err, wiki.ErrInvalidLocale):
		status, message = http.StatusBadRequest, wiki.ErrInvalidLocale.Error()
	case errors.Is(err, wiki.ErrInactiveEdition):
		status, message = http.StatusConflict, wiki.ErrInactiveEdition.Error()
	case errors.Is(err, storage.ErrDocumentNotFound):
		status, message = http.StatusNotFound, "document not found"
	}
	http.Error(w, message, status)
}

func (s *Server) handleRecordPageView(w http.ResponseWriter, r *http.Request) {
	locale, slug, ok := pageLocaleSlug(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !s.requireActiveEdition(w, r, locale) {
		return
	}
	doc, err := s.store.GetLatest(r.Context(), locale, slug)
	if err != nil {
		if errors.Is(err, storage.ErrDocumentNotFound) {
			http.Error(w, "document not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if doc.Revision != "system" {
		_ = s.store.RecordPageView(r.Context(), locale, doc.Slug, time.Now().UTC())
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListRevisions(w http.ResponseWriter, r *http.Request) {
	setRevisionPrivacyHeaders(w)
	locale, slug, ok := pageLocaleSlug(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !s.requireActiveEdition(w, r, locale) {
		return
	}

	doc, err := s.store.GetLatest(r.Context(), locale, slug)
	if errors.Is(err, storage.ErrDocumentNotFound) {
		http.Error(w, "document not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	revisions, err := s.store.ListRevisions(r.Context(), locale, slug, parseRevisionLimit(r.URL.Query().Get("limit")))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	publicRevisions := make([]publicRevisionSummary, 0, len(revisions))
	for _, revision := range revisions {
		if !isPublicRevision(revision.Revision) {
			continue
		}
		publicRevisions = append(publicRevisions, publicRevisionSummary{
			Revision:  revision.Revision,
			Author:    revision.Author,
			Summary:   revision.Summary,
			UpdatedAt: revision.UpdatedAt,
		})
	}
	requestUser, authErr := s.currentUser(r)
	canCompare := doc.EditPolicy != document.EditPolicyAdmins || authErr == nil && requestUser.Has(auth.PermissionAdmin)
	canResetHistory := authErr == nil && requestUser.Has(auth.PermissionOwner)
	writeJSON(w, http.StatusOK, publicRevisionListResponse{
		Locale:          locale,
		Name:            doc.Title,
		URL:             wikiURL(locale, doc.Title),
		CanCompare:      canCompare,
		CanResetHistory: canResetHistory,
		Revisions:       publicRevisions,
	})
}

func (s *Server) handleGetRevision(w http.ResponseWriter, r *http.Request) {
	setRevisionPrivacyHeaders(w)
	locale, slug, ok := pageLocaleSlug(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !s.requireActiveEdition(w, r, locale) {
		return
	}
	if _, err := s.store.GetLatest(r.Context(), locale, slug); err != nil {
		http.Error(w, "revision not found", http.StatusNotFound)
		return
	}

	revision := r.PathValue("revision")
	if !isPublicRevision(revision) {
		http.Error(w, "revision not found", http.StatusNotFound)
		return
	}
	doc, err := s.store.GetRevision(r.Context(), locale, slug, revision)
	if err != nil {
		if errors.Is(err, storage.ErrDocumentNotFound) {
			http.Error(w, "revision not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if doc.Source != "" {
		doc.HTML, err = s.renderDocument(r.Context(), doc)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, http.StatusOK, publicRevisionResponse{
		Locale:    doc.Locale,
		Name:      doc.Title,
		URL:       wikiURL(doc.Locale, doc.Title),
		Revision:  doc.Revision,
		Author:    doc.Author,
		Summary:   doc.Summary,
		HTML:      doc.HTML,
		UpdatedAt: doc.UpdatedAt.UTC().Format(time.RFC3339),
	})
}

func (s *Server) writePageResponse(w http.ResponseWriter, r *http.Request, doc document.Document) {
	if doc.Revision != "system" {
		// Page metrics are best-effort and must never make a readable page unavailable.
		_ = s.store.RecordPageView(r.Context(), doc.Locale, doc.Slug, time.Now().UTC())
	}
	if doc.Source != "" {
		var err error
		doc.HTML, err = s.renderDocument(r.Context(), doc)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	translations, err := s.translationsForPage(r.Context(), doc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Language", doc.Locale)
	writeJSON(w, http.StatusOK, newPublicPageResponse(doc, translations))
}

func (s *Server) translationsForPage(ctx context.Context, doc document.Document) ([]document.Document, error) {
	doc = document.NormalizeDocument(doc)
	if doc.Revision == "system" {
		return s.systemTranslations(ctx, doc.Slug)
	}
	if doc.ID == "" {
		return []document.Document{}, nil
	}
	item, err := s.store.GetItemByWikiPage(ctx, doc.ID)
	if errors.Is(err, techdata.ErrItemNotFound) {
		return []document.Document{}, nil
	}
	if err != nil {
		return nil, err
	}
	translations := make([]document.Document, 0, len(item.WikiPages))
	for locale, wid := range item.WikiPages {
		translation, err := s.store.GetLatestByWID(ctx, wid)
		if errors.Is(err, storage.ErrDocumentNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if translation.Locale != locale {
			continue
		}
		translations = append(translations, translation)
	}
	sort.SliceStable(translations, func(i, j int) bool { return translations[i].Locale < translations[j].Locale })
	return translations, nil
}

func (s *Server) translationsForGroup(ctx context.Context, group string) ([]document.Document, error) {
	group = strings.TrimSpace(group)
	if group == "" {
		return []document.Document{}, nil
	}
	translations, err := s.store.GetTranslations(ctx, group)
	if err != nil {
		return nil, err
	}
	if isSystemSlug(group) {
		system, err := s.systemTranslations(ctx, group)
		if err != nil {
			return nil, err
		}
		seen := make(map[string]struct{}, len(translations))
		for _, translation := range translations {
			seen[translation.Locale] = struct{}{}
		}
		for _, translation := range system {
			if _, ok := seen[translation.Locale]; !ok {
				translations = append(translations, translation)
			}
		}
	}
	return translations, nil
}

func (s *Server) systemTranslations(ctx context.Context, slug string) ([]document.Document, error) {
	if !isSystemSlug(slug) {
		return []document.Document{}, nil
	}

	editions, err := s.store.ListEditions(ctx)
	if err != nil {
		return nil, err
	}
	translations := make([]document.Document, 0, len(editions))
	for _, value := range editions {
		if !value.Active() {
			continue
		}
		if slug == "main" {
			doc, err := s.store.GetLatest(ctx, value.Tag, techmark.Slugify(value.MainPageName))
			if err == nil {
				translations = append(translations, doc)
				continue
			}
			if !isDocumentNotFound(err) {
				return nil, err
			}
		}
		doc, ok, err := s.systemPage(ctx, value.Tag, slug)
		if err != nil {
			return nil, err
		}
		if ok {
			translations = append(translations, doc)
		}
	}
	return translations, nil
}

func (s *Server) resolveLink(ctx context.Context, locale string, slug string, requestedName string) (publicResolvedLink, error) {
	doc, exists, err := s.store.Exists(ctx, locale, slug)
	if err != nil {
		return publicResolvedLink{}, err
	}
	if exists {
		return publicResolvedLink{
			Locale: locale,
			Name:   doc.Title,
			URL:    wikiURL(locale, doc.Title),
			Exists: true,
		}, nil
	}
	if target, err := s.store.GetDocumentRedirect(ctx, locale, slug); err == nil {
		if moved, err := s.store.GetLatest(ctx, locale, target); err == nil {
			return publicResolvedLink{
				Locale: locale,
				Name:   moved.Title,
				URL:    wikiURL(locale, moved.Title),
				Exists: true,
			}, nil
		}
	} else if !errors.Is(err, storage.ErrDocumentNotFound) {
		return publicResolvedLink{}, err
	}

	if isSystemSlug(slug) {
		name := systemTitle(locale, slug)
		return publicResolvedLink{
			Locale: locale,
			Name:   name,
			URL:    wikiURL(locale, name),
			Exists: true,
		}, nil
	}

	return publicResolvedLink{
		Locale: locale,
		Name:   requestedName,
		URL:    wikiURL(locale, requestedName),
		Exists: false,
	}, nil
}

func parseSearchLimit(value string) int {
	if value == "" {
		return 8
	}

	limit, err := strconv.Atoi(value)
	if err != nil {
		return 8
	}
	if limit < 1 {
		return 1
	}
	if limit > 50 {
		return 50
	}
	return limit
}

func requestLocale(r *http.Request) (string, bool) {
	locale := r.PathValue("locale")
	if locale == "" {
		trimmedPath := strings.TrimPrefix(r.URL.Path, "/")
		if segment, _, ok := strings.Cut(trimmedPath, "/"); ok {
			return document.CanonicalLocale(segment)
		}
	}
	if locale == "" {
		return document.DefaultLocale, true
	}
	return document.CanonicalLocale(locale)
}

func queryLocale(r *http.Request) (string, bool) {
	locale := r.URL.Query().Get("locale")
	if locale == "" {
		locale = r.URL.Query().Get("lang")
	}
	if locale == "" {
		locale = document.DefaultLocale
	}
	return document.CanonicalLocale(locale)
}

func pageLocaleSlug(r *http.Request) (string, string, bool) {
	locale, ok := requestLocale(r)
	if !ok {
		return "", "", false
	}
	name := r.PathValue("name")
	if name == "" {
		name = r.PathValue("slug")
	}
	return locale, techmark.Slugify(strings.ReplaceAll(name, "_", " ")), true
}

func (s *Server) requirePermission(w http.ResponseWriter, r *http.Request, permission auth.Permission) (auth.User, bool) {
	user, err := s.currentUser(r)
	if err != nil {
		http.Error(w, "login required", http.StatusUnauthorized)
		return auth.User{}, false
	}
	if !user.Has(permission) {
		http.Error(w, "permission denied", http.StatusForbidden)
		return auth.User{}, false
	}
	return user, true
}

func (s *Server) currentUser(r *http.Request) (auth.User, error) {
	return s.accounts.Authenticate(r.Context(), sessionToken(r))
}

func sessionToken(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time, publicURL string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		Secure:   r.TLS != nil || strings.HasPrefix(strings.ToLower(publicURL), "https://"),
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request, publicURL string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   r.TLS != nil || strings.HasPrefix(strings.ToLower(publicURL), "https://"),
		SameSite: http.SameSiteLaxMode,
	})
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return false
	}
	return true
}

func validPermissions(permissions []auth.Permission) bool {
	for _, permission := range permissions {
		switch permission {
		case auth.PermissionEdit, auth.PermissionTechdata, auth.PermissionAdmin, auth.PermissionOwner:
		default:
			return false
		}
	}
	return true
}

func containsPermission(permissions []auth.Permission, expected auth.Permission) bool {
	for _, permission := range permissions {
		if permission == expected {
			return true
		}
	}
	return false
}

func isPublicRevision(revision string) bool {
	if len(revision) < 2 || revision[0] != 'r' || revision[1] == '0' {
		return false
	}
	_, err := strconv.ParseUint(revision[1:], 10, 64)
	return err == nil
}

func parseRevisionLimit(value string) int {
	if value == "" {
		return 50
	}
	limit, err := strconv.Atoi(value)
	if err != nil {
		return 50
	}
	if limit < 1 {
		return 1
	}
	if limit > 100 {
		return 100
	}
	return limit
}

const sessionCookieName = "techpedia_session"

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}

		next.ServeHTTP(recorder, r)

		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		if !shouldLogRequest(r, status) {
			return
		}

		requestURI := r.URL.RequestURI()
		if r.URL.Path == "/mcp" || r.URL.Path == "/login" || r.URL.Path == "/signup" || strings.HasPrefix(r.URL.Path, "/oauth/") || strings.HasPrefix(r.URL.Path, "/admin/connections") || strings.HasPrefix(r.URL.Path, "/api/admin/oauth") {
			requestURI = r.URL.Path
		}
		log.Printf(
			"request method=%s path=%q status=%d duration=%s bytes=%d remote=%q",
			r.Method,
			requestURI,
			status,
			time.Since(started).Round(time.Millisecond),
			recorder.bytes,
			r.RemoteAddr,
		)
	})
}

func shouldLogRequest(r *http.Request, status int) bool {
	if r.URL.Path == "/api/csp-reports" && status == http.StatusNoContent {
		return false
	}
	if (r.URL.Path == "/api/analytics/ping" || r.URL.Path == "/api/analytics/visits") && status == http.StatusNoContent {
		return false
	}
	if !strings.HasPrefix(r.URL.Path, "/assets/") && !strings.HasPrefix(r.URL.Path, "/fonts/") {
		return true
	}
	return status < http.StatusOK || status >= http.StatusMultipleChoices && status != http.StatusNotModified
}

func securityHeaders(cspMode, publicURL string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") || strings.HasPrefix(r.URL.Path, "/fonts/") {
			w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			next.ServeHTTP(w, r)
			return
		}
		policy := "default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; script-src 'self' https://static.cloudflareinsights.com; script-src-attr 'none'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' data:; connect-src 'self' https://cloudflareinsights.com; media-src 'self' https:; worker-src 'self' blob:; require-trusted-types-for 'script'; trusted-types techpedia vue dompurify; report-uri /api/csp-reports; report-to techpedia-csp"
		if cspMode == "enforce" {
			w.Header().Set("Content-Security-Policy", policy)
		} else {
			w.Header().Set("Content-Security-Policy-Report-Only", policy)
		}
		w.Header().Set("Reporting-Endpoints", `techpedia-csp="`+publicURL+`/api/csp-reports"`)
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("Origin-Agent-Cluster", "?1")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

const maxCSPReportBytes = 64 << 10

func (s *Server) handleCSPReport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	contentType := strings.ToLower(strings.TrimSpace(strings.SplitN(r.Header.Get("Content-Type"), ";", 2)[0]))
	if contentType != "application/csp-report" && contentType != "application/reports+json" && contentType != "application/json" {
		http.Error(w, "unsupported CSP report content type", http.StatusUnsupportedMediaType)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxCSPReportBytes))
	if err != nil {
		http.Error(w, "CSP report is too large", http.StatusRequestEntityTooLarge)
		return
	}
	if !json.Valid(body) {
		http.Error(w, "invalid CSP report", http.StatusBadRequest)
		return
	}
	for _, report := range parseCSPReports(body) {
		if s.allowCSPReportLog(time.Now().UTC()) {
			log.Printf("csp violation disposition=%q directive=%q blocked=%q source=%q", report.Disposition, report.Directive, redactDiagnosticURL(report.BlockedURL), redactDiagnosticURL(report.SourceFile))
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

type cspReportSummary struct {
	Disposition string
	Directive   string
	BlockedURL  string
	SourceFile  string
}

func parseCSPReports(data []byte) []cspReportSummary {
	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	objects := make([]map[string]any, 0, 1)
	switch value := payload.(type) {
	case []any:
		for _, item := range value {
			if object, ok := item.(map[string]any); ok {
				objects = append(objects, object)
			}
		}
	case map[string]any:
		objects = append(objects, value)
	}
	reports := make([]cspReportSummary, 0, len(objects))
	for _, object := range objects {
		body := object
		if value, ok := object["body"].(map[string]any); ok {
			body = value
		} else if value, ok := object["csp-report"].(map[string]any); ok {
			body = value
		}
		reports = append(reports, cspReportSummary{
			Disposition: reportString(body, "disposition"),
			Directive:   firstReportString(body, "effectiveDirective", "effective-directive", "violated-directive"),
			BlockedURL:  firstReportString(body, "blockedURL", "blocked-uri"),
			SourceFile:  firstReportString(body, "sourceFile", "source-file"),
		})
	}
	return reports
}

func firstReportString(report map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := reportString(report, key); value != "" {
			return value
		}
	}
	return ""
}

func reportString(report map[string]any, key string) string {
	value, _ := report[key].(string)
	if len(value) > 500 {
		return value[:500]
	}
	return value
}

func (s *Server) allowCSPReportLog(now time.Time) bool {
	s.cspReportMu.Lock()
	defer s.cspReportMu.Unlock()
	if s.cspReportAt.IsZero() || now.Sub(s.cspReportAt) >= time.Minute {
		s.cspReportAt = now
		s.cspReports = 0
	}
	if s.cspReports >= 20 {
		return false
	}
	s.cspReports++
	return true
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (r *statusRecorder) WriteHeader(status int) {
	if r.status != 0 {
		return
	}

	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}

	n, err := r.ResponseWriter.Write(data)
	r.bytes += n
	return n, err
}

func Shutdown(ctx context.Context, server *http.Server) error {
	return server.Shutdown(ctx)
}
