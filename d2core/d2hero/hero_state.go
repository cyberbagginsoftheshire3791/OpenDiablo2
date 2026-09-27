package d2hero

import (
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2inventory"
)

// HeroState stores the state of the player
type HeroState struct {
	HeroName   string                         `json:"heroName"`
	HeroType   d2enum.Hero                    `json:"heroType"`
	Act        int                            `json:"act"`
	FilePath   string                         `json:"-"`
	Equipment  d2inventory.CharacterEquipment `json:"equipment"`
	Stats      *HeroStatsState                `json:"stats"`
	Skills     map[int]*HeroSkill             `json:"skills"`
	X          float64                        `json:"x"`
	Y          float64                        `json:"y"`
	LeftSkill  int                            `json:"leftSkill"`
	RightSkill int                            `json:"rightSkill"`
	Gold       int                            `json:"Gold"`
	Difficulty d2enum.DifficultyType          `json:"difficulty"`

	// savedSkills are an old save's Diablo II skills, read in Strigoi's game
	// -- which has no skill table to hydrate them with, and whose hero has no
	// skills -- and kept out of the hero it plays. They are written back
	// unchanged when he is saved, so a later -classic load of the same file
	// still has them (HeroStateFactory.loadSkills and Save; the tables
	// burst's review, 27 Sep 2026). Not serialised as itself.
	savedSkills map[int]*HeroSkill
}
