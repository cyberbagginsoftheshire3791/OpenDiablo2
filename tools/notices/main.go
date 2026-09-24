// Command notices writes THIRD_PARTY_NOTICES.md: the Go toolchain's standard
// library and runtime, and every Go module linked into the game, each with its
// version and the licence, notice and patent texts it ships -- so a build
// handed to anyone carries the notices those licences ask for.
//
// The build is the one handed out: untagged (no harness), for Windows
// (GOOS=windows, GOARCH=amd64 -- friends build #1), without cgo, with GOFLAGS
// and go.work ignored, so the file comes out the same on any machine with the
// pinned Go. The fonts and other data under data/strigoi carry their own
// licences beside them (data/strigoi/fonts); this file lists the code.
//
//	go run ./tools/notices           (from the repo root) rewrites the file
//	go run ./tools/notices -check    fails if the file is not what it would write
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const outFile = "THIRD_PARTY_NOTICES.md"

// module is one linked module: where its source is, and the directories of the
// packages the build links from it (a licence can sit beside the code it
// covers rather than at the module's root -- otto's underscore.js does).
type module struct {
	Path, Version, Dir string
	PkgDirs            []string
}

// licenceName matches the files licence terms are kept in: LICENSE, LICENCE,
// COPYING, NOTICE, PATENTS, with or without a suffix and an extension
// (LICENSE.md, LICENSE-APACHE, LICENSE.underscorejs) -- but no Go source that
// happens to start so (license_test.go, license.go).
var licenceName = regexp.MustCompile(`(?i)^(licen[sc]e|copying|notice|patents)(-[a-z0-9.]+)?(\.[a-z0-9]+)?$`)

func isLicence(name string) bool {
	return licenceName.MatchString(name) && !strings.HasSuffix(strings.ToLower(name), ".go")
}

// buildEnv is the build the notices describe.
func buildEnv() []string {
	return append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0", "GOFLAGS=", "GOWORK=off")
}

func main() {
	check := flag.Bool("check", false, "fail if "+outFile+" is not what this would write")
	flag.Parse()

	mods, err := linkedModules()
	if err != nil {
		log.Fatal(err)
	}

	goroot, goversion, err := toolchain()
	if err != nil {
		log.Fatal(err)
	}

	text, err := render(goroot, goversion, mods)
	if err != nil {
		log.Fatal(err)
	}

	if *check {
		have, err := os.ReadFile(outFile)
		if err != nil {
			log.Fatalf("%s: %v (run: go run ./tools/notices)", outFile, err)
		}

		if !bytes.Equal(bytes.ReplaceAll(have, []byte("\r\n"), []byte("\n")), text) {
			log.Fatalf("%s is out of date: the toolchain, the linked modules or their licences changed (run: go run ./tools/notices)", outFile)
		}

		fmt.Printf("%s: current, %s and %d module(s)\n", outFile, goversion, len(mods))

		return
	}

	if err := os.WriteFile(outFile, text, 0o644); err != nil { //nolint:gosec // a document in the repo
		log.Fatal(err)
	}

	fmt.Printf("wrote %s: %s and %d module(s)\n", outFile, goversion, len(mods))
}

// toolchain is the Go installation's root and version.
func toolchain() (goroot, goversion string, err error) {
	cmd := exec.Command("go", "env", "GOROOT", "GOVERSION")
	cmd.Env = buildEnv()

	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("go env: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(string(out), "\r\n", "\n")), "\n")
	if len(lines) != 2 {
		return "", "", fmt.Errorf("go env: %q", out)
	}

	return lines[0], lines[1], nil
}

// linkedModules is every module but the game's own that the build links, with
// the directories of its linked packages, sorted by path.
func linkedModules() ([]module, error) {
	const format = "{{with .Module}}{{if not .Main}}" +
		"{{.Path}}\t{{.Version}}\t{{.Dir}}\t" +
		"{{with .Replace}}{{.Path}}{{end}}\t{{with .Replace}}{{.Version}}{{end}}\t{{with .Replace}}{{.Dir}}{{end}}\t" +
		"{{end}}{{end}}"

	cmd := exec.Command("go", "list", "-deps", "-f", format+"{{if .Module}}{{if not .Module.Main}}{{.Dir}}{{end}}{{end}}", ".")
	cmd.Env = buildEnv()
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}

	byPath := map[string]*module{}

	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		if len(f) != 7 {
			continue
		}

		m := byPath[f[0]]
		if m == nil {
			m = &module{Path: f[0], Version: f[1], Dir: f[2]}

			if f[3] != "" { // replaced: the replacement is what is linked
				m.Version, m.Dir = strings.TrimSpace(f[4]+" (replaced by "+f[3]+")"), f[5]
			}

			byPath[f[0]] = m
		}

		m.PkgDirs = append(m.PkgDirs, f[6])
	}

	if err := sc.Err(); err != nil {
		return nil, err
	}

	mods := make([]module, 0, len(byPath))
	for _, m := range byPath {
		mods = append(mods, *m)
	}

	sort.Slice(mods, func(i, j int) bool { return mods[i].Path < mods[j].Path })

	return mods, nil
}

func render(goroot, goversion string, mods []module) ([]byte, error) {
	var b bytes.Buffer

	b.WriteString("# Third-party notices\n\n")
	b.WriteString("Strigoi is built on OpenDiablo2 (GPL-3.0, see LICENSE). The game handed out -- the untagged\n")
	b.WriteString("Windows build -- contains the Go standard library and runtime and links the Go modules below,\n")
	b.WriteString("each listed with the licence, notice and patent texts it ships. Generated by\n")
	b.WriteString("`go run ./tools/notices`; `go run ./tools/notices -check` (in the gate and CI) fails when it is\n")
	b.WriteString("out of date. The fonts and other data under `data/strigoi` carry their own licences beside them.\n")

	fmt.Fprintf(&b, "\n## The Go standard library and runtime (%s)\n", goversion)

	if err := writeLicences(&b, goroot, []string{goroot}); err != nil {
		return nil, fmt.Errorf("the Go toolchain: %w", err)
	}

	for _, m := range mods {
		fmt.Fprintf(&b, "\n## %s %s\n", m.Path, m.Version)

		if err := writeLicences(&b, m.Dir, append([]string{m.Dir}, m.PkgDirs...)); err != nil {
			return nil, fmt.Errorf("%s: %w", m.Path, err)
		}
	}

	return b.Bytes(), nil
}

// writeLicences writes every licence file in dirs (each once), named by its
// path under root.
func writeLicences(b *bytes.Buffer, root string, dirs []string) error {
	seen := map[string]bool{}

	var files []string

	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			return err
		}

		for _, e := range entries {
			p := filepath.Join(d, e.Name())
			if e.IsDir() || !isLicence(e.Name()) || seen[p] {
				continue
			}

			seen[p] = true
			files = append(files, p)
		}
	}

	if len(files) == 0 {
		b.WriteString("\n(no licence file in the module)\n")
		return nil
	}

	names := make(map[string]string, len(files))

	for _, p := range files {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}

		names[p] = filepath.ToSlash(rel)
	}

	sort.Slice(files, func(i, j int) bool { return names[files[i]] < names[files[j]] })

	for _, p := range files {
		text, err := os.ReadFile(p)
		if err != nil {
			return err
		}

		text = bytes.ReplaceAll(text, []byte("\r\n"), []byte("\n"))
		text = bytes.TrimRight(text, "\n")

		fmt.Fprintf(b, "\n`%s`:\n\n````text\n%s\n````\n", names[p], text)
	}

	return nil
}
