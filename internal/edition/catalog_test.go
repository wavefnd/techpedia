package edition

import "testing"

func TestLanguageCatalogUsesRegistryAndExclusions(t *testing.T) {
	results := SearchLanguageCatalog("french", map[string]struct{}{"fr": {}}, 20)
	for _, result := range results {
		if result.Tag == "fr" {
			t.Fatal("registered language must be excluded")
		}
	}

	results = SearchLanguageCatalog("arabic", nil, 20)
	if len(results) == 0 || results[0].Tag != "ar" {
		t.Fatalf("expected Arabic registry entry first, got %#v", results)
	}

}

func TestLanguageProfileUsesOneWritingSystemPerLanguageCode(t *testing.T) {
	name, native, direction, ok := DescribeLanguage("ar")
	if !ok || name != "Arabic" || native == "" || direction != "rtl" {
		t.Fatalf("unexpected Arabic metadata: %q %q %q %v", name, native, direction, ok)
	}

	chinese, ok := LanguageProfileFor("zh")
	if !ok || chinese.Name != "Chinese" || chinese.WritingSystem == "" || chinese.Direction != "ltr" {
		t.Fatalf("unexpected Chinese registry profile: %#v %v", chinese, ok)
	}

	for _, code := range []string{"sh", "sr", "bs", "hr", "cmn", "yue"} {
		if profile, ok := LanguageProfileFor(code); !ok || profile.Tag != code || profile.WritingSystem == "" {
			t.Fatalf("LanguageProfileFor(%q) = %#v, %v; language policy must remain an administrator decision", code, profile, ok)
		}
	}

	for _, invalid := range []string{"fr-CA", "zh-Hans", "zh-Hant", "az-Arab"} {
		if profile, ok := LanguageProfileFor(invalid); ok {
			t.Fatalf("LanguageProfileFor(%q) = %#v, true; want rejected", invalid, profile)
		}
	}
}

func TestWritingSystemRuneCountUsesFixedPolicy(t *testing.T) {
	if got := WritingSystemRuneCount("fr", "Menu principal"); got < 3 {
		t.Fatalf("French writing-system rune count = %d", got)
	}
	if got := WritingSystemRuneCount("ar", "القائمة الرئيسية"); got < 3 {
		t.Fatalf("Arabic writing-system rune count = %d", got)
	}
}

func TestRegistrySnapshotIsPopulated(t *testing.T) {
	if len(languageCatalog) < 7000 {
		t.Fatalf("expected comprehensive IANA language catalog, got %d records", len(languageCatalog))
	}
}

func TestCountryCatalogUsesCurrentTwoLetterRegions(t *testing.T) {
	countries := CountryCatalog()
	if len(countries) < 249 {
		t.Fatalf("expected a comprehensive country and region catalog, got %d records", len(countries))
	}
	if !ValidCountryCode("kr") || !ValidCountryCode("US") || ValidCountryCode("ZZ") {
		t.Fatal("country code validation did not follow the embedded registry")
	}
}
