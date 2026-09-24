package d2asset

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
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
