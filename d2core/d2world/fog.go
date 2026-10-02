package d2world

import (
	"fmt"
	"image"
	"math"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2geom"
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
// F4, "raised sight" (1 Oct 2026; Josh's ruling 4: "all of them" -- talents,
// structures, gear / squad type and height raise sight; the plan's Q6-Q9
// decided on their recommended defaults while Josh was away, his to
// overturn): an eye's sight is its base (the day sight; a tower's own
// sight_radius) + its gear (Q9: an equipped composite bow, +2) + HeightTiles
// a level of the ground it stands on; its dark radius is tonight's + its
// talent (Q8: Night Eyes, +1). A TOWER (a structure with a sight_radius) is
// an eye of its own with no garrison (Q6): by day it sees its radius, by
// night, like a squad, its dark radius and what is lit -- so a beacon lit on
// it lets it see by night. Height adds radius only: v1 does not see over
// blockers. Squad TYPES add theirs when squads get a type (not built: nothing
// would set it).
//
// THE COST (F4): every eye's lines of sight are cached per tile it stands on
// (eyeLines) -- tiles never change after generation, and an eye sees from the
// centre of its tile -- so a recompute for the sky, the moon, a lit source or
// a dial reads the cache, a standing tower walks its lines once a game, and
// only an eye that changed tile walks again.
//
// WHAT IT IS NOT YET (the plan's last burst): default-on (F5).
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

	// HeightTiles is how much further an eye sees per level of the ground it
	// stands on (F4; the plan's §2.2: 2). [DIAL]
	HeightTiles float64

	// MemoryLevel is the brightness an explored-but-unseen tile is drawn at
	// AT MOST: min(the tile's light, MemoryLevel), never a product, so a
	// remembered tile at night is not darkened twice. MemorySaturation is its
	// colour (0 grey .. 1 as it is). The look dials: Josh's eye sets them.
	MemoryLevel      float64
	MemorySaturation float64
}

// DefaultFogDials are F1's shipped dials.
func DefaultFogDials() FogDials {
	return FogDials{DaySight: 12, DarkRadius: 1.5, MoonDarkRadius: 4, HeightTiles: 2, MemoryLevel: 0.45, MemorySaturation: 0.25}
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

// OpaqueTiles is optionally implemented by a TileSight whose TileSightClear
// is d2geom.TileLineClear over these answers (F4): *d2mapengine.MapEngine is.
// Fog then reads every tile's answer once a map into a grid of its own and
// walks that grid with the same TileLineClear -- the same line, a quarter of
// the cost (no subtile lookup per cell).
type OpaqueTiles interface {
	TileBlocksSight(tx, ty int) bool
}

// Heights is optionally implemented by a TileSight that knows the ground's
// height (F4): *d2mapengine.MapEngine does. An eye on ground of height h sees
// HeightTiles * h further.
type Heights interface {
	HeightAt(tx, ty int) int
}

// Towered is optionally implemented by a TileSight that knows the map's towers
// (F4): their footprints and sight radii, one for one. *d2mapengine.MapEngine
// does. Fog reads them when it sizes itself to a map, and every one is an eye.
type Towered interface {
	TowerSights() (footprints []image.Rectangle, sight []float64)
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
	// LitCarriedAt is Lit with the carried sources shining from (cx, cy)
	// rather than from where he stands this frame. Fog asks it with his torch
	// at the CENTRE OF HIS TILE (the F3 review's A1), so what his own torch
	// shows him is a function of his tile alone, as his eye is.
	LitCarriedAt(tileX, tileY int, cx, cy float64) bool
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

	// F4, raised sight. The zero values are an F3 eye.
	//
	// Base is the eye's sight before its gear and its ground: 0 is the dials'
	// DaySight (a squad), a tower's sight_radius otherwise.
	Base float64
	// Gear is the sight its gear gives (Q9: his equipped composite bow, +2).
	// Squad types will add theirs here when squads have a type.
	Gear float64
	// DarkTalent widens its dark radius (Q8: Night Eyes, +1).
	DarkTalent float64
	// Tower is a structure's eye (Q6): fog makes them from the map (Towered).
	Tower bool
}

// EyeSight is one eye of the last recompute with what it saw by (F4): its
// sight and the terms it is made of, its dark radius, its unlit reach, and the
// height of the ground it stands on.
type EyeSight struct {
	Eye

	Sight      float64 // BaseTerm + Gear + HeightTerm
	BaseTerm   float64 // the day sight, or a tower's own
	HeightTerm float64 // HeightTiles * Height
	Height     int     // the ground's height under it, in levels
	Dark       float64 // tonight's dark radius + DarkTalent
	UnlitReach float64 // lerp(Dark, Sight, sky)
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

	// F4: what each eye of the last recompute saw by (one for one with
	// eyes), the map's towers and ground, and every eye's cached lines.
	sights   []EyeSight
	towers   []Eye
	heights  Heights
	lines    map[string]*eyeLines
	eyeLine  []*eyeLines // one for one with eyes, this recompute
	linesHit int

	// opaque is the map's sight-blocking tiles, read once a map from an
	// OpaqueTiles sight (nil: walk the sight's own TileSightClear); rays are
	// the walk's reads recorded once per offset (rayRead), indexed over
	// (2w-1) x (2h-1) offsets.
	opaque []bool
	rays   [][]rayRead

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

	base, gear, darkTalent float64 // F4: a bow taken up recomputes
}

// eyeLines is one eye's lines of sight, cached (F4): from the centre of tile
// (x, y), which tiles' lines were tried and which were clear. Tiles never
// change after generation (plan §1.4) and an eye sees from the centre of its
// tile, so a line walked once is the same line until the eye changes tile.
type eyeLines struct {
	x, y         int
	tried, clear []uint64
	claimed      uint32 // the recompute (epoch) whose eye holds it
}

// reset empties the cache for an eye now at tile (x, y) on an n-tile grid.
func (c *eyeLines) reset(x, y, n int) {
	c.x, c.y = x, y
	words := (n + 63) / 64

	if cap(c.tried) < words {
		c.tried, c.clear = make([]uint64, words), make([]uint64, words)

		return
	}

	c.tried, c.clear = c.tried[:words], c.clear[:words]

	for i := range c.tried {
		c.tried[i], c.clear[i] = 0, 0
	}
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
// anyway. A fixed source is keyed exactly: it does not move.
//
// AND FOG SEES BY IT FROM THE TILE'S CENTRE (the F3 review's A1, 1 Oct 2026):
// the disc fog iterates and the "lit" it asks (LitCarriedAt) both put the
// carried source at the centre of his tile, never his exact point. Keyed by
// tile but lit from the exact point, the lit set was the one computed where he
// ENTERED the tile and kept until he left it -- so a game saved mid-tile and
// resumed (which recomputes from where he STANDS) remembered different ground
// from the game that ran on. The drawn light still follows him exactly; the
// seen edge of his own torch can differ from the drawn one by under a tile.
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

// keyOf is a lit disc as fog sees it: a carried one shining from the centre
// of its tile (A1). It is idempotent.
func keyOf(d LitDisc) LitDisc {
	if d.Carried {
		d.X, d.Y = float64(tileOf(d.X))+0.5, float64(tileOf(d.Y))+0.5
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

	grid := image.Rect(0, 0, w, h)

	if s, ok := f.sight.(Structured); ok {
		for _, r := range s.Structures() {
			if r = r.Intersect(grid); !r.Empty() {
				f.structures = append(f.structures, r)
			}
		}
	}

	// F4: the ground's height, and every tower as an eye at the centre tile
	// of its footprint (towers are 1x1 for now: the tile itself). A line is
	// never read through the eye's own tile, so a tower sees past its walls.
	f.heights, _ = f.sight.(Heights)
	f.towers = f.towers[:0]

	if t, ok := f.sight.(Towered); ok {
		footprints, sight := t.TowerSights()

		for i, r := range footprints {
			if r = r.Intersect(grid); r.Empty() || i >= len(sight) || sight[i] <= 0 {
				continue
			}

			cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
			f.towers = append(f.towers, Eye{
				ID: fmt.Sprintf("tower/%d,%d", cx, cy), X: float64(cx) + 0.5, Y: float64(cy) + 0.5,
				Base: sight[i], Tower: true,
			})
		}
	}

	// A line cached on another map is not this map's (§1.4 holds per map),
	// nor is its opacity, nor (sized to it) its rays.
	f.lines = nil
	f.opaque = nil
	f.rays = nil
}

// readOpacity reads the map's sight-blocking tiles into fog's own grid, once
// a map (F4), when the sight can say them.
func (f *Fog) readOpacity() {
	o, ok := f.sight.(OpaqueTiles)
	if !ok || f.w <= 0 || f.h <= 0 {
		return
	}

	f.opaque = make([]bool, f.w*f.h)

	for ty := 0; ty < f.h; ty++ {
		for tx := 0; tx < f.w; tx++ {
			f.opaque[ty*f.w+tx] = o.TileBlocksSight(tx, ty)
		}
	}

	f.rays = make([][]rayRead, (2*f.w-1)*(2*f.h-1))
}

// rayRead is one read of the tile walk (d2geom.TileLineReads) from a tile's
// centre, relative to the eye's tile: one tile (x1, y1), or at a corner the
// two beside it (x2, y2 too), which stop the line only if both block.
type rayRead struct {
	x1, y1, x2, y2 int16
	corner         bool
}

// rayTo is the walk from the centre of a tile to the centre of the tile (dx,
// dy) off it, recorded once (F4): from one tile's centre to another's the walk
// is the same whatever the tiles -- every coordinate it computes is the
// offset's, to the bit -- so fog records it once and replays it from every
// eye against its own grid.
func (f *Fog) rayTo(dx, dy int) []rayRead {
	i := (dy+f.h-1)*(2*f.w-1) + dx + f.w - 1
	if r := f.rays[i]; r != nil {
		return r
	}

	r := make([]rayRead, 0, 2*(abs(dx)+abs(dy))/3+1)
	d2geom.TileLineReads(0.5, 0.5, dx, dy, func(x1, y1, x2, y2 int, corner bool) bool {
		r = append(r, rayRead{int16(x1), int16(y1), int16(x2), int16(y2), corner})

		return false
	})

	f.rays[i] = r

	return r
}

// gridLineClear is the walk replayed from eye tile (ex, ey) to tile (tx, ty)
// against fog's own grid: d2geom.TileLineClear's answer and cell count, to
// the bit (TestFogsGridWalksTheMapsLines), without the walk's arithmetic.
// Off the grid blocks, as off the map does for the map's own walk.
func (f *Fog) gridLineClear(ex, ey, tx, ty int) (clear bool, cells int) {
	w, h, opaque := f.w, f.h, f.opaque
	blocked := func(x, y int) bool { return x < 0 || y < 0 || x >= w || y >= h || opaque[y*w+x] }

	for _, rd := range f.rayTo(tx-ex, ty-ey) {
		if rd.corner {
			cells += 2

			if blocked(ex+int(rd.x1), ey+int(rd.y1)) && blocked(ex+int(rd.x2), ey+int(rd.y2)) {
				return false, cells
			}

			continue
		}

		cells++

		if blocked(ex+int(rd.x1), ey+int(rd.y1)) {
			return false, cells
		}
	}

	return true, cells
}

func abs(v int) int {
	if v < 0 {
		return -v
	}

	return v
}

// Update recomputes what the eyes see on a w x h map when something that
// decides it changed -- an eye moved to another tile, an eye came or went, a
// dial, the explored set or the map's size -- and otherwise counts the call
// as skipped and does nothing. It says whether it recomputed.
//
// The key is the eyes' tiles and the light's signature (F2): the sky
// quantised to 1/64, the moon, and every lit source where it shines from --
// a carried one by its tile (B4, A1: lightKey), so a lit torch recomputes once
// a tile step of a walk. Tiles never change after generation (plan §1.4).
func (f *Fog) Update(w, h int, eyes []Eye) bool {
	if w != f.w || h != f.h {
		f.resize(w, h)
	}

	sky, moon, band := 1.0, 0.0, 1.0
	f.discs = f.discs[:0]

	if f.light != nil {
		sky, moon, band = clamp01(f.light.SkyFraction()), clamp01(f.light.Moon()), f.light.SkyBand()
		f.discs = f.light.LitDiscs(f.discs)

		for i := range f.discs {
			f.discs[i] = keyOf(f.discs[i]) // a carried disc at its tile's centre (A1)
		}
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
		f.eyeKeys = append(f.eyeKeys, keyOfEye(e))
		e.X, e.Y = float64(x)+0.5, float64(y)+0.5
		f.eyes = append(f.eyes, e)
	}

	// F4: the map's towers see too (Q6: no garrison). They never move.
	f.eyes = append(f.eyes, f.towers...)

	f.recompute()
	f.dirty = false

	return true
}

func (f *Fog) sameEyes(eyes []Eye) bool {
	if len(eyes) != len(f.eyeKeys) {
		return false
	}

	for i, e := range eyes {
		if f.eyeKeys[i] != keyOfEye(e) {
			return false
		}
	}

	return true
}

func keyOfEye(e Eye) eyeKey {
	return eyeKey{e.ID, tileOf(e.X), tileOf(e.Y), e.Contact, e.Base, e.Gear, e.DarkTalent}
}

// sightOf is what one eye sees by (F4): sight = base (the day sight, or a
// tower's own) + gear + HeightTiles x the height of the ground under it; dark
// = tonight's dark radius + its talent; and its unlit reach, blended between
// them by the sky as UnlitReach blends a plain eye's. Never a reach past its
// sight: a talent cannot make the dark wider than the day.
func (f *Fog) sightOf(e Eye) EyeSight {
	s := EyeSight{Eye: e, BaseTerm: e.Base}
	if s.BaseTerm <= 0 {
		s.BaseTerm = f.dials.DaySight
	}

	if f.heights != nil {
		if x, y := tileOf(e.X), tileOf(e.Y); f.in(x, y) {
			s.Height = f.heights.HeightAt(x, y)
		}
	}

	s.HeightTerm = f.dials.HeightTiles * float64(s.Height)
	s.Sight = s.BaseTerm + e.Gear + s.HeightTerm
	s.Dark = f.darkRadius + e.DarkTalent

	dark := math.Min(s.Dark, s.Sight)
	s.UnlitReach = dark + (s.Sight-dark)*clamp01(f.sky)

	return s
}

// linesFor is eye i's line cache for this recompute: the one it had, if it
// stands on the same tile, else emptied for its new one.
func (f *Fog) linesFor(e Eye) *eyeLines {
	if f.lines == nil {
		f.lines = make(map[string]*eyeLines)
	}

	x, y := tileOf(e.X), tileOf(e.Y)

	c, ok := f.lines[e.ID]
	if ok && c.claimed == f.epoch && (c.x != x || c.y != y) {
		return nil // two eyes of one id on two tiles: the second walks uncached
	}

	if !ok {
		c = &eyeLines{}
		f.lines[e.ID] = c
		c.reset(x, y, f.w*f.h)
	} else if c.x != x || c.y != y || len(c.tried) != (f.w*f.h+63)/64 {
		c.reset(x, y, f.w*f.h)
	}

	c.claimed = f.epoch

	return c
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

	// F4: the map's opacity, once a map; what every eye sees by; its lines.
	if f.opaque == nil {
		f.readOpacity()
	}

	f.sights = f.sights[:0]
	f.eyeLine = f.eyeLine[:0]

	for _, e := range f.eyes {
		f.sights = append(f.sights, f.sightOf(e))

		var c *eyeLines
		if !e.Contact && f.w > 0 && f.h > 0 {
			c = f.linesFor(e)
		}

		f.eyeLine = append(f.eyeLine, c)
	}

	f.pruneLines()

	if f.w <= 0 || f.h <= 0 {
		return
	}

	for i, e := range f.eyes {
		if e.Contact {
			continue // shown last, below
		}

		// The eye's own tile, even an eye standing off the grid's last row.
		if ex, ey := tileOf(e.X), tileOf(e.Y); f.in(ex, ey) && !f.isVisible(ex, ey) {
			f.see(ex, ey)
		}

		r := f.sights[i].UnlitReach
		x0, x1 := clampInt(tileOf(e.X-r), 0, f.w-1), clampInt(tileOf(e.X+r), 0, f.w-1)
		y0, y1 := clampInt(tileOf(e.Y-r), 0, f.h-1), clampInt(tileOf(e.Y+r), 0, f.h-1)

		for ty := y0; ty <= y1; ty++ {
			for tx := x0; tx <= x1; tx++ {
				if f.isVisible(tx, ty) {
					continue // another eye saw it already
				}

				if f.sees(i, tx, ty, r) {
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
	// His carried sources shine, for fog, from the centre of his tile (A1):
	// every carried disc in the key is already there (keyOf).
	cx, cy, carried := 0.0, 0.0, false

	for _, d := range f.key.discs {
		if d.Carried {
			cx, cy, carried = d.X, d.Y, true

			break
		}
	}

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

				if !f.litFor(tx, ty, cx, cy, carried) {
					continue
				}

				for i, e := range f.eyes {
					if e.Contact {
						continue
					}

					if f.lineClear(i, tx, ty) {
						f.see(tx, ty)
						f.litSeen++

						break
					}
				}
			}
		}
	}
}

// litFor is "lit" as fog sees it: with his carried sources shining from
// (cx, cy), the centre of his tile, when there are any (A1).
func (f *Fog) litFor(tx, ty int, cx, cy float64, carried bool) bool {
	if carried {
		return f.light.LitCarriedAt(tx, ty, cx, cy)
	}

	return f.light.Lit(tx, ty)
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
func (f *Fog) sees(i, tx, ty int, r float64) bool {
	e := f.eyes[i]
	if tx == tileOf(e.X) && ty == tileOf(e.Y) {
		return true
	}

	if math.Hypot(float64(tx)+0.5-e.X, float64(ty)+0.5-e.Y) > r {
		return false
	}

	return f.lineClear(i, tx, ty)
}

// lineClear is the map's line of sight from eye i (of this recompute) to a
// tile on the grid, counted: from the eye's line cache when it was walked
// before from the same tile (F4), else walked and remembered.
func (f *Fog) lineClear(i, tx, ty int) bool {
	if f.sight == nil {
		return true
	}

	c, t := f.eyeLine[i], ty*f.w+tx
	word, bit := t/64, uint64(1)<<(uint(t)%64)

	if c != nil && c.tried[word]&bit != 0 {
		f.linesHit++

		return c.clear[word]&bit != 0
	}

	e := f.eyes[i]

	var (
		clear bool
		cells int
	)

	if f.opaque != nil {
		clear, cells = f.gridLineClear(tileOf(e.X), tileOf(e.Y), tx, ty)
	} else {
		clear, cells = f.sight.TileSightClear(e.X, e.Y, tx, ty)
	}

	f.cellsRead += cells

	if c != nil {
		c.tried[word] |= bit

		if clear {
			c.clear[word] |= bit
		}
	}

	return clear
}

// pruneLines forgets the line caches of eyes that are gone, so a game of many
// comings and goings holds one cache an eye.
func (f *Fog) pruneLines() {
	if len(f.lines) <= len(f.eyes) {
		return
	}

	keep := make(map[string]bool, len(f.eyes))
	for _, e := range f.eyes {
		keep[e.ID] = true
	}

	for id := range f.lines {
		if !keep[id] {
			delete(f.lines, id)
		}
	}
}

// EyeSights are the eyes of the last recompute with what each saw by (F4):
// sight, its terms, dark radius, unlit reach and the ground's height.
func (f *Fog) EyeSights() []EyeSight { return append([]EyeSight(nil), f.sights...) }

// LinesCached is how many lines of sight were read from the eyes' caches
// rather than walked (F4's cost counter, beside Counters' cells read).
func (f *Fog) LinesCached() int { return f.linesHit }

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
// in the game forgets. It forgets the eyes' cached lines and fog's read of the
// map's opacity too (F4), so the next Update walks every line from the map again: the one way to make fog see a
// map whose tiles changed. Tiles never change after generation today (plan
// §1.4); the day a house can burn mid-game, that change must drop the lines
// as this does (§3.9).
func (f *Fog) Forget() {
	for i := range f.explored {
		f.explored[i] = 0
	}

	f.exploredCount = 0
	f.lines = nil
	f.opaque = nil
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
