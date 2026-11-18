package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	badger "github.com/dgraph-io/badger/v4"

	"techpedia/internal/analytics"
)

const analyticsDailyPrefix = "analytics:v1:day:"

type analyticsRecord struct {
	ctx   context.Context
	visit analytics.Visit
	done  chan error
}

func (s *BadgerDocumentStore) RecordAnalyticsVisit(ctx context.Context, visit analytics.Visit) error {
	value, err := analytics.NormalizeVisit(visit)
	if err != nil {
		return err
	}
	record := analyticsRecord{ctx: ctx, visit: value, done: make(chan error, 1)}
	s.analyticsLifecycle.RLock()
	if s.analyticsClosed {
		s.analyticsLifecycle.RUnlock()
		return badger.ErrDBClosed
	}
	select {
	case s.analyticsQueue <- record:
		s.analyticsLifecycle.RUnlock()
	case <-ctx.Done():
		s.analyticsLifecycle.RUnlock()
		return ctx.Err()
	}
	select {
	case err := <-record.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Acknowledgements follow the Badger commit, not enqueueing. The bounded queue
// applies backpressure, and Close drains it before closing the database.
func (s *BadgerDocumentStore) writeAnalyticsBatches() {
	defer close(s.analyticsDone)
	for first := range s.analyticsQueue {
		batch := []analyticsRecord{first}
		timer := time.NewTimer(time.Millisecond)
	collect:
		for len(batch) < 64 {
			select {
			case record, open := <-s.analyticsQueue:
				if !open {
					break collect
				}
				batch = append(batch, record)
			case <-timer.C:
				break collect
			}
		}
		timer.Stop()
		active := batch[:0]
		for _, record := range batch {
			if err := contextDone(record.ctx); err != nil {
				record.done <- err
			} else {
				active = append(active, record)
			}
		}
		if len(active) == 0 {
			continue
		}
		err := s.commitAnalyticsBatch(active)
		for _, record := range active {
			record.done <- err
		}
	}
}

func (s *BadgerDocumentStore) commitAnalyticsBatch(batch []analyticsRecord) error {
	byDay := make(map[string][]analytics.Visit)
	for _, record := range batch {
		key := analyticsDailyKey(record.visit.RecordedAt)
		byDay[key] = append(byDay[key], record.visit)
	}
	return s.db.Update(func(txn *badger.Txn) error {
		for key, visits := range byDay {
			if err := updateAnalyticsDailyTxn(txn, []byte(key), visits); err != nil {
				return err
			}
		}
		return nil
	})
}

func updateAnalyticsDailyTxn(txn *badger.Txn, key []byte, visits []analytics.Visit) error {
	day := visits[0].RecordedAt.UTC().Truncate(24 * time.Hour)
	daily := analytics.NewDaily(day)
	item, err := txn.Get(key)
	if err == nil {
		if err := item.Value(func(data []byte) error { return json.Unmarshal(data, &daily) }); err != nil {
			return err
		}
		if daily.SchemaVersion != analytics.SchemaVersion || daily.Date != day.Format("2006-01-02") {
			return errors.New("invalid analytics daily aggregate")
		}
	} else if !errors.Is(err, badger.ErrKeyNotFound) {
		return err
	}
	ensureAnalyticsMaps(&daily)
	for _, value := range visits {
		daily.Views++
		daily.Countries[value.CountryCode]++
		daily.Routes[string(value.Route)]++
		if value.Locale != "" {
			daily.Locales[value.Locale]++
		}
		if value.RTTMillis > 0 {
			daily.RTTTotal += uint64(value.RTTMillis)
			daily.RTTSamples++
		}
	}
	blob, err := json.Marshal(daily)
	if err != nil {
		return err
	}
	return txn.Set(key, blob)
}

func (s *BadgerDocumentStore) ListAnalyticsDaily(ctx context.Context, from time.Time, through time.Time) ([]analytics.Daily, error) {
	from = from.UTC().Truncate(24 * time.Hour)
	through = through.UTC().Truncate(24 * time.Hour)
	if through.Before(from) || through.Sub(from) > 366*24*time.Hour {
		return nil, analytics.ErrInvalidVisit
	}
	values := make([]analytics.Daily, 0, int(through.Sub(from)/(24*time.Hour))+1)
	err := s.db.View(func(txn *badger.Txn) error {
		for day := from; !day.After(through); day = day.AddDate(0, 0, 1) {
			if err := contextDone(ctx); err != nil {
				return err
			}
			daily := analytics.NewDaily(day)
			item, err := txn.Get([]byte(analyticsDailyKey(day)))
			if err == nil {
				if err := item.Value(func(data []byte) error { return json.Unmarshal(data, &daily) }); err != nil {
					return err
				}
				if daily.SchemaVersion != analytics.SchemaVersion || daily.Date != day.Format("2006-01-02") {
					return fmt.Errorf("invalid analytics aggregate for %s", day.Format("2006-01-02"))
				}
			} else if !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
			ensureAnalyticsMaps(&daily)
			values = append(values, daily)
		}
		return nil
	})
	return values, err
}

func analyticsDailyKey(day time.Time) string {
	return analyticsDailyPrefix + day.UTC().Format("2006-01-02")
}

func ensureAnalyticsMaps(value *analytics.Daily) {
	if value.Countries == nil {
		value.Countries = make(map[string]uint64)
	}
	if value.Locales == nil {
		value.Locales = make(map[string]uint64)
	}
	if value.Routes == nil {
		value.Routes = make(map[string]uint64)
	}
}
