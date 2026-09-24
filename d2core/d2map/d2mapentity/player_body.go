package d2mapentity

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	slashpath "path"
	"strings"
	"sync"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
)

// playerBody is what a Player is drawn with: Diablo II's layered composite
// (the amazon, the barbarian ...) or, since M5.3, Strigoi's own PNG sprite
// sheets -- the hero Josh and GPT are making, drawn by the same path as the
// bestiary's creatures (M5.1). Player talks to either through this, and to
// nothing else of the composite's.
type playerBody interface {
	Advance(elapsed float64) error
	Render(target d2interface.Surface) error
	SetMode(mode d2enum.PlayerAnimationMode, weaponClass string) error
	GetAnimationMode() string
	GetWeaponClass() string
	GetDirection() int
	SetDirection(direction int)
	GetPlayedCount() int
	GetCurrentFrame() int
	GetFrameCount() int
	GetSize() (w, h int)
}

// compositeBody adapts the composite: its SetMode takes an unexported
// interface, which a PlayerAnimationMode satisfies.
type compositeBody struct{ *d2asset.Composite }

func (c compositeBody) SetMode(mode d2enum.PlayerAnimationMode, weaponClass string) error {
	return c.Composite.SetMode(mode, weaponClass)
}

var _ playerBody = compositeBody{}

// ---- the hero's art -------------------------------------------------------------------

// HeroArt is a PNG hero's sprite sheets, one per motion, each a PNG with the
// usual `.png.json` sheet manifest beside it (d2asset.PNGSheet: directions,
// frames, frame size, origin). Idle is required; every other falls back
// (run -> walk, block -> hit, dead -> death, anything missing -> idle), so a
// hero can arrive in the game with one sheet and gain the rest.
type HeroArt struct {
	Idle   string `json:"idle"`
	Walk   string `json:"walk,omitempty"`
	Run    string `json:"run,omitempty"`
	Attack string `json:"attack,omitempty"`
	Hit    string `json:"hit,omitempty"`
	Block  string `json:"block,omitempty"`
	Death  string `json:"death,omitempty"`
	Dead   string `json:"dead,omitempty"`
}

// heroManifest is the file a hero is chosen by:
// {"name": ..., "animations": HeroArt, "fps": {"walk": 12, ...}}.
//
// fps is optional, per motion (the HeroArt keys): how many frames a second
// that sheet plays. A motion without one plays its whole sheet in
// heroDefaultPlayLength, the engine's default for a loaded animation.
type heroManifest struct {
	Name       string             `json:"name"`
	Animations HeroArt            `json:"animations"`
	FPS        map[string]float64 `json:"fps,omitempty"`

	// Height is how tall the figure stands in its cell, in pixels: what the
	// overhead bar, the hover label and the pick box measure by. Without it
	// they use the whole cell, and a cell drawn with headroom floats the bar
	// above the head. Optional; at most the cell's height.
	Height int `json:"height,omitempty"`
}

// heroMotions are the HeroArt keys, in load order.
var heroMotions = []string{"idle", "walk", "run", "attack", "hit", "block", "death", "dead"}

// heroDefaultPlayLength is how long a sheet with no fps takes to play once, in
// seconds -- the engine's own default (d2asset's defaultPlayLength), set
// explicitly so a hero's timing is this file's decision.
const heroDefaultPlayLength = 1.0

// parseHeroManifest reads a hero manifest, refusing unknown keys so a typo
// ("atack") cannot silently fall back to idle, and resolves each sheet path
// against the manifest's own folder.
func parseHeroManifest(data []byte, manifestPath string) (heroManifest, error) {
	var m heroManifest

	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&m); err != nil {
		return heroManifest{}, fmt.Errorf("hero manifest %s: %w", manifestPath, err)
	}

	// One JSON value and nothing after it: a manifest with a second object
	// pasted below the first is a mistake, not a comment.
	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return heroManifest{}, fmt.Errorf("hero manifest %s has something after its closing brace", manifestPath)
	}

	for motion, fps := range m.FPS {
		known := false

		for _, k := range heroMotions {
			known = known || k == motion
		}

		if !known {
			return heroManifest{}, fmt.Errorf("hero manifest %s: fps for %q, which is not a motion (%s)",
				manifestPath, motion, strings.Join(heroMotions, ", "))
		}

		if fps <= 0 {
			return heroManifest{}, fmt.Errorf("hero manifest %s: fps for %s is %v; it must be above zero", manifestPath, motion, fps)
		}
	}

	if m.Animations.Idle == "" {
		return heroManifest{}, fmt.Errorf("hero manifest %s has no idle sheet", manifestPath)
	}

	dir := slashpath.Dir(manifestPath)
	for _, p := range []*string{&m.Animations.Idle, &m.Animations.Walk, &m.Animations.Run, &m.Animations.Attack,
		&m.Animations.Hit, &m.Animations.Block, &m.Animations.Death, &m.Animations.Dead} {
		*p = strings.ReplaceAll(*p, `\`, "/")

		if *p != "" && !strings.HasPrefix(*p, "/") {
			*p = slashpath.Join(dir, *p)
		}
	}

	return m, nil
}

// The hero art setting, process-wide for the same reason as d2mapgen's
// authored map: every player the game creates -- the server's and the
// client's -- must be drawn the same way, and a flag is set before either
// exists. "" is Diablo II's composite.
//
// nolint:gochecknoglobals // deliberate process-wide setting, see above
var heroArt struct {
	sync.Mutex
	path string
	used string
	err  error
}

// SetHeroArt makes every later player a PNG hero drawn from the manifest at
// p (game-relative, e.g. data/strigoi/hero/placeholder/hero.json); "" returns
// to Diablo II's composite. It forgets the last attempt's outcome.
func SetHeroArt(p string) {
	p = strings.TrimSpace(strings.ReplaceAll(p, `\`, "/"))
	if p != "" {
		// Rooted and cleaned: "a/../b" and "/a/./b" name one hero once, and
		// ".." cannot climb above the game's root.
		p = slashpath.Clean("/" + p)
	}

	heroArt.Lock()
	defer heroArt.Unlock()

	heroArt.path, heroArt.used, heroArt.err = p, "", nil
}

// HeroArtReport returns the hero art asked for, the art last used ("" if the
// composite was), and why the last attempt was refused.
func HeroArtReport() (asked, used string, err error) {
	heroArt.Lock()
	defer heroArt.Unlock()

	return heroArt.path, heroArt.used, heroArt.err
}

func heroArtPath() string {
	heroArt.Lock()
	defer heroArt.Unlock()

	return heroArt.path
}

func recordHeroArt(p string, err error) {
	heroArt.Lock()
	defer heroArt.Unlock()

	// A result for a setting since replaced says nothing about this one: a
	// player built from the old path, recorded after SetHeroArt changed it.
	if p != heroArt.path {
		return
	}

	// As with the authored map: the first refusal stands until SetHeroArt,
	// and "used" means every player since then got the art.
	if err != nil {
		if heroArt.err == nil {
			heroArt.err = err
		}

		heroArt.used = ""

		return
	}

	if heroArt.err == nil {
		heroArt.used = p
	}
}

// loadHeroBody builds a PNG hero body from the manifest at p.
func (f *MapEntityFactory) loadHeroBody(p, weaponClass string, direction int) (*pngBody, error) {
	data, err := f.asset.LoadFile(p)
	if err != nil {
		return nil, fmt.Errorf("loading hero manifest %s: %w", p, err)
	}

	m, err := parseHeroManifest(data, p)
	if err != nil {
		return nil, err
	}

	sheets := map[string]d2interface.Animation{}
	lengths := map[string]float64{}

	// In a fixed order: loads are observable (the asset census), and a
	// ranged map would load them in a different order every launch.
	for _, s := range []struct{ name, sheet string }{
		{"idle", m.Animations.Idle}, {"walk", m.Animations.Walk}, {"run", m.Animations.Run},
		{"attack", m.Animations.Attack}, {"hit", m.Animations.Hit}, {"block", m.Animations.Block},
		{"death", m.Animations.Death}, {"dead", m.Animations.Dead},
	} {
		name, sheet := s.name, s.sheet
		if sheet == "" {
			continue
		}

		anim, err := f.asset.LoadAnimation(sheet, "")
		if err != nil {
			return nil, fmt.Errorf("hero %s sheet %s: %w", name, sheet, err)
		}

		anim.PlayForward()
		sheets[name] = anim

		lengths[name] = heroDefaultPlayLength
		if fps := m.FPS[name]; fps > 0 {
			lengths[name] = float64(anim.GetFrameCount()) / fps
		}
	}

	body, err := newPNGBody(sheets, lengths, weaponClass, direction)
	if err != nil {
		return nil, err
	}

	if m.Height < 0 {
		return nil, fmt.Errorf("hero manifest %s: height %d is below zero", p, m.Height)
	}

	// Every sheet's cell, not only the one he stands in: GetSize reports
	// the height whatever sheet is playing.
	for _, s := range []struct{ name, sheet string }{
		{"idle", m.Animations.Idle}, {"walk", m.Animations.Walk}, {"run", m.Animations.Run},
		{"attack", m.Animations.Attack}, {"hit", m.Animations.Hit}, {"block", m.Animations.Block},
		{"death", m.Animations.Death}, {"dead", m.Animations.Dead},
	} {
		anim := sheets[s.name]
		if anim == nil {
			continue
		}

		if _, cellH := anim.GetCurrentFrameSize(); m.Height > cellH {
			return nil, fmt.Errorf("hero manifest %s: height %d is taller than the %s sheet's %d px cell",
				p, m.Height, s.name, cellH)
		}
	}

	body.height = m.Height

	return body, nil
}

// ---- the PNG body -------------------------------------------------------------------------

// pngBody draws a hero from PNG sheets keyed idle, walk, run, attack, hit,
// block, death, dead.
type pngBody struct {
	sheets      map[string]d2interface.Animation
	lengths     map[string]float64 // seconds to play each sheet once
	height      int                // the figure's height in its cell (0: the cell's)
	sheet       string             // which sheet is drawing
	anim        d2interface.Animation
	mode        string // the PlayerAnimationMode asked for, as the composite reports it
	weaponClass string
	direction   int
}

func newPNGBody(sheets map[string]d2interface.Animation, lengths map[string]float64,
	weaponClass string, direction int) (*pngBody, error) {
	if sheets["idle"] == nil {
		return nil, errors.New("a PNG hero needs an idle sheet")
	}

	b := &pngBody{sheets: sheets, lengths: lengths, weaponClass: weaponClass, direction: direction}

	if err := b.SetMode(d2enum.PlayerAnimationModeTownNeutral, weaponClass); err != nil {
		return nil, err
	}

	return b, nil
}

// sheetsFor is which sheets a mode draws from, best first; idle is always the
// last resort.
func sheetsFor(mode d2enum.PlayerAnimationMode) []string {
	switch mode {
	case d2enum.PlayerAnimationModeWalk, d2enum.PlayerAnimationModeTownWalk:
		return []string{"walk"}
	case d2enum.PlayerAnimationModeRun:
		return []string{"run", "walk"}
	case d2enum.PlayerAnimationModeAttack1, d2enum.PlayerAnimationModeAttack2, d2enum.PlayerAnimationModeKick,
		d2enum.PlayerAnimationModeThrow, d2enum.PlayerAnimationModeCast, d2enum.PlayerAnimationModeSkill1,
		d2enum.PlayerAnimationModeSkill2, d2enum.PlayerAnimationModeSkill3, d2enum.PlayerAnimationModeSkill4:
		return []string{"attack"}
	case d2enum.PlayerAnimationModeGetHit, d2enum.PlayerAnimationModeKnockBack, d2enum.PlayerAnimationModeSequence:
		// Sequence and KnockBack are both "GH" to the composite (their
		// String()), which draws its get-hit art for them.
		return []string{"hit"}
	case d2enum.PlayerAnimationModeBlock:
		return []string{"block", "hit"}
	case d2enum.PlayerAnimationModeDeath:
		return []string{"death"}
	case d2enum.PlayerAnimationModeDead:
		return []string{"dead", "death"}
	}

	return nil
}

// SetMode switches sheet. Like the composite it is a no-op for the mode and
// weapon class already set, so a caller may ask every tick.
func (b *pngBody) SetMode(mode d2enum.PlayerAnimationMode, weaponClass string) error {
	if b.anim != nil && b.mode == mode.String() && b.weaponClass == weaponClass {
		return nil
	}

	sheet := "idle"

	for _, name := range sheetsFor(mode) {
		if b.sheets[name] != nil {
			sheet = name
			break
		}
	}

	// A run drawn with the walk sheet steps faster, or his feet slide: the
	// same sheet, played in the time a walk would take to cover the ground a
	// run covers.
	length := b.lengths[sheet]
	if length <= 0 {
		length = heroDefaultPlayLength
	}

	if mode == d2enum.PlayerAnimationModeRun && sheet == "walk" {
		length *= baseWalkSpeed / baseRunSpeed
	}

	b.anim, b.sheet, b.mode, b.weaponClass = b.sheets[sheet], sheet, mode.String(), weaponClass
	b.anim.SetPlayLength(length)
	b.anim.Rewind()
	b.anim.ResetPlayedCount()

	return b.anim.SetDirection(b.direction)
}

func (b *pngBody) Advance(elapsed float64) error { return b.anim.Advance(elapsed) }

func (b *pngBody) Render(target d2interface.Surface) error {
	b.anim.RenderFromOrigin(target, false)
	return nil
}

func (b *pngBody) GetAnimationMode() string { return b.mode }

// Sheet is which of the hero's sheets is drawing (idle, walk, ...): what the
// mode resolved to, fallbacks included.
func (b *pngBody) Sheet() string          { return b.sheet }
func (b *pngBody) GetWeaponClass() string { return b.weaponClass }
func (b *pngBody) GetDirection() int      { return b.direction }
func (b *pngBody) GetPlayedCount() int    { return b.anim.GetPlayedCount() }
func (b *pngBody) GetCurrentFrame() int   { return b.anim.GetCurrentFrame() }
func (b *pngBody) GetFrameCount() int     { return b.anim.GetFrameCount() }

// GetSize is the frame's width and the figure's height (the manifest's
// height, when it gives one; else the frame's).
func (b *pngBody) GetSize() (w, h int) {
	w, h = b.anim.GetCurrentFrameSize()
	if b.height > 0 {
		h = b.height
	}

	return w, h
}

func (b *pngBody) SetDirection(direction int) {
	b.direction = direction
	_ = b.anim.SetDirection(direction)
}

var _ playerBody = (*pngBody)(nil)
