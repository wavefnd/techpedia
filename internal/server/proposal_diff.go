package server

import (
	"strconv"
	"strings"
)

type proposalDiff struct {
	Lines       []revisionDiffLine `json:"lines"`
	Offset      int                `json:"diff_offset"`
	Limit       int                `json:"diff_limit"`
	Total       int                `json:"diff_total"`
	Added       int                `json:"added_lines"`
	Removed     int                `json:"removed_lines"`
	ChangesOnly bool               `json:"changes_only"`
}

// Page the whole source, not just its beginning. Trim common edges before LCS;
// very large replacement regions use linear remove/add comparison instead.
func proposalDiffPage(before, after string, offset, limit int, changesOnly bool) proposalDiff {
	result := proposalDiff{Lines: []revisionDiffLine{}, Offset: offset, Limit: limit, ChangesOnly: changesOnly}
	split := func(source string) []string {
		if source == "" {
			return nil
		}
		return strings.Split(source, "\n")
	}
	old, next := split(before), split(after)
	prefix := 0
	for prefix < len(old) && prefix < len(next) && old[prefix] == next[prefix] {
		prefix++
	}
	oldEnd, newEnd := len(old), len(next)
	for oldEnd > prefix && newEnd > prefix && old[oldEnd-1] == next[newEnd-1] {
		oldEnd--
		newEnd--
	}
	emit := func(line revisionDiffLine) {
		if line.Kind == "added" {
			result.Added++
		}
		if line.Kind == "removed" {
			result.Removed++
		}
		if result.Total >= offset && len(result.Lines) < limit {
			result.Lines = append(result.Lines, line)
		}
		result.Total++
	}
	equal := func(lines []string, oldStart, newStart int, edge string) {
		gapStart, gapEnd := 3, len(lines)-3
		if edge == "prefix" {
			gapStart = 0
		}
		if edge == "suffix" {
			gapEnd = len(lines)
		}
		if edge == "unchanged" {
			gapStart, gapEnd = 0, len(lines)
		}
		for i := 0; i < len(lines); i++ {
			if changesOnly && gapEnd-gapStart > 0 && len(lines) > 6 && i == gapStart {
				emit(revisionDiffLine{Kind: "context", OldLine: oldStart + i, NewLine: newStart + i, Text: "변경 없는 줄 " + strconv.Itoa(gapEnd-gapStart) + "개"})
				i = gapEnd - 1
				continue
			}
			emit(revisionDiffLine{Kind: "equal", OldLine: oldStart + i, NewLine: newStart + i, Text: lines[i]})
		}
	}
	if prefix == len(old) && prefix == len(next) {
		equal(old, 1, 1, "unchanged")
		return result
	}
	equal(old[:prefix], 1, 1, "prefix")
	a, b := old[prefix:oldEnd], next[prefix:newEnd]
	if len(a) == 0 || len(b) == 0 || len(a)+len(b) > 4000 || len(a)*len(b) > 2_000_000 {
		for i, text := range a {
			emit(revisionDiffLine{Kind: "removed", OldLine: prefix + i + 1, Text: text})
		}
		for i, text := range b {
			emit(revisionDiffLine{Kind: "added", NewLine: prefix + i + 1, Text: text})
		}
	} else {
		middle := lineDiff(strings.Join(a, "\n"), strings.Join(b, "\n"))
		for i := 0; i < len(middle); {
			line := middle[i]
			if line.Kind == "equal" {
				end := i + 1
				for end < len(middle) && middle[end].Kind == "equal" {
					end++
				}
				texts := make([]string, end-i)
				for j := i; j < end; j++ {
					texts[j-i] = middle[j].Text
				}
				equal(texts, line.OldLine+prefix, line.NewLine+prefix, "middle")
				i = end
			} else {
				if line.OldLine > 0 {
					line.OldLine += prefix
				}
				if line.NewLine > 0 {
					line.NewLine += prefix
				}
				emit(line)
				i++
			}
		}
	}
	equal(old[oldEnd:], oldEnd+1, newEnd+1, "suffix")
	return result
}
