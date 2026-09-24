package docscount

// THREE NUMBERS IN docs/harness.md THAT USED TO BE TYPED FROM MEMORY, and a
// gate so they cannot be again. Remediation R4: the doc said "seven scripts"
// when there were fourteen, and "harness 0.9.1" when the code said 0.10.0.
// Correcting the numbers once fixes the doc for a day; deriving them fixes it
// for good.
//
// THIS FILE CARRIES NO BUILD TAG ON PURPOSE. The harness and playtest sources
// are behind the `harness` and `playtest` tags and are not compiled here --
// they are READ AS TEXT, which needs no tag -- so a plain `go test ./...`,
// which is what CI runs, is enough to catch the drift.
//
// See doc.go for why this package is under tools/ rather than in package main.
//
// Each check names the derived value AND the string it looked for, because a
// gate whose failure does not say what to type is a gate people disable.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// harnessDoc is the one document these numbers live in, relative to the root.
const harnessDoc = "docs/harness.md"

// playtestStart is the project's own definition of "a playtest script": a file
// under playtest/ that calls the start() helper -- or startWith(), the same
// launch with switches of its own (23 Sep 2026) -- which is what builds and
// launches the harness binary. The shell form of the same count is
//
//	git grep -l --untracked -E '\bstart(With)?\(' -- 'playtest/*_test.go' | wc -l
//
// and this regexp is that -E pattern. Nothing shells out to git: the test must
// work in a tree that is not a checkout.
var playtestStart = regexp.MustCompile(`\bstart(With)?\(`)

// harnessVersionRe reads the constant rather than a doc comment or a changelog
// entry, so the version the server reports is the version this gate compares.
var harnessVersionRe = regexp.MustCompile(`harnessVersion\s*=\s*"([^"]+)"`)

// repoRoot walks up from the working directory until it finds go.mod. `go test`
// runs each package in its own directory, so nothing here may assume the root
// is the working directory -- the first version of this gate did, which is one
// of the two reasons it had to move.
func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot read the working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod in any parent of the working directory -- this gate " +
				"resolves every path from the repo root and cannot find it")
		}

		dir = parent
	}
}

// mustRead fails the test rather than returning an error: every path below is
// checked into the repo, so a missing one is a broken gate, not a skip.
func mustRead(t *testing.T, path string) string {
	t.Helper()

	full := filepath.Join(repoRoot(t), filepath.FromSlash(path))

	b, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("cannot read %s: %v", full, err)
	}

	return string(b)
}

// playtestScripts returns the base names of every playtest script, sorted --
// filepath.Glob sorts already, and the order is what makes the failure message
// readable.
func playtestScripts(t *testing.T) []string {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(repoRoot(t), "playtest", "*_test.go"))
	if err != nil {
		t.Fatalf("globbing playtest/*_test.go: %v", err)
	}

	var scripts []string

	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("cannot read %s: %v", p, err)
		}

		if playtestStart.Match(b) {
			scripts = append(scripts, filepath.Base(p))
		}
	}

	if len(scripts) == 0 {
		t.Fatalf("found no playtest scripts under playtest/*_test.go -- either the "+
			"directory moved or %q stopped matching, and a gate that counts zero "+
			"of anything is not a gate", playtestStart.String())
	}

	return scripts
}

// harnessVersionFromSource reads d2app/harness.go, which is behind the harness
// tag and is read here as bytes.
func harnessVersionFromSource(t *testing.T) string {
	t.Helper()

	m := harnessVersionRe.FindStringSubmatch(mustRead(t, "d2app/harness.go"))
	if m == nil {
		t.Fatalf(`no harnessVersion = "..." constant in d2app/harness.go -- if it was `+
			"renamed, rename it in %s and in this test together", harnessDoc)
	}

	return m[1]
}

// harnessToolCount counts mcp.AddTool calls across the tagged harness sources.
// harness_off.go is the !harness stub and is skipped by the same test that
// picks the others up: it does not contain the string "//go:build harness".
func harnessToolCount(t *testing.T) int {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(repoRoot(t), "d2app", "harness*.go"))
	if err != nil {
		t.Fatalf("globbing d2app/harness*.go: %v", err)
	}

	n, scanned := 0, 0

	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("cannot read %s: %v", p, err)
		}

		src := string(b)
		if !strings.Contains(src, "//go:build harness") {
			continue
		}

		scanned++
		n += strings.Count(src, "AddTool(")
	}

	if scanned == 0 || n == 0 {
		t.Fatalf("scanned %d tagged harness files and found %d AddTool( calls -- the "+
			"registration shape changed, and this gate now measures nothing", scanned, n)
	}

	return n
}

// TestHarnessDocScriptCount is R4's first half: docs/harness.md used to say
// "The seven scripts" while playtest/ held fourteen.
func TestHarnessDocScriptCount(t *testing.T) {
	scripts := playtestScripts(t)
	doc := mustRead(t, harnessDoc)
	want := fmt.Sprintf("The %d playtest scripts", len(scripts))

	if !strings.Contains(doc, want) {
		t.Fatalf("%s does not contain %q. There are %d playtest scripts: %s",
			harnessDoc, want, len(scripts), strings.Join(scripts, ", "))
	}
}

// TestHarnessDocListsEveryScript is what stops the count being satisfied by a
// stale list: a script added and counted but never described is the same defect
// one step later.
func TestHarnessDocListsEveryScript(t *testing.T) {
	doc := mustRead(t, harnessDoc)

	var missing []string

	scripts := playtestScripts(t)

	for _, name := range scripts {
		if !strings.Contains(doc, name) {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		t.Fatalf("%s names %d of the playtest scripts but not these: %s. Add a line "+
			"for each to the script list.", harnessDoc, len(scripts)-len(missing),
			strings.Join(missing, ", "))
	}
}

// TestHarnessDocToolsHeading is R4's second half. The heading carries BOTH
// remaining numbers, so it is asserted as one string: the doc said
// "## The tools (36; harness 0.9.1)" while the constant said 0.10.0.
func TestHarnessDocToolsHeading(t *testing.T) {
	tools := harnessToolCount(t)
	version := harnessVersionFromSource(t)
	doc := mustRead(t, harnessDoc)
	want := fmt.Sprintf("## The tools (%d; harness %s)", tools, version)

	if strings.Contains(doc, want) {
		return
	}

	got := "no line starting \"## The tools\""

	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(line, "## The tools") {
			got = strings.TrimRight(line, "\r")

			break
		}
	}

	t.Fatalf("%s heading is %q; derived from source it must be %q (%d AddTool( calls "+
		"in d2app/harness*.go, harnessVersion %s in d2app/harness.go)",
		harnessDoc, got, want, tools, version)
}
