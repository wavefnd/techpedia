package techmark

import (
	"crypto/sha256"
	"strings"
	"testing"
)

func TestExcerptSkipsRepeatedTitleAndStructuralMarkup(t *testing.T) {
	source := `C++

<wiki-infobox title="C++">
| 종류 = 프로그래밍 언어
</wiki-infobox>

C++는 [[C (프로그래밍 언어)|C]]에서 파생된 범용 프로그래밍 언어이다.<wiki-footnote>출처</wiki-footnote>`

	got := Excerpt(source, "C++", 80)
	want := "C++는 C에서 파생된 범용 프로그래밍 언어이다."
	if got != want {
		t.Fatalf("Excerpt() = %q, want %q", got, want)
	}
}

func TestExcerptSkipsWikiImageBlock(t *testing.T) {
	source := `# WaveOS

<wiki-image file="WaveOS_Example_Kernel.webp" layout="frame" alt="WaveOS kernel">
WaveOS 커널 화면
</wiki-image>

WaveOS는 운영체제 개발 프로젝트이다.`

	if got, want := Excerpt(source, "WaveOS", 80), "WaveOS는 운영체제 개발 프로젝트이다."; got != want {
		t.Fatalf("Excerpt() = %q, want %q", got, want)
	}
}

func TestExcerptSkipsHeadingCodeAndTable(t *testing.T) {
	source := `# C (프로그래밍 언어)

` + "```c\nint main(void) {}\n```" + `

| 이름 | 값 |
| --- | --- |
| C | 언어 |

C는 Dennis Ritchie가 설계한 프로그래밍 언어이다.`

	if got, want := Excerpt(source, "C (프로그래밍 언어)", 80), "C는 Dennis Ritchie가 설계한 프로그래밍 언어이다."; got != want {
		t.Fatalf("Excerpt() = %q, want %q", got, want)
	}
}

func TestExcerptSkipsHorizontalRule(t *testing.T) {
	if got, want := Excerpt("---\n\n실제 문서 설명", "문서", 80), "실제 문서 설명"; got != want {
		t.Fatalf("Excerpt() = %q, want %q", got, want)
	}
}

func TestExcerptTruncatesByRuneAndCanBeEmpty(t *testing.T) {
	if got := Excerpt("Rust\n\n가나다라마바사", "Rust", 4); got != "가나다라..." {
		t.Fatalf("unexpected rune truncation: %q", got)
	}
	if got := Excerpt("# Rust\n\nRust", "Rust", 30); got != "" {
		t.Fatalf("title-only source must not repeat the title: %q", got)
	}
}

func TestExcerptPreservesTechnicalAngleBracketNames(t *testing.T) {
	if got, want := Excerpt("# vector\n\n`<vector>`는 C++ 표준 라이브러리 컨테이너이다.", "vector", 80), "<vector>는 C++ 표준 라이브러리 컨테이너이다."; got != want {
		t.Fatalf("Excerpt() = %q, want %q", got, want)
	}
}

func TestExcerptRemovesMultilineFootnotesOutsideCode(t *testing.T) {
	source := `C++는 범용 프로그래밍 언어이다.<wiki-footnote>
[Bjarne Stroustrup](https://www.stroustrup.com/)
</wiki-footnote>

다음 문단`

	if got, want := Excerpt(source, "C++", 80), "C++는 범용 프로그래밍 언어이다."; got != want {
		t.Fatalf("Excerpt() = %q, want %q", got, want)
	}
}

func TestExcerptRecognizesPunctuatedCodeFenceLanguage(t *testing.T) {
	source := "```objective-c++ id=\"example\"\n#import <Foundation/Foundation.h>\n```\n\n실제 문서 설명"
	if got, want := Excerpt(source, "Objective-C++", 80), "실제 문서 설명"; got != want {
		t.Fatalf("Excerpt() = %q, want %q", got, want)
	}
}

func TestExcerptDoesNotTreatInlineCodeAsStructuralBlock(t *testing.T) {
	source := "`<wiki-infobox>`는 문서 정보상자 문법이다."
	if got, want := Excerpt(source, "TechMark", 80), "<wiki-infobox>는 문서 정보상자 문법이다."; got != want {
		t.Fatalf("Excerpt() = %q, want %q", got, want)
	}
}

func TestParserRenderCacheIsBounded(t *testing.T) {
	parser := NewParser()
	want := parser.RenderWithLocale("## 제목\n\n본문", "ko")
	if got := parser.RenderWithLocale("## 제목\n\n본문", "ko"); got != want {
		t.Fatalf("cached render changed output: %q", got)
	}
	for index := 0; index < 300; index++ {
		parser.Render(strings.Repeat("x", index+1))
	}
	if got := len(parser.cache.entries); got != parser.cache.limit {
		t.Fatalf("render cache size = %d, want %d", got, parser.cache.limit)
	}
	if parser.cache.bytes > parser.cache.maxBytes {
		t.Fatalf("render cache bytes = %d, limit %d", parser.cache.bytes, parser.cache.maxBytes)
	}
	large := strings.Repeat("x", parser.cache.maxEntryBytes+1)
	parser.Render(large)
	if _, exists := parser.cache.entries[sha256.Sum256([]byte("/wiki/\x00"+large))]; exists {
		t.Fatal("oversized render result must not be cached")
	}
}

func TestParserClearCacheRemovesRenderedDocuments(t *testing.T) {
	parser := NewParser()
	parser.RenderWithLocale("old personal value", "ko")
	if len(parser.cache.entries) == 0 {
		t.Fatal("render did not populate cache")
	}
	parser.ClearCache()
	if len(parser.cache.entries) != 0 || parser.cache.bytes != 0 || len(parser.cache.keys) != 0 {
		t.Fatalf("cache was not cleared: entries=%d bytes=%d keys=%d", len(parser.cache.entries), parser.cache.bytes, len(parser.cache.keys))
	}
}
