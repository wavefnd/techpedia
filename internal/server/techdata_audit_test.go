package server

import (
	"testing"
	"time"

	"techpedia/internal/document"
	"techpedia/internal/techdata"
)

func TestBuildTechdataAuditFindsUnlinkedAndCrossLocaleCandidates(t *testing.T) {
	now := time.Date(2026, 7, 16, 1, 2, 3, 0, time.UTC)
	documents := []document.Document{
		{ID: "W1", Locale: "en", Slug: "rust", Title: "Rust", UpdatedAt: now},
		{ID: "W2", Locale: "ko", Slug: "rust", Title: "Rust", UpdatedAt: now.Add(time.Minute)},
		{ID: "W3", Locale: "ko", Slug: "에이", Title: "에이", UpdatedAt: now.Add(2 * time.Minute)},
		{ID: "W4", Locale: "en", Slug: "a", Title: "A", UpdatedAt: now.Add(3 * time.Minute)},
		{ID: "W5", Locale: "en", Slug: "c", Title: "C", UpdatedAt: now.Add(4 * time.Minute)},
		{ID: "W6", Locale: "ko", Slug: "c++", Title: "C++", UpdatedAt: now.Add(5 * time.Minute)},
	}
	items := []techdata.Item{{
		ID:        "T1",
		Labels:    map[string]string{"en": "A", "ko": "에이"},
		Aliases:   map[string][]string{"ko": {"A"}},
		WikiPages: map[string]document.WID{"en": "W4"},
	}}

	report := buildTechdataAudit(documents, items)
	if report.Documents != 6 || report.LinkedDocuments != 1 || len(report.UnlinkedDocuments) != 5 {
		t.Fatalf("unexpected document totals: %#v", report)
	}
	if len(report.MetadataCandidates) != 1 {
		t.Fatalf("expected one metadata candidate, got %#v", report.MetadataCandidates)
	}
	metadata := report.MetadataCandidates[0]
	if metadata.Page.WID != "W3" || metadata.TID != "T1" || metadata.Label != "에이" {
		t.Fatalf("unexpected metadata candidate: %#v", metadata)
	}
	if len(metadata.LinkedLocales) != 1 || metadata.LinkedLocales[0] != "en" {
		t.Fatalf("unexpected linked locales: %#v", metadata.LinkedLocales)
	}
	if len(report.NameCandidates) != 1 || len(report.NameCandidates[0].Pages) != 2 {
		t.Fatalf("expected the two Rust documents to be grouped: %#v", report.NameCandidates)
	}
	if report.NameCandidates[0].Pages[0].Locale != "en" || report.NameCandidates[0].Pages[1].Locale != "ko" {
		t.Fatalf("candidate pages are not sorted by locale: %#v", report.NameCandidates[0].Pages)
	}
}

func TestNormalizeAuditNamePreservesTechnicalPunctuation(t *testing.T) {
	if normalizeAuditName("Ｃ ++") != normalizeAuditName("C++") {
		t.Fatal("expected width and spacing differences to normalize")
	}
	if normalizeAuditName("C") == normalizeAuditName("C++") {
		t.Fatal("technical punctuation must keep C and C++ separate")
	}
}
