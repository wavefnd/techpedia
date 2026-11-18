package document

import "testing"

func TestCanonicalLocaleAcceptsOnlySingleLanguageCodes(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"ko":  "ko",
		"EN":  "en",
		"sh":  "sh",
		"eng": "eng",
	}
	for input, expected := range tests {
		actual, ok := CanonicalLocale(input)
		if !ok || actual != expected {
			t.Fatalf("CanonicalLocale(%q) = %q, %v; want %q, true", input, actual, ok, expected)
		}
	}
	for _, input := range []string{"", "a", "abcd", "und", "pt-BR", "zh-Hans", "sr-Latn", "x-private", "not_a_language", "bad_@", "12"} {
		if actual, ok := CanonicalLocale(input); ok {
			t.Fatalf("CanonicalLocale(%q) = %q, true; want invalid", input, actual)
		}
	}
}
