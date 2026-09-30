package d2save

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The M4.6 B5 review: RestoreWorld (A2, BUG-98) and the torn file's name
// (C5, BUG-104).

// A SAVE THAT FAILED AFTER ITS WORLD FILE LANDED PUTS IT BACK: the previous
// save, kept as the .bak, goes back where it was (and the .bak keeps it too);
// a first save, with nothing before it, is removed, as though never written.
func TestRestoreWorldPutsTheLastSaveBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saves", "0.od2.world.json")
	last, next := b3Save(t, "the last save"), b3Save(t, "the save that failed")

	_, err := WriteWorld(path, []byte(last))
	require.NoError(t, err)

	w, err := WriteWorld(path, []byte(next))
	require.NoError(t, err)
	require.Equal(t, next, b3Read(t, path), "the premise: the new world file landed")

	require.NoError(t, RestoreWorld(w))
	require.Equal(t, last, b3Read(t, path), "the world file is the last save again")
	require.Equal(t, last, b3Read(t, path+".bak"), "and the .bak still holds it")
	b3Absent(t, path+".tmp")

	// A first save: nothing to put back, so the new file goes.
	first := filepath.Join(t.TempDir(), "0.od2.world.json")

	w, err = WriteWorld(first, []byte(next))
	require.NoError(t, err)
	require.Empty(t, w.Bak)

	require.NoError(t, RestoreWorld(w))
	b3Absent(t, first)

	// A file this build cannot read, set aside by the write, STAYS aside
	// (rule 7): the put-back removes only the file the save wrote.
	odd := filepath.Join(t.TempDir(), "0.od2.world.json")
	require.NoError(t, os.WriteFile(odd, []byte(`{"version": `+b3Next+`}`), 0o600))

	w, err = WriteWorld(odd, []byte(next))
	require.NoError(t, err)
	require.NotEmpty(t, w.SetAside)

	require.NoError(t, RestoreWorld(w))
	b3Absent(t, odd)
	require.Equal(t, `{"version": `+b3Next+`}`, b3Read(t, w.SetAside), "the file set aside is kept, never lost")

	require.Error(t, RestoreWorld(Written{}), "nothing written, nothing to put back")
}

// A TORN FILE IS SET ASIDE UNDER A NAME THAT SAYS SO: .torn.unread, not the
// .v<Version>.unread that reads "another version" (probe B's .v2.unread); a
// second never overwrites the first.
func TestATornFileIsSetAsideAsTorn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "0.od2.world.json")
	one, two := b3Save(t, "torn"), b3Save(t, "torn again")

	require.Equal(t, path+".torn.unread", TornPath(path))

	require.NoError(t, os.WriteFile(path, []byte(one), 0o600))

	aside, err := SetAsideTorn(path)
	require.NoError(t, err)
	require.Equal(t, path+".torn.unread", aside)
	require.Equal(t, one, b3Read(t, aside))
	b3Absent(t, path)

	require.NoError(t, os.WriteFile(path, []byte(two), 0o600))

	aside, err = SetAsideTorn(path)
	require.NoError(t, err)
	require.Equal(t, path+".torn.unread.1", aside)
	require.Equal(t, one, b3Read(t, path+".torn.unread"), "the first is kept")

	_, err = SetAsideTorn(path)
	require.Error(t, err, "nothing to set aside is an error")
}
