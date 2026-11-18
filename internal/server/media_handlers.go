package server

import (
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"techpedia/internal/auth"
	"techpedia/internal/media"
)

type mediaFileResponse struct {
	Name         string `json:"name"`
	OriginalName string `json:"original_name,omitempty"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	SizeBytes    int64  `json:"size_bytes"`
	Author       string `json:"author,omitempty"`
	License      string `json:"license,omitempty"`
	Source       string `json:"source,omitempty"`
	UploadedBy   string `json:"uploaded_by"`
	CreatedAt    string `json:"created_at"`
	ThumbnailURL string `json:"thumbnail_url"`
	PreviewURL   string `json:"preview_url"`
	OriginalURL  string `json:"original_url"`
	Reused       bool   `json:"reused,omitempty"`
}

func (s *Server) handleDeleteMedia(w http.ResponseWriter, r *http.Request) {
	setRevisionPrivacyHeaders(w)
	u, ok := s.requirePermission(w, r, auth.PermissionOwner)
	if !ok {
		return
	}
	name := r.PathValue("name")
	fail := func(code int, message string) {
		s.recordAudit(r.Context(), u.Username, "media.delete", "media", name, "", "failed")
		writeJSON(w, code, map[string]string{"error": message})
	}
	if r.Header.Get("Origin") != s.publicURL || r.Header.Get("Content-Type") != "application/json" {
		fail(403, "소유자 화면에서만 파일을 삭제할 수 있습니다.")
		return
	}
	var req struct {
		Confirmation string `json:"confirmation"`
	}
	if !oauthDecode(w, r, &req) {
		s.recordAudit(r.Context(), u.Username, "media.delete", "media", name, "", "failed")
		return
	}
	normalized, err := media.NormalizeName(name)
	if err != nil || normalized != name || req.Confirmation != name {
		fail(400, "삭제할 정확한 파일 이름을 입력하세요.")
		return
	}
	result, err := s.media.Delete(r.Context(), name, u.Username)
	if err != nil {
		switch {
		case errors.Is(err, media.ErrNotFound):
			fail(404, "파일이 없거나 이미 삭제되었습니다.")
		case errors.Is(err, media.ErrInUse):
			fail(409, "현재 문서에서 사용하는 이미지입니다. 먼저 문서의 이미지 사용을 제거하세요.")
		case errors.Is(err, media.ErrInvalidName):
			fail(400, "삭제할 파일 정보가 변경되었거나 잘못되었습니다.")
		default:
			fail(500, "파일을 삭제하지 못했습니다. 현재 목록을 확인하세요.")
		}
		return
	}
	if result.CleanupPending {
		s.recordAudit(r.Context(), u.Username, "media.cleanup", "media", name, "", "failed")
	}
	writeJSON(w, 200, result)
}

func (s *Server) handleSearchMedia(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, auth.PermissionEdit); !ok {
		return
	}
	limit := 24
	if value, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && value > 0 {
		limit = value
	}
	files, err := s.store.SearchMedia(r.Context(), r.URL.Query().Get("q"), limit)
	if err != nil {
		http.Error(w, "failed to search media", http.StatusInternalServerError)
		return
	}
	usage, err := s.media.Usage()
	if err != nil {
		http.Error(w, "failed to inspect media storage", http.StatusInternalServerError)
		return
	}
	result := make([]mediaFileResponse, 0, len(files))
	for _, file := range files {
		result = append(result, publicMediaFile(file, false))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"files":       result,
		"usage_bytes": usage,
		"limit_bytes": s.media.StorageLimit(),
	})
}

func (s *Server) handleUploadMedia(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, auth.PermissionEdit)
	if !ok {
		return
	}
	uploadLimit := media.DefaultUploadLimit
	if s.media != nil {
		uploadLimit = s.media.UploadLimit()
	}
	r.Body = http.MaxBytesReader(w, r.Body, uploadLimit+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		http.Error(w, "invalid media upload", http.StatusBadRequest)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	input, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "image file is required", http.StatusBadRequest)
		return
	}
	defer input.Close()

	result, err := s.media.Upload(r.Context(), input, media.Upload{
		Name:         r.FormValue("name"),
		OriginalName: header.Filename,
		Author:       r.FormValue("author"),
		License:      r.FormValue("license"),
		Source:       r.FormValue("source"),
		UploadedBy:   user.Username,
	})
	if err != nil {
		switch {
		case errors.Is(err, media.ErrInvalidName), errors.Is(err, media.ErrInvalidImage):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, media.ErrUploadTooLarge):
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		case errors.Is(err, media.ErrAlreadyExists):
			http.Error(w, err.Error(), http.StatusConflict)
		case errors.Is(err, media.ErrStorageFull):
			http.Error(w, err.Error(), http.StatusInsufficientStorage)
		default:
			http.Error(w, "failed to process media", http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusCreated, publicMediaFile(result.File, result.Reused))
}

func (s *Server) handleMediaFile(w http.ResponseWriter, r *http.Request) {
	variantName := strings.TrimSuffix(strings.ToLower(r.PathValue("variant")), filepath.Ext(r.PathValue("variant")))
	switch variantName {
	case "thumb", "article", "large":
	default:
		http.NotFound(w, r)
		return
	}
	file, _, opened, err := s.media.OpenVariant(r.Context(), r.PathValue("name"), variantName)
	if errors.Is(err, media.ErrNotFound) || errors.Is(err, media.ErrInvalidName) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "failed to load media", http.StatusInternalServerError)
		return
	}
	defer opened.Close()
	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", `"`+file.Hash+`-`+variantName+`"`)
	http.ServeContent(w, r, file.Name, file.CreatedAt, opened)
}

func publicMediaFile(file media.File, reused bool) mediaFileResponse {
	return mediaFileResponse{
		Name:         file.Name,
		OriginalName: file.OriginalName,
		Width:        file.Width,
		Height:       file.Height,
		SizeBytes:    file.SizeBytes,
		Author:       file.Author,
		License:      file.License,
		Source:       file.Source,
		UploadedBy:   file.UploadedBy,
		CreatedAt:    file.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		ThumbnailURL: mediaURL(file.Name, "thumb"),
		PreviewURL:   mediaURL(file.Name, "article"),
		OriginalURL:  mediaURL(file.Name, "large"),
		Reused:       reused,
	}
}

func mediaURL(name string, variant string) string {
	return "/media/files/" + url.PathEscape(name) + "/" + variant + ".webp"
}
