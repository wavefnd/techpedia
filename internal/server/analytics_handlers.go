package server

import (
	"errors"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"techpedia/internal/analytics"
	"techpedia/internal/auth"
)

const analyticsEnabledHeader = "X-TechPedia-Analytics"

type analyticsVisitRequest struct {
	Route     string  `json:"route"`
	Locale    string  `json:"locale,omitempty"`
	RTTMillis *uint32 `json:"rtt_ms,omitempty"`
}

type analyticsDailyResponse struct {
	Date         string  `json:"date"`
	Views        uint64  `json:"views"`
	AverageRTTMS float64 `json:"average_rtt_ms"`
}

type analyticsBreakdownResponse struct {
	Key   string `json:"key"`
	Views uint64 `json:"views"`
}

type adminAnalyticsResponse struct {
	Days         int                          `json:"days"`
	From         string                       `json:"from"`
	Through      string                       `json:"through"`
	TotalViews   uint64                       `json:"total_views"`
	AverageRTTMS float64                      `json:"average_rtt_ms"`
	RTTSamples   uint64                       `json:"rtt_samples"`
	Daily        []analyticsDailyResponse     `json:"daily"`
	Countries    []analyticsBreakdownResponse `json:"countries"`
	Locales      []analyticsBreakdownResponse `json:"locales"`
	Routes       []analyticsBreakdownResponse `json:"routes"`
}

func (s *Server) handleAnalyticsPing(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.analyticsEnabled {
		w.Header().Set(analyticsEnabledHeader, "enabled")
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRecordAnalyticsVisit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.analyticsEnabled {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")), "cross-site") {
		http.Error(w, "cross-site analytics submission denied", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var request analyticsVisitRequest
	if !decodeJSONBody(w, r, &request) {
		return
	}
	rtt := uint32(0)
	if request.RTTMillis != nil {
		rtt = *request.RTTMillis
	}
	visit, err := analytics.NormalizeVisit(analytics.Visit{
		RecordedAt:  time.Now().UTC(),
		Route:       analytics.RouteKind(strings.TrimSpace(request.Route)),
		Locale:      request.Locale,
		CountryCode: r.Header.Get(s.analyticsCountryHeader),
		RTTMillis:   rtt,
	})
	if err != nil {
		http.Error(w, "invalid analytics visit", http.StatusBadRequest)
		return
	}
	if err := s.store.RecordAnalyticsVisit(r.Context(), visit); err != nil {
		if errors.Is(err, analytics.ErrInvalidVisit) {
			http.Error(w, "invalid analytics visit", http.StatusBadRequest)
			return
		}
		http.Error(w, "failed to record analytics visit", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminAnalytics(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, auth.PermissionAdmin); !ok {
		return
	}
	days := 30
	if value := strings.TrimSpace(r.URL.Query().Get("days")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 365 {
			http.Error(w, "days must be between 1 and 365", http.StatusBadRequest)
			return
		}
		days = parsed
	}
	through := time.Now().UTC().Truncate(24 * time.Hour)
	from := through.AddDate(0, 0, -(days - 1))
	values, err := s.store.ListAnalyticsDaily(r.Context(), from, through)
	if err != nil {
		http.Error(w, "failed to load analytics", http.StatusInternalServerError)
		return
	}
	summary := analytics.Summarize(values)
	response := adminAnalyticsResponse{
		Days:         days,
		From:         from.Format("2006-01-02"),
		Through:      through.Format("2006-01-02"),
		TotalViews:   summary.Views,
		AverageRTTMS: averageRTT(summary.RTTTotal, summary.RTTSamples),
		RTTSamples:   summary.RTTSamples,
		Daily:        make([]analyticsDailyResponse, 0, len(summary.Daily)),
		Countries:    analyticsBreakdown(summary.Countries),
		Locales:      analyticsBreakdown(summary.Locales),
		Routes:       analyticsBreakdown(summary.Routes),
	}
	for _, value := range summary.Daily {
		response.Daily = append(response.Daily, analyticsDailyResponse{
			Date:         value.Date,
			Views:        value.Views,
			AverageRTTMS: averageRTT(value.RTTTotal, value.RTTSamples),
		})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, response)
}

func averageRTT(total uint64, samples uint64) float64 {
	if samples == 0 {
		return 0
	}
	return math.Round(float64(total)/float64(samples)*10) / 10
}

func analyticsBreakdown(values map[string]uint64) []analyticsBreakdownResponse {
	result := make([]analyticsBreakdownResponse, 0, len(values))
	for key, views := range values {
		result = append(result, analyticsBreakdownResponse{Key: key, Views: views})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Views == result[j].Views {
			return result[i].Key < result[j].Key
		}
		return result[i].Views > result[j].Views
	})
	return result
}
