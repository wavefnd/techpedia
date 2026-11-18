package media

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memoryStore struct {
	files map[string]File
}

type deletionMemoryStore struct {
	*memoryStore
	err error
}

func (s *deletionMemoryStore) DeleteMedia(ctx context.Context, name, hash, actor string) (File, error) {
	if err := ctx.Err(); err != nil {
		return File{}, err
	}
	if s.err != nil {
		return File{}, s.err
	}
	file, err := s.GetMedia(ctx, name)
	if err != nil {
		return file, err
	}
	if file.Hash != hash || actor == "" {
		return File{}, ErrInvalidName
	}
	delete(s.files, strings.ToLower(name))
	return file, nil
}

func TestManagerDeleteRemovesAllVariantsOnlyAfterMetadataCommit(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := &deletionMemoryStore{memoryStore: newMemoryStore()}
	m := NewManager(root, store, Limits{})
	input := testPNG(t, 80, 40)
	upload, err := m.Upload(ctx, bytes.NewReader(input), Upload{Name: "Photo.png"})
	if err != nil {
		t.Fatal(err)
	}
	if used, err := m.Usage(); err != nil || used <= 0 {
		t.Fatal(used, err)
	}
	store.err = ErrInUse
	if _, err := m.Delete(ctx, upload.File.Name, "owner"); !errors.Is(err, ErrInUse) {
		t.Fatal(err)
	}
	_, _, file, err := m.OpenVariant(ctx, upload.File.Name, "thumb")
	if err != nil {
		t.Fatal("failed delete changed live bytes", err)
	}
	file.Close()
	store.err = nil
	result, err := m.Delete(ctx, upload.File.Name, "owner")
	if err != nil || result.CleanupPending {
		t.Fatal(result, err)
	}
	for _, variant := range upload.File.Variants {
		if _, err := os.Stat(filepath.Join(root, "objects", filepath.FromSlash(variant.Path))); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("variant survived", variant.Path, err)
		}
	}
	if used, err := m.Usage(); err != nil || used != 0 {
		t.Fatal("usage not updated", used, err)
	}
	if _, _, _, err := m.OpenVariant(ctx, upload.File.Name, "thumb"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := m.Upload(ctx, bytes.NewReader(input), Upload{Name: "Different.png"}); err != nil {
		t.Fatal("same content cannot be uploaded with a new name", err)
	}
}

func newMemoryStore() *memoryStore { return &memoryStore{files: map[string]File{}} }

func (s *memoryStore) SaveMedia(_ context.Context, file File) (File, error) {
	key := strings.ToLower(file.Name)
	if _, ok := s.files[key]; ok {
		return File{}, ErrAlreadyExists
	}
	s.files[key] = file
	return file, nil
}

func (s *memoryStore) GetMedia(_ context.Context, name string) (File, error) {
	normalized, err := NormalizeName(name)
	if err != nil {
		return File{}, err
	}
	file, ok := s.files[strings.ToLower(normalized)]
	if !ok {
		return File{}, ErrNotFound
	}
	return file, nil
}

func (s *memoryStore) GetMediaByHash(_ context.Context, hash string) (File, error) {
	for _, file := range s.files {
		if file.Hash == hash {
			return file, nil
		}
	}
	return File{}, ErrNotFound
}

func (s *memoryStore) SearchMedia(context.Context, string, int) ([]File, error) { return nil, nil }

func TestNormalizeNameCreatesGlobalWebPName(t *testing.T) {
	name, err := NormalizeName(" Linux distribution tree.PNG ")
	if err != nil || name != "Linux_distribution_tree.webp" {
		t.Fatalf("unexpected normalized name %q: %v", name, err)
	}
	for _, invalid := range []string{"../secret.png", `bad\name.png`, `bad"name.png`} {
		if _, err := NormalizeName(invalid); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("expected %q to be rejected, got %v", invalid, err)
		}
	}
}

func TestManagerCompressesVariantsAndReusesDuplicateContent(t *testing.T) {
	root := t.TempDir()
	store := newMemoryStore()
	manager := NewManager(root, store, Limits{StorageBytes: 20 << 20, UploadBytes: 2 << 20})
	source := testPNG(t, 420, 210)

	result, err := manager.Upload(context.Background(), bytes.NewReader(source), Upload{
		Name: "Linux tree.png", OriginalName: "camera.png", UploadedBy: "editor",
	})
	if err != nil {
		t.Fatalf("upload image: %v", err)
	}
	if result.Reused || result.File.Name != "Linux_tree.webp" || result.File.OriginalMIME != "image/png" {
		t.Fatalf("unexpected upload result: %#v", result)
	}
	if len(result.File.Variants) != 3 || result.File.SizeBytes < 1 {
		t.Fatalf("expected three compressed variants: %#v", result.File.Variants)
	}
	for name, variant := range result.File.Variants {
		path := filepath.Join(root, "objects", filepath.FromSlash(variant.Path))
		file, err := os.Open(path)
		if err != nil {
			t.Fatalf("open %s variant: %v", name, err)
		}
		header := make([]byte, 12)
		_, readErr := io.ReadFull(file, header)
		_ = file.Close()
		if readErr != nil || string(header[:4]) != "RIFF" || string(header[8:12]) != "WEBP" {
			t.Fatalf("%s is not a WebP file", path)
		}
	}
	if matches, _ := filepath.Glob(filepath.Join(root, "objects", "*", "*", "*", "original")); len(matches) != 0 {
		t.Fatalf("original upload must not be retained: %v", matches)
	}

	reused, err := manager.Upload(context.Background(), bytes.NewReader(source), Upload{
		Name: "Same image, another name.png", OriginalName: "copy.png", UploadedBy: "editor",
	})
	if err != nil || !reused.Reused || reused.File.Name != result.File.Name {
		t.Fatalf("expected content-addressed reuse, got %#v, %v", reused, err)
	}
}

func testPNG(t *testing.T, width int, height int) []byte {
	t.Helper()
	value := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			value.SetNRGBA(x, y, color.NRGBA{R: uint8(x % 255), G: uint8(y % 255), B: 80, A: 255})
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, value); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	return output.Bytes()
}
