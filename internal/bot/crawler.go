package bot

import (
	"sort"
	"strings"

	"gonum.org/v1/gonum/graph/network"
	"gonum.org/v1/gonum/graph/simple"

	"techpedia/internal/document"
	"techpedia/internal/techmark"
)

type CrawlResult struct {
	Documents       []document.Document
	DocumentCount   int
	LinkCount       int
	MissingLinks    int
	OrphanDocuments int
}

type MissingLink struct {
	Source     document.Document
	TargetName string
	TargetSlug string
}

type LinkEdge struct {
	Source document.Document
	Target document.Document
}

type LinkAnalysis struct {
	Documents       []document.Document
	Backlinks       map[string][]document.Document
	Edges           []LinkEdge
	Missing         []MissingLink
	LinkCount       int
	OrphanDocuments []document.Document
}

// CrawlDocuments orders the analyzer frontier using the internal link graph.
func CrawlDocuments(documents []document.Document) CrawlResult {
	analysis := AnalyzeLinks(documents)
	return CrawlResult{
		Documents:       analysis.Documents,
		DocumentCount:   len(analysis.Documents),
		LinkCount:       analysis.LinkCount,
		MissingLinks:    len(analysis.Missing),
		OrphanDocuments: len(analysis.OrphanDocuments),
	}
}

// AnalyzeLinks builds a locale-aware graph. Rank is used internally to order
// analyzers and is never exposed through public or administrator DTOs.
func AnalyzeLinks(documents []document.Document) LinkAnalysis {
	pages := make([]document.LinkDocument, 0, len(documents))
	for _, doc := range documents {
		pages = append(pages, document.LinkDocument{Document: doc, Targets: techmark.InternalLinkTargets(doc.Source)})
	}
	return AnalyzeLinkDocuments(pages)
}

// AnalyzeLinkDocuments accepts storage-maintained targets so graph requests do
// not load XML article bodies or reparse their TechMark.
func AnalyzeLinkDocuments(documents []document.LinkDocument) LinkAnalysis {
	result := LinkAnalysis{
		Documents: []document.Document{},
		Backlinks: map[string][]document.Document{},
		Edges:     []LinkEdge{},
		Missing:   []MissingLink{},
	}
	if len(documents) == 0 {
		return result
	}

	graph := simple.NewDirectedGraph()
	nodes := make(map[string]int64, len(documents))
	documentsByKey := make(map[string]document.Document, len(documents))
	targetsByKey := make(map[string][]string, len(documents))
	nodeDocuments := make(map[int64]document.Document, len(documents))
	for _, raw := range documents {
		doc := document.NormalizeDocument(raw.Document)
		key := crawlDocumentKey(doc.Locale, doc.Slug)
		if _, exists := nodes[key]; exists {
			continue
		}
		node := graph.NewNode()
		graph.AddNode(node)
		nodes[key] = node.ID()
		documentsByKey[key] = doc
		targetsByKey[key] = raw.Targets
		nodeDocuments[node.ID()] = doc
	}

	incoming := make(map[int64]int, len(nodes))
	seenEdges := make(map[string]struct{})
	seenMissing := make(map[string]struct{})
	for key, doc := range documentsByKey {
		fromID := nodes[key]
		for _, target := range targetsByKey[key] {
			targetName := strings.TrimSpace(strings.ReplaceAll(target, "_", " "))
			targetSlug := techmark.Slugify(targetName)
			targetKey := crawlDocumentKey(doc.Locale, targetSlug)
			toID, exists := nodes[targetKey]
			if !exists {
				missingKey := key + "\x00" + targetKey
				if _, seen := seenMissing[missingKey]; !seen {
					seenMissing[missingKey] = struct{}{}
					result.Missing = append(result.Missing, MissingLink{Source: doc, TargetName: targetName, TargetSlug: targetSlug})
				}
				continue
			}
			edgeKey := key + "\x00" + targetKey
			if _, seen := seenEdges[edgeKey]; seen {
				continue
			}
			seenEdges[edgeKey] = struct{}{}
			if fromID != toID {
				graph.SetEdge(graph.NewEdge(graph.Node(fromID), graph.Node(toID)))
				incoming[toID]++
				result.Backlinks[targetKey] = append(result.Backlinks[targetKey], doc)
				result.Edges = append(result.Edges, LinkEdge{Source: doc, Target: documentsByKey[targetKey]})
			}
			result.LinkCount++
		}
	}

	for nodeID, doc := range nodeDocuments {
		if incoming[nodeID] == 0 {
			result.OrphanDocuments = append(result.OrphanDocuments, doc)
		}
	}
	ranks := network.PageRank(graph, 0.85, 1e-8)
	result.Documents = make([]document.Document, 0, len(nodeDocuments))
	for _, doc := range nodeDocuments {
		result.Documents = append(result.Documents, doc)
	}
	sort.SliceStable(result.Documents, func(i, j int) bool {
		iRank := ranks[nodes[crawlDocumentKey(result.Documents[i].Locale, result.Documents[i].Slug)]]
		jRank := ranks[nodes[crawlDocumentKey(result.Documents[j].Locale, result.Documents[j].Slug)]]
		if iRank != jRank {
			return iRank > jRank
		}
		return documentLess(result.Documents[i], result.Documents[j])
	})
	for key := range result.Backlinks {
		sort.SliceStable(result.Backlinks[key], func(i, j int) bool {
			return documentLess(result.Backlinks[key][i], result.Backlinks[key][j])
		})
	}
	sort.SliceStable(result.Missing, func(i, j int) bool {
		if documentLess(result.Missing[i].Source, result.Missing[j].Source) {
			return true
		}
		if documentLess(result.Missing[j].Source, result.Missing[i].Source) {
			return false
		}
		return strings.ToLower(result.Missing[i].TargetName) < strings.ToLower(result.Missing[j].TargetName)
	})
	sort.SliceStable(result.OrphanDocuments, func(i, j int) bool {
		return documentLess(result.OrphanDocuments[i], result.OrphanDocuments[j])
	})
	return result
}

func (analysis LinkAnalysis) BacklinksFor(locale string, slug string) []document.Document {
	return append([]document.Document(nil), analysis.Backlinks[crawlDocumentKey(locale, slug)]...)
}

func crawlDocumentKey(locale string, slug string) string {
	return document.NormalizeLocale(locale) + "\x00" + strings.TrimSpace(slug)
}

func documentLess(left document.Document, right document.Document) bool {
	if left.Locale != right.Locale {
		return left.Locale < right.Locale
	}
	return strings.ToLower(left.Title) < strings.ToLower(right.Title)
}
