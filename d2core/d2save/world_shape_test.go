package d2save

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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
//     change green: the hash for version 1 is what it was, so the change must
//     also bump Version and record the new version's hash -- which is the
//     rule. (A field removed or renamed changes the hash too.)
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
