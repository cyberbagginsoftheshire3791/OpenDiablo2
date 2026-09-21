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
}
