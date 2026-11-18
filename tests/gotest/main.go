// Command gotest runs separately stored tests in their original package context.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.ExitCode())
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, "tests/go/go.mod")); err != nil {
		return errors.New("run ./tests/go.sh from the repository checkout")
	}
	module := "."
	if len(args) >= 2 && args[0] == "--module" {
		module, args = args[1], args[2:]
	}
	if module != "." && module != "benchmark" {
		return errors.New("test module must be . or benchmark")
	}
	if len(args) == 0 {
		args = []string{"test", "./..."}
	}
	if len(args) == 1 && (args[0] == "test" || args[0] == "vet" || args[0] == "list") {
		args = append(args, "./...")
	}
	if args[0] != "test" && args[0] != "vet" && args[0] != "list" {
		return errors.New("usage: ./tests/go.sh [--module benchmark] [test|vet|list] [Go arguments]")
	}
	for _, arg := range args[1:] {
		if arg == "-overlay" || strings.HasPrefix(arg, "-overlay=") {
			return errors.New("the runner owns -overlay; pass ordinary Go test/vet/list flags")
		}
	}
	replacements, err := testOverlay(root)
	if err != nil {
		return err
	}
	tmpRoot := filepath.Join(root, ".local/test/tmp")
	if err := makePrivateTempRoot(root, tmpRoot); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(tmpRoot, "go-overlay-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	data, err := json.Marshal(struct {
		Replace map[string]string
	}{replacements})
	if err != nil {
		return err
	}
	overlay := filepath.Join(tmp, "overlay.json")
	if err := os.WriteFile(overlay, data, 0600); err != nil {
		return err
	}
	command := exec.Command("go", append([]string{args[0], "-overlay=" + overlay}, args[1:]...)...)
	command.Dir = filepath.Join(root, module)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	command.Env = append(os.Environ(), "TMPDIR="+tmpRoot)
	return command.Run()
}

func makePrivateTempRoot(root, tmp string) error {
	for _, relative := range []string{".local", ".local/test", ".local/test/tmp"} {
		path := filepath.Join(root, relative)
		info, err := os.Lstat(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
			return fmt.Errorf("refusing a symlink or non-directory test path: %s", relative)
		}
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(path, 0700); err != nil {
				return err
			}
		}
	}
	return os.Chmod(tmp, 0700)
}

func testOverlay(root string) (map[string]string, error) {
	base := filepath.Join(root, "tests/go")
	replacements := make(map[string]string)
	err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not allowed in Go test sources: %s", path)
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		target := filepath.Join(root, relative)
		if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
			if err == nil {
				return fmt.Errorf("test exists in both source and tests/go: %s", relative)
			}
			return err
		}
		if info, err := os.Stat(filepath.Dir(target)); err != nil || !info.IsDir() {
			return fmt.Errorf("test has no production package directory: %s", relative)
		}
		replacements[target] = path
		return nil
	})
	if err == nil && len(replacements) == 0 {
		return nil, errors.New("no Go test sources found under tests/go")
	}
	return replacements, err
}
