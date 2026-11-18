package techmark

import (
	"reflect"
	"testing"
)

func TestInternalLinkTargetsSkipsCodeExamples(t *testing.T) {
	source := "[[C]]와 [[Rust|러스트]]\n\n`[[인라인 예시]]`\n\n```techmark id=\"links-example\"\n[[코드 블록 예시]]\n```\n[[Go (프로그래밍 언어)]]\n\n<wiki-navbox title=\"언어\">\n- 계열: [[C++]], [[D (프로그래밍 언어)]]\n</wiki-navbox>\n\n<wiki-tree title=\"계보\">\n[[UNIX]]\n└── [[BSD]]\n</wiki-tree>\n\n<wiki-flow title=\"흐름\">\n[[획득]]\n↓\n[[반환]]\n</wiki-flow>\n[[programming language]]"
	want := []string{"C", "Rust", "Go (프로그래밍 언어)", "C++", "D (프로그래밍 언어)", "UNIX", "BSD", "획득", "반환", "Programming language"}
	if got := InternalLinkTargets(source); !reflect.DeepEqual(got, want) {
		t.Fatalf("InternalLinkTargets() = %#v, want %#v", got, want)
	}
}

func TestInternalLinkTargetsRecognizesPunctuatedCodeFenceLanguage(t *testing.T) {
	source := "```objective-c++ id=\"example\"\n[[Foundation]]\n```\n[[Objective-C++]]"
	want := []string{"Objective-C++"}
	if got := InternalLinkTargets(source); !reflect.DeepEqual(got, want) {
		t.Fatalf("InternalLinkTargets() = %#v, want %#v", got, want)
	}
}

func TestInternalLinksReturnsTargetSpansAndPreservesLabels(t *testing.T) {
	source := "앞 [[ c 언어 |표시 이름]] 뒤 [[Rust]]"
	links := InternalLinks(source)
	if len(links) != 2 {
		t.Fatalf("InternalLinks() returned %d links: %#v", len(links), links)
	}
	first := links[0]
	if first.Target != "C 언어" || first.Label != "표시 이름" || !first.HasLabel {
		t.Fatalf("unexpected first link: %#v", first)
	}
	if got := source[first.TargetStart:first.TargetEnd]; got != "c 언어" {
		t.Fatalf("target span = %q", got)
	}
	if got := source[first.Start:first.End]; got != "[[ c 언어 |표시 이름]]" {
		t.Fatalf("link span = %q", got)
	}
	if links[1].HasLabel || links[1].Target != "Rust" {
		t.Fatalf("unexpected second link: %#v", links[1])
	}
}

func TestInternalLinksSkipsVariableLengthInlineCode(t *testing.T) {
	source := "``[[예제]]`` [[실제 문서]] `닫히지 않은 [[예제]]"
	want := []string{"실제 문서"}
	if got := InternalLinkTargets(source); !reflect.DeepEqual(got, want) {
		t.Fatalf("InternalLinkTargets() = %#v, want %#v", got, want)
	}
}

func TestInternalLinksIgnoresEmptyLabelsLikeTheRenderer(t *testing.T) {
	source := "[[C|]] [[Rust|Rust language]]"
	want := []string{"Rust"}
	if got := InternalLinkTargets(source); !reflect.DeepEqual(got, want) {
		t.Fatalf("InternalLinkTargets() = %#v, want %#v", got, want)
	}
}
