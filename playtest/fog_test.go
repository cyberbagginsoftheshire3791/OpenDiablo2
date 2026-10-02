//go:build playtest

package playtest

import (
	"fmt"
	"image"
	"math"
	"sort"
	"testing"
)

// TestFogOfWar is fog of war F1's script: "black until explored", by day
// (claude/fog-of-war-build-plan.md §4 F1; Josh's Q1, day sight 12). UNAIDED:
// nothing is spawned, watched or pursued; the village's own villagers are the
// people it looks for.
//
//  1. The day start, at zoom 0.5 with -fog: the provider sees his disc and not
//     beyond it, the screen around him is drawn (the black-floor control), and
//     a tile past his sight is exact black on screen. 1b: the same counts at
//     zoom 1.0 (fog is in world tiles; the camera is not in its rule).
//  4. (run here, on act 1's frame, so the camera has not moved) The control:
//     fog turned off over the harness draws that same point, so the instrument
//     can see the difference; turned back on, it is black again.
//  2. A walk of 13+ tiles: the start tile is explored and not visible; and a
//     remembered tile on screen is drawn grey -- not black, saturation under
//     0.35.
//  3. A villager more than 12 tiles off is not shown (get_entity.shown false);
//     walk to him and he is.
//  5. (run on act 1's frame) The house to his right is visible whole and its
//     art is drawn -- a house seen at all is seen whole.
//
// The provider is the primary evidence; the pixels are the second (plan
// §3.10).
//
// Negative control (1 Oct 2026, as a build overlay: STRIGOI_HARNESS_OVERLAY):
// draw every tile, explored or not (tileDrawn always true), and act 1 fails,
// "the unexplored tile (31,27) at screen (760,436) is drawn (36,34,32);
// unexplored is exact black" (strigoi-harness-runs\wt-fog\nc\ncp2-unexplored-drawn.txt).
// Act 4 is the script's own control on the instrument.
func TestFogOfWar(t *testing.T) {
	const (
		dawnMinute = 165.0 // 02:45, the epoch
		daySight   = 12.0
	)

	s := startGame(t, "-fog", "-zoom", "0.5")

	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Fog", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	t.Logf("spawned at %v", game["spawn_tile"])

	// The dead stay down and nothing spawns: this script watches the ground.
	setField(s, "rising", "p", 0.0)
	setField(s, "rising", "edge_floor", 0)
	setField(s, "spawns", "chance", 0)

	s.call("strigoi_step_world", map[string]any{"world_minutes": 12*60 - dawnMinute})
	s.call("strigoi_step", map[string]any{"frames": 2})

	// --- act 1: the day start ------------------------------------------------
	fog := fogState(s)
	if !flag(t, fog, "enabled") || !flag(t, fog, "drawn") {
		t.Fatalf("-fog: the fog is not on (enabled %v, drawn %v, off_reason %q)",
			fog["enabled"], fog["drawn"], str(fog, "off_reason"))
	}

	if got := mustNum(t, fog, "day_sight"); got != daySight {
		t.Fatalf("day sight is %v; Josh's Q1 is 12", got)
	}

	px, py := playerTile(s)
	t.Logf("act 1: he stands on tile (%d,%d); fog %v explored, %v visible, %v recomputes",
		px, py, fog["explored"], fog["visible"], fog["recomputes"])

	if e, v := mustNum(t, fog, "explored"), mustNum(t, fog, "visible"); v < 100 || e != v {
		t.Fatalf("at the start he sees %v tiles and has explored %v; want a disc (100+) and nothing remembered yet", v, e)
	}

	// A tile 6 away with a clear line is visible; a tile 15 away is not seen.
	near := findProbe(t, s, px, py, 6, func(p map[string]any) bool { return clearFromEye(p) })
	if near == nil {
		t.Fatal("no tile 6 away has a clear line from him: the village walls him in")
	}

	if st := str(near, "state"); st != "visible" {
		t.Fatalf("the tile 6 away at (%v,%v) with a clear line is %s", near["x"], near["y"], st)
	}

	day := s.frame(t, "fog-1-day-start-v2") // Josh's evidence: black beyond what he has seen

	// --- act 5: the house to his right is drawn whole -----------------------
	// (pulled into F1 on 1 Oct 2026: a house's art stands on its front tiles,
	// which its own footprint hides from an eye behind or beside it; fog shows a
	// structure whole once any of it is seen.)
	houseActRightOfHim(t, s, day, px, py)

	// THE BLACK, IN PIXELS. At zoom 0.5 day sight 12 reaches past the
	// screen's sides and nearly to its corners (plan §1.8: 7.1 tiles to the
	// sides, 12.7 to the corners), so little unexplored ground is on screen.
	// To measure it the explored set is FORGOTTEN and the dial narrowed to 5
	// for this look -- the black is what is measured, not the radius -- and
	// the tile is taken BELOW him on screen, where only tiles still further
	// down (unexplored, undrawn) could draw over it.
	setField(s, "fog", "forget", true)
	setField(s, "fog", "day_sight", 5.0)
	s.call("strigoi_step", map[string]any{"frames": 2})

	img := s.frame(t, "fog-1-black")
	psx, psy := playerScreen(t, s)

	if lum := meanLum(img, image.Rect(psx-60, psy-60, psx+60, psy+20)); lum < 10 {
		s.stop()
		t.Skipf("the noon frame around him is black (mean %.1f/255) -- a black-floor launch (P3 §5.3); "+
			"nothing can be measured against it", lum)
	}

	far := lowestProbe(t, s, 6.5, 9, 0.5, func(p map[string]any) bool { return str(p, "state") == "unexplored" })
	if far == nil {
		t.Fatal("no tile 6.5..9 tiles off is both unexplored and on screen below him")
	}

	fx, fy := probeScreen(far)
	r, g, b := rgbAt(img, fx, fy)
	t.Logf("act 1: the unexplored tile (%v,%v) is at screen (%d,%d), pixel (%d,%d,%d)", far["x"], far["y"], fx, fy, r, g, b)

	if r != 0 || g != 0 || b != 0 {
		t.Fatalf("the unexplored tile (%v,%v) at screen (%d,%d) is drawn (%d,%d,%d); unexplored is exact black",
			far["x"], far["y"], fx, fy, r, g, b)
	}

	// --- act 4 (the control, on act 1's frame) -------------------------------
	setField(s, "fog", "enabled", false)
	s.call("strigoi_step", map[string]any{"frames": 2})

	if off := fogState(s); flag(t, off, "enabled") || flag(t, off, "drawn") {
		t.Fatalf("enabled=false left the fog on (enabled %v, drawn %v)", off["enabled"], off["drawn"])
	}

	unfogged := s.frame(t, "fog-4-control-off")
	r2, g2, b2 := rgbAt(unfogged, fx, fy)
	t.Logf("act 4: with fog off the same point is (%d,%d,%d)", r2, g2, b2)

	if r2+g2+b2 == 0 {
		t.Fatalf("with fog off the point (%d,%d) is still black: the instrument cannot tell fog from the ground", fx, fy)
	}

	setField(s, "fog", "enabled", true)
	s.call("strigoi_step", map[string]any{"frames": 2})

	if r3, g3, b3 := rgbAt(s.frame(t, "fog-4-control-on"), fx, fy); r3+g3+b3 != 0 {
		t.Fatalf("fog back on: the point (%d,%d) is (%d,%d,%d), not black again", fx, fy, r3, g3, b3)
	}

	setField(s, "fog", "day_sight", daySight)
	s.call("strigoi_step", map[string]any{"frames": 2})

	// --- act 1b: the camera is not in the rule --------------------------------
	before := fogState(s)

	setField(s, "ui", "zoom", 1.0)
	s.call("strigoi_step", map[string]any{"frames": 2})

	after := fogState(s)
	if mustNum(t, after, "visible") != mustNum(t, before, "visible") ||
		mustNum(t, after, "explored") != mustNum(t, before, "explored") {
		t.Fatalf("zoom 0.5 -> 1.0 changed the fog: visible %v -> %v, explored %v -> %v",
			before["visible"], after["visible"], before["explored"], after["explored"])
	}

	if mustNum(t, after, "recomputes") != mustNum(t, before, "recomputes") {
		t.Fatalf("a zoom recomputed the fog (%v -> %v recomputes)", before["recomputes"], after["recomputes"])
	}

	setField(s, "ui", "zoom", 0.5)
	s.call("strigoi_step", map[string]any{"frames": 2})

	// --- act 2: a walk --------------------------------------------------------
	startX, startY := px, py

	walked := fogWalkAway(t, s, float64(startX)+0.5, float64(startY)+0.5, 13.5)
	nx, ny := playerTile(s)
	t.Logf("act 2: walked %.1f tiles to (%d,%d)", walked, nx, ny)

	setField(s, "fog", "probe", map[string]any{"x": startX, "y": startY})

	if st := str(sub(fogState(s), "probe"), "state"); st != "explored" {
		t.Fatalf("after a walk of %.1f tiles the start tile (%d,%d) is %s; want explored (remembered, not seen)",
			walked, startX, startY, st)
	}

	s.frame(t, "fog-2-after-walk-v2") // Josh's evidence: the remembered ground behind him

	fog = fogState(s)
	t.Logf("act 2: %v explored, %v visible, %v recomputes, %v skipped, %v cells read",
		fog["explored"], fog["visible"], fog["recomputes"], fog["skipped"], fog["cells_read"])

	if mustNum(t, fog, "explored") <= mustNum(t, fog, "visible") {
		t.Fatal("after the walk nothing is remembered")
	}

	if mustNum(t, fog, "skipped") == 0 {
		t.Fatal("no frame of the walk was skipped: the fog recomputes every frame")
	}

	// The grey: at zoom 0.5 day sight 12 reaches past the screen's sides, so
	// the shot above shows little remembered ground. To put some on screen
	// the DIAL is narrowed to 5 for this one look (the provider's dial; the
	// grey is what is measured, not the radius).
	setField(s, "fog", "day_sight", 5.0)
	s.call("strigoi_step", map[string]any{"frames": 2})

	grey := s.frame(t, "fog-2-grey")

	memory := lowestProbe(t, s, 6.5, 9, 0.5, func(p map[string]any) bool { return str(p, "state") == "explored" })
	if memory == nil {
		t.Fatal("no remembered tile 6.5..9 off is on screen with day sight 5")
	}

	mx, my := probeScreen(memory)
	gr, gg, gb := rgbAt(grey, mx, my)
	sat := saturation(gr, gg, gb)
	t.Logf("act 2: the remembered tile (%v,%v) at screen (%d,%d) is (%d,%d,%d), saturation %.2f",
		memory["x"], memory["y"], mx, my, gr, gg, gb, sat)

	if gr+gg+gb < 6 {
		t.Fatalf("the remembered tile (%v,%v) is drawn black (%d,%d,%d); remembered ground is grey, not gone",
			memory["x"], memory["y"], gr, gg, gb)
	}

	if sat >= 0.35 {
		t.Fatalf("the remembered tile (%v,%v) is drawn at saturation %.2f; remembered ground is greyed (< 0.35)",
			memory["x"], memory["y"], sat)
	}

	setField(s, "fog", "day_sight", daySight)
	s.call("strigoi_step", map[string]any{"frames": 2})

	// --- act 3: a villager ----------------------------------------------------
	handle, dist := farVillager(t, s, daySight+0.5)
	if handle == "" {
		t.Fatalf("no villager is more than %.1f tiles from him", daySight+0.5)
	}

	ent := s.call("strigoi_get_entity", map[string]any{"handle": handle})
	t.Logf("act 3: %s at (%.1f,%.1f), %.1f tiles off: shown %v", handle, num(ent, "x"), num(ent, "y"), dist, ent["shown"])

	if shown, ok := ent["shown"].(bool); !ok || shown {
		t.Fatalf("the villager %s %.1f tiles off is shown (%v); past his sight no one is", handle, dist, ent["shown"])
	}

	walkNearFog(t, s, handle, 2.5)

	ent = s.call("strigoi_get_entity", map[string]any{"handle": handle})
	if shown, ok := ent["shown"].(bool); !ok || !shown {
		t.Fatalf("beside him, the villager %s is not shown (%v)", handle, ent["shown"])
	}
}

// houseActRightOfHim finds the structure whose footprint is nearest to the
// right of him on screen, and asserts every footprint tile is visible and its
// art is drawn: a box just above the footprint's middle tile is not black.
func houseActRightOfHim(t *testing.T, s *session, img image.Image, px, py int) {
	t.Helper()

	psx, _ := playerScreen(t, s)

	var house []int

	for dx := 2; dx <= 8 && house == nil; dx++ {
		for dy := -4; dy <= 2 && house == nil; dy++ {
			pr := probeAt(s, px+dx, py+dy)
			sx, _ := probeScreen(pr)

			if fp, ok := pr["structure"].([]any); ok && len(fp) == 4 && sx > psx+120 {
				house = make([]int, 4)
				for i := range fp {
					v, _ := fp[i].(float64)
					house[i] = int(v)
				}
			}
		}
	}

	if house == nil {
		t.Fatal("act 5: no structure stands to his right")
	}

	for ty := house[1]; ty < house[3]; ty++ {
		for tx := house[0]; tx < house[2]; tx++ {
			if st := str(probeAt(s, tx, ty), "state"); st != "visible" {
				t.Fatalf("act 5: the house %v to his right has tile (%d,%d) %s; a house seen at all is seen whole",
					house, tx, ty, st)
			}
		}
	}

	mid := probeAt(s, (house[0]+house[2])/2, (house[1]+house[3])/2)
	mx, my := probeScreen(mid)
	lum := meanLum(img, image.Rect(mx-4, my-24, mx+5, my-15))
	t.Logf("act 5: the house %v to his right: every tile visible; its art above the middle tile (%d,%d) on screen has mean %.1f",
		house, mx, my-20, lum)

	if lum < 15 {
		t.Fatalf("act 5: the house %v to his right is not drawn: the art above its middle tile at screen (%d,%d) is black (%.1f)",
			house, mx, my-20, lum)
	}
}

func fogState(s *session) map[string]any {
	return sub(s.call("strigoi_get_system_state", map[string]any{"system": "fog"}), "state")
}

func playerTile(s *session) (int, int) {
	p := s.call("strigoi_get_player", map[string]any{})

	return int(math.Floor(num(p, "x"))), int(math.Floor(num(p, "y")))
}

// probeAt asks the fog about one tile: its state, the line from his eye, and
// where its centre is on screen.
func probeAt(s *session, x, y int) map[string]any {
	setField(s, "fog", "probe", map[string]any{"x": x, "y": y})

	return sub(fogState(s), "probe")
}

// findProbe probes the tiles about d tiles from (x, y), in sixteen
// directions, and returns the first the predicate accepts.
func findProbe(t *testing.T, s *session, x, y int, d float64, ok func(map[string]any) bool) map[string]any {
	t.Helper()

	for i := 0; i < 16; i++ {
		a := 2 * math.Pi * float64(i) / 16
		tx := int(math.Floor(float64(x) + 0.5 + d*math.Cos(a)))
		ty := int(math.Floor(float64(y) + 0.5 + d*math.Sin(a)))

		if tx < 0 || ty < 0 {
			continue
		}

		if p := probeAt(s, tx, ty); ok(p) {
			return p
		}
	}

	return nil
}

// playerScreen is where his feet are on screen.
func playerScreen(t *testing.T, s *session) (int, int) {
	t.Helper()

	x, y := pair(s.call("strigoi_get_player", map[string]any{}), "screen")
	if x == 0 && y == 0 {
		t.Fatal("no player screen position")
	}

	return int(x), int(y)
}

// lowestProbe looks at every tile whose centre is between lo and hi tiles
// from him and, lowest on screen first (inside the play area), returns the
// first whose probe the predicate accepts. Low on screen matters: art stands
// UP from its tile, so only tiles lower still could draw over a tile's centre.
// The screen point is first estimated (the isometric step at the view's
// scale) to order the tiles, then taken from the probe, the engine's own.
func lowestProbe(t *testing.T, s *session, lo, hi, scale float64, ok func(map[string]any) bool) map[string]any {
	t.Helper()

	p := s.call("strigoi_get_player", map[string]any{})
	wx, wy := num(p, "x"), num(p, "y")
	psx, psy := playerScreen(t, s)

	type cand struct {
		x, y   int
		sx, sy float64
	}

	var cands []cand

	for ty := int(wy - hi - 1); ty <= int(wy+hi+1); ty++ {
		for tx := int(wx - hi - 1); tx <= int(wx+hi+1); tx++ {
			dx, dy := float64(tx)+0.5-wx, float64(ty)+0.5-wy
			if d := math.Hypot(dx, dy); d < lo || d > hi || tx < 0 || ty < 0 {
				continue
			}

			sx := float64(psx) + 80*(dx-dy)*scale
			sy := float64(psy) + 40*(dx+dy)*scale

			if sx >= 30 && sx < 770 && sy >= 30 && sy < 450 && sy > float64(psy) {
				cands = append(cands, cand{tx, ty, sx, sy})
			}
		}
	}

	sort.Slice(cands, func(i, j int) bool { return cands[i].sy > cands[j].sy })

	for _, c := range cands {
		pr := probeAt(s, c.x, c.y)
		sx, sy := probeScreen(pr)

		if sx >= 20 && sx < 780 && sy >= 20 && sy < 460 && ok(pr) {
			return pr
		}
	}

	return nil
}

func clearFromEye(p map[string]any) bool {
	c, _ := p["clear_from"].(map[string]any)
	v, _ := c["s:1"].(bool)

	return v
}

func probeScreen(p map[string]any) (int, int) {
	l, _ := p["screen"].([]any)
	if len(l) != 2 {
		return -1, -1
	}

	x, _ := l[0].(float64)
	y, _ := l[1].(float64)

	return int(x), int(y)
}

func rgbAt(img image.Image, x, y int) (int, int, int) {
	r, g, b, _ := img.At(x, y).RGBA()

	return int(r >> 8), int(g >> 8), int(b >> 8)
}

func meanLum(img image.Image, r image.Rectangle) float64 {
	sum, n := 0.0, 0

	for y := r.Min.Y; y < r.Max.Y; y += 2 {
		for x := r.Min.X; x < r.Max.X; x += 2 {
			cr, cg, cb := rgbAt(img, x, y)
			sum += float64(cr+cg+cb) / 3
			n++
		}
	}

	return sum / float64(n)
}

// saturation is HSV saturation, (max - min) / max.
func saturation(r, g, b int) float64 {
	hi := math.Max(float64(r), math.Max(float64(g), float64(b)))
	lo := math.Min(float64(r), math.Min(float64(g), float64(b)))

	if hi == 0 {
		return 0
	}

	return (hi - lo) / hi
}

// fogWalkAway walks him from (x0, y0) until he is at least d tiles from it,
// trying sixteen headings, and says how far he got. Paused: the walk is
// stepped.
func fogWalkAway(t *testing.T, s *session, x0, y0, d float64) float64 {
	t.Helper()

	best := 0.0

	// Up the screen first (toward lower x and y), so the ground he leaves is
	// below him on screen, where act 2 looks for it.
	for i := 0; i < 16 && best < d; i++ {
		a := 2 * math.Pi * float64((10+i*5)%16) / 16
		gx, gy := x0+(d+1)*math.Cos(a), y0+(d+1)*math.Sin(a)

		s.call("strigoi_move_player_to", map[string]any{"x": gx, "y": gy})

		for k := 0; k < 120; k++ {
			s.call("strigoi_step", map[string]any{"frames": 6})

			p := s.call("strigoi_get_player", map[string]any{})
			best = math.Hypot(num(p, "x")-x0, num(p, "y")-y0)

			if best >= d || math.Hypot(num(p, "x")-gx, num(p, "y")-gy) < 0.5 {
				break
			}
		}
	}

	if best < d {
		t.Fatalf("could not walk %.1f tiles from (%.1f,%.1f): got %.1f", d, x0, y0, best)
	}

	return best
}

// farVillager is the handle of a villager more than d tiles from him, and
// how far.
func farVillager(t *testing.T, s *session, d float64) (string, float64) {
	t.Helper()

	p := s.call("strigoi_get_player", map[string]any{})
	px, py := num(p, "x"), num(p, "y")

	for _, raw := range asList(s.call("strigoi_get_entities", map[string]any{"kind": "npc", "limit": 200})["items"]) {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		if dist := math.Hypot(num(row, "x")-px, num(row, "y")-py); dist > d {
			return str(row, "handle"), dist
		}
	}

	return "", 0
}

// walkNearFog walks him to within r tiles of an entity, which may wander.
func walkNearFog(t *testing.T, s *session, handle string, r float64) {
	t.Helper()

	for k := 0; k < 40; k++ {
		e := s.call("strigoi_get_entity", map[string]any{"handle": handle})
		p := s.call("strigoi_get_player", map[string]any{})

		if math.Hypot(num(e, "x")-num(p, "x"), num(e, "y")-num(p, "y")) <= r {
			return
		}

		s.call("strigoi_move_player_to", map[string]any{"x": num(e, "x") + 1, "y": num(e, "y") + 1})
		s.call("strigoi_step", map[string]any{"frames": 30})
	}

	t.Fatalf("could not walk within %.1f tiles of %s", r, handle)
}

// TestZoomInToTwo is the zoom-in's script (1 Oct 2026; Josh: "Raise the limit
// a bit, being able to focus down on things is a helpful thing to those of us
// who need eyes."): with fog off, ui.zoom takes 2.0 and refuses 2.1, the map
// is drawn at 2.0, and the camera stays on him -- his feet are where they
// were on screen at 1.0. The frame is Josh's evidence of the zoom-in.
//
// Negative control (1 Oct 2026, as a build overlay): put gameZoomMax back at
// 1.0 and this fails, "ui.zoom: zoom 2 is outside the game's range 0.4..1"
// (strigoi-harness-runs\wt-fog\nc\ncp1-zoom-max-back-at-1.txt).
func TestZoomInToTwo(t *testing.T) {
	s := startGame(t)

	s.call("strigoi_pause", map[string]any{})
	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Near", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	s.call("strigoi_step_world", map[string]any{"world_minutes": 12*60 - 165.0})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if flag(t, fogState(s), "enabled") {
		t.Fatal("fog is on without -fog")
	}

	x1, y1 := playerScreen(t, s)

	setField(s, "ui", "zoom", 2.0)
	s.call("strigoi_step", map[string]any{"frames": 30}) // the camera eases onto him

	if got := mustNum(t, uiState(s), "view_scale"); got != 2 {
		t.Fatalf("ui.zoom 2.0: the map is drawn at %v", got)
	}

	x2, y2 := playerScreen(t, s)
	t.Logf("his feet on screen: (%d,%d) at 1.0, (%d,%d) at 2.0", x1, y1, x2, y2)

	if abs(x2-x1) > 3 || abs(y2-y1) > 3 {
		t.Fatalf("zoomed in to 2.0 he moved on screen from (%d,%d) to (%d,%d); the camera stays on him", x1, y1, x2, y2)
	}

	s.frame(t, "zoom-2-hero") // Josh's evidence: the zoom-in, fog off

	if msg := s.callErr("strigoi_set_system_field", map[string]any{"system": "ui", "field": "zoom", "value": 2.1}); msg == "" {
		t.Fatal("ui.zoom 2.1 was taken; the range stops at 2.0")
	}

	if got := mustNum(t, uiState(s), "view_scale"); got != 2 {
		t.Fatalf("a refused zoom moved the view to %v", got)
	}
}

// TestFogAtNight is fog of war F2's script: "the night closes it"
// (claude/fog-of-war-build-plan.md §4 F2, acts 5-9, with Josh's rulings of 1
// Oct 2026: Q2 dark sight 1.5 rising to ~4 under a full moon, Q3 lit ground
// seen at any distance, Q5 every squad an eye). UNAIDED: nothing is spawned,
// watched or pursued; the hearth is the light provider's place_source and the
// second squad the meters' squad_add, the plan's own verbs.
//
//  5. Deep night, new moon, no torch: a tile 4 away he saw at noon is
//     remembered, not seen; he sees 1.5 tiles. 5b: a full moon widens it to
//     4 (a tile 3 away is seen), a new moon closes it again.
//  6. His torch lit: the tile 4 away is seen; the tile 6 away is not.
//  7. A hearth 10.5-13 tiles off in the dark, with a clear line: its ground is
//     seen from where he stands; the dark tile between is not.
//  8. A second squad 11 tiles off is an eye of its own.
//  9. Hidden is hidden in the HUD: a villager on remembered ground is not
//     shown, not hovered and carries no bar -- with the control that the same
//     villager, seen (the dark radius opened), IS hovered.
//
// The provider is the primary evidence; the frames are Josh's (zoom 0.5).
func TestFogAtNight(t *testing.T) {
	const dawnMinute = 165.0

	s := startGame(t, "-fog", "-zoom", "0.5")

	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Night", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	t.Logf("spawned at %v", game["spawn_tile"])

	setField(s, "rising", "p", 0.0)
	setField(s, "rising", "edge_floor", 0)
	setField(s, "spawns", "chance", 0)

	// Noon: he sees his 12 tiles.
	s.call("strigoi_step_world", map[string]any{"world_minutes": 12*60 - dawnMinute})
	s.call("strigoi_step", map[string]any{"frames": 2})

	px, py := playerTile(s)

	line := clearLineOnScreen(t, s, px, py, []float64{3, 4, 6})
	if line == nil {
		t.Fatal("no direction from him has a clear line 6 tiles out on screen")
	}

	at3, at4, at6 := line[0], line[1], line[2]
	for _, p := range line {
		if st := str(p, "state"); st != "visible" {
			t.Fatalf("at noon the tile (%v,%v) on a clear line is %s", p["x"], p["y"], st)
		}
	}

	// --- act 5: deep night, new moon, no torch --------------------------------
	s.call("strigoi_step_world", map[string]any{"world_minutes": 11 * 60}) // 23:00
	setField(s, "clock", "moon", 0.0)
	s.call("strigoi_step", map[string]any{"frames": 2})

	fog := fogState(s)
	t.Logf("act 5: 23:00, new moon: tonight_dark %v, unlit_reach %v, %v visible, %v explored, draws_by %v",
		fog["tonight_dark"], fog["unlit_reach"], fog["visible"], fog["explored"], fog["draws_by"])

	if mustNum(t, fog, "tonight_dark") != 1.5 || mustNum(t, fog, "unlit_reach") != 1.5 {
		t.Fatalf("act 5: at deep night under a new moon he sees %v (dark %v); Josh's Q2 is 1.5",
			fog["unlit_reach"], fog["tonight_dark"])
	}

	if str(fog, "draws_by") != "view" {
		t.Fatalf("act 5: with fog on the renderer draws by %v; want the light view (BUG-110)", fog["draws_by"])
	}

	if st := probeState(s, at4); st != "explored" {
		t.Fatalf("act 5: the tile 4 away (%v,%v), seen at noon, is %s at deep night; want explored", at4["x"], at4["y"], st)
	}

	s.frame(t, "fog-n5-deep-night-new-moon") // Josh's: deep night, no torch, new moon

	// 5b: the moon.
	setField(s, "clock", "moon", 1.0)
	s.call("strigoi_step", map[string]any{"frames": 2})

	if got := mustNum(t, fogState(s), "unlit_reach"); got != 4 {
		t.Fatalf("act 5b: under a full moon he sees %v; Josh's Q2 is about 4", got)
	}

	if st := probeState(s, at3); st != "visible" {
		t.Fatalf("act 5b: under a full moon the tile 3 away (%v,%v) is %s", at3["x"], at3["y"], st)
	}

	s.frame(t, "fog-n5b-deep-night-full-moon") // Josh's: full moon

	setField(s, "clock", "moon", 0.0)
	s.call("strigoi_step", map[string]any{"frames": 2})

	if st := probeState(s, at3); st != "explored" {
		t.Fatalf("act 5b: back under a new moon the tile 3 away is %s; want explored", st)
	}

	// --- act 6: his torch -----------------------------------------------------
	setField(s, "light", "carried_source", "torch")
	s.call("strigoi_step", map[string]any{"frames": 2})

	p4, p6 := probeAt(s, int(num(at4, "x")), int(num(at4, "y"))), probeAt(s, int(num(at6, "x")), int(num(at6, "y")))
	t.Logf("act 6: torch lit: the tile 4 away is %s (lit %v), the tile 6 away %s (lit %v)",
		str(p4, "state"), p4["lit"], str(p6, "state"), p6["lit"])

	if str(p4, "state") != "visible" || !flag(t, p4, "lit") {
		t.Fatalf("act 6: with his torch lit the tile 4 away is %s (lit %v); the torch's ground is seen", str(p4, "state"), p4["lit"])
	}

	if str(p6, "state") != "explored" || flag(t, p6, "lit") {
		t.Fatalf("act 6: with his torch lit the tile 6 away is %s (lit %v); past the torch it is dark", str(p6, "state"), p6["lit"])
	}

	s.frame(t, "fog-n6-torch") // Josh's: the torch

	setField(s, "light", "carried_source", "")

	// --- act 7: a hearth seen from afar --------------------------------------
	hearth, between := hearthSpot(t, s, px, py)
	if hearth == nil {
		// At 0.5 the screen reaches ~10.6 tiles up and down and ~12.7 to the
		// corners; if no such spot is on screen, look from 0.4 (zoom is not in
		// fog's rule, act 1b).
		setField(s, "ui", "zoom", 0.4)
		s.call("strigoi_step", map[string]any{"frames": 30})

		hearth, between = hearthSpot(t, s, px, py)
	}

	if hearth == nil {
		t.Fatal("act 7: no spot 10.5-13 tiles off with a clear line and on screen for a hearth, at 0.5 or 0.4")
	}

	hx, hy := num(hearth, "x"), num(hearth, "y")
	setField(s, "light", "place_source", map[string]any{"kind": "hearth", "x": hx + 0.5, "y": hy + 0.5})
	s.call("strigoi_step", map[string]any{"frames": 2})

	ph := probeAt(s, int(hx), int(hy))
	pb := probeAt(s, int(num(between, "x")), int(num(between, "y")))
	fog = fogState(s)
	t.Logf("act 7: the hearth at (%v,%v), %.1f tiles off: %s (lit %v); between (%v,%v): %s (lit %v); lit_seen %v",
		hx, hy, math.Hypot(hx-float64(px), hy-float64(py)), str(ph, "state"), ph["lit"],
		between["x"], between["y"], str(pb, "state"), pb["lit"], fog["lit_seen"])

	if str(ph, "state") != "visible" {
		t.Fatalf("act 7: the hearth's own tile with a clear line is %s; lit ground is seen from afar (Q3)", str(ph, "state"))
	}

	if str(pb, "state") == "visible" {
		t.Fatalf("act 7: the dark tile between, (%v,%v), is visible", between["x"], between["y"])
	}

	if mustNum(t, fog, "lit_seen") == 0 {
		t.Fatal("act 7: the provider counts no tile seen by the lit term")
	}

	// The OS cursor is drawn over the frame wherever the mouse is: park it.
	s.call("strigoi_move_cursor", map[string]any{"x": 6, "y": 594})
	s.call("strigoi_step", map[string]any{"frames": 2})

	img := s.frame(t, "fog-n7-hearth-afar") // Josh's: a hearth seen from afar
	hsx, hsy := probeScreen(ph)
	bsx, bsy := probeScreen(pb)
	hl := meanLum(img, image.Rect(hsx-8, hsy-4, hsx+8, hsy+4))
	bl := meanLum(img, image.Rect(bsx-8, bsy-4, bsx+8, bsy+4))
	t.Logf("act 7: on screen the hearth's ground at (%d,%d) is %.1f, the dark between at (%d,%d) %.1f", hsx, hsy, hl, bsx, bsy, bl)

	if hl < 10 {
		t.Logf("act 7: the hearth's ground is drawn black (%.1f) -- a black-floor launch (P3 §5.3); the pixels are not asserted", hl)
	} else if hl < bl+15 {
		t.Fatalf("act 7: the hearth's ground (%.1f) is not drawn brighter than the dark between (%.1f)", hl, bl)
	}

	setField(s, "ui", "zoom", 0.5)

	srcs := asList(sub(s.call("strigoi_get_system_state", map[string]any{"system": "light"}), "state")["source_list"])
	for _, raw := range srcs {
		if src, ok := raw.(map[string]any); ok && str(src, "kind") == "hearth" {
			setField(s, "light", "remove_source", num(src, "id"))
		}
	}

	s.call("strigoi_step", map[string]any{"frames": 2})

	// --- act 8: a second squad is an eye --------------------------------------
	if st := probeState(s, hearth); st == "visible" {
		t.Fatalf("act 8: the control: with the hearth gone its tile is still %s", st)
	}

	setField(s, "meters", "squad_add", map[string]any{"x": hx + 0.5, "y": hy + 0.5})
	s.call("strigoi_step", map[string]any{"frames": 2})

	fog = fogState(s)
	eyes := asList(fog["eyes"])
	t.Logf("act 8: %d eyes: %v", len(eyes), eyes)

	if len(eyes) != 2 {
		t.Fatalf("act 8: with a second squad there are %d eyes; every squad model is one", len(eyes))
	}

	if st := probeState(s, hearth); st != "visible" {
		t.Fatalf("act 8: the second squad's own tile (%v,%v) is %s", hx, hy, st)
	}

	s.frame(t, "fog-n8-second-squad")

	// --- act 9: hidden is hidden in the HUD ----------------------------------
	handle, vid := villagerInMemory(t, s)
	if handle == "" {
		t.Fatal("act 9: no villager on remembered ground (explored, not seen) within 2.5..9 tiles and on screen")
	}

	ent := s.call("strigoi_get_entity", map[string]any{"handle": handle})
	if shown, ok := ent["shown"].(bool); !ok || shown {
		t.Fatalf("act 9: the villager %s on remembered ground is shown (%v)", handle, ent["shown"])
	}

	label := fogHoverAt(t, s, handle)
	bar := barFor(t, uiState(s), vid, true)
	t.Logf("act 9: the villager %s (%s) on remembered ground: hover %q, bar %v", handle, vid, label, bar != nil)

	if label != "" {
		t.Fatalf("act 9: the cursor on the hidden villager %s hovers %q; what he does not see is not named (BUG-107)", handle, label)
	}

	if bar != nil {
		t.Fatalf("act 9: the hidden villager %s carries a bar (BUG-107)", handle)
	}

	// The control: open the dark radius so he is seen; the same hover names him.
	setField(s, "fog", "dark_radius", 30.0)
	s.call("strigoi_step", map[string]any{"frames": 2})

	seen := fogHoverAt(t, s, handle)
	t.Logf("act 9: the control, dark radius 30: shown %v, hover %q, bar %v", s.call("strigoi_get_entity", map[string]any{"handle": handle})["shown"],
		seen, barFor(t, uiState(s), vid, true) != nil)

	if seen == "" {
		t.Fatalf("act 9: the control: seen, the villager %s is not hovered either -- the instrument is blind", handle)
	}

	setField(s, "fog", "dark_radius", 1.5)
	s.call("strigoi_step", map[string]any{"frames": 2})
}

func probeState(s *session, p map[string]any) string {
	return str(probeAt(s, int(num(p, "x")), int(num(p, "y"))), "state")
}

// onPlay is whether a probe's tile centre is on screen in the play area.
func onPlay(p map[string]any) bool {
	x, y := probeScreen(p)

	return x >= 40 && x < 760 && y >= 30 && y < 450
}

// clearLineOnScreen tries 32 headings from his tile and returns the probes at
// the given distances along the first heading whose every tile has a clear
// line from him and is on screen.
func clearLineOnScreen(t *testing.T, s *session, px, py int, dists []float64) []map[string]any {
	t.Helper()

	for i := 0; i < 32; i++ {
		a := 2 * math.Pi * float64(i) / 32
		var out []map[string]any

		for _, d := range dists {
			tx := int(math.Floor(float64(px) + 0.5 + d*math.Cos(a)))
			ty := int(math.Floor(float64(py) + 0.5 + d*math.Sin(a)))

			if tx < 0 || ty < 0 {
				break
			}

			p := probeAt(s, tx, ty)
			if !clearFromEye(p) || !onPlay(p) {
				break
			}

			out = append(out, p)
		}

		if len(out) == len(dists) {
			return out
		}
	}

	return nil
}

// hearthSpot finds a tile 10.5..13 tiles from him, on screen, with a clear
// line, whose tile 2.5 out along the same line is clear too and at least 8
// tiles from it: the hearth and the dark ground between (a hearth lights 8
// tiles, and under a new moon ground past ~7.6 is drawn at the night's own
// band, not lit).
func hearthSpot(t *testing.T, s *session, px, py int) (hearth, between map[string]any) {
	t.Helper()

	for _, d := range []float64{13, 12, 11.5, 11, 10.5} {
		for i := 0; i < 32; i++ {
			a := 2 * math.Pi * float64(i) / 32
			tx := int(math.Floor(float64(px) + 0.5 + d*math.Cos(a)))
			ty := int(math.Floor(float64(py) + 0.5 + d*math.Sin(a)))

			if tx < 0 || ty < 0 {
				continue
			}

			h := probeAt(s, tx, ty)
			if !clearFromEye(h) || !onPlay(h) || h["structure"] != nil {
				continue
			}

			bx := int(math.Floor(float64(px) + 0.5 + 2.5*math.Cos(a)))
			by := int(math.Floor(float64(py) + 0.5 + 2.5*math.Sin(a)))

			if math.Hypot(float64(bx-tx), float64(by-ty)) < 8 {
				continue
			}

			b := probeAt(s, bx, by)
			if clearFromEye(b) && onPlay(b) {
				return h, b
			}
		}
	}

	return nil, nil
}

// villagerInMemory is a villager standing on remembered ground -- the fog
// probe says his tile is explored and not visible (plan §3.11: prove the
// villager is hidden for the right reason before asserting the HUD hides
// him) -- 2.5..9 tiles off and on screen; his handle and id.
func villagerInMemory(t *testing.T, s *session) (handle, id string) {
	t.Helper()

	p := s.call("strigoi_get_player", map[string]any{})
	px, py := num(p, "x"), num(p, "y")

	for _, raw := range asList(s.call("strigoi_get_entities", map[string]any{"kind": "npc", "limit": 200})["items"]) {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		if d := math.Hypot(num(row, "x")-px, num(row, "y")-py); d < 2.5 || d > 9 {
			continue
		}

		pr := probeAt(s, int(math.Floor(num(row, "x"))), int(math.Floor(num(row, "y"))))
		if str(pr, "state") != "explored" || !onPlay(pr) {
			continue
		}

		e := s.call("strigoi_get_entity", map[string]any{"handle": str(row, "handle")})

		return str(row, "handle"), str(e, "id")
	}

	return "", ""
}

// fogHoverAt puts the cursor just above the entity's feet and says what the HUD
// names there.
func fogHoverAt(t *testing.T, s *session, handle string) string {
	t.Helper()

	sx, sy := screenOf(t, s, handle)
	s.call("strigoi_move_cursor", map[string]any{"x": sx, "y": sy - 8})
	s.call("strigoi_step", map[string]any{"frames": 2})

	return str(uiState(s), "hover_label")
}

// TestFogNeverTouchesTheSim (plan §3.3; strengthened by the F2 review's B5
// and B6): the same seed and the same scripted night -- a torch-lit walk at
// 23:00, then a FIGHT under the shipped tactical layer (a zombie three tiles
// off, set to watch him; 600 frames of it, his turn held as the shipped game
// holds it) with a second zombie standing out in the dark --
// with fog on and with fog off give the same world: EVERY system's world
// hash agrees (fog's included; the ui's too, now that the bars are in the
// process part), every digest part but the process part agrees, and the ui
// itself, less only `bars` and `hover_label` (what the HUD drew), is equal
// field for field. With fog on the fight's enemies were contacts (fog was
// engaged, the control that the run is not vacuous).
//
// The unit control is d2world's TestFogNeverTouchesTheSim (the plan's
// one-liner, fog calling the light model's SetPlayer, goes red there).
func TestFogNeverTouchesTheSim(t *testing.T) {
	on := nightDigest(t, true)
	off := nightDigest(t, false)

	differ := []string{}

	for name, h := range on.systems {
		if off.systems[name] != h {
			differ = append(differ, name)
		}
	}

	sort.Strings(differ)
	t.Logf("systems whose world hash differs with fog on and off: %v", differ)
	t.Logf("bars drawn at the end: %d with fog, %d without; contacts seen during the fight with fog: %d",
		on.bars, off.bars, on.contacts)

	if len(differ) != 0 {
		t.Errorf("the world differs with fog on and off in %v: fog touched the sim (or reached a world part)", differ)
	}

	for part, h := range on.parts {
		if part != "process" && off.parts[part] != h {
			t.Errorf("the digest's %s part differs with fog on and off", part)
		}
	}

	for k, v := range on.ui {
		if fmt.Sprint(off.ui[k]) != fmt.Sprint(v) {
			t.Errorf("the ui's %s differs with fog on and off: %v / %v", k, v, off.ui[k])
		}
	}

	if on.walked != off.walked || on.fought != off.fought {
		t.Errorf("the runs differ: walked %v / %v, fought %v / %v", on.walked, off.walked, on.fought, off.fought)
	}

	if !on.fought {
		t.Fatal("no fight opened: the scripted fight is the point of this test")
	}

	if on.contacts == 0 {
		t.Fatal("the control: with fog on, no enemy of his fight was ever a contact -- fog was not engaged")
	}
}

type nightRun struct {
	systems  map[string]any
	parts    map[string]any
	ui       map[string]any
	walked   [2]float64
	fought   bool
	contacts int
	bars     int
}

func nightDigest(t *testing.T, fog bool) nightRun {
	t.Helper()

	args := []string{"-zoom", "0.5"}
	if fog {
		args = append(args, "-fog")
	}

	s := startGame(t, args...)

	s.call("strigoi_pause", map[string]any{})
	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Same", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})

	// Nothing comes for him before the scripted fight: no pack, and the dead
	// stay down (without this the risen killed him before 23:00, and both
	// launches agreed on a dead man -- the first draft of this test).
	setField(s, "spawns", "chance", 0)
	setField(s, "rising", "p", 0.0)
	setField(s, "rising", "edge_floor", 0)
	s.call("strigoi_step_world", map[string]any{"world_minutes": 23*60 - 165.0})

	if hp := mustNum(t, metersState(s), "health"); hp <= 0 {
		t.Fatalf("he is dead (health %v) before the scripted fight", hp)
	}
	setField(s, "light", "carried_source", "torch")
	s.call("strigoi_move_cursor", map[string]any{"x": 6, "y": 594})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if got := flag(t, fogState(s), "enabled"); got != fog {
		t.Fatalf("fog enabled %v, want %v", got, fog)
	}

	var run nightRun

	for _, d := range [][2]float64{{6, 0}, {-6, 0}, {0, 6}, {0, -6}} {
		p := s.call("strigoi_get_player", map[string]any{})
		fromX, fromY := num(p, "x"), num(p, "y")

		res := s.call("strigoi_move_player_to", map[string]any{
			"x": fromX + d[0], "y": fromY + d[1], "wait": true, "max_ticks": 900,
		})

		if x, y := pair(res, "position_tile"); math.Hypot(x-fromX, y-fromY) >= 2 {
			run.walked = [2]float64{x, y}
			break
		}
	}

	// The fight: a zombie three tiles off watching him, and a second standing
	// nine tiles off in the dark, outside his torch. The SHIPPED screen's
	// tactical layer (the launcher drops every script to policy, where a
	// fight opens only at adjacency): the fight opens at the engage radius,
	// as TestTacticalFight's does.
	setField(s, "combat", "player_control", "human")

	pl := s.call("strigoi_get_player", map[string]any{})
	px, py, handle := num(pl, "x"), num(pl, "y"), str(pl, "handle")

	near := clearSpotAt(t, s, px, py, 3)
	enemy := spawnNPC(t, s, "zombie1", near[0], near[1])

	if far := farSpot(s, px, py, 9); far != nil {
		spawnNPC(t, s, "zombie1", far[0], far[1])
	}

	s.call("strigoi_watch", map[string]any{"watcher": enemy, "target": handle})

	for i := 0; i < 20; i++ {
		s.call("strigoi_step", map[string]any{"frames": 30})

		if flag(t, combatState(s), "fighting") {
			run.fought = true

			if fog {
				run.contacts = max(run.contacts, len(asList(fogState(s)["contacts"])))
			}
		} else if run.fought {
			break
		}
	}

	s.call("strigoi_move_cursor", map[string]any{"x": 6, "y": 594})
	s.call("strigoi_step", map[string]any{"frames": 2})

	out := s.call("strigoi_get_state_digest", map[string]any{})
	run.systems, run.parts = sub(out, "systems"), sub(out, "parts")

	run.ui = uiState(s)
	run.bars = len(asList(run.ui["bars"]))
	delete(run.ui, "bars")
	delete(run.ui, "hover_label")

	s.stop()

	return run
}

// farSpot is a point d tiles from (x, y) on an orthogonal bearing with a
// clear straight line, or nil.
func farSpot(s *session, x, y, d float64) []float64 {
	for _, dir := range [][2]float64{{0, 1}, {-1, 0}, {0, -1}, {1, 0}} {
		tx, ty := x+dir[0]*d, y+dir[1]*d
		if b, _ := s.call("strigoi_find_path", map[string]any{"to_x": tx, "to_y": ty})["straight_line_clear"].(bool); b {
			return []float64{tx, ty}
		}
	}

	return nil
}
