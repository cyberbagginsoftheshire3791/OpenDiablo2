package d2world

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ----------------------------------------------------------------------------
// The B2a helpers every *_snapshot_test.go in this package uses. They are named
// b2a* so that B2b's snapshots, built in parallel, cannot collide with them.
// ----------------------------------------------------------------------------

// b2aJSON is v as JSON: the form every observable is compared in, because it
// is the form the digest and the save-resume playtest compare.
func b2aJSON(t *testing.T, v interface{}) string {
	t.Helper()

	raw, err := json.Marshal(v)
	require.NoError(t, err)

	return string(raw)
}

// b2aThroughJSON writes v to JSON and reads it back into a new value: the trip
// the world file makes.
func b2aThroughJSON[T any](t *testing.T, v T) T {
	t.Helper()

	raw, err := json.Marshal(v)
	require.NoError(t, err)

	var out T
	require.NoError(t, json.Unmarshal(raw, &out))

	return out
}

// b2aClassified requires every field of the struct kind (a zero value of it)
// to be classified, and every classification to name a field, so a field added
// to a system later fails here until someone decides whether the save carries
// it. A classification is one of:
//
//	"S:<path>"  saved: <path> must resolve in snap's JSON (arrays as [0])
//	"D: why"    derived on load (by B4's order, or by the game's own wiring)
//	"T: why"    transient: empty at every moment a save is allowed
//	"W: why"    wiring: a callback, an interface or construction-time config
func b2aClassified(t *testing.T, kind interface{}, snap interface{}, fields map[string]string) {
	t.Helper()

	typ := reflect.TypeOf(kind)
	tree := b2aTree(t, snap)
	have := map[string]bool{}

	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		have[name] = true

		class, ok := fields[name]
		if !ok {
			t.Errorf("%s.%s is not classified: decide whether the save carries it (S), derives it on load (D), "+
				"finds it empty at every save (T), or it is wiring (W)", typ.Name(), name)

			continue
		}

		switch {
		case strings.HasPrefix(class, "S:"):
			path := strings.TrimPrefix(class, "S:")
			if _, ok := b2aResolve(tree, path); !ok {
				t.Errorf("%s.%s is classified saved at %q, but the snapshot has no such key", typ.Name(), name, path)
			}
		case strings.HasPrefix(class, "D: "), strings.HasPrefix(class, "T: "), strings.HasPrefix(class, "W: "):
			if len(strings.TrimSpace(class[3:])) < 10 {
				t.Errorf("%s.%s: a %c classification needs its reason", typ.Name(), name, class[0])
			}
		default:
			t.Errorf("%s.%s: classification %q is not S:, D:, T: or W:", typ.Name(), name, class)
		}
	}

	for name := range fields {
		if !have[name] {
			t.Errorf("%s.%s is classified but is not a field", typ.Name(), name)
		}
	}
}

// b2aTree is v's JSON as a generic tree, numbers kept exact.
func b2aTree(t *testing.T, v interface{}) interface{} {
	t.Helper()

	raw, err := json.Marshal(v)
	require.NoError(t, err)

	return b2aDecode(t, raw)
}

func b2aDecode(t *testing.T, raw []byte) interface{} {
	t.Helper()

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	var tree interface{}
	require.NoError(t, dec.Decode(&tree))

	return tree
}

// b2aStep is one step of a path into a JSON tree.
type b2aStep struct {
	key   string
	index int
	isIdx bool
}

func b2aPathString(path []b2aStep) string {
	var b strings.Builder

	for _, s := range path {
		if s.isIdx {
			fmt.Fprintf(&b, "[%d]", s.index)
			continue
		}

		if b.Len() > 0 {
			b.WriteByte('.')
		}

		b.WriteString(s.key)
	}

	return b.String()
}

// b2aResolve follows a path like "squads[0].members[0].entity".
func b2aResolve(tree interface{}, path string) (interface{}, bool) {
	node := tree

	for _, part := range strings.Split(path, ".") {
		key, rest := part, ""
		if i := strings.IndexByte(part, '['); i >= 0 {
			key, rest = part[:i], part[i:]
		}

		if key != "" {
			obj, ok := node.(map[string]interface{})
			if !ok {
				return nil, false
			}

			if node, ok = obj[key]; !ok {
				return nil, false
			}
		}

		for rest != "" {
			end := strings.IndexByte(rest, ']')
			if !strings.HasPrefix(rest, "[") || end < 0 {
				return nil, false
			}

			i, err := strconv.Atoi(rest[1:end])
			arr, ok := node.([]interface{})

			if err != nil || !ok || i < 0 || i >= len(arr) {
				return nil, false
			}

			node, rest = arr[i], rest[end+1:]
		}
	}

	return node, true
}

// b2aVariant is the snapshot with one thing taken from it.
type b2aVariant struct {
	path string
	how  string // zeroed | perturbed (it was already zero, so it is set) | dropped
	raw  []byte
}

// b2aVariants is every way of taking one thing from snap: each leaf zeroed
// (or, if it is already zero, set, since zeroing a zero takes nothing), and
// each array element and each object key dropped -- a map entry's loss is its
// key's. A seed written as a
// decimal string is zeroed as a number. The snapshot must hold no empty
// collection -- an empty list has nothing to take, so a fixture with one has
// not tested it -- and no variant may decode to the snapshot it came from.
func b2aVariants(t *testing.T, snap interface{}) []b2aVariant {
	t.Helper()

	raw, err := json.Marshal(snap)
	require.NoError(t, err)

	base := b2aDecode(t, raw)

	var leaves, drops [][]b2aStep

	var walk func(path []b2aStep, node interface{})

	walk = func(path []b2aStep, node interface{}) {
		switch n := node.(type) {
		case map[string]interface{}:
			if len(n) == 0 {
				t.Errorf("fixture: %q is an empty object; fill it, or nothing in it is tested", b2aPathString(path))
			}

			keys := make([]string, 0, len(n))
			for k := range n {
				keys = append(keys, k)
			}

			sort.Strings(keys)

			for _, k := range keys {
				child := append(append([]b2aStep(nil), path...), b2aStep{key: k})
				drops = append(drops, child)
				walk(child, n[k])
			}
		case []interface{}:
			if len(n) == 0 {
				t.Errorf("fixture: %q is an empty list; fill it, or nothing in it is tested", b2aPathString(path))
			}

			for i := range n {
				child := append(append([]b2aStep(nil), path...), b2aStep{index: i, isIdx: true})
				drops = append(drops, child)
				walk(child, n[i])
			}
		case nil:
			t.Errorf("fixture: %q is null; a snapshot writes an empty collection as [] or {}, and the fixture "+
				"should fill it", b2aPathString(path))
		default:
			leaves = append(leaves, path)
		}
	}

	walk(nil, base)

	typ := reflect.TypeOf(snap)
	out := make([]b2aVariant, 0, len(leaves)+len(drops))

	// at is the node a path leads to in tree.
	at := func(tree interface{}, path []b2aStep) interface{} {
		node := tree

		for _, s := range path {
			if s.isIdx {
				node = node.([]interface{})[s.index]
			} else {
				node = node.(map[string]interface{})[s.key]
			}
		}

		return node
	}

	// put writes v at path in tree (path is never empty: the root is the
	// snapshot object itself).
	put := func(tree interface{}, path []b2aStep, v interface{}) {
		parent, last := at(tree, path[:len(path)-1]), path[len(path)-1]

		if last.isIdx {
			parent.([]interface{})[last.index] = v
		} else {
			parent.(map[string]interface{})[last.key] = v
		}
	}

	emit := func(path []b2aStep, how string, tree interface{}) {
		b, err := json.Marshal(tree)
		require.NoError(t, err)

		// A variant that decodes to the snapshot it came from took nothing.
		back := reflect.New(typ)
		if json.Unmarshal(b, back.Interface()) == nil {
			again, err := json.Marshal(back.Elem().Interface())
			require.NoError(t, err)

			if bytes.Equal(again, raw) {
				// Dropping a key whose value is already its zero takes
				// nothing, and the zeroed variant of that key covers it.
				if how == "dropped" {
					return
				}

				t.Errorf("fixture: %s %s decodes to the same snapshot; the fixture must hold a value "+
					"whose loss the snapshot itself can see", how, b2aPathString(path))

				return
			}
		}

		out = append(out, b2aVariant{path: b2aPathString(path), how: how, raw: b})
	}

	for _, path := range leaves {
		tree := b2aDecode(t, raw)
		v, wasZero := b2aPerturb(at(tree, path))
		put(tree, path, v)

		how := "zeroed"
		if wasZero {
			how = "perturbed"
		}

		emit(path, how, tree)
	}

	for _, path := range drops {
		tree := b2aDecode(t, raw)
		parentPath, last := path[:len(path)-1], path[len(path)-1]

		if last.isIdx {
			arr := at(tree, parentPath).([]interface{})
			put(tree, parentPath, append(append([]interface{}{}, arr[:last.index]...), arr[last.index+1:]...))
		} else {
			delete(at(tree, parentPath).(map[string]interface{}), last.key)
		}

		emit(path, "dropped", tree)
	}

	return out
}

// b2aPerturb is a leaf's zero, or -- when it is already zero -- a value that
// is not; wasZero reports the second case.
func b2aPerturb(v interface{}) (out interface{}, wasZero bool) {
	switch x := v.(type) {
	case json.Number:
		if f, err := x.Float64(); err == nil && f == 0 {
			return json.Number("1"), true
		}

		return json.Number("0"), false
	case bool:
		return !x, !x
	case string:
		// A seed is an int64 written as a decimal string, so its zero is
		// "0" rather than "", which would not decode at all.
		if n, err := strconv.ParseInt(x, 10, 64); err == nil {
			if n == 0 {
				return "1", true
			}

			return "0", false
		}

		if x == "" {
			return "b2a", true
		}

		return "", false
	default:
		panic(fmt.Sprintf("b2aPerturb: %T is not a leaf", v))
	}
}

// b2aSweep restores every variant of snap and requires each one to be refused
// by the decode or the Restore, or to diverge from reference, the original's
// observable after the same steps. try decodes a variant, restores it into a
// fresh system, runs those steps and returns what they observed.
//
// exempt names the paths whose loss is unobservable BY DESIGN, each with the
// reason. Every variant at an exempt path must be silent (an exemption that
// diverges is a wrong claim), and every exemption must be reached, so the list
// cannot rot into a place for holes to hide.
//
// THE BASELINE COMES FIRST: the untouched snapshot, put through try exactly as
// every variant is, must be refused by nothing and must observe reference.
// Without it the sweep proves nothing about a try that restores differently
// from the original -- a binding made in another order, a step missed, a trace
// that is not deterministic -- because then EVERY variant diverges, the loss of
// each field included, and each divergence is counted as that field being
// seen. (Added at the B2a review, 28 Sep 2026: the squads sweep restores along
// a path -- BindPlayer after Restore -- that no run-on comparison had covered.)
func b2aSweep(t *testing.T, snap interface{}, reference string, exempt map[string]string,
	try func(raw []byte) (string, error)) {
	t.Helper()

	raw, err := json.Marshal(snap)
	require.NoError(t, err)

	base, err := try(raw)
	require.NoError(t, err, "the baseline: the untouched snapshot must restore")

	if at, want, got := b2aFirstDiff(reference, base); at >= 0 {
		t.Fatalf("the baseline: the untouched snapshot, restored and run on as every variant is, does not "+
			"match the original (first difference at byte %d: original %q, copy %q); every divergence below "+
			"would be this one, not the field's", at, want, got)
	}

	variants := b2aVariants(t, snap)
	require.NotEmpty(t, variants)

	reached := map[string]bool{}
	counts := map[string]int{}

	for _, v := range variants {
		obs, err := try(v.raw)

		why, isExempt := exempt[v.path]
		if isExempt {
			reached[v.path] = true
		}

		switch {
		case isExempt && (err != nil || obs != reference):
			t.Errorf("%s %s: exempt as %q, but its loss is seen (err=%v); the exemption is wrong", v.how, v.path, why, err)
		case isExempt:
			counts["exempt"]++
			t.Logf("%-9s %-44s exempt: %s", v.how, v.path, why)
		case err != nil:
			counts["refused"]++
			t.Logf("%-9s %-44s refused: %v", v.how, v.path, err)
		case obs != reference:
			counts["diverged"]++
			t.Logf("%-9s %-44s diverged", v.how, v.path)
		default:
			t.Errorf("%s %s: restored and run on, the copy matches the original -- losing it changed nothing "+
				"observable, so the save either does not need it or nothing can see it", v.how, v.path)
		}
	}

	for path := range exempt {
		if !reached[path] {
			t.Errorf("exemption %q is reached by no variant; remove it", path)
		}
	}

	t.Logf("sweep: %d variants: %d refused, %d diverged, %d exempt",
		len(variants), counts["refused"], counts["diverged"], counts["exempt"])
}

// b2aFirstDiff is where a and b first differ (-1 if they do not), with a little
// of each from there, so a failure says what moved rather than dumping two
// traces.
func b2aFirstDiff(a, b string) (at int, fromA, fromB string) {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}

	at = 0
	for at < n && a[at] == b[at] {
		at++
	}

	if at == n && len(a) == len(b) {
		return -1, "", ""
	}

	clip := func(s string) string {
		if len(s) > at+80 {
			return s[at : at+80]
		}

		return s[at:]
	}

	return at, clip(a), clip(b)
}

// b2aMustDiverge is the sweep's complement for the fields a Restore checks.
//
// The sweep counts a variant Restore refuses as its field being seen, and for
// a field that is only ever refused -- an id, a next id, a count tied to
// another count -- that proves the CHECK works, not that the field matters:
// lose it together with what it is checked against, consistently, and nothing
// would refuse. So each wrong mutates snap into a snapshot that is VALID but
// not the one saved (a fresh model's ids and counts, a next id one past the
// last, another stage, another squad selected), and each must be ACCEPTED by
// Restore and must DIVERGE from reference. One that is refused is a check that
// goes further than it should or a probe that is not valid; one that matches
// is a field the save does not need or nothing can see.
func b2aMustDiverge[T any](t *testing.T, snap T, reference string, wrongs map[string]func(s *T),
	try func(raw []byte) (string, error)) {
	t.Helper()

	names := make([]string, 0, len(wrongs))
	for name := range wrongs {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		v := b2aThroughJSON(t, snap) // a deep copy: a probe must not reach snap
		wrongs[name](&v)

		raw, err := json.Marshal(v)
		require.NoError(t, err)

		obs, err := try(raw)

		switch {
		case err != nil:
			t.Errorf("probe %q: refused (%v); a valid-but-wrong snapshot must restore, so that what it "+
				"lost can be seen", name, err)
		case obs == reference:
			t.Errorf("probe %q: accepted and run on, the copy matches the original -- what it changed is "+
				"not seen", name)
		default:
			t.Logf("probe     %-44s diverged", name)
		}
	}
}
