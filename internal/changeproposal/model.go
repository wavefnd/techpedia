// Package changeproposal defines immutable, administrator-reviewed MCP changes.
package changeproposal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"techpedia/internal/document"
	"techpedia/internal/techmark"
)

var (
	ErrInvalid        = errors.New("invalid change proposal")
	ErrNotFound       = errors.New("change proposal not found")
	ErrConflict       = errors.New("proposal no longer matches the document or language links; submit a new proposal")
	ErrUnavailable    = errors.New("proposal expired, already decided, or its connection is no longer authorized")
	ErrSourceTooLarge = errors.New("TechMark source exceeds the 2 MiB safety limit")
)

const MaxSourceBytes = 2 << 20

const DefaultRetentionDays = 2

type Policy struct {
	RetentionDays int `json:"retention_days"`
}

type Page struct {
	Locale   string `json:"locale"`
	Name     string `json:"name"`
	Revision string `json:"revision"`
}

type Input struct {
	Kind         string     `json:"kind" jsonschema:"create, edit, or link; deletion and unlinking are not supported"`
	Locale       string     `json:"locale" jsonschema:"Active base language code"`
	Name         string     `json:"name" jsonschema:"Exact canonical document name; not a WID or TID"`
	BaseRevision string     `json:"base_revision,omitempty" jsonschema:"Required rN of the current document for edit and link; empty for create"`
	Source       string     `json:"source,omitempty" jsonschema:"Complete replacement TechMark source for create/edit, up to 2 MiB of UTF-8 text; never rendered HTML"`
	Edits        []TextEdit `json:"edits,omitempty" jsonschema:"For edit only: exact replacements against base_revision, instead of source; server builds the complete proposal"`
	Summary      string     `json:"summary" jsonschema:"Short explanation for the administrator, up to 500 characters"`
	Target       *Page      `json:"target,omitempty" jsonschema:"For link only: an existing document in a different active language and its expected rN"`
}

// Snapshot identity and hashes are storage-only, never MCP/public DTO fields.
type Snapshot struct {
	WID              document.WID
	Fingerprint      string
	LinksFingerprint string
}

type Proposal struct {
	ID             string    `json:"id"`
	Input          Input     `json:"input"`
	GrantID        string    `json:"grant_id"`
	UserID         string    `json:"user_id"`
	Username       string    `json:"username"`
	Base           Snapshot  `json:"base"`
	Target         Snapshot  `json:"target"`
	Before         string    `json:"before"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	DecidedBy      string    `json:"decided_by,omitempty"`
	DecidedAt      time.Time `json:"decided_at,omitempty"`
	ResultRevision string    `json:"result_revision,omitempty"`
	Supersedes     string    `json:"supersedes,omitempty"`
	SupersededBy   string    `json:"superseded_by,omitempty"`
	AmendedAt      time.Time `json:"amended_at,omitempty"`
}

func (p Proposal) Digest() string {
	data, _ := json.Marshal(p)
	return Hash(data)
}
func Hash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func Revision(value string) bool {
	if len(value) < 2 || len(value) > 20 || value[0] != 'r' || value[1] == '0' {
		return false
	}
	for _, r := range value[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	_, err := strconv.ParseUint(value[1:], 10, 64)
	return err == nil
}
func ValidName(value string) bool {
	if !utf8.ValidString(value) || value != strings.TrimSpace(value) || value == "" || utf8.RuneCountInString(value) > 120 || techmark.Slugify(value) == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func Validate(in Input) error {
	if len(in.Source) > MaxSourceBytes {
		return ErrSourceTooLarge
	}
	locale, ok := document.CanonicalLocale(in.Locale)
	if !ok || locale != in.Locale || !ValidName(in.Name) || strings.TrimSpace(in.Summary) == "" || utf8.RuneCountInString(in.Summary) > 500 || !utf8.ValidString(in.Summary) || !utf8.ValidString(in.Source) || len(in.Source) > MaxSourceBytes {
		return ErrInvalid
	}
	switch in.Kind {
	case "create":
		if in.BaseRevision != "" || in.Target != nil || strings.TrimSpace(in.Source) == "" || len(in.Edits) != 0 {
			return ErrInvalid
		}
	case "edit":
		if !Revision(in.BaseRevision) || in.Target != nil || (len(in.Edits) == 0 && strings.TrimSpace(in.Source) == "") || (len(in.Edits) != 0 && in.Source != "") {
			return ErrInvalid
		}
		if len(in.Edits) != 0 {
			if err := ValidateEdits(in.Edits); err != nil {
				return err
			}
		}
	case "link":
		if !Revision(in.BaseRevision) || in.Source != "" || len(in.Edits) != 0 || in.Target == nil || !ValidName(in.Target.Name) || !Revision(in.Target.Revision) || in.Target.Locale == in.Locale {
			return ErrInvalid
		}
		tag, ok := document.CanonicalLocale(in.Target.Locale)
		if !ok || tag != in.Target.Locale {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	// External proposals cannot introduce privileged scripts/layout or main-page syntax.
	if techmark.ContainsSpecialSyntax(in.Source) || techmark.ContainsHomePageSyntax(in.Source) {
		return ErrInvalid
	}
	return nil
}

type Store interface {
	GetProposalPolicy(context.Context) (Policy, error)
	SetProposalPolicy(context.Context, Policy, string) error
	SubmitChangeProposal(context.Context, Input, string, string, string) (Proposal, error)
	AmendChangeProposal(context.Context, Amendment, string, string, string) (Proposal, error)
	GetChangeProposal(context.Context, string) (Proposal, error)
	// List returns metadata only; Before and Input.Source are empty.
	ListChangeProposals(context.Context) ([]Proposal, error)
	DecideChangeProposal(context.Context, string, string, bool, string, string) (Proposal, error)
}
