package media

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const (
	DefaultStorageLimit int64 = 900 << 20
	DefaultUploadLimit  int64 = 10 << 20
	DefaultPixelLimit   int64 = 25_000_000
)

var (
	ErrNotFound       = errors.New("media file not found")
	ErrAlreadyExists  = errors.New("media filename already exists")
	ErrInvalidName    = errors.New("invalid media filename")
	ErrInvalidImage   = errors.New("invalid or unsupported image")
	ErrUploadTooLarge = errors.New("media upload is too large")
	ErrStorageFull    = errors.New("media storage limit reached")
	ErrInUse          = errors.New("media file is used by a current document")
)

type Variant struct {
	Path      string `json:"path"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	SizeBytes int64  `json:"size_bytes"`
}

type File struct {
	Name         string             `json:"name"`
	SearchName   string             `json:"search_name"`
	Hash         string             `json:"hash"`
	MIME         string             `json:"mime"`
	OriginalName string             `json:"original_name"`
	OriginalMIME string             `json:"original_mime"`
	Width        int                `json:"width"`
	Height       int                `json:"height"`
	SizeBytes    int64              `json:"size_bytes"`
	Variants     map[string]Variant `json:"variants"`
	Author       string             `json:"author,omitempty"`
	License      string             `json:"license,omitempty"`
	Source       string             `json:"source,omitempty"`
	UploadedBy   string             `json:"uploaded_by"`
	CreatedAt    time.Time          `json:"created_at"`
}

type Store interface {
	SaveMedia(ctx context.Context, file File) (File, error)
	GetMedia(ctx context.Context, name string) (File, error)
	GetMediaByHash(ctx context.Context, hash string) (File, error)
	SearchMedia(ctx context.Context, query string, limit int) ([]File, error)
}

// Deletion is separate from the upload/read contract and is called only after
// the HTTP boundary verifies owner permission and exact-name confirmation.
type DeletionStore interface {
	DeleteMedia(context.Context, string, string, string) (File, error)
}

type DeleteResult struct {
	CleanupPending bool `json:"cleanup_pending"`
}

func NormalizeName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrInvalidName
	}
	if strings.ContainsAny(value, "/\\") || strings.IndexByte(value, 0) >= 0 {
		return "", ErrInvalidName
	}
	value = filepath.Base(value)
	if value == "." || value == ".." || strings.ContainsAny(value, "<>\"") {
		return "", ErrInvalidName
	}

	extension := filepath.Ext(value)
	base := strings.TrimSpace(strings.TrimSuffix(value, extension))
	if base == "" {
		return "", ErrInvalidName
	}
	base = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsControl(r):
			return -1
		case unicode.IsSpace(r):
			return '_'
		default:
			return r
		}
	}, base)
	base = strings.Trim(base, "._")
	if base == "" || len([]rune(base)) > 170 {
		return "", ErrInvalidName
	}
	return base + ".webp", nil
}

func SearchKey(value string) string {
	value = strings.TrimSuffix(value, filepath.Ext(value))
	value = strings.ReplaceAll(value, "_", " ")
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func Validate(file File) error {
	name, err := NormalizeName(file.Name)
	if err != nil || name != file.Name {
		return ErrInvalidName
	}
	if file.Hash == "" || file.Width < 1 || file.Height < 1 || file.SizeBytes < 1 {
		return fmt.Errorf("%w: incomplete metadata", ErrInvalidImage)
	}
	for _, required := range []string{"thumb", "article", "large"} {
		variant, ok := file.Variants[required]
		if !ok || variant.Path == "" || variant.Width < 1 || variant.Height < 1 || variant.SizeBytes < 1 {
			return fmt.Errorf("%w: missing %s variant", ErrInvalidImage, required)
		}
	}
	return nil
}
