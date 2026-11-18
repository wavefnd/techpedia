package media

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gen2brain/webp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const maxEncodedBytes int64 = 4 << 20

type Limits struct {
	StorageBytes int64
	UploadBytes  int64
	Pixels       int64
}

type Upload struct {
	Name         string
	OriginalName string
	Author       string
	License      string
	Source       string
	UploadedBy   string
}

type UploadResult struct {
	File   File
	Reused bool
}

type Manager struct {
	root       string
	store      Store
	limits     Limits
	mu         sync.Mutex
	usage      int64
	usageReady bool
}

func NewManager(root string, store Store, limits Limits) *Manager {
	if limits.StorageBytes <= 0 {
		limits.StorageBytes = DefaultStorageLimit
	}
	if limits.UploadBytes <= 0 {
		limits.UploadBytes = DefaultUploadLimit
	}
	if limits.Pixels <= 0 {
		limits.Pixels = DefaultPixelLimit
	}
	return &Manager{root: strings.TrimSpace(root), store: store, limits: limits}
}

func (m *Manager) StorageLimit() int64 { return m.limits.StorageBytes }

func (m *Manager) UploadLimit() int64 { return m.limits.UploadBytes }

func (m *Manager) Upload(ctx context.Context, input io.Reader, value Upload) (UploadResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.root == "" {
		return UploadResult{}, errors.New("media storage is not configured")
	}
	name, err := NormalizeName(firstNonEmpty(value.Name, value.OriginalName))
	if err != nil {
		return UploadResult{}, err
	}
	if _, err := m.store.GetMedia(ctx, name); err == nil {
		return UploadResult{}, ErrAlreadyExists
	} else if !errors.Is(err, ErrNotFound) {
		return UploadResult{}, err
	}
	if err := m.ensureDirs(); err != nil {
		return UploadResult{}, err
	}

	tempDir, err := os.MkdirTemp(filepath.Join(m.root, "temp"), "upload-")
	if err != nil {
		return UploadResult{}, err
	}
	defer os.RemoveAll(tempDir)

	inputPath := filepath.Join(tempDir, "original")
	originalSize, originalMIME, err := copyUpload(inputPath, input, m.limits.UploadBytes)
	if err != nil {
		return UploadResult{}, err
	}
	_ = originalSize
	if !supportedMIME(originalMIME) {
		return UploadResult{}, ErrInvalidImage
	}

	sourceFile, err := os.Open(inputPath)
	if err != nil {
		return UploadResult{}, err
	}
	config, _, err := image.DecodeConfig(sourceFile)
	_ = sourceFile.Close()
	if err != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > m.limits.Pixels {
		return UploadResult{}, ErrInvalidImage
	}

	sourceFile, err = os.Open(inputPath)
	if err != nil {
		return UploadResult{}, err
	}
	decoded, _, err := image.Decode(sourceFile)
	_ = sourceFile.Close()
	if err != nil {
		return UploadResult{}, ErrInvalidImage
	}

	variants, totalSize, err := encodeVariants(tempDir, decoded)
	if err != nil {
		return UploadResult{}, err
	}
	if totalSize > maxEncodedBytes {
		return UploadResult{}, ErrUploadTooLarge
	}
	hash, err := hashFile(filepath.Join(tempDir, "large.webp"))
	if err != nil {
		return UploadResult{}, err
	}
	if existing, err := m.store.GetMediaByHash(ctx, hash); err == nil {
		return UploadResult{File: existing, Reused: true}, nil
	} else if !errors.Is(err, ErrNotFound) {
		return UploadResult{}, err
	}

	used, err := m.usageLocked()
	if err != nil {
		return UploadResult{}, err
	}
	if err := os.Remove(inputPath); err != nil {
		return UploadResult{}, err
	}

	relativeDir := filepath.Join(hash[:2], hash[2:4], hash)
	finalDir := filepath.Join(m.root, "objects", relativeDir)
	if err := os.MkdirAll(filepath.Dir(finalDir), 0o755); err != nil {
		return UploadResult{}, err
	}
	// A previous committed deletion may have been interrupted before its byte
	// cleanup. This hash has no live metadata (checked above) and is serialized
	// with deletion by m.mu, so only its exact orphan directory is removed.
	if _, err := os.Lstat(finalDir); err == nil {
		if err := os.RemoveAll(finalDir); err != nil {
			return UploadResult{}, err
		}
		m.usageReady = false
		used, err = m.usageLocked()
		if err != nil {
			return UploadResult{}, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return UploadResult{}, err
	}
	if used+totalSize > m.limits.StorageBytes {
		return UploadResult{}, ErrStorageFull
	}
	if err := os.Rename(tempDir, finalDir); err != nil {
		return UploadResult{}, err
	}
	removeFinal := true
	defer func() {
		if removeFinal {
			_ = os.RemoveAll(finalDir)
		}
	}()

	for key, variant := range variants {
		variant.Path = filepath.ToSlash(filepath.Join(relativeDir, key+".webp"))
		variants[key] = variant
	}
	file := File{
		Name:         name,
		SearchName:   SearchKey(name),
		Hash:         hash,
		MIME:         "image/webp",
		OriginalName: strings.TrimSpace(value.OriginalName),
		OriginalMIME: originalMIME,
		Width:        config.Width,
		Height:       config.Height,
		SizeBytes:    totalSize,
		Variants:     variants,
		Author:       strings.TrimSpace(value.Author),
		License:      strings.TrimSpace(value.License),
		Source:       strings.TrimSpace(value.Source),
		UploadedBy:   strings.TrimSpace(value.UploadedBy),
		CreatedAt:    time.Now().UTC(),
	}
	saved, err := m.store.SaveMedia(ctx, file)
	if err != nil {
		return UploadResult{}, err
	}
	m.usage = used + totalSize
	m.usageReady = true
	removeFinal = false
	return UploadResult{File: saved}, nil
}

func (m *Manager) Delete(ctx context.Context, name, actor string) (DeleteResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	store, ok := m.store.(DeletionStore)
	if !ok || m.root == "" {
		return DeleteResult{}, errors.New("media deletion is not configured")
	}
	file, err := m.store.GetMedia(ctx, name)
	if err != nil {
		return DeleteResult{}, err
	}
	decoded, err := hex.DecodeString(file.Hash)
	if err != nil || len(decoded) != sha256.Size || file.Name != name {
		return DeleteResult{}, ErrInvalidName
	}
	if _, err := store.DeleteMedia(ctx, name, file.Hash, actor); err != nil {
		return DeleteResult{}, err
	}
	// Metadata, name reservation and audit commit first. An interrupted cleanup
	// cannot leave a publicly readable deleted image or a reusable old URL.
	path := filepath.Join(m.root, "objects", file.Hash[:2], file.Hash[2:4], file.Hash)
	err = os.RemoveAll(path)
	m.usageReady = false
	return DeleteResult{CleanupPending: err != nil}, nil
}

func (m *Manager) OpenVariant(ctx context.Context, name string, variantName string) (File, Variant, *os.File, error) {
	file, err := m.store.GetMedia(ctx, name)
	if err != nil {
		return File{}, Variant{}, nil, err
	}
	variant, ok := file.Variants[variantName]
	if !ok {
		return File{}, Variant{}, nil, ErrNotFound
	}
	cleanPath := filepath.Clean(filepath.FromSlash(variant.Path))
	if filepath.IsAbs(cleanPath) || cleanPath == "." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return File{}, Variant{}, nil, ErrNotFound
	}
	opened, err := os.Open(filepath.Join(m.root, "objects", cleanPath))
	if errors.Is(err, os.ErrNotExist) {
		return File{}, Variant{}, nil, ErrNotFound
	}
	return file, variant, opened, err
}

func (m *Manager) Usage() (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.usageLocked()
}

func (m *Manager) usageLocked() (int64, error) {
	if m.root == "" {
		return 0, nil
	}
	if m.usageReady {
		return m.usage, nil
	}
	usage, err := directorySize(filepath.Join(m.root, "objects"))
	if err != nil {
		return 0, err
	}
	m.usage = usage
	m.usageReady = true
	return usage, nil
}

func (m *Manager) ensureDirs() error {
	for _, dir := range []string{filepath.Join(m.root, "objects"), filepath.Join(m.root, "temp")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func copyUpload(path string, source io.Reader, limit int64) (int64, string, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, "", err
	}
	buffered := bufio.NewWriter(file)
	written, copyErr := io.Copy(buffered, io.LimitReader(source, limit+1))
	flushErr := buffered.Flush()
	closeErr := file.Close()
	if copyErr != nil {
		return 0, "", copyErr
	}
	if flushErr != nil {
		return 0, "", flushErr
	}
	if closeErr != nil {
		return 0, "", closeErr
	}
	if written > limit {
		return 0, "", ErrUploadTooLarge
	}
	file, err = os.Open(path)
	if err != nil {
		return 0, "", err
	}
	header := make([]byte, 512)
	n, readErr := io.ReadFull(file, header)
	_ = file.Close()
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		return 0, "", readErr
	}
	return written, strings.ToLower(strings.TrimSpace(strings.Split(httpDetectContentType(header[:n]), ";")[0])), nil
}

var httpDetectContentType = http.DetectContentType

func supportedMIME(value string) bool {
	return value == "image/jpeg" || value == "image/png" || value == "image/webp"
}

func encodeVariants(tempDir string, source image.Image) (map[string]Variant, int64, error) {
	targets := []struct {
		name    string
		width   int
		quality int
	}{{"thumb", 320, 76}, {"article", 960, 80}, {"large", 1920, 82}}
	variants := make(map[string]Variant, len(targets))
	var total int64
	for _, target := range targets {
		resized := resize(source, target.width)
		path := filepath.Join(tempDir, target.name+".webp")
		output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, 0, err
		}
		encodeErr := webp.Encode(output, resized, webp.Options{Quality: target.quality, Method: 4})
		closeErr := output.Close()
		if encodeErr != nil {
			return nil, 0, fmt.Errorf("encode %s image: %w", target.name, encodeErr)
		}
		if closeErr != nil {
			return nil, 0, closeErr
		}
		stat, err := os.Stat(path)
		if err != nil {
			return nil, 0, err
		}
		bounds := resized.Bounds()
		variants[target.name] = Variant{Width: bounds.Dx(), Height: bounds.Dy(), SizeBytes: stat.Size()}
		total += stat.Size()
	}
	return variants, total, nil
}

func resize(source image.Image, maxWidth int) image.Image {
	bounds := source.Bounds()
	if bounds.Dx() <= maxWidth {
		return source
	}
	height := int(float64(bounds.Dy()) * float64(maxWidth) / float64(bounds.Dx()))
	destination := image.NewNRGBA(image.Rect(0, 0, maxWidth, height))
	draw.CatmullRom.Scale(destination, destination.Bounds(), source, bounds, draw.Over, nil)
	return destination
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func directorySize(root string) (int64, error) {
	var size int64
	err := filepath.WalkDir(root, func(_ string, entry os.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		size += info.Size()
		return nil
	})
	return size, err
}

func SortFiles(files []File) {
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].CreatedAt.Equal(files[j].CreatedAt) {
			return files[i].Name < files[j].Name
		}
		return files[i].CreatedAt.After(files[j].CreatedAt)
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
