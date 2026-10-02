package d2gamescreen

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
)

// M4.6 B6, BUG-113: THE KIT IS IN THE DIGEST. The "kit" provider reports what
// he carries, all of it world state (no process part), and a kit that differs
// -- an emptied pack, another torch's minutes -- reports differently. Its
// control is the same kit read twice, which reports the same.
func TestTheKitProviderReportsWhatHeCarries(t *testing.T) {
	v, _ := b4Game(t)
	require.NotNil(t, v.kit, "the unit game has a torch-and-blade kit")

	p := kitProvider{v}
	require.Equal(t, "kit", p.HarnessName())

	_, isDigester := interface{}(p).(d2harness.Digester)
	require.False(t, isDigester, "every field is the world's: a resume must reproduce the kit")

	before := p.HarnessState()
	require.Equal(t, true, before["bound"])
	require.Equal(t, v.kit.Loadout, before["loadout"])
	require.NotEmpty(t, before["pack"], "the loadout gives him a pack")
	require.NotEmpty(t, before["worn"])
	require.Equal(t, before, p.HarnessState(), "the control: the same kit reports the same")

	pack := v.kit.Pack
	v.kit.Pack = []d2items.Instance{}
	require.NotEqual(t, before, p.HarnessState(), "an emptied pack reports differently")
	v.kit.Pack = pack

	_, torch, ok := v.kit.OffHandTorch()
	require.True(t, ok, "the loadout's torch is in his off hand")

	burn := torch.BurnLeft
	torch.BurnLeft = burn + 1
	require.NotEqual(t, before, p.HarnessState(), "another torch's minutes report differently")
	torch.BurnLeft = burn

	require.Equal(t, before, p.HarnessState())

	v.kit = nil
	require.Equal(t, false, p.HarnessState()["bound"])
}
