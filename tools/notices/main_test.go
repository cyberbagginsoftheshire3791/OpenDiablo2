package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsLicence(t *testing.T) {
	for name, want := range map[string]bool{
		"LICENSE": true, "LICENSE.md": true, "LICENSE-APACHE": true, "licence.txt": true,
		"COPYING": true, "NOTICE": true, "NOTICE.txt": true, "PATENTS": true,
		"LICENSE.underscorejs": true, "LICENSE-2.0.txt": true,
		"LICENSES": false, "license_test.go": false, "license.go": false, "LICENSE.go": false,
		"README.md": false, "licensing.go": false, "go.mod": false,
	} {
		if got := isLicence(name); got != want {
			t.Errorf("%q: matched %v, want %v", name, got, want)
		}
	}
}

// A licence beside the code it covers is found and named by its path in the
// module, each file once, CRLF read as LF, in path order.
func TestWriteLicences(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "underscore")

	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatal(err)
	}

	for p, text := range map[string]string{
		filepath.Join(root, "LICENSE"):             "MIT\r\nroot\r\n",
		filepath.Join(root, "PATENTS"):             "patents",
		filepath.Join(root, "license_test.go"):     "package x",
		filepath.Join(sub, "LICENSE.underscorejs"): "underscore",
	} {
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var b bytes.Buffer
	if err := writeLicences(&b, root, []string{root, sub, root}); err != nil {
		t.Fatal(err)
	}

	out := b.String()

	if strings.Count(out, "`LICENSE`:") != 1 || strings.Contains(out, "\r") {
		t.Fatalf("the root licence once, LF only:\n%s", out)
	}

	i, j, k := strings.Index(out, "`LICENSE`"), strings.Index(out, "`PATENTS`"), strings.Index(out, "`underscore/LICENSE.underscorejs`")
	if i < 0 || j < 0 || k < 0 || !(i < j && j < k) {
		t.Fatalf("want LICENSE, PATENTS, underscore/LICENSE.underscorejs in that order:\n%s", out)
	}

	if strings.Contains(out, "license_test.go") {
		t.Fatalf("a Go file was taken for a licence:\n%s", out)
	}
}
