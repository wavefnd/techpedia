package buildinfo

import (
	"regexp"
	"testing"
)

func TestVersionUsesThreeNumericComponents(t *testing.T) {
	if !regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(Version) {
		t.Fatalf("engine version %q must use MAJOR.MINOR.PATCH without a v prefix", Version)
	}
}
