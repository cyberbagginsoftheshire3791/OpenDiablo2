package d2mapentity

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2hero"
)

func actionPlayer(t *testing.T) *Player {
	t.Helper()
	return &Player{mapEntity: newMapEntity(5, 5), composite: placeholderHero(t),
		Stats: &d2hero.HeroStatsState{Health: 100}, isInTown: true}
}

func advanceAction(p *Player, frames int) {
	for i := 0; i < frames; i++ {
		p.Advance(1.0 / 60)
	}
}

func TestPlayerCombatActionsSurviveTownAndRestart(t *testing.T) {
	p := actionPlayer(t)
	for _, mode := range []d2enum.PlayerAnimationMode{
		d2enum.PlayerAnimationModeAttack1, d2enum.PlayerAnimationModeGetHit, d2enum.PlayerAnimationModeBlock,
	} {
		if err := p.StartAction(mode); err != nil {
			t.Fatal(err)
		}
		advanceAction(p, 5)
		if p.GetAnimationMode() != mode || p.composite.GetAnimationMode() != mode.String() || p.IsCasting() {
			t.Fatalf("%s must remain a visual action in town, without casting locks", mode)
		}
		if err := p.StartAction(mode); err != nil {
			t.Fatal(err)
		}
		if p.composite.GetCurrentFrame() != 0 || p.composite.GetPlayedCount() != 0 {
			t.Fatal("a second identical action must start at its first frame")
		}
		advanceAction(p, 100)
		if p.GetAnimationMode() != d2enum.PlayerAnimationModeTownNeutral {
			t.Fatalf("%s did not return to town idle", mode)
		}
	}
}

func TestPlayerReactionPreservesOneSkillCallback(t *testing.T) {
	p := actionPlayer(t)
	casts := 0
	p.StartCasting(d2enum.PlayerAnimationModeAttack2, func() { casts++ })
	advanceAction(p, 5)
	if p.GetAnimationMode() != d2enum.PlayerAnimationModeAttack2 {
		t.Fatal("a requested skill mode must not turn into SC or town idle")
	}
	if err := p.StartAction(d2enum.PlayerAnimationModeGetHit); err != nil {
		t.Fatal(err)
	}
	advanceAction(p, 20)
	if casts != 0 || !p.IsCasting() {
		t.Fatal("a reaction must neither invoke nor discard the pending skill")
	}
	advanceAction(p, 180)
	if casts != 1 || p.IsCasting() {
		t.Fatalf("skill callback ran %d times; casting=%v, want once and finished", casts, p.IsCasting())
	}
}

func TestPlayerDeathHaltsBeforeMovementAndHoldsCorpse(t *testing.T) {
	p := actionPlayer(t)
	p.mapEntity = movingEntity()
	casts := 0
	p.StartCasting(d2enum.PlayerAnimationModeAttack1, func() { casts++ })
	x, y := p.Position.X(), p.Position.Y()
	p.Stats.Health = 0 // neglect uses the same Advance path as a resolver death
	p.Advance(1.0 / 60)
	if p.GetAnimationMode() != d2enum.PlayerAnimationModeDeath || p.Position.X() != x || p.Position.Y() != y {
		t.Fatal("death must start before any movement")
	}
	advanceAction(p, 180)
	if p.GetAnimationMode() != d2enum.PlayerAnimationModeDead || casts != 0 || p.IsCasting() {
		t.Fatal("the completed fall must remain dead and cancel its skill callback")
	}
	for _, mode := range []d2enum.PlayerAnimationMode{d2enum.PlayerAnimationModeAttack1, d2enum.PlayerAnimationModeGetHit, d2enum.PlayerAnimationModeDeath} {
		_ = p.StartAction(mode)
		advanceAction(p, 3)
		if p.GetAnimationMode() != d2enum.PlayerAnimationModeDead {
			t.Fatal("later actions must not restart or revive a corpse")
		}
	}
	p.Stats.Health = 100
	p.StandAt(12, 18, 24)
	p.Advance(1.0 / 60)
	if p.GetAnimationMode() != d2enum.PlayerAnimationModeTownNeutral || p.actionHeld || p.corpse || p.onFinishedCasting != nil {
		t.Fatal("load reconstruction must normalize transient visual state")
	}
}

func TestPlayerSameModeQueuedCastStillFiresOnce(t *testing.T) {
	p := actionPlayer(t)
	casts := 0
	if err := p.StartAction(d2enum.PlayerAnimationModeAttack1); err != nil {
		t.Fatal(err)
	}
	p.StartCasting(d2enum.PlayerAnimationModeAttack1, func() { casts++ })
	advanceAction(p, 30)
	if casts != 0 {
		t.Fatal("queued skill fired during the visual action")
	}
	advanceAction(p, 180)
	if casts != 1 || p.IsCasting() {
		t.Fatalf("same-mode queued cast fired %d times; casting=%v", casts, p.IsCasting())
	}
}

func TestPlayerRepeatedSwingRestartsRequestedMode(t *testing.T) {
	p := actionPlayer(t)
	p.StartCasting(d2enum.PlayerAnimationModeAttack1, nil)
	advanceAction(p, 10)
	if !p.IsCasting() || p.composite.GetCurrentFrame() == 0 {
		t.Fatal("control: the first swing must be playing")
	}
	p.StartCasting(d2enum.PlayerAnimationModeAttack1, nil)
	if p.composite.GetCurrentFrame() != 0 || p.composite.GetPlayedCount() != 0 {
		t.Fatal("a repeated swing must restart, not inherit the first swing's progress")
	}
	advanceAction(p, 5)
	if !p.IsCasting() || p.GetAnimationMode() != d2enum.PlayerAnimationModeAttack1 {
		t.Fatal("the swing must retain its existing casting lock and requested A1 pose")
	}
	// Stop on the completion tick, before the next Advance changes the body
	// to town idle. Its A1 play count is nonzero: a second request must not
	// immediately finish by inheriting that count.
	for frame := 0; frame < 180 && p.IsCasting(); frame++ {
		p.Advance(1.0 / 60)
	}
	if p.IsCasting() || p.composite.GetAnimationMode() != d2enum.PlayerAnimationModeAttack1.String() || p.composite.GetPlayedCount() < 1 {
		t.Fatal("control: first cast must have completed while the body is still A1")
	}
	casts := 0
	p.StartCasting(d2enum.PlayerAnimationModeAttack1, func() { casts++ })
	if p.composite.GetCurrentFrame() != 0 || p.composite.GetPlayedCount() != 0 {
		t.Fatal("a swing after completion must discard the completed A1 play count")
	}
	p.Advance(1.0 / 60)
	if !p.IsCasting() || casts != 0 {
		t.Fatal("the new swing must hold its lock and not fire its callback on its first tick")
	}
	advanceAction(p, 180)
	if casts != 1 || p.IsCasting() {
		t.Fatalf("post-completion swing fired %d times; casting=%v, want once and finished", casts, p.IsCasting())
	}
}
