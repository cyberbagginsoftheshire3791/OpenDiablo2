package d2gamescreen

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2hero"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
)

// TestShouldSaveOnUnloadSkipsADeadHero pins item 2's save half (12 Sep 2026):
// OnUnload must not persist a hero at or below zero health -- a saved 0-HP hero
// loads as an un-killable, un-feedable corpse on every launch of that .od2
// (audit A2). d2gamescreen links no ebiten, so this runs on CI. The load-path
// reset that revives one is pinned in d2hero (TestReviveIfDead).
//
// Negative control: change shouldSaveOnUnload to `return true` and the dead-hero
// case fails.
func TestShouldSaveOnUnloadSkipsADeadHero(t *testing.T) {
	alive := &d2mapentity.Player{Stats: &d2hero.HeroStatsState{Health: 40, MaxHealth: 240}}
	require.True(t, shouldSaveOnUnload(alive), "a living hero is saved")

	dead := &d2mapentity.Player{Stats: &d2hero.HeroStatsState{Health: 0, MaxHealth: 240}}
	require.False(t, shouldSaveOnUnload(dead), "a dead hero is never written")

	require.True(t, shouldSaveOnUnload(nil), "a nil player keeps the always-save behaviour")
	require.True(t, shouldSaveOnUnload(&d2mapentity.Player{}), "nil stats cannot be judged dead")
}
