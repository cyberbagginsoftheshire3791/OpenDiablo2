package d2mapentity

import (
	"path/filepath"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2loader/asset/types"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
)

func TestCreatureLoadsAndPlaysShippedAnimationSet(t *testing.T) {
	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		t.Fatal(err)
	}

	repoRoot := filepath.Join("..", "..", "..")
	if err := asset.AddSource(repoRoot, types.AssetSourceFileSystem); err != nil {
		t.Fatal(err)
	}

	factory := &MapEntityFactory{asset: asset}
	creature, err := factory.NewCreature(
		5, 10, "Feral dog", CreatureAnimationPaths{
			Idle:   "/data/strigoi/creatures/feral-dog/idle.png",
			Walk:   "/data/strigoi/creatures/feral-dog/walk.png",
			Attack: "/data/strigoi/creatures/feral-dog/attack.png",
			Hit:    "/data/strigoi/creatures/feral-dog/hit.png",
			Death:  "/data/strigoi/creatures/feral-dog/death.png",
			Dead:   "/data/strigoi/creatures/feral-dog/dead.png",
		}, 0, nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if width, height := creature.GetSize(); width != 96 || height != 96 {
		t.Fatalf("sprite size = %dx%d, want 96x96", width, height)
	}
	if got := creature.HarnessState()["animation_mode"]; got != "idle" {
		t.Fatalf("animation_mode = %v, want idle", got)
	}
	if got := creature.HarnessState()["sheet"]; got != "/data/strigoi/creatures/feral-dog/idle.png" {
		t.Fatalf("sheet = %v, want the idle sheet", got)
	}
	if len(creature.animations) != 6 {
		t.Fatalf("loaded %d animation modes, want 6", len(creature.animations))
	}

	attackFinished := false
	if err := creature.StartAction(d2enum.MonsterAnimationModeAttack1, func() {
		attackFinished = true
	}); err != nil {
		t.Fatal(err)
	}
	if got := creature.HarnessState()["animation_mode"]; got != "attack" {
		t.Fatalf("animation_mode = %v during attack, want attack", got)
	}
	if got := creature.HarnessState()["sheet"]; got != "/data/strigoi/creatures/feral-dog/attack.png" {
		t.Fatalf("sheet = %v during attack, want the attack sheet", got)
	}
	creature.Advance(1.1)
	if !attackFinished {
		t.Fatal("attack animation did not finish after one complete play")
	}
	if got := creature.HarnessState()["animation_mode"]; got != "idle" {
		t.Fatalf("animation_mode = %v after attack, want idle", got)
	}

	deathFinished := false
	if err := creature.StartAction(d2enum.MonsterAnimationModeDeath, func() {
		deathFinished = true
	}); err != nil {
		t.Fatal(err)
	}
	if got := creature.HarnessState()["animation_mode"]; got != "death" {
		t.Fatalf("animation_mode = %v during death, want death", got)
	}
	creature.Advance(1.1)
	if !deathFinished || !creature.corpse {
		t.Fatalf("death completion = %v, corpse = %v; want both true", deathFinished, creature.corpse)
	}
	if got := creature.HarnessState()["animation_mode"]; got != "dead" {
		t.Fatalf("animation_mode = %v after death, want dead", got)
	}
	if got := creature.HarnessState()["sheet"]; got != "/data/strigoi/creatures/feral-dog/dead.png" {
		t.Fatalf("sheet = %v after death, want the dead sheet", got)
	}
}

// A mode with no sheet of its own is drawn from idle, and says so: the harness
// reports the sheet actually on screen, not the one the mode would have had.
// And it reports the speed the creature walks at.
func TestCreatureReportsTheSheetItIsDrawnFrom(t *testing.T) {
	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		t.Fatal(err)
	}

	if err := asset.AddSource(filepath.Join("..", "..", ".."), types.AssetSourceFileSystem); err != nil {
		t.Fatal(err)
	}

	const idle = "/data/strigoi/creatures/strigoi/idle.png"

	creature, err := (&MapEntityFactory{asset: asset}).NewCreature(5, 10, "Idle only",
		CreatureAnimationPaths{Idle: idle}, 0, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := creature.StartAction(d2enum.MonsterAnimationModeAttack1, nil); err != nil {
		t.Fatal(err)
	}

	if got := creature.HarnessState()["sheet"]; got != idle {
		t.Fatalf("an attack with no attack sheet is drawn from %v, want idle's %s", got, idle)
	}

	creature.SetSpeed(7)
	if got := creature.HarnessState()["speed"]; got != 7.0 {
		t.Fatalf("speed = %v, want 7", got)
	}
}
