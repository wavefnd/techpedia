package server

import (
	"errors"
	"html"
	"net/http"
	"strings"
	"time"

	"techpedia/internal/auth"
	"techpedia/internal/document"
	"techpedia/internal/support"
)

type supportSettingsResponse struct {
	Enabled     bool             `json:"enabled"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Wallets     []support.Wallet `json:"wallets"`
	UpdatedAt   string           `json:"updated_at,omitempty"`
}

type supportSettingsRequest struct {
	Enabled     bool             `json:"enabled"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Wallets     []support.Wallet `json:"wallets"`
}

func (s *Server) handleSupportIndex(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.GetSupportSettings(r.Context())
	if err != nil {
		http.Error(w, "failed to load support page", http.StatusInternalServerError)
		return
	}
	if !settings.Enabled {
		metadata := defaultIndexMetadata()
		metadata.Title = "Support page unavailable - TechPedia"
		s.writeIndex(w, r, http.StatusNotFound, metadata)
		return
	}
	description := settings.Description
	if description == "" {
		description = "Official cryptocurrency addresses for supporting TechPedia."
	}
	updatedAt := ""
	if !settings.UpdatedAt.IsZero() {
		updatedAt = settings.UpdatedAt.UTC().Format(time.RFC3339)
	}
	s.writeIndex(w, r, http.StatusOK, indexMetadata{
		Lang:        document.DefaultLocale,
		Direction:   "ltr",
		Title:       settings.Title + " - TechPedia",
		Description: description,
		Canonical:   s.absoluteURL("/support"),
		Robots:      "index,follow",
		UpdatedAt:   updatedAt,
		InitialHTML: supportInitialHTML(settings),
	})
}

func (s *Server) handleGetSupportSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.GetSupportSettings(r.Context())
	if err != nil {
		http.Error(w, "failed to load support page", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, publicSupportSettingsResponse(settings))
}

func (s *Server) handleGetAdminSupportSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, auth.PermissionAdmin); !ok {
		return
	}
	settings, err := s.store.GetSupportSettings(r.Context())
	if err != nil {
		http.Error(w, "failed to load support page settings", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, newSupportSettingsResponse(settings))
}

func (s *Server) handleSaveAdminSupportSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, auth.PermissionAdmin); !ok {
		return
	}
	var request supportSettingsRequest
	if !decodeJSONBody(w, r, &request) {
		return
	}
	saved, err := s.store.SaveSupportSettings(r.Context(), support.Settings{
		Enabled: request.Enabled, Title: request.Title, Description: request.Description, Wallets: request.Wallets,
	})
	if errors.Is(err, support.ErrInvalidSettings) {
		http.Error(w, "invalid support page settings", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "failed to save support page settings", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, newSupportSettingsResponse(saved))
}

func newSupportSettingsResponse(settings support.Settings) supportSettingsResponse {
	updatedAt := ""
	if !settings.UpdatedAt.IsZero() {
		updatedAt = settings.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	return supportSettingsResponse{
		Enabled: settings.Enabled, Title: settings.Title, Description: settings.Description,
		Wallets: settings.Wallets, UpdatedAt: updatedAt,
	}
}

func publicSupportSettingsResponse(settings support.Settings) supportSettingsResponse {
	if !settings.Enabled {
		return supportSettingsResponse{Enabled: false, Wallets: []support.Wallet{}}
	}
	return newSupportSettingsResponse(settings)
}

func supportInitialHTML(settings support.Settings) string {
	var out strings.Builder
	out.WriteString(`<main class="support-page seo-initial-content"><header><p>TechPedia</p><h1>`)
	out.WriteString(html.EscapeString(settings.Title))
	out.WriteString(`</h1>`)
	if settings.Description != "" {
		out.WriteString(`<p>`)
		out.WriteString(html.EscapeString(settings.Description))
		out.WriteString(`</p>`)
	}
	out.WriteString(`</header><div class="support-wallet-list">`)
	for _, wallet := range settings.Wallets {
		out.WriteString(`<section><h2>`)
		out.WriteString(html.EscapeString(wallet.Asset))
		out.WriteString(`</h2><p>`)
		out.WriteString(html.EscapeString(wallet.Network))
		out.WriteString(`</p><code>`)
		out.WriteString(html.EscapeString(wallet.Address))
		out.WriteString(`</code>`)
		if wallet.Memo != "" {
			out.WriteString(`<p>Memo / tag: `)
			out.WriteString(html.EscapeString(wallet.Memo))
			out.WriteString(`</p>`)
		}
		out.WriteString(`</section>`)
	}
	out.WriteString(`</div></main>`)
	return out.String()
}
