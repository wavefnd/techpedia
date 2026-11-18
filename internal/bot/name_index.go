package bot

import (
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	"techpedia/internal/document"
	"techpedia/internal/techdata"
)

type NameSource string

const (
	NameIndexGeneratorVersion            = "name-index-v1"
	NameSourceTitle           NameSource = "title"
	NameSourceTechdataLabel   NameSource = "techdata_label"
	NameSourceTechdataAlias   NameSource = "techdata_alias"
	NameSourceGenerated       NameSource = "generated"
)

type NameMatchKind string

const (
	NameMatchNone      NameMatchKind = "none"
	NameMatchExact     NameMatchKind = "exact"
	NameMatchFuzzy     NameMatchKind = "fuzzy"
	NameMatchAmbiguous NameMatchKind = "ambiguous"
)

type NameCandidate struct {
	Locale        string
	CanonicalName string
	Slug          string
	MatchedName   string
	Source        NameSource
}

type NameMatch struct {
	Kind         NameMatchKind
	Query        string
	Candidate    NameCandidate
	Alternatives []NameCandidate
	Score        float64
}

func (match NameMatch) Actionable() bool {
	return match.Kind == NameMatchExact || match.Kind == NameMatchFuzzy
}

type NameIndexStats struct {
	GeneratorVersion string
	Documents        int
	Terms            int
	GramKeys         int
	GramPostings     int
	Collisions       int
}

type nameIndexCandidate struct {
	locale string
	name   string
	slug   string
}

type nameIndexTerm struct {
	candidate int
	key       string
	fuzzyKey  string
	display   string
	source    NameSource
	rank      int
}

// NameIndex is immutable after construction. A complete replacement can be
// built off the request path and swapped without exposing a partial index.
type NameIndex struct {
	candidates []nameIndexCandidate
	terms      []nameIndexTerm
	exact      map[string][]int
	grams      map[uint64][]int
	stats      NameIndexStats
}

type pendingNameTerm struct {
	candidate int
	key       string
	fuzzyKey  string
	display   string
	source    NameSource
	rank      int
}

func BuildNameIndex(documents []document.Document, items []techdata.Item) *NameIndex {
	index := &NameIndex{
		exact: make(map[string][]int),
		grams: make(map[uint64][]int),
		stats: NameIndexStats{GeneratorVersion: NameIndexGeneratorVersion},
	}

	normalizedDocuments := make([]document.Document, 0, len(documents))
	for _, raw := range documents {
		doc := document.NormalizeDocument(raw)
		if doc.Title == "" || doc.Slug == "" {
			continue
		}
		normalizedDocuments = append(normalizedDocuments, doc)
	}
	sort.SliceStable(normalizedDocuments, func(left, right int) bool {
		if normalizedDocuments[left].Locale != normalizedDocuments[right].Locale {
			return normalizedDocuments[left].Locale < normalizedDocuments[right].Locale
		}
		if normalizedDocuments[left].Title != normalizedDocuments[right].Title {
			return normalizedDocuments[left].Title < normalizedDocuments[right].Title
		}
		return normalizedDocuments[left].Slug < normalizedDocuments[right].Slug
	})

	candidateByDocument := make(map[string]int, len(normalizedDocuments))
	candidateByWID := make(map[string]int, len(normalizedDocuments))
	pending := make(map[string]pendingNameTerm)
	for _, doc := range normalizedDocuments {
		documentKey := indexLocaleKey(doc.Locale, doc.Slug)
		candidateID, exists := candidateByDocument[documentKey]
		if !exists {
			candidateID = len(index.candidates)
			candidateByDocument[documentKey] = candidateID
			index.candidates = append(index.candidates, nameIndexCandidate{locale: doc.Locale, name: doc.Title, slug: doc.Slug})
			if doc.ID != "" {
				candidateByWID[doc.ID.String()] = candidateID
			}
		}
		addPendingName(pending, candidateID, doc.Title, NameSourceTitle)
		if base := parentheticalBase(doc.Title); base != "" {
			addPendingName(pending, candidateID, base, NameSourceGenerated)
		}
	}

	sortedItems := append([]techdata.Item(nil), items...)
	sort.SliceStable(sortedItems, func(left, right int) bool { return sortedItems[left].ID.String() < sortedItems[right].ID.String() })
	for _, item := range sortedItems {
		locales := make([]string, 0, len(item.WikiPages))
		for locale := range item.WikiPages {
			locales = append(locales, locale)
		}
		sort.Strings(locales)
		for _, locale := range locales {
			candidateID, exists := candidateByWID[item.WikiPages[locale].String()]
			if !exists || index.candidates[candidateID].locale != locale {
				continue
			}
			addPendingName(pending, candidateID, item.Labels[locale], NameSourceTechdataLabel)
			for _, alias := range item.Aliases[locale] {
				addPendingName(pending, candidateID, alias, NameSourceTechdataAlias)
			}
		}
	}

	terms := make([]pendingNameTerm, 0, len(pending))
	for _, term := range pending {
		terms = append(terms, term)
	}
	sort.SliceStable(terms, func(left, right int) bool {
		leftCandidate := index.candidates[terms[left].candidate]
		rightCandidate := index.candidates[terms[right].candidate]
		if leftCandidate.locale != rightCandidate.locale {
			return leftCandidate.locale < rightCandidate.locale
		}
		if terms[left].key != terms[right].key {
			return terms[left].key < terms[right].key
		}
		if terms[left].rank != terms[right].rank {
			return terms[left].rank > terms[right].rank
		}
		return leftCandidate.name < rightCandidate.name
	})

	fuzzySeen := make(map[string]struct{}, len(terms))
	for _, pendingTerm := range terms {
		termID := len(index.terms)
		term := nameIndexTerm(pendingTerm)
		index.terms = append(index.terms, term)
		locale := index.candidates[term.candidate].locale
		exactKey := indexLocaleKey(locale, term.key)
		index.exact[exactKey] = append(index.exact[exactKey], termID)

		fuzzyID := strconv.Itoa(term.candidate) + "\x00" + term.fuzzyKey
		if term.fuzzyKey == "" {
			continue
		}
		if _, exists := fuzzySeen[fuzzyID]; exists {
			continue
		}
		fuzzySeen[fuzzyID] = struct{}{}
		for _, gram := range indexNameNGramHashes(locale, term.fuzzyKey) {
			index.grams[gram] = append(index.grams[gram], termID)
			index.stats.GramPostings++
		}
	}

	for _, termIDs := range index.exact {
		seenCandidates := make(map[int]struct{}, len(termIDs))
		for _, termID := range termIDs {
			seenCandidates[index.terms[termID].candidate] = struct{}{}
		}
		if len(seenCandidates) > 1 {
			index.stats.Collisions++
		}
	}
	index.stats.Documents = len(index.candidates)
	index.stats.Terms = len(index.terms)
	index.stats.GramKeys = len(index.grams)
	return index
}

func (index *NameIndex) Stats() NameIndexStats {
	if index == nil {
		return NameIndexStats{GeneratorVersion: NameIndexGeneratorVersion}
	}
	return index.stats
}

func (index *NameIndex) Match(locale string, query string) NameMatch {
	query = strings.TrimSpace(query)
	canonicalLocale, ok := document.CanonicalLocale(locale)
	if index == nil || !ok || query == "" {
		return NameMatch{Kind: NameMatchNone, Query: query}
	}
	strict := strictNameKey(query)
	if match, found := index.matchExact(canonicalLocale, query, strict); found {
		return match
	}
	compact := compactNameKey(query)
	if compact != "" && compact != strict {
		if match, found := index.matchExact(canonicalLocale, query, compact); found {
			return match
		}
	}
	return index.matchFuzzy(canonicalLocale, query, compact)
}

func (index *NameIndex) matchExact(locale string, query string, key string) (NameMatch, bool) {
	termIDs := index.exact[indexLocaleKey(locale, key)]
	if len(termIDs) == 0 {
		return NameMatch{}, false
	}
	bestByCandidate := make(map[int]int, len(termIDs))
	for _, termID := range termIDs {
		candidateID := index.terms[termID].candidate
		current, exists := bestByCandidate[candidateID]
		if !exists || index.terms[termID].rank > index.terms[current].rank {
			bestByCandidate[candidateID] = termID
		}
	}
	ranked := make([]int, 0, len(bestByCandidate))
	for _, termID := range bestByCandidate {
		ranked = append(ranked, termID)
	}
	sort.SliceStable(ranked, func(left, right int) bool {
		leftTerm := index.terms[ranked[left]]
		rightTerm := index.terms[ranked[right]]
		if leftTerm.rank != rightTerm.rank {
			return leftTerm.rank > rightTerm.rank
		}
		return index.candidates[leftTerm.candidate].name < index.candidates[rightTerm.candidate].name
	})
	match := NameMatch{Kind: NameMatchExact, Query: query, Score: 1, Candidate: index.publicCandidate(ranked[0])}
	if len(ranked) > 1 && index.terms[ranked[0]].rank == index.terms[ranked[1]].rank {
		match.Kind = NameMatchAmbiguous
		match.Alternatives = index.publicCandidates(ranked, 5)
	}
	return match, true
}

func (index *NameIndex) matchFuzzy(locale string, query string, queryKey string) NameMatch {
	queryLength := len([]rune(queryKey))
	if queryLength < 4 {
		return NameMatch{Kind: NameMatchNone, Query: query}
	}
	queryGrams := queryNameNGramHashes(locale, queryKey)
	overlaps := make(map[int]int)
	for _, gram := range queryGrams {
		postings := index.grams[gram]
		if len(postings) > 256 {
			continue
		}
		for _, termID := range postings {
			overlaps[termID]++
		}
	}
	termIDs := make([]int, 0, len(overlaps))
	for termID := range overlaps {
		termIDs = append(termIDs, termID)
	}
	sort.SliceStable(termIDs, func(left, right int) bool {
		if overlaps[termIDs[left]] != overlaps[termIDs[right]] {
			return overlaps[termIDs[left]] > overlaps[termIDs[right]]
		}
		return termIDs[left] < termIDs[right]
	})
	if len(termIDs) > 64 {
		termIDs = termIDs[:64]
	}

	type fuzzyResult struct {
		termID int
		score  float64
	}
	bestByCandidate := make(map[int]fuzzyResult)
	for _, termID := range termIDs {
		term := index.terms[termID]
		jaccard := ngramJaccard(queryKey, term.fuzzyKey)
		edit := editSimilarity(queryKey, term.fuzzyKey)
		score := 0.65*jaccard + 0.35*edit
		current, exists := bestByCandidate[term.candidate]
		if !exists || score > current.score {
			bestByCandidate[term.candidate] = fuzzyResult{termID: termID, score: score}
		}
	}
	results := make([]fuzzyResult, 0, len(bestByCandidate))
	for _, result := range bestByCandidate {
		results = append(results, result)
	}
	sort.SliceStable(results, func(left, right int) bool {
		if results[left].score != results[right].score {
			return results[left].score > results[right].score
		}
		return index.candidates[index.terms[results[left].termID].candidate].name < index.candidates[index.terms[results[right].termID].candidate].name
	})
	threshold := 0.80
	if queryLength <= 6 {
		threshold = 0.88
	}
	if len(results) == 0 || results[0].score < threshold {
		return NameMatch{Kind: NameMatchNone, Query: query}
	}
	match := NameMatch{
		Kind:      NameMatchFuzzy,
		Query:     query,
		Candidate: index.publicCandidate(results[0].termID),
		Score:     results[0].score,
	}
	if len(results) > 1 && results[0].score-results[1].score < 0.08 {
		match.Kind = NameMatchAmbiguous
		limit := len(results)
		if limit > 5 {
			limit = 5
		}
		match.Alternatives = make([]NameCandidate, 0, limit)
		for _, result := range results[:limit] {
			match.Alternatives = append(match.Alternatives, index.publicCandidate(result.termID))
		}
	}
	return match
}

func (index *NameIndex) publicCandidate(termID int) NameCandidate {
	term := index.terms[termID]
	candidate := index.candidates[term.candidate]
	return NameCandidate{
		Locale:        candidate.locale,
		CanonicalName: candidate.name,
		Slug:          candidate.slug,
		MatchedName:   term.display,
		Source:        term.source,
	}
}

func (index *NameIndex) publicCandidates(termIDs []int, limit int) []NameCandidate {
	if len(termIDs) > limit {
		termIDs = termIDs[:limit]
	}
	result := make([]NameCandidate, 0, len(termIDs))
	for _, termID := range termIDs {
		result = append(result, index.publicCandidate(termID))
	}
	return result
}

func addPendingName(pending map[string]pendingNameTerm, candidate int, rawName string, source NameSource) {
	rawName = strings.TrimSpace(rawName)
	if rawName == "" {
		return
	}
	strict := strictNameKey(rawName)
	addPendingTerm(pending, pendingNameTerm{
		candidate: candidate,
		key:       strict,
		fuzzyKey:  compactNameKey(rawName),
		display:   rawName,
		source:    source,
		rank:      nameSourceRank(source),
	})
	compact := compactNameKey(rawName)
	if compact != "" && compact != strict {
		addPendingTerm(pending, pendingNameTerm{
			candidate: candidate,
			key:       compact,
			fuzzyKey:  compact,
			display:   rawName,
			source:    NameSourceGenerated,
			rank:      nameSourceRank(source) - 100,
		})
	}
}

func addPendingTerm(pending map[string]pendingNameTerm, term pendingNameTerm) {
	if term.key == "" || term.fuzzyKey == "" {
		return
	}
	key := strconv.Itoa(term.candidate) + "\x00" + term.key
	current, exists := pending[key]
	if !exists || term.rank > current.rank {
		pending[key] = term
	}
}

func nameSourceRank(source NameSource) int {
	switch source {
	case NameSourceTitle:
		return 500
	case NameSourceTechdataLabel:
		return 400
	case NameSourceTechdataAlias:
		return 300
	case NameSourceGenerated:
		return 200
	default:
		return 0
	}
}

func parentheticalBase(name string) string {
	name = norm.NFKC.String(strings.TrimSpace(name))
	if !strings.HasSuffix(name, ")") {
		return ""
	}
	opening := strings.Index(name, " (")
	if opening <= 0 || opening+2 >= len(name)-1 {
		return ""
	}
	return strings.TrimSpace(name[:opening])
}

func strictNameKey(value string) string {
	value = cases.Fold().String(norm.NFKC.String(strings.TrimSpace(value)))
	var result strings.Builder
	space := false
	for _, character := range value {
		if character == '_' || unicode.IsSpace(character) {
			space = result.Len() > 0
			continue
		}
		if space {
			result.WriteByte(' ')
			space = false
		}
		result.WriteRune(character)
	}
	return strings.TrimSpace(result.String())
}

func compactNameKey(value string) string {
	value = cases.Fold().String(norm.NFKC.String(strings.TrimSpace(value)))
	var result strings.Builder
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || strings.ContainsRune(".+#*-/", character) {
			result.WriteRune(character)
		}
	}
	return result.String()
}

func indexNameNGramHashes(locale string, value string) []uint64 {
	characters := []rune(value)
	if len(characters) == 0 {
		return nil
	}
	sizes := []int{3}
	if len(characters) <= 6 {
		sizes = append(sizes, 2)
	}
	result := make([]uint64, 0, len(characters))
	for _, size := range sizes {
		result = append(result, nameNGramHashesForSize(locale, characters, size)...)
	}
	return result
}

func queryNameNGramHashes(locale string, value string) []uint64 {
	characters := []rune(value)
	size := 3
	if len(characters) < 5 {
		size = 2
	}
	return nameNGramHashesForSize(locale, characters, size)
}

func nameNGramHashesForSize(locale string, characters []rune, size int) []uint64 {
	if len(characters) < size {
		return nil
	}
	seen := make(map[uint64]struct{}, len(characters)-size+1)
	result := make([]uint64, 0, len(characters)-size+1)
	for index := 0; index+size <= len(characters); index++ {
		gram := nameNGramHash(locale, size, characters[index:index+size])
		if _, exists := seen[gram]; exists {
			continue
		}
		seen[gram] = struct{}{}
		result = append(result, gram)
	}
	return result
}

func ngramJaccard(left string, right string) float64 {
	leftRunes := []rune(left)
	size := 3
	if len(leftRunes) < 5 {
		size = 2
	}
	leftGrams := nameNGramHashesForSize("", leftRunes, size)
	rightGrams := nameNGramHashesForSize("", []rune(right), size)
	if len(leftGrams) == 0 || len(rightGrams) == 0 {
		return 0
	}
	leftSet := make(map[uint64]struct{}, len(leftGrams))
	for _, gram := range leftGrams {
		leftSet[gram] = struct{}{}
	}
	intersection := 0
	union := len(leftSet)
	for _, gram := range rightGrams {
		if _, exists := leftSet[gram]; exists {
			intersection++
		} else {
			union++
		}
	}
	return float64(intersection) / float64(union)
}

func nameNGramHash(locale string, size int, characters []rune) uint64 {
	const (
		offset = uint64(1469598103934665603)
		prime  = uint64(1099511628211)
	)
	hash := offset
	for index := 0; index < len(locale); index++ {
		hash ^= uint64(locale[index])
		hash *= prime
	}
	hash ^= uint64(size)
	hash *= prime
	for _, character := range characters {
		value := uint32(character)
		for shift := 0; shift < 32; shift += 8 {
			hash ^= uint64(byte(value >> shift))
			hash *= prime
		}
	}
	return hash
}

func editSimilarity(left string, right string) float64 {
	leftRunes := []rune(left)
	rightRunes := []rune(right)
	longest := len(leftRunes)
	if len(rightRunes) > longest {
		longest = len(rightRunes)
	}
	if longest == 0 {
		return 1
	}
	distance := damerauLevenshtein(leftRunes, rightRunes)
	return 1 - float64(distance)/float64(longest)
}

// damerauLevenshtein computes the optimal string alignment distance using
// three rows, keeping memory proportional to the shorter input.
func damerauLevenshtein(left []rune, right []rune) int {
	if len(left) > len(right) {
		left, right = right, left
	}
	previousPrevious := make([]int, len(left)+1)
	previous := make([]int, len(left)+1)
	current := make([]int, len(left)+1)
	for index := range previous {
		previous[index] = index
	}
	for rightIndex := 1; rightIndex <= len(right); rightIndex++ {
		current[0] = rightIndex
		for leftIndex := 1; leftIndex <= len(left); leftIndex++ {
			cost := 0
			if left[leftIndex-1] != right[rightIndex-1] {
				cost = 1
			}
			current[leftIndex] = minInt(
				current[leftIndex-1]+1,
				previous[leftIndex]+1,
				previous[leftIndex-1]+cost,
			)
			if leftIndex > 1 && rightIndex > 1 && left[leftIndex-1] == right[rightIndex-2] && left[leftIndex-2] == right[rightIndex-1] {
				current[leftIndex] = minInt(current[leftIndex], previousPrevious[leftIndex-2]+1)
			}
		}
		previousPrevious, previous, current = previous, current, previousPrevious
	}
	return previous[len(left)]
}

func minInt(values ...int) int {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func indexLocaleKey(locale string, value string) string {
	return locale + "\x00" + value
}
