package d2asset

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2records"
)

// A table ensured is loaded once: a second EnsureRecords does not read it
// again. One that does not load is not marked, and is tried again.
func TestEnsureRecordsLoadsATableOnce(t *testing.T) {
	dir, err := os.MkdirTemp("", "records")
	require.NoError(t, err)

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	excel := filepath.Join(dir, "data", "global", "excel")
	require.NoError(t, os.MkdirAll(excel, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(excel, "objtype.txt"), []byte("Name\tToken\nChest\tCH\n"), 0o600))

	am := fontAssets(t, dir)

	require.False(t, am.RecordsLoaded(d2resource.ObjectType), "nothing is loaded before it is asked for")
	require.NoError(t, am.EnsureRecords(d2resource.ObjectType))
	require.True(t, am.RecordsLoaded(d2resource.ObjectType))
	require.Len(t, am.Records.Object.Types, 1)
	assert.Equal(t, "ch", am.Records.Object.Types[0].Token)

	// Were it loaded again, the loader would put the record back.
	am.Records.Object.Types = nil

	require.NoError(t, am.EnsureRecords(d2resource.ObjectType))
	assert.Nil(t, am.Records.Object.Types, "an ensured table was loaded a second time")

	// monpreset.txt is not there: refused, not marked, and tried again.
	require.Error(t, am.EnsureRecords(d2resource.MonPreset))
	assert.False(t, am.RecordsLoaded(d2resource.MonPreset))
	require.Error(t, am.EnsureRecords(d2resource.MonPreset), "a refused table is tried again")
}

// The server and the client both build the world, each on its own goroutine:
// ensuring the same table from both at once loads it once and races nothing
// (run under -race in CI).
func TestEnsureRecordsFromTwoGoroutines(t *testing.T) {
	dir, err := os.MkdirTemp("", "records")
	require.NoError(t, err)

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	excel := filepath.Join(dir, "data", "global", "excel")
	require.NoError(t, os.MkdirAll(excel, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(excel, "objtype.txt"), []byte("Name\tToken\nChest\tCH\n"), 0o600))

	am := fontAssets(t, dir)

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			assert.NoError(t, am.EnsureRecords(d2resource.ObjectType))
			assert.True(t, am.RecordsLoaded(d2resource.ObjectType))
		}()
	}

	wg.Wait()
	require.Len(t, am.Records.Object.Types, 1)
}

// levelsAssets is an asset manager whose only table is a one-row levels.txt:
// level 1, the Rogue Encampment.
func levelsAssets(t *testing.T) *AssetManager {
	t.Helper()

	// Not t.TempDir: its cleanup fails the test on Windows while the loader
	// still holds the file (the pattern of the tests above).
	dir, err := os.MkdirTemp("", "levels")
	require.NoError(t, err)

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	excel := filepath.Join(dir, "data", "global", "excel")
	require.NoError(t, os.MkdirAll(excel, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(excel, "Levels.txt"),
		[]byte("Name\tId\tSoundEnv\tLevelName\nAct 1 - Town\t1\t1\tRogue Encampment\n"), 0o600))

	return fontAssets(t, dir)
}

// BUG-23 (27 Sep 2026): the level record is read under the lock the lazy load
// writes it under, so a reader -- Game.Advance's region check, the harness's
// tile tool -- waits out a load in progress instead of racing it. Measured
// by holding the lock as a load holds it: the read must not return until the
// lock is let go. Deterministic on purpose: -race needs cgo, which the laptop
// does not have (CI runs TestLevelDetailsBesideALoad under -race).
//
// Negative control (27 Sep 2026): read the map without the lock and this
// fails -- the read returns while the load holds it.
func TestLevelDetailsWaitsForALoad(t *testing.T) {
	am := levelsAssets(t)

	assert.Nil(t, am.LevelDetails(1), "levels.txt is not loaded until it is ensured")
	require.NoError(t, am.EnsureRecords(d2resource.LevelDetails))

	town := am.LevelDetails(1)
	require.NotNil(t, town)
	assert.Equal(t, "Rogue Encampment", town.LevelDisplayName)
	assert.Equal(t, 1, town.SoundEnvironmentID)
	assert.Nil(t, am.LevelDetails(99), "a level levels.txt does not have is nil")

	am.recordsLoaded.mu.Lock() // a load in progress

	got := make(chan *d2records.LevelDetailRecord, 1)

	go func() { got <- am.LevelDetails(1) }()

	select {
	case <-got:
		am.recordsLoaded.mu.Unlock()
		t.Fatal("LevelDetails returned while a load held the records lock: it reads without the lock")
	case <-time.After(200 * time.Millisecond):
	}

	am.recordsLoaded.mu.Unlock()

	select {
	case r := <-got:
		assert.Same(t, town, r)
	case <-time.After(5 * time.Second):
		t.Fatal("LevelDetails never returned once the load let go of the lock")
	}
}

// The race BUG-23 names, as it would happen: one goroutine builds a generated
// world, loading levels.txt, while the game screen reads the level record
// once a second. Run under -race in CI, where a read of the RecordManager's
// map without the lock is reported; the laptop has no cgo, so here it proves
// only that the two finish and agree.
func TestLevelDetailsBesideALoad(t *testing.T) {
	am := levelsAssets(t)
	done := make(chan struct{})

	go func() {
		defer close(done)

		assert.NoError(t, am.EnsureRecords(d2resource.LevelDetails))
	}()

	for {
		select {
		case <-done:
			require.NotNil(t, am.LevelDetails(1), "the load finished and the record is there")
			return
		default:
			_ = am.LevelDetails(1)
		}
	}
}
