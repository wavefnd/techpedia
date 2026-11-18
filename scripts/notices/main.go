// Command notices collects upstream license texts without relicensing them.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type dependency struct {
	Ecosystem string   `json:"ecosystem"`
	Name      string   `json:"name"`
	Version   string   `json:"version"`
	License   string   `json:"declared_license,omitempty"`
	Files     []string `json:"notice_files"`
}

func main() {
	output := flag.String("out", "build/licenses", "destination for generated notices")
	docs := flag.Bool("docs", false, "also collect documentation toolchain notices")
	docsOnly := flag.Bool("docs-only", false, "collect only the documentation dependency notices")
	flag.Parse()
	if err := collect(*output, *docs || *docsOnly, !*docsOnly); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func collect(output string, docs, runtime bool) error {
	if err := os.MkdirAll(output, 0755); err != nil {
		return err
	}
	var dependencies []dependency
	missing := []string{}
	add := func(ecosystem, name, version, license, dir string) error {
		id := strings.NewReplacer("/", "_", "\\", "_", "@", "_").Replace(name + "@" + version)
		destination := filepath.Join(output, ecosystem, id)
		if err := os.MkdirAll(destination, 0755); err != nil {
			return err
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		d := dependency{Ecosystem: ecosystem, Name: name, Version: version, License: license, Files: []string{}}
		sources := []string{}
		for _, entry := range entries {
			n := strings.ToLower(entry.Name())
			if entry.IsDir() || !(strings.HasPrefix(n, "license") || strings.HasPrefix(n, "licence") || strings.HasPrefix(n, "copying") || strings.HasPrefix(n, "notice") || n == "copyright") {
				continue
			}
			sources = append(sources, filepath.Join(dir, entry.Name()))
		}
		switch name + "@" + version {
		case "pretendard@1.3.9":
			sources = append(sources, filepath.Join(dir, "dist/LICENSE.txt"))
		case "@vue/devtools-api@6.6.4":
			sources = append(sources, "third_party/vue-devtools-api-6.6.4-LICENSE.txt")
		case "vuestic-ui@1.10.3":
			sources = append(sources, "third_party/vuestic-ui-1.10.3-LICENSE.txt")
		}
		if strings.HasPrefix(name, "@rolldown/binding-") && version == "1.1.8" {
			sources = append(sources, "web/node_modules/rolldown/LICENSE", "third_party/rolldown-1.1.8-NOTICE.txt")
		}
		if strings.HasPrefix(name, "@algolia/autocomplete-") && version == "1.17.7" {
			sources = append(sources, "third_party/algolia-autocomplete-1.17.7-LICENSE.txt")
		}
		if strings.HasPrefix(name, "@docsearch/") && version == "3.8.2" {
			sources = append(sources, "third_party/algolia-docsearch-3.8.2-LICENSE.txt")
		}
		if name == "@iconify-json/simple-icons" && version == "1.2.103" {
			sources = append(sources, "third_party/simple-icons-16.38.0-LICENSE.txt")
		}
		if strings.HasPrefix(name, "@esbuild/") && version == "0.25.12" {
			sources = append(sources, "docs/node_modules/esbuild/LICENSE.md")
		}
		if strings.HasPrefix(name, "@rollup/rollup-") && version == "4.68.5" {
			sources = append(sources, "docs/node_modules/rollup/LICENSE.md")
		}
		for _, source := range sources {
			data, err := os.ReadFile(source)
			if err != nil {
				return err
			}
			target := filepath.Join(destination, filepath.Base(source))
			if err := os.WriteFile(target, data, 0644); err != nil {
				return err
			}
			rel, err := filepath.Rel(output, target)
			if err != nil {
				return err
			}
			d.Files = append(d.Files, filepath.ToSlash(rel))
		}
		if len(d.Files) == 0 {
			missing = append(missing, ecosystem+":"+name+"@"+version)
		}
		dependencies = append(dependencies, d)
		return nil
	}

	if runtime {
		cmd := exec.Command("go", "list", "-deps", "-json", "./cmd/techpedia")
		cmd.Stderr = os.Stderr
		data, err := cmd.Output()
		if err != nil {
			return err
		}
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		seen := map[string]bool{}
		for {
			var pkg struct {
				Module *struct {
					Path, Version, Dir string
					Main               bool
				}
			}
			if err := decoder.Decode(&pkg); err == io.EOF {
				break
			} else if err != nil {
				return err
			}
			m := pkg.Module
			if m == nil || m.Main || seen[m.Path] {
				continue
			}
			seen[m.Path] = true
			if m.Dir == "" {
				return fmt.Errorf("download Go module before collecting notices: %s", m.Path)
			}
			if err := add("go", m.Path, m.Version, "", m.Dir); err != nil {
				return err
			}
		}
	}
	packages := []string{}
	if runtime {
		packages = append(packages, "web")
	}
	if docs {
		packages = append(packages, "docs")
	}
	for _, root := range packages {
		data, err := os.ReadFile(filepath.Join(root, "package-lock.json"))
		if err != nil {
			return err
		}
		var lock struct {
			Packages map[string]struct {
				Name, Version string
				License       any
				Optional      bool
			}
		}
		if err := json.Unmarshal(data, &lock); err != nil {
			return err
		}
		paths := make([]string, 0, len(lock.Packages))
		for path := range lock.Packages {
			if path != "" {
				paths = append(paths, path)
			}
		}
		sort.Strings(paths)
		for _, path := range paths {
			p := lock.Packages[path]
			dir := filepath.Join(root, filepath.FromSlash(path))
			if _, err := os.Stat(dir); os.IsNotExist(err) && p.Optional {
				continue
			} else if err != nil {
				return err
			}
			name := p.Name
			if name == "" {
				_, name, _ = strings.Cut(path, "node_modules/")
			}
			license, _ := p.License.(string)
			if err := add(root, name, p.Version, license, dir); err != nil {
				return err
			}
		}
	}
	for _, source := range []string{"LICENSE", "THIRD_PARTY_NOTICES.md"} {
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(output, source), data, 0644); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir("web/public/fonts/licenses")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(output, "fonts"), 0755); err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join("web/public/fonts/licenses", e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(output, "fonts", e.Name()), data, 0644); err != nil {
			return err
		}
	}
	inventory := struct {
		Dependencies []dependency `json:"dependencies"`
		Missing      []string     `json:"missing_notice_texts"`
	}{dependencies, missing}
	data, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(output, "inventory.json"), append(data, '\n'), 0644); err != nil {
		return err
	}
	fmt.Printf("Collected %d dependencies; %d need notice review. Output: %s\n", len(dependencies), len(missing), output)
	if len(missing) != 0 {
		return fmt.Errorf("missing license texts: %s", strings.Join(missing, ", "))
	}
	return nil
}
