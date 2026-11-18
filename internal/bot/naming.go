package bot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"

	"techpedia/internal/document"
	"techpedia/internal/techmark"
)

var (
	ErrInvalid  = errors.New("invalid naming bot value")
	ErrNotFound = errors.New("naming bot value not found")
	ErrReviewed = errors.New("naming suggestion already reviewed")
	ErrStale    = errors.New("naming suggestion is stale")
)

const NamingSuggestionSchemaVersion = 2

type SuggestionStatus string

const (
	SuggestionOpen     SuggestionStatus = "open"
	SuggestionAccepted SuggestionStatus = "accepted"
	SuggestionRejected SuggestionStatus = "rejected"
)

type Finding struct {
	CheckerID string        `json:"checker_id"`
	Area      string        `json:"area"`
	Before    string        `json:"before"`
	After     string        `json:"after"`
	MatchKind NameMatchKind `json:"match_kind"`
	Source    NameSource    `json:"source"`
}

type NamingSuggestion struct {
	SchemaVersion  int              `json:"schema_version"`
	ID             string           `json:"id"`
	Locale         string           `json:"locale"`
	Slug           string           `json:"slug"`
	PageName       string           `json:"page_name"`
	BaseRevision   string           `json:"base_revision"`
	OriginalSource string           `json:"original_source"`
	ProposedName   string           `json:"proposed_name"`
	ProposedSource string           `json:"proposed_source"`
	Findings       []Finding        `json:"findings"`
	Status         SuggestionStatus `json:"status"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	ReviewedBy     string           `json:"reviewed_by,omitempty"`
	ReviewedAt     time.Time        `json:"reviewed_at,omitempty"`
}

type Store interface {
	CreateNamingSuggestion(ctx context.Context, suggestion NamingSuggestion) (NamingSuggestion, bool, error)
	GetNamingSuggestion(ctx context.Context, id string) (NamingSuggestion, error)
	ListNamingSuggestions(ctx context.Context, status SuggestionStatus, limit int) ([]NamingSuggestion, error)
	ReviewNamingSuggestion(ctx context.Context, id string, status SuggestionStatus, reviewedBy string, proposedName string, proposedSource string) (NamingSuggestion, error)
}

func ValidSuggestionStatus(status SuggestionStatus) bool {
	return status == SuggestionOpen || status == SuggestionAccepted || status == SuggestionRejected
}

type sourceEdit struct {
	start       int
	end         int
	replacement string
}

// ScanDocument uses the immutable title index to propose canonical link
// targets. It does not rewrite prose or display labels, and never persists a
// change without the existing administrator review path.
func ScanDocument(raw document.Document, index *NameIndex) (NamingSuggestion, bool) {
	doc := document.NormalizeDocument(raw)
	links := techmark.InternalLinks(doc.Source)
	edits := make([]sourceEdit, 0)
	findings := make([]Finding, 0)
	seenFindings := make(map[string]struct{})

	for _, link := range links {
		match := index.Match(doc.Locale, link.Target)
		if !match.Actionable() || match.Candidate.CanonicalName == link.Target {
			continue
		}
		canonical := match.Candidate.CanonicalName
		if link.HasLabel {
			edits = append(edits, sourceEdit{start: link.TargetStart, end: link.TargetEnd, replacement: canonical})
		} else {
			display := strings.TrimSpace(doc.Source[link.TargetStart:link.TargetEnd])
			edits = append(edits, sourceEdit{
				start:       link.Start,
				end:         link.End,
				replacement: "[[" + canonical + "|" + display + "]]",
			})
		}
		addFinding(&findings, seenFindings, Finding{
			CheckerID: "name-index:v1",
			Area:      "link",
			Before:    link.Target,
			After:     canonical,
			MatchKind: match.Kind,
			Source:    match.Candidate.Source,
		})
	}

	if len(edits) == 0 {
		return NamingSuggestion{}, false
	}
	proposedSource := applySourceEdits(doc.Source, edits)
	suggestion := NamingSuggestion{
		SchemaVersion:  NamingSuggestionSchemaVersion,
		Locale:         doc.Locale,
		Slug:           doc.Slug,
		PageName:       doc.Title,
		BaseRevision:   doc.Revision,
		OriginalSource: doc.Source,
		ProposedName:   doc.Title,
		ProposedSource: proposedSource,
		Findings:       findings,
		Status:         SuggestionOpen,
	}
	suggestion.ID = suggestionID(suggestion)
	return suggestion, true
}

func applySourceEdits(source string, edits []sourceEdit) string {
	sort.SliceStable(edits, func(left, right int) bool { return edits[left].start > edits[right].start })
	for _, edit := range edits {
		if edit.start < 0 || edit.start > edit.end || edit.end > len(source) {
			continue
		}
		source = source[:edit.start] + edit.replacement + source[edit.end:]
	}
	return source
}

func suggestionID(value NamingSuggestion) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		value.Locale, value.Slug, value.BaseRevision, value.ProposedName, value.ProposedSource,
	}, "\x00")))
	return "ns2_" + hex.EncodeToString(digest[:12])
}

func addFinding(findings *[]Finding, seen map[string]struct{}, finding Finding) {
	key := strings.Join([]string{finding.CheckerID, finding.Area, finding.Before, finding.After}, "\x00")
	if _, exists := seen[key]; exists {
		return
	}
	seen[key] = struct{}{}
	*findings = append(*findings, finding)
}
