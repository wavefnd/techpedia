package document

import (
	"strings"
	"time"
)

const DefaultLocale = "ko"

type EditPolicy string

const (
	EditPolicyEditors EditPolicy = "editors"
	EditPolicyAdmins  EditPolicy = "admins"
)

type Document struct {
	ID         WID
	Locale     string
	Group      string
	Slug       string
	Title      string
	Source     string
	HTML       string
	Author     string
	Summary    string
	EditPolicy EditPolicy
	Revision   string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type SaveRequest struct {
	Title  string `json:"title"`
	Source string `json:"source"`
	Author string `json:"author"`
	Group  string `json:"group"`
}

type PageResponse struct {
	Locale       string                     `json:"locale"`
	Group        string                     `json:"group"`
	Slug         string                     `json:"slug"`
	Title        string                     `json:"title"`
	Source       string                     `json:"source"`
	HTML         string                     `json:"html"`
	Author       string                     `json:"author"`
	Revision     string                     `json:"revision"`
	CreatedAt    string                     `json:"created_at"`
	UpdatedAt    string                     `json:"updated_at"`
	Translations map[string]TranslationLink `json:"translations"`
}

type SearchResult struct {
	Locale    string `json:"locale"`
	Group     string `json:"group"`
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Snippet   string `json:"snippet"`
	Revision  string `json:"revision"`
	UpdatedAt string `json:"updated_at"`
	Score     int    `json:"score"`
}

type SearchCreateSuggestion struct {
	Locale string `json:"locale"`
	Title  string `json:"title"`
	Slug   string `json:"slug"`
}

type SearchResponse struct {
	Query      string                 `json:"query"`
	Results    []SearchResult         `json:"results"`
	Create     SearchCreateSuggestion `json:"create"`
	ExactMatch bool                   `json:"exact_match"`
}

type ResolvedLink struct {
	Locale string `json:"locale"`
	Slug   string `json:"slug"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	Exists bool   `json:"exists"`
}

type LinkResolveResponse struct {
	Locale string         `json:"locale"`
	Links  []ResolvedLink `json:"links"`
}

type TranslationLink struct {
	Locale string `json:"locale"`
	Slug   string `json:"slug"`
	Title  string `json:"title"`
	Exists bool   `json:"exists"`
}

type TranslationResponse struct {
	Group        string                     `json:"group"`
	Translations map[string]TranslationLink `json:"translations"`
}

type RevisionSummary struct {
	Locale    string `json:"locale"`
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Revision  string `json:"revision"`
	Author    string `json:"author"`
	Summary   string `json:"summary"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func NormalizeLocale(locale string) string {
	if normalized, ok := CanonicalLocale(locale); ok {
		return normalized
	}
	return DefaultLocale
}

func IsSupportedLocale(locale string) bool {
	_, ok := CanonicalLocale(locale)
	return ok
}

func CanonicalLocale(locale string) (string, bool) {
	locale = strings.TrimSpace(locale)
	if len(locale) < 2 || len(locale) > 3 {
		return "", false
	}
	for _, value := range locale {
		if value < 'A' || value > 'Z' {
			if value < 'a' || value > 'z' {
				return "", false
			}
		}
	}
	locale = strings.ToLower(locale)
	if locale == "und" {
		return "", false
	}
	return locale, true
}

func NormalizeDocument(doc Document) Document {
	doc.Locale = NormalizeLocale(doc.Locale)
	doc.Group = strings.TrimSpace(doc.Group)
	if doc.Group == "" {
		doc.Group = doc.Slug
	}
	doc.EditPolicy = NormalizeEditPolicy(doc.EditPolicy)
	return doc
}

func NormalizeEditPolicy(policy EditPolicy) EditPolicy {
	if policy == EditPolicyAdmins {
		return EditPolicyAdmins
	}
	return EditPolicyEditors
}

func IsValidEditPolicy(policy EditPolicy) bool {
	return policy == EditPolicyEditors || policy == EditPolicyAdmins
}

func NewPageResponse(doc Document, translations []Document) PageResponse {
	doc = NormalizeDocument(doc)

	translationLinks := NewTranslationResponse(doc.Group, translations).Translations
	if _, ok := translationLinks[doc.Locale]; !ok {
		translationLinks[doc.Locale] = TranslationLink{
			Locale: doc.Locale,
			Slug:   doc.Slug,
			Title:  doc.Title,
			Exists: true,
		}
	}

	return PageResponse{
		Locale:       doc.Locale,
		Group:        doc.Group,
		Slug:         doc.Slug,
		Title:        doc.Title,
		Source:       doc.Source,
		HTML:         doc.HTML,
		Author:       doc.Author,
		Revision:     doc.Revision,
		CreatedAt:    doc.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    doc.UpdatedAt.Format(time.RFC3339),
		Translations: translationLinks,
	}
}

func NewTranslationResponse(group string, translations []Document) TranslationResponse {
	translationLinks := make(map[string]TranslationLink, len(translations))
	for _, translation := range translations {
		translation = NormalizeDocument(translation)
		translationLinks[translation.Locale] = TranslationLink{
			Locale: translation.Locale,
			Slug:   translation.Slug,
			Title:  translation.Title,
			Exists: true,
		}
	}

	return TranslationResponse{
		Group:        strings.TrimSpace(group),
		Translations: translationLinks,
	}
}

func NewSearchResult(doc Document, snippet string, score int) SearchResult {
	doc = NormalizeDocument(doc)
	return SearchResult{
		Locale:    doc.Locale,
		Group:     doc.Group,
		Slug:      doc.Slug,
		Title:     doc.Title,
		Snippet:   snippet,
		Revision:  doc.Revision,
		UpdatedAt: doc.UpdatedAt.Format(time.RFC3339),
		Score:     score,
	}
}

func NewRevisionSummary(doc Document) RevisionSummary {
	doc = NormalizeDocument(doc)
	return RevisionSummary{
		Locale:    doc.Locale,
		Slug:      doc.Slug,
		Title:     doc.Title,
		Revision:  doc.Revision,
		Author:    doc.Author,
		Summary:   doc.Summary,
		CreatedAt: doc.CreatedAt.Format(time.RFC3339),
		UpdatedAt: doc.UpdatedAt.Format(time.RFC3339),
	}
}
