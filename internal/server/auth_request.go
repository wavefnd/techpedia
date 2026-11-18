package server

import (
	"mime"
	"net/http"
	"strings"
)

func (s *Server) allowSessionRequest(w http.ResponseWriter, r *http.Request, jsonBody bool) bool {
	origin := r.Header.Get("Origin")
	if (origin != "" && origin != s.publicURL) || strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		http.Error(w, "cross-origin authentication denied", http.StatusForbidden)
		return false
	}
	if jsonBody {
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			http.Error(w, "application/json required", http.StatusUnsupportedMediaType)
			return false
		}
	}
	return true
}
