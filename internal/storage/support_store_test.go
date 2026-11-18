package storage

import (
	"context"
	"testing"

	"techpedia/internal/support"
)

func TestSupportSettingsLifecycle(t *testing.T) {
	store, err := OpenBadgerDocumentStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	initial, err := store.GetSupportSettings(context.Background())
	if err != nil || initial.Enabled || len(initial.Wallets) != 0 {
		t.Fatalf("unexpected default settings: %#v, %v", initial, err)
	}
	saved, err := store.SaveSupportSettings(context.Background(), support.Settings{
		Enabled: true, Title: "Support TechPedia", Description: "Crypto addresses",
		Wallets: []support.Wallet{{Asset: "BTC", Network: "Bitcoin", Address: "bc1qexampleaddress"}},
	})
	if err != nil || saved.UpdatedAt.IsZero() {
		t.Fatalf("save settings: %#v, %v", saved, err)
	}
	loaded, err := store.GetSupportSettings(context.Background())
	if err != nil || !loaded.Enabled || loaded.Wallets[0].Address != "bc1qexampleaddress" {
		t.Fatalf("load settings: %#v, %v", loaded, err)
	}
}
