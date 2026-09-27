package d2hero

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2loader/asset/types"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2records"
)

// heroAssets is an asset manager over a folder holding one hero manifest,
// /hero.json, with the given body; no MPQ and no table is loaded, which is
// Strigoi's game as far as the hero is concerned.
func heroAssets(t *testing.T, manifest string) *d2asset.AssetManager {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hero.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		t.Fatal(err)
	}

	if err := asset.AddSource(dir, types.AssetSourceFileSystem); err != nil {
		t.Fatal(err)
	}

	SetHeroManifest("/hero.json")
	t.Cleanup(func() { SetHeroManifest("") })

	return asset
}

// The body a manifest gives, field by field, with charstats.txt's amazon
// (measured at a690dfc4: 240, 84, 20) for each one it leaves out.
//
// Negative control: make withDefaults keep a zero and the first case reads
// 0 health; make Body ignore the manifest and the second reads 240, not 300.
func TestBodyIsTheManifestsWithTheAmazonsDefaults(t *testing.T) {
	for _, c := range []struct {
		name, manifest string
		want           BodyStats
	}{
		{"says nothing", `{"animations":{"idle":"i.png"}}`, BodyStats{240, 84, 20}},
		{"says all three", `{"max_health":300,"max_stamina":90,"stamina_run_drain":7}`, BodyStats{300, 90, 7}},
		{"says one", `{"max_stamina":50}`, BodyStats{240, 50, 20}},
		{"negative", `{"max_health":-1,"max_stamina":50}`, BodyStats{240, 84, 20}},
		{"not a number", `{"max_health":"240"}`, BodyStats{240, 84, 20}},
		{"not json", `max_health: 300`, BodyStats{240, 84, 20}},
		// A key Body does not read is ignored HERE, though d2mapentity's
		// strict read refuses the manifest for it and draws the composite:
		// the body is still the manifest's (Body's comment, corrected by the
		// tables burst's review, 27 Sep 2026).
		{"unknown key", `{"max_health":300,"max_helth":1}`, BodyStats{300, 84, 20}},
	} {
		if got := Body(heroAssets(t, c.manifest)); got != c.want {
			t.Errorf("%s: body %+v, want %+v", c.name, got, c.want)
		}
	}
}

// No manifest at all (Diablo II's composite hero in Strigoi's game) is the
// default body, not a zero one.
func TestBodyWithoutAManifest(t *testing.T) {
	asset := heroAssets(t, `{}`)
	SetHeroManifest("")

	if got := Body(asset); got != defaultBody || got.MaxHealth != 240 {
		t.Fatalf("body %+v, want the default %+v", got, defaultBody)
	}
}

// A new hero in Strigoi's game has his manifest's health and stamina and none
// of Diablo II's attributes, mana or experience table -- read with NO table
// loaded, which is what his game now does. Before the tables burst this read
// charstats.txt's amazon row and, with none loaded, took the game down.
//
// Negative control: route NewHeroStats to CreateHeroStatsState whatever the
// launch and this panics on the nil class row.
func TestNewHeroStatsInStrigoisGameReadNoTable(t *testing.T) {
	asset := heroAssets(t, `{"max_health":250}`)
	f := &HeroStateFactory{asset: asset}

	s := f.NewHeroStats(d2enum.HeroAmazon)

	if s.MaxHealth != 250 || s.Health != 250 || s.MaxStamina != 84 || s.Stamina != 84 || s.Level != 1 {
		t.Fatalf("stats %+v; want 250/250 health and 84/84 stamina at level 1", s)
	}

	if s.Strength != 0 || s.MaxMana != 0 || s.NextLevelExp != 0 {
		t.Fatalf("stats %+v; want none of Diablo II's attributes, mana or experience table", s)
	}
}

// The run drain is the body's in Strigoi's game.
func TestStaminaRunDrainIsTheBodys(t *testing.T) {
	if got := StaminaRunDrain(heroAssets(t, `{"stamina_run_drain":7}`), d2enum.HeroAmazon); got != 7 {
		t.Fatalf("drain %v, want the manifest's 7", got)
	}

	if got := StaminaRunDrain(heroAssets(t, `{}`), d2enum.HeroAmazon); got != 20 {
		t.Fatalf("drain %v, want the default 20", got)
	}
}

// Strigoi's hero has no Diablo II skills, and assembling them reads no table.
// Before the tables burst this ranged over charstats.txt's base skills and
// asked skills.txt for "Attack"; with neither loaded it panicked.
//
// Negative control: drop the early return and this panics on the nil class
// row.
func TestStrigoisHeroHasNoDiabloSkills(t *testing.T) {
	f := &HeroStateFactory{asset: heroAssets(t, `{}`)}

	skills, err := f.CreateHeroSkillsState(nil, d2enum.HeroAmazon)
	if err != nil || len(skills) != 0 {
		t.Fatalf("skills %v, error %v; want none and no error", skills, err)
	}
}

// A save made before the tables burst carries ~30 Diablo II skills. Strigoi's
// game plays him without them (it has no records to hydrate them from), and
// does not dereference the missing records doing so.
//
// Negative control: hydrate whatever the launch and this panics on the nil
// skill record.
func TestAnOldSavesSkillsAreLeftOutOfStrigoisGame(t *testing.T) {
	dir := t.TempDir()
	save := filepath.Join(dir, "0.od2")

	old := `{"heroName":"Old","heroType":6,"act":1,"stats":{"level":1,"health":240,"maxHealth":240},` +
		`"skills":{"0":{"skillId":0,"skillPoints":1},"6":{"skillId":6,"skillPoints":0}},"leftSkill":0,"rightSkill":0}`
	if err := os.WriteFile(save, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}

	f := &HeroStateFactory{asset: heroAssets(t, `{}`)}

	state := f.LoadHeroState(save)
	if state == nil {
		t.Fatal("the old save did not load")
	}

	if len(state.Skills) != 0 {
		t.Fatalf("skills %v; want none in Strigoi's game", state.Skills)
	}
}

// ...and saving him in Strigoi's game writes them back unchanged, so a later
// -classic load of the same file still has them: Strigoi's game leaves his
// skills out, it does not delete them (the tables burst's review, 27 Sep
// 2026 -- until then the first save in Strigoi's game wrote "skills": {}).
// A hero with skills of his own (-classic's) saves those, not the kept ones.
//
// Negative control (27 Sep 2026): make Save write the state as it is in
// memory and this fails -- the file's skills are empty after the save.
func TestAnOldSavesSkillsSurviveStrigoisSave(t *testing.T) {
	save := filepath.Join(t.TempDir(), "0.od2")

	old := `{"heroName":"Old","heroType":6,"act":1,"stats":{"level":1,"health":240,"maxHealth":240},` +
		`"skills":{"0":{"skillId":0,"skillPoints":1},"6":{"skillId":6,"skillPoints":3}},"leftSkill":0,"rightSkill":6}`
	if err := os.WriteFile(save, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}

	f := &HeroStateFactory{asset: heroAssets(t, `{}`)}

	state := f.LoadHeroState(save)
	if state == nil || len(state.Skills) != 0 {
		t.Fatalf("the old save loaded as %+v; want him without skills in memory", state)
	}

	if err := f.Save(state); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(save)
	if err != nil {
		t.Fatal(err)
	}

	var onDisk struct {
		Skills     map[int]*shallowHeroSkill `json:"skills"`
		RightSkill int                       `json:"rightSkill"`
	}

	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatal(err)
	}

	if len(onDisk.Skills) != 2 || onDisk.Skills[0] == nil || onDisk.Skills[0].SkillPoints != 1 ||
		onDisk.Skills[6] == nil || onDisk.Skills[6].SkillPoints != 3 || onDisk.RightSkill != 6 {
		t.Fatalf("after Strigoi's save the file holds skills %v (right %d); want the old save's two, unchanged:\n%s",
			onDisk.Skills, onDisk.RightSkill, data)
	}

	// A hero with skills of his own saves his own.
	state.Skills = map[int]*HeroSkill{9: {Shallow: &shallowHeroSkill{SkillID: 9, SkillPoints: 1}}}
	if disk := state.onDisk(); len(disk.Skills) != 1 || disk.Skills[9] == nil {
		t.Fatalf("a hero with his own skills saves %v", disk.Skills)
	}
}

// -classic is Diablo II's game: a new hero's body is his class's charstats.txt
// row, whatever a hero manifest says, and so is his run drain. (The row here
// is the amazon's as measured at a690dfc4, with the drain changed so the row
// and the default cannot be mistaken for each other.)
//
// Negative control: make NewHeroStats or StaminaRunDrain read the body
// whatever the launch and this reads the manifest's 999, or the default 20.
func TestUnderClassicTheBodyIsTheClassRow(t *testing.T) {
	asset := heroAssets(t, `{"max_health":999}`)
	asset.SetClassic(true)
	asset.Records.Character.Stats = d2records.CharStats{d2enum.HeroAmazon: {
		InitStr: 20, InitDex: 25, InitVit: 20, InitEne: 15, LifePerVit: 12, ManaPerEne: 6,
		InitStamina: 84, StaminaRunDrain: 33,
	}}

	s := (&HeroStateFactory{asset: asset}).NewHeroStats(d2enum.HeroAmazon)

	if s.MaxHealth != 240 || s.MaxStamina != 84 || s.Strength != 20 || s.MaxMana != 90 {
		t.Fatalf("stats %+v; want the class row's 240 health, 84 stamina, 20 strength and 90 mana", s)
	}

	if got := StaminaRunDrain(asset, d2enum.HeroAmazon); got != 33 {
		t.Fatalf("drain %v, want the class row's 33", got)
	}
}
