package support

import (
	"errors"
	"testing"
)

func TestNormalizeSupportSettings(t *testing.T) {
	value, err := NormalizeSettings(Settings{
		Enabled: true,
		Title:   " Support TechPedia ",
		Wallets: []Wallet{{Asset: " BTC ", Network: " Bitcoin ", Address: " bc1qexampleaddress "}},
	})
	if err != nil {
		t.Fatalf("normalize settings: %v", err)
	}
	if value.Version != SettingsVersion || value.Title != "Support TechPedia" || value.Wallets[0].Address != "bc1qexampleaddress" {
		t.Fatalf("unexpected normalized settings: %#v", value)
	}
	if _, err := NormalizeSettings(Settings{Enabled: true, Title: "Support"}); !errors.Is(err, ErrInvalidSettings) {
		t.Fatal("enabled page without a wallet must fail")
	}
	if _, err := NormalizeSettings(Settings{Wallets: []Wallet{{Asset: "BTC", Network: "Bitcoin", Address: "seed phrase is not an address"}}}); !errors.Is(err, ErrInvalidSettings) {
		t.Fatal("wallet addresses containing spaces must fail")
	}
}
