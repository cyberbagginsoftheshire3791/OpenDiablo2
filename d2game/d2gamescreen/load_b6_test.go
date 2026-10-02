package d2gamescreen

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
)

// M4.6 B6: A REFUSAL OF THE FILE NAMES ITS RULE. The omit sweep holds each
// dropped block's refusal to the rule that guards it, not only to its code
// (a FILE refusal is any of twenty-odd rules: the B3 review's A2, one level
// up). Each case is step 1 of a real load (PrepareLoad), and the rule must be
// on the refusal and on the report the harness hands a script; a refusal that
// is not d2save's names none. THE CONTROL is the good file, which is taken.
func TestARefusalNamesItsRule(t *testing.T) {
	saved, save := b4Game(t)
	b4Busy(t, saved)

	good, _ := b4File(t, saved, save, "t.world.json")

	top := func(t *testing.T, s string) map[string]json.RawMessage {
		data, err := os.ReadFile(d2save.WorldPath(s))
		require.NoError(t, err)

		var m map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(data, &m))

		return m
	}

	put := func(t *testing.T, s string, m map[string]json.RawMessage) {
		data, err := json.Marshal(m)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(d2save.WorldPath(s), data, 0o600))
	}

	// The control.
	s := b4Files(t, good, "Saver", b4Amazon)
	w, r := PrepareLoad(s, false)
	require.Nil(t, r)
	require.NotNil(t, w)
	require.Empty(t, LastLoad().Rule, "a file taken names no rule")
	forgetPreload(s)

	for _, c := range []struct {
		name, code, rule string
		network          bool
		edit             func(t *testing.T, s string)
	}{
		{"a block omitted", LoadRefusedFile, d2save.ReasonBlockMissing, false, func(t *testing.T, s string) {
			m := top(t, s)
			delete(m, "seek")
			put(t, s, m)
		}},
		{"no version", LoadRefusedVersion, d2save.ReasonVersion, false, func(t *testing.T, s string) {
			m := top(t, s)
			delete(m, "version")
			put(t, s, m)
		}},
		{"a seed its streams were not derived from", LoadRefusedFile, d2save.ReasonRNGStream, false, func(t *testing.T, s string) {
			m := top(t, s)
			m["seed"] = json.RawMessage(`"12345"`)
			put(t, s, m)
		}},
		{"another hero's .od2", LoadRefusedHero, d2save.ReasonPairHero, false, func(t *testing.T, s string) {
			od2, err := json.Marshal(map[string]interface{}{"heroName": "Other", "heroType": b4Amazon})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(s, od2, 0o600))
		}},
		{"a sidecar of another generation", LoadRefusedTorn, d2save.ReasonPairMoment, false, func(t *testing.T, s string) {
			require.NoError(t, os.WriteFile(d2items.SidecarPath(s), []byte(`{"version": 1, "generation": "another", "kit": {}}`), 0o600))
		}},
		{"a network game (not the file's own rule)", LoadRefusedNetwork, "", true, func(*testing.T, string) {}},
	} {
		s := b4Files(t, good, "Saver", b4Amazon)
		c.edit(t, s)

		_, r := PrepareLoad(s, c.network)
		require.NotNil(t, r, c.name)
		require.Equal(t, c.code, r.Code, "%s: %s", c.name, r.Detail)
		require.Equal(t, c.rule, r.Rule, "%s: %s", c.name, r.Detail)

		rep := LastLoad()
		require.Equal(t, c.code, rep.Refused, c.name)
		require.Equal(t, c.rule, rep.Rule, "%s: the report the harness hands a script names the rule", c.name)
		require.NotEmpty(t, rep.SetAside, "%s: set aside (rule 7)", c.name)
	}

	// The next load starts a report of its own: no rule carried over.
	s = b4Files(t, good, "Saver", b4Amazon)
	_, r = PrepareLoad(s, false)
	require.Nil(t, r)
	require.Empty(t, LastLoad().Rule, "a load after a refusal names no rule of the one before")
	forgetPreload(s)
}
