package d2world

import (
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
// WHAT F1 IS NOT (the plan's later bursts): the night (F2: sight shrinks to
// the dark radius and what is lit; here the sky fraction is pinned to 1, every
// hour is day), every squad as an eye (F2; here s:1, the player, alone), the
// save (F3: the explored grid is NOT saved -- a load starts black), raised
// sight (F4: talents, gear, height, towers), and default-on (F5).
//
// Resolution is the TILE (§6): authored maps block whole tiles, the renderer
// draws and lights per tile, entities are bucketed per tile. The village is
// 48x48: 2,304 bits.

// FogDials are fog's numbers. [DIAL]
type FogDials struct {
	// DaySight is how far an eye sees by day, in world tiles from the eye to
	// a tile's centre. Josh's Q1 (1 Oct 2026): 12, the notice radius.
	DaySight float64

	// MemoryLevel is the brightness an explored-but-unseen tile is drawn at
	// AT MOST: min(the tile's light, MemoryLevel), never a product, so a
	// remembered tile at night is not darkened twice. MemorySaturation is its
	// colour (0 grey .. 1 as it is). The look dials: Josh's eye sets them.
	MemoryLevel      float64
	MemorySaturation float64
}

// DefaultFogDials are F1's shipped dials.
func DefaultFogDials() FogDials {
	return FogDials{DaySight: 12, MemoryLevel: 0.45, MemorySaturation: 0.25}
}

// TileSight is the line of sight fog needs: whether the line from a point to
// a tile's centre crosses no sight-blocking tile strictly between them, and
// how many tiles it read. *d2mapengine.MapEngine is one (TileSightClear); d2world
// never imports the map, as Notice never does.
type TileSight interface {
	TileSightClear(fx, fy float64, tx, ty int) (clear bool, cells int)
}

// Eye is one point the fog sees from, in world tiles. F1 has one: the player.
type Eye struct {
	ID   string
	X, Y float64
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

	w, h     int
	explored []uint64 // bit per tile, row-major
	stamp    []uint32 // a tile is visible when its stamp is the epoch
	epoch    uint32

	exploredCount, visibleCount int

	eyes    []Eye
	eyeKeys []eyeKey // the eyes' tiles at the last recompute
	dirty   bool     // a dial, the grid or the explored set changed: recompute

	recomputes, skipped, cellsRead int
}

type eyeKey struct {
	id   string
	x, y int
}

// NewFog is an empty fog over no map; Update sizes it to the map it is given.
func NewFog(dials FogDials, sight TileSight) *Fog {
	return &Fog{dials: dials, sight: sight, dirty: true}
}

// Dials are fog's dials.
func (f *Fog) Dials() FogDials { return f.dials }

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
	f.epoch = 1
	f.exploredCount, f.visibleCount = 0, 0
	f.dirty = true
}

// Update recomputes what the eyes see on a w x h map when something that
// decides it changed -- an eye moved to another tile, an eye came or went, a
// dial, the explored set or the map's size -- and otherwise counts the call
// as skipped and does nothing. It says whether it recomputed.
//
// F1 needs no clock: by day nothing but the eyes changes what is seen (tiles
// never change after generation, plan §1.4). F2's night adds the light's
// signature to the key.
func (f *Fog) Update(w, h int, eyes []Eye) bool {
	if w != f.w || h != f.h {
		f.resize(w, h)
	}

	if !f.dirty && f.sameEyes(eyes) {
		f.skipped++

		return false
	}

	f.eyes = append(f.eyes[:0], eyes...)
	f.eyeKeys = f.eyeKeys[:0]

	for _, e := range eyes {
		f.eyeKeys = append(f.eyeKeys, eyeKey{e.ID, tileOf(e.X), tileOf(e.Y)})
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
		if f.eyeKeys[i] != (eyeKey{e.ID, tileOf(e.X), tileOf(e.Y)}) {
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
		}

		f.epoch = 1
	}

	f.visibleCount = 0

	if f.w <= 0 || f.h <= 0 {
		return
	}

	for _, e := range f.eyes {
		r := f.dials.DaySight
		x0, x1 := clampInt(tileOf(e.X-r), 0, f.w-1), clampInt(tileOf(e.X+r), 0, f.w-1)
		y0, y1 := clampInt(tileOf(e.Y-r), 0, f.h-1), clampInt(tileOf(e.Y+r), 0, f.h-1)

		for ty := y0; ty <= y1; ty++ {
			for tx := x0; tx <= x1; tx++ {
				if f.isVisible(tx, ty) {
					continue // another eye saw it already
				}

				if f.sees(e, tx, ty) {
					f.see(tx, ty)
				}
			}
		}

		// The eye's own tile, even an eye standing off the grid's last row.
		if ex, ey := tileOf(e.X), tileOf(e.Y); f.in(ex, ey) && !f.isVisible(ex, ey) {
			f.see(ex, ey)
		}
	}
}

// sees is THE VISIBILITY RULE (plan §2.3), F1's day form: tile T is visible
// to eye E when it is E's own tile, or when T's centre is within E's sight
// AND the line to it crosses no blocking tile strictly between (T itself may
// block: a wall is seen by its face). F2 adds "and within the night's reach,
// or lit"; by day the night's reach is the sight radius, so the term is true.
func (f *Fog) sees(e Eye, tx, ty int) bool {
	if tx == tileOf(e.X) && ty == tileOf(e.Y) {
		return true
	}

	if math.Hypot(float64(tx)+0.5-e.X, float64(ty)+0.5-e.Y) > f.dials.DaySight {
		return false
	}

	if f.sight == nil {
		return true
	}

	clear, cells := f.sight.TileSightClear(e.X, e.Y, tx, ty)
	f.cellsRead += cells

	return clear
}

func (f *Fog) see(tx, ty int) {
	i := ty*f.w + tx
	f.stamp[i] = f.epoch
	f.visibleCount++
	f.explore(i)
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

// Eyes are the eyes of the last recompute.
func (f *Fog) Eyes() []Eye { return append([]Eye(nil), f.eyes...) }

// ClearFrom is whether the line from each of the last recompute's eyes to
// tile (tx, ty) is clear, keyed by eye id -- the probe's evidence of WHY a
// tile is or is not seen. It reads the map; it is not counted.
func (f *Fog) ClearFrom(tx, ty int) map[string]bool {
	out := make(map[string]bool, len(f.eyes))

	for _, e := range f.eyes {
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
