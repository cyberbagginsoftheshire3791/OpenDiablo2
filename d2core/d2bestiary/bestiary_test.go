package d2bestiary

import (
	"os"
	"path/filepath"
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
}
