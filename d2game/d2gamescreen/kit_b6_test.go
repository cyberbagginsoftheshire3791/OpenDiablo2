package d2gamescreen

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
)

// M4.6 B6, BUG-113: THE KIT IS IN THE DIGEST. The "kit" provider reports what
// he carries, all of it world state (no process part), and EXACTLY the kit
// the sidecar writes (the B6 review's B3: the provider is held to the
// sidecar's own JSON of the kit, field for field, so a provider that dropped
// a field -- make, condition, points, count -- is red, not only one that
// dropped the pack). Each of those fields is then changed in turn, and each
// change is reported. The control is the same kit read twice.
func TestTheKitProviderReportsWhatHeCarries(t *testing.T) {
	v, _ := b4Game(t)
	require.NotNil(t, v.kit, "the unit game has a torch-and-blade kit")

	p := kitProvider{v}
	require.Equal(t, "kit", p.HarnessName())

	_, isDigester := interface{}(p).(d2harness.Digester)
	require.False(t, isDigester, "every field is the world's: a resume must reproduce the kit")

	// The provider is the sidecar's kit, and nothing else but bound and
	// choosing_loadout.
	asTheSidecar := func(what string) map[string]interface{} {
		state := p.HarnessState()
		require.Equal(t, true, state["bound"], what)
		require.Equal(t, false, state["choosing_loadout"], what)

		delete(state, "bound")
		delete(state, "choosing_loadout")

		data, err := d2items.HeroBytes(v.kit, d2items.Extras{})
		require.NoError(t, err)

		var doc struct {
			Kit map[string]interface{} `json:"kit"`
		}

		require.NoError(t, json.Unmarshal(data, &doc))
		require.Equal(t, doc.Kit, state, "%s: the provider reports the kit as the sidecar writes it", what)

		return state
	}

	before := asTheSidecar("the loadout")
	require.NotEmpty(t, before["pack"], "the loadout gives him a pack")
	require.NotEmpty(t, before["worn"])
	require.Equal(t, before, asTheSidecar("the control: read again"))

	body := v.kit.Worn[d2items.SlotBody]
	require.NotNil(t, body, "the loadout's mail")
	require.NotZero(t, body.Points, "the mail has points")

	counted := -1

	for i := range v.kit.Pack {
		if v.kit.Pack[i].Count > 0 {
			counted = i
			break
		}
	}

	require.GreaterOrEqual(t, counted, 0, "the pack holds a counted item (arrows, peksimet)")

	_, torch, ok := v.kit.OffHandTorch()
	require.True(t, ok, "the loadout's torch is in his off hand")

	for _, c := range []struct {
		name   string
		change func() func()
	}{
		{"an emptied pack", func() func() {
			pack := v.kit.Pack
			v.kit.Pack = []d2items.Instance{}

			return func() { v.kit.Pack = pack }
		}},
		{"the mail's make", func() func() {
			was := body.Make
			body.Make = d2items.MakeLord

			return func() { body.Make = was }
		}},
		{"the mail's condition", func() func() {
			was := body.Condition
			body.Condition = d2items.Damaged

			return func() { body.Condition = was }
		}},
		{"the mail's points", func() func() {
			was := body.Points
			body.Points = was - 1

			return func() { body.Points = was }
		}},
		{"a count in the pack", func() func() {
			was := v.kit.Pack[counted].Count
			v.kit.Pack[counted].Count = was + 1

			return func() { v.kit.Pack[counted].Count = was }
		}},
		{"the torch's minutes", func() func() {
			was := torch.BurnLeft
			torch.BurnLeft = was + 1

			return func() { torch.BurnLeft = was }
		}},
	} {
		undo := c.change()
		require.NotEqual(t, before, asTheSidecar(c.name), "%s is reported", c.name)
		undo()
		require.Equal(t, before, asTheSidecar(c.name+", undone"))
	}

	v.kit = nil
	require.Equal(t, false, p.HarnessState()["bound"])
}
