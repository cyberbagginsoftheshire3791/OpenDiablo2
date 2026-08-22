package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// allowlistFile is the machine-readable companion of docs/fixtures-manifest.md:
// one repo-relative path per line; blank lines and # comments ignored.
const allowlistFile = "docs/fixtures-allowlist.txt"

// checkFixtures lists every tracked file whose extension is a protected
// Diablo II format and compares it with the allowlist. It returns a one-line
// report, the violations (tracked but not allow-listed), and an error when
// git or the allowlist cannot be read. Stale allowlist entries are reported
// in the summary but are not violations.
func checkFixtures(root string) (report string, violations []string, err error) {
	ls := runCmd(root, gitTimeout, "git", "ls-files", "-z")
	if !ls.ok {
		return "", nil, fmt.Errorf("git ls-files: %s", firstLine(ls.output))
	}

	allowed, err := readAllowlist(filepath.Join(root, filepath.FromSlash(allowlistFile)))
	if err != nil {
		return "", nil, err
	}

	var tracked []string

	for _, f := range strings.Split(ls.output, "\x00") {
		if f == "" {
			continue
		}

		if protectedExt[strings.ToLower(filepath.Ext(f))] {
			tracked = append(tracked, f)
		}
	}

	sort.Strings(tracked)

	seen := map[string]bool{}

	for _, f := range tracked {
		seen[f] = true

		if !allowed[f] {
			violations = append(violations, f)
		}
	}

	var stale []string

	for f := range allowed {
		if !seen[f] {
			stale = append(stale, f)
		}
	}

	sort.Strings(stale)

	switch {
	case len(violations) > 0:
		report = fmt.Sprintf("%d tracked file(s) with a protected extension are NOT in %s: %s",
			len(violations), allowlistFile, strings.Join(violations, ", "))
	default:
		report = fmt.Sprintf("%d tracked file(s) with a protected extension, all listed in %s",
			len(tracked), allowlistFile)
		if len(tracked) > 0 {
			report += " (" + strings.Join(tracked, ", ") + ")"
		}
	}

	if len(stale) > 0 {
		report += fmt.Sprintf("; %d allow-listed path(s) no longer tracked: %s", len(stale), strings.Join(stale, ", "))
	}

	return report, violations, nil
}

func readAllowlist(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s is missing", allowlistFile)
		}

		return nil, err
	}
	defer f.Close()

	allowed := map[string]bool{}
	sc := bufio.NewScanner(f)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		allowed[strings.ReplaceAll(line, "\\", "/")] = true
	}

	return allowed, sc.Err()
}

// runCheckFixtures is the CI entry point: exit 1 on a violation.
func runCheckFixtures(root string, w, errw io.Writer) int {
	report, violations, err := checkFixtures(root)
	if err != nil {
		fmt.Fprintf(errw, "strigoihook check-fixtures: %v\n", err)

		return 1
	}

	if len(violations) > 0 {
		fmt.Fprintf(errw, "Article V violation: %s\nEither remove the file (synthesize the bytes in the test instead) or, if it is provably not Blizzard-derived, add it to %s with a row in docs/fixtures-manifest.md.\n",
			report, allowlistFile)

		return 1
	}

	fmt.Fprintf(w, "Article V: %s\n", report)

	return 0
}
