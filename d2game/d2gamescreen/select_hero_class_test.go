package d2gamescreen

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
)

// The class pin, ruled 11 September 2026 and folded into M4.4c-2a on 17
// September 2026 (build brief clause 19, act 11, negative-control row 13).
// A new game offers exactly one class and it is the amazon, because the
// winnability run measured the night against her 240 health and the
// barbarian's 400 is a difficulty slider nobody gets to move.
//
// These run with no MPQ and no harness: getHeroRenderConfiguration is pure
// map-building over string constants, so gate.ps1's own `go test` covers it.

// TestOnlyTheAmazonIsOffered is clause 19's named assertion: the map the select
// screen loads its sprites from holds one entry and that entry is the amazon.
// Everything the screen does downstream -- loading, drawing, hovering,
// clicking -- ranges over this map, so one entry here is one hero on screen.
func TestOnlyTheAmazonIsOffered(t *testing.T) {
	configs := getHeroRenderConfiguration()

	require.Len(t, configs, 1, "a new game must offer exactly one class")
	require.Contains(t, configs, d2enum.HeroAmazon, "the one class must be the amazon")
	require.NotNil(t, configs[d2enum.HeroAmazon], "the amazon's config must be real, not a nil placeholder")
}

// TestThePinKeepsThePinnedClassOwnConfig guards against pinning by substitution:
// handing back a fresh, empty heroRenderConfig would pass a length check and
// then load no sprites at all. The surviving entry must be the roster's own.
func TestThePinKeepsThePinnedClassOwnConfig(t *testing.T) {
	roster := allHeroRenderConfigurations()

	pinned := pinRoster(roster, d2enum.HeroAmazon)

	require.Len(t, pinned, 1)
	require.Same(t, roster[d2enum.HeroAmazon], pinned[d2enum.HeroAmazon],
		"the pin must keep the class's own config, not a substitute")
}

// TestAnUnbuildablePinDoesNotFallBackToTheRoster pins the failure mode. If the
// pinned class ever loses its config, the tempting fallback is "then offer them
// all" -- which unpins the build silently and ships the barbarian's 400 health
// to everyone. An empty screen is the loud failure, and loud is what we want.
func TestAnUnbuildablePinDoesNotFallBackToTheRoster(t *testing.T) {
	roster := allHeroRenderConfigurations()
	delete(roster, d2enum.HeroAmazon)

	pinned := pinRoster(roster, d2enum.HeroAmazon)

	require.Empty(t, pinned, "a pin that cannot be built must offer nothing, not everything")
}

// TestTheRosterStillHoldsEveryClass separates "pinned" from "deleted". The six
// other classes stay in the tree as data so the pin is one constant to move,
// not six blocks to write again. If this goes red the art constants were
// removed, and moving the pin would no longer be enough to change the offer.
func TestTheRosterStillHoldsEveryClass(t *testing.T) {
	roster := allHeroRenderConfigurations()

	require.Len(t, roster, 7, "the D2 roster stays whole behind the pin")

	for _, hero := range []d2enum.Hero{
		d2enum.HeroBarbarian, d2enum.HeroSorceress, d2enum.HeroNecromancer,
		d2enum.HeroPaladin, d2enum.HeroAmazon, d2enum.HeroAssassin, d2enum.HeroDruid,
	} {
		require.NotNil(t, roster[hero], "roster is missing a class: %v", hero)
	}
}
