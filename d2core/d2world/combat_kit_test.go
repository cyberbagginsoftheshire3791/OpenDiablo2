package d2world

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T2, the kit in the resolver. The kits are built from the SHIPPED item table,
// so these numbers are the game's, and every assertion has its bare-handed
// control -- a kit that did nothing and a kit that did everything both pass a
// one-sided test.

type fakeKits map[string]*d2items.Kit

func (f fakeKits) KitOf(id string) *d2items.Kit { return f[id] }

func shippedKit(t *testing.T, loadout string) *d2items.Kit {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "data", "strigoi", "items.json"))
	require.NoError(t, err)

	c, err := d2items.Load(data)
	require.NoError(t, err)

	k, err := c.NewKit(loadout)
	require.NoError(t, err)

	return k
}

// blowsOn is every action row in the round's log with the given target.
func blowsOn(t *testing.T, f *resolverFight, target string) []map[string]interface{} {
	t.Helper()

	var out []map[string]interface{}

	for _, row := range f.actions(t) {
		if row["target"] == target {
			out = append(out, row)
		}
	}

	return out
}

func TestKitMailAbsorbsAndWears(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.set(t, "forced_band", BandHit)
	f.set(t, "player_action", PlayerActionHold)

	kit := shippedKit(t, "torch-and-blade")
	f.c.SetKits(fakeKits{"p:1": kit})

	f.add(t, "e:1", 400, Profile{}) // placeholder bite: cut, 2-5
	f.open(t)

	points := kit.Worn[d2items.SlotBody].Points
	f.round()

	rows := blowsOn(t, f, "p:1")
	require.Len(t, rows, 1)

	row := rows[0]
	assert.Equal(t, 4, row["absorbed"], "mail takes 4 off a cut")
	assert.Equal(t, "cut", row["damage_class"])
	assert.Equal(t, 1, row["damage"], "2-5 less 4 floors at 1: bounded at the bottom")
	assert.Equal(t, points-1, kit.Worn[d2items.SlotBody].Points, "a hit wears the mail a point")

	// THE CONTROL: the same seed, the same forced hit, no kit.
	g := newResolverFight(t, 1462)
	g.set(t, "forced_band", BandHit)
	g.set(t, "player_action", PlayerActionHold)
	g.add(t, "e:1", 400, Profile{})
	g.open(t)
	g.round()

	bare := blowsOn(t, g, "p:1")[0]
	assert.Equal(t, 0, bare["absorbed"])
	assert.Equal(t, bare["base"], bare["damage"], "bare, a hit does its base")
	assert.Equal(t, row["base"], bare["base"], "and the kit drew nothing from the RNG: same base")
	assert.Equal(t, row["roll"], bare["roll"], "same roll")
}

func TestKitShieldTurnsOneBlowARound(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.set(t, "forced_band", BandCrit)
	f.set(t, "player_action", PlayerActionHold)

	kit := shippedKit(t, "sword-and-board")
	f.c.SetKits(fakeKits{"p:1": kit})

	pack := Profile{Group: "g:1", Row: "wolves", Speed: 5, DamageMin: 6, DamageMax: 12, DamageClass: "thrust"}
	f.add(t, "e:1", 400, pack)
	f.add(t, "e:2", 400, pack).y = 41
	f.open(t)

	shield := kit.Worn[d2items.SlotOff].Points
	f.round()

	rows := blowsOn(t, f, "p:1")
	require.Len(t, rows, 2)

	assert.Equal(t, true, rows[0]["blocked"], "the first crit of the round is turned")
	assert.Equal(t, BandHit, rows[0]["band"])
	assert.Equal(t, BandCrit, rows[0]["band_rolled"])
	assert.Equal(t, false, rows[1]["blocked"], "the second is not: one a round")
	assert.Equal(t, BandCrit, rows[1]["band"])
	assert.Equal(t, shield-1, kit.Worn[d2items.SlotOff].Points, "a block costs the shield a point")

	// Next round, the shield is ready again.
	f.round()
	assert.Equal(t, true, blowsOn(t, f, "p:1")[0]["blocked"])

	// THE CONTROL: torch-and-blade has no shield and turns nothing.
	g := newResolverFight(t, 1462)
	g.set(t, "forced_band", BandCrit)
	g.set(t, "player_action", PlayerActionHold)
	g.c.SetKits(fakeKits{"p:1": shippedKit(t, "torch-and-blade")})
	g.add(t, "e:1", 400, pack)
	g.open(t)
	g.round()

	for _, row := range blowsOn(t, g, "p:1") {
		assert.Equal(t, false, row["blocked"])
	}
}

func TestKitWeaponSetsHisBite(t *testing.T) {
	f := newResolverFight(t, 1462)
	f.set(t, "forced_band", BandHit)

	kit := shippedKit(t, "torch-and-blade")
	require.NoError(t, kit.Unequip(d2items.SlotMain, false, false))
	f.c.SetKits(fakeKits{"p:1": kit})

	// The knife is on his belt, not in his hand: with the sabre in the pack he
	// strikes bare -- no bite, so the placeholder stands.
	f.add(t, "e:1", 4000, Profile{})
	f.open(t)
	f.round()

	var theirs []map[string]interface{}
	for _, row := range f.actions(t) {
		if row["attacker"] == "p:1" {
			theirs = append(theirs, row)
		}
	}

	require.NotEmpty(t, theirs)
	assert.Equal(t, "", theirs[0]["weapon"], "an empty hand has no weapon")

	// Knife in hand: its 6-11, never the sabre's 12-20.
	g := newResolverFight(t, 1462)
	g.set(t, "forced_band", BandHit)

	k2 := shippedKit(t, "torch-and-blade")
	require.NoError(t, k2.Unequip(d2items.SlotMain, false, false))
	require.NoError(t, k2.Unequip(d2items.SlotBelt1, false, false))

	for i, inst := range k2.Pack {
		if inst.Item == "knife" {
			k2.Worn[d2items.SlotMain] = &k2.Pack[i]
		}
	}

	g.c.SetKits(fakeKits{"p:1": k2})
	g.add(t, "e:1", 4000, Profile{})
	g.open(t)

	struck := 0

	for i := 0; i < 8; i++ {
		g.round()

		for _, row := range g.actions(t) {
			if row["attacker"] != "p:1" {
				continue
			}

			struck++

			base := row["base"].(int)
			assert.Equal(t, "knife", row["weapon"])
			assert.True(t, base >= 6 && base <= 11, "a knife bites 6-11, got %d", base)
		}
	}

	require.NotZero(t, struck, "he must actually have struck, or the range was never checked")
}

func TestKitRiposteNeedsABladeAndATrueGraze(t *testing.T) {
	// A kılıç ripostes a graze -- the base case, pinned.
	riposted := func(kit *d2items.Kit, band string) (bool, bool) {
		f := newResolverFight(t, 1462)
		f.set(t, "forced_band", band)
		f.set(t, "player_action", PlayerActionHold)
		f.c.SetKits(fakeKits{"p:1": kit})
		f.add(t, "e:1", 4000, Profile{})
		f.open(t)
		f.round()

		answered, blocked := false, false

		for _, row := range f.actions(t) {
			if row["reaction"] == "riposte" {
				answered = true
			}

			if row["blocked"] == true {
				blocked = true
			}
		}

		return answered, blocked
	}

	answered, _ := riposted(shippedKit(t, "torch-and-blade"), BandGraze)
	assert.True(t, answered, "the sabre answers a graze")

	// A mace answers nothing (E3 §4's reaction column).
	mace := shippedKit(t, "torch-and-blade")
	macePack := len(mace.Pack)
	mace.Pack = append(mace.Pack, d2items.Instance{Item: "mace", Make: d2items.MakeIssue, Condition: d2items.Sound})
	require.NoError(t, mace.Equip(macePack, false, false))
	answered, _ = riposted(mace, BandGraze)
	assert.False(t, answered, "a mace gives no riposte")

	// A HIT the shield turned into a graze is not a graze to answer -- and the
	// block must actually have happened, or this proves nothing (review
	// finding: an unblocked hit never ripostes either).
	answered, blocked := riposted(shippedKit(t, "sword-and-board"), BandHit)
	require.True(t, blocked, "the shield turned the hit")
	assert.False(t, answered, "a blocked hit reads as a graze but was not a poor blow")
}
