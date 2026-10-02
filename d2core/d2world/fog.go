package d2world

import (
	"image"
	"math"
	"strings"
)

// FOG OF WAR, AGE OF EMPIRES II STYLE -- F1, "black until explored" (by day).
//
// Josh, 1 Oct 2026: "Black until explored with line of sight that can be
// upgraded and extended." The plan is claude/fog-of-war-build-plan.md; this is
// its first burst (§4 F1) with Josh's answer to Q1 (§7): a squad sees 12 tiles
// by day, the range beasts notice him at.
//
// Fog holds, per map tile, whether the player has EVER seen it (explored, kept
// for the game) and whether he sees it NOW (visible, recomputed from his eyes).
// The renderer draws an unexplored tile not at all (the screen is cleared
// black), an explored-but-unseen tile greyed, and a visible one as it is.
//
// FOG IS DISPLAY ONLY. Nothing in the world reads it: not Notice, Combat, Seek
// or Pursuit. A beast that cannot be seen still sees him.
//
// F2, "the night closes it" (1 Oct 2026, Josh's Q2-Q5): by night an eye sees
// only its dark radius (1.5 tiles, rising to about 4 under a full moon) and
// what is LIT -- lit ground with a clear line is seen at any distance (Q3);
// dusk and dawn blend by the sky fraction; every one of his squads' models is
// an eye (Q5: villagers' eyes do not count); and the enemies of his own fight
// are shown for as long as it lasts (Q4: "contact" eyes, which reveal the tile
// they stand on and nothing else).
//
// F3, "kept" (1 Oct 2026): the explored grid is saved -- the world file's
// fog block, keyed on the map it was explored on (fog_snapshot.go). What he
// sees now is derived and is not.
//
// WHAT IT IS NOT YET (the plan's later bursts): raised sight (F4: talents,
// gear, height, towers), and default-on (F5).
//
// Resolution is the TILE (§6): authored maps block whole tiles, the renderer
// draws and lights per tile, entities are bucketed per tile. The village is
// 48x48: 2,304 bits.

// FogDials are fog's numbers. [DIAL]
type FogDials struct {
	// DaySight is how far an eye sees by day, in world tiles from the eye to
	// a tile's centre. Josh's Q1 (1 Oct 2026): 12, the notice radius.
	DaySight float64

	// DarkRadius is how far an eye sees in the dark with no light and no
	// moon; MoonDarkRadius is how far under a full moon. Tonight's dark
	// radius is lerp(DarkRadius, MoonDarkRadius, moon). Josh's Q2 (1 Oct
	// 2026): "1.5 tiles, rising to about 4 under a full moon"; 1.5 is the
	// light model's own FloorRadius (S1 §4: "the tile they stand on and
	// little else"). Both ends [DIAL].
	DarkRadius     float64
	MoonDarkRadius float64

	// MemoryLevel is the brightness an explored-but-unseen tile is drawn at
	// AT MOST: min(the tile's light, MemoryLevel), never a product, so a
	// remembered tile at night is not darkened twice. MemorySaturation is its
	// colour (0 grey .. 1 as it is). The look dials: Josh's eye sets them.
	MemoryLevel      float64
	MemorySaturation float64
}

// DefaultFogDials are F1's shipped dials.
func DefaultFogDials() FogDials {
	return FogDials{DaySight: 12, DarkRadius: 1.5, MoonDarkRadius: 4, MemoryLevel: 0.45, MemorySaturation: 0.25}
}

// TileSight is the line of sight fog needs: whether the line from a point to
// a tile's centre crosses no sight-blocking tile strictly between them, and
// how many tiles it read. *d2mapengine.MapEngine is one (TileSightClear); d2world
// never imports the map, as Notice never does.
type TileSight interface {
	TileSightClear(fx, fy float64, tx, ty int) (clear bool, cells int)
}

// Structured is optionally implemented by a TileSight that knows the map's
// structure footprints (whole tiles, Max exclusive): *d2mapengine.MapEngine
// does. Fog reads them when it sizes itself to a map.
type Structured interface {
	Structures() []image.Rectangle
}

// FogLight is the light fog needs (F2): the sky (0 night floor .. 1 day),
// tonight's moon, whether a tile is lit above the sky, and the lit sources to
// look for lit ground around. *LightView is one. A fog with no FogLight sees
// by day at every hour (F1's rule).
type FogLight interface {
	SkyFraction() float64
	Moon() float64
	// SkyBand is the sky as drawn: the quantised ambient that "lit" must
	// exceed. It is in the recompute key (the F2 review's C1): the sky
	// fraction's 1/64 steps and the ambient's 1/16 bands do not cross
	// together, so a band could change with the key unchanged.
	SkyBand() float64
	Lit(tileX, tileY int) bool
	LitDiscs(dst []LitDisc) []LitDisc
}

// Eye is one point the fog sees from, in world tiles: every model of his
// squads (F2; Q5: villagers' eyes do not count). A CONTACT eye is an enemy of
// his own fight (Q4): it reveals the tile it stands on and nothing else, for
// as long as the fight lasts, so the enemy is shown lit or not.
type Eye struct {
	ID      string
	X, Y    float64
	Contact bool
}

// FogTile is what fog says of one tile.
type FogTile int

// The three states of a tile.
const (
	FogUnexplored FogTile = iota
	FogExplored           // seen before, not seen now: drawn greyed, no one on it shown
	FogVisible            // seen now: drawn as it is
)

func (s FogTile) String() string {
	switch s {
	case FogVisible:
		return "visible"
	case FogExplored:
		return "explored"
	default:
		return "unexplored"
	}
}

// Fog is the explored and visible grids for one game. Not safe for concurrent
// use: the game goroutine updates and the renderer reads, on one goroutine.
type Fog struct {
	dials FogDials
	sight TileSight
	light FogLight // nil: day at every hour (F1)

	// The light the last recompute saw (lightKey) and what it decided: the
	// sky fraction quantised to fogSkySteps, and tonight's dark radius.
	key        lightKey
	discs      []LitDisc // scratch for the next key
	sky        float64
	darkRadius float64
	litTried   []uint32 // a lit tile whose lines were tried this epoch
	litSeen    int      // tiles made visible by the lit term alone, last recompute

	w, h     int
	explored []uint64 // bit per tile, row-major
	stamp    []uint32 // a tile is visible when its stamp is the epoch
	epoch    uint32

	exploredCount, visibleCount int

	eyes    []Eye
	eyeKeys []eyeKey // the eyes' tiles at the last recompute

	// structures are the map's footprints, clipped to the grid: a structure
	// with any tile seen is seen whole, and remembered whole (wholeStructures).
	structures []image.Rectangle
	dirty      bool // a dial, the grid or the explored set changed: recompute

	recomputes, skipped, cellsRead int
}

type eyeKey struct {
	id      string
	x, y    int
	contact bool
}

// fogSkySteps quantises the sky fraction fog keys on and reaches by: dusk's
// sky moves every frame, and a recompute per frame for a 1/1000 change in a
// radius would be the M4.3a shape. 1/64 of the way from 1.5 to 12 tiles is
// 0.16 tiles -- below a tile, which is fog's resolution.
const fogSkySteps = 64

// lightKey is everything in the light that decides what fog sees: the
// quantised sky, the moon, and every lit source where it shines from. A
// changed key recomputes.
//
// A CARRIED source is keyed by the TILE it shines from (the F2 review's B4):
// keyed exactly, a lit torch recomputed fog on every frame of a walk. Keyed by
// its tile it recomputes once a tile step -- when his own eye moves tile
// anyway -- and between steps the lit set is the one computed as he entered
// the tile (the drawn light still follows him exactly; the seen edge of his
// own torch can lag by under a tile until the next step). A fixed source is
// keyed exactly: it does not move.
type lightKey struct {
	sky   int
	moon  float64
	band  float64
	discs []LitDisc
}

func (k *lightKey) same(sky int, moon, band float64, discs []LitDisc) bool {
	if k.sky != sky || k.moon != moon || k.band != band || len(k.discs) != len(discs) {
		return false
	}

	for i := range discs {
		if keyOf(k.discs[i]) != keyOf(discs[i]) {
			return false
		}
	}

	return true
}

// keyOf is a lit disc as the key sees it: a carried one at its tile.
func keyOf(d LitDisc) LitDisc {
	if d.Carried {
		d.X, d.Y = float64(tileOf(d.X)), float64(tileOf(d.Y))
	}

	return d
}

// NewFog is an empty fog over no map; Update sizes it to the map it is given.
func NewFog(dials FogDials, sight TileSight) *Fog {
	return &Fog{dials: dials, sight: sight, dirty: true}
}

// Dials are fog's dials.
func (f *Fog) Dials() FogDials { return f.dials }

// SetLight gives fog the light it sees the night by (F2); nil is day at every
// hour. The next Update recomputes.
func (f *Fog) SetLight(l FogLight) {
	f.light = l
	f.dirty = true
}

// SetDials changes fog's dials; the next Update recomputes.
func (f *Fog) SetDials(d FogDials) {
	f.dials = d
	f.dirty = true
}

// Size is the grid's size in tiles (0, 0 before the first Update).
func (f *Fog) Size() (w, h int) { return f.w, f.h }

// resize makes a fresh, unexplored grid of w x h.
func (f *Fog) resize(w, h int) {
	f.w, f.h = w, h
	f.explored = make([]uint64, (w*h+63)/64)
	f.stamp = make([]uint32, w*h)
	f.litTried = make([]uint32, w*h)
	f.epoch = 1
	f.exploredCount, f.visibleCount = 0, 0
	f.dirty = true

	f.structures = f.structures[:0]

	if s, ok := f.sight.(Structured); ok {
		grid := image.Rect(0, 0, w, h)

		for _, r := range s.Structures() {
			if r = r.Intersect(grid); !r.Empty() {
				f.structures = append(f.structures, r)
			}
		}
	}
}

// Update recomputes what the eyes see on a w x h map when something that
// decides it changed -- an eye moved to another tile, an eye came or went, a
// dial, the explored set or the map's size -- and otherwise counts the call
// as skipped and does nothing. It says whether it recomputed.
//
// The key is the eyes' tiles and the light's signature (F2): the sky
// quantised to 1/64, the moon, and every lit source where it shines from --
// a torch carried as he walks moves its disc every frame, so a lit torch
// recomputes per frame of a walk (measured: BenchmarkFogRecompute's night
// rows). Tiles never change after generation (plan §1.4).
func (f *Fog) Update(w, h int, eyes []Eye) bool {
	if w != f.w || h != f.h {
		f.resize(w, h)
	}

	sky, moon, band := 1.0, 0.0, 1.0
	f.discs = f.discs[:0]

	if f.light != nil {
		sky, moon, band = clamp01(f.light.SkyFraction()), clamp01(f.light.Moon()), f.light.SkyBand()
		f.discs = f.light.LitDiscs(f.discs)
	}

	skyQ := int(math.Round(sky * fogSkySteps))

	if !f.dirty && f.sameEyes(eyes) && f.key.same(skyQ, moon, band, f.discs) {
		f.skipped++

		return false
	}

	f.key.sky, f.key.moon, f.key.band = skyQ, moon, band
	f.key.discs = append(f.key.discs[:0], f.discs...)
	f.sky = float64(skyQ) / fogSkySteps
	f.darkRadius = f.dials.DarkRadius + (f.dials.MoonDarkRadius-f.dials.DarkRadius)*moon

	f.eyes = f.eyes[:0]
	f.eyeKeys = f.eyeKeys[:0]

	// AN EYE SEES FROM THE CENTRE OF ITS TILE (the review's B1, 1 Oct 2026).
	// Fog is tile-resolution and skips every update while he stays on his
	// tile, so what he sees must be a function of his tile alone: computed
	// from his exact point, it was computed from wherever he ENTERED the tile
	// and kept until he left it -- two walks ending on one spot saw different
	// ground.
	for _, e := range eyes {
		x, y := tileOf(e.X), tileOf(e.Y)
		f.eyeKeys = append(f.eyeKeys, eyeKey{e.ID, x, y, e.Contact})
		f.eyes = append(f.eyes, Eye{ID: e.ID, X: float64(x) + 0.5, Y: float64(y) + 0.5, Contact: e.Contact})
	}

	f.recompute()
	f.dirty = false

	return true
}

func (f *Fog) sameEyes(eyes []Eye) bool {
	if len(eyes) != len(f.eyeKeys) {
		return false
	}

	for i, e := range eyes {
		if f.eyeKeys[i] != (eyeKey{e.ID, tileOf(e.X), tileOf(e.Y), e.Contact}) {
			return false
		}
	}

	return true
}

// recompute is the visible set from every eye, and explored |= visible.
func (f *Fog) recompute() {
	f.recomputes++

	f.epoch++
	if f.epoch == 0 { // wrapped: every stamp is stale again
		for i := range f.stamp {
			f.stamp[i] = 0
			f.litTried[i] = 0
		}

		f.epoch = 1
	}

	f.visibleCount = 0

	if f.w <= 0 || f.h <= 0 {
		return
	}

	reach := f.UnlitReach()

	for _, e := range f.eyes {
		if e.Contact {
			continue // shown last, below
		}

		// The eye's own tile, even an eye standing off the grid's last row.
		if ex, ey := tileOf(e.X), tileOf(e.Y); f.in(ex, ey) && !f.isVisible(ex, ey) {
			f.see(ex, ey)
		}

		r := reach
		x0, x1 := clampInt(tileOf(e.X-r), 0, f.w-1), clampInt(tileOf(e.X+r), 0, f.w-1)
		y0, y1 := clampInt(tileOf(e.Y-r), 0, f.h-1), clampInt(tileOf(e.Y+r), 0, f.h-1)

		for ty := y0; ty <= y1; ty++ {
			for tx := x0; tx <= x1; tx++ {
				if f.isVisible(tx, ty) {
					continue // another eye saw it already
				}

				if f.sees(e, tx, ty, r) {
					f.see(tx, ty)
				}
			}
		}
	}

	f.litSeen = 0
	if f.light != nil && f.sky < 1 {
		f.seeLitGround()
	}

	f.wholeStructures()

	// A CONTACT (an enemy of his fight, Q4) shows the tile it stands on and
	// nothing else (the F2 review's B1 and B2): after the whole-structure
	// reveal, so a wolf standing in a house does not show the house, and
	// SHOWN, not seen -- the tile is not explored, so when the fight ends
	// the ground he never saw is black again, not remembered.
	for _, e := range f.eyes {
		if ex, ey := tileOf(e.X), tileOf(e.Y); e.Contact && f.in(ex, ey) && !f.isVisible(ex, ey) {
			f.show(ex, ey)
		}
	}
}

// UnlitReach is how far an eye sees ground that is not lit: its day sight by
// day, tonight's dark radius at deep night, and between them by the sky
// fraction at dusk and dawn -- exactly as the light model's Radius blends
// (plan §2.3). Never past the day sight.
func (f *Fog) UnlitReach() float64 {
	dark := math.Min(f.darkRadius, f.dials.DaySight)

	return dark + (f.dials.DaySight-dark)*clamp01(f.sky)
}

// DarkRadius is tonight's dark radius (Q2): DarkRadius rising to
// MoonDarkRadius with the moon, as of the last recompute.
func (f *Fog) DarkRadius() float64 { return f.darkRadius }

// seeLitGround is the lit term (Josh's Q3, 1 Oct 2026: "lit ground with a
// clear line is seen at any distance"): every tile brighter than the sky that
// some squad eye has a clear line to is visible, however far. It iterates the
// LIT SOURCES' discs, never the whole map -- a tile outside every disc gets
// nothing from any source, so it cannot be lit above the sky -- and tries
// each lit tile's lines once per recompute.
func (f *Fog) seeLitGround() {
	for _, d := range f.key.discs {
		r := d.Radius
		x0, x1 := clampInt(tileOf(d.X-r), 0, f.w-1), clampInt(tileOf(d.X+r), 0, f.w-1)
		y0, y1 := clampInt(tileOf(d.Y-r), 0, f.h-1), clampInt(tileOf(d.Y+r), 0, f.h-1)

		for ty := y0; ty <= y1; ty++ {
			for tx := x0; tx <= x1; tx++ {
				i := ty*f.w + tx
				if f.stamp[i] == f.epoch || f.litTried[i] == f.epoch {
					continue
				}

				if math.Hypot(float64(tx)+0.5-d.X, float64(ty)+0.5-d.Y) >= r {
					continue // outside the disc: this source gives it nothing
				}

				f.litTried[i] = f.epoch

				if !f.light.Lit(tx, ty) {
					continue
				}

				for _, e := range f.eyes {
					if e.Contact {
						continue
					}

					if f.lineClear(e, tx, ty) {
						f.see(tx, ty)
						f.litSeen++

						break
					}
				}
			}
		}
	}
}

// LitAt is whether a tile is lit above the sky now (false with no light).
func (f *Fog) LitAt(tx, ty int) bool {
	return f.light != nil && f.light.Lit(tx, ty)
}

// LitSeen is how many tiles the last recompute saw by the lit term alone:
// lit ground past every eye's unlit reach.
func (f *Fog) LitSeen() int { return f.litSeen }

// wholeStructures is the plan's whole-structure reveal (§2.3; pulled into F1
// on 1 Oct 2026 so Josh's first look shows no house missing): a structure with
// ANY footprint tile visible is visible whole, and one with any tile explored
// is explored whole. A house's art stands on its front tiles (d2mapgen's
// strips), which its own footprint hides from an eye behind or beside it --
// without this the house seen from behind is not drawn at all, and one seen
// from the side is drawn in slices.
func (f *Fog) wholeStructures() { f.structuresWhole(true) }

// structuresWhole is wholeStructures; with reveal false only the explored
// half runs (the harness's explore verb, which sees nothing).
func (f *Fog) structuresWhole(reveal bool) {
	for _, r := range f.structures {
		anyVisible, anyExplored := false, false

		for ty := r.Min.Y; ty < r.Max.Y; ty++ {
			for tx := r.Min.X; tx < r.Max.X; tx++ {
				anyVisible = anyVisible || f.isVisible(tx, ty)
				anyExplored = anyExplored || f.isExplored(tx, ty)
			}
		}

		for ty := r.Min.Y; ty < r.Max.Y; ty++ {
			for tx := r.Min.X; tx < r.Max.X; tx++ {
				switch {
				case reveal && anyVisible && !f.isVisible(tx, ty):
					f.see(tx, ty)
				case anyExplored:
					f.explore(ty*f.w + tx)
				}
			}
		}
	}
}

// StructureAt is the footprint of the structure standing on tile (tx, ty), if
// one does.
func (f *Fog) StructureAt(tx, ty int) (image.Rectangle, bool) {
	p := image.Pt(tx, ty)

	for _, r := range f.structures {
		if p.In(r) {
			return r, true
		}
	}

	return image.Rectangle{}, false
}

// sees is THE VISIBILITY RULE's unlit half (plan §2.3; F2): tile T is
// visible to eye E (at the centre of its tile) when it is E's own tile, or
// when T's centre is within E's unlit reach r -- the day sight by day,
// tonight's dark radius by night, blended at dusk (UnlitReach) -- AND the line
// to it crosses no blocking tile strictly between (T itself may block: a wall
// is seen by its face). The other half, lit ground at any distance with a
// clear line, is seeLitGround.
func (f *Fog) sees(e Eye, tx, ty int, r float64) bool {
	if tx == tileOf(e.X) && ty == tileOf(e.Y) {
		return true
	}

	if math.Hypot(float64(tx)+0.5-e.X, float64(ty)+0.5-e.Y) > r {
		return false
	}

	return f.lineClear(e, tx, ty)
}

// lineClear is the map's line of sight from an eye to a tile, counted.
func (f *Fog) lineClear(e Eye, tx, ty int) bool {
	if f.sight == nil {
		return true
	}

	clear, cells := f.sight.TileSightClear(e.X, e.Y, tx, ty)
	f.cellsRead += cells

	return clear
}

func (f *Fog) see(tx, ty int) {
	f.show(tx, ty)
	f.explore(ty*f.w + tx)
}

// show makes a tile visible this epoch without exploring it: a contact's
// tile (Q4), drawn while the fight lasts and forgotten after it.
func (f *Fog) show(tx, ty int) {
	f.stamp[ty*f.w+tx] = f.epoch
	f.visibleCount++
}

func (f *Fog) explore(i int) bool {
	word, bit := i/64, uint64(1)<<(uint(i)%64)
	if f.explored[word]&bit != 0 {
		return false
	}

	f.explored[word] |= bit
	f.exploredCount++

	return true
}

func (f *Fog) in(tx, ty int) bool { return tx >= 0 && ty >= 0 && tx < f.w && ty < f.h }

func (f *Fog) isVisible(tx, ty int) bool { return f.stamp[ty*f.w+tx] == f.epoch }

func (f *Fog) isExplored(tx, ty int) bool {
	i := ty*f.w + tx

	return f.explored[i/64]&(uint64(1)<<(uint(i)%64)) != 0
}

// FogAt is what the renderer asks of a tile: has he seen it, and does he see
// it now. A tile off the grid is neither.
func (f *Fog) FogAt(tx, ty int) (explored, visible bool) {
	if !f.in(tx, ty) {
		return false, false
	}

	return f.isExplored(tx, ty), f.isVisible(tx, ty)
}

// At is a tile's state.
func (f *Fog) At(tx, ty int) FogTile {
	switch explored, visible := f.FogAt(tx, ty); {
	case visible:
		return FogVisible
	case explored:
		return FogExplored
	default:
		return FogUnexplored
	}
}

// MemoryLook is how a remembered tile is drawn: at most this brightness, at
// this saturation (the look dials).
func (f *Fog) MemoryLook() (level, saturation float64) {
	return f.dials.MemoryLevel, f.dials.MemorySaturation
}

// Explore marks every tile whose centre is within r of (x, y) explored, and
// says how many it newly marked. The harness's "explore" verb; nothing in the
// game calls it (an eye explores by seeing).
func (f *Fog) Explore(x, y, r float64) int {
	n := 0

	if f.w <= 0 || f.h <= 0 {
		return 0
	}

	for ty := clampInt(tileOf(y-r), 0, f.h-1); ty <= clampInt(tileOf(y+r), 0, f.h-1); ty++ {
		for tx := clampInt(tileOf(x-r), 0, f.w-1); tx <= clampInt(tileOf(x+r), 0, f.w-1); tx++ {
			if math.Hypot(float64(tx)+0.5-x, float64(ty)+0.5-y) <= r && f.explore(ty*f.w+tx) {
				n++
			}
		}
	}

	f.structuresWhole(false)

	return n
}

// Forget clears the explored set: every tile unexplored again until the next
// Update, which sees what the eyes see. The harness's "forget" verb; nothing
// in the game forgets.
func (f *Fog) Forget() {
	for i := range f.explored {
		f.explored[i] = 0
	}

	f.exploredCount = 0
	f.dirty = true
}

// RevealAll marks every tile explored. The harness's "reveal_all" verb.
func (f *Fog) RevealAll() {
	for i := 0; i < f.w*f.h; i++ {
		f.explore(i)
	}
}

// Counts are how many tiles are explored and how many visible.
func (f *Fog) Counts() (explored, visible int) { return f.exploredCount, f.visibleCount }

// Counters are fog's cost counters: recomputes, Updates skipped because
// nothing changed, and tiles the line-of-sight walks read.
func (f *Fog) Counters() (recomputes, skipped, cellsRead int) {
	return f.recomputes, f.skipped, f.cellsRead
}

// Eyes are the eyes of the last recompute, each at the centre of its tile.
func (f *Fog) Eyes() []Eye { return append([]Eye(nil), f.eyes...) }

// ClearFrom is whether the line from each of the last recompute's eyes to
// tile (tx, ty) is clear, keyed by eye id -- the probe's evidence of WHY a
// tile is or is not seen. It reads the map; it is not counted.
func (f *Fog) ClearFrom(tx, ty int) map[string]bool {
	out := make(map[string]bool, len(f.eyes))

	for _, e := range f.eyes {
		if e.Contact {
			continue // a contact sees its own tile only; no line is asked
		}

		clear := true
		if f.sight != nil {
			clear, _ = f.sight.TileSightClear(e.X, e.Y, tx, ty)
		}

		out[e.ID] = clear
	}

	return out
}

// Rows is the grid as text, one string a row: '0' unexplored, '1' explored,
// '2' visible.
func (f *Fog) Rows() []string {
	rows := make([]string, f.h)

	var b strings.Builder

	for ty := 0; ty < f.h; ty++ {
		b.Reset()

		for tx := 0; tx < f.w; tx++ {
			b.WriteByte(byte('0' + f.At(tx, ty)))
		}

		rows[ty] = b.String()
	}

	return rows
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}

	if v > hi {
		return hi
	}

	return v
}
