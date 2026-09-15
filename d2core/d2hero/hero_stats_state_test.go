package d2hero

import "testing"

// TestStatsIsDead pins item 2's predicate (12 Sep 2026): a saved hero at or
// below zero health is dead, nil is not. d2hero links no ebiten, so this runs on
// CI.
func TestStatsIsDead(t *testing.T) {
	cases := []struct {
		name  string
		stats *HeroStatsState
		want  bool
	}{
		{"nil is not dead", nil, false},
		{"zero health is dead", &HeroStatsState{Health: 0, MaxHealth: 240}, true},
		{"negative health is dead", &HeroStatsState{Health: -5, MaxHealth: 240}, true},
		{"positive health is alive", &HeroStatsState{Health: 1, MaxHealth: 240}, false},
	}

	for _, c := range cases {
		if got := c.stats.IsDead(); got != c.want {
			t.Errorf("%s: IsDead() = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestReviveIfDead pins the load reset: a state loaded at 0 reports MaxHealth, a
// living one is left alone, and nil is safe (an old save with no stats block).
//
// Negative control: remove the reset in reviveIfDead and the 0-HP case stays 0.
func TestReviveIfDead(t *testing.T) {
	dead := &HeroStatsState{Health: 0, MaxHealth: 240}
	reviveIfDead(dead)

	if dead.Health != 240 {
		t.Fatalf("a loaded death must revive to MaxHealth; got %d of %d", dead.Health, dead.MaxHealth)
	}

	alive := &HeroStatsState{Health: 40, MaxHealth: 240}
	reviveIfDead(alive)

	if alive.Health != 40 {
		t.Fatalf("a living hero's health must be left alone; got %d", alive.Health)
	}

	// nil-safe.
	reviveIfDead(nil)
}
