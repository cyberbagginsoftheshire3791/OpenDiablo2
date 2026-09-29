package d2mapentity

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2loader/asset/types"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2records"
)

// M4.6 B2b: the next-id seam. A load rebuilds an entity with its saved id by
// setting it on the factory; the next construction of any map entity takes
// it, once -- NewNPC and NewCreature wear it, the rest spend it -- and then
// ids come from the uuid stream again.

// b2bFactory is a factory over the repo's own files: enough for a Strigoi
// creature (PNG sheets), with no MPQs.
func b2bFactory(t *testing.T) *MapEntityFactory {
	t.Helper()

	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	require.NoError(t, err)
	require.NoError(t, asset.AddSource(filepath.Join("..", "..", ".."), types.AssetSourceFileSystem))

	return &MapEntityFactory{asset: asset}
}

var b2bDog = CreatureAnimationPaths{
	Idle:   "/data/strigoi/creatures/feral-dog/idle.png",
	Walk:   "/data/strigoi/creatures/feral-dog/walk.png",
	Attack: "/data/strigoi/creatures/feral-dog/attack.png",
	Hit:    "/data/strigoi/creatures/feral-dog/hit.png",
	Death:  "/data/strigoi/creatures/feral-dog/death.png",
	Dead:   "/data/strigoi/creatures/feral-dog/dead.png",
}

func b2bNewDog(t *testing.T, f *MapEntityFactory, x, y int) *Creature {
	t.Helper()

	c, err := f.NewCreature(x, y, "Feral dog", b2bDog, 0, nil)
	require.NoError(t, err)

	return c
}

// A set id is used once; then generation resumes from the uuid stream.
func TestNextEntityIDIsUsedOnceThenGenerationResumes(t *testing.T) {
	f := b2bFactory(t)

	_, waiting := f.PendingEntityID()
	require.False(t, waiting)

	require.NoError(t, f.SetNextEntityID("0b9e3c2e-saved"))

	id, waiting := f.PendingEntityID()
	require.True(t, waiting)
	require.Equal(t, "0b9e3c2e-saved", id)

	first := b2bNewDog(t, f, 50, 50)
	require.Equal(t, "0b9e3c2e-saved", first.ID(), "the next creature takes the set id")

	_, waiting = f.PendingEntityID()
	require.False(t, waiting, "consumed")

	second := b2bNewDog(t, f, 55, 50)
	require.NotEqual(t, "0b9e3c2e-saved", second.ID(), "once: the next one is fresh")

	_, err := uuid.Parse(second.ID())
	require.NoError(t, err, "generation resumed from the uuid stream: %q", second.ID())
}

// A given id draws nothing from the uuid stream: the load restores the
// stream's count after rebuilding (trap 6), so a draw here would be waste --
// and it would shift every id the unseeded stream hands out after it.
func TestASetIDDrawsNoUUID(t *testing.T) {
	var drawn int

	uuid.SetRand(b2bCountingReader{&drawn})
	t.Cleanup(func() { uuid.SetRand(nil) })

	f := b2bFactory(t)

	b2bNewDog(t, f, 50, 50)
	require.Positive(t, drawn, "the control: a fresh id reads the uuid stream")

	drawn = 0

	require.NoError(t, f.SetNextEntityID("e:saved"))
	b2bNewDog(t, f, 50, 50)
	require.Zero(t, drawn, "a set id reads nothing")
}

type b2bCountingReader struct{ n *int }

func (r b2bCountingReader) Read(p []byte) (int, error) {
	*r.n += len(p)

	for i := range p {
		p[i] = byte(i)
	}

	return len(p), nil
}

// A duplicate is REFUSED, with an error, not a panic -- a save holding one id
// twice is a bad file, set aside at load. So is an id set over one still
// waiting, and an empty id or the player's word.
func TestNextEntityIDRefusesDuplicates(t *testing.T) {
	f := b2bFactory(t)

	require.NoError(t, f.SetNextEntityID("e:1"))

	err := f.SetNextEntityID("e:2")
	require.True(t, errors.Is(err, ErrEntityID), "one still waiting: %v", err)

	id, _ := f.PendingEntityID()
	require.Equal(t, "e:1", id, "the refused set changed nothing")

	b2bNewDog(t, f, 50, 50)

	err = f.SetNextEntityID("e:1")
	require.True(t, errors.Is(err, ErrEntityID), "already given: %v", err)

	for _, bad := range []string{"", "player"} {
		require.True(t, errors.Is(f.SetNextEntityID(bad), ErrEntityID), "%q", bad)
	}

	require.NoError(t, f.SetNextEntityID("e:2"), "a new id is still fine")
}

// A construction that FAILS still consumes the id: otherwise it would wait
// for whatever is built next, and the wrong entity would take a saved name.
func TestAFailedConstructionConsumesTheID(t *testing.T) {
	f := b2bFactory(t)

	require.NoError(t, f.SetNextEntityID("e:dog"))

	_, err := f.NewCreature(50, 50, "Nobody", CreatureAnimationPaths{Idle: "/no/such/idle.png"}, 0, nil)
	require.Error(t, err)

	_, waiting := f.PendingEntityID()
	require.False(t, waiting, "a failed creature took its id with it")

	// NewNPC takes it too. With no MPQs its composite cannot load, which is
	// the failure this needs; the records it reads first are stubbed.
	f.asset.Records = &d2records.RecordManager{}
	f.asset.Records.Monster.Stats2 = d2records.MonStats2{"x": &d2records.MonStat2Record{}}

	require.NoError(t, f.SetNextEntityID("e:npc"))

	_, err = f.NewNPC(50, 50, &d2records.MonStatRecord{ExtraDataKey: "x", AnimationDirectoryToken: "NOPE"}, 0)
	require.Error(t, err)

	_, waiting = f.PendingEntityID()
	require.False(t, waiting, "a failed NPC took its id with it")
	require.True(t, errors.Is(f.SetNextEntityID("e:npc"), ErrEntityID), "and it counts as given")
}

// The kinds a save does not rebuild SPEND a waiting id and do not wear it
// (entity_id.go): a missile or an overlay built while an id waits takes it
// out of the seam, so it cannot linger for a later monster -- and does not
// carry a monster's saved name either. Both constructions here fail after the
// take (no such animation), which is also the "consumed even when it fails
// half-way" case for these two.
func TestTheKindsASaveDoesNotRebuildSpendTheID(t *testing.T) {
	f := b2bFactory(t)

	require.NoError(t, f.SetNextEntityID("e:wolf"))

	_, err := f.NewMissile(50, 50, &d2records.MissileRecord{})
	require.Error(t, err, "no such missile animation")

	_, waiting := f.PendingEntityID()
	require.False(t, waiting, "the missile took the waiting id out of the seam")
	require.True(t, errors.Is(f.SetNextEntityID("e:wolf"), ErrEntityID), "and it counts as used")

	dog := b2bNewDog(t, f, 50, 50)
	require.NotEqual(t, "e:wolf", dog.ID(), "the next creature does not inherit it")

	require.NoError(t, f.SetNextEntityID("e:boar"))

	_, err = f.NewCastOverlay(50, 50, &d2records.OverlayRecord{Filename: "nope"})
	require.Error(t, err, "no such overlay")

	_, waiting = f.PendingEntityID()
	require.False(t, waiting, "the overlay spent it too")

	// The seam still works after a spend: a new id is worn by the next creature.
	require.NoError(t, f.SetNextEntityID("e:dog"))
	require.Equal(t, "e:dog", b2bNewDog(t, f, 50, 50).ID())
}

// b2bSeamBirths are the calls that birth a map entity's id: the two that make
// a mapEntity, and an inline uuid.New (b2bIsUUIDNew). Anything that reaches one
// of them -- directly or through any chain of this package's own functions and
// methods -- births an id (the B2b review's B6: the first version matched four
// names in the constructor's own body, so a helper that reached newMapEntity,
// or an id minted inline, walked past it).
var b2bSeamBirths = map[string]bool{"newMapEntity": true, "newMapEntityWithID": true}

// b2bSeamTakes are the two calls that consume the waiting id.
var b2bSeamTakes = map[string]bool{"takeEntityID": true, "spendEntityID": true}

// b2bSeamViolations reads a package's files and returns every EXPORTED
// MapEntityFactory method that births an entity id, transitively, and the
// ones among them that do not take the waiting id in their first statement.
//
// Unexported methods are helpers a constructor calls AFTER its take, so they
// are not held to it themselves; they are what makes the check transitive.
// Calls are matched by name -- a method call by its selector, a function call
// by its identifier -- which over-approximates (two methods with one name on
// different types share their calls), and an over-approximation can only make
// the check stricter.
func b2bSeamViolations(t *testing.T, files []*ast.File) (constructors, violations []string) {
	t.Helper()

	type fn struct {
		decl    *ast.FuncDecl
		factory bool
	}

	funcs := map[string][]fn{}

	for _, file := range files {
		for _, decl := range file.Decls {
			d, ok := decl.(*ast.FuncDecl)
			if !ok || d.Body == nil {
				continue
			}

			funcs[d.Name.Name] = append(funcs[d.Name.Name], fn{d, d.Recv != nil && b2bOnFactory(d)})
		}
	}

	// births[name] is true once a function of that name is known to reach a
	// birth; iterate to a fixed point.
	births := map[string]bool{}

	for changed := true; changed; {
		changed = false

		for name, fs := range funcs {
			if births[name] {
				continue
			}

			for _, f := range fs {
				if b2bReaches(f.decl.Body, births) {
					births[name], changed = true, true

					break
				}
			}
		}
	}

	for name, fs := range funcs {
		for _, f := range fs {
			if !f.factory || !ast.IsExported(name) || !births[name] || b2bSeamTakes[name] {
				continue
			}

			constructors = append(constructors, name)

			if !b2bCalls(f.decl.Body.List[0], b2bSeamTakes) {
				violations = append(violations, name)
			}
		}
	}

	sort.Strings(constructors)
	sort.Strings(violations)

	return constructors, violations
}

// b2bReaches reports whether n calls a birth, an inline uuid.New, or a
// function already known to reach one.
func b2bReaches(n ast.Node, births map[string]bool) bool {
	found := false

	ast.Inspect(n, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || found {
			return !found
		}

		switch fun := call.Fun.(type) {
		case *ast.Ident:
			found = b2bSeamBirths[fun.Name] || births[fun.Name]
		case *ast.SelectorExpr:
			found = b2bIsUUIDNew(fun) || b2bSeamBirths[fun.Sel.Name] || births[fun.Sel.Name]
		}

		return !found
	})

	return found
}

// b2bIsUUIDNew is a call of uuid.New: an id minted inline.
func b2bIsUUIDNew(sel *ast.SelectorExpr) bool {
	pkg, ok := sel.X.(*ast.Ident)

	return ok && pkg.Name == "uuid" && (sel.Sel.Name == "New" || sel.Sel.Name == "NewString" || sel.Sel.Name == "NewRandom")
}

func b2bParseDir(t *testing.T) []*ast.File {
	t.Helper()

	fset := token.NewFileSet()

	paths, err := filepath.Glob("*.go")
	require.NoError(t, err)

	var files []*ast.File

	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		require.NoError(t, err)

		files = append(files, file)
	}

	return files
}

// EVERY factory constructor that births a map entity takes the waiting id,
// and takes it in its FIRST statement -- before anything that can fail. The
// seam is only as good as its coverage: a constructor added later that
// reaches newMapEntity and skips the take would build a fresh-id entity while
// a saved id waits, and the next monster would wear a name meant for another.
// Read from the source, transitively, so a new constructor cannot slip past
// it through a helper or an inline uuid.New.
func TestEveryEntityConstructorTakesTheWaitingID(t *testing.T) {
	constructors, violations := b2bSeamViolations(t, b2bParseDir(t))

	require.Empty(t, violations, "these make a map entity and do not take the waiting id in their first statement")

	// The positive control: the instrument found the seven it must --
	// NewObject by its inline uuid.New, NewItem, NewMissile and
	// NewCastOverlay through NewAnimatedEntity, NewCreature through
	// newCreature.
	require.Equal(t, []string{"NewCastOverlay", "NewCreature", "NewItem", "NewMissile", "NewNPC", "NewObject", "NewPlayer"},
		constructors)
}

// The instrument's negative controls, kept: a constructor that births its
// entity through a helper, and one that mints its id inline, are both found
// and both flagged -- and the same two, taking the id first, are clean.
func TestTheSeamReaderFindsAHelperAndAnInlineUUID(t *testing.T) {
	const src = `package d2mapentity

import "github.com/google/uuid"

type MapEntityFactory struct{}

func newMapEntity(x, y int) int { return 0 }

func (f *MapEntityFactory) spendEntityID() {}

func (f *MapEntityFactory) build(x int) int { return newMapEntity(x, x) }

func (f *MapEntityFactory) deeper(x int) int { return f.build(x) }

func (f *MapEntityFactory) NewThroughAHelper(x int) int { return f.deeper(x) }

func (f *MapEntityFactory) NewWithAnInlineID() string { return uuid.New().String() }

func (f *MapEntityFactory) NewTakingFirst(x int) int {
	f.spendEntityID()
	return f.deeper(x)
}

func (f *MapEntityFactory) NewNothing() int { return 1 }
`

	file, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", src, 0)
	require.NoError(t, err)

	constructors, violations := b2bSeamViolations(t, []*ast.File{file})
	require.Equal(t, []string{"NewTakingFirst", "NewThroughAHelper", "NewWithAnInlineID"}, constructors,
		"a helper chain and an inline uuid.New are both births; a method that births nothing is not a constructor")
	require.Equal(t, []string{"NewThroughAHelper", "NewWithAnInlineID"}, violations)
}

// NewNPC and NewCreature WEAR the waiting id; the rest only spend it (the B2b
// review's B7). No NPC can be BUILT without the MPQs -- NewNPC loads a
// composite from them, and fails after its take -- so this is read from the
// source: in each wearer, the value takeEntityID returns reaches the call that
// makes the entity (newMapEntityWithID, newCreature), and spendEntityID is
// never called. A NewNPC that spent the id and built the NPC with a fresh one
// fails here. The runtime proof is B4b's playtest: a deployed squad model --
// an NPC -- saved and rebuilt must answer to its saved id (docs/m4.6-world-
// save-notes.md). NewCreature's is also proved at runtime, without MPQs, by
// TestNextEntityIDIsUsedOnceThenGenerationResumes.
func TestTheWearersWearTheID(t *testing.T) {
	wearers := map[string]bool{"NewNPC": false, "NewCreature": false}

	for _, file := range b2bParseDir(t) {
		for _, decl := range file.Decls {
			d, ok := decl.(*ast.FuncDecl)
			if !ok || d.Body == nil || d.Recv == nil || !b2bOnFactory(d) {
				continue
			}

			if _, want := wearers[d.Name.Name]; !want {
				continue
			}

			require.False(t, b2bCalls(d.Body, map[string]bool{"spendEntityID": true}), "%s spends the id", d.Name.Name)
			wearers[d.Name.Name] = b2bWearsTheTake(d.Body)
		}
	}

	for name, worn := range wearers {
		require.True(t, worn, "%s does not put the id takeEntityID returns on the entity it makes", name)
	}
}

// b2bWearsTheTake reports whether takeEntityID's result reaches a call of
// newMapEntityWithID or newCreature: passed straight in as an argument, or
// assigned to a local that is.
func b2bWearsTheTake(body *ast.BlockStmt) bool {
	isTake := func(e ast.Expr) bool {
		call, ok := e.(*ast.CallExpr)
		if !ok {
			return false
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)

		return ok && sel.Sel.Name == "takeEntityID"
	}

	held := map[string]bool{} // locals holding the taken id

	ast.Inspect(body, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok && len(as.Lhs) == 1 && len(as.Rhs) == 1 && isTake(as.Rhs[0]) {
			if id, ok := as.Lhs[0].(*ast.Ident); ok {
				held[id.Name] = true
			}
		}

		return true
	})

	worn := false

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return !worn
		}

		name := ""
		if id, ok := call.Fun.(*ast.Ident); ok {
			name = id.Name
		}

		if name != "newMapEntityWithID" && name != "newCreature" {
			return true
		}

		for _, arg := range call.Args {
			if id, ok := arg.(*ast.Ident); (ok && held[id.Name]) || isTake(arg) {
				worn = true
			}
		}

		return !worn
	})

	return worn
}

func b2bOnFactory(fn *ast.FuncDecl) bool {
	star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}

	id, ok := star.X.(*ast.Ident)

	return ok && id.Name == "MapEntityFactory"
}

// b2bCalls reports whether n calls any of the named functions or methods.
func b2bCalls(n ast.Node, names map[string]bool) bool {
	found := false

	ast.Inspect(n, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return !found
		}

		switch fun := call.Fun.(type) {
		case *ast.Ident:
			found = found || names[fun.Name]
		case *ast.SelectorExpr:
			found = found || names[fun.Sel.Name]
		}

		return !found
	})

	return found
}
