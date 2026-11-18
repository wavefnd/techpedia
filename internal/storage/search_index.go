package storage

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	badger "github.com/dgraph-io/badger/v4"

	"techpedia/internal/document"
	"techpedia/internal/techmark"
)

const searchCacheBudget = 16 << 20
const searchCacheLocales = 8

type searchIndexEntry struct {
	SchemaVersion    int               `json:"schema_version"`
	Document         document.Document `json:"document"`
	NormalizedTitle  string            `json:"normalized_title"`
	NormalizedSlug   string            `json:"normalized_slug"`
	NormalizedSource string            `json:"normalized_source"`
	Snippet          string            `json:"snippet"`
}

func searchDocumentPrefix(locale string) string {
	return "search:v1:document:" + document.NormalizeLocale(locale) + ":"
}

func searchDocumentKey(locale, slug string) []byte {
	return []byte(searchDocumentPrefix(locale) + hex.EncodeToString([]byte(slug)))
}

func setSearchDocumentTxn(txn *badger.Txn, doc document.Document) error {
	entry := searchIndexEntry{
		SchemaVersion:   1,
		NormalizedTitle: normalizeSearchText(doc.Title), NormalizedSlug: normalizeSearchText(doc.Slug),
		NormalizedSource: normalizeSearchText(doc.Source), Snippet: techmark.Excerpt(doc.Source, doc.Title, 96),
	}
	doc.Source, doc.HTML, doc.Author, doc.Summary = "", "", "", ""
	entry.Document = doc
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return txn.Set(searchDocumentKey(doc.Locale, doc.Slug), data)
}

func (s *BadgerDocumentStore) visitSearchCandidates(ctx context.Context, locale string, visit func(searchCandidate) error) error {
	return s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(searchDocumentPrefix(locale))
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(opts.Prefix); it.ValidForPrefix(opts.Prefix); it.Next() {
			if err := contextDone(ctx); err != nil {
				return err
			}
			if err := it.Item().Value(func(data []byte) error {
				var entry searchIndexEntry
				if err := json.Unmarshal(data, &entry); err != nil {
					return err
				}
				if entry.SchemaVersion != 1 {
					return errors.New("invalid search index schema")
				}
				return visit(searchCandidate{doc: entry.Document, normalizedTitle: entry.NormalizedTitle, normalizedSlug: entry.NormalizedSlug, normalizedSource: entry.NormalizedSource, snippet: entry.Snippet})
			}); err != nil {
				return err
			}
		}
		return contextDone(ctx)
	})
}

type rankedSearchResult struct {
	doc     document.Document
	title   string
	snippet string
	score   int
}

func keepSearchResult(top []rankedSearchResult, candidate searchCandidate, score, limit int) []rankedSearchResult {
	title := strings.ToLower(candidate.doc.Title)
	index := sort.Search(len(top), func(i int) bool {
		return score > top[i].score || (score == top[i].score && title < top[i].title)
	})
	if index >= limit {
		return top
	}
	if len(top) < limit {
		top = append(top, rankedSearchResult{})
	}
	copy(top[index+1:], top[index:len(top)-1])
	top[index] = rankedSearchResult{doc: candidate.doc, title: title, snippet: candidate.snippet, score: score}
	return top
}

func searchCandidateBytes(candidate searchCandidate) int {
	return 512 + len(candidate.doc.Locale) + len(candidate.doc.Group) + len(candidate.doc.Slug) + len(candidate.doc.Title) + len(candidate.doc.Revision) + len(candidate.normalizedTitle) + len(candidate.normalizedSlug) + len(candidate.normalizedSource) + len(candidate.snippet)
}
