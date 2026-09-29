package d2items

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// M4.6 B3 review, B7: the sidecar carries the generation of the world save it
// belongs to, LoadHero reads it back, and a sidecar no world save has touched
// writes none.
func TestTheSidecarCarriesItsGeneration(t *testing.T) {
	k := kitFor(t, "torch-and-blade")
	path := filepath.Join(t.TempDir(), "0.od2.strigoi.json")

	if err := SaveHero(path, k, Extras{Generation: "2026-09-29T08:00:00Z"}); err != nil {
		t.Fatal(err)
	}

	_, x, err := LoadHero(path, shipped(t))
	if err != nil {
		t.Fatal(err)
	}

	if x.Generation != "2026-09-29T08:00:00Z" {
		t.Fatalf("LoadHero read generation %q", x.Generation)
	}

	if err := SaveHero(path, k, Extras{}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatal(err)
	}

	if _, ok := top["generation"]; ok {
		t.Fatalf("a sidecar of no world save writes no generation: %s", data)
	}
}

// The review's C item: a rename refused while something holds the file is
// retried. On Windows a file open for reading cannot be renamed; the holder
// lets go after 150 ms, well inside the half second of retries. On Linux the
// rename succeeds at once and this shows nothing.
func TestRenameRetryingWaitsOutAHolder(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "a"), filepath.Join(dir, "b")

	if err := os.WriteFile(from, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	held, err := os.Open(from)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})

	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = held.Close()
		close(done)
	}()

	err = RenameRetrying(from, to)
	<-done

	if err != nil {
		t.Fatalf("a rename refused while the file was held was not retried: %v", err)
	}

	if _, err := os.Stat(to); err != nil {
		t.Fatal(err)
	}
}

// The review's C item: the temporary file is flushed to disk before the
// rename. No test can watch an fsync happen, so this one reads the source:
// WriteFileAtomic writes through writeSynced, before it renames, and
// writeSynced calls File.Sync.
func TestWriteFileAtomicFlushesBeforeItRenames(t *testing.T) {
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "sidecar.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	calls := map[string][]string{}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				switch f := call.Fun.(type) {
				case *ast.Ident:
					calls[fn.Name.Name] = append(calls[fn.Name.Name], f.Name)
				case *ast.SelectorExpr:
					calls[fn.Name.Name] = append(calls[fn.Name.Name], f.Sel.Name)
				}
			}

			return true
		})
	}

	index := func(list []string, name string) int {
		for i, s := range list {
			if s == name {
				return i
			}
		}

		return -1
	}

	w, r := index(calls["WriteFileAtomic"], "writeSynced"), index(calls["WriteFileAtomic"], "RenameRetrying")
	if w < 0 || r < 0 || w > r {
		t.Fatalf("WriteFileAtomic must write the temporary file through writeSynced before RenameRetrying; it calls %v",
			calls["WriteFileAtomic"])
	}

	s, c := index(calls["writeSynced"], "Sync"), index(calls["writeSynced"], "Close")
	if s < 0 || c < 0 || s > c {
		t.Fatalf("writeSynced must Sync the file before it closes it; it calls %v", calls["writeSynced"])
	}
}
