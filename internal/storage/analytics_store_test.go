package storage

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	badger "github.com/dgraph-io/badger/v4"

	"techpedia/internal/analytics"
)

func TestAnalyticsStoreKeepsOnlyDailyAggregates(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	day := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	for _, visit := range []analytics.Visit{
		{RecordedAt: day, Route: analytics.RouteArticle, Locale: "ko", CountryCode: "KR", RTTMillis: 80},
		{RecordedAt: day.Add(time.Hour), Route: analytics.RouteSearch, Locale: "en", CountryCode: "US", RTTMillis: 120},
		{RecordedAt: day.Add(2 * time.Hour), Route: analytics.RoutePortal, CountryCode: "", RTTMillis: 0},
	} {
		if err := store.RecordAnalyticsVisit(context.Background(), visit); err != nil {
			t.Fatal(err)
		}
	}

	values, err := store.ListAnalyticsDaily(context.Background(), day.AddDate(0, 0, -1), day)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0].Views != 0 {
		t.Fatalf("missing zero-filled day: %#v", values)
	}
	value := values[1]
	if value.Views != 3 || value.RTTTotal != 200 || value.RTTSamples != 2 {
		t.Fatalf("unexpected aggregate: %#v", value)
	}
	if value.Countries["KR"] != 1 || value.Countries["US"] != 1 || value.Countries[analytics.UnknownCountry] != 1 {
		t.Fatalf("unexpected countries: %#v", value.Countries)
	}
	if value.Locales["ko"] != 1 || value.Locales["en"] != 1 || value.Routes[string(analytics.RoutePortal)] != 1 {
		t.Fatalf("unexpected aggregate dimensions: %#v", value)
	}
}

func TestAnalyticsCloseDrainsAcknowledgedVisits(t *testing.T) {
	path := t.TempDir()
	store, err := OpenBadgerDocumentStore(path)
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	visit := analytics.Visit{RecordedAt: day, Route: analytics.RouteArticle, Locale: "ko", CountryCode: "KR", RTTMillis: 50}
	if err := store.RecordAnalyticsVisit(context.Background(), visit); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	results := make(chan error, 128)
	for range 128 {
		wait.Add(1)
		go func() { defer wait.Done(); results <- store.RecordAnalyticsVisit(context.Background(), visit) }()
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	wait.Wait()
	close(results)
	acknowledged := uint64(1)
	for err := range results {
		if err == nil {
			acknowledged++
		} else if !errors.Is(err, badger.ErrDBClosed) {
			t.Fatal(err)
		}
	}
	if err := store.RecordAnalyticsVisit(context.Background(), visit); !errors.Is(err, badger.ErrDBClosed) {
		t.Fatalf("record after close: %v", err)
	}
	store, err = OpenBadgerDocumentStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	daily, err := store.ListAnalyticsDaily(context.Background(), day, day)
	if err != nil || daily[0].Views != acknowledged {
		t.Fatalf("acknowledged visits not durable: want %d, got %#v, %v", acknowledged, daily, err)
	}
}

func TestAnalyticsCancellationAndCommitFailure(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	day := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	visit := analytics.Visit{RecordedAt: day, Route: analytics.RouteArticle}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.RecordAnalyticsVisit(ctx, visit); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled visit: %v", err)
	}
	if err := store.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(analyticsDailyKey(day)), []byte("invalid aggregate"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordAnalyticsVisit(context.Background(), visit); err == nil {
		t.Fatal("failed transaction acknowledged as durable")
	}
	if err := store.db.Update(func(txn *badger.Txn) error { return txn.Delete([]byte(analyticsDailyKey(day))) }); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordAnalyticsVisit(context.Background(), visit); err != nil {
		t.Fatal(err)
	}
	daily, err := store.ListAnalyticsDaily(context.Background(), day, day)
	if err != nil || daily[0].Views != 1 {
		t.Fatalf("failed/cancelled visit counted: %#v, %v", daily, err)
	}
}

func TestAnalyticsStoreSerializesConcurrentDailyUpdates(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	day := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	var wait sync.WaitGroup
	errors := make(chan error, 64)
	for range 64 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errors <- store.RecordAnalyticsVisit(context.Background(), analytics.Visit{
				RecordedAt: day, Route: analytics.RouteArticle, Locale: "ko", CountryCode: "KR", RTTMillis: 50,
			})
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	values, err := store.ListAnalyticsDaily(context.Background(), day, day)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].Views != 64 || values[0].RTTSamples != 64 {
		t.Fatalf("concurrent updates were lost: %#v", values)
	}
}
