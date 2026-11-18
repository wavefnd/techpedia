package bot

import (
	"testing"

	"techpedia/internal/document"
)

func TestCrawlDocumentsBuildsAndRanksWikiGraph(t *testing.T) {
	documents := []document.Document{
		{Locale: "ko", Slug: "a", Title: "A", Source: "[[B]] [[없는 문서]]"},
		{Locale: "ko", Slug: "b", Title: "B", Source: "[[C]]"},
		{Locale: "ko", Slug: "c", Title: "C", Source: "내용"},
	}
	result := CrawlDocuments(documents)
	if result.DocumentCount != 3 || result.LinkCount != 2 || result.MissingLinks != 1 || result.OrphanDocuments != 1 {
		t.Fatalf("unexpected crawl statistics: %#v", result)
	}
	if len(result.Documents) != 3 || result.Documents[0].Title != "C" {
		t.Fatalf("expected PageRank frontier to prioritize C, got %#v", result.Documents)
	}
}

func TestAnalyzeLinksReportsBacklinksAndIgnoresSelfLinksForOrphans(t *testing.T) {
	documents := []document.Document{
		{Locale: "ko", Slug: "a", Title: "A", Source: "[[B]]"},
		{Locale: "ko", Slug: "b", Title: "B", Source: "[[B]] [[Missing Page]]"},
	}
	analysis := AnalyzeLinks(documents)
	backlinks := analysis.BacklinksFor("ko", "b")
	if len(backlinks) != 1 || backlinks[0].Title != "A" {
		t.Fatalf("unexpected backlinks: %#v", backlinks)
	}
	if len(analysis.Missing) != 1 || analysis.Missing[0].TargetName != "Missing Page" {
		t.Fatalf("unexpected missing links: %#v", analysis.Missing)
	}
	if len(analysis.OrphanDocuments) != 1 || analysis.OrphanDocuments[0].Title != "A" {
		t.Fatalf("unexpected orphan documents: %#v", analysis.OrphanDocuments)
	}
	if len(analysis.Edges) != 1 || analysis.Edges[0].Source.Title != "A" || analysis.Edges[0].Target.Title != "B" {
		t.Fatalf("unexpected graph edges: %#v", analysis.Edges)
	}
}
