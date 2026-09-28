package d2mapentity

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2loader/asset/types"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2records"
)

// M4.6 B2b: the next-id seam. A load rebuilds an entity with its saved id by
// setting it on the factory; the next NewNPC or NewCreature takes it, once,
// and then ids come from the uuid stream again.

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
