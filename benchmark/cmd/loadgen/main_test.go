package main

import (
	"net/url"
	"testing"
	"time"
)

func TestParseTargets(t *testing.T) {
	targets, err := parseTargets("100, 500,1000")
	if err != nil || len(targets) != 3 || targets[2] != 1000 {
		t.Fatalf("unexpected targets: %#v, %v", targets, err)
	}
	for _, invalid := range []string{"", "100,100", "200,100", "nope"} {
		if _, err := parseTargets(invalid); err == nil {
			t.Fatalf("expected %q to be rejected", invalid)
		}
	}
}

func TestPercentileUsesNearestRank(t *testing.T) {
	values := []time.Duration{time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond, 4 * time.Millisecond}
	if got := percentile(values, 0.95); got != 4*time.Millisecond {
		t.Fatalf("unexpected p95: %s", got)
	}
}

func TestValidateTargetRejectsPublicHost(t *testing.T) {
	local, _ := url.Parse("http://engine:8080")
	if err := validateTarget(local); err != nil {
		t.Fatalf("isolated engine target rejected: %v", err)
	}
	public, _ := url.Parse("https://example.com")
	if err := validateTarget(public); err == nil {
		t.Fatal("public target must be rejected by default")
	}
}
