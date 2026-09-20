package d2mapentity

import (
	"path/filepath"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2loader/asset/types"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
)

func TestCreatureLoadsTheShippedIdlePNG(t *testing.T) {
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
		5, 10, "Feral dog", "/data/strigoi/creatures/feral-dog/idle.png", 0, nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if width, height := creature.GetSize(); width != 64 || height != 54 {
		t.Fatalf("sprite size = %dx%d, want 64x54", width, height)
	}
	if got := creature.HarnessState()["animation_mode"]; got != "idle" {
		t.Fatalf("animation_mode = %v, want idle", got)
	}
}
