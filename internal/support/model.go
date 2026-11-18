package support

import (
	"errors"
	"strings"
	"time"
	"unicode"
)

const SettingsVersion = 1

var ErrInvalidSettings = errors.New("invalid support page settings")

type Wallet struct {
	Asset   string `json:"asset"`
	Network string `json:"network"`
	Address string `json:"address"`
	Memo    string `json:"memo,omitempty"`
}

type Settings struct {
	Version     int       `json:"version"`
	Enabled     bool      `json:"enabled"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Wallets     []Wallet  `json:"wallets"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func DefaultSettings() Settings {
	return Settings{Version: SettingsVersion, Wallets: []Wallet{}}
}

func NormalizeSettings(value Settings) (Settings, error) {
	value.Version = SettingsVersion
	value.Title = strings.TrimSpace(value.Title)
	value.Description = strings.TrimSpace(value.Description)
	if len(value.Title) > 120 || len(value.Description) > 2000 || containsControl(value.Title) || containsControl(value.Description) {
		return Settings{}, ErrInvalidSettings
	}
	if value.Wallets == nil {
		value.Wallets = []Wallet{}
	}
	if len(value.Wallets) > 32 {
		return Settings{}, ErrInvalidSettings
	}
	seen := make(map[string]struct{}, len(value.Wallets))
	for index := range value.Wallets {
		wallet := &value.Wallets[index]
		wallet.Asset = strings.TrimSpace(wallet.Asset)
		wallet.Network = strings.TrimSpace(wallet.Network)
		wallet.Address = strings.TrimSpace(wallet.Address)
		wallet.Memo = strings.TrimSpace(wallet.Memo)
		if wallet.Asset == "" || len(wallet.Asset) > 32 || containsControl(wallet.Asset) {
			return Settings{}, ErrInvalidSettings
		}
		if wallet.Network == "" || len(wallet.Network) > 64 || containsControl(wallet.Network) {
			return Settings{}, ErrInvalidSettings
		}
		if len(wallet.Address) < 3 || len(wallet.Address) > 256 || containsSpaceOrControl(wallet.Address) {
			return Settings{}, ErrInvalidSettings
		}
		if len(wallet.Memo) > 128 || containsControl(wallet.Memo) {
			return Settings{}, ErrInvalidSettings
		}
		key := strings.ToLower(wallet.Asset + "\x00" + wallet.Network + "\x00" + wallet.Address + "\x00" + wallet.Memo)
		if _, exists := seen[key]; exists {
			return Settings{}, ErrInvalidSettings
		}
		seen[key] = struct{}{}
	}
	if value.Enabled && (value.Title == "" || len(value.Wallets) == 0) {
		return Settings{}, ErrInvalidSettings
	}
	return value, nil
}

func containsControl(value string) bool {
	for _, char := range value {
		if unicode.IsControl(char) && char != '\n' && char != '\t' {
			return true
		}
	}
	return false
}

func containsSpaceOrControl(value string) bool {
	for _, char := range value {
		if unicode.IsSpace(char) || unicode.IsControl(char) {
			return true
		}
	}
	return false
}
