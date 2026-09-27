// Package d2bestiary loads Strigoi's project-owned creature definitions.
package d2bestiary

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Entry is one authored creature. StandIn names the inherited monstats record
// used for the values Strigoi has not authored yet.
//
// Speed is how fast it walks, in the map engine's movement units (the unit of
// monstats' Velocity, which the stand-in supplied before M5.1b). Absent or
// zero, the stand-in's SpeedBase is used -- see SpeedOr.
type Entry struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	SpawnRow   string       `json:"spawn_row"`
	StandIn    string       `json:"stand_in"`
	Animations AnimationSet `json:"animations"`
	MaxHealth  int          `json:"max_health"`
	Speed      float64      `json:"speed,omitempty"`
}

// SpeedOr is the creature's walking speed: its authored Speed, or standIn --
// the stand-in record's SpeedBase -- when none is authored.
func (e Entry) SpeedOr(standIn float64) float64 {
	if e.Speed > 0 {
		return e.Speed
	}

	return standIn
}

// AnimationSet names the independently sized sprite sheet for each mode.
// Idle is required; the rest may be added as a creature's art matures.
type AnimationSet struct {
	Idle   string `json:"idle"`
	Walk   string `json:"walk,omitempty"`
	Attack string `json:"attack,omitempty"`
	Hit    string `json:"hit,omitempty"`
	Death  string `json:"death,omitempty"`
	Dead   string `json:"dead,omitempty"`
}

// DeadWere is what a risen man is called before the priest's tale when his
// body came from no spawn row (Josh's ruling of 27 Sep 2026: before the tale
// the hover names a risen man by what he was in life). A man the night slew
// is called by his row's creature's name instead, and after the tale every
// risen man is called by the risen row's creature's name. The words are data
// so they are renamed here, not in code.
type DeadWere struct {
	// PlacedDead is Night 1's dead, the Janissary's comrades ("A fallen
	// soldier").
	PlacedDead string `json:"placed_dead"`
	// Wanderer is the edge floor's nameless dead, whom he cannot know ("A
	// stranger").
	Wanderer string `json:"wanderer"`
}

// Catalog is the validated bestiary indexed by creature id and spawn row.
type Catalog struct {
	byID       map[string]Entry
	bySpawnRow map[string]Entry
	deadWere   DeadWere
}

type document struct {
	Creatures []Entry `json:"creatures"`
	// DeadWere is optional here -- a bestiary need not have the dead -- and
	// whole when present. The game requires it (d2gamescreen deadNamesFrom).
	DeadWere *DeadWere `json:"the_dead_were,omitempty"`
}

// Load parses and validates a bestiary document.
func Load(data []byte) (*Catalog, error) {
	var doc document
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode bestiary: %w", err)
	}
	if len(doc.Creatures) == 0 {
		return nil, fmt.Errorf("bestiary has no creatures")
	}

	catalog := &Catalog{
		byID:       make(map[string]Entry, len(doc.Creatures)),
		bySpawnRow: make(map[string]Entry, len(doc.Creatures)),
	}

	if doc.DeadWere != nil {
		catalog.deadWere = DeadWere{
			PlacedDead: strings.TrimSpace(doc.DeadWere.PlacedDead),
			Wanderer:   strings.TrimSpace(doc.DeadWere.Wanderer),
		}
		if catalog.deadWere.PlacedDead == "" || catalog.deadWere.Wanderer == "" {
			return nil, fmt.Errorf("the_dead_were requires placed_dead and wanderer")
		}
	}

	for index, entry := range doc.Creatures {
		entry.ID = strings.ToLower(strings.TrimSpace(entry.ID))
		entry.Name = strings.TrimSpace(entry.Name)
		entry.SpawnRow = strings.ToLower(strings.TrimSpace(entry.SpawnRow))
		entry.StandIn = strings.TrimSpace(entry.StandIn)
		entry.Animations.Idle = strings.TrimSpace(entry.Animations.Idle)
		entry.Animations.Walk = strings.TrimSpace(entry.Animations.Walk)
		entry.Animations.Attack = strings.TrimSpace(entry.Animations.Attack)
		entry.Animations.Hit = strings.TrimSpace(entry.Animations.Hit)
		entry.Animations.Death = strings.TrimSpace(entry.Animations.Death)
		entry.Animations.Dead = strings.TrimSpace(entry.Animations.Dead)

		if entry.ID == "" || entry.Name == "" || entry.StandIn == "" || entry.Animations.Idle == "" {
			return nil, fmt.Errorf("creature %d requires id, name, stand_in, and idle", index)
		}
		for mode, path := range map[string]string{
			"idle": entry.Animations.Idle, "walk": entry.Animations.Walk,
			"attack": entry.Animations.Attack, "hit": entry.Animations.Hit,
			"death": entry.Animations.Death, "dead": entry.Animations.Dead,
		} {
			if path != "" && !strings.HasPrefix(path, "/") {
				return nil, fmt.Errorf("creature %q %s path must begin with /", entry.ID, mode)
			}
		}
		if entry.MaxHealth <= 0 {
			return nil, fmt.Errorf("creature %q max_health must be positive", entry.ID)
		}
		if entry.Speed < 0 {
			return nil, fmt.Errorf("creature %q speed must not be negative", entry.ID)
		}
		if _, exists := catalog.byID[entry.ID]; exists {
			return nil, fmt.Errorf("duplicate creature id %q", entry.ID)
		}
		if entry.SpawnRow != "" {
			if _, exists := catalog.bySpawnRow[entry.SpawnRow]; exists {
				return nil, fmt.Errorf("duplicate spawn_row %q", entry.SpawnRow)
			}
			catalog.bySpawnRow[entry.SpawnRow] = entry
		}
		catalog.byID[entry.ID] = entry
	}

	return catalog, nil
}

// ByID returns a creature used by commands and authored placements.
func (c *Catalog) ByID(id string) (Entry, bool) {
	if c == nil {
		return Entry{}, false
	}
	entry, ok := c.byID[strings.ToLower(strings.TrimSpace(id))]
	return entry, ok
}

// DeadWere is what a risen man is called before the priest's tale when no
// spawn row names him; the zero value when the bestiary has no the_dead_were.
func (c *Catalog) DeadWere() DeadWere {
	if c == nil {
		return DeadWere{}
	}
	return c.deadWere
}

// ForSpawnRow returns the project creature replacing an inherited spawn row.
func (c *Catalog) ForSpawnRow(row string) (Entry, bool) {
	if c == nil {
		return Entry{}, false
	}
	entry, ok := c.bySpawnRow[strings.ToLower(strings.TrimSpace(row))]
	return entry, ok
}
