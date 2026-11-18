package analytics

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"techpedia/internal/document"
)

const (
	SchemaVersion  = 1
	UnknownCountry = "ZZ"
)

var ErrInvalidVisit = errors.New("invalid analytics visit")

type RouteKind string

const (
	RoutePortal    RouteKind = "portal"
	RouteArticle   RouteKind = "article"
	RouteSearch    RouteKind = "search"
	RouteGraph     RouteKind = "graph"
	RouteSupport   RouteKind = "support"
	RouteHistory   RouteKind = "history"
	RouteLanguages RouteKind = "languages"
	RouteProfile   RouteKind = "profile"
)

type Visit struct {
	RecordedAt  time.Time
	Route       RouteKind
	Locale      string
	CountryCode string
	RTTMillis   uint32
}

type Daily struct {
	SchemaVersion int               `json:"schema_version"`
	Date          string            `json:"date"`
	Views         uint64            `json:"views"`
	RTTTotal      uint64            `json:"rtt_total_ms"`
	RTTSamples    uint64            `json:"rtt_samples"`
	Countries     map[string]uint64 `json:"countries"`
	Locales       map[string]uint64 `json:"locales"`
	Routes        map[string]uint64 `json:"routes"`
}

type Summary struct {
	Views      uint64
	RTTTotal   uint64
	RTTSamples uint64
	Countries  map[string]uint64
	Locales    map[string]uint64
	Routes     map[string]uint64
	Daily      []Daily
}

type Store interface {
	RecordAnalyticsVisit(ctx context.Context, visit Visit) error
	ListAnalyticsDaily(ctx context.Context, from time.Time, through time.Time) ([]Daily, error)
}

func NormalizeVisit(value Visit) (Visit, error) {
	if value.RecordedAt.IsZero() {
		return Visit{}, ErrInvalidVisit
	}
	value.RecordedAt = value.RecordedAt.UTC()
	if !ValidRoute(value.Route) {
		return Visit{}, ErrInvalidVisit
	}
	value.Locale = strings.ToLower(strings.TrimSpace(value.Locale))
	if value.Locale != "" {
		canonical, ok := document.CanonicalLocale(value.Locale)
		if !ok {
			return Visit{}, ErrInvalidVisit
		}
		value.Locale = canonical
	}
	value.CountryCode = NormalizeCountry(value.CountryCode)
	if value.RTTMillis > 60_000 {
		return Visit{}, ErrInvalidVisit
	}
	return value, nil
}

func ValidRoute(value RouteKind) bool {
	switch value {
	case RoutePortal, RouteArticle, RouteSearch, RouteGraph, RouteSupport, RouteHistory, RouteLanguages, RouteProfile:
		return true
	default:
		return false
	}
}

func NormalizeCountry(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) != 2 {
		return UnknownCountry
	}
	for _, char := range value {
		if char < 'A' || char > 'Z' {
			return UnknownCountry
		}
	}
	return value
}

func NewDaily(day time.Time) Daily {
	return Daily{
		SchemaVersion: SchemaVersion,
		Date:          day.UTC().Format("2006-01-02"),
		Countries:     make(map[string]uint64),
		Locales:       make(map[string]uint64),
		Routes:        make(map[string]uint64),
	}
}

func Summarize(values []Daily) Summary {
	result := Summary{
		Countries: make(map[string]uint64),
		Locales:   make(map[string]uint64),
		Routes:    make(map[string]uint64),
		Daily:     append([]Daily(nil), values...),
	}
	for _, value := range values {
		result.Views += value.Views
		result.RTTTotal += value.RTTTotal
		result.RTTSamples += value.RTTSamples
		mergeCounts(result.Countries, value.Countries)
		mergeCounts(result.Locales, value.Locales)
		mergeCounts(result.Routes, value.Routes)
	}
	sort.Slice(result.Daily, func(i, j int) bool { return result.Daily[i].Date < result.Daily[j].Date })
	return result
}

func mergeCounts(target map[string]uint64, source map[string]uint64) {
	for key, count := range source {
		target[key] += count
	}
}
