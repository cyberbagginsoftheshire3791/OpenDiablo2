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

// b2bSeamCallees are the calls that birth a map entity's id.
var b2bSeamCallees = map[string]bool{
	"newMapEntity": true, "newMapEntityWithID": true, "NewAnimatedEntity": true, "newCreature": true,
}

// EVERY factory constructor that births a map entity takes the waiting id,
// and takes it in its FIRST statement -- before anything that can fail. The
// seam is only as good as its coverage: a constructor added later that calls
// newMapEntity and skips the take would build a fresh-id entity while a saved
// id waits, and the next monster would wear a name meant for another. Read
// from the source, so a new constructor cannot slip past it.
func TestEveryEntityConstructorTakesTheWaitingID(t *testing.T) {
	fset := token.NewFileSet()

	paths, err := filepath.Glob("*.go")
	require.NoError(t, err)

	var constructors []string

	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		require.NoError(t, err)

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil || !b2bOnFactory(fn) {
				continue
			}

			if !b2bCalls(fn.Body, b2bSeamCallees) {
				continue
			}

			constructors = append(constructors, fn.Name.Name)

			first := fn.Body.List[0]
			require.True(t, b2bCalls(first, map[string]bool{"takeEntityID": true, "spendEntityID": true}),
				"%s makes a map entity and does not take the waiting id in its first statement", fn.Name.Name)
		}
	}

	sort.Strings(constructors)

	// The positive control: the instrument found the six it must.
	require.Equal(t, []string{"NewCastOverlay", "NewCreature", "NewItem", "NewMissile", "NewNPC", "NewPlayer"}, constructors)
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
