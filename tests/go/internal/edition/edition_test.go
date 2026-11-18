package edition

import (
	"errors"
	"testing"
)

func TestActiveEditionRequiresInterfaceAndNavigation(t *testing.T) {
	value := Edition{Tag: "fr", Status: StatusActive, MainPageName: "Accueil", Name: "French", NativeName: "Français", Direction: "ltr"}
	if _, err := Normalize(value); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected incomplete active edition to be invalid, got %v", err)
	}

	value.InterfaceMessages = make(map[string]string, len(InterfaceMessageKeys()))
	for _, key := range InterfaceMessageKeys() {
		value.InterfaceMessages[key] = "configured " + key
	}
	value.Navigation = []NavigationSection{{Title: "Explorer", Items: []NavigationItem{{Label: "Accueil", Target: "Accueil"}}}}
	if _, err := Normalize(value); err != nil {
		t.Fatalf("expected fully configured active edition: %v", err)
	}
}

func TestNormalizePortalMetadata(t *testing.T) {
	value, err := Normalize(Edition{
		Tag: "ko", Name: "Korean", NativeName: "한국어", Direction: "ltr", Status: StatusPending,
		PortalOrder: 20, CountryCodes: []string{"kr", "US", "KR"},
	})
	if err != nil {
		t.Fatalf("normalize portal metadata: %v", err)
	}
	if value.PortalOrder != 20 || len(value.CountryCodes) != 2 || value.CountryCodes[0] != "KR" || value.CountryCodes[1] != "US" {
		t.Fatalf("unexpected portal metadata: %#v", value)
	}
	if _, err := Normalize(Edition{Tag: "ko", Direction: "ltr", Status: StatusPending, CountryCodes: []string{"ZZ"}}); err == nil {
		t.Fatal("expected an unknown country code to fail validation")
	}
}

func TestNormalizeEnforcesCatalogMetadataAndRejectsCompositeTags(t *testing.T) {
	value, err := Normalize(Edition{
		Tag: "ZH", Name: "Injected", NativeName: "Injected", Direction: "rtl", Status: StatusPending,
	})
	if err != nil {
		t.Fatalf("normalize Chinese edition: %v", err)
	}
	if value.Tag != "zh" || value.Name != "Chinese" || value.Name == "Injected" || value.NativeName == "Injected" || value.Direction != "ltr" {
		t.Fatalf("catalog metadata was not enforced: %#v", value)
	}
	for _, tag := range []string{"zh-Hans", "pt-BR"} {
		if _, err := Normalize(Edition{Tag: tag, Status: StatusPending}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Normalize(%q) error = %v, want ErrInvalid", tag, err)
		}
	}
}
