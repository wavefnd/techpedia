package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"techpedia/internal/media"
)

type Config struct {
	Addr                      string
	DataDir                   string
	PublicURL                 string
	CSPMode                   string
	ServerLocation            string
	BootstrapAdminUsername    string
	BootstrapAdminPassword    string
	MediaStorageLimitBytes    int64
	MediaUploadLimitBytes     int64
	InterfaceAutoTranslation  bool
	InterfaceTranslationURL   string
	InterfaceTranslationEmail string
	AnalyticsEnabled          bool
	AnalyticsCountryHeader    string
	MCPEnabled                bool
}

func FromEnv() Config {
	addr := os.Getenv("TECHPEDIA_ADDR")
	if addr == "" {
		addr = "0.0.0.0:8080"
	}

	dataDir := os.Getenv("TECHPEDIA_DATA_DIR")
	if dataDir == "" {
		dataDir = filepath.Join("data")
	}
	publicURL := strings.TrimRight(strings.TrimSpace(os.Getenv("TECHPEDIA_PUBLIC_URL")), "/")
	if publicURL == "" {
		publicURL = "http://localhost:8080"
	}

	return Config{
		Addr:                      addr,
		DataDir:                   dataDir,
		PublicURL:                 publicURL,
		CSPMode:                   envCSPMode(),
		ServerLocation:            strings.TrimSpace(os.Getenv("TECHPEDIA_SERVER_LOCATION")),
		BootstrapAdminUsername:    os.Getenv("TECHPEDIA_BOOTSTRAP_ADMIN_USERNAME"),
		BootstrapAdminPassword:    os.Getenv("TECHPEDIA_BOOTSTRAP_ADMIN_PASSWORD"),
		MediaStorageLimitBytes:    envPositiveInt64("TECHPEDIA_MEDIA_STORAGE_LIMIT_BYTES", media.DefaultStorageLimit),
		MediaUploadLimitBytes:     envPositiveInt64("TECHPEDIA_MEDIA_UPLOAD_LIMIT_BYTES", media.DefaultUploadLimit),
		InterfaceAutoTranslation:  envBool("TECHPEDIA_INTERFACE_AUTO_TRANSLATION", true),
		InterfaceTranslationURL:   envString("TECHPEDIA_INTERFACE_TRANSLATION_URL", "https://api.mymemory.translated.net/get"),
		InterfaceTranslationEmail: strings.TrimSpace(os.Getenv("TECHPEDIA_INTERFACE_TRANSLATION_EMAIL")),
		AnalyticsEnabled:          envBool("TECHPEDIA_ANALYTICS_ENABLED", true),
		AnalyticsCountryHeader:    envString("TECHPEDIA_ANALYTICS_COUNTRY_HEADER", "CF-IPCountry"),
		MCPEnabled:                envBool("TECHPEDIA_MCP_ENABLED", false),
	}
}

func envCSPMode() string {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TECHPEDIA_CSP_MODE")), "enforce") {
		return "enforce"
	}
	return "report-only"
}

func (c Config) BadgerDir() string {
	return filepath.Join(c.DataDir, "badger")
}

func (c Config) AuthDir() string {
	return filepath.Join(c.DataDir, "auth")
}

func (c Config) SearchDir() string {
	return filepath.Join(c.DataDir, "search")
}

func (c Config) MediaDir() string {
	return filepath.Join(c.DataDir, "media")
}

func (c Config) EnsureDataDirs() error {
	dirs := []string{
		c.DataDir,
		c.BadgerDir(),
		c.AuthDir(),
		c.SearchDir(),
		c.MediaDir(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}

func envPositiveInt64(name string, fallback int64) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(name)), 10, 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envBool(name string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envString(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}
