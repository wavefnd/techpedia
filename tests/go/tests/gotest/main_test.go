package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOverlayMapsRootAndBenchmarkTestsWithoutCopyingSources(t *testing.T) {
	root := t.TempDir()
	for _, relative := range []string{"internal/example", "benchmark/cmd/loadgen"} {
		if err := os.MkdirAll(filepath.Join(root, relative), 0700); err != nil {
			t.Fatal(err)
		}
		stored := filepath.Join(root, "tests/go", relative, "sample_test.go")
		if err := os.MkdirAll(filepath.Dir(stored), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(stored, []byte("package example\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	values, err := testOverlay(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 {
		t.Fatalf("want both modules, got %d test files", len(values))
	}
	for target, backing := range values {
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("test copied into product package: %s, %v", target, err)
		}
		if _, err := os.Stat(backing); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOverlayRejectsEmptyOrMissingTestSources(t *testing.T) {
	root := t.TempDir()
	if _, err := testOverlay(root); err == nil {
		t.Fatal("missing test tree accepted")
	}
	if err := os.MkdirAll(filepath.Join(root, "tests/go"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := testOverlay(root); err == nil {
		t.Fatal("empty test tree could silently skip the suite")
	}
}

func TestOverlayRejectsDuplicateAndOrphanTests(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(map[bool]string{false: "orphan", true: "duplicate"}[duplicate], func(t *testing.T) {
			root := t.TempDir()
			stored := filepath.Join(root, "tests/go/internal/example/sample_test.go")
			if err := os.MkdirAll(filepath.Dir(stored), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(stored, []byte("package example\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if duplicate {
				path := filepath.Join(root, "internal/example/sample_test.go")
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("package example\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := testOverlay(root); err == nil {
				t.Fatal("invalid test mapping accepted")
			}
		})
	}
}

func TestOverlayRejectsSymlinkedTests(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "tests/go")
	if err := os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(base, "outside")); err != nil {
		t.Fatal(err)
	}
	if _, err := testOverlay(root); err == nil {
		t.Fatal("symlinked test tree accepted")
	}
}

func TestRunnerRejectsCommandsAndOverlayOverrides(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	for _, args := range [][]string{
		{"build", "./cmd/techpedia"},
		{"--module", "../outside", "test"},
		{"test", "-overlay=other.json", "./..."},
		{"vet", "-overlay", "other.json", "./..."},
	} {
		if err := run(args); err == nil {
			t.Fatalf("invalid test command accepted: %v", args)
		}
	}
}

func TestTemporaryOverlayStateIsPrivateAndRejectsSymlinkedRuntime(t *testing.T) {
	root := t.TempDir()
	tmp := filepath.Join(root, ".local/test/tmp")
	if err := makePrivateTempRoot(root, tmp); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(tmp)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("private temporary path: %v, %v", info, err)
	}
	other := t.TempDir()
	if err := os.Symlink(root, filepath.Join(other, ".local")); err != nil {
		t.Fatal(err)
	}
	if err := makePrivateTempRoot(other, filepath.Join(other, ".local/test/tmp")); err == nil {
		t.Fatal("symlinked local runtime accepted")
	}
}
