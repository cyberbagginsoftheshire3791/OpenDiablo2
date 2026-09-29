//go:build playtest

package playtest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestSaveResume is M4.6's acceptance test (build plan section 4): the save
// "stops at that point then resumes at that point" (Josh, 25 Sep 2026).
//
// BURST B3 BUILDS ACT 7 ONLY -- the save verb and its refusals. Acts 1-6 are
// the resume itself (fill every block, save at T, step 120 world minutes,
// quit, relaunch, load, and compare S_R0 = S_T and S_R = S_U), and they are
// burst B4's: nothing can load the file yet. Act 8 (rule 4, a save mid-walk)
// is B4's too, and act 9's negative controls (omit every block in turn) are
// B6's -- this act proves the omit/to verb they will drive.
//
// Act 7, as the plan has it, and what B3 adds to it:
//
//	7a  a save outside a fight writes N.od2.world.json with every block, and
//	    the .od2 and the sidecar with it -- the sidecar byte for byte the
//	    document the world file embeds -- and SAVING CHANGES NOTHING: the
//	    state digest before and after is one digest (rule 10's autosave runs
//	    inside a frame; a save that drew a number would move the night).
//	7b  mid-fight: FIGHTING, and every file untouched (hashed before and
//	    after), a save to another path included.
//	7c  a busy world -- a lit torch, a pack on the map, a chase, the placed
//	    dead, a slain dog with his wounds, a fight's counters -- saves with
//	    every list block non-empty and every live pack member in the entity
//	    list where his pack says he stands; the previous save is the .bak;
//	    and the digest is still unmoved.
//	7d  to writes only the world file, where it is told, the same file but
//	    for saved_at; omit leaves a block out, and needs to.
//	7e  dead: DEAD, every file untouched -- and the death put his sidecar
//	    back to the LAST SAVE's bytes, not to the moment he entered, because
//	    the save re-takes the death screen's copy.
//
// The B3 review (29 Sep 2026) adds, in the same acts:
//
//	7a  the file names its hero -- the .od2's heroName and heroType -- and
//	    carries his facing and stamina, each what the player reports; he is
//	    walked a step first, so he faces somewhere other than where he
//	    started. The sidecar file is of the world file's generation.
//	7c  before the save, the sidecar a fight's end rewrote still carries
//	    7a's generation; at least one pack member is checked in the entity
//	    list; the .od2's .bak is 7a's .od2, byte for byte.
//	7d  to is fenced: relative, inside the repository, or one of his own
//	    files is BAD_ARGUMENT and writes nothing; world_path is absolute.
//	7f  (run before 7e, which kills him) the slain dog taken off the map by
//	    the harness takes his body with him (body_dropped), and the next save
//	    is made -- it used to fail INTERNAL on a body with no entity.
func TestSaveResume(t *testing.T) {
	s := start(t)

	s.call("strigoi_pause", map[string]any{})

	game := s.call("strigoi_start_game", map[string]any{
		"hero_name": "Saver", "hero_class": "amazon", "seed": 99, "wait_seconds": 90,
	})
	setField(s, "spawns", "chance", 0)

	save := str(game, "save_path")
	world := save + ".world.json"
	sidecar := save + ".strigoi.json"

	if save == "" {
		t.Fatalf("start_game reported no save path: %v", game)
	}

	// Acts 1-6: B4 (the resume). Act 8: B4. Act 9: B6.

	// --- 7a: a quiet save writes all three, and moves nothing --------------
	// A step first, so the facing the file must carry is not the one he was
	// made with (the B3 review, B6): one neighbour, and back if that one left
	// him facing 0.
	faceSomewhere(t, s)

	out := saveUnmoved(t, s, "7a", map[string]any{})

	if got := str(out, "world_path"); got != world {
		t.Fatalf("7a: world_path %q, want %q beside his save", got, world)
	}

	for _, p := range []string{world, save, sidecar} {
		if !hasString(stringsOf(out["written"]), p) {
			t.Fatalf("7a: the save writes %s; written %v", p, out["written"])
		}
	}

	first := mustRead(t, world)
	file := worldFile(t, "7a", first)

	// One moment in both files (B7): the sidecar file is of this save's
	// generation, before it is compared whole.
	gen7a := str(file, "saved_at")
	if got := generationOf(t, mustRead(t, sidecar)); got != gen7a || gen7a == "" {
		t.Fatalf("7a: the sidecar is of this save's generation %q; it carries %q", gen7a, got)
	}

	sameSidecar(t, "7a", file, mustRead(t, sidecar))

	if got := str(file, "seed"); got != "99" {
		t.Fatalf("7a: the seed is written as the exact string \"99\", got %v", file["seed"])
	}

	p := s.call("strigoi_get_player", map[string]any{})
	hero := sub(file, "hero")

	if num(hero, "x") != num(p, "x") || num(hero, "y") != num(p, "y") {
		t.Fatalf("7a: the hero is saved where he stands: file %v,%v, player %v,%v", hero["x"], hero["y"], p["x"], p["y"])
	}

	if num(hero, "health") != num(sub(p, "state"), "health") || num(hero, "health") <= 0 {
		t.Fatalf("7a: the hero's health is saved: file %v, player %v", hero["health"], sub(p, "state")["health"])
	}

	// Who he is, which way he faces, how much wind he has (A1, B6).
	firstOD2 := mustRead(t, save)
	sameHero(t, "7a", hero, firstOD2)

	if num(hero, "facing") != num(sub(p, "state"), "direction") || num(hero, "facing") == 0 {
		t.Fatalf("7a: his facing is saved as he faces, and he was walked to face somewhere: file %v, player %v",
			hero["facing"], sub(p, "state")["direction"])
	}

	if num(hero, "stamina") != num(sub(p, "state"), "stamina") {
		t.Fatalf("7a: his stamina is saved: file %v, player %v", hero["stamina"], sub(p, "state")["stamina"])
	}

	t.Logf("7a PASS: %d bytes, %d blocks, the sidecar embedded byte for byte, digest unmoved; %s the %s facing %v with %v stamina, generation %s",
		len(first), len(file), str(hero, "name"), str(hero, "class"), hero["facing"], hero["stamina"], gen7a)

	// --- 7b: mid-fight, FIGHTING, and nothing is touched --------------------
	pl := s.call("strigoi_get_player", map[string]any{})
	spot := clearNeighbour(t, s, num(pl, "x"), num(pl, "y"))
	dog := spawnNPC(t, s, "fallen1", spot[0], spot[1])
	dogID := entityID(t, s, dog)

	s.call("strigoi_watch", map[string]any{"watcher": dog, "target": str(pl, "handle")})
	fightNow(t, s)

	files := []string{world, world + ".bak", save, save + ".bak", sidecar}
	before := hashFiles(t, files)
	elsewhere := filepath.Join(t.TempDir(), "mid-fight.world.json")

	refusedWith(t, s, "7b", "FIGHTING", map[string]any{})
	refusedWith(t, s, "7b (to)", "FIGHTING", map[string]any{"to": elsewhere, "omit": []any{"corpses"}})

	if after := hashFiles(t, files); !equalHashes(before, after) {
		t.Fatalf("7b: a refused save touched a file:\n before %v\n after  %v", before, after)
	}

	if _, err := os.Stat(elsewhere); !os.IsNotExist(err) {
		t.Fatalf("7b: a refused save wrote %s (%v)", elsewhere, err)
	}

	t.Logf("7b PASS: mid-fight FIGHTING, %d files untouched, nothing written elsewhere", len(files))

	// --- 7c: a busy world saves whole ---------------------------------------
	// The dog dies (forced crits, as TestCombatResolver finishes its own), so
	// the file carries a fight's counters and a body with its wounds.
	for i := 0; i < 8 && flag(t, combatState(s), "fighting"); i++ {
		setField(s, "combat", "forced_band", "crit")
		stepToNewRound(t, s)
	}

	setField(s, "combat", "forced_band", "")

	if got := str(combatState(s), "ended_reason"); got != "enemies_dead" {
		t.Fatalf("7c: the dog must die for the file to carry a fight's end; ended_reason %q", got)
	}

	s.call("strigoi_step", map[string]any{"frames": 2})

	// His torch, lit.
	s.call("strigoi_key", map[string]any{"key": "l"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if !flag(t, lightState(s), "carried_lit") {
		t.Fatalf("7c: L lights his torch: %v", lightState(s))
	}

	// A pack on the map: the table forced, and the save taken the moment one
	// arrives -- before it can walk in and make a fight.
	setField(s, "spawns", "chance", 100)

	for i := 0; i < 60 && num(spawnsState(s), "groups") == 0; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 1})
	}

	setField(s, "spawns", "chance", 0)

	if num(spawnsState(s), "groups") == 0 {
		t.Fatalf("7c: a certain table never placed a pack: %v", spawnsState(s))
	}

	// A chase, of one entity after another: a second fallen, far off and
	// watching nobody, sent after a villager.
	chaser := spawnNPC(t, s, "fallen1", num(pl, "x")+9, num(pl, "y")+9)
	villager := nativeHandle(t, s)

	s.call("strigoi_pursue", map[string]any{"hunter": chaser, "quarry": villager})
	s.call("strigoi_step", map[string]any{"frames": 1})

	if flag(t, combatState(s), "fighting") {
		t.Fatalf("7c: the arranged world must not be in a fight when it saves: %v", combatState(s))
	}

	// The fight's end rewrote his sidecar (saveKit); it carries the last
	// world save's generation forward, so the two files still agree (B7).
	if got := generationOf(t, mustRead(t, sidecar)); got != gen7a {
		t.Fatalf("7c: a kit save between world saves carries 7a's generation %q; the sidecar carries %q", gen7a, got)
	}

	out = saveUnmoved(t, s, "7c", map[string]any{})

	second := mustRead(t, world)
	file = worldFile(t, "7c", second)

	// The file, kept beside the run's logs for a person to read.
	if s.RunBase != "" {
		keep := filepath.Join(s.RunBase, "pt", safeName(t.Name()), "save-resume-7c.world.json")
		if err := os.WriteFile(keep, second, 0o600); err == nil {
			t.Logf("7c: the world file is kept at %s", keep)
		}
	}

	if bak := mustRead(t, world+".bak"); !bytes.Equal(bak, first) {
		t.Fatalf("7c: the previous save is kept as .bak, byte for byte (rule 5)")
	}

	if bak := mustRead(t, save+".bak"); !bytes.Equal(bak, firstOD2) {
		t.Fatalf("7c: the .od2 keeps its previous generation too -- 7a's .od2, byte for byte (rule 5)")
	}

	sameSidecar(t, "7c", file, mustRead(t, sidecar))
	sameHero(t, "7c", sub(file, "hero"), mustRead(t, save))

	if got, want := generationOf(t, mustRead(t, sidecar)), str(file, "saved_at"); got != want {
		t.Fatalf("7c: the sidecar is of 7c's generation %q; it carries %q", want, got)
	}

	savedSidecar := mustRead(t, sidecar)

	for _, path := range []string{"light.sources", "squads.squads", "spawns.groups", "notice.watches",
		"pursuit.chases", "corpses.bodies", "bodies", "entities", "scene.field_dead"} {
		if n := len(listAt(t, file, path)); n == 0 {
			t.Fatalf("7c: %s is empty in a world arranged to fill it", path)
		}
	}

	if num(sub(file, "combat"), "started") < 1 || num(sub(file, "clock"), "elapsed") <= 0 {
		t.Fatalf("7c: a fight's counters and the clock are saved: combat %v, clock %v", file["combat"], file["clock"])
	}

	rng := sub(file, "rng")
	if num(sub(rng, "world"), "draws") <= 0 || sub(rng, "uuid") == nil || num(sub(rng, "uuid"), "bytes") <= 0 {
		t.Fatalf("7c: the world and uuid streams are saved where they stand: %v", rng)
	}

	for _, stream := range []string{"spawns", "combat", "rising"} {
		if fmt.Sprint(sub(rng, stream)) != fmt.Sprint(sub(sub(file, stream), "rng")) {
			t.Fatalf("7c: rng.%s %v is the %s block's own stream %v", stream, sub(rng, stream), stream, sub(sub(file, stream), "rng"))
		}
	}

	checkMap(t, sub(file, "map"))

	entities := map[string]map[string]any{}
	for _, raw := range listAt(t, file, "entities") {
		e, _ := raw.(map[string]any)
		entities[str(e, "id")] = e
	}

	if dogE := entities[dogID]; dogE == nil || str(dogE, "kind") != "npc" || str(dogE, "monstat") != "fallen1" {
		t.Fatalf("7c: the slain dog is an npc entity of monstat fallen1: %v", dogE)
	}

	members := 0

	for _, raw := range listAt(t, file, "spawns.groups") {
		g, _ := raw.(map[string]any)

		for _, rm := range asList(g["members"]) {
			// gone is written only when true (omitempty).
			m, _ := rm.(map[string]any)
			if gone, _ := m["gone"].(bool); gone {
				continue
			}

			e := entities[str(m, "id")]
			if e == nil || num(e, "x") != num(m, "x") || num(e, "y") != num(m, "y") {
				t.Fatalf("7c: %s's member %s stands at %v,%v and the entity list has %v", str(g, "id"), str(m, "id"), m["x"], m["y"], e)
			}

			members++
		}
	}

	natives := 0

	for _, e := range entities {
		if flag(t, e, "native") {
			natives++

			if len(asList(e["born"])) != 2 {
				t.Fatalf("7c: a native entity carries where the map put it: %v", e)
			}
		}
	}

	if natives == 0 {
		t.Fatalf("7c: the map's villagers are marked native")
	}

	// The loop above checks every member on the map against the entity list;
	// with none, it checked nothing (the review's C item).
	if members == 0 {
		t.Fatalf("7c: no pack member was on the map to check against the entity list")
	}

	t.Logf("7c PASS: %d entities (%d native), %d pack member(s) where their pack says, every list block non-empty, .bak = 7a's file",
		len(entities), natives, members)

	// --- 7d: to and omit -------------------------------------------------
	ours := hashFiles(t, files)
	to := filepath.Join(t.TempDir(), "copy.world.json")

	out = s.call("strigoi_save_game", map[string]any{"to": to})
	if str(out, "world_path") != to || !filepath.IsAbs(str(out, "world_path")) || len(stringsOf(out["written"])) != 1 {
		t.Fatalf("7d: to writes the world file there and nothing else, and says where, absolute: %v", out)
	}

	// The fence (B5): relative (the game's working directory is the
	// repository), inside the repository, or one of his own files.
	repo, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}

	inRepo := filepath.Join(repo, "docs", "b3-to.world.json")

	for _, bad := range []map[string]any{
		{"to": "b3-relative.world.json"},
		{"to": inRepo},
		{"to": world},
		{"to": world, "omit": []any{"corpses"}},
		{"to": sidecar},
	} {
		if e := s.callErr("strigoi_save_game", bad); !strings.Contains(e, "BAD_ARGUMENT") {
			t.Fatalf("7d: %v must be BAD_ARGUMENT, got %q", bad, e)
		}
	}

	for _, p := range []string{inRepo, filepath.Join(repo, "b3-relative.world.json")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("7d: a refused to wrote %s (%v)", p, err)
		}
	}

	if !equalHashes(ours, hashFiles(t, files)) {
		t.Fatalf("7d: a save to another path touched his save")
	}

	if !bytes.Equal(withoutSavedAt(t, mustRead(t, to)), withoutSavedAt(t, mustRead(t, world))) {
		t.Fatalf("7d: the same moment written elsewhere is the same file, but for saved_at")
	}

	omitted := filepath.Join(t.TempDir(), "no-corpses.world.json")
	s.call("strigoi_save_game", map[string]any{"to": omitted, "omit": []any{"corpses", "scene"}})

	var top map[string]json.RawMessage
	if err := json.Unmarshal(mustRead(t, omitted), &top); err != nil {
		t.Fatal(err)
	}

	if _, ok := top["corpses"]; ok || len(top) != len(worldBlocks)-2 {
		t.Fatalf("7d: omit leaves out corpses and scene and nothing else: %d blocks", len(top))
	}

	for _, bad := range []map[string]any{{"omit": []any{"corpses"}}, {"to": omitted, "omit": []any{"weather"}}} {
		if e := s.callErr("strigoi_save_game", bad); !strings.Contains(e, "BAD_ARGUMENT") {
			t.Fatalf("7d: %v must be BAD_ARGUMENT, got %q", bad, e)
		}
	}

	if !equalHashes(ours, hashFiles(t, files)) {
		t.Fatalf("7d: omit and its refusals touched his save")
	}

	t.Logf("7d PASS: to writes only there (the same file but saved_at); omit drops exactly its blocks; omit without to is refused; to is fenced")

	// --- 7f: a monster taken off the map takes his body with him ------------
	// (Run before 7e, which kills him.) The slain dog's body is in the file
	// at 0; the harness's removal used to leave it, with no entity, and every
	// save after it failed INTERNAL "a body with no entity in the file".
	removed := s.call("strigoi_remove_entity", map[string]any{"handle": dog})
	if removed["removed"] != true || removed["body_dropped"] != true {
		t.Fatalf("7f: removing the slain dog drops his body: %v", removed)
	}

	saveUnmoved(t, s, "7f", map[string]any{})

	after := worldFile(t, "7f", mustRead(t, world))

	for _, raw := range listAt(t, after, "bodies") {
		if b, _ := raw.(map[string]any); str(b, "id") == dogID {
			t.Fatalf("7f: the removed dog's body is still saved: %v", b)
		}
	}

	for _, raw := range listAt(t, after, "entities") {
		if e, _ := raw.(map[string]any); str(e, "id") == dogID {
			t.Fatalf("7f: the removed dog is still an entity: %v", e)
		}
	}

	// 7f's save is the last save now: the death below puts ITS sidecar back.
	savedSidecar = mustRead(t, sidecar)

	t.Logf("7f PASS: the slain dog removed with his body, and the next save made")

	// --- 7e: dead, DEAD, and the death put the LAST SAVE back ---------------
	setField(s, "meters", "health", 0.0)
	s.call("strigoi_step", map[string]any{"frames": 3})

	if !flag(t, uiState(s), "death_open") {
		t.Fatalf("7e: at 0 health the death screen is up: %v", uiState(s))
	}

	if now := mustRead(t, sidecar); !bytes.Equal(now, savedSidecar) {
		t.Fatalf("7e: the death puts his sidecar back to the LAST SAVE's bytes (the save re-took the death screen's copy):\n saved: %s\n now:   %s",
			savedSidecar, now)
	}

	dead := hashFiles(t, files)

	refusedWith(t, s, "7e", "DEAD", map[string]any{})

	if !equalHashes(dead, hashFiles(t, files)) {
		t.Fatalf("7e: a refused save touched a file")
	}

	t.Logf("7e PASS: DEAD, every file untouched; the death restored the last save's sidecar")
}

// worldBlocks is every top-level block of the world file, in order
// (d2save.Blocks): the script keeps its own copy, so a block dropped from the
// code is a red script, not a quietly shorter list.
var worldBlocks = []string{
	"version", "build", "saved_at",
	"map", "seed", "rng", "hero", "sidecar",
	"clock", "light", "squads", "spawns", "spawner", "notice", "pursuit",
	"corpses", "rising", "combat", "bodies", "entities", "scene",
}

// saveUnmoved saves and requires that the save moved nothing: the digest
// before and after is one digest (every part named when it is not).
func saveUnmoved(t *testing.T, s *session, act string, args map[string]any) map[string]any {
	t.Helper()

	d0, parts0 := digest(s)
	out := s.call("strigoi_save_game", args)
	d1, parts1 := digest(s)

	if d0 != d1 {
		var moved []string

		for k, v := range parts0 {
			if fmt.Sprint(parts1[k]) != fmt.Sprint(v) {
				moved = append(moved, k)
			}
		}

		t.Fatalf("%s: saving moved the world: the digest parts %v changed", act, moved)
	}

	return out
}

// refusedWith requires a save refused with code.
func refusedWith(t *testing.T, s *session, act, code string, args map[string]any) {
	t.Helper()

	if e := s.callErr("strigoi_save_game", args); !strings.HasPrefix(e, code+":") {
		t.Fatalf("%s: the save must be refused %s, got %q", act, code, e)
	}
}

// worldFile decodes a world file and requires every block, in order,
// version 1.
func worldFile(t *testing.T, act string, data []byte) map[string]any {
	t.Helper()

	var file map[string]any
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("%s: the world file is not JSON: %v", act, err)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	_, _ = dec.Token()

	var order []string

	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}

		order = append(order, tok.(string))

		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}

	if strings.Join(order, ",") != strings.Join(worldBlocks, ",") {
		t.Fatalf("%s: the world file's blocks are %v, want %v", act, order, worldBlocks)
	}

	if num(file, "version") != 1 {
		t.Fatalf("%s: version %v", act, file["version"])
	}

	return file
}

// sameSidecar requires the world file's embedded sidecar to be the sidecar
// file's document (compared compact).
func sameSidecar(t *testing.T, act string, file map[string]any, onDisk []byte) {
	t.Helper()

	embedded, err := json.Marshal(file["sidecar"])
	if err != nil {
		t.Fatal(err)
	}

	var disk any
	if err := json.Unmarshal(onDisk, &disk); err != nil {
		t.Fatalf("%s: the sidecar is not JSON: %v", act, err)
	}

	want, _ := json.Marshal(disk)

	if !bytes.Equal(embedded, want) {
		t.Fatalf("%s: the world file's sidecar is not the sidecar file's document:\n embedded %s\n on disk  %s", act, embedded, want)
	}
}

// checkMap: the authored village names its .tmj and its SHA; the generated
// world (the -classic sweep) says it is generated.
func checkMap(t *testing.T, m map[string]any) {
	t.Helper()

	if flag(t, m, "generated") {
		if str(m, "path") != "" || str(m, "sha") != "" {
			t.Fatalf("the generated world has no path or sha: %v", m)
		}

		return
	}

	if !strings.HasSuffix(str(m, "path"), ".tmj") || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(str(m, "sha")) {
		t.Fatalf("an authored map names its .tmj and its sha256: %v", m)
	}
}

// listAt is the list at a dotted path of the file.
func listAt(t *testing.T, file map[string]any, path string) []any {
	t.Helper()

	parts := strings.Split(path, ".")
	cur := file

	for _, p := range parts[:len(parts)-1] {
		cur = sub(cur, p)
	}

	return asList(cur[parts[len(parts)-1]])
}

// nativeHandle is the handle of one of the map's villagers (scene.natives).
func nativeHandle(t *testing.T, s *session) string {
	t.Helper()

	scene := sub(s.call("strigoi_get_system_state", map[string]any{"system": "scene"}), "state")

	natives := map[string]bool{}
	for _, raw := range asList(scene["natives"]) {
		n, _ := raw.(map[string]any)
		natives[str(n, "id")] = true
	}

	list := s.call("strigoi_get_entities", map[string]any{"kind": "npc", "limit": 200})

	for _, raw := range asList(list["items"]) {
		e, _ := raw.(map[string]any)
		if natives[str(e, "id")] {
			return str(e, "handle")
		}
	}

	t.Fatalf("no native entity among the npcs (scene natives %v)", scene["natives"])

	return ""
}

func stringsOf(v any) []string {
	var out []string

	for _, x := range asList(v) {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}

	return out
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()

	data, err := readSaved(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	return data
}

// hashFiles is each file's SHA-256, or "absent".
func hashFiles(t *testing.T, paths []string) map[string]string {
	t.Helper()

	out := map[string]string{}

	for _, p := range paths {
		data, err := readSaved(p)

		switch {
		case os.IsNotExist(err):
			out[p] = "absent"
		case err != nil:
			t.Fatalf("reading %s: %v", p, err)
		default:
			sum := sha256.Sum256(data)
			out[p] = hex.EncodeToString(sum[:])
		}
	}

	return out
}

func equalHashes(a, b map[string]string) bool {
	return fmt.Sprint(a) == fmt.Sprint(b)
}

// withoutSavedAt is a world file with its saved_at line blanked, and the
// embedded sidecar's generation, which is the same moment (B7).
func withoutSavedAt(t *testing.T, data []byte) []byte {
	t.Helper()

	re := regexp.MustCompile(`(?m)^  "saved_at": "[^"]*",$`)
	if !re.Match(data) {
		t.Fatalf("no saved_at line in the world file")
	}

	gen := regexp.MustCompile(`(?m)^    "generation": "[^"]*",$`)
	if !gen.Match(data) {
		t.Fatalf("no generation line in the world file's sidecar")
	}

	return gen.ReplaceAll(re.ReplaceAll(data, []byte(`  "saved_at": "",`)), []byte(`    "generation": "",`))
}

// generationOf is a sidecar document's generation.
func generationOf(t *testing.T, sidecar []byte) string {
	t.Helper()

	var sc struct {
		Generation string `json:"generation"`
	}

	if err := json.Unmarshal(sidecar, &sc); err != nil {
		t.Fatalf("the sidecar is not JSON: %v", err)
	}

	return sc.Generation
}

// sameHero requires the world file's hero to be the .od2's: its heroName, and
// its heroType by name (the B3 review, A1).
func sameHero(t *testing.T, act string, hero map[string]any, od2 []byte) {
	t.Helper()

	var h struct {
		Name string `json:"heroName"`
		Type int    `json:"heroType"`
	}

	if err := json.Unmarshal(od2, &h); err != nil {
		t.Fatalf("%s: the .od2 is not JSON: %v", act, err)
	}

	classes := []string{"", "Barbarian", "Necromancer", "Paladin", "Assassin", "Sorceress", "Amazon", "Druid"}

	if h.Type < 1 || h.Type >= len(classes) || str(hero, "name") != h.Name || str(hero, "class") != classes[h.Type] {
		t.Fatalf("%s: the world file names %q the %q; the .od2 is %q, heroType %d", act, hero["name"], hero["class"], h.Name, h.Type)
	}
}

// faceSomewhere walks him to a clear neighbour, and back if that left him
// facing direction 0, so a facing the save drops cannot pass for one it kept.
func faceSomewhere(t *testing.T, s *session) {
	t.Helper()

	p := s.call("strigoi_get_player", map[string]any{})
	x, y := num(p, "x"), num(p, "y")
	spot := clearNeighbour(t, s, x, y)

	for _, to := range [][2]float64{spot, {x, y}} {
		s.call("strigoi_move_player_to", map[string]any{"x": to[0], "y": to[1], "wait": true, "max_ticks": 600})
		s.call("strigoi_step", map[string]any{"frames": 2})

		if num(sub(s.call("strigoi_get_player", map[string]any{}), "state"), "direction") != 0 {
			digestSettled(t, s)

			return
		}
	}

	t.Fatalf("walked to a neighbour and back, and he faces direction 0 both ways")
}

// digestSettled waits, in wall time, for two digests a moment apart to agree.
// After a walk the camera eases toward him over render frames, which run while
// the simulation is paused, and the ui provider reports the overhead bars in
// SCREEN coordinates -- so the digest's systems part moves on its own for a
// second or so, save or no save (measured 29 Sep 2026: the bar's x 385 -> 382
// -> 381 -> 380 across four digests with nothing called between them). A save
// taken then would read as one that moved the world.
func digestSettled(t *testing.T, s *session) {
	t.Helper()

	last, _ := digest(s)

	for i := 0; i < 40; i++ {
		time.Sleep(150 * time.Millisecond)

		d, _ := digest(s)
		if d == last {
			return
		}

		last = d
	}

	t.Fatalf("the digest did not settle in 6 s after a walk")
}
