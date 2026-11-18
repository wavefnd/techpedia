package changeproposal

import (
	"errors"
	"strings"
	"testing"
)

func TestProposalInputExcludesDeletionPrivilegeAndInvalidLocales(t *testing.T) {
	base := Input{Kind: "create", Locale: "fr", Name: "Article", Source: "# Article\n[[한글🙂]]", Summary: "설명"}
	if err := Validate(base); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"delete", "unlink", "move", "merge", "approve", "commit", ""} {
		in := base
		in.Kind = kind
		if Validate(in) == nil {
			t.Fatal("accepted", kind)
		}
	}
	for _, locale := range []string{"zh-Hans", "en-US", "sr-Latn", "", "und"} {
		in := base
		in.Locale = locale
		if Validate(in) == nil {
			t.Fatal("accepted locale", locale)
		}
	}
	for _, source := range []string{"", " \n", "<wiki-home-grid>", "<wiki-recent-changes>", strings.Repeat("a", MaxSourceBytes+1), string([]byte{0xff})} {
		in := base
		in.Source = source
		if Validate(in) == nil {
			t.Fatal("accepted source")
		}
	}
	for _, revision := range []string{"r0", "r01", "r-1", "r+1", "r1x", "opaque", ""} {
		if Revision(revision) {
			t.Fatal("invalid revision", revision)
		}
	}
	for _, revision := range []string{"r1", "r10", "r999"} {
		if !Revision(revision) {
			t.Fatal(revision)
		}
	}
}

func TestProposalLongUTF8SourceAndSafetyLimit(t *testing.T) {
	in := Input{Kind: "create", Locale: "ko", Name: "긴 문서", Summary: "큰 원문"}
	in.Source = strings.Repeat("한", MaxSourceBytes/3) + strings.Repeat("a", MaxSourceBytes%3)
	if len(in.Source) != MaxSourceBytes || Validate(in) != nil {
		t.Fatal("valid 2 MiB source was rejected")
	}
	in.Source += "a"
	if !errors.Is(Validate(in), ErrSourceTooLarge) {
		t.Fatal("oversize source did not return a clear size error")
	}
}
