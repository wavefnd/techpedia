package techmark

import (
	"html"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2/lexers"
)

func TestCompactCodeHighlightPreservesTextAndColors(t *testing.T) {
	for _, language := range []string{"c", "cpp", "go", "python", "javascript", "rust", "html", "text", "unknown-language"} {
		t.Run(language, func(t *testing.T) {
			source := "\tvalue = a + b; // 한글 & <script>alert(1)</script>\n\n  café🙂\n"
			if lexer := lexers.Get(codeLexerLanguage(language)); lexer != nil {
				// Lexers may normalize a final newline, so use the same lexer as
				// the old formatter to compare the actual token stream verbatim.
				iterator, err := lexer.Tokenise(nil, source)
				if err != nil {
					t.Fatal(err)
				}
				var normalized strings.Builder
				for _, token := range iterator.Tokens() {
					normalized.WriteString(token.Value)
				}
				source = normalized.String()
			}
			rendered := highlightCode(source, language)
			plain := html.UnescapeString(htmlTagPattern.ReplaceAllString(rendered, ""))
			if plain != source {
				t.Fatalf("code text changed: got %q, want %q", plain, source)
			}
			for _, token := range []string{"syntax-n\"", "syntax-p\"", "syntax-w\"", "<script>"} {
				if strings.Contains(rendered, token) {
					t.Fatalf("unexpected output %q", token)
				}
			}
		})
	}
	rendered := highlightCode("int value = 42; // comment\n", "c")
	for _, token := range []string{`class="syntax-kt"`, `class="syntax-o"`, `class="syntax-mi"`, `class="syntax-c1"`} {
		if !strings.Contains(rendered, token) {
			t.Fatalf("colored token missing: %s", token)
		}
	}
}

func TestIdentifierHeavyCodeDoesNotInflateIntoPerTokenSpans(t *testing.T) {
	source := strings.Repeat("alpha, beta, gamma;\n", 4000)
	rendered := highlightCode(source, "c")
	if len(rendered) > len(source)+1000 {
		t.Fatalf("unstyled token wrappers inflated code: %d -> %d bytes", len(source), len(rendered))
	}
	if html.UnescapeString(htmlTagPattern.ReplaceAllString(rendered, "")) != source {
		t.Fatal("large code was truncated or changed")
	}
}
