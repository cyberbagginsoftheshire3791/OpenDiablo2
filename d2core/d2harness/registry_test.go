package d2harness

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type fakeProvider struct {
	name  string
	state map[string]interface{}
}

func (f *fakeProvider) HarnessName() string                  { return f.name }
func (f *fakeProvider) HarnessState() map[string]interface{} { return f.state }
func (f *fakeProvider) HarnessSet(string, interface{}) error { return nil }
func (f *fakeProvider) HarnessSettableFields() []string      { return []string{"day"} }

func TestRegisterLookup(t *testing.T) {
	first := &fakeProvider{name: "clock", state: map[string]interface{}{"day": 1}}
	second := &fakeProvider{name: "clock", state: map[string]interface{}{"day": 2}}

	Register(first)
	Register(second)

	got, ok := Lookup("clock")
	assert.True(t, ok)
	assert.Equal(t, 2, got.HarnessState()["day"], "Lookup returns the newest registration")

	_, ok = Lookup("meters")
	assert.False(t, ok)

	assert.GreaterOrEqual(t, len(Providers()), 2)

	var s Settable = second
	assert.NoError(t, s.HarnessSet("day", 3))

	var fl FieldLister = second
	assert.Equal(t, []string{"day"}, fl.HarnessSettableFields())

	// Stateful is the entity half of the contract; every Provider satisfies it.
	var st Stateful = first
	assert.Equal(t, 1, st.HarnessState()["day"])
}

func TestUnregisterAndNames(t *testing.T) {
	ui := &fakeProvider{name: "ui", state: map[string]interface{}{}}
	stale := &fakeProvider{name: "ui", state: map[string]interface{}{"stale": true}}
	light := &fakeProvider{name: "light", state: map[string]interface{}{}}

	Register(stale)
	Register(ui)
	Register(light)

	names := Names()
	assert.Contains(t, names, "ui")
	assert.Contains(t, names, "light")
	assert.Equal(t, 1, count(names, "ui"), "Names dedupes duplicate registrations")

	Unregister(ui)

	got, ok := Lookup("ui")
	assert.True(t, ok, "the older registration is still there")
	assert.Equal(t, true, got.HarnessState()["stale"])

	Unregister(stale)
	Unregister(light)

	_, ok = Lookup("ui")
	assert.False(t, ok)
	assert.NotContains(t, Names(), "light")
}

func count(names []string, want string) int {
	n := 0

	for _, name := range names {
		if name == want {
			n++
		}
	}

	return n
}
