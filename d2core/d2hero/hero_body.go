package d2hero

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
)

// THE HERO'S BODY IS STRIGOI'S DATA (M5.3's tables burst, 26 Sep 2026).
// Diablo II's charstats.txt gave the hero his health, his stamina and how
// fast running spends it: the amazon's row WAS the Janissary's body, and the
// 240 health Josh pinned on 11 Sep 2026 -- the buffer the winnability run
// measured -- was that row's InitVit x LifePerVit. In Strigoi's game those
// three are the hero manifest's (hero.json: max_health, max_stamina,
// stamina_run_drain), so the pin is data, where a pin belongs. -classic still
// reads charstats.txt.

// BodyStats is what a hero manifest says about the body it draws. Each field
// is optional; one left out (0) is defaultBody's.
type BodyStats struct {
	MaxHealth       int `json:"max_health,omitempty"`
	MaxStamina      int `json:"max_stamina,omitempty"`
	StaminaRunDrain int `json:"stamina_run_drain,omitempty"`
}

// defaultBody is the body a manifest that says nothing gets: exactly what
// charstats.txt gave the amazon, measured from the records at boot at
// a690dfc4 (InitVit 20 x LifePerVit 12 = 240, InitStamina 84, RunDrain 20),
// so a hero drawn from a manifest without the three fields plays exactly as
// he did when the table was read.
//
// nolint:gochecknoglobals // a fixed value, read like a constant
var defaultBody = BodyStats{MaxHealth: 240, MaxStamina: 84, StaminaRunDrain: 20}

// Check refuses a body no manifest can mean: a negative health, stamina or
// drain. Zero is "not given".
func (b BodyStats) Check() error {
	switch {
	case b.MaxHealth < 0:
		return fmt.Errorf("max_health %d is below zero", b.MaxHealth)
	case b.MaxStamina < 0:
		return fmt.Errorf("max_stamina %d is below zero", b.MaxStamina)
	case b.StaminaRunDrain < 0:
		return fmt.Errorf("stamina_run_drain %d is below zero", b.StaminaRunDrain)
	}

	return nil
}

// withDefaults fills each field the manifest left out from defaultBody.
func (b BodyStats) withDefaults() BodyStats {
	if b.MaxHealth == 0 {
		b.MaxHealth = defaultBody.MaxHealth
	}

	if b.MaxStamina == 0 {
		b.MaxStamina = defaultBody.MaxStamina
	}

	if b.StaminaRunDrain == 0 {
		b.StaminaRunDrain = defaultBody.StaminaRunDrain
	}

	return b
}

// The hero manifest the body is read from: set by d2mapentity.SetHeroArt,
// the one setting that says which hero this game is played with, so the body
// and the sheets always come from the same file. "" is no manifest (Diablo
// II's composite hero), which in Strigoi's game has defaultBody.
//
// nolint:gochecknoglobals // deliberate process-wide setting, like the hero art
var heroManifest struct {
	sync.Mutex
	path string
}

// SetHeroManifest names the hero manifest the body is read from (a
// game-relative path, as d2mapentity.SetHeroArt cleans it).
func SetHeroManifest(p string) {
	heroManifest.Lock()
	defer heroManifest.Unlock()

	heroManifest.path = p
}

func heroManifestPath() string {
	heroManifest.Lock()
	defer heroManifest.Unlock()

	return heroManifest.path
}

// Body is the hero's body in Strigoi's game: the three fields of the hero
// manifest, each one left out taken from defaultBody. A manifest that cannot
// be read, or whose three are not numbers or are negative, gives defaultBody
// whole -- and d2mapentity's strict read of the same file refuses it too,
// draws the composite instead and says why (HeroArtReport), so the mistake is
// not silent.
//
// THE TWO READS DO NOT ALWAYS AGREE. The strict read also refuses a manifest
// for a key it does not know, a second value after the first, or a bad fps
// (d2mapentity.parseHeroManifest); Body reads only its three keys and ignores
// the rest. So a manifest refused for a typo draws the composite hero while
// its max_health, max_stamina and stamina_run_drain STILL apply -- the body is
// the manifest's even when the sheets are not. (Corrected by the tables
// burst's review, 27 Sep 2026: this comment said such a manifest gave
// defaultBody. TestBodyIsTheManifestsWithTheAmazonsDefaults' "unknown key"
// case pins it.)
func Body(asset *d2asset.AssetManager) BodyStats {
	var b BodyStats

	if p := heroManifestPath(); p != "" {
		if data, err := readManifest(asset, p); err == nil {
			if json.Unmarshal(data, &b) != nil || b.Check() != nil {
				b = BodyStats{}
			}
		}
	}

	return b.withDefaults()
}

// readManifest reads the manifest and closes it: Body is read for every new
// hero and every player built, and AssetManager.LoadFile leaves its file open.
func readManifest(asset *d2asset.AssetManager, p string) ([]byte, error) {
	stream, err := asset.LoadFileStream(p)
	if err != nil {
		return nil, err
	}

	if c, ok := stream.(io.Closer); ok {
		defer func() { _ = c.Close() }()
	}

	return io.ReadAll(stream)
}

// StaminaRunDrain is how fast running spends the hero's stamina: the hero
// manifest's (Body) in Strigoi's game, charstats.txt's under -classic.
func StaminaRunDrain(asset *d2asset.AssetManager, hero d2enum.Hero) float64 {
	if !asset.Classic() {
		return float64(Body(asset).StaminaRunDrain)
	}

	if cs := asset.Records.Character.Stats[hero]; cs != nil {
		return float64(cs.StaminaRunDrain)
	}

	return 0
}
