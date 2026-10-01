package d2world

import "math"

// LIGHT AS HE SEES IT THIS FRAME (fog of war F2, 1 Oct 2026; BUG-110).
//
// The light model's carried torch shines from where the player was last told
// to be (Light.SetPlayer), and SetPlayer runs only inside the game's gated
// advanceWorld. During a held player turn the world is stopped but his Move
// is a real walk (MapEngine.Advance is not gated), so the torch's light stayed
// where the turn opened while he walked out of it: BUG-110.
//
// A LightView is the same light with the carried sources shining from where
// he stands THIS frame. It is what fog reads ("lit" is "brighter than the
// sky", Level > quantise(Ambient)) and, with fog on, what the map renderer
// draws by -- so "he sees it" and "it is drawn lit" stay one fact.
//
// IT NEVER WRITES THE LIGHT MODEL: the sim (Notice, the combat resolver's
// lit/dark rule, spawns, the corpse check) keeps reading Light itself, as
// master does, so fog stays display only (plan §3.3). Fixing what the SIM
// reads during a held turn is a change to combat, and Josh's to rule.
type LightView struct {
	light *Light
	x, y  float64
	live  bool // false until SetCarriedAt: the light's own SetPlayer position

	// The sky as of the last SetCarriedAt: the game calls it once a frame,
	// after the clock has moved and before the frame is drawn, so the view's
	// levels are the light model's to the bit (one Ambient a frame instead of
	// one a tile: fog asks the level of every tile of every lit disc).
	ambient, skyBand float64
}

// NewLightView is a view of l with the carried lights at the light's own
// player position until SetCarriedAt moves them.
func NewLightView(l *Light) *LightView { return &LightView{light: l} }

// SetCarriedAt puts the carried lights where he stands now, for this view.
func (v *LightView) SetCarriedAt(x, y float64) {
	v.x, v.y, v.live = x, y, true
	v.ambient = v.light.Ambient()
	v.skyBand = v.light.quantise(clamp01(v.ambient))
}

// Light is the model the view reads.
func (v *LightView) Light() *Light { return v.light }

func (v *LightView) carried() (x, y float64) {
	if v.live {
		return v.x, v.y
	}

	return v.light.playerX, v.light.playerY
}

// Level is Light.Level with the carried lights where he stands now. It is a
// d2maprenderer.LightSampler.
func (v *LightView) Level(tileX, tileY int) float64 {
	if v.live {
		return v.light.levelOver(tileX, tileY, v.x, v.y, v.ambient)
	}

	return v.light.levelWith(tileX, tileY, v.light.playerX, v.light.playerY)
}

// Lit is whether a tile is brighter than the sky: its drawn level above the
// quantised ambient. Never a comparison with the raw ambient -- Level is
// quantised and never returns Ambient exactly, so "> Ambient()" marks half
// the night lit (plan §3.2).
func (v *LightView) Lit(tileX, tileY int) bool {
	if v.live {
		return v.Level(tileX, tileY) > v.skyBand
	}

	return v.Level(tileX, tileY) > v.light.quantise(clamp01(v.light.Ambient()))
}

// SkyBand is the sky as drawn: the quantised ambient a lit tile exceeds.
func (v *LightView) SkyBand() float64 {
	if v.live {
		return v.skyBand
	}

	return v.light.quantise(clamp01(v.light.Ambient()))
}

// SkyFraction is the sky from the night floor (0) to full day (1).
func (v *LightView) SkyFraction() float64 { return v.light.SkyFraction() }

// Moon is tonight's illuminated fraction, [0, 1].
func (v *LightView) Moon() float64 { return v.light.clock.Moon() }

// LitDisc is one lit source as fog needs it: where it shines from and how far.
type LitDisc struct {
	ID      int
	X, Y    float64
	Radius  float64
	Carried bool // follows him: fog keys it by its tile (B4)
}

// LitDiscs appends every lit source, carried ones where he stands now, in the
// light's order. Fog iterates these, never the whole map, for "lit ground is
// seen at any distance" (Josh's Q3).
func (v *LightView) LitDiscs(dst []LitDisc) []LitDisc {
	cx, cy := v.carried()

	for _, s := range v.light.sources {
		if !s.Lit || s.Radius <= 0 || math.IsNaN(s.Radius) {
			continue
		}

		x, y := s.X, s.Y
		if s.Carried {
			x, y = cx, cy
		}

		dst = append(dst, LitDisc{ID: s.ID, X: x, Y: y, Radius: s.Radius, Carried: s.Carried})
	}

	return dst
}
