// Command villagemap writes the M5.4 v0 village: a Tiled map
// (data/strigoi/maps/village.tmj) and the PLACEHOLDER tile art it uses
// (data/strigoi/maps/tiles/placeholder-*.png).
//
// Everything it draws is a flat-coloured shape standing in for art that is
// Josh's and GPT's to make (art is their lane) -- except where their art
// already exists: the houses, the burned house, the well and the hearth are
// the art-ready renders from strigoi-art (copied into data/strigoi/structures,
// provenance there), placed as structures on their footprints. The map is the point: the
// village laid out by the slice spec and G4 -- a hasty ditch and wattle fence
// with its gate to the south and one corner unfinished, the church and its
// graves on the high ground at the north end, a well on the green, timber
// houses, the smithy by the gate, forest on the slope -- in a file that opens
// in Tiled, where it can be moved about by hand from here on. Once it has
// been edited there, do not re-run this over it.
//
//	go run ./tools/villagemap            (from the repo root)
package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
)

const (
	side  = 48 // tiles on each side of the map
	tileW = 160
	tileH = 80
)

// ---- placeholder art -----------------------------------------------------------

type rgb struct{ r, g, b uint8 }

func (c rgb) shade(f float64) color.NRGBA {
	s := func(v uint8) uint8 { return uint8(math.Max(0, math.Min(255, float64(v)*f))) }
	return color.NRGBA{R: s(c.r), G: s(c.g), B: s(c.b), A: 255}
}

type pt struct{ x, y float64 }

// fillPoly fills a convex polygon, pixel centres inside.
func fillPoly(img *image.NRGBA, poly []pt, c func(x, y int) color.NRGBA) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if inside(poly, float64(x)+0.5, float64(y)+0.5) {
				img.SetNRGBA(x, y, c(x, y))
			}
		}
	}
}

func inside(poly []pt, x, y float64) bool {
	sign := 0
	for i := range poly {
		a, b := poly[i], poly[(i+1)%len(poly)]
		cross := (b.x-a.x)*(y-a.y) - (b.y-a.y)*(x-a.x)
		s := 1
		if cross < 0 {
			s = -1
		} else if cross == 0 {
			continue
		}
		if sign == 0 {
			sign = s
		} else if s != sign {
			return false
		}
	}
	return true
}

// noise is a deterministic per-pixel speckle, so a flat colour reads as ground.
func noise(x, y, salt int) float64 {
	h := uint32(x*73856093) ^ uint32(y*19349663) ^ uint32(salt*83492791)
	h ^= h >> 13
	h *= 0x5bd1e995
	h ^= h >> 15
	return float64(h%1000) / 1000
}

func diamond(top float64) []pt {
	return []pt{{80, top}, {160, top + 40}, {80, top + 80}, {0, top + 40}}
}

func floorTile(base rgb, salt int, edge bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, tileW, tileH))
	fillPoly(img, diamond(0), func(x, y int) color.NRGBA {
		return base.shade(0.9 + 0.2*noise(x, y, salt))
	})
	if edge {
		// A faint rim so the grid reads while the map is being authored.
		for x := 0; x < tileW; x++ {
			for y := 0; y < tileH; y++ {
				c := img.NRGBAAt(x, y)
				if c.A == 0 {
					continue
				}
				fx, fy := float64(x)+0.5, float64(y)+0.5
				d := math.Abs(fx-80)/80 + math.Abs(fy-40)/40
				if d > 0.97 {
					img.SetNRGBA(x, y, base.shade(0.7))
				}
			}
		}
	}
	return img
}

// box draws an isometric block standing on the floor diamond: height e pixels
// above it, faces lit from the left.
func box(h int, e float64, side, top rgb, inset float64) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, tileW, h))
	H := float64(h)
	// Footprint, optionally inset from the tile's edges.
	T, R := pt{80, H - 80 + inset}, pt{160 - 2*inset, H - 40}
	B, L := pt{80, H - inset}, pt{2 * inset, H - 40}
	up := func(p pt) pt { return pt{p.x, p.y - e} }
	fillPoly(img, []pt{L, B, up(B), up(L)}, func(x, y int) color.NRGBA { return side.shade(0.95) })
	fillPoly(img, []pt{B, R, up(R), up(B)}, func(x, y int) color.NRGBA { return side.shade(0.7) })
	fillPoly(img, []pt{up(T), up(R), up(B), up(L)}, func(x, y int) color.NRGBA {
		return top.shade(0.9 + 0.2*noise(x, y, 7))
	})
	return img
}

func stripe(img *image.NRGBA, every int, dark float64) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			if c.A != 0 && x%every == 0 {
				img.SetNRGBA(x, y, color.NRGBA{R: uint8(float64(c.R) * dark), G: uint8(float64(c.G) * dark), B: uint8(float64(c.B) * dark), A: 255})
			}
		}
	}
}

func tree() *image.NRGBA {
	h := 230
	img := image.NewNRGBA(image.Rect(0, 0, tileW, h))
	trunk := rgb{70, 50, 35}
	fillPoly(img, []pt{{74, float64(h) - 40}, {86, float64(h) - 40}, {86, float64(h) - 120}, {74, float64(h) - 120}},
		func(x, y int) color.NRGBA { return trunk.shade(1) })
	leaf := rgb{30, 70, 35}
	cx, cy, r := 80.0, float64(h)-150, 58.0
	for y := 0; y < h; y++ {
		for x := 0; x < tileW; x++ {
			dx, dy := float64(x)+0.5-cx, (float64(y)+0.5-cy)*1.15
			if dx*dx+dy*dy < r*r {
				img.SetNRGBA(x, y, leaf.shade(0.8+0.35*noise(x/3, y/3, 11)-0.2*dx/r))
			}
		}
	}
	return img
}

func cross() *image.NRGBA {
	h := 130
	img := image.NewNRGBA(image.Rect(0, 0, tileW, h))
	wood := rgb{95, 80, 60}
	H := float64(h)
	fillPoly(img, []pt{{77, H - 40}, {83, H - 40}, {83, H - 95}, {77, H - 95}}, func(x, y int) color.NRGBA { return wood.shade(1) })
	fillPoly(img, []pt{{66, H - 82}, {94, H - 82}, {94, H - 76}, {66, H - 76}}, func(x, y int) color.NRGBA { return wood.shade(0.9) })
	// the grave mound, drawn on the floor so the cross stands on something
	fillPoly(img, []pt{{80, H - 52}, {104, H - 40}, {80, H - 28}, {56, H - 40}}, func(x, y int) color.NRGBA {
		return rgb{85, 70, 50}.shade(0.9 + 0.2*noise(x, y, 5))
	})
	return img
}

func well() *image.NRGBA {
	const h, e = 120, 34
	img := box(h, e, rgb{120, 115, 105}, rgb{120, 115, 105}, 30)
	// Dark water in the middle of the top face, whose centre is 40+e above
	// the image's bottom edge.
	cx, cy := 80.0, float64(h)-40-e
	fillPoly(img, []pt{{cx, cy - 6}, {cx + 12, cy}, {cx, cy + 6}, {cx - 12, cy}},
		func(x, y int) color.NRGBA { return rgb{20, 30, 45}.shade(1) })
	return img
}

// ---- the map ---------------------------------------------------------------------

type tileDef struct {
	name        string
	img         *image.NRGBA
	blocked     *bool
	blocksSight *bool
	// art is a path, relative to the maps folder, to real art from
	// strigoi-art; img is then nil and nothing is drawn for it.
	art string
	// footprint marks a structure (placed as a tile object), in tiles.
	footprint [2]int
}

func b(v bool) *bool { return &v }

type tileset struct {
	defs []tileDef
	gid  map[string]int
}

func (t *tileset) add(d tileDef) {
	t.defs = append(t.defs, d)
	t.gid[d.name] = len(t.defs) // firstgid 1, ids from 0
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	dir := filepath.Join(root, "data", "strigoi", "maps")
	if err := os.MkdirAll(filepath.Join(dir, "tiles"), 0o750); err != nil {
		log.Fatal(err)
	}

	ts := &tileset{gid: map[string]int{}}

	// Floors.
	ts.add(tileDef{name: "grass", img: floorTile(rgb{62, 92, 48}, 1, true)})
	ts.add(tileDef{name: "grass-dark", img: floorTile(rgb{50, 78, 42}, 2, true)})
	ts.add(tileDef{name: "yard", img: floorTile(rgb{98, 84, 60}, 3, true)})
	ts.add(tileDef{name: "road", img: floorTile(rgb{122, 104, 76}, 4, true)})
	ts.add(tileDef{name: "churchyard", img: floorTile(rgb{70, 88, 58}, 5, true)})
	// The ditch: nobody walks it, everybody sees across it.
	ts.add(tileDef{name: "ditch", img: floorTile(rgb{58, 44, 32}, 6, true), blocked: b(true), blocksSight: b(false)})

	// Walls.
	fence := box(130, 50, rgb{120, 96, 62}, rgb{104, 84, 55}, 12)
	stripe(fence, 7, 0.6)
	ts.add(tileDef{name: "fence", img: fence, blocked: b(true), blocksSight: b(false)})
	ts.add(tileDef{name: "house", img: box(200, 120, rgb{128, 96, 64}, rgb{150, 132, 80}, 0), blocked: b(true)})
	ts.add(tileDef{name: "smithy", img: box(180, 100, rgb{90, 80, 75}, rgb{60, 55, 55}, 0), blocked: b(true)})
	ts.add(tileDef{name: "church", img: box(240, 160, rgb{150, 130, 105}, rgb{105, 80, 60}, 0), blocked: b(true)})
	ts.add(tileDef{name: "church-tower", img: box(330, 250, rgb{150, 130, 105}, rgb{95, 70, 55}, 14), blocked: b(true)})
	ts.add(tileDef{name: "well", img: well(), blocked: b(true), blocksSight: b(false)})
	ts.add(tileDef{name: "tree", img: tree(), blocked: b(true)})
	ts.add(tileDef{name: "grave", img: cross(), blocked: b(false)})

	// The art that exists (strigoi-art, art-ready v1).
	ts.add(tileDef{name: "village-well", art: "../structures/village-well/intact.png", blocked: b(true), blocksSight: b(false)})
	ts.add(tileDef{name: "village-hearth", art: "../structures/village-hearth/unlit.png", blocked: b(true), blocksSight: b(false)})
	ts.add(tileDef{name: "peasant-house", art: "../structures/peasant-house/intact.png", footprint: [2]int{3, 3}})
	ts.add(tileDef{name: "burned-house", art: "../structures/burned-house/cold-ruin.png", footprint: [2]int{3, 3}})

	for _, d := range ts.defs {
		if d.img == nil {
			continue
		}

		f, err := os.Create(filepath.Join(dir, "tiles", "placeholder-"+d.name+".png"))
		if err != nil {
			log.Fatal(err)
		}
		if err := png.Encode(f, d.img); err != nil {
			log.Fatal(err)
		}
		f.Close()
	}

	floor := make([]int, side*side)
	walls := make([]int, side*side)
	set := func(layer []int, x, y int, name string) {
		if x < 0 || y < 0 || x >= side || y >= side {
			return
		}
		layer[x+y*side] = ts.gid[name]
	}

	// Ground: grass, darker toward the north-west slope.
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			name := "grass"
			if x+y < 22 || noise(x, y, 99) < 0.18 {
				name = "grass-dark"
			}
			set(floor, x, y, name)
		}
	}

	// The enclosure: fence on the ring x,y in [lo,hi], ditch one tile outside.
	lo, hi := 12, 35
	gate := map[int]bool{23: true, 24: true} // on the south side (y = hi)
	// The unfinished corner: the north-east run of the fence stops short.
	unfinished := func(x, y int) bool { return y == lo && x >= hi-4 }

	for i := lo; i <= hi; i++ {
		for _, p := range [][2]int{{i, lo}, {i, hi}, {lo, i}, {hi, i}} {
			x, y := p[0], p[1]
			if y == hi && gate[x] {
				set(floor, x, y, "road")
				continue
			}
			if unfinished(x, y) {
				continue
			}
			set(walls, x, y, "fence")
		}
	}

	for i := lo - 1; i <= hi+1; i++ {
		for _, p := range [][2]int{{i, lo - 1}, {i, hi + 1}, {lo - 1, i}, {hi + 1, i}} {
			x, y := p[0], p[1]
			if y == hi+1 && gate[x] {
				continue
			}
			if y == lo-1 && x >= hi-5 {
				continue // no ditch where the fence was never finished
			}
			set(floor, x, y, "ditch")
		}
	}

	// The road: from the gate south off the map, and in to the green.
	for y := 24; y < side; y++ {
		if y == hi+1 {
			set(floor, 23, y, "road")
			set(floor, 24, y, "road")
			continue
		}
		set(floor, 23, y, "road")
		set(floor, 24, y, "road")
	}
	for x := 16; x <= 31; x++ {
		set(floor, x, 23, "road")
	}

	// The churchyard and church at the north end (high ground).
	for y := lo + 1; y <= 19; y++ {
		for x := lo + 1; x <= 21; x++ {
			set(floor, x, y, "churchyard")
		}
	}
	for x := 15; x <= 17; x++ {
		for y := 14; y <= 15; y++ {
			set(walls, x, y, "church")
		}
	}
	set(walls, 18, 14, "church-tower")
	set(walls, 18, 15, "church")
	for _, g := range [][2]int{{14, 17}, {16, 18}, {18, 17}, {20, 18}, {20, 16}, {14, 19}, {17, 19}, {21, 14}} {
		set(walls, g[0], g[1], "grave")
	}

	// The green and the well.
	for y := 21; y <= 26; y++ {
		for x := 21; x <= 26; x++ {
			if floor[x+y*side] != ts.gid["road"] {
				set(floor, x, y, "yard")
			}
		}
	}
	set(walls, 24, 24, "village-well")
	set(walls, 21, 25, "village-hearth")

	// Houses: 2x2 blocks on yards, 10-35 m apart (G4); the headman's is 3x2.
	house := func(x, y, w, h int, kind string) {
		for yy := y - 1; yy <= y+h; yy++ {
			for xx := x - 1; xx <= x+w; xx++ {
				if floor[xx+yy*side] != ts.gid["road"] {
					set(floor, xx, yy, "yard")
				}
			}
		}
		for yy := y; yy < y+h; yy++ {
			for xx := x; xx < x+w; xx++ {
				set(walls, xx, yy, kind)
			}
		}
	}
	// The houses are structures (3x3, placed as tile objects below); here
	// only their yards. The smithy has no art yet and stays a placeholder
	// block on the walls layer.
	type building struct {
		x, y int
		kind string
	}

	buildings := []building{
		{27, 18, "peasant-house"}, // the headman's
		{28, 26, "peasant-house"},
		{31, 30, "burned-house"}, // last month's raid, inside the fence
		{14, 26, "peasant-house"},
		{17, 30, "peasant-house"},
		{29, 13, "peasant-house"},
		{23, 13, "peasant-house"},
	}

	for _, bl := range buildings {
		for yy := bl.y - 1; yy <= bl.y+3; yy++ {
			for xx := bl.x - 1; xx <= bl.x+3; xx++ {
				if floor[xx+yy*side] != ts.gid["road"] {
					set(floor, xx, yy, "yard")
				}
			}
		}
	}

	house(26, 31, 2, 2, "smithy") // by the gate

	// Forest on the slope above (north-west), thinning out.
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			inEnclosure := x >= lo-1 && x <= hi+1 && y >= lo-1 && y <= hi+1
			if inEnclosure || walls[x+y*side] != 0 || floor[x+y*side] == ts.gid["road"] {
				continue
			}
			density := 0.03
			if x+y < 26 {
				density = 0.35
			}
			if noise(x, y, 42) < density {
				set(walls, x, y, "tree")
			}
		}
	}

	// ---- objects ----
	type prop struct {
		Name  string `json:"name"`
		Type  string `json:"type"`
		Value any    `json:"value"`
	}
	type object struct {
		ID         int     `json:"id"`
		Name       string  `json:"name"`
		Type       string  `json:"type"`
		X          float64 `json:"x"`
		Y          float64 `json:"y"`
		Width      float64 `json:"width"`
		Height     float64 `json:"height"`
		Rotation   float64 `json:"rotation"`
		Visible    bool    `json:"visible"`
		Point      bool    `json:"point"`
		GID        int     `json:"gid,omitempty"`
		Properties []prop  `json:"properties,omitempty"`
	}
	objects := []object{}
	add := func(name, typ string, tx, ty float64, props ...prop) {
		objects = append(objects, object{ID: len(objects) + 1, Name: name, Type: typ,
			X: tx * tileH, Y: ty * tileH, Visible: true, Point: true, Properties: props})
	}
	add("start", "player_start", 23.5, 28.5)

	// The village within its fence, the ring included: the night does not
	// arrive on it (arrivals are placed outside and come in by the gate).
	objects = append(objects, object{ID: len(objects) + 1, Name: "the village", Type: "inside",
		X: float64(lo) * tileH, Y: float64(lo) * tileH,
		Width: float64(hi-lo+1) * tileH, Height: float64(hi-lo+1) * tileH, Visible: true})
	// The structures: tile objects whose bottom corner is the footprint's
	// bottom corner, where Tiled draws them and where the game stands them.
	for _, bl := range buildings {
		w, h := artSize(dir, ts.defs[ts.gid[bl.kind]-1].art)
		objects = append(objects, object{ID: len(objects) + 1, Name: bl.kind, Type: "structure",
			X: float64(bl.x+3) * tileH, Y: float64(bl.y+3) * tileH,
			Width: float64(w), Height: float64(h), Visible: true, GID: ts.gid[bl.kind]})
	}

	npc := func(name, monstat string, tx, ty float64) {
		add(name, "npc", tx, ty, prop{Name: "monstat", Type: "string", Value: monstat})
	}
	// The four speakers' D2 stand-ins (data/strigoi/dialogue.json).
	npc("headman (Warriv)", "warriv1", 28.5, 22.5)
	npc("woman at the well (Kashya)", "kashya", 25.5, 25.5)
	npc("smith (Charsi)", "charsi", 25.5, 29.5)
	npc("priest (Akara)", "akara", 19.5, 16.5)

	// ---- the tileset ----
	type tsTile struct {
		ID          int    `json:"id"`
		Image       string `json:"image"`
		ImageWidth  int    `json:"imagewidth"`
		ImageHeight int    `json:"imageheight"`
		Properties  []prop `json:"properties,omitempty"`
	}
	tiles := []tsTile{}
	maxH, maxW := 0, tileW
	for i, d := range ts.defs {
		t := tsTile{ID: i, Image: "tiles/placeholder-" + d.name + ".png", ImageWidth: tileW}
		if d.img != nil {
			t.ImageHeight = d.img.Bounds().Dy()
		} else {
			t.Image = d.art
			t.ImageWidth, t.ImageHeight = artSize(dir, d.art)
		}

		if d.footprint != [2]int{} {
			t.Properties = append(t.Properties,
				prop{Name: "footprint_w", Type: "int", Value: d.footprint[0]},
				prop{Name: "footprint_h", Type: "int", Value: d.footprint[1]})
		}

		if d.blocked != nil {
			t.Properties = append(t.Properties, prop{Name: "blocked", Type: "bool", Value: *d.blocked})
		}
		if d.blocksSight != nil {
			t.Properties = append(t.Properties, prop{Name: "blocks_sight", Type: "bool", Value: *d.blocksSight})
		}
		if t.ImageWidth > maxW {
			maxW = t.ImageWidth
		}

		if t.ImageHeight > maxH {
			maxH = t.ImageHeight
		}
		tiles = append(tiles, t)
	}

	layer := func(id int, name string, data []int) map[string]any {
		return map[string]any{"id": id, "name": name, "type": "tilelayer", "width": side, "height": side,
			"x": 0, "y": 0, "opacity": 1, "visible": true, "data": data}
	}

	m := map[string]any{
		"type": "map", "version": "1.10", "tiledversion": "1.10.2",
		"orientation": "isometric", "renderorder": "right-down",
		"width": side, "height": side, "tilewidth": tileW, "tileheight": tileH,
		"infinite": false, "compressionlevel": -1,
		"nextlayerid": 4, "nextobjectid": len(objects) + 1,
		"properties": []prop{{Name: "note", Type: "string",
			Value: "M5.4 v0 village. ALL TILE ART IS PLACEHOLDER (tools/villagemap); the layout is a proposal from S1 section 9.1 and G4, Josh decides."}},
		"layers": []any{
			layer(1, "floor", floor),
			layer(2, "walls", walls),
			map[string]any{"id": 3, "name": "objects", "type": "objectgroup", "draworder": "topdown",
				"x": 0, "y": 0, "opacity": 1, "visible": true, "objects": objects},
		},
		"tilesets": []any{map[string]any{
			"firstgid": 1, "name": "village-placeholder", "columns": 0, "margin": 0, "spacing": 0,
			"tilewidth": maxW, "tileheight": maxH, "tilecount": len(tiles),
			"objectalignment": "bottom", "tiles": tiles,
		}},
	}

	// "note" is a map property, not a tile property: d2maptiled reads tile
	// properties only, so it is documentation to whoever opens the file.
	out, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "village.tmj"), append(out, '\n'), 0o640); err != nil {
		log.Fatal(err)
	}

	names := make([]string, 0, len(ts.gid))
	for n := range ts.gid {
		names = append(names, n)
	}
	sort.Strings(names)
	fmt.Printf("wrote %s: %dx%d, %d tile kinds (%v), %d objects\n", filepath.Join(dir, "village.tmj"), side, side, len(names), names, len(objects))
}

// artSize reads the size of a real art file, relative to the maps folder.
func artSize(dir, rel string) (int, int) {
	f, err := os.Open(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	cfg, err := png.DecodeConfig(f)
	if err != nil {
		log.Fatalf("%s: %v", rel, err)
	}

	return cfg.Width, cfg.Height
}
