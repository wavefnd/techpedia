package config

import "testing"

func TestMCPDefaultsOffAndRequiresExplicitEnablement(t *testing.T) {
	for _, value := range []string{"", "false", "invalid", "true"} {
		t.Setenv("TECHPEDIA_MCP_ENABLED", value)
		if got := FromEnv().MCPEnabled; got != (value == "true") {
			t.Fatalf("MCP enabled for %q = %v", value, got)
		}
	}
}

func TestCSPModeDefaultsToReportOnlyAndAcceptsEnforce(t *testing.T) {
	t.Setenv("TECHPEDIA_CSP_MODE", "")
	if got := FromEnv().CSPMode; got != "report-only" {
		t.Fatalf("default CSP mode = %q, want report-only", got)
	}

	t.Setenv("TECHPEDIA_CSP_MODE", " EnFoRcE ")
	if got := FromEnv().CSPMode; got != "enforce" {
		t.Fatalf("enforced CSP mode = %q, want enforce", got)
	}

	t.Setenv("TECHPEDIA_CSP_MODE", "invalid")
	if got := FromEnv().CSPMode; got != "report-only" {
		t.Fatalf("invalid CSP mode = %q, want report-only", got)
	}
}

func TestInterfaceAutoTranslationDefaultsOnAndCanBeDisabled(t *testing.T) {
	t.Setenv("TECHPEDIA_INTERFACE_AUTO_TRANSLATION", "")
	t.Setenv("TECHPEDIA_INTERFACE_TRANSLATION_URL", "")
	value := FromEnv()
	if !value.InterfaceAutoTranslation || value.InterfaceTranslationURL != "https://api.mymemory.translated.net/get" {
		t.Fatalf("unexpected default interface translation config: %#v", value)
	}

	t.Setenv("TECHPEDIA_INTERFACE_AUTO_TRANSLATION", "false")
	t.Setenv("TECHPEDIA_INTERFACE_TRANSLATION_URL", "https://translate.example/get")
	value = FromEnv()
	if value.InterfaceAutoTranslation || value.InterfaceTranslationURL != "https://translate.example/get" {
		t.Fatalf("unexpected configured interface translation: %#v", value)
	}
}

func TestAnalyticsDefaultsOnWithCloudflareCountryHeader(t *testing.T) {
	t.Setenv("TECHPEDIA_ANALYTICS_ENABLED", "")
	t.Setenv("TECHPEDIA_ANALYTICS_COUNTRY_HEADER", "")
	value := FromEnv()
	if !value.AnalyticsEnabled || value.AnalyticsCountryHeader != "CF-IPCountry" {
		t.Fatalf("unexpected default analytics config: %#v", value)
	}

	t.Setenv("TECHPEDIA_ANALYTICS_ENABLED", "false")
	t.Setenv("TECHPEDIA_ANALYTICS_COUNTRY_HEADER", "X-Visitor-Country")
	value = FromEnv()
	if value.AnalyticsEnabled || value.AnalyticsCountryHeader != "X-Visitor-Country" {
		t.Fatalf("unexpected configured analytics: %#v", value)
	}
}
