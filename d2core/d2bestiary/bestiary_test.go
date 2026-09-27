package d2bestiary

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadIndexesCreatureByIDAndSpawnRow(t *testing.T) {
	catalog, err := Load([]byte(`{
  "creatures": [{
    "id": "feral-dog",
    "name": "Feral dog",
    "spawn_row": "dogs",
    "stand_in": "fallen1",
    "animations": {"idle": "/data/strigoi/creatures/feral-dog/idle.png"},
    "max_health": 72
  }]
}`))
	if err != nil {
		t.Fatal(err)
	}

	byID, ok := catalog.ByID("FERAL-DOG")
	if !ok || byID.MaxHealth != 72 {
		t.Fatalf("ByID = %+v, %v", byID, ok)
	}
	byRow, ok := catalog.ForSpawnRow("Dogs")
	if !ok || byRow.ID != "feral-dog" {
		t.Fatalf("ForSpawnRow = %+v, %v", byRow, ok)
	}
}

func TestLoadRejectsDuplicateSpawnRows(t *testing.T) {
	_, err := Load([]byte(`{
  "creatures": [
    {"id":"one","name":"One","spawn_row":"dogs","stand_in":"fallen1","animations":{"idle":"/one.png"},"max_health":1},
    {"id":"two","name":"Two","spawn_row":"DOGS","stand_in":"fallen1","animations":{"idle":"/two.png"},"max_health":1}
  ]
}`))
	if err == nil {
		t.Fatal("duplicate spawn_row accepted")
	}
}

func TestShippedBestiary(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "data", "strigoi", "bestiary.json"))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	dog, ok := catalog.ByID("feral-dog")
	if !ok {
		t.Fatal("shipped bestiary has no feral-dog")
	}
	if dog.SpawnRow != "dogs" || dog.MaxHealth != 72 {
		t.Fatalf("shipped feral-dog = %+v", dog)
	}
	if dog.Animations.Walk == "" || dog.Animations.Attack == "" || dog.Animations.Death == "" {
		t.Fatalf("shipped feral-dog has an incomplete animation set: %+v", dog.Animations)
	}
	wolf, ok := catalog.ByID("wolf")
	if !ok {
		t.Fatal("shipped bestiary has no wolf")
	}
	if wolf.SpawnRow != "wolves" || wolf.MaxHealth != 96 {
		t.Fatalf("shipped wolf = %+v", wolf)
	}
	if wolf.Animations.Walk == "" || wolf.Animations.Attack == "" ||
		wolf.Animations.Hit == "" || wolf.Animations.Death == "" || wolf.Animations.Dead == "" {
		t.Fatalf("shipped wolf has an incomplete animation set: %+v", wolf.Animations)
	}
	boar, ok := catalog.ByID("wild-boar")
	if !ok {
		t.Fatal("shipped bestiary has no wild-boar")
	}
	if boar.SpawnRow != "boar" || boar.MaxHealth != 120 {
		t.Fatalf("shipped wild-boar = %+v", boar)
	}
	if boar.Animations.Walk == "" || boar.Animations.Attack == "" ||
		boar.Animations.Hit == "" || boar.Animations.Death == "" || boar.Animations.Dead == "" {
		t.Fatalf("shipped wild-boar has an incomplete animation set: %+v", boar.Animations)
	}
	opportunist, ok := catalog.ByID("opportunist")
	if !ok {
		t.Fatal("shipped bestiary has no opportunist")
	}
	if opportunist.SpawnRow != "opportunists" || opportunist.MaxHealth != 84 {
		t.Fatalf("shipped opportunist = %+v", opportunist)
	}
	if opportunist.Animations.Walk == "" || opportunist.Animations.Attack == "" ||
		opportunist.Animations.Hit == "" || opportunist.Animations.Death == "" || opportunist.Animations.Dead == "" {
		t.Fatalf("shipped opportunist has an incomplete animation set: %+v", opportunist.Animations)
	}

	// M5.1b: the risen dead's own art at the numbers they had before it --
	// the men's max health (84, measured on the village). Its name is what
	// one of the dead is called AFTER the priest's tale; before it he is
	// called what he was in life (Josh's ruling of 27 Sep 2026), which is
	// the_dead_were's or his row's -- never this.
	strigoi, ok := catalog.ForSpawnRow("risen")
	if !ok {
		t.Fatal("shipped bestiary draws nothing for the risen row")
	}
	if strigoi.ID != "strigoi" || strigoi.StandIn != "fallen1" || strigoi.MaxHealth != 84 || strigoi.Name != "the dead" {
		t.Fatalf("shipped strigoi = %+v", strigoi)
	}
	were := catalog.DeadWere()
	if were.PlacedDead != "A fallen soldier" || were.Wanderer != "A stranger" {
		t.Fatalf("shipped the_dead_were = %+v, want the 27 Sep labels", were)
	}
	for _, live := range []string{were.PlacedDead, were.Wanderer, opportunist.Name} {
		if live == strigoi.Name {
			t.Fatalf("a risen man before the tale would be called %q, the dead's own name", live)
		}
	}
	for mode, path := range sheetsOf(strigoi) {
		if !strings.HasPrefix(path, "/data/strigoi/creatures/strigoi/") {
			t.Fatalf("shipped strigoi %s is drawn from %q, not its own sheets", mode, path)
		}
	}
	if len(sheetsOf(strigoi)) != 6 {
		t.Fatalf("shipped strigoi has an incomplete animation set: %+v", strigoi.Animations)
	}

	// Every creature walks at the speed it walked before speed was authored:
	// the fallen1 stand-in's SpeedBase, read off a fallen1 in the running game
	// (27 Sep). A creature that should walk faster is a new ruling, not this.
	for _, id := range []string{"feral-dog", "wolf", "wild-boar", "opportunist", "strigoi"} {
		if c, _ := catalog.ByID(id); c.Speed != 5 {
			t.Errorf("shipped %s speed = %v, want 5", id, c.Speed)
		}
	}
}

// Every sheet the shipped bestiary names is on disk. The strigoi's were
// untracked work in one tree (26 Sep): a checkout without them loads a
// bestiary whose risen cannot be drawn, and only the game would notice.
func TestShippedBestiarySheetsAreOnDisk(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "data", "strigoi", "bestiary.json"))
	if err != nil {
		t.Fatal(err)
	}

	var doc document
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}

	for _, c := range doc.Creatures {
		for mode, path := range sheetsOf(c) {
			if _, err := os.Stat(filepath.Join("..", "..", filepath.FromSlash(path))); err != nil {
				t.Errorf("%s %s: %v", c.ID, mode, err)
			}
		}
	}
}

// The loader refuses a second creature for the risen row: the dead are drawn
// from exactly one entry. The control is the same copy without a spawn row,
// which loads -- so what is refused is the row, not the copy.
func TestShippedBestiaryRefusesASecondRisen(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "data", "strigoi", "bestiary.json"))
	if err != nil {
		t.Fatal(err)
	}

	withCopy := func(spawnRow string) []byte {
		t.Helper()

		var doc struct {
			Creatures []map[string]any `json:"creatures"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}

		for _, c := range doc.Creatures {
			if c["id"] != "strigoi" {
				continue
			}

			again := map[string]any{}
			for k, v := range c {
				again[k] = v
			}

			again["id"] = "strigoi-again"
			if spawnRow == "" {
				delete(again, "spawn_row")
			} else {
				again["spawn_row"] = spawnRow
			}

			doc.Creatures = append(doc.Creatures, again)

			out, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}

			return out
		}

		t.Fatal("shipped bestiary has no strigoi to copy")

		return nil
	}

	if _, err := Load(withCopy("")); err != nil {
		t.Fatalf("control: a second strigoi on no row must load: %v", err)
	}

	_, err = Load(withCopy("RISEN"))
	if err == nil || !strings.Contains(err.Error(), `duplicate spawn_row "risen"`) {
		t.Fatalf("a second creature for the risen row must be refused, got %v", err)
	}
}

func TestLoadReadsSpeedAndFallsBackToTheStandIn(t *testing.T) {
	catalog, err := Load([]byte(`{
  "creatures": [
    {"id":"fast","name":"Fast","stand_in":"fallen1","animations":{"idle":"/f.png"},"max_health":1,"speed":7.5},
    {"id":"plain","name":"Plain","stand_in":"fallen1","animations":{"idle":"/p.png"},"max_health":1}
  ]
}`))
	if err != nil {
		t.Fatal(err)
	}

	fast, _ := catalog.ByID("fast")
	if fast.Speed != 7.5 || fast.SpeedOr(3) != 7.5 {
		t.Fatalf("an authored speed is the speed: %+v, SpeedOr(3) = %v", fast, fast.SpeedOr(3))
	}

	plain, _ := catalog.ByID("plain")
	if plain.Speed != 0 || plain.SpeedOr(3) != 3 {
		t.Fatalf("no speed authored falls back to the stand-in's: %+v, SpeedOr(3) = %v", plain, plain.SpeedOr(3))
	}
}

func TestLoadRejectsNegativeSpeed(t *testing.T) {
	_, err := Load([]byte(`{
  "creatures": [
    {"id":"back","name":"Back","stand_in":"fallen1","animations":{"idle":"/b.png"},"max_health":1,"speed":-1}
  ]
}`))
	if err == nil || !strings.Contains(err.Error(), "speed") {
		t.Fatalf("a negative speed must be refused, got %v", err)
	}
}

// sheetsOf lists a creature's authored sheets by mode.
func sheetsOf(e Entry) map[string]string {
	out := map[string]string{}

	for mode, path := range map[string]string{
		"idle": e.Animations.Idle, "walk": e.Animations.Walk, "attack": e.Animations.Attack,
		"hit": e.Animations.Hit, "death": e.Animations.Death, "dead": e.Animations.Dead,
	} {
		if path != "" {
			out[mode] = path
		}
	}

	return out
}

// the_dead_were is optional and whole: absent reads as nothing, present is
// trimmed, and half of it is refused (the game refuses a bestiary without it:
// d2gamescreen deadNamesFrom).
func TestLoadReadsTheDeadWere(t *testing.T) {
	one := `{"id":"one","name":"One","stand_in":"fallen1","animations":{"idle":"/o.png"},"max_health":1}`

	catalog, err := Load([]byte(`{"creatures":[` + one + `],
  "the_dead_were": {"placed_dead": " A fallen soldier ", "wanderer": "A stranger"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.DeadWere(); got != (DeadWere{PlacedDead: "A fallen soldier", Wanderer: "A stranger"}) {
		t.Fatalf("DeadWere = %+v", got)
	}

	bare, err := Load([]byte(`{"creatures":[` + one + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := bare.DeadWere(); got != (DeadWere{}) {
		t.Fatalf("absent the_dead_were reads as nothing, got %+v", got)
	}

	_, err = Load([]byte(`{"creatures":[` + one + `], "the_dead_were": {"placed_dead": "A fallen soldier"}}`))
	if err == nil || !strings.Contains(err.Error(), "the_dead_were") {
		t.Fatalf("half a the_dead_were must be refused, got %v", err)
	}
}
