package changeproposal

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Every replacement is located in the same original source, never in another
// replacement's output. Empty After removes a passage, not a wiki document.
type TextEdit struct {
	Before     string `json:"before" jsonschema:"Exact nonempty original TechMark passage, including enough context to match uniquely"`
	After      string `json:"after" jsonschema:"Replacement TechMark; empty removes only the matched passage"`
	Occurrence int    `json:"occurrence,omitempty" jsonschema:"Optional 1-based occurrence in the original source; omit only when before matches exactly once"`
}

type Amendment struct {
	ProposalID string     `json:"proposal_id" jsonschema:"Own pending create/edit proposal to replace; not an applied or rejected proposal"`
	Source     string     `json:"source,omitempty" jsonschema:"Complete corrected proposed TechMark, instead of edits"`
	Edits      []TextEdit `json:"edits,omitempty" jsonschema:"Exact replacements against the submitted proposal source, not the live document; instead of source"`
	Summary    string     `json:"summary" jsonschema:"Updated explanation for administrator review"`
}

func ValidateEdits(edits []TextEdit) error {
	if len(edits) == 0 || len(edits) > 256 {
		return ErrInvalid
	}
	size := 0
	for _, edit := range edits {
		if edit.Before == "" || !utf8.ValidString(edit.Before) || !utf8.ValidString(edit.After) || edit.Occurrence < 0 {
			return ErrInvalid
		}
		size += len(edit.Before) + len(edit.After)
		if size > MaxSourceBytes {
			return ErrSourceTooLarge
		}
	}
	return nil
}

func ApplyEdits(source string, edits []TextEdit) (string, error) {
	if err := ValidateEdits(edits); err != nil {
		return "", err
	}
	if len(source) > MaxSourceBytes {
		return "", ErrSourceTooLarge
	}
	if !utf8.ValidString(source) {
		return "", ErrInvalid
	}
	type replacement struct {
		start, end int
		after      string
	}
	values := make([]replacement, 0, len(edits))
	size := len(source)
	for index, edit := range edits {
		count := strings.Count(source, edit.Before)
		first := strings.Index(source, edit.Before)
		ambiguous := edit.Occurrence == 0 && first >= 0 && strings.Contains(source[first+1:], edit.Before)
		if count == 0 || ambiguous || edit.Occurrence > count {
			return "", fmt.Errorf("%w: edit %d must match exactly once; otherwise specify its 1-based occurrence", ErrConflict, index+1)
		}
		start, remaining := 0, max(1, edit.Occurrence)
		for remaining > 0 {
			position := strings.Index(source[start:], edit.Before)
			start += position
			remaining--
			if remaining > 0 {
				start += len(edit.Before)
			}
		}
		values = append(values, replacement{start, start + len(edit.Before), edit.After})
		size += len(edit.After) - len(edit.Before)
	}
	if size > MaxSourceBytes {
		return "", ErrSourceTooLarge
	}
	sort.Slice(values, func(i, j int) bool { return values[i].start < values[j].start })
	for index := 1; index < len(values); index++ {
		if values[index].start < values[index-1].end {
			return "", fmt.Errorf("%w: overlapping edits", ErrConflict)
		}
	}
	var result strings.Builder
	result.Grow(max(0, size))
	previous := 0
	for _, value := range values {
		result.WriteString(source[previous:value.start])
		result.WriteString(value.after)
		previous = value.end
	}
	result.WriteString(source[previous:])
	return result.String(), nil
}
