//go:build playtest

package playtest

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2save"
)

// THE OMIT SWEEP (the build plan's section 4 act 9; M4.6 B6, 2 Oct 2026):
// every top-level block of the world file, dropped in turn from the hunted
// night's file at T, and the file resumed. Each must make the load either
// DIVERGE from the saved moment (S_R0 != S_T, where the block shows: the part
// or system of the digest that reports it) or REFUSE the file with the right reason (the load report's code
// and, for a refusal of the file itself, d2save's rule). "A block whose
// omission doesn't diverge is a hole in observability, not a pass."
//
// The rows come from d2save.Blocks, so a block added to the file cannot be
// forgotten: every block gets an OMITTED row (its key gone, as
// strigoi_save_game{omit} writes it) by construction, and
// TestTheOmitSweepCoversEveryBlock fails while any block has no EMPTIED row
// (its content taken out, or for the one block a quiet evening leaves empty,
// filled). The two halves prove different things (the B4b review's C6,
// BUG-83):
//
//   - OMITTED: d2save.Decode refuses a missing block before the load's own
//     steps run (block-missing; version's is a VERSION refusal), so these rows
//     pin Decode's rule and the game's refusal path -- code, rule, set aside,
//     dawn -- not the load's restore.
//   - EMPTIED: the file differs from T's in that block alone. A load is
//     deterministic, so if it never read the block the emptied file would
//     resume exactly as the untouched one does -- S_T, which the control of
//     the controls shows first. Divergence therefore proves the load READS the
//     block; where the file's own checks refuse the emptied block, the row
//     names the rule that does.
//
// One block is TOLERATED by design, and its row says so and holds it: build is
// informational (World.Build: a load does not refuse another build's file of
// the same version), so emptying it resumes T exactly. Omitting it is still
// refused. Should a load ever start to read it, the row goes red and is
// re-decided.
//
// THE SWEEP RUNS IN THE SUITE as TestSaveResume's act 9, on the evening acts
// 1-3 built, in the session acts 4-6i used: the evening is built once per
// suite run, not per block, and one game at a time is loaded, so it is safe
// under the suite's -parallel. Each load is a game start (about 1.3 s on the
// laptop alone), each with the harness's 90 s wait.
//
// TestSaveResumeNegatives is the same sweep against an evening a green
// TestSaveResume kept (STRIGOI_SAVE_RESUME_FROM), for the controls: a minute,
// not four.
func TestSaveResumeNegatives(t *testing.T) {
	from := os.Getenv("STRIGOI_SAVE_RESUME_FROM")
	if from == "" {
		t.Skip("the omit sweep runs in the suite as TestSaveResume's act 9; set STRIGOI_SAVE_RESUME_FROM " +
			"to an evening a green TestSaveResume kept to run it alone")
	}

	ev := keptEvening(t, from)

	s := start(t)
	s.call("strigoi_pause", map[string]any{})
	eveningHarness(t, s, ev) // a stale evening is refused, not compared (BUG-93)

	omitSweep(t, s, ev)
}

// sweepWant is what a row's resume must show.
type sweepWant int

const (
	// wantDiverge: the file resumes, and is not the saved moment -- in one of
	// the row's moves when it names any.
	wantDiverge sweepWant = iota
	// wantRefused: the load refuses the file with the row's code (and rule).
	wantRefused
	// wantSame: the block is not read, by design (the row's why); the file
	// resumes the saved moment exactly.
	wantSame
)

// sweepRow is one variant of T's file: a block dropped one way.
type sweepRow struct {
	block string // the top-level block (d2save.Blocks) the row drops
	name  string
	edit  func(t *testing.T, file map[string]any)
	want  sweepWant

	// code and rule: wantRefused's load report refused and rule (rule ""
	// for a refusal that is not d2save's -- a system's Validate, BLOCK).
	code, rule string

	// moves: wantDiverge's -- at least one of these ("part X", "system Y")
	// differs from S_T. Empty: any divergence will do.
	moves []string

	// why: wantSame's reason the block is not read.
	why string

	// twin, when set, is a second edit of T's file the row is judged
	// AGAINST instead of S_T (the B6 review's M7): a field whose effect is
	// spent on the first frame -- scene.last_stage, watch_clock_set -- shows
	// only as a difference between two files that differ in it alone, at a
	// moment where it matters. The twin must resume.
	twin func(t *testing.T, file map[string]any)

	// check, when set, runs after a row that passed: an assertion of its own
	// on the resumed game (the fog grid, bit for bit). twinFog is the fog
	// provider's state right after the twin's load (nil for a row with no
	// twin).
	check func(t *testing.T, s *session, file, twinFog map[string]any) error
}

// omitRows are the omitted half: one per block of d2save.Blocks, built from
// it, so no block can lack one.
func omitRows() []sweepRow {
	rows := make([]sweepRow, 0, len(d2save.Blocks))

	for _, b := range d2save.Blocks {
		block := b
		r := sweepRow{
			block: block, name: "omit " + block, want: wantRefused,
			code: "FILE", rule: d2save.ReasonBlockMissing,
			edit: func(_ *testing.T, file map[string]any) { delete(file, block) },
		}

		// Decode reads the version before any block: a file without one is
		// of no version this build reads (a *VersionError, "absent").
		if block == "version" {
			r.code, r.rule = "VERSION", d2save.ReasonVersion
		}

		rows = append(rows, r)
	}

	return rows
}

// emptyRows are the emptied half (and two of the B4b brief's own variants,
// under entities). TestTheOmitSweepCoversEveryBlock holds every block to at
// least one. Each was measured on the hunted evening (2 Oct 2026; the notes'
// "B6") before its expectation was written, and each expectation is the
// block's own: a refusal names the rule that guards that block, a divergence
// where the block shows (the part or system of the digest that reports it).
func emptyRows() []sweepRow {
	set := func(block, field string, v any) func(*testing.T, map[string]any) {
		return func(t *testing.T, file map[string]any) { blockOf(t, file, block)[field] = v }
	}

	return []sweepRow{
		{block: "version", name: "empty version (0)", edit: func(_ *testing.T, f map[string]any) { f["version"] = json.Number("0") },
			want: wantRefused, code: "VERSION", rule: d2save.ReasonVersion},
		{block: "build", name: "empty build", edit: func(_ *testing.T, f map[string]any) { f["build"] = "" },
			want: wantSame, why: "informational: a load does not refuse another build's file of the same version (World.Build)"},
		{block: "saved_at", name: "empty saved_at", edit: func(_ *testing.T, f map[string]any) { f["saved_at"] = "" },
			want: wantRefused, code: "FILE", rule: d2save.ReasonGeneration},
		{block: "map", name: "empty map", edit: func(_ *testing.T, f map[string]any) {
			f["map"] = map[string]any{"path": "", "sha": "", "generated": false}
		}, want: wantRefused, code: "FILE", rule: d2save.ReasonMap},
		{block: "seed", name: "empty seed (0)", edit: func(_ *testing.T, f map[string]any) { f["seed"] = "0" },
			want: wantRefused, code: "FILE", rule: d2save.ReasonRNGStream},
		{block: "rng", name: "empty rng.world (no draws)", edit: func(t *testing.T, f map[string]any) {
			blockOf(t, blockOf(t, f, "rng"), "world")["draws"] = json.Number("0")
		}, want: wantDiverge, moves: []string{"part rng"}},
		{block: "rng", name: "empty rng.uuid (no bytes)", edit: func(t *testing.T, f map[string]any) {
			blockOf(t, blockOf(t, f, "rng"), "uuid")["bytes"] = json.Number("0")
		}, want: wantDiverge, moves: []string{"system uuid"}},
		{block: "hero", name: "empty hero (facing, wind, run)", edit: func(t *testing.T, f map[string]any) {
			h := blockOf(t, f, "hero")
			h["facing"], h["stamina"], h["run"] = json.Number("0"), json.Number("0"), false
		}, want: wantDiverge, moves: []string{"part entities"}},
		// The sidecar is one block of five documents, each bound by its own
		// code (bindKit, bindProgress, bindStanding, bindLand, bindJournal), so
		// each is emptied on its own. The kit's row was the sweep's first find:
		// no provider reported the kit, and it resumed the saved moment
		// (BUG-113; the "kit" provider since).
		{block: "sidecar", name: "empty sidecar.kit.pack", edit: func(t *testing.T, f map[string]any) {
			blockOf(t, blockOf(t, f, "sidecar"), "kit")["pack"] = []any{}
		}, want: wantDiverge, moves: []string{"system kit"}},
		{block: "sidecar", name: "empty sidecar.progress (xp, talents)", edit: func(t *testing.T, f map[string]any) {
			p := blockOf(t, blockOf(t, f, "sidecar"), "progress")
			p["xp"], p["talents"] = json.Number("0"), []any{}
		}, want: wantDiverge, moves: []string{"system progress"}},
		{block: "sidecar", name: "empty sidecar.village (rep, flags)", edit: func(t *testing.T, f map[string]any) {
			v := blockOf(t, blockOf(t, f, "sidecar"), "village")
			v["rep"], v["flags"] = json.Number("0"), []any{}
		}, want: wantDiverge, moves: []string{"system village"}},
		{block: "sidecar", name: "empty sidecar.land (gathered)", edit: func(t *testing.T, f map[string]any) {
			blockOf(t, blockOf(t, f, "sidecar"), "land")["gathered"] = json.Number("0")
		}, want: wantDiverge, moves: []string{"system village"}},
		{block: "sidecar", name: "empty sidecar.journal (written)", edit: func(t *testing.T, f map[string]any) {
			blockOf(t, blockOf(t, f, "sidecar"), "journal")["written"] = map[string]any{}
		}, want: wantDiverge, moves: []string{"system journal"}},
		{block: "clock", name: "empty clock (elapsed 0)", edit: set("clock", "elapsed", json.Number("0")),
			want: wantDiverge, moves: []string{"system clock"}},
		{block: "light", name: "empty light.sources", edit: set("light", "sources", []any{}),
			want: wantDiverge, moves: []string{"system light"}},
		{block: "squads", name: "empty squads-deployed", edit: func(t *testing.T, f map[string]any) {
			sq := blockOf(t, f, "squads")
			sq["squads"] = asList(sq["squads"])[:1]
			sq["selected"] = "s:1"
		}, want: wantDiverge, moves: []string{"system meters"}},
		{block: "spawns", name: "empty spawns.groups", edit: set("spawns", "groups", []any{}),
			want: wantDiverge, moves: []string{"system spawns"}},
		{block: "spawner", name: "empty spawner.arrival", edit: set("spawner", "arrival", json.Number("0")),
			want: wantDiverge, moves: []string{"system scene"}},
		{block: "notice", name: "empty notice.watches", edit: set("notice", "watches", []any{}),
			want: wantDiverge, moves: []string{"system spawns"}},
		{block: "pursuit", name: "empty pursuit.chases", edit: set("pursuit", "chases", []any{}),
			want: wantDiverge, moves: []string{"system pursuit"}},
		{block: "seek", name: "empty seek.rows", edit: set("seek", "rows", []any{}),
			want: wantDiverge, moves: []string{"system seek"}},
		{block: "corpses", name: "empty corpses", edit: func(t *testing.T, f map[string]any) {
			c := blockOf(t, f, "corpses")
			c["bodies"], c["risen_as"], c["walker"], c["last"] = []any{}, map[string]any{}, map[string]any{}, map[string]any{}
		}, want: wantDiverge, moves: []string{"system corpses"}},
		{block: "rising", name: "empty rising (pressure and counts)", edit: func(t *testing.T, f map[string]any) {
			r := blockOf(t, f, "rising")
			for _, k := range []string{"pressure_accrued", "last_band", "rolls", "risen", "stood_again", "wandered"} {
				r[k] = json.Number("0")
			}
		}, want: wantDiverge, moves: []string{"system rising"}},
		// Every count of his fights taken back to none -- next_id with them:
		// combat's Validate holds it one past the fights started (measured:
		// counts zeroed under next_id 3 are refused BLOCK).
		{block: "combat", name: "empty combat (its counts)", edit: func(t *testing.T, f map[string]any) {
			c := blockOf(t, f, "combat")
			for k, v := range c {
				if _, isNum := v.(json.Number); isNum {
					c[k] = json.Number("0")
				}
			}
			c["next_id"] = json.Number("1")
		}, want: wantDiverge, moves: []string{"system combat"}},
		{block: "bodies", name: "empty bodies", edit: func(_ *testing.T, f map[string]any) { f["bodies"] = []any{} },
			want: wantDiverge, moves: []string{"system combat"}},
		{block: "entities", name: "empty entities (non-native)", edit: func(t *testing.T, f map[string]any) {
			kept := []any{}
			for _, raw := range asList(f["entities"]) {
				if e, _ := raw.(map[string]any); e["native"] == true {
					kept = append(kept, e)
				}
			}
			f["entities"] = kept
		}, want: wantRefused, code: "FILE", rule: d2save.ReasonBodyOrphan},
		{block: "entities", name: "swap two entities' ids", edit: swapTwoEntities,
			want: wantRefused, code: "FILE", rule: d2save.ReasonMemberPlace},
		{block: "scene", name: "empty scene (watch stood, field dead)", edit: func(t *testing.T, f map[string]any) {
			sc := blockOf(t, f, "scene")
			sc["watch_stood"], sc["field_dead"] = json.Number("0"), []any{}
		}, want: wantDiverge, moves: []string{"system scene"}},
		// The hunted evening was played without fog until F5 (2 Oct 2026), so its
		// grid was empty and an emptied block would be T's file: the row FILLS
		// it instead -- with
		// an UNEVEN pattern (a corner block and a scatter), and the resumed
		// grid must be the file's bit for bit (the B6 review's B1: an
		// all-explored grid is its own mirror, so a load that laid the tiles
		// out reversed passed). Emptying a grid he had is TestFogIsKept's act 5.
		//
		// Its TWIN is the same file with a grid of the same map and size and
		// no tile explored (the F5 review's C1): with fog on, a load re-sees
		// from where he stands, so the twin's resumed grid is exactly what
		// the first frames see, and the filled file must resume as the fill
		// OR that, bit for bit. Both are read two frames after their loads.
		{block: "fog", name: "fill fog (a corner block and a scatter)", edit: fillFog, twin: zeroFog,
			want: wantDiverge, moves: []string{"system fog"}, check: fogGridIsTheFiles},
		// The raid's R3a (version 6): every house's stock taken to none. The
		// village's houses are the map's; their stock is the file's alone, and
		// the households provider reports it.
		{block: "households", name: "empty households (every house's stock 0)", edit: func(t *testing.T, f map[string]any) {
			for _, raw := range asList(blockOf(t, f, "households")["houses"]) {
				h, _ := raw.(map[string]any)
				h["incense"], h["stakes"] = json.Number("0"), json.Number("0")
			}
		}, want: wantDiverge, moves: []string{"system households"}},
	}
}

// perturbRows are rows that put a value T's evening does not have -- not
// empty, not the default -- where a load that dropped or defaulted the field
// would resume the saved moment (the B6 review, A1: every squad's morale is
// 100 at T and nobody had stood again, so a load that restored 100 and 0 for
// them passed). Every expectation here was measured before it was written.
func perturbRows() []sweepRow {
	squad := func(t *testing.T, f map[string]any, id string) map[string]any {
		t.Helper()

		for _, raw := range asList(blockOf(t, f, "squads")["squads"]) {
			if sq, _ := raw.(map[string]any); str(sq, "id") == id {
				return sq
			}
		}

		t.Fatalf("T's file has no squad %s", id)

		return nil
	}

	// Four world minutes: under watchJumpMinutes (5), so the first frame's
	// keepWatch credits all of it while he stands the watch at T.
	const behind = 4.0

	watchBehind := func(set bool) func(*testing.T, map[string]any) {
		return func(t *testing.T, f map[string]any) {
			sc := blockOf(t, f, "scene")
			sc["watch_clock"] = json.Number(fmt.Sprint(mustNumber(t, sc["watch_clock"]) - behind))
			sc["watch_clock_set"] = set
		}
	}

	// 03:15 of day index 2: the dawn after T's night (02:45 is the epoch, and
	// day index 2 began at elapsed 2715), that dawn not yet paid
	// (dawn_paid_day 1 at T).
	const dawnAfterT = 2910.0

	atDawn := func(lastStage string) func(*testing.T, map[string]any) {
		return func(t *testing.T, f map[string]any) {
			blockOf(t, f, "clock")["elapsed"] = json.Number(fmt.Sprint(dawnAfterT))
			sc := blockOf(t, f, "scene")
			sc["watch_clock"] = json.Number(fmt.Sprint(dawnAfterT))
			sc["last_stage"] = lastStage
		}
	}

	return []sweepRow{
		{block: "squads", name: "perturb squads: s:2's morale 37", edit: func(t *testing.T, f map[string]any) {
			squad(t, f, "s:2")["morale"] = json.Number("37")
		}, want: wantDiverge, moves: []string{"system meters"}},
		{block: "squads", name: "perturb squads: s:2's man wounded (23 of his health)", edit: func(t *testing.T, f map[string]any) {
			m, _ := asList(squad(t, f, "s:2")["members"])[0].(map[string]any)
			m["health"] = json.Number("23")
		}, want: wantDiverge, moves: []string{"system meters"}},
		{block: "rising", name: "perturb rising: stood again 3, wandered 2", edit: func(t *testing.T, f map[string]any) {
			r := blockOf(t, f, "rising")
			r["stood_again"], r["wandered"] = json.Number("3"), json.Number("2")
		}, want: wantDiverge, moves: []string{"system rising"}},
		{block: "scene", name: "perturb scene: the watch clock 4 minutes behind", edit: watchBehind(true),
			want: wantDiverge, moves: []string{"system village"}},
		{block: "scene", name: "perturb scene: watch_clock_set, against its twin", edit: watchBehind(true), twin: watchBehind(false),
			want: wantDiverge, moves: []string{"system village"}},
		{block: "scene", name: "perturb scene: last_stage night at an unpaid dawn, against dawn", edit: atDawn("night"), twin: atDawn("dawn"),
			want: wantDiverge, moves: []string{"system progress"}},
		{block: "combat", name: "perturb combat: the first logged blow's damage", edit: func(t *testing.T, f map[string]any) {
			b, _ := asList(blockOf(t, f, "combat")["blow_log"])[0].(map[string]any)
			b["damage"] = json.Number(fmt.Sprint(mustNumber(t, b["damage"]) + 1))
		}, want: wantDiverge, moves: []string{"system combat"}},
		// Version 5 (pursuit-budget): the chase's from_watch, flipped -- a
		// world chase made a script's, or the other way (written only when
		// true, so "false" is the key taken out).
		{block: "pursuit", name: "perturb pursuit: the chase's from_watch flipped", edit: func(t *testing.T, f map[string]any) {
			c, _ := asList(blockOf(t, f, "pursuit")["chases"])[0].(map[string]any)
			if c["from_watch"] == true {
				delete(c, "from_watch")
			} else {
				c["from_watch"] = true
			}
		}, want: wantDiverge, moves: []string{"system pursuit"}},
		// The raid's R3a: one house's stock moved, the others as T had them.
		{block: "households", name: "perturb households: the last house's incense 1, stakes 5", edit: func(t *testing.T, f map[string]any) {
			houses := asList(blockOf(t, f, "households")["houses"])
			h, _ := houses[len(houses)-1].(map[string]any)
			h["incense"], h["stakes"] = json.Number("1"), json.Number("5")
		}, want: wantDiverge, moves: []string{"system households"}},
		{block: "sidecar", name: "perturb sidecar.journal: two entries' order swapped", edit: func(t *testing.T, f map[string]any) {
			w := blockOf(t, blockOf(t, blockOf(t, f, "sidecar"), "journal"), "written")
			w["b1_taken"], w["b2_corps"] = w["b2_corps"], w["b1_taken"]
		}, want: wantDiverge, moves: []string{"system journal"}},
		{block: "entities", name: "perturb entities: a pack member's motion (target, dir, speed)", edit: func(t *testing.T, f map[string]any) {
			for _, raw := range asList(f["entities"]) {
				if e, _ := raw.(map[string]any); e["native"] != true {
					m := blockOf(t, e, "motion")
					tg := asList(m["target"])
					tg[0] = json.Number(fmt.Sprint(mustNumber(t, tg[0]) + 1))
					m["dir"] = json.Number(fmt.Sprint(mustNumber(t, m["dir"]) + 1))
					m["speed"] = json.Number(fmt.Sprint(mustNumber(t, m["speed"]) + 1))

					return
				}
			}

			t.Fatal("T's file has no entity the map does not build")
		}, want: wantDiverge, moves: []string{"part entities"}},
	}
}

// mustNumber is a json.Number as a float64, or the test fails.
func mustNumber(t *testing.T, v any) float64 {
	t.Helper()

	n, ok := v.(json.Number)
	if !ok {
		t.Fatalf("not a number: %T %v", v, v)
	}

	f, err := n.Float64()
	if err != nil {
		t.Fatal(err)
	}

	return f
}

// villageSide is the authored village's side in tiles (TestFogIsKept act 2
// holds the fog grid to it).
const villageSide = 48

// fillFog gives T's file a fog grid on its own map: a 6 x 6 block in the
// north corner and every 37th tile besides -- a pattern no reflection or
// transposition of the grid maps onto itself. Tile (x, y) is bit y*w+x
// (TestTheGridsLayoutIsPinned).
func fillFog(t *testing.T, file map[string]any) {
	t.Helper()

	grid := make([]byte, villageSide*villageSide/8)

	for i := 0; i < villageSide*villageSide; i++ {
		if x, y := i%villageSide, i/villageSide; (x < 6 && y < 6) || i%37 == 0 {
			grid[i/8] |= 1 << uint(i%8)
		}
	}

	file["fog"] = map[string]any{
		"map": str(blockOf(t, file, "map"), "sha"), "w": json.Number(fmt.Sprint(villageSide)),
		"h": json.Number(fmt.Sprint(villageSide)), "explored": base64.StdEncoding.EncodeToString(grid),
	}
}

// checkFrames are the frames a row's check, and its twin's reading, are taken
// after: a load reports before the resumed game's fog has run a frame, so a
// grid read at once is the file's alone (the zero twin read so, wt-fog5\
// neg-green-3.txt) or not, by when the game's first frame fell.
const checkFrames = 2

// zeroFog is fillFog's twin: a grid on T's own map, of the village's size,
// with no tile explored.
func zeroFog(t *testing.T, file map[string]any) {
	t.Helper()

	file["fog"] = map[string]any{
		"map": str(blockOf(t, file, "map"), "sha"), "w": json.Number(fmt.Sprint(villageSide)),
		"h": json.Number(fmt.Sprint(villageSide)), "explored": base64.StdEncoding.EncodeToString(make([]byte, villageSide*villageSide/8)),
	}
}

// fogGridIsTheFiles: the resumed game's explored grid is the file's, bit for
// bit (the fog provider reports it as the file writes it) -- with fog off.
//
// WITH FOG ON (the shipped game since F5, 2 Oct 2026) the resumed game's
// first frames see again from where he stands (docs/fog.md, "Kept"), so the
// grid is the file's OR the ground those frames saw -- which is exactly the
// grid the twin resumed with, its grid explored nowhere (the F5 review's C1: the
// first form of this check allowed any extra tiles under half the map, and a
// load that also explored a band of 864 tiles passed it) -- and, by fog's
// own rule, the whole footprint of any structure one of the file's tiles
// lies in ("a structure with any tile explored is explored whole",
// Fog.structuresWhole): the scatter puts tiles in two 3x3 footprints, and
// their other 16 tiles came explored (wt-fog5\r3-green.txt). The footprints
// are read from the fog provider's probe, tile by tile. Tile (x, y) is bit
// y*w+x, as fillFog writes it.
//
// Negative controls (2 Oct 2026): a restore that also explores the top 18
// rows for the filled file alone (the review's M2, narrowed so the sweep's
// untouched-file control still passes; the wide M2 and M1 are red at that
// control first) and one that drops the north corner both fail this
// (wt-fog5\nc-m2b-restore-band-fillonly-2.txt, nc-h-restore-drops-corner-2.txt).
func fogGridIsTheFiles(t *testing.T, s *session, file, twinFog map[string]any) error {
	t.Helper()

	want := str(blockOf(t, file, "fog"), "explored")
	f := fogState(s)
	got := str(f, "grid")

	if !flag(t, f, "enabled") {
		if got != want {
			return fmt.Errorf("the resumed fog grid is not the file's:\n file %s\n game %s", want, got)
		}

		return nil
	}

	if twinFog == nil {
		return fmt.Errorf("fog is on and the row has no twin: the ground the load re-sees is not known")
	}

	wb, err := base64.StdEncoding.DecodeString(want)
	if err != nil {
		return fmt.Errorf("the file's fog grid: %v", err)
	}

	seen := str(twinFog, "grid")

	sb, err := base64.StdEncoding.DecodeString(seen)
	if err != nil || len(sb) != len(wb) {
		return fmt.Errorf("the twin's resumed grid (%d bytes, %v) is not the file's shape (%d bytes): %s", len(sb), err, len(wb), seen)
	}

	expect := make([]byte, len(wb))
	for i := range wb {
		expect[i] = wb[i] | sb[i]
	}

	whole := 0

	for i := 0; i < villageSide*villageSide; i++ {
		if wb[i/8]>>uint(i%8)&1 == 0 {
			continue
		}

		setField(s, "fog", "probe", map[string]any{"x": i % villageSide, "y": i / villageSide})

		r := asList(sub(fogState(s), "probe")["structure"])
		if len(r) != 4 {
			continue
		}

		x0, y0, x1, y1 := r[0].(float64), r[1].(float64), r[2].(float64), r[3].(float64)

		for y := int(y0); y < int(y1); y++ {
			for x := int(x0); x < int(x1); x++ {
				if x >= 0 && y >= 0 && x < villageSide && y < villageSide {
					j := y*villageSide + x
					if expect[j/8]>>uint(j%8)&1 == 0 {
						whole++
					}

					expect[j/8] |= 1 << uint(j%8)
				}
			}
		}
	}

	if exp := base64.StdEncoding.EncodeToString(expect); got != exp {
		return fmt.Errorf("the resumed fog grid is not the file's OR what the load re-sees (the twin's):\n file %s\n seen %s\n want %s\n game %s",
			want, seen, exp, got)
	}

	t.Logf("the resumed fog grid is the file's (%d tiles) OR the %d the load re-sees OR the %d more of the structures the file's tiles lie in, bit for bit",
		ones(wb), ones(sb), whole)

	return nil
}

// ones counts the set bits of a grid.
func ones(b []byte) int {
	n := 0

	for _, x := range b {
		for ; x != 0; x &= x - 1 {
			n++
		}
	}

	return n
}

// sweepRows is every row: the omitted half, the emptied, then the perturbed.
func sweepRows() []sweepRow {
	return append(append(omitRows(), emptyRows()...), perturbRows()...)
}

// omitSweep is act 9: the control of the controls, then every row, each
// judged; every row is run and every failure reported.
func omitSweep(t *testing.T, s *session, ev evening) {
	t.Helper()

	began := time.Now()

	// THE CONTROL OF THE CONTROLS: the untouched file resumes S_T exactly.
	sT := negLoad(t, s, ev, "the untouched file", ev.fileT)
	if sT.Resume != ev.sT.Resume {
		sameWorld(t, "act 9's control (the untouched file)", ev.sT, sT)
	}

	t.Logf("act 9 control PASS: the untouched file resumes S_T (%.12s)", sT.Resume)

	rows := sweepRows()
	failed := 0
	dawnSeen := false

	slowest, slowestName := time.Duration(0), ""

	edited := func(edit func(*testing.T, map[string]any)) (map[string]any, []byte) {
		file := decodeNumbers(t, ev.fileT)
		edit(t, file)

		data, err := json.MarshalIndent(file, "", "  ")
		if err != nil {
			t.Fatal(err)
		}

		return file, append(data, '\n')
	}

	for _, r := range rows {
		at := time.Now()

		// The row's twin, when it has one, is what it is judged against.
		against, againstName := ev.sT, "S_T"

		var twinFog map[string]any

		if r.twin != nil {
			_, twin := edited(r.twin)
			against, againstName = negLoad(t, s, ev, r.name+" (its twin)", twin), "its twin"

			if r.check != nil {
				// The same two frames the row's own check is read after, so
				// both grids are read after the same fog updates.
				s.call("strigoi_step", map[string]any{"frames": checkFrames})
				twinFog = fogState(s)
			}

			if tl := sub(s.call("strigoi_get_game_info", map[string]any{}), "load"); !flag(t, tl, "resumed") {
				failed++

				t.Errorf("act 9, %s: its twin must resume, and was refused %s (%s)", r.name, str(tl, "refused"), cut(str(tl, "reason")))

				continue
			}
		}

		file, data := edited(r.edit)
		snap := negLoad(t, s, ev, r.name, data)
		load := sub(s.call("strigoi_get_game_info", map[string]any{}), "load")

		seen, err := judgeRow(r, load, against, snap)
		if err == nil && r.check != nil {
			s.call("strigoi_step", map[string]any{"frames": checkFrames})
			err = r.check(t, s, file, twinFog)
		}

		if err != nil {
			failed++

			t.Errorf("act 9, %s (against %s): %v", r.name, againstName, err)

			continue
		}

		took := time.Since(at)
		if took > slowest {
			slowest, slowestName = took, r.name
		}

		t.Logf("act 9, %s PASS (%.1f s, against %s): %s", r.name, took.Seconds(), againstName, seen)

		// THE J1 REVIEW'S B6, on the path that does not resume (BUG-114,
		// Josh's default (a)): a refused file falls back to dawn of the day
		// after the last day his sidecar's journal knows -- not the epoch.
		// Asserted once per sweep, on the first refusal.
		if r.want == wantRefused && !dawnSeen {
			dawnSeen = true
			c := clockState(s)
			pages := stringsOf(journalState(s)["pages"])

			if want := wakeMinutesOf(t, ev.sidecarLeft); mustNum(t, c, "world_minutes") < want || mustNum(t, c, "world_minutes") > want+1 ||
				str(c, "stage") != "dawn" {
				failed++

				t.Errorf("act 9 (BUG-114): fallen back, he wakes at %s %s (%v world minutes); want dawn %v minutes in, the day after his journal's last",
					str(c, "date"), str(c, "time_of_day"), c["world_minutes"], want)
			} else {
				t.Logf("act 9 (BUG-114) PASS: fallen back to dawn of %s %s, the day after the last his journal knows; its pages are %v",
					str(c, "date"), str(c, "time_of_day"), pages)
			}
		}
	}

	if failed == 0 {
		t.Logf("act 9 PASS: %d rows over %d blocks, every one diverged, refused with its reason, or is the one tolerated by design "+
			"(%.0f s; the slowest row %.1f s, %s)",
			len(rows), len(d2save.Blocks), time.Since(began).Seconds(), slowest.Seconds(), slowestName)
	}
}

// judgeRow is a row's verdict on what its load did: what was seen, or why it
// is not what the row wants. It reads nothing but its arguments, so
// TestTheSweepJudge can hand it every wrong outcome.
func judgeRow(r sweepRow, load map[string]any, want, got worldSnap) (string, error) {
	resumed, _ := load["resumed"].(bool)
	refused, _ := load["refused"].(string)
	rule, _ := load["rule"].(string)
	reason, _ := load["reason"].(string)
	moved := diffOf(want, got)

	switch r.want {
	case wantRefused:
		switch {
		case resumed || refused == "":
			return "", fmt.Errorf("resumed (%s); want it refused %s (%s)", seenMoves(moved), r.code, r.rule)
		case refused != r.code || rule != r.rule:
			return "", fmt.Errorf("refused %s (rule %q: %s); want %s (rule %q)", refused, rule, cut(reason), r.code, r.rule)
		case got.Resume == want.Resume:
			return "", fmt.Errorf("refused %s, and yet the game is the saved moment: the fall back to dawn did not happen", refused)
		}

		return fmt.Sprintf("refused %s (%s: %s), fell back to dawn", refused, rule, cut(reason)), nil

	case wantDiverge:
		switch {
		case !resumed:
			return "", fmt.Errorf("refused %s (rule %q: %s); this emptied block is one the file's checks take, and the load must "+
				"resume it -- its restore is what is on trial", refused, rule, cut(reason))
		case got.Resume == want.Resume:
			return "", fmt.Errorf("the load resumed the saved moment with the block dropped or changed -- a hole: nothing observes what it restores")
		case len(r.moves) > 0 && !anyOf(moved, r.moves):
			return "", fmt.Errorf("diverged, but not where the block shows: %s; want one of %s", seenMoves(moved), strings.Join(r.moves, ", "))
		}

		return seenMoves(moved), nil

	case wantSame:
		switch {
		case !resumed:
			return "", fmt.Errorf("refused %s (rule %q: %s); the block is tolerated by design (%s) -- re-decide the row", refused, rule, cut(reason), r.why)
		case got.Resume != want.Resume:
			return "", fmt.Errorf("%s; the block is tolerated by design (%s), and now a load reads it -- re-decide the row", seenMoves(moved), r.why)
		}

		return "resumed the saved moment, as designed: " + r.why, nil
	}

	return "", fmt.Errorf("a row with no expectation")
}

// diffOf is where got differs from want: the digest's parts, then its systems.
func diffOf(want, got worldSnap) []string {
	var why []string

	for _, p := range []string{"world", "entities", "rng", "systems"} {
		if want.Parts[p] != got.Parts[p] {
			why = append(why, "part "+p)
		}
	}

	for _, n := range keysOfSnap(want.Systems, got.Systems) {
		if want.Systems[n] != got.Systems[n] {
			why = append(why, "system "+n)
		}
	}

	return why
}

func seenMoves(moved []string) string {
	if len(moved) == 0 {
		return "resumed the saved moment"
	}

	return "resumed and diverged: " + strings.Join(moved, ", ")
}

func anyOf(have, want []string) bool {
	for _, w := range want {
		for _, h := range have {
			if h == w {
				return true
			}
		}
	}

	return false
}

// negLoad puts his act-3 files beside the world file given, starts him, sets
// the evening's dials, and returns the world as it resumed (or the dawn he
// fell back to).
//
// It clears what an earlier save in this home left beside his files first:
// the previous world save (.bak) is what a TORN refusal resumes in its place
// (the M4.6 B5 review, A2), so in TestSaveResume's home -- where act 5 saved
// again -- a row the file's pairing refuses would have resumed T's own file
// from the .bak, and read as a hole.
func negLoad(t *testing.T, s *session, ev evening, name string, world []byte) worldSnap {
	t.Helper()

	if flag(t, s.call("strigoi_get_game_info", map[string]any{}), "in_game") {
		s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
		awaitMenu(t, s)
	}

	for _, left := range []string{ev.save + ".world.json.bak", ev.save + ".strigoi.json.preload"} {
		if err := os.Remove(left); err != nil && !os.IsNotExist(err) {
			t.Fatalf("%s: clearing %s: %v", name, left, err)
		}
	}

	actThree(t, ev, world)

	g := s.call("strigoi_start_game", map[string]any{"save_path": ev.save, "wait_seconds": 90})
	t.Logf("%s: load %v", name, sub(g, "load"))

	for _, d := range ev.savedDial {
		setField(s, d.System, d.Field, d.Value)
	}

	return snapWorld(t, s)
}

// divergence says how a variant's world differs from S_T: the refusal, or the
// parts and systems that moved.
func divergence(t *testing.T, want, got worldSnap, load map[string]any) string {
	t.Helper()

	if r := str(load, "refused"); r != "" {
		return "refused " + r + " (" + cut(str(load, "reason")) + "), fell back to dawn"
	}

	return seenMoves(diffOf(want, got))
}

// blockOf is file[name] as an object, or the test fails.
func blockOf(t *testing.T, file map[string]any, name string) map[string]any {
	t.Helper()

	b, ok := file[name].(map[string]any)
	if !ok {
		t.Fatalf("the world file's %q is not an object: %T", name, file[name])
	}

	return b
}

// decodeNumbers is a world file as a map with every number kept as written
// (json.Number): a seed past 2^53 and a float's last digit survive the edit.
func decodeNumbers(t *testing.T, data []byte) map[string]any {
	t.Helper()

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	var file map[string]any
	if err := dec.Decode(&file); err != nil {
		t.Fatalf("the world file is not JSON: %v", err)
	}

	return file
}

// swapTwoEntities swaps the ids of the first two non-native entities in the
// entity list alone -- every record naming them (a pack's members, a watch, a
// chase, a body, a squad) still names each by his own -- and re-sorts the
// list, as a load would have to read it.
func swapTwoEntities(t *testing.T, file map[string]any) {
	t.Helper()

	var picked []map[string]any

	for _, raw := range asList(file["entities"]) {
		if e, _ := raw.(map[string]any); e["native"] != true && len(picked) < 2 {
			picked = append(picked, e)
		}
	}

	if len(picked) < 2 {
		t.Fatalf("the file has fewer than two entities the map does not build")
	}

	picked[0]["id"], picked[1]["id"] = picked[1]["id"], picked[0]["id"]

	list := asList(file["entities"])
	sort.Slice(list, func(i, j int) bool {
		a, _ := list[i].(map[string]any)
		b, _ := list[j].(map[string]any)

		return str(a, "id") < str(b, "id")
	})

	file["entities"] = list
}

// TestTheOmitSweepCoversEveryBlock holds the sweep to the file (no game):
// every block of d2save.Blocks has its omitted row and at least one emptied
// row, no row names a block the file does not have, no two rows share a name,
// and every row says what it wants. A block added to the file without a row
// is red here before any game runs.
func TestTheOmitSweepCoversEveryBlock(t *testing.T) {
	if err := sweepCovers(d2save.Blocks, omitRows(), append(emptyRows(), perturbRows()...)); err != nil {
		t.Fatal(err)
	}

	t.Logf("PASS: %d blocks, %d omitted rows, %d emptied rows, %d perturbed rows",
		len(d2save.Blocks), len(omitRows()), len(emptyRows()), len(perturbRows()))
}

// sweepCovers is TestTheOmitSweepCoversEveryBlock's rule, apart from the
// tables, so its own test can hand it tables with holes.
func sweepCovers(blocks []string, omitted, emptied []sweepRow) error {
	isBlock := map[string]bool{}
	for _, b := range blocks {
		isBlock[b] = true
	}

	omits, empties, names := map[string]int{}, map[string]int{}, map[string]bool{}

	for i, rows := range [][]sweepRow{omitted, emptied} {
		for _, r := range rows {
			switch {
			case !isBlock[r.block]:
				return fmt.Errorf("row %q drops %q, which is not a block of the world file (%s)", r.name, r.block, strings.Join(blocks, ", "))
			case names[r.name]:
				return fmt.Errorf("two rows are named %q", r.name)
			case r.edit == nil:
				return fmt.Errorf("row %q edits nothing", r.name)
			case r.want == wantRefused && r.code == "":
				return fmt.Errorf("row %q wants a refusal and names no code", r.name)
			case r.want == wantSame && r.why == "":
				return fmt.Errorf("row %q is tolerated and says not why", r.name)
			case i == 0 && r.want != wantRefused:
				return fmt.Errorf("omitted row %q must want a refusal: a missing block is never an empty system", r.name)
			}

			names[r.name] = true

			if i == 0 {
				omits[r.block]++
			} else {
				empties[r.block]++
			}
		}
	}

	for _, b := range blocks {
		switch {
		case omits[b] != 1:
			return fmt.Errorf("block %q has %d omitted rows; want 1", b, omits[b])
		case empties[b] == 0:
			return fmt.Errorf("block %q has no emptied row: the sweep would not know whether a load reads it", b)
		}
	}

	return nil
}

// TestTheSweepJudge hands the coverage rule and the judge every wrong outcome
// (no game): each must be named, and the right outcomes passed.
func TestTheSweepJudge(t *testing.T) {
	sT := worldSnap{Resume: "T", Parts: map[string]string{"rng": "r", "systems": "s"}, Systems: map[string]string{"light": "l", "seek": "k"}}
	dark := worldSnap{Resume: "D", Parts: map[string]string{"rng": "r", "systems": "s2"}, Systems: map[string]string{"light": "l2", "seek": "k"}}

	resumed := map[string]any{"resumed": true}
	refused := func(code, rule string) map[string]any {
		return map[string]any{"resumed": false, "refused": code, "rule": rule, "reason": "why"}
	}

	diverge := sweepRow{name: "d", want: wantDiverge, moves: []string{"system light"}}
	refuse := sweepRow{name: "r", want: wantRefused, code: "FILE", rule: "map"}
	same := sweepRow{name: "s", want: wantSame, why: "informational"}

	for _, c := range []struct {
		name  string
		row   sweepRow
		load  map[string]any
		got   worldSnap
		wrong bool
	}{
		{"diverged where the block shows", diverge, resumed, dark, false},
		{"A HOLE: resumed the saved moment", diverge, resumed, sT, true},
		{"diverged elsewhere only", sweepRow{name: "d", want: wantDiverge, moves: []string{"system seek"}}, resumed, dark, true},
		{"refused where a resume was wanted", diverge, refused("FILE", "map"), dark, true},
		{"refused with its code and rule", refuse, refused("FILE", "map"), dark, false},
		{"refused with another rule", refuse, refused("FILE", "rng-stream"), dark, true},
		{"refused with another code", refuse, refused("BLOCK", "map"), dark, true},
		{"resumed where a refusal was wanted", refuse, resumed, dark, true},
		{"refused, yet the saved moment", refuse, refused("FILE", "map"), sT, true},
		{"tolerated, as designed", same, resumed, sT, false},
		{"tolerated, and now read", same, resumed, dark, true},
		{"tolerated, and now refused", same, refused("FILE", "x"), dark, true},
	} {
		seen, err := judgeRow(c.row, c.load, sT, c.got)
		if (err != nil) != c.wrong {
			t.Errorf("%s: judged %q / %v; want wrong=%v", c.name, seen, err, c.wrong)
		}
	}

	edit := func(*testing.T, map[string]any) {}
	blocks := []string{"a", "b"}
	full := []sweepRow{{block: "a", name: "omit a", edit: edit, code: "FILE", want: wantRefused}, {block: "b", name: "omit b", edit: edit, code: "FILE", want: wantRefused}}
	empties := []sweepRow{{block: "a", name: "empty a", edit: edit}, {block: "b", name: "empty b", edit: edit}}

	if err := sweepCovers(blocks, full, empties); err != nil {
		t.Errorf("a whole table: %v", err)
	}

	for _, c := range []struct {
		name             string
		omitted, emptied []sweepRow
	}{
		{"a block with no emptied row", full, empties[:1]},
		{"a block with no omitted row", full[:1], empties},
		{"a row of a block the file lacks", full, append(append([]sweepRow{}, empties...), sweepRow{block: "z", name: "empty z", edit: edit})},
		{"two rows of one name", full, append(append([]sweepRow{}, empties...), sweepRow{block: "a", name: "empty a", edit: edit})},
		{"a refusal with no code", full, append(append([]sweepRow{}, empties...), sweepRow{block: "a", name: "r", edit: edit, want: wantRefused})},
		{"a tolerated row with no why", full, append(append([]sweepRow{}, empties...), sweepRow{block: "a", name: "t", edit: edit, want: wantSame})},
		{"an omitted row that does not want a refusal", []sweepRow{full[0], {block: "b", name: "omit b", edit: edit}}, empties},
	} {
		if err := sweepCovers(blocks, c.omitted, c.emptied); err == nil {
			t.Errorf("%s: the coverage rule took it", c.name)
		}
	}
}
