//go:build harness

package d2app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
)

// dialProbe is a settable system that records what is written to it.
type dialProbe struct {
	name   string
	writes []string
}

func (p *dialProbe) HarnessName() string                  { return p.name }
func (p *dialProbe) HarnessState() map[string]interface{} { return map[string]interface{}{} }
func (p *dialProbe) HarnessSet(field string, value interface{}) error {
	p.writes = append(p.writes, field)
	return nil
}

// M4.6 B4a, the load's step 7: a script's DIALS are re-applied by a load --
// the last value of each, in the order each was first written -- and nothing
// that is state or a verb is ever written again over what the load restored.
func TestTheLoadReappliesTheScriptsDialsAndNothingElse(t *testing.T) {
	harnessDials.Lock()
	harnessDials.order, harnessDials.values = nil, nil
	harnessDials.Unlock()

	rising := &dialProbe{name: "rising"}
	meters := &dialProbe{name: "meters"}
	spawns := &dialProbe{name: "spawns"}

	for _, p := range []d2harness.Provider{rising, meters, spawns} {
		d2harness.Register(p)
		defer d2harness.Unregister(p)
	}

	harnessRecordDial("rising", "p", 1.0)
	harnessRecordDial("meters", "health", 0.0)      // state: never re-applied
	harnessRecordDial("spawns", "despawn", "g:1")   // a verb: never re-applied
	harnessRecordDial("spawns", "open_bodies", 3.0) // state: never re-applied
	harnessRecordDial("spawns", "chance", 0.0)
	harnessRecordDial("rising", "p", 0.0) // the last value of p, in p's first place

	if n := harnessReapplyDials(); n != 2 {
		t.Fatalf("two dials re-applied, got %d", n)
	}

	if len(meters.writes) != 0 || len(spawns.writes) != 1 || spawns.writes[0] != "chance" || len(rising.writes) != 1 {
		t.Fatalf("only the dials are written again: meters %v, spawns %v, rising %v", meters.writes, spawns.writes, rising.writes)
	}

	if got := harnessDialNames(); len(got) != 2 || got[0] != "rising.p" || got[1] != "spawns.chance" {
		t.Fatalf("the recorded dials: %v", got)
	}
}

// The uuid provider's report is split for the digest: the stream (seed,
// bytes) is the world file's rng.uuid, and the game counts are this process's.
func TestTheUUIDReportsGameCountsAsProcessState(t *testing.T) {
	t.Cleanup(func() { harnessSeedUUID(0) })

	harnessSeedUUID(1462)

	world, process := harnessUUIDProvider{}.HarnessDigest()

	for _, k := range harnessUUIDProcessKeys {
		if _, ok := world[k]; ok {
			t.Errorf("%s is this process's, and is in the world part", k)
		}

		if _, ok := process[k]; !ok {
			t.Errorf("%s is missing from the process part", k)
		}
	}

	for _, k := range []string{"seeded", "seed", "seed_str", "bytes", "uuids"} {
		if _, ok := world[k]; !ok {
			t.Errorf("%s is the stream's, and is missing from the world part", k)
		}
	}
}

// THE B4a REVIEW, B3: EVERY SETTABLE FIELD OF EVERY PROVIDER IS A DIAL OR NAMED
// AS STATE. The load re-applies the dials a script set (harnessDialFields);
// the village's three radii were dials the list missed, so "load last save"
// put them back at the data's. This reads every provider in the tree -- every
// type with a HarnessSet method, from the source, so a provider added
// tomorrow is read too -- takes its HarnessName and its HarnessSettableFields
// (both must be literals, so they can be read), and requires each field to be
// in exactly one of harnessDialFields and harnessNotDials, and each entry of
// those lists to be a field some provider really has.
func TestEverySettableFieldIsADialOrNamedState(t *testing.T) {
	settable := settableFieldsInTheTree(t)

	if len(settable["village"]) == 0 || len(settable["combat"]) == 0 {
		t.Fatalf("the source read found no village or combat fields -- the reader is broken: %v", settable)
	}

	classified := map[string]string{}

	for system, fields := range harnessDialFields {
		for _, f := range fields {
			classified[system+"."+f] = "dial"
		}
	}

	var problems []string

	for system, fields := range harnessNotDials {
		for _, f := range fields {
			key := system + "." + f
			if classified[key] != "" {
				problems = append(problems, key+" is listed as a dial AND as not one")
			}

			classified[key] = "not a dial"
		}
	}

	exists := map[string]bool{}

	for system, fields := range settable {
		for f := range fields {
			key := system + "." + f
			exists[key] = true

			if classified[key] == "" {
				problems = append(problems, key+" is settable and is in neither harnessDialFields nor harnessNotDials: "+
					"is it a dial a load must re-apply, or state it restores?")
			}
		}
	}

	for key := range classified {
		if !exists[key] {
			problems = append(problems, key+" is classified and no provider has it")
		}
	}

	sort.Strings(problems)

	if len(problems) > 0 {
		t.Fatalf("the dial list is not the providers' fields:\n  %s", strings.Join(problems, "\n  "))
	}
}

// settableFieldsInTheTree reads, from the source under the repository root,
// every type with a HarnessSet method: its system (HarnessName) and its
// fields (HarnessSettableFields), each of which must be a literal.
func settableFieldsInTheTree(t *testing.T) map[string]map[string]bool {
	t.Helper()

	type methods struct {
		name     string
		hasName  bool
		fields   []string
		hasList  bool
		hasSet   bool
		file     string
		listNote string
	}

	byType := map[string]*methods{}
	fset := token.NewFileSet()

	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if base := d.Name(); base == ".git" || base == "node_modules" || base == "testdata" {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		src, err := os.ReadFile(path) // nolint:gosec // the repository's own source
		if err != nil || !strings.Contains(string(src), "Harness") {
			return err
		}

		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			return err
		}

		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}

			recv := fn.Recv.List[0].Type
			if star, ok := recv.(*ast.StarExpr); ok {
				recv = star.X
			}

			ident, ok := recv.(*ast.Ident)
			if !ok {
				continue
			}

			key := filepath.Dir(path) + "." + ident.Name
			m := byType[key]

			if m == nil {
				m = &methods{file: path}
				byType[key] = m
			}

			switch fn.Name.Name {
			case "HarnessSet":
				m.hasSet = true
			case "HarnessName":
				if lit := returnedString(fn); lit != "" {
					m.name, m.hasName = lit, true
				}
			case "HarnessSettableFields":
				m.hasList = true
				m.fields, m.listNote = returnedStrings(fn)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("reading the source: %v", err)
	}

	out := map[string]map[string]bool{}

	for key, m := range byType {
		if !m.hasSet {
			continue
		}

		switch {
		case !m.hasName:
			t.Fatalf("%s (%s) is settable and its HarnessName is not a string literal this test can read", key, m.file)
		case !m.hasList:
			t.Fatalf("%s (%s) is settable and lists no HarnessSettableFields: its fields cannot be classified", key, m.file)
		case m.listNote != "":
			t.Fatalf("%s (%s): HarnessSettableFields %s", key, m.file, m.listNote)
		}

		if out[m.name] == nil {
			out[m.name] = map[string]bool{}
		}

		for _, f := range m.fields {
			out[m.name][f] = true
		}
	}

	return out
}

// returnedString is the string literal a one-statement method returns, or "".
func returnedString(fn *ast.FuncDecl) string {
	if fn.Body == nil || len(fn.Body.List) != 1 {
		return ""
	}

	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return ""
	}

	lit, ok := ret.Results[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}

	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}

	return s
}

// returnedStrings is the []string literal (or nil) a one-statement method
// returns, or a note saying why it cannot be read.
func returnedStrings(fn *ast.FuncDecl) ([]string, string) {
	if fn.Body == nil || len(fn.Body.List) != 1 {
		return nil, "is not one return statement"
	}

	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return nil, "is not one return statement"
	}

	if id, ok := ret.Results[0].(*ast.Ident); ok && id.Name == "nil" {
		return nil, ""
	}

	comp, ok := ret.Results[0].(*ast.CompositeLit)
	if !ok {
		return nil, "does not return a []string literal"
	}

	var out []string

	for _, e := range comp.Elts {
		lit, ok := e.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return nil, "holds an element that is not a string literal"
		}

		s, err := strconv.Unquote(lit.Value)
		if err != nil {
			return nil, err.Error()
		}

		out = append(out, s)
	}

	return out, ""
}
