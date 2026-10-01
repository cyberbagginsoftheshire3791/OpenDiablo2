package d2save

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// B3-8 WITH TEETH (the B3 review, B1): "any change to the file's shape bumps
// Version". Two halves:
//
//   - THE GOLDEN FILE: testdata/world-v<Version>.json is a real file of this
//     version, committed. It must decode -- with no field missing, which
//     Decode now refuses -- and encode back to itself byte for byte. A field
//     added to any snapshot turns it red at once ("clock.rate is missing").
//   - THE SHAPE IS PINNED TO THE VERSION: every path the World type can write,
//     with its JSON options and its kind, hashed. The hash is recorded here
//     per version. Regenerating the golden file alone does not make a shape
//     change green: the hash recorded for the current Version is what it was,
//     so the change must also bump Version and record the new version's hash
//     -- which is the rule. (A field removed or renamed changes the hash too.)
//
// BOTH HALVES ARE "THE CURRENT VERSION", never version 1 (the raid's R0.5,
// 29 Sep 2026): the golden file is world-v<Version>.json and the pin is
// b3ShapeHashes[Version]. A bump leaves every older golden file committed,
// as the file an older build wrote, and TestAnOlderVersionsFileIsRefusedAndSetAside
// holds each one refused on its version and set aside by its name (rule 7:
// there is no migration).
//
// To move to a new shape: bump Version, run
//
//	go test ./d2core/d2save -run TestTheFileShapeIsPinnedToItsVersion -update-golden
//
// which writes testdata/world-v<Version>.json from the fixture and prints the
// new shape's hash, and record the hash below under the new version. Never
// edit the hash of a version that has shipped.
var b3ShapeHashes = map[int]string{
	1: "3f7fb4b1b510b802fd0c7c6f2da0dd5c0f161921275ba0b26a81cd490c97087a",
	// The raid's R1 (29 Sep 2026): combat.clock and rng.combat_clock. The
	// merge with M4.6 B4b recomputes this once, if B4b changed a shape too.
	2: "fa82243b4569ce29eb4b2a5c7179f36489d4a90accf8800fa1fe61c586b71565",
	// The raid's R2 and its review fixes (29 Sep and 1 Oct 2026):
	// notice.watches[].side, the seek block and pursuit.rechase_solves. R2
	// first amended version 2 with no bump (f0748aba..., never shipped);
	// Josh's ruling of 30 Sep (every shape change bumps) made it version 3.
	3: "60cf9f52b1c40fe9cd1e24e1069e9c5c6eb9240ef8c5e956a6eba8e6a07d0b9c",
}

var b3UpdateGolden = flag.Bool("update-golden", false, "write testdata/world-v<Version>.json from the fixture and print the shape hash")

func b3GoldenPath() string {
	return filepath.Join("testdata", fmt.Sprintf("world-v%d.json", Version))
}

func TestTheFileShapeIsPinnedToItsVersion(t *testing.T) {
	shape := b3Shape()
	sum := sha256.Sum256([]byte(strings.Join(shape, "\n")))
	hash := hex.EncodeToString(sum[:])

	if *b3UpdateGolden {
		require.NoError(t, os.MkdirAll("testdata", 0o750))
		require.NoError(t, os.WriteFile(b3GoldenPath(), b3Encode(t, b3Fixture()), 0o600))
		t.Logf("wrote %s; the shape of version %d hashes to %s (record it in b3ShapeHashes)", b3GoldenPath(), Version, hash)
	}

	golden, err := os.ReadFile(b3GoldenPath())
	require.NoError(t, err, "every version has its golden file")

	w, err := Decode(golden)
	require.NoError(t, err, "the committed version-%d file must still be read -- a field added without a bump is missing from it", Version)

	again := b3Encode(t, w)
	require.True(t, bytes.Equal(golden, again), "the golden file encodes back to itself:\n%s", again)

	require.Equal(t, b3ShapeHashes[Version], hash,
		"THE WORLD FILE'S SHAPE CHANGED and Version is still %d. B3-8: any change to the file's shape bumps Version "+
			"(there is no migration; an older file is set aside). Bump Version, regenerate the golden file with "+
			"-update-golden, and record the new hash. The shape now:\n%s", Version, strings.Join(shape, "\n"))
}

// b3Shape is every path the World type can write, one line each: the path
// (a list is [], a map {}), the field's JSON options, and the kind of what
// is written there. Sorted.
func b3Shape() []string {
	var out []string

	b3ShapeOf(reflect.TypeOf(World{}), "", &out)
	sort.Strings(out)

	return out
}

var (
	b3Marshaler  = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	b3RawMessage = reflect.TypeOf(json.RawMessage{})
)

func b3ShapeOf(t reflect.Type, path string, out *[]string) {
	switch {
	case t == b3RawMessage:
		*out = append(*out, path+" raw")

		return
	case t.Implements(b3Marshaler) || reflect.PtrTo(t).Implements(b3Marshaler):
		*out = append(*out, path+" marshaler "+t.String())

		return
	}

	switch t.Kind() {
	case reflect.Ptr:
		b3ShapeOf(t.Elem(), path+"?", out)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				continue
			}

			tag := f.Tag.Get("json")
			if tag == "-" {
				continue
			}

			name, opts := tag, ""
			if j := strings.Index(tag, ","); j >= 0 {
				name, opts = tag[:j], tag[j:]
			}

			if name == "" {
				name = f.Name
			}

			sub := name + opts
			if path != "" {
				sub = path + "." + sub
			}

			b3ShapeOf(f.Type, sub, out)
		}
	case reflect.Slice:
		b3ShapeOf(t.Elem(), path+"[]", out)
	case reflect.Array:
		b3ShapeOf(t.Elem(), fmt.Sprintf("%s[%d]", path, t.Len()), out)
	case reflect.Map:
		b3ShapeOf(t.Elem(), path+"{}", out)
	default:
		*out = append(*out, path+" "+t.Kind().String())
	}
}

// AN OLDER VERSION'S FILE IS REFUSED AND SET ASIDE BY ITS NAME (the raid's
// R0.5, 29 Sep 2026; rule 7: "a save this build cannot read ... is neither
// opened nor overwritten", and there is no migration). A bump moves Version,
// writes testdata/world-v<Version>.json and records its hash; each older
// golden file stays committed as what an older build wrote. Every one must be
// refused on its version -- a *VersionError naming it, before a block is read
// -- and both ways a file is moved must set it aside under its own version's
// name: the load's SetAside, and the save's WriteWorld, which must never keep
// it as the .bak. The current golden file with its version set one lower
// always stands in as well, so at a version with no older file committed the
// test still has one to refuse.
func TestAnOlderVersionsFileIsRefusedAndSetAside(t *testing.T) {
	golden, err := os.ReadFile(b3GoldenPath())
	require.NoError(t, err, "every version has its golden file")

	cur := []byte(fmt.Sprintf("\"version\": %d,", Version))
	require.True(t, bytes.Contains(golden, cur), "the golden file holds this build's version")

	type older struct {
		what    string
		version int
		data    []byte
	}

	cases := []older{{"the golden file, one version back", Version - 1,
		bytes.Replace(golden, cur, []byte(fmt.Sprintf("\"version\": %d,", Version-1)), 1)}}

	committed, err := filepath.Glob(filepath.Join("testdata", "world-v*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, committed)

	for _, path := range committed {
		var n int

		_, err := fmt.Sscanf(filepath.Base(path), "world-v%d.json", &n)
		require.NoError(t, err, path)
		require.LessOrEqual(t, n, Version, "%s: a golden file of a version this build has not reached", path)
		require.NotEmpty(t, b3ShapeHashes[n], "%s: a version's recorded shape hash stays recorded", path)

		if n < Version {
			data, err := os.ReadFile(path)
			require.NoError(t, err)

			cases = append(cases, older{path, n, data})
		}
	}

	for _, tc := range cases {
		name := strconv.Itoa(tc.version)
		t.Logf("version %d, %s: refused on its version and set aside as .v%s.unread", tc.version, tc.what, name)

		_, err := Decode(tc.data)
		require.ErrorIs(t, err, ErrWorldVersion, tc.what)
		require.Equal(t, ReasonVersion, ReasonOf(err), tc.what)

		var ve *VersionError
		require.True(t, errors.As(err, &ve), tc.what)
		require.Equal(t, name, ve.Version, tc.what)
		require.Equal(t, name, VersionOf(tc.data), tc.what)

		path := filepath.Join(t.TempDir(), "1.od2.world.json")

		// The load's half (step 1's refusal and teardown).
		require.NoError(t, os.WriteFile(path, tc.data, 0o600))

		aside, err := SetAside(path)
		require.NoError(t, err, tc.what)
		require.Equal(t, path+".v"+name+".unread", aside, tc.what)

		moved, err := os.ReadFile(aside)
		require.NoError(t, err)
		require.True(t, bytes.Equal(tc.data, moved), "%s: set aside byte for byte", tc.what)

		// The save's half: a save over an older file sets it aside too, under
		// a name nothing holds, and keeps no .bak of it.
		require.NoError(t, os.WriteFile(path, tc.data, 0o600))

		w, err := WriteWorld(path, golden)
		require.NoError(t, err, tc.what)
		require.Equal(t, path+".v"+name+".unread.1", w.SetAside, "%s: the second never overwrites the first", tc.what)
		require.Empty(t, w.Bak, "%s: an older version's file is never kept as the .bak", tc.what)
	}
}
