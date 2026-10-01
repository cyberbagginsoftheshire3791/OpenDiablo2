//go:build playtest

package playtest

import (
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

	s.frame(t, "fog-1-day-start") // Josh's evidence: black beyond what he has seen

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

	s.frame(t, "fog-2-after-walk") // Josh's evidence: the remembered ground behind him

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
