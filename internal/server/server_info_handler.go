package server

import (
	"net/http"

	"techpedia/internal/auth"
	"techpedia/internal/buildinfo"
	"techpedia/internal/systeminfo"
)

func (s *Server) handleAdminServerInfo(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, auth.PermissionAdmin); !ok {
		return
	}
	info, err := systeminfo.Collect(r.Context(), s.dataDir, s.location, buildinfo.Version, s.startedAt)
	if err != nil {
		http.Error(w, "failed to inspect server runtime", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, info)
}
