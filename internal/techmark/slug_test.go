package techmark

import "testing"

func TestSlugifyTechnicalNames(t *testing.T) {
	tests := map[string]string{
		"C":       "c",
		"C++":     "c-plus-plus",
		"C#":      "c-sharp",
		"F#":      "f-sharp",
		"A*":      "a-star",
		"A/B":     "a-slash-b",
		"Node.js": "node-js",
	}

	for input, expected := range tests {
		if got := Slugify(input); got != expected {
			t.Fatalf("Slugify(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestURLNamePreservesTechnicalPunctuation(t *testing.T) {
	if got, want := URLName("C++ / C# * A-B"), "C++_%2F_C%23_*_A-B"; got != want {
		t.Fatalf("URLName() = %q, want %q", got, want)
	}
}

func TestWikiLinkTargetNameCapitalizesOnlyInitialLatinLetter(t *testing.T) {
	tests := map[string]string{
		"programming language": "Programming language",
		"programming Language": "Programming Language",
		"éclair":               "Éclair",
		" C++ ":                "C++",
		"운영체제":                 "운영체제",
		"123 protocol":         "123 protocol",
	}
	for input, want := range tests {
		if got := WikiLinkTargetName(input); got != want {
			t.Fatalf("WikiLinkTargetName(%q) = %q, want %q", input, got, want)
		}
	}
}
