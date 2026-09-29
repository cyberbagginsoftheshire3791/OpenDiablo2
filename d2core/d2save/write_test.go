package d2save

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func b3Read(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(data)
}

func b3Absent(t *testing.T, path string) {
	t.Helper()

	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err), "%s must not exist (%v)", path, err)
}

func TestWorldPath(t *testing.T) {
	require.Equal(t, "N.od2.world.json", WorldPath("N.od2"))
	require.Equal(t, "", WorldPath(""))
	require.Equal(t, "w.v2.unread", UnreadPath("w", "2"))
	require.Equal(t, "w.v-1.unread", UnreadPath("w", "-1"))
	require.Equal(t, "w.unread", UnreadPath("w", `"1"`))
	require.Equal(t, "w.unread", UnreadPath("w", ""))
}

// THE .BAK GENERATION (rule 5): each save keeps the one before it, and only
// that one; no temporary file is left behind.
func TestWriteWorldKeepsTheLastGeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saves", "0.od2.world.json")
	a, b, c := `{"version": 1, "n": "a"}`, `{"version": 1, "n": "b"}`, `{"version": 1, "n": "c"}`

	w, err := WriteWorld(path, []byte(a))
	require.NoError(t, err)
	require.Equal(t, Written{Path: path}, w, "the first save has no generation to keep")
	require.Equal(t, a, b3Read(t, path))
	b3Absent(t, path+".bak")

	w, err = WriteWorld(path, []byte(b))
	require.NoError(t, err)
	require.Equal(t, path+".bak", w.Bak)
	require.Equal(t, b, b3Read(t, path))
	require.Equal(t, a, b3Read(t, path+".bak"))

	_, err = WriteWorld(path, []byte(c))
	require.NoError(t, err)
	require.Equal(t, c, b3Read(t, path))
	require.Equal(t, b, b3Read(t, path+".bak"), "the .bak is the LAST save, not the first")

	b3Absent(t, path+".tmp")
	b3Absent(t, path+".bak.tmp")

	_, err = WriteWorld("", []byte(a))
	require.Error(t, err)
}

// RULE 7: a file this build cannot read is never overwritten. It is set aside
// under a name nothing holds, the new file is written, and the .bak -- the
// last save THIS build made -- is left as it was.
func TestWriteWorldSetsAsideWhatItCannotRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "0.od2.world.json")
	bak, v2, v2b := `{"version": 1, "n": "bak"}`, `{"version": 2, "n": "newer"}`, `{"version": 2, "n": "newer again"}`

	require.NoError(t, os.WriteFile(path+".bak", []byte(bak), 0o600))
	require.NoError(t, os.WriteFile(path, []byte(v2), 0o600))

	w, err := WriteWorld(path, []byte(`{"version": 1, "n": "mine"}`))
	require.NoError(t, err)
	require.Equal(t, path+".v2.unread", w.SetAside)
	require.Empty(t, w.Bak, "a file this build cannot read is not its last save")
	require.Equal(t, v2, b3Read(t, path+".v2.unread"))
	require.Equal(t, bak, b3Read(t, path+".bak"), "the .bak is untouched")
	require.Equal(t, `{"version": 1, "n": "mine"}`, b3Read(t, path))

	// A second such file never overwrites the first set aside.
	require.NoError(t, os.WriteFile(path, []byte(v2b), 0o600))

	w, err = WriteWorld(path, []byte(`{"version": 1, "n": "mine again"}`))
	require.NoError(t, err)
	require.Equal(t, path+".v2.unread.1", w.SetAside)
	require.Equal(t, v2, b3Read(t, path+".v2.unread"))
	require.Equal(t, v2b, b3Read(t, path+".v2.unread.1"))

	// Not a world file at all: set aside as plain .unread.
	require.NoError(t, os.WriteFile(path, []byte("garbage"), 0o600))

	w, err = WriteWorld(path, []byte(`{"version": 1}`))
	require.NoError(t, err)
	require.Equal(t, path+".unread", w.SetAside)
	require.Equal(t, "garbage", b3Read(t, path+".unread"))
}

// THE WRITE GOES THROUGH A TEMPORARY FILE AND A RENAME: with the temporary
// file's place taken by a directory, WriteWorld fails and the file it would
// have replaced is exactly as it was. A write straight into the file would
// succeed here, and a crash in the middle of one would leave half a world.
func TestWriteWorldGoesThroughATemporaryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "0.od2.world.json")
	old := `{"version": 1, "n": "old"}`

	require.NoError(t, os.WriteFile(path, []byte(old), 0o600))
	require.NoError(t, os.Mkdir(path+".tmp", 0o750))

	_, err := WriteWorld(path, []byte(`{"version": 1, "n": "new"}`))
	require.Error(t, err)
	require.Equal(t, old, b3Read(t, path), "a failed write leaves the file as it was")
}
