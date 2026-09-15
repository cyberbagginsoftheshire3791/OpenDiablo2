package d2hero

import (
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2records"
)

// HeroStatsState is a serializable state of hero stats.
type HeroStatsState struct {
	Level      int `json:"level"`
	Experience int `json:"experience"`

	Strength  int `json:"strength"`
	Energy    int `json:"energy"`
	Dexterity int `json:"dexterity"`
	Vitality  int `json:"vitality"`
	// there are stats and skills points remaining to add.
	StatsPoints int `json:"statsPoints"`
	SkillPoints int `json:"skillPoints"`

	Health     int     `json:"health"`
	MaxHealth  int     `json:"maxHealth"`
	Mana       int     `json:"mana"`
	MaxMana    int     `json:"maxMana"`
	Stamina    float64 `json:"-"` // only MaxStamina is saved, Stamina gets reset on entering world
	MaxStamina int     `json:"maxStamina"`

	// values which are not saved/loaded(computed)
	NextLevelExp int `json:"-"`
}

// CreateHeroStatsState generates a running state from a hero stats.
func (f *HeroStateFactory) CreateHeroStatsState(heroClass d2enum.Hero, classStats *d2records.CharStatRecord) *HeroStatsState {
	result := HeroStatsState{
		Level:        1,
		Experience:   0,
		NextLevelExp: f.asset.Records.GetExperienceBreakpoint(heroClass, 1),
		Strength:     classStats.InitStr,
		Dexterity:    classStats.InitDex,
		Vitality:     classStats.InitVit,
		Energy:       classStats.InitEne,
		StatsPoints:  0,
		SkillPoints:  0,

		MaxHealth:  classStats.InitVit * classStats.LifePerVit,
		MaxMana:    classStats.InitEne * classStats.ManaPerEne,
		MaxStamina: classStats.InitStamina,
		// https://github.com/OpenDiablo2/OpenDiablo2/issues/814
	}

	result.Mana = result.MaxMana
	result.Health = result.MaxHealth
	result.Stamina = float64(result.MaxStamina)

	return &result
}

// IsDead reports whether these stats describe a hero at or below zero health.
// It is nil-safe so callers need not guard a missing stats block.
//
// Ruled 12 Sep 2026: friends build #1 has no mid-run save, so "load last save"
// is a new dawn -- a saved 0-HP hero must never persist as an un-killable,
// un-feedable corpse (audit A2). This predicate is what the save guard
// (OnUnload) and the load reset (reviveIfDead) both ask. A real world save
// (clock, meters, day index, open bodies) is owed to a later milestone.
func (s *HeroStatsState) IsDead() bool {
	return s != nil && s.Health <= 0
}
