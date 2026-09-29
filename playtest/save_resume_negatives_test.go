//go:build playtest

package playtest

import (
	"bytes"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

// TestSaveResumeNegatives is the build plan's section 4 act 9 -- B6's sweep --
// as far as B4b takes it (29 Sep 2026): every block B4b loads, dropped from a
// hunted night's world file, must make the load diverge from the saved moment
// (S_R0 != S_T) or refuse the file. "A block whose omission doesn't diverge
// is a hole in observability, not a pass."
//
// Two ways to drop a block, and both are run:
//   - OMITTED, as strigoi_save_game{omit} writes it: the block's key gone.
//     d2save.Decode refuses a missing block (a missing block is never an
//     empty system), so every omission is refused at step 1 -- FILE -- and he
//     begins at dawn: divergent, but by the file's own refusal, which proves
//     the file needs the block and not that the load restores it.
//   - EMPTIED: the block present and its content taken out (no entities, no
//     bodies, no groups, no watches, no chases, no arrivals, no deployed
//     squad). A file World.Check still takes resumes, and the load must then
//     restore the emptiness: a load that ignored the block would resume the
//     saved moment anyway and this act would go red. That is the half with
//     teeth; the omitted half is the plan's.
//
// And two more of the brief's: two entities' ids swapped (refused, or
// divergent), and the untouched file -- the control of the controls, which
// must resume the saved moment exactly.
//
// OPT-IN: it runs against a hunted night a green TestSaveResume kept
// (STRIGOI_SAVE_RESUME_FROM=<dir>, the evening's hero files at T and S_T),
// and is skipped without one. Whether it joins the suite with its own acts
// 1-3 (about two minutes more) is B6's to decide.
func TestSaveResumeNegatives(t *testing.T) {
	from := os.Getenv("STRIGOI_SAVE_RESUME_FROM")
	if from == "" {
		t.Skip("opt-in: set STRIGOI_SAVE_RESUME_FROM to an evening a green TestSaveResume kept")
	}

	ev := keptEvening(t, from)

	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	type variant struct {
		name   string
		edit   func(file map[string]any)
		resume bool // the file must still be one the load resumes (emptied, not refused)
	}

	omit := func(block string) func(map[string]any) {
		return func(file map[string]any) { delete(file, block) }
	}

	variants := []variant{
		{"omit entities", omit("entities"), false},
		{"omit bodies", omit("bodies"), false},
		{"omit spawns", omit("spawns"), false},
		{"omit spawner", omit("spawner"), false},
		{"omit notice", omit("notice"), false},
		{"omit pursuit", omit("pursuit"), false},
		{"omit squads", omit("squads"), false},
		{"empty bodies", func(file map[string]any) { file["bodies"] = []any{} }, true},
		{"empty spawns.groups", func(file map[string]any) { sub(file, "spawns")["groups"] = []any{} }, true},
		{"empty spawner.arrival", func(file map[string]any) { sub(file, "spawner")["arrival"] = json.Number("0") }, true},
		{"empty notice.watches", func(file map[string]any) { sub(file, "notice")["watches"] = []any{} }, true},
		{"empty pursuit.chases", func(file map[string]any) { sub(file, "pursuit")["chases"] = []any{} }, true},
		{"empty squads-deployed", func(file map[string]any) {
			sq := sub(file, "squads")
			sq["squads"] = asList(sq["squads"])[:1]
			sq["selected"] = "s:1"
		}, true},
		{"empty entities (non-native)", func(file map[string]any) {
			kept := []any{}
			for _, raw := range asList(file["entities"]) {
				if e, _ := raw.(map[string]any); flag(t, e, "native") {
					kept = append(kept, e)
				}
			}
			file["entities"] = kept
		}, false},
		{"swap two entities' ids", swapTwoEntities(t), false},
	}

	// THE CONTROL OF THE CONTROLS: the untouched file resumes S_T exactly.
	sT := negLoad(t, s, ev, "the untouched file", ev.fileT)
	if sT.Resume != ev.sT.Resume {
		sameWorld(t, "negatives' control (the untouched file)", ev.sT, sT)
	}

	t.Logf("control PASS: the untouched file resumes S_T (%.12s)", sT.Resume)

	for _, v := range variants {
		file := decodeNumbers(t, ev.fileT)
		v.edit(file)

		data, err := json.MarshalIndent(file, "", "  ")
		if err != nil {
			t.Fatal(err)
		}

		snap := negLoad(t, s, ev, v.name, append(data, '\n'))
		load := sub(s.call("strigoi_get_game_info", map[string]any{}), "load")

		switch {
		case v.resume && !flag(t, load, "resumed"):
			t.Fatalf("%s: an emptied block the file's checks take must RESUME (the load's restore is what is on trial): %v", v.name, load)
		case snap.Resume == ev.sT.Resume:
			t.Fatalf("%s: the load resumed the saved moment with the block dropped -- a hole: nothing observes what it restores", v.name)
		}

		t.Logf("%s PASS: %s", v.name, divergence(t, ev.sT, snap, load))
	}
}

// negLoad puts his act-3 files beside the world file given, starts him, sets
// the evening's dials, and returns the world as it resumed (or the dawn he
// fell back to).
func negLoad(t *testing.T, s *session, ev evening, name string, world []byte) worldSnap {
	t.Helper()

	if flag(t, s.call("strigoi_get_game_info", map[string]any{}), "in_game") {
		s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
		awaitMenu(t, s)
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

	return "resumed and diverged: " + strings.Join(why, ", ")
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
func swapTwoEntities(t *testing.T) func(file map[string]any) {
	return func(file map[string]any) {
		var picked []map[string]any

		for _, raw := range asList(file["entities"]) {
			if e, _ := raw.(map[string]any); !flag(t, e, "native") && len(picked) < 2 {
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
}
