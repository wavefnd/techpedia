package translation

import (
	"strings"
	"testing"

	"techpedia/internal/edition"
)

func TestInterfaceMessagesCoverEveryEditionMessageKey(t *testing.T) {
	seeds := InterfaceMessages()
	wantKeys := edition.AllInterfaceMessageKeys()
	if len(seeds) != len(wantKeys) {
		t.Fatalf("seed count = %d, want %d", len(seeds), len(wantKeys))
	}

	want := make(map[string]struct{}, len(wantKeys))
	for _, key := range wantKeys {
		want[key] = struct{}{}
	}
	seen := make(map[string]struct{}, len(seeds))
	for _, seed := range seeds {
		if _, ok := want[seed.Key]; !ok {
			t.Errorf("unexpected interface seed key %q", seed.Key)
		}
		if _, ok := seen[seed.Key]; ok {
			t.Errorf("duplicate interface seed key %q", seed.Key)
		}
		seen[seed.Key] = struct{}{}
		if strings.TrimSpace(seed.Source) == "" || strings.ContainsAny(seed.Source, "\r\n") {
			t.Errorf("invalid English seed for %q: %q", seed.Key, seed.Source)
		}
	}
	for key := range want {
		if _, ok := seen[key]; !ok {
			t.Errorf("missing interface seed key %q", key)
		}
	}
}
