package box

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
	"techpedia/internal/document"
	"techpedia/internal/techmark"
)

var (
	ErrInvalid  = errors.New("invalid box: use a name and nonempty TechMark without nested boxes, headings, footnotes or reserved system tags")
	ErrNotFound = errors.New("box or language content not found")
	ErrConflict = errors.New("box changed; reload before saving")
)

const MaxSourceBytes = 128 << 10

type Definition struct {
	Name           string    `json:"name"`
	Locales        []string  `json:"locales"`
	FallbackLocale string    `json:"fallback_locale"`
	Version        uint64    `json:"version"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Content struct {
	Locale    string    `json:"locale"`
	Source    string    `json:"source"`
	Author    string    `json:"author"`
	Version   uint64    `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Entry struct {
	Definition
	Content Content `json:"content"`
}

type Save struct {
	Name           string `json:"name"`
	Locale         string `json:"locale"`
	Source         string `json:"source"`
	FallbackLocale string `json:"fallback_locale"`
	BaseVersion    uint64 `json:"base_version"`
}

type Page struct {
	Boxes []Definition `json:"boxes"`
	Next  string       `json:"next"`
}

// Box names are resource names, not document identities or language titles.
func NormalizeName(value string) string {
	return norm.NFC.String(strings.Join(strings.Fields(value), " "))
}

func ValidName(value string) bool {
	if value == "" || value != NormalizeName(value) || utf8.RuneCountInString(value) > 120 || !utf8.ValidString(value) {
		return false
	}
	return !strings.ContainsAny(value, "<>\"/\\\x00") && !strings.ContainsFunc(value, unicode.IsControl)
}

func Validate(v Save) error {
	locale, ok := document.CanonicalLocale(v.Locale)
	if !ok || locale != v.Locale || !ValidName(v.Name) || len(v.Source) > MaxSourceBytes || strings.TrimSpace(v.Source) == "" || !utf8.ValidString(v.Source) {
		return ErrInvalid
	}
	if v.FallbackLocale != "" {
		tag, ok := document.CanonicalLocale(v.FallbackLocale)
		if !ok || tag != v.FallbackLocale {
			return ErrInvalid
		}
	}
	if techmark.ContainsSpecialSyntax(v.Source) || !techmark.ValidBoxSource(v.Source) {
		return ErrInvalid
	}
	return nil
}

type Store interface {
	ListBoxes(context.Context, string, string, int) (Page, error)
	GetBox(context.Context, string) (Definition, error)
	GetBoxContent(context.Context, string, string) (Content, error)
	GetBoxRevision(context.Context, string, uint64) (Entry, error)
	SaveBox(context.Context, Save, string) (Entry, error)
}
