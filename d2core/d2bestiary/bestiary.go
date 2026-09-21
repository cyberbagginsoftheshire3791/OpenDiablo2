// Package d2bestiary loads Strigoi's project-owned creature definitions.
package d2bestiary

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Entry is one authored creature. StandIn names the inherited monstats record
// used for movement and other values Strigoi has not authored yet.
type Entry struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	SpawnRow   string       `json:"spawn_row"`
	StandIn    string       `json:"stand_in"`
	Animations AnimationSet `json:"animations"`
	MaxHealth  int          `json:"max_health"`
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

// Catalog is the validated bestiary indexed by creature id and spawn row.
type Catalog struct {
	byID       map[string]Entry
	bySpawnRow map[string]Entry
}

type document struct {
	Creatures []Entry `json:"creatures"`
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

// ForSpawnRow returns the project creature replacing an inherited spawn row.
func (c *Catalog) ForSpawnRow(row string) (Entry, bool) {
	if c == nil {
		return Entry{}, false
	}
	entry, ok := c.bySpawnRow[strings.ToLower(strings.TrimSpace(row))]
	return entry, ok
}
