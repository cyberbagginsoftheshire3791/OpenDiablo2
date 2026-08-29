//go:build playtest

package playtest

import (
	"fmt"
	"testing"
)

// The controls for flag(), A3's replacement for the fail-open bool read.
//
// These need no game and no MPQs -- run them alone with
//
//	go test -tags playtest ./playtest/... -run TestFlag -count=1
//
// A helper whose entire purpose is to FAIL when a field is missing has to be
// shown failing, or it is exactly the act of faith it was written to replace.
// The pattern it replaces, `if v, _ := m[k].(bool); !v`, passed happily on an
// absent key for four milestones; nobody noticed because nothing ever asked it
// to prove it could tell the two apart.

// fakeTB captures the failure instead of ending the test. Embedding testing.TB
// satisfies the interface's unexported method; the embedded value is nil and
// any method this test does not override would panic, which is the loud
// failure we want if flag() ever starts calling something else.
type fakeTB struct {
	testing.TB

	failed bool
	msg    string
}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Fatalf(format string, args ...any) {
	f.failed = true
	f.msg = fmt.Sprintf(format, args...)
}

func TestFlagReadsARealBool(t *testing.T) {
	m := map[string]any{"noticed": true, "sees": false}

	f := &fakeTB{}
	if got := flag(f, m, "noticed"); !got || f.failed {
		t.Fatalf("true field read as %v (failed=%v, %s)", got, f.failed, f.msg)
	}

	f = &fakeTB{}
	if got := flag(f, m, "sees"); got || f.failed {
		t.Fatalf("false field read as %v (failed=%v, %s)", got, f.failed, f.msg)
	}
}

// THE NEGATIVE CONTROL, and the whole reason A3 exists. Under the old form
// this case returned false and a negative assertion passed.
func TestFlagFailsOnAnAbsentField(t *testing.T) {
	f := &fakeTB{}

	flag(f, map[string]any{"noticed": true}, "notic3d")

	if !f.failed {
		t.Fatal("a misspelt field name did not fail -- this is precisely the fail-open read A3 removed")
	}

	// The message has to name what IS there, because the failure it is most
	// often reporting is a rename, and a rename is only legible next to the
	// names that replaced it.
	if want := "noticed"; !contains(f.msg, want) {
		t.Fatalf("failure message does not list the present keys, so a rename is unreadable from it: %s", f.msg)
	}
}

func TestFlagFailsOnTheWrongType(t *testing.T) {
	f := &fakeTB{}

	flag(f, map[string]any{"noticed": 1.0}, "noticed")

	if !f.failed {
		t.Fatal("a float in a bool's place did not fail")
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}

	return false
}
