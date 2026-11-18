package storage

import (
	"context"
	cryptoRand "crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	badger "github.com/dgraph-io/badger/v4"

	"techpedia/internal/document"
	"techpedia/internal/techmark"
)

var (
	ErrDocumentNotFound = errors.New("document not found")
	ErrDocumentExists   = errors.New("document already exists")
	ErrRedirectCycle    = errors.New("document redirect cycle")
	ErrInvalidLocale    = errors.New("invalid document locale")
	ErrRevisionConflict = errors.New("document revision conflict")
)

type BadgerDocumentStore struct {
	db                           *badger.DB
	proposalMu                   sync.Mutex
	searchMu                     sync.RWMutex
	searchBuildMu                sync.Mutex
	searchCache                  map[string]searchCacheEntry
	searchGeneration             map[string]uint64
	searchCacheBytes             int
	searchMaxBytes               int
	searchCacheTick              uint64
	documentCountMu              sync.RWMutex
	documentCountCache           map[string]int
	documentCountGeneration      uint64
	documentCountCacheGeneration uint64
	analyticsLifecycle           sync.RWMutex
	analyticsClosed              bool
	analyticsQueue               chan analyticsRecord
	analyticsDone                chan struct{}
}

type searchCandidate struct {
	doc              document.Document
	normalizedTitle  string
	normalizedSlug   string
	normalizedSource string
	snippet          string
}

type searchCacheEntry struct {
	generation uint64
	candidates []searchCandidate
	bytes      int
	used       uint64
}

func OpenBadgerDocumentStore(path string) (*BadgerDocumentStore, error) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, err
	}

	opts := badger.DefaultOptions(path)
	opts.Logger = nil

	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o755); err != nil {
		_ = db.Close()
		return nil, err
	}

	store := &BadgerDocumentStore{
		db:               db,
		searchCache:      make(map[string]searchCacheEntry),
		searchGeneration: make(map[string]uint64),
		searchMaxBytes:   searchCacheBudget,
	}
	if err := store.ensureDocumentWIDs(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize document identities: %w", err)
	}
	if err := store.ensureWorkflowChanges(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize document change index: %w", err)
	}
	if err := store.startupOnce("techdata-wiki-labels-v1", store.ensureTechdataWikiPageLabels); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize techdata wiki page labels: %w", err)
	}
	if err := store.ensureDocumentReadIndexes(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize document read indexes: %w", err)
	}
	if err := store.startupOnce("mcp-proposal-retention-v2", store.initializeProposalPolicy); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize proposal retention: %w", err)
	}
	store.analyticsQueue = make(chan analyticsRecord, 256)
	store.analyticsDone = make(chan struct{})
	go store.writeAnalyticsBatches()
	return store, nil
}

func (s *BadgerDocumentStore) ensureDocumentWIDs() error {
	return s.migrateDocuments("document-identities-v1", []string{latestDocumentPrefix}, func(txn *badger.Txn, candidate document.Document, _ string) error {
		if candidate.ID != "" {
			return nil
		}
		if err := assignDocumentWID(txn, &candidate); err != nil {
			return err
		}
		blob, err := marshalDocumentXML(candidate)
		if err != nil {
			return err
		}
		return txn.Set([]byte(latestDocumentKey(candidate.Locale, candidate.Slug)), blob)
	})
}

func (s *BadgerDocumentStore) Save(ctx context.Context, doc document.Document) (document.Document, error) {
	if err := prepareDocumentForSave(ctx, &doc); err != nil {
		return document.Document{}, err
	}

	err := s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		return saveDocumentTxn(txn, &doc)
	})
	if err != nil {
		return document.Document{}, err
	}
	s.invalidateSearchCache(doc.Locale)
	return doc, nil
}

func (s *BadgerDocumentStore) SaveIfRevision(ctx context.Context, doc document.Document, baseRevision string) (document.Document, error) {
	if err := prepareDocumentForSave(ctx, &doc); err != nil {
		return document.Document{}, err
	}
	baseRevision = strings.TrimSpace(baseRevision)
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		current, err := documentFromTxn(txn, latestDocumentKey(doc.Locale, doc.Slug))
		if errors.Is(err, ErrDocumentNotFound) {
			if baseRevision != "" {
				return ErrRevisionConflict
			}
		} else if err != nil {
			return err
		} else if current.Revision != baseRevision {
			return ErrRevisionConflict
		}
		return saveDocumentTxn(txn, &doc)
	})
	if err != nil {
		return document.Document{}, err
	}
	s.invalidateSearchCache(doc.Locale)
	return doc, nil
}

func prepareDocumentForSave(ctx context.Context, doc *document.Document) error {
	if err := contextDone(ctx); err != nil {
		return err
	}
	locale, ok := document.CanonicalLocale(doc.Locale)
	if !ok {
		return ErrInvalidLocale
	}
	doc.Locale = locale
	*doc = document.NormalizeDocument(*doc)
	now := time.Now().UTC()
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = now
	}
	doc.UpdatedAt = now
	return nil
}

func saveDocumentTxn(txn *badger.Txn, doc *document.Document) error {
	action := document.ChangeEdit
	if _, err := txn.Get([]byte(latestDocumentKey(doc.Locale, doc.Slug))); errors.Is(err, badger.ErrKeyNotFound) {
		if exists, err := documentRedirectExistsTxn(txn, doc.Locale, doc.Slug); err != nil {
			return err
		} else if exists {
			return ErrDocumentExists
		}
		action = document.ChangeCreate
	} else if err != nil {
		return err
	}
	return saveDocumentTxnWithAction(txn, doc, action)
}

func saveDocumentTxnWithAction(txn *badger.Txn, doc *document.Document, action document.ChangeAction) error {
	if err := assignDocumentWID(txn, doc); err != nil {
		return err
	}
	revisionNumber, err := nextDocumentRevision(txn, doc.Locale, doc.Slug)
	if err != nil {
		return err
	}
	doc.Revision = fmt.Sprintf("r%d", revisionNumber)
	blob, err := marshalDocumentXML(*doc)
	if err != nil {
		return err
	}
	if err := txn.Set([]byte(latestDocumentKey(doc.Locale, doc.Slug)), blob); err != nil {
		return err
	}
	if err := txn.Set([]byte(revisionDocumentKey(doc.Locale, doc.Slug, doc.Revision)), blob); err != nil {
		return err
	}
	if err := setRevisionSummaryTxn(txn, *doc); err != nil {
		return err
	}
	if err := setDocumentReadIndexesTxn(txn, *doc); err != nil {
		return err
	}
	return appendRevisionChangeTxn(txn, document.Change{
		WID: doc.ID.String(), Locale: doc.Locale, Slug: doc.Slug, Title: doc.Title, Revision: doc.Revision,
		Author: doc.Author, Summary: doc.Summary, Action: action, CreatedAt: doc.UpdatedAt,
	})
}

func (s *BadgerDocumentStore) GetLatest(ctx context.Context, locale string, slug string) (document.Document, error) {
	select {
	case <-ctx.Done():
		return document.Document{}, ctx.Err()
	default:
	}

	locale = document.NormalizeLocale(locale)
	var data []byte
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(latestDocumentKey(locale, slug)))
		if errors.Is(err, badger.ErrKeyNotFound) && locale == document.DefaultLocale {
			item, err = txn.Get([]byte(legacyLatestDocumentKey(slug)))
		}
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrDocumentNotFound
			}
			return err
		}

		return item.Value(func(value []byte) error {
			data = append([]byte(nil), value...)
			return nil
		})
	})
	if err != nil {
		return document.Document{}, err
	}

	doc, err := unmarshalDocumentXML(data)
	if err != nil {
		return document.Document{}, err
	}
	if doc.Locale == "" {
		doc.Locale = locale
	}
	if doc.Group == "" {
		doc.Group = doc.Slug
	}
	return doc, nil
}

func (s *BadgerDocumentStore) GetLatestByWID(ctx context.Context, wid document.WID) (document.Document, error) {
	if err := ctx.Err(); err != nil {
		return document.Document{}, err
	}
	if err := wid.Validate(); err != nil {
		return document.Document{}, err
	}

	var doc document.Document
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(documentWIDIndexKey(wid))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrDocumentNotFound
		}
		if err != nil {
			return err
		}
		var target string
		if err := item.Value(func(value []byte) error {
			target = string(value)
			return nil
		}); err != nil {
			return err
		}
		locale, slug, ok := splitDocumentWIDTarget(target)
		if !ok {
			return fmt.Errorf("invalid document WID index for %s", wid)
		}
		doc, err = documentFromTxn(txn, latestDocumentKey(locale, slug))
		return err
	})
	if err != nil {
		return document.Document{}, err
	}
	if doc.ID != wid {
		return document.Document{}, fmt.Errorf("document WID index mismatch for %s", wid)
	}
	return doc, nil
}

func (s *BadgerDocumentStore) GetRevision(ctx context.Context, locale string, slug string, revision string) (document.Document, error) {
	select {
	case <-ctx.Done():
		return document.Document{}, ctx.Err()
	default:
	}

	locale = document.NormalizeLocale(locale)
	var data []byte
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(revisionDocumentKey(locale, slug, revision)))
		if errors.Is(err, badger.ErrKeyNotFound) && locale == document.DefaultLocale {
			item, err = txn.Get([]byte(legacyRevisionDocumentKey(slug, revision)))
		}
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrDocumentNotFound
			}
			return err
		}

		return item.Value(func(value []byte) error {
			data = append([]byte(nil), value...)
			return nil
		})
	})
	if err != nil {
		return document.Document{}, err
	}

	doc, err := unmarshalDocumentXML(data)
	if err != nil {
		return document.Document{}, err
	}
	if doc.Locale == "" {
		doc.Locale = locale
	}
	if doc.Group == "" {
		doc.Group = doc.Slug
	}
	return doc, nil
}

func (s *BadgerDocumentStore) ListRevisions(ctx context.Context, locale string, slug string, limit int) ([]document.RevisionSummary, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	locale = document.NormalizeLocale(locale)
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	return s.listRevisionSummaries(ctx, revisionSummaryPrefix(locale, slug), limit)
}

func (s *BadgerDocumentStore) ListRecentRevisions(ctx context.Context, locale string, limit int) ([]document.RevisionSummary, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	locale = document.NormalizeLocale(locale)
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	return s.listRevisionSummaries(ctx, recentSummaryPrefix(locale), limit)
}

func (s *BadgerDocumentStore) GetTranslations(ctx context.Context, group string) ([]document.Document, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	group = strings.TrimSpace(group)
	if group == "" {
		return []document.Document{}, nil
	}

	translations := make([]document.Document, 0)
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(latestDocumentPrefix)
		opts.PrefetchValues = true

		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(latestDocumentPrefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			doc, err := documentFromItem(it.Item())
			if err != nil {
				return err
			}
			doc = document.NormalizeDocument(doc)
			if doc.Group == group {
				translations = append(translations, doc)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.SliceStable(translations, func(i, j int) bool {
		return translations[i].Locale < translations[j].Locale
	})
	return translations, nil
}

func (s *BadgerDocumentStore) Exists(ctx context.Context, locale string, slug string) (document.Document, bool, error) {
	doc, err := s.GetLatest(ctx, locale, slug)
	if err == nil {
		return doc, true, nil
	}
	if errors.Is(err, ErrDocumentNotFound) {
		return document.Document{}, false, nil
	}
	return document.Document{}, false, err
}

func (s *BadgerDocumentStore) ListLatest(ctx context.Context, locale string, limit int) ([]document.Document, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	locale = document.NormalizeLocale(locale)
	if limit <= 0 {
		limit = 30
	}
	if limit > 200 {
		limit = 200
	}

	docs, err := s.latestDocuments(ctx, locale)
	if err != nil {
		return nil, err
	}

	sort.SliceStable(docs, func(i, j int) bool {
		return docs[i].UpdatedAt.After(docs[j].UpdatedAt)
	})
	if len(docs) > limit {
		docs = docs[:limit]
	}
	return docs, nil
}

func (s *BadgerDocumentStore) ListAllLatest(ctx context.Context, locale string) ([]document.Document, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	docs, err := s.latestDocuments(ctx, document.NormalizeLocale(locale))
	if err != nil {
		return nil, err
	}
	sort.SliceStable(docs, func(i, j int) bool {
		return docs[i].UpdatedAt.After(docs[j].UpdatedAt)
	})
	return docs, nil
}

func (s *BadgerDocumentStore) CountLatestByLocale(ctx context.Context) (map[string]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.documentCountMu.RLock()
	generation := s.documentCountGeneration
	if s.documentCountCache != nil && s.documentCountCacheGeneration == generation {
		counts := cloneDocumentCounts(s.documentCountCache)
		s.documentCountMu.RUnlock()
		return counts, nil
	}
	s.documentCountMu.RUnlock()

	seen := make(map[string]map[string]struct{})
	err := s.db.View(func(txn *badger.Txn) error {
		if err := collectLatestDocumentKeys(ctx, txn, latestDocumentPrefix, "", seen); err != nil {
			return err
		}
		return collectLatestDocumentKeys(ctx, txn, legacyLatestDocumentPrefix, document.DefaultLocale, seen)
	})
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(seen))
	for locale, slugs := range seen {
		counts[locale] = len(slugs)
	}
	s.documentCountMu.Lock()
	if s.documentCountGeneration == generation {
		s.documentCountCache = cloneDocumentCounts(counts)
		s.documentCountCacheGeneration = generation
	}
	s.documentCountMu.Unlock()
	return counts, nil
}

func cloneDocumentCounts(value map[string]int) map[string]int {
	cloned := make(map[string]int, len(value))
	for locale, count := range value {
		cloned[locale] = count
	}
	return cloned
}

func collectLatestDocumentKeys(ctx context.Context, txn *badger.Txn, prefixValue string, fixedLocale string, seen map[string]map[string]struct{}) error {
	options := badger.DefaultIteratorOptions
	options.Prefix = []byte(prefixValue)
	options.PrefetchValues = false
	iterator := txn.NewIterator(options)
	defer iterator.Close()
	prefix := []byte(prefixValue)
	for iterator.Seek(prefix); iterator.ValidForPrefix(prefix); iterator.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		remainder := string(iterator.Item().Key()[len(prefix):])
		locale, slug := fixedLocale, remainder
		if locale == "" {
			separator := strings.IndexByte(remainder, ':')
			if separator < 1 || separator == len(remainder)-1 {
				continue
			}
			locale, slug = remainder[:separator], remainder[separator+1:]
		}
		canonical, ok := document.CanonicalLocale(locale)
		if !ok || strings.TrimSpace(slug) == "" {
			continue
		}
		if seen[canonical] == nil {
			seen[canonical] = make(map[string]struct{})
		}
		seen[canonical][slug] = struct{}{}
	}
	return nil
}

func (s *BadgerDocumentStore) RandomLatest(ctx context.Context, locale string) (document.Document, error) {
	docs, err := s.latestDocuments(ctx, locale)
	if err != nil {
		return document.Document{}, err
	}
	if len(docs) == 0 {
		return document.Document{}, ErrDocumentNotFound
	}
	return docs[rand.Intn(len(docs))], nil
}

func (s *BadgerDocumentStore) RecordPageView(ctx context.Context, locale string, slug string, viewedAt time.Time) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	locale, ok := document.CanonicalLocale(locale)
	if !ok {
		return ErrInvalidLocale
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ErrDocumentNotFound
	}

	randomBytes := make([]byte, 16)
	if _, err := cryptoRand.Read(randomBytes); err != nil {
		return fmt.Errorf("create page metric identity: %w", err)
	}
	proposedID := hex.EncodeToString(randomBytes)
	day := viewedAt.UTC().Format("2006-01-02")

	return s.db.Update(func(txn *badger.Txn) error {
		identityKey := []byte(pageMetricIdentityKey(locale, slug))
		metricID := proposedID
		item, err := txn.Get(identityKey)
		if err == nil {
			if err := item.Value(func(value []byte) error {
				metricID = string(value)
				return nil
			}); err != nil {
				return err
			}
		} else if errors.Is(err, badger.ErrKeyNotFound) {
			if err := txn.Set(identityKey, []byte(metricID)); err != nil {
				return err
			}
		} else {
			return err
		}

		viewKey := []byte(pageMetricViewKey(metricID, day))
		var views uint64
		item, err = txn.Get(viewKey)
		if err == nil {
			if err := item.Value(func(value []byte) error {
				if len(value) != 8 {
					return errors.New("invalid page view count")
				}
				views = binary.BigEndian.Uint64(value)
				return nil
			}); err != nil {
				return err
			}
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		value := make([]byte, 8)
		binary.BigEndian.PutUint64(value, views+1)
		return txn.Set(viewKey, value)
	})
}

func (s *BadgerDocumentStore) ListPopular(ctx context.Context, locale string, since time.Time, limit int) ([]PopularDocument, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	locale = document.NormalizeLocale(locale)
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	pages, err := s.ListLinkDocuments(ctx, locale)
	if err != nil {
		return nil, err
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	firstDay := since.UTC().Truncate(24 * time.Hour)
	oldestAllowed := today.AddDate(0, 0, -30)
	if firstDay.Before(oldestAllowed) {
		firstDay = oldestAllowed
	}
	if firstDay.After(today) {
		firstDay = today
	}

	popular := make([]PopularDocument, 0, len(pages))
	err = s.db.View(func(txn *badger.Txn) error {
		for _, page := range pages {
			doc := page.Document
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			identityItem, err := txn.Get([]byte(pageMetricIdentityKey(locale, doc.Slug)))
			if errors.Is(err, badger.ErrKeyNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			var metricID string
			if err := identityItem.Value(func(value []byte) error {
				metricID = string(value)
				return nil
			}); err != nil {
				return err
			}

			var views uint64
			for day := firstDay; !day.After(today); day = day.AddDate(0, 0, 1) {
				item, err := txn.Get([]byte(pageMetricViewKey(metricID, day.Format("2006-01-02"))))
				if errors.Is(err, badger.ErrKeyNotFound) {
					continue
				}
				if err != nil {
					return err
				}
				if err := item.Value(func(value []byte) error {
					if len(value) != 8 {
						return errors.New("invalid page view count")
					}
					views += binary.BigEndian.Uint64(value)
					return nil
				}); err != nil {
					return err
				}
			}
			if views > 0 {
				popular = append(popular, PopularDocument{Document: doc, Views: views})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.SliceStable(popular, func(i, j int) bool {
		if popular[i].Views == popular[j].Views {
			return popular[i].Document.UpdatedAt.After(popular[j].Document.UpdatedAt)
		}
		return popular[i].Views > popular[j].Views
	})
	if len(popular) > limit {
		popular = popular[:limit]
	}
	results := make([]PopularDocument, 0, len(popular))
	for _, result := range popular {
		doc, err := s.GetLatest(ctx, locale, result.Document.Slug)
		if errors.Is(err, ErrDocumentNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result.Document = doc
		results = append(results, result)
	}
	return results, nil
}

func (s *BadgerDocumentStore) Search(ctx context.Context, locale string, query string, limit int) ([]document.SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []document.SearchResult{}, nil
	}
	if limit <= 0 {
		limit = 8
	}
	if limit > 50 {
		limit = 50
	}

	queries := expandSearchQueries(query)
	locale = document.NormalizeLocale(locale)
	top := make([]rankedSearchResult, 0, limit)
	candidates, err := s.searchCandidates(ctx, locale)
	if err != nil {
		return nil, err
	}
	consider := func(candidate searchCandidate) error {
		if err := contextDone(ctx); err != nil {
			return err
		}
		score := scoreCandidateAgainstQueries(candidate, queries)
		if score > 0 {
			top = keepSearchResult(top, candidate, score, limit)
		}
		return nil
	}
	if candidates == nil {
		// Editions larger than the cache budget stream the persisted normalized
		// index. Only the top results, not the whole corpus, stay in memory.
		if err := s.visitSearchCandidates(ctx, locale, consider); err != nil {
			return nil, err
		}
	} else {
		for _, candidate := range candidates {
			if err := consider(candidate); err != nil {
				return nil, err
			}
		}
	}
	results := make([]document.SearchResult, 0, len(top))
	for _, value := range top {
		results = append(results, document.NewSearchResult(value.doc, value.snippet, value.score))
	}

	return results, nil
}

func (s *BadgerDocumentStore) searchCandidates(ctx context.Context, locale string) ([]searchCandidate, error) {
	locale = document.NormalizeLocale(locale)
	if entry, cached := s.cachedSearch(locale); cached {
		return entry.candidates, nil
	}
	s.searchBuildMu.Lock()
	defer s.searchBuildMu.Unlock()
	if err := contextDone(ctx); err != nil {
		return nil, err
	}
	if entry, cached := s.cachedSearch(locale); cached {
		return entry.candidates, nil
	}
	s.searchMu.RLock()
	generation := s.searchGeneration[locale]
	s.searchMu.RUnlock()
	candidates := make([]searchCandidate, 0)
	bytes := 0
	errBudget := errors.New("search cache budget reached")
	err := s.visitSearchCandidates(ctx, locale, func(candidate searchCandidate) error {
		bytes += searchCandidateBytes(candidate)
		if bytes > s.searchMaxBytes {
			return errBudget
		}
		candidates = append(candidates, candidate)
		return nil
	})
	if errors.Is(err, errBudget) {
		candidates, bytes = nil, 0
	} else if err != nil {
		return nil, err
	}

	s.searchMu.Lock()
	if s.searchCache == nil {
		s.searchCache = make(map[string]searchCacheEntry)
	}
	if s.searchGeneration == nil {
		s.searchGeneration = make(map[string]uint64)
	}
	if s.searchGeneration[locale] == generation {
		for len(s.searchCache) >= searchCacheLocales || s.searchCacheBytes+bytes > s.searchMaxBytes {
			oldest, used := "", ^uint64(0)
			for tag, entry := range s.searchCache {
				if entry.used < used {
					oldest, used = tag, entry.used
				}
			}
			if oldest == "" {
				break
			}
			s.searchCacheBytes -= s.searchCache[oldest].bytes
			delete(s.searchCache, oldest)
		}
		s.searchCacheTick++
		s.searchCache[locale] = searchCacheEntry{generation: generation, candidates: candidates, bytes: bytes, used: s.searchCacheTick}
		s.searchCacheBytes += bytes
	}
	s.searchMu.Unlock()
	return candidates, nil
}

func (s *BadgerDocumentStore) cachedSearch(locale string) (searchCacheEntry, bool) {
	s.searchMu.Lock()
	defer s.searchMu.Unlock()
	entry, ok := s.searchCache[locale]
	if !ok || entry.generation != s.searchGeneration[locale] {
		return searchCacheEntry{}, false
	}
	s.searchCacheTick++
	entry.used = s.searchCacheTick
	s.searchCache[locale] = entry
	return entry, true
}

func (s *BadgerDocumentStore) invalidateSearchCache(locale string) {
	locale = document.NormalizeLocale(locale)
	s.searchMu.Lock()
	defer s.searchMu.Unlock()
	if s.searchCache == nil {
		s.searchCache = make(map[string]searchCacheEntry)
	}
	if s.searchGeneration == nil {
		s.searchGeneration = make(map[string]uint64)
	}
	s.searchGeneration[locale]++
	s.searchCacheBytes -= s.searchCache[locale].bytes
	delete(s.searchCache, locale)

	s.documentCountMu.Lock()
	s.documentCountGeneration++
	s.documentCountCache = nil
	s.documentCountMu.Unlock()
}

func (s *BadgerDocumentStore) latestDocuments(ctx context.Context, locale string) ([]document.Document, error) {
	locale = document.NormalizeLocale(locale)
	docs := make([]document.Document, 0)
	seen := make(map[string]struct{})

	err := s.db.View(func(txn *badger.Txn) error {
		if err := collectLatestDocuments(ctx, txn, latestDocumentLocalePrefix(locale), locale, seen, &docs); err != nil {
			return err
		}
		if locale == document.DefaultLocale {
			if err := collectLatestDocuments(ctx, txn, legacyLatestDocumentPrefix, locale, seen, &docs); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return docs, nil
}

func (s *BadgerDocumentStore) Close() error {
	s.analyticsLifecycle.Lock()
	if !s.analyticsClosed {
		s.analyticsClosed = true
		close(s.analyticsQueue)
	}
	s.analyticsLifecycle.Unlock()
	<-s.analyticsDone
	return s.db.Close()
}

const (
	latestDocumentPrefix       = "doc:v2:latest:"
	revisionDocumentPrefixRoot = "doc:v2:revision:"
	legacyLatestDocumentPrefix = "doc:latest:"
	legacyRevisionDocumentRoot = "doc:revision:"
	pageMetricIdentityPrefix   = "metrics:v1:page:"
	pageMetricViewPrefix       = "metrics:v1:view:"
	documentRedirectPrefix     = "doc:v2:redirect:"
	documentAliasPrefix        = "doc:v3:alias:"
	documentWIDIndexPrefix     = "doc:v3:wid:"
	documentWIDCounter         = "doc:v3:counter:wid"
)

func documentWIDIndexKey(wid document.WID) []byte {
	return []byte(documentWIDIndexPrefix + wid.String())
}

func splitDocumentWIDTarget(target string) (string, string, bool) {
	parts := strings.SplitN(target, "\x00", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func assignDocumentWID(txn *badger.Txn, doc *document.Document) error {
	latest, err := documentFromTxn(txn, latestDocumentKey(doc.Locale, doc.Slug))
	if err == nil {
		doc.ID = latest.ID
	} else if err != nil && !errors.Is(err, ErrDocumentNotFound) {
		return err
	} else {
		doc.ID = ""
	}

	if doc.ID == "" {
		wid, err := nextDocumentWID(txn)
		if err != nil {
			return err
		}
		doc.ID = wid
		if latest.Slug != "" {
			revisions, err := documentRevisionsFromTxn(txn, doc.Locale, doc.Slug)
			if err != nil {
				return err
			}
			for _, revision := range revisions {
				revision.doc.ID = wid
				blob, err := marshalDocumentXML(revision.doc)
				if err != nil {
					return err
				}
				if err := txn.Set([]byte(revision.key), blob); err != nil {
					return err
				}
			}
		}
	}
	if err := doc.ID.Validate(); err != nil {
		return err
	}
	return txn.Set(documentWIDIndexKey(doc.ID), []byte(doc.Locale+"\x00"+doc.Slug))
}

func nextDocumentWID(txn *badger.Txn) (document.WID, error) {
	for {
		number, err := nextTechdataID(txn, documentWIDCounter)
		if err != nil {
			return "", err
		}
		wid, err := document.NewWID(number)
		if err != nil {
			return "", err
		}
		if _, err := txn.Get(documentWIDIndexKey(wid)); err == nil {
			continue
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return "", err
		}
		if _, err := txn.Get(techdataWikiKey(wid)); err == nil {
			continue
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return "", err
		}
		return wid, nil
	}
}

func documentRedirectKey(locale string, slug string) string {
	return documentRedirectPrefix + document.NormalizeLocale(locale) + ":" + slug
}

func documentAliasLocalePrefix(locale string) string {
	return documentAliasPrefix + document.NormalizeLocale(locale) + ":"
}

func documentAliasKey(locale string, slug string) string {
	return documentAliasLocalePrefix(locale) + slug
}

func pageMetricIdentityKey(locale string, slug string) string {
	return pageMetricIdentityPrefix + document.NormalizeLocale(locale) + ":" + slug
}

func pageMetricViewKey(metricID string, day string) string {
	return pageMetricViewPrefix + metricID + ":" + day
}

func latestDocumentLocalePrefix(locale string) string {
	return latestDocumentPrefix + document.NormalizeLocale(locale) + ":"
}

func latestDocumentKey(locale string, slug string) string {
	return latestDocumentLocalePrefix(locale) + slug
}

func legacyLatestDocumentKey(slug string) string {
	return legacyLatestDocumentPrefix + slug
}

func revisionDocumentPrefix(locale string, slug string) string {
	return revisionDocumentLocalePrefix(locale) + slug + ":"
}

func revisionDocumentLocalePrefix(locale string) string {
	return revisionDocumentPrefixRoot + document.NormalizeLocale(locale) + ":"
}

func revisionDocumentKey(locale string, slug string, revision string) string {
	return revisionDocumentPrefix(locale, slug) + revision
}

func revisionSequenceKey(locale string, slug string) string {
	return "doc:v2:revision-seq:" + document.NormalizeLocale(locale) + ":" + slug
}

func nextDocumentRevision(txn *badger.Txn, locale string, slug string) (uint64, error) {
	key := []byte(revisionSequenceKey(locale, slug))
	var current uint64
	item, err := txn.Get(key)
	if err == nil {
		if err := item.Value(func(value []byte) error {
			if len(value) != 8 {
				return errors.New("invalid revision sequence")
			}
			current = binary.BigEndian.Uint64(value)
			return nil
		}); err != nil {
			return 0, err
		}
	} else if !errors.Is(err, badger.ErrKeyNotFound) {
		return 0, err
	}
	current++
	value := make([]byte, 8)
	binary.BigEndian.PutUint64(value, current)
	if err := txn.Set(key, value); err != nil {
		return 0, err
	}
	return current, nil
}

func legacyRevisionDocumentPrefix(slug string) string {
	return legacyRevisionDocumentRoot + slug + ":"
}

func legacyRevisionDocumentKey(slug string, revision string) string {
	return legacyRevisionDocumentPrefix(slug) + revision
}

func documentFromItem(item *badger.Item) (document.Document, error) {
	var data []byte
	if err := item.Value(func(value []byte) error {
		data = append([]byte(nil), value...)
		return nil
	}); err != nil {
		return document.Document{}, err
	}
	return unmarshalDocumentXML(data)
}

func collectDocumentsByPrefix(ctx context.Context, txn *badger.Txn, prefixValue string, docs *[]document.Document) error {
	opts := badger.DefaultIteratorOptions
	opts.Prefix = []byte(prefixValue)
	opts.PrefetchValues = true

	it := txn.NewIterator(opts)
	defer it.Close()

	prefix := []byte(prefixValue)
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		doc, err := documentFromItem(it.Item())
		if err != nil {
			return err
		}
		doc = document.NormalizeDocument(doc)
		*docs = append(*docs, doc)
	}
	return nil
}

func scoreCandidateAgainstQueries(candidate searchCandidate, queries []string) int {
	bestScore := 0
	for _, query := range queries {
		score := scoreCandidate(candidate, query, techmark.Slugify(query))
		if score > bestScore {
			bestScore = score
		}
	}
	return bestScore
}

func collectLatestDocuments(
	ctx context.Context,
	txn *badger.Txn,
	prefixValue string,
	locale string,
	seen map[string]struct{},
	docs *[]document.Document,
) error {
	collected := make([]document.Document, 0)
	if err := collectDocumentsByPrefix(ctx, txn, prefixValue, &collected); err != nil {
		return err
	}

	locale = document.NormalizeLocale(locale)
	for _, doc := range collected {
		doc = document.NormalizeDocument(doc)
		if doc.Locale == "" {
			doc.Locale = locale
		}
		if doc.Locale != locale {
			continue
		}

		key := doc.Locale + ":" + doc.Slug
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		*docs = append(*docs, doc)
	}
	return nil
}

func scoreCandidate(candidate searchCandidate, query string, querySlug string) int {
	score := 0
	switch {
	case candidate.normalizedTitle == query:
		score += 1000
	case strings.HasPrefix(candidate.normalizedTitle, query):
		score += 760
	case containsSearchTerm(candidate.normalizedTitle, query):
		score += 540
	}

	switch {
	case candidate.doc.Slug == querySlug:
		score += 900
	case strings.HasPrefix(candidate.normalizedSlug, querySlug):
		score += 620
	case containsSearchTerm(candidate.normalizedSlug, querySlug):
		score += 430
	}

	if shouldSearchBody(query) && containsSearchTerm(candidate.normalizedSource, query) {
		score += 120
	}

	return score
}

func normalizeSearchText(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var out strings.Builder
	lastSpace := false

	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsPunct(r) && r != '+' && r != '#' {
			if !lastSpace && out.Len() > 0 {
				out.WriteByte(' ')
				lastSpace = true
			}
			continue
		}
		out.WriteRune(r)
		lastSpace = false
	}

	return strings.TrimSpace(out.String())
}

func expandSearchQueries(query string) []string {
	normalized := normalizeSearchText(query)
	if normalized == "" {
		return []string{}
	}

	aliases := map[string][]string{
		"c언어":           {"c"},
		"씨언어":           {"c"},
		"c language":    {"c"},
		"c-language":    {"c"},
		"c programming": {"c"},
		"cplusplus":     {"c++"},
		"c plus plus":   {"c++"},
		"c플러스플러스":       {"c++"},
		"씨플플":           {"c++"},
		"cpp":           {"c++"},
		"c sharp":       {"c#"},
		"c샵":            {"c#"},
		"씨샵":            {"c#"},
		"javascript":    {"javascript", "js"},
		"자바스크립트":        {"javascript", "js"},
		"typescript":    {"typescript", "ts"},
		"타입스크립트":        {"typescript", "ts"},
	}

	seen := map[string]struct{}{}
	queries := make([]string, 0, 4)
	add := func(value string) {
		value = normalizeSearchText(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		queries = append(queries, value)
	}

	add(normalized)
	for _, alias := range aliases[normalized] {
		add(alias)
	}

	return queries
}

func containsSearchTerm(value string, query string) bool {
	if query == "" {
		return false
	}
	return strings.Contains(value, query)
}

func shouldSearchBody(query string) bool {
	runes := []rune(query)
	if len(runes) == 0 {
		return false
	}
	if len(runes) > 1 {
		return true
	}
	return !unicode.IsLetter(runes[0]) && !unicode.IsDigit(runes[0])
}
