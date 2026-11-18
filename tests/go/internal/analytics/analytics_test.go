package analytics

import (
	"testing"
	"time"
)

func TestNormalizeVisitAcceptsOnlyKnownAggregateDimensions(t *testing.T) {
	value, err := NormalizeVisit(Visit{
		RecordedAt:  time.Date(2026, 8, 1, 9, 0, 0, 0, time.FixedZone("KST", 9*60*60)),
		Route:       RouteArticle,
		Locale:      "KO",
		CountryCode: "kr",
		RTTMillis:   87,
	})
	if err != nil {
		t.Fatal(err)
	}
	if value.RecordedAt.Location() != time.UTC || value.Locale != "ko" || value.CountryCode != "KR" {
		t.Fatalf("unexpected normalized visit: %#v", value)
	}

	for _, invalid := range []Visit{
		{RecordedAt: time.Now(), Route: "document-title"},
		{RecordedAt: time.Now(), Route: RouteArticle, Locale: "zh-Hans"},
		{RecordedAt: time.Now(), Route: RouteArticle, RTTMillis: 60_001},
	} {
		if _, err := NormalizeVisit(invalid); err == nil {
			t.Fatalf("accepted invalid visit: %#v", invalid)
		}
	}
}

func TestSummarizeCombinesDailyAggregates(t *testing.T) {
	first := NewDaily(time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC))
	first.Views = 2
	first.RTTTotal = 180
	first.RTTSamples = 2
	first.Countries["KR"] = 2
	first.Routes[string(RouteArticle)] = 2
	second := NewDaily(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	second.Views = 1
	second.RTTTotal = 120
	second.RTTSamples = 1
	second.Countries["US"] = 1
	second.Routes[string(RoutePortal)] = 1

	value := Summarize([]Daily{second, first})
	if value.Views != 3 || value.RTTTotal != 300 || value.RTTSamples != 3 || value.Countries["KR"] != 2 {
		t.Fatalf("unexpected summary: %#v", value)
	}
	if value.Daily[0].Date != "2026-07-31" {
		t.Fatalf("daily values were not sorted: %#v", value.Daily)
	}
}
