package d2save

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

// b3Save is a whole world file, readable, told apart by its build.
func b3Save(t *testing.T, build string) string {
	t.Helper()

	w := b3Fixture()
	w.Build = build

	return string(b3Encode(t, w))
}

// THE .BAK GENERATION (rule 5): each save keeps the one before it, and only
// that one; no temporary file is left behind.
func TestWriteWorldKeepsTheLastGeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saves", "0.od2.world.json")
	a, b, c := b3Save(t, "a"), b3Save(t, "b"), b3Save(t, "c")

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

	w, err = WriteWorld(path, []byte(b3Save(t, "mine again")))
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

// A BAD VERSION-1 FILE IS NEVER KEPT AS THE .BAK (the B3 review's C item). It
// used to be: anything of version 1 was copied over the .bak, so a file that
// no load reads -- a torn write, a hand edit, a file from before a field
// existed -- replaced the one good save a load could fall back on. It is set
// aside as a version-1 file this build cannot read, and the .bak is kept.
func TestWriteWorldNeverKeepsABadFileAsTheBak(t *testing.T) {
	path := filepath.Join(t.TempDir(), "0.od2.world.json")
	good, bad := b3Save(t, "good"), `{"version": 1, "n": "torn"}`

	require.NoError(t, os.WriteFile(path+".bak", []byte(good), 0o600))
	require.NoError(t, os.WriteFile(path, []byte(bad), 0o600))

	w, err := WriteWorld(path, []byte(b3Save(t, "new")))
	require.NoError(t, err)
	require.Empty(t, w.Bak, "a file no load reads is not the last save")
	require.Equal(t, path+".v1.unread", w.SetAside)
	require.Equal(t, good, b3Read(t, path+".bak"), "the good .bak is kept")
	require.Equal(t, bad, b3Read(t, path+".v1.unread"), "the bad file is set aside, not lost")
	require.Equal(t, b3Save(t, "new"), b3Read(t, path))

	// The control: a readable version-1 file IS the .bak.
	_, err = WriteWorld(path, []byte(b3Save(t, "newer")))
	require.NoError(t, err)
	require.Equal(t, b3Save(t, "new"), b3Read(t, path+".bak"))
}

// A FILE HELD OPEN IS STILL SET ASIDE (the B3 review's C item). On Windows a
// rename is refused while anything has the file open -- a reader, a scanner,
// Explorer's preview -- and the setting aside used to try once and fail the
// save. It retries, as WriteFileAtomic always did (d2items.RenameRetrying).
// On Linux the rename succeeds at once and the test shows nothing; the laptop
// the game ships on is Windows, where it has teeth.
func TestWriteWorldSetsAsideAFileHeldOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "0.od2.world.json")
	v2 := `{"version": 2, "n": "newer"}`
	require.NoError(t, os.WriteFile(path, []byte(v2), 0o600))

	held, err := os.Open(path)
	require.NoError(t, err)

	released := make(chan struct{})

	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = held.Close()
		close(released)
	}()

	w, err := WriteWorld(path, []byte(b3Save(t, "mine")))
	<-released

	require.NoError(t, err, "a file held open for a moment is set aside once it is let go")
	require.Equal(t, path+".v2.unread", w.SetAside)
	require.Equal(t, v2, b3Read(t, path+".v2.unread"))
}

// THE WRITE GOES THROUGH A TEMPORARY FILE AND A RENAME: with the temporary
// file's place taken by a directory, WriteWorld fails and the file it would
// have replaced is exactly as it was. A write straight into the file would
// succeed here, and a crash in the middle of one would leave half a world.
func TestWriteWorldGoesThroughATemporaryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "0.od2.world.json")
	old := b3Save(t, "old")

	require.NoError(t, os.WriteFile(path, []byte(old), 0o600))
	require.NoError(t, os.Mkdir(path+".tmp", 0o750))

	_, err := WriteWorld(path, []byte(b3Save(t, "new")))
	require.Error(t, err)
	require.Equal(t, old, b3Read(t, path), "a failed write leaves the file as it was")
}

// M4.6 B4a: THE LOAD SETS A REFUSED FILE ASIDE, never overwriting it or
// anything already set aside, under the version it holds (rule 7), and leaves
// the .bak alone.
func TestSetAsideKeepsWhatTheLoadRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "0.od2.world.json")
	one, two, newer := b3Save(t, "refused"), b3Save(t, "refused again"), `{"version": 2}`

	require.NoError(t, os.WriteFile(path+".bak", []byte("the last save"), 0o600))
	require.NoError(t, os.WriteFile(path, []byte(one), 0o600))

	aside, err := SetAside(path)
	require.NoError(t, err)
	require.Equal(t, path+".v1.unread", aside)
	require.Equal(t, one, b3Read(t, aside))
	b3Absent(t, path)
	require.Equal(t, "the last save", b3Read(t, path+".bak"), "the .bak is untouched")

	require.NoError(t, os.WriteFile(path, []byte(two), 0o600))

	aside, err = SetAside(path)
	require.NoError(t, err)
	require.Equal(t, path+".v1.unread.1", aside, "a second refusal never overwrites the first")
	require.Equal(t, one, b3Read(t, path+".v1.unread"))

	require.NoError(t, os.WriteFile(path, []byte(newer), 0o600))

	aside, err = SetAside(path)
	require.NoError(t, err)
	require.Equal(t, path+".v2.unread", aside, "a newer build's file keeps its version in the name")

	_, err = SetAside(path)
	require.Error(t, err, "nothing to set aside is an error, not a silent success")
}
