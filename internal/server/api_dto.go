package server

import (
	"strings"
	"time"

	"techpedia/internal/auth"
	"techpedia/internal/document"
	"techpedia/internal/edition"
	"techpedia/internal/techmark"
	"techpedia/internal/translation"
)

type publicLanguageLink struct {
	Locale string `json:"locale"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	Exists bool   `json:"exists"`
}

type publicPageResponse struct {
	Locale          string               `json:"locale"`
	Name            string               `json:"name"`
	URL             string               `json:"url"`
	Description     string               `json:"description"`
	MetaDescription string               `json:"meta_description"`
	HTML            string               `json:"html"`
	UpdatedAt       string               `json:"updated_at"`
	LineCount       int                  `json:"line_count"`
	CharCount       int                  `json:"character_count"`
	EditPolicy      document.EditPolicy  `json:"edit_policy"`
	Languages       []publicLanguageLink `json:"languages"`
}

type publicSearchResult struct {
	Locale      string `json:"locale"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

type publicSearchResponse struct {
	Query   string               `json:"query"`
	Results []publicSearchResult `json:"results"`
	Create  *publicCreateLink    `json:"create,omitempty"`
}

type publicCreateLink struct {
	Locale string `json:"locale"`
	Name   string `json:"name"`
	URL    string `json:"url"`
}

type publicResolvedLink struct {
	Locale string `json:"locale"`
	Target string `json:"target"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	Exists bool   `json:"exists"`
}

type publicLinkResolveResponse struct {
	Locale string               `json:"locale"`
	Links  []publicResolvedLink `json:"links"`
}

type adminPageResponse struct {
	Locale     string              `json:"locale"`
	Name       string              `json:"name"`
	Source     string              `json:"source"`
	EditPolicy document.EditPolicy `json:"edit_policy"`
	Revision   string              `json:"revision"`
	UpdatedAt  string              `json:"updated_at"`
}

type adminDocumentSummary struct {
	WID        string              `json:"wid,omitempty"`
	TID        string              `json:"tid,omitempty"`
	Locale     string              `json:"locale"`
	Name       string              `json:"name"`
	URL        string              `json:"url"`
	EditURL    string              `json:"edit_url"`
	Revision   string              `json:"revision"`
	Author     string              `json:"author"`
	UpdatedAt  string              `json:"updated_at"`
	System     bool                `json:"system"`
	Editable   bool                `json:"editable"`
	EditPolicy document.EditPolicy `json:"edit_policy"`
}

type adminLocaleCount struct {
	Locale    string `json:"locale"`
	Documents int    `json:"documents"`
}

type adminDashboardResponse struct {
	Documents       int                    `json:"documents"`
	Users           int                    `json:"users"`
	ActiveEditions  int                    `json:"active_editions"`
	PendingEditions int                    `json:"pending_editions"`
	ByLocale        []adminLocaleCount     `json:"by_locale"`
	RecentDocuments []adminDocumentSummary `json:"recent_documents"`
}

type adminSavePageRequest struct {
	Name              string                    `json:"name"`
	Source            string                    `json:"source"`
	Summary           string                    `json:"summary"`
	EditPolicy        document.EditPolicy       `json:"edit_policy"`
	BaseRevision      string                    `json:"base_revision"`
	TranslationSource *translationSourceRequest `json:"translation_source,omitempty"`
}

type translationSourceRequest struct {
	Locale string `json:"locale"`
	Name   string `json:"name"`
}

type adminMovePageRequest struct {
	Name string `json:"name"`
}

type adminDocumentRedirectRequest struct {
	Locale string `json:"locale"`
	Name   string `json:"name"`
	Target string `json:"target"`
}

type adminDocumentRedirectResponse struct {
	Locale       string `json:"locale"`
	Name         string `json:"name"`
	URL          string `json:"url"`
	TargetName   string `json:"target_name,omitempty"`
	TargetURL    string `json:"target_url,omitempty"`
	TargetExists bool   `json:"target_exists"`
	CreatedBy    string `json:"created_by"`
	CreatedAt    string `json:"created_at"`
}

type adminPreviewRequest struct {
	Locale string `json:"locale"`
	Name   string `json:"name"`
	Source string `json:"source"`
}

type signupRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type accountResponse struct {
	Username    string            `json:"username"`
	DisplayName string            `json:"display_name"`
	Bio         string            `json:"bio"`
	Permissions []auth.Permission `json:"permissions"`
}

type profileUpdateRequest struct {
	DisplayName string `json:"display_name"`
	Bio         string `json:"bio"`
}

type publicProfileResponse struct {
	Username      string           `json:"username"`
	DisplayName   string           `json:"display_name"`
	Bio           string           `json:"bio"`
	CreatedAt     string           `json:"created_at"`
	Contributions []map[string]any `json:"contributions"`
}

type permissionUpdateRequest struct {
	Permissions []auth.Permission `json:"permissions"`
}

type regularPermissionUpdateResponse struct {
	Permissions  []auth.Permission `json:"permissions"`
	UpdatedUsers int               `json:"updated_users"`
}

type editionResponse struct {
	Tag                        string                      `json:"tag"`
	Name                       string                      `json:"name"`
	NativeName                 string                      `json:"native_name"`
	WritingSystem              string                      `json:"writing_system"`
	ScriptName                 string                      `json:"script_name"`
	Direction                  string                      `json:"direction"`
	Status                     edition.Status              `json:"status"`
	MainPageName               string                      `json:"main_page_name,omitempty"`
	MainPageURL                string                      `json:"main_page_url,omitempty"`
	InterfaceMessages          map[string]string           `json:"interface_messages"`
	Navigation                 []edition.NavigationSection `json:"navigation"`
	PortalOrder                int                         `json:"portal_order"`
	CountryCodes               []string                    `json:"country_codes"`
	DocumentCount              int                         `json:"document_count"`
	InterfaceTranslationStatus string                      `json:"interface_translation_status,omitempty"`
	InterfaceTranslationFilled int                         `json:"interface_translation_filled,omitempty"`
}

type editionProposalRequest struct {
	Tag string `json:"tag"`
}

type editionActivationRequest struct {
	MainPageName      string                      `json:"main_page_name"`
	Source            string                      `json:"source"`
	InterfaceMessages map[string]string           `json:"interface_messages"`
	Navigation        []edition.NavigationSection `json:"navigation"`
}

type editionInterfaceRequest struct {
	InterfaceMessages map[string]string `json:"interface_messages"`
}

type editionRejectionRequest struct {
	Confirmation string `json:"confirmation"`
}

type editionNavigationRequest struct {
	Navigation []edition.NavigationSection `json:"navigation"`
}

type editionPortalRequest struct {
	PortalOrder  int      `json:"portal_order"`
	CountryCodes []string `json:"country_codes"`
}

func newEditionResponse(value edition.Edition) editionResponse {
	messages := translation.GraphInterfaceDefaults(value.Tag)
	for key, message := range value.InterfaceMessages {
		if strings.TrimSpace(message) != "" || messages[key] == "" {
			messages[key] = message
		}
	}
	navigation := value.Navigation
	if navigation == nil {
		navigation = []edition.NavigationSection{}
	}
	countryCodes := value.CountryCodes
	if countryCodes == nil {
		countryCodes = []string{}
	}
	profile, _ := edition.LanguageProfileFor(value.Tag)
	response := editionResponse{
		Tag:               value.Tag,
		Name:              value.Name,
		NativeName:        value.NativeName,
		WritingSystem:     profile.WritingSystem,
		ScriptName:        profile.ScriptName,
		Direction:         value.Direction,
		Status:            value.Status,
		MainPageName:      value.MainPageName,
		InterfaceMessages: messages,
		Navigation:        navigation,
		PortalOrder:       value.PortalOrder,
		CountryCodes:      countryCodes,
	}
	if value.Active() {
		response.MainPageURL = wikiURL(value.Tag, value.MainPageName)
	}
	return response
}

type publicRevisionSummary struct {
	Revision  string `json:"revision"`
	Author    string `json:"author"`
	Summary   string `json:"summary"`
	UpdatedAt string `json:"updated_at"`
}

type publicRevisionListResponse struct {
	Locale          string                  `json:"locale"`
	Name            string                  `json:"name"`
	URL             string                  `json:"url"`
	CanCompare      bool                    `json:"can_compare"`
	CanResetHistory bool                    `json:"can_reset_history"`
	Revisions       []publicRevisionSummary `json:"revisions"`
}

type publicRevisionResponse struct {
	Locale    string `json:"locale"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Revision  string `json:"revision"`
	Author    string `json:"author"`
	Summary   string `json:"summary"`
	HTML      string `json:"html"`
	UpdatedAt string `json:"updated_at"`
}

func newAccountResponse(user auth.User) accountResponse {
	return accountResponse{
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Bio:         user.Bio,
		Permissions: user.Permissions,
	}
}

func newPublicPageResponse(doc document.Document, translations []document.Document) publicPageResponse {
	doc = document.NormalizeDocument(doc)
	languages := make([]publicLanguageLink, 0, len(translations))
	seen := make(map[string]struct{}, len(translations)+1)
	for _, translation := range translations {
		translation = document.NormalizeDocument(translation)
		if _, exists := seen[translation.Locale]; exists {
			continue
		}
		seen[translation.Locale] = struct{}{}
		languages = append(languages, publicLanguageLink{
			Locale: translation.Locale,
			Name:   translation.Title,
			URL:    wikiURL(translation.Locale, translation.Title),
			Exists: true,
		})
	}
	if _, exists := seen[doc.Locale]; !exists {
		languages = append(languages, publicLanguageLink{
			Locale: doc.Locale,
			Name:   doc.Title,
			URL:    wikiURL(doc.Locale, doc.Title),
			Exists: true,
		})
	}

	updatedAt := ""
	if doc.Revision != "system" && !doc.UpdatedAt.IsZero() {
		updatedAt = doc.UpdatedAt.UTC().Format(time.RFC3339)
	}
	lineCount, charCount := documentStatistics(doc.Source)
	return publicPageResponse{
		Locale:          doc.Locale,
		Name:            doc.Title,
		URL:             wikiURL(doc.Locale, doc.Title),
		Description:     techmark.Excerpt(doc.Source, doc.Title, 48),
		MetaDescription: documentDescription(doc.Source, doc.Title),
		HTML:            doc.HTML,
		UpdatedAt:       updatedAt,
		LineCount:       lineCount,
		CharCount:       charCount,
		EditPolicy:      doc.EditPolicy,
		Languages:       languages,
	}
}

func documentStatistics(source string) (int, int) {
	source = strings.ReplaceAll(source, "\r\n", "\n")
	source = strings.ReplaceAll(source, "\r", "\n")
	withoutTrailingLines := strings.TrimRight(source, "\n")
	lineCount := 0
	if withoutTrailingLines != "" {
		lineCount = strings.Count(withoutTrailingLines, "\n") + 1
	}
	charCount := 0
	for _, character := range source {
		if character != '\n' {
			charCount++
		}
	}
	return lineCount, charCount
}

func newPublicSearchResult(result document.SearchResult) publicSearchResult {
	return publicSearchResult{
		Locale:      result.Locale,
		Name:        result.Title,
		URL:         wikiURL(result.Locale, result.Title),
		Description: result.Snippet,
	}
}

func wikiURL(locale string, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "/" + locale + "/wiki/"
	}
	return "/" + locale + "/wiki/" + techmark.URLName(name)
}

func editorURL(locale string, name string) string {
	return "/edit/" + locale + "/" + techmark.URLName(name)
}

func documentDescription(source string, fallback string) string {
	if description := techmark.Excerpt(source, fallback, 160); description != "" {
		return description
	}
	return defaultSiteDescription
}
