package server

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"techpedia/internal/auth"
	"techpedia/internal/box"
	"techpedia/internal/document"
	"techpedia/internal/edition"
	"techpedia/internal/storage"
	"techpedia/internal/techdata"
	"techpedia/internal/techmark"
)

var boxElementPattern = regexp.MustCompile(`<wiki-box name="[^"]*"></wiki-box>`)

func (s *Server) boxRoutes() {
	s.mux.HandleFunc("GET /admin/boxes", s.handleIndex)
	s.mux.HandleFunc("GET /api/editor/boxes", s.handleListBoxes)
	s.mux.HandleFunc("GET /api/editor/boxes/{name}/preview/{locale}", s.handleBoxPreview)
	s.mux.HandleFunc("GET /api/admin/boxes/{name}/{locale}", s.handleGetBox)
	s.mux.HandleFunc("GET /api/admin/boxes/{name}/revisions/{version}", s.handleBoxRevision)
	s.mux.HandleFunc("GET /api/admin/boxes/{name}/draft/{locale}", s.handleBoxDraft)
	s.mux.HandleFunc("PUT /api/admin/boxes/{name}/{locale}", s.handleSaveBox)
	s.mux.HandleFunc("POST /api/admin/boxes/preview/{locale}", s.handleBoxSourcePreview)
}

func (s *Server) handleBoxSourcePreview(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.boxAccess(w, r, auth.PermissionAdmin); !ok {
		return
	}
	var input struct {
		Source string `json:"source"`
	}
	if !decodeJSONBody(w, r, &input) {
		return
	}
	locale := r.PathValue("locale")
	if err := box.Validate(box.Save{Name: "preview", Locale: locale, Source: input.Source}); err != nil {
		boxError(w, err)
		return
	}
	e, err := s.store.GetEdition(r.Context(), locale)
	if err != nil || (e.Status != edition.StatusApproved && !e.Active()) {
		boxError(w, box.ErrInvalid)
		return
	}
	writeJSON(w, 200, map[string]string{"html": s.parser.RenderWithLocale(input.Source, locale)})
}

func (s *Server) boxAccess(w http.ResponseWriter, r *http.Request, permission auth.Permission) (auth.User, bool) {
	setRevisionPrivacyHeaders(w)
	u, ok := s.requirePermission(w, r, permission)
	if !ok {
		return u, false
	}
	if s.boxes == nil {
		writeJSON(w, 503, map[string]string{"error": "박스 저장소를 사용할 수 없습니다."})
		return u, false
	}
	return u, true
}

func boxError(w http.ResponseWriter, err error) {
	code := 500
	message := "박스를 처리하지 못했습니다."
	switch {
	case errors.Is(err, box.ErrNotFound):
		code = 404
		message = "박스 또는 해당 언어 내용이 없습니다."
	case errors.Is(err, box.ErrInvalid):
		code = 400
		message = "박스 이름·언어·원문을 확인하세요. 중첩 박스, 제목, 각주, 운영 예약 태그는 사용할 수 없습니다."
	case errors.Is(err, box.ErrConflict):
		code = 409
		message = "다른 변경이 저장되었습니다. 입력을 보존하고 최신 박스를 다시 불러와 비교하세요."
	}
	writeJSON(w, code, map[string]string{"error": message})
}

func (s *Server) handleListBoxes(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.boxAccess(w, r, auth.PermissionEdit); !ok {
		return
	}
	if len(r.URL.Query().Get("q")) > 512 || len(r.URL.Query().Get("after")) > 512 {
		boxError(w, box.ErrInvalid)
		return
	}
	values, err := s.boxes.ListBoxes(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("after"), 50)
	if err != nil {
		boxError(w, err)
		return
	}
	writeJSON(w, 200, values)
}

func (s *Server) handleGetBox(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.boxAccess(w, r, auth.PermissionAdmin); !ok {
		return
	}
	v, err := s.boxes.GetBox(r.Context(), r.PathValue("name"))
	if err != nil {
		boxError(w, err)
		return
	}
	locale := r.PathValue("locale")
	if tag, ok := document.CanonicalLocale(locale); !ok || tag != locale {
		boxError(w, box.ErrInvalid)
		return
	}
	c, err := s.boxes.GetBoxContent(r.Context(), v.Name, locale)
	if errors.Is(err, box.ErrNotFound) {
		c = box.Content{Locale: locale}
	} else if err != nil {
		boxError(w, err)
		return
	}
	writeJSON(w, 200, box.Entry{Definition: v, Content: c})
}

func (s *Server) handleSaveBox(w http.ResponseWriter, r *http.Request) {
	u, ok := s.boxAccess(w, r, auth.PermissionAdmin)
	if !ok {
		return
	}
	if r.Header.Get("Origin") != s.publicURL || strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		writeJSON(w, 403, map[string]string{"error": "관리자 화면에서 저장하세요."})
		return
	}
	var in box.Save
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		boxError(w, box.ErrInvalid)
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		boxError(w, box.ErrInvalid)
		return
	}
	in.Name = r.PathValue("name")
	in.Locale = r.PathValue("locale")
	v, err := s.boxes.SaveBox(r.Context(), in, u.Username)
	if err != nil {
		boxError(w, err)
		return
	}
	writeJSON(w, 200, v)
}

func (s *Server) handleBoxRevision(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.boxAccess(w, r, auth.PermissionAdmin); !ok {
		return
	}
	version, err := strconv.ParseUint(r.PathValue("version"), 10, 64)
	if err != nil || version == 0 {
		boxError(w, box.ErrInvalid)
		return
	}
	v, err := s.boxes.GetBoxRevision(r.Context(), r.PathValue("name"), version)
	if err != nil {
		boxError(w, err)
		return
	}
	writeJSON(w, 200, v)
}

func (s *Server) handleBoxPreview(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.boxAccess(w, r, auth.PermissionEdit); !ok {
		return
	}
	locale, ok := document.CanonicalLocale(r.PathValue("locale"))
	if !ok || locale != r.PathValue("locale") {
		boxError(w, box.ErrInvalid)
		return
	}
	if !s.requireActiveEdition(w, r, locale) {
		return
	}
	rendered, err := s.renderBoxes(r.Context(), `<wiki-box name="`+html.EscapeString(r.PathValue("name"))+`"></wiki-box>`, locale)
	if err != nil {
		boxError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"html": rendered})
}

// Resolution happens AFTER the source parser cache on every response. Changing
// a shared resource therefore never serves an old expanded fragment cache.
func (s *Server) renderBoxes(ctx context.Context, rendered, locale string) (string, error) {
	if !strings.Contains(rendered, "<wiki-box ") {
		return rendered, nil
	}
	cache := make(map[string]string)
	total := 0
	count := 0
	return replaceSpecialElements(rendered, boxElementPattern, func(element string) (string, error) {
		count++
		if count > 64 || s.boxes == nil {
			return "", nil
		}
		name := specialAttribute(element, "name")
		if value, ok := cache[name]; ok {
			total += len(value)
			if total > 2<<20 {
				return "", nil
			}
			return value, nil
		}
		v, err := s.boxes.GetBox(ctx, name)
		if errors.Is(err, box.ErrNotFound) {
			cache[name] = ""
			return "", nil
		}
		if err != nil {
			return "", err
		}
		c, err := s.boxes.GetBoxContent(ctx, name, locale)
		if errors.Is(err, box.ErrNotFound) && v.FallbackLocale != "" {
			c, err = s.boxes.GetBoxContent(ctx, name, v.FallbackLocale)
		}
		if errors.Is(err, box.ErrNotFound) {
			cache[name] = ""
			return "", nil
		}
		if err != nil {
			return "", err
		}
		e, err := s.store.GetEdition(ctx, c.Locale)
		if err != nil && !errors.Is(err, edition.ErrNotFound) {
			return "", err
		}
		if err != nil || !e.Active() {
			cache[name] = ""
			return "", nil
		}
		if !techmark.ValidBoxSource(c.Source) || techmark.ContainsSpecialSyntax(c.Source) {
			return "", nil
		}
		profile, _ := edition.LanguageProfileFor(c.Locale)
		value := `<div class="wiki-box-content" lang="` + html.EscapeString(c.Locale) + `" dir="` + html.EscapeString(e.Direction) + `" data-writing-system="` + html.EscapeString(profile.WritingSystem) + `">` + s.parser.RenderWithLocale(c.Source, c.Locale) + `</div>`
		cache[name] = value
		total += len(value)
		if total > 2<<20 {
			return "", nil
		}
		return value, nil
	})
}

// Copying creates an unsaved draft. Only real Techdata sitelinks are rewritten;
// missing target editions become plain labels rather than guessed wiki URLs.
func (s *Server) handleBoxDraft(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.boxAccess(w, r, auth.PermissionAdmin); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	from := r.URL.Query().Get("from")
	to := r.PathValue("locale")
	if from == to {
		boxError(w, box.ErrInvalid)
		return
	}
	for _, tag := range []string{from, to} {
		locale, ok := document.CanonicalLocale(tag)
		if !ok || locale != tag {
			boxError(w, box.ErrInvalid)
			return
		}
	}
	targetEdition, err := s.store.GetEdition(ctx, to)
	if err != nil || (targetEdition.Status != edition.StatusApproved && !targetEdition.Active()) {
		boxError(w, box.ErrInvalid)
		return
	}
	v, err := s.boxes.GetBox(ctx, r.PathValue("name"))
	if err != nil {
		boxError(w, err)
		return
	}
	c, err := s.boxes.GetBoxContent(ctx, v.Name, from)
	if err != nil {
		boxError(w, err)
		return
	}
	var out strings.Builder
	pos := 0
	unresolved := []string{}
	resolved := map[string]string{}
	links := techmark.InternalLinks(c.Source)
	if len(links) > 512 {
		boxError(w, box.ErrInvalid)
		return
	}
	for _, link := range links {
		target, seen := resolved[link.Target]
		if !seen {
			doc, lookupErr := s.store.GetLatest(ctx, from, techmark.Slugify(link.Target))
			if lookupErr != nil && !errors.Is(lookupErr, storage.ErrDocumentNotFound) {
				boxError(w, lookupErr)
				return
			}
			if lookupErr == nil {
				item, itemErr := s.store.GetItemByWikiPage(ctx, doc.ID)
				if itemErr != nil && !errors.Is(itemErr, techdata.ErrItemNotFound) {
					boxError(w, itemErr)
					return
				}
				if wid := item.WikiPages[to]; wid != "" {
					translated, readErr := s.store.GetLatestByWID(ctx, wid)
					if readErr != nil && !errors.Is(readErr, storage.ErrDocumentNotFound) {
						boxError(w, readErr)
						return
					}
					if readErr == nil && translated.Locale == to {
						target = translated.Title
					}
				}
			}
			resolved[link.Target] = target
			if target == "" {
				unresolved = append(unresolved, link.Target)
			}
		}
		out.WriteString(c.Source[pos:link.Start])
		if target != "" {
			out.WriteString("[[" + target)
			if link.HasLabel {
				out.WriteString("|" + link.Label)
			}
			out.WriteString("]]")
		} else if link.HasLabel {
			out.WriteString(link.Label)
		} else {
			out.WriteString(link.Target)
		}
		pos = link.End
	}
	out.WriteString(c.Source[pos:])
	writeJSON(w, 200, map[string]any{"source": out.String(), "base_version": v.Version, "unresolved": unresolved})
}
