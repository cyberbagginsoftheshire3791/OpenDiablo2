package d2hero

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2inventory"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2records"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
)

// mkdirPermission is the save directory's. The file's own is
// d2items.WriteFileAtomic's (0600), the one every Strigoi save uses.
const mkdirPermission = 0750

// NewHeroStateFactory creates a new HeroStateFactory and initializes it.
func NewHeroStateFactory(asset *d2asset.AssetManager) (*HeroStateFactory, error) {
	inventoryItemFactory, err := d2inventory.NewInventoryItemFactory(asset)
	if err != nil {
		return nil, err
	}

	factory := &HeroStateFactory{
		asset:                asset,
		InventoryItemFactory: inventoryItemFactory,
	}

	return factory, nil
}

// HeroStateFactory is responsible for creating player state objects
type HeroStateFactory struct {
	asset *d2asset.AssetManager
	*d2inventory.InventoryItemFactory
}

// CreateHeroState creates a HeroState instance and returns a pointer to it
func (f *HeroStateFactory) CreateHeroState(
	heroName string,
	hero d2enum.Hero,
	statsState *HeroStatsState,
) (*HeroState, error) {
	result := &HeroState{
		HeroName:  heroName,
		HeroType:  hero,
		Act:       1,
		Stats:     statsState,
		Equipment: f.DefaultHeroItems[hero],
		FilePath:  "",
	}

	defaultStats := f.asset.Records.Character.Stats[hero]
	skillState, err := f.CreateHeroSkillsState(defaultStats, hero)

	if err != nil {
		return nil, err
	}

	result.Skills = skillState

	return result, nil
}

// GetAllHeroStates returns all player saves
func (f *HeroStateFactory) GetAllHeroStates() ([]*HeroState, error) {
	basePath, _ := f.getGameBaseSavePath()
	files, _ := ioutil.ReadDir(basePath)
	result := make([]*HeroState, 0)

	for _, file := range files {
		fileName := file.Name()
		if file.IsDir() || len(fileName) < 5 || !strings.EqualFold(fileName[len(fileName)-4:], ".od2") {
			continue
		}

		gameState := f.LoadHeroState(filepath.Join(basePath, file.Name()))
		if gameState == nil || gameState.HeroType == d2enum.HeroNone {

		} else if gameState.Stats == nil || gameState.Skills == nil {
			// temporarily loading default class stats if the character was created before saving stats/skills was introduced
			// to be removed in the future
			gameState.Stats = f.NewHeroStats(gameState.HeroType)

			skillState, err := f.CreateHeroSkillsState(f.asset.Records.Character.Stats[gameState.HeroType], gameState.HeroType)
			if err != nil {
				return nil, err
			}

			gameState.Skills = skillState

			if err := f.Save(gameState); err != nil {
				fmt.Printf("failed to save game state!, err: %v\n", err)
			}
		}

		result = append(result, gameState)
	}

	return result, nil
}

// CreateHeroSkillsState will assemble the hero skills from the class stats record.
//
// In Strigoi's game there are none (M5.3's tables burst; Josh, 25 Sep 2026:
// the right mouse button is the torch, the left owns the blade hand, and
// shift-click does nothing). The Janissary carried Diablo II's "Attack", the
// class's base skills and ~30 amazon skills at 0 points, all read from
// skills.txt and skilldesc.txt, which his game no longer loads. -classic
// assembles them as before.
func (f *HeroStateFactory) CreateHeroSkillsState(classStats *d2records.CharStatRecord, heroType d2enum.Hero) (map[int]*HeroSkill, error) {
	baseSkills := map[int]*HeroSkill{}

	if !f.asset.Classic() {
		return baseSkills, nil
	}

	for idx := range classStats.BaseSkill {
		skillName := &classStats.BaseSkill[idx]

		if *skillName == "" {
			continue
		}

		skill, err := f.CreateHeroSkill(1, *skillName)
		if err != nil {
			continue
		}

		baseSkills[skill.ID] = skill
	}

	skillList := f.asset.Records.Skill.Details
	token := strings.ToLower(heroType.GetToken3())

	for idx := range skillList {
		if skillList[idx].Charclass == token {
			skill, _ := f.CreateHeroSkill(0, skillList[idx].Skill)
			baseSkills[skill.ID] = skill
		}
	}

	skillRecord, err := f.CreateHeroSkill(1, "Attack")
	if err != nil {
		return nil, err
	}

	baseSkills[skillRecord.ID] = skillRecord

	return baseSkills, nil
}

// CreateHeroSkill creates an instance of a skill
func (f *HeroStateFactory) CreateHeroSkill(points int, name string) (*HeroSkill, error) {
	skillRecord := f.asset.Records.GetSkillByName(name)
	if skillRecord == nil {
		return nil, fmt.Errorf("skill not found: %s", name)
	}

	skillDescRecord, found := f.asset.Records.Skill.Descriptions[skillRecord.Skilldesc]
	if !found {
		return nil, fmt.Errorf("skill Description not found: %s", name)
	}

	result := &HeroSkill{
		SkillPoints:            points,
		SkillRecord:            skillRecord,
		SkillDescriptionRecord: skillDescRecord,
		Shallow:                &shallowHeroSkill{SkillID: skillRecord.ID, SkillPoints: points},
	}

	return result, nil
}

// HasGameStates returns true if the player has any previously saved game
func (f *HeroStateFactory) HasGameStates() bool {
	basePath, _ := f.getGameBaseSavePath()
	files, _ := ioutil.ReadDir(basePath)

	return len(files) > 0
}

// CreateTestGameState is used for the map engine previewer
func (f *HeroStateFactory) CreateTestGameState() *HeroState {
	result := &HeroState{}
	return result
}

// reviveIfDead brings a loaded hero back to full health when the save records a
// death. See HeroStatsState.IsDead for the ruling. It is nil-safe: an old save
// with no stats block (handled in GetAllHeroStates) passes a nil here.
//
// Negative control: remove the reset and TestReviveIfDead / the LoadHeroState
// pin go red -- a state loaded at 0 stays at 0.
func reviveIfDead(s *HeroStatsState) {
	if s.IsDead() {
		s.Health = s.MaxHealth
	}
}

// LoadHeroState loads the player state from the file
func (f *HeroStateFactory) LoadHeroState(filePath string) *HeroState {
	strData, err := ioutil.ReadFile(filepath.Clean(filePath))
	if err != nil {
		return nil
	}

	result := &HeroState{
		FilePath: filePath,
	}

	err = json.Unmarshal(strData, result)
	if err != nil {
		return nil
	}

	// A loaded death is a new dawn, not an un-killable corpse (12 Sep 2026
	// ruling; audit A2). This is the one place every load passes through, and it
	// revives only the in-memory state -- the .od2 on disk is untouched -- so
	// "load last save means a new dawn" without rewriting the file.
	reviveIfDead(result.Stats)

	f.loadSkills(result)

	return result
}

// loadSkills turns a loaded save's skills back into records.
//
// Strigoi's game has no Diablo II skills and does not load their tables
// (CreateHeroSkillsState), so a save's skills -- every hero made before the
// tables burst carries ~30 -- are left out of the hero it plays, and kept
// aside (savedSkills) so that saving him writes them back unchanged: a later
// -classic load of the same file still has them (Save).
//
// -classic hydrates them, and gives a hero that has none (one made or saved
// by Strigoi's game) his class's skills, exactly as a new -classic hero gets
// them. That is for the SAVE and what the server sends other clients
// (AddPlayer): the hero drawn in the game is built by NewPlayer, which makes
// his class's skills afresh from the records and ignores the ones loaded
// here (d2mapentity.MapEntityFactory.NewPlayer). The review of 27 Sep 2026
// corrected the comment that said a hero without them "could not be drawn".
func (f *HeroStateFactory) loadSkills(result *HeroState) {
	if !f.asset.Classic() {
		result.savedSkills = result.Skills
		result.Skills = map[int]*HeroSkill{}

		return
	}

	// Here, we turn the Shallow skill data back into records from the asset manager.
	// This is because this factory has a reference to the asset manager with loaded records.
	// We cant do this while unmarshalling because there is no reference to the asset manager.
	for idx := range result.Skills {
		hs := result.Skills[idx]

		if hs == nil || hs.Shallow == nil {
			delete(result.Skills, idx)
			continue
		}

		hs.SkillRecord = f.asset.Records.Skill.Details[hs.Shallow.SkillID]
		if hs.SkillRecord == nil {
			delete(result.Skills, idx)
			continue
		}

		hs.SkillDescriptionRecord = f.asset.Records.Skill.Descriptions[hs.SkillRecord.Skilldesc]
		hs.SkillPoints = hs.Shallow.SkillPoints
	}

	if len(result.Skills) == 0 && result.HeroType != d2enum.HeroNone {
		if skills, err := f.CreateHeroSkillsState(f.asset.Records.Character.Stats[result.HeroType], result.HeroType); err == nil {
			result.Skills = skills
		}
	}
}

func (f *HeroStateFactory) getGameBaseSavePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "OpenDiablo2", "Saves"), nil
}

func (f *HeroStateFactory) getFirstFreeFileName() string {
	i := 0
	basePath, _ := f.getGameBaseSavePath()

	for {
		filePath := filepath.Join(basePath, strconv.Itoa(i)+".od2")
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return filePath
		}
		i++
	}
}

// onDisk is the state as its file holds it: an old save's Diablo II skills,
// kept out of Strigoi's game (savedSkills), go back into the file unchanged
// while the hero has none of his own.
func (s *HeroState) onDisk() *HeroState {
	if len(s.Skills) > 0 || len(s.savedSkills) == 0 {
		return s
	}

	out := *s
	out.Skills = s.savedSkills

	return &out
}

// Save saves the player state to a file.
//
// ATOMICALLY, AND KEEPING THE LAST GENERATION (M4.6 B3, rule 5 of the world
// save). It used to be a plain WriteFile: a crash or a full disk mid-write
// left half a hero, and nothing of the one before. Now the file already there
// is kept as N.od2.bak and the new one goes in through a temporary file and a
// rename (d2items.WriteFileAtomic, which carries the Windows lesson: the
// rename is retried while anything holds the target open, then written in
// place). The hero screen lists only *.od2, so neither N.od2.bak nor the
// temporary N.od2.tmp is ever offered as a hero.
func (f *HeroStateFactory) Save(state *HeroState) error {
	if state.FilePath == "" {
		state.FilePath = f.getFirstFreeFileName()
	}

	if err := os.MkdirAll(filepath.Dir(state.FilePath), mkdirPermission); err != nil {
		return err
	}

	fileJSON, err := json.MarshalIndent(state.onDisk(), "", "   ")
	if err != nil {
		return err
	}

	if err := d2items.KeepGeneration(state.FilePath); err != nil {
		return err
	}

	return d2items.WriteFileAtomic(state.FilePath, fileJSON)
}
