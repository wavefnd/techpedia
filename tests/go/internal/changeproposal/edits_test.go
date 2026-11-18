package changeproposal

import (
	"errors"
	"strings"
	"testing"
)

func TestExactEditsShareOneOriginalBaseline(t *testing.T) {
	source := "# 제목\n한글🙂 A\nA 끝"
	result, err := ApplyEdits(source, []TextEdit{{Before: "한글🙂", After: "다국어 العربية"}, {Before: "A", After: "B", Occurrence: 2}})
	if err != nil || result != "# 제목\n다국어 العربية A\nB 끝" {
		t.Fatal(result, err)
	}
	result, err = ApplyEdits("a b", []TextEdit{{Before: "a", After: "b"}, {Before: "b", After: "c"}})
	if err != nil || result != "b c" {
		t.Fatal("cascading replacements", result, err)
	}
	result, err = ApplyEdits("keep remove", []TextEdit{{Before: " remove", After: ""}})
	if err != nil || result != "keep" {
		t.Fatal(result, err)
	}
}

func TestExactEditsRejectMissingAmbiguousOverlappingAndUnsafeInputs(t *testing.T) {
	if _, err := ApplyEdits("ababa", []TextEdit{{Before: "aba", After: "x"}}); !errors.Is(err, ErrConflict) {
		t.Fatal("overlapping matches were considered unique", err)
	}
	for _, edits := range [][]TextEdit{
		{{Before: "missing", After: "x"}}, {{Before: "a", After: "x"}},
		{{Before: "a", After: "x", Occurrence: 3}}, {{Before: "a b", After: "x"}, {Before: "b a", After: "y"}},
	} {
		if _, err := ApplyEdits("a b a", edits); !errors.Is(err, ErrConflict) {
			t.Fatal(edits, err)
		}
	}
	for _, edits := range [][]TextEdit{nil, {{Before: "", After: "x"}}, {{Before: "x", After: string([]byte{0xff})}}, {{Before: "x", Occurrence: -1}}, make([]TextEdit, 257)} {
		if _, err := ApplyEdits("x", edits); !errors.Is(err, ErrInvalid) {
			t.Fatal(edits, err)
		}
	}
	if _, err := ApplyEdits(strings.Repeat("a", MaxSourceBytes), []TextEdit{{Before: "a", After: "aa", Occurrence: 1}}); !errors.Is(err, ErrSourceTooLarge) {
		t.Fatal(err)
	}
}

func TestPartialInputIsEditOnlyAndMutuallyExclusiveWithSource(t *testing.T) {
	in := Input{Kind: "edit", Locale: "ko", Name: "문서", BaseRevision: "r1", Summary: "부분 수정", Edits: []TextEdit{{Before: "a", After: "b"}}}
	if err := Validate(in); err != nil {
		t.Fatal(err)
	}
	in.Source = "full"
	if err := Validate(in); !errors.Is(err, ErrInvalid) {
		t.Fatal("mixed inputs accepted", err)
	}
	in.Source, in.Kind, in.BaseRevision = "", "create", ""
	if err := Validate(in); !errors.Is(err, ErrInvalid) {
		t.Fatal("create patch accepted", err)
	}
}
