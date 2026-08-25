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
}
