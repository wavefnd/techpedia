package server

import (
	"fmt"
	"strings"
	"testing"
)

func TestProposalDiffFindsChangesBeyondOldTruncation(t *testing.T) {
	old := make([]string, 6000)
	for i := range old {
		old[i] = fmt.Sprintf("unchanged %d", i+1)
	}
	next := append([]string(nil), old...)
	next[5000] = "changed at 5001"
	diff := proposalDiffPage(strings.Join(old, "\n"), strings.Join(next, "\n"), 0, 500, true)
	if diff.Added != 1 || diff.Removed != 1 || len(diff.Lines) > 12 {
		t.Fatalf("unexpected compact comparison: %+v", diff)
	}
	for _, line := range diff.Lines {
		if line.Kind == "added" && line.NewLine == 5001 && line.Text == next[5000] {
			return
		}
	}
	t.Fatal("change at document end was hidden")
}

func TestProposalDiffPagingPreservesEveryLine(t *testing.T) {
	before := strings.Repeat("old line\n", 1200)
	after := strings.Repeat("new line\n", 1300)
	var old, next []string
	for offset := 0; ; offset += 500 {
		diff := proposalDiffPage(before, after, offset, 500, false)
		if len(diff.Lines) > 500 {
			t.Fatal("unbounded DOM page")
		}
		for _, line := range diff.Lines {
			if line.Kind != "added" {
				old = append(old, line.Text)
			}
			if line.Kind != "removed" {
				next = append(next, line.Text)
			}
		}
		if offset+len(diff.Lines) >= diff.Total {
			break
		}
	}
	if strings.Join(old, "\n") != before || strings.Join(next, "\n") != after {
		t.Fatal("paged comparison lost source content")
	}
}

func TestProposalDiffContextDoesNotHideChangedMiddle(t *testing.T) {
	before := "old start\n" + strings.Repeat("same\n", 200) + "old end"
	after := "new start\n" + strings.Repeat("same\n", 200) + "new end"
	diff := proposalDiffPage(before, after, 0, 500, true)
	if diff.Added != 2 || diff.Removed != 2 || len(diff.Lines) != 11 {
		t.Fatalf("unexpected context fold: %+v", diff)
	}
	created := proposalDiffPage("", "new", 0, 500, true)
	if created.Added != 1 || created.Removed != 0 {
		t.Fatal(created)
	}
	unchanged := proposalDiffPage(strings.Repeat("same\n", 3000), strings.Repeat("same\n", 3000), 0, 500, true)
	if len(unchanged.Lines) != 1 || unchanged.Lines[0].Kind != "context" {
		t.Fatal("unchanged lines were not folded")
	}
}
