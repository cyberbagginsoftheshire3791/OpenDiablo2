package d2mapentity

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// M4.6 B2b review, B5: every field of the three structs a Motion is taken
// from -- mapEntity, Creature and NPC -- is labelled, as B2a's systems are
// (d2world's b2aClassified), so a field added later fails here until someone
// decides whether the save carries it:
//
//	"S:<key>"  saved at that top-level key of Motion's JSON, and exercised
//	           by a mutation in the motion sweeps (TestCreatureMotionEveryFieldIsSeen,
//	           TestNPCMotionCarriesItsPose) -- a label nobody checks is not one
//	"D: why"   derived on load (rebuilt by the constructor, or by the restore)
//	"W: why"   wiring: a function, an interface, loaded art
//
// There is no "T" here: nothing a Motion is taken from is refused at save;
// what would be (a walk's arrival callback in a paced fight) is a function,
// and a function is wiring.

type b2bMotionClass struct {
	kind   interface{}
	fields map[string]string
}

func b2bMapEntityClass() b2bMotionClass {
	return b2bMotionClass{mapEntity{}, map[string]string{
		"uuid":        "D: the entity's id, given back by the next-id seam at rebuild -- the entity list's, not the motion's",
		"Position":    "S:pos",
		"Target":      "S:target",
		"velocity":    "S:velocity",
		"Speed":       "S:speed",
		"path":        "S:path",
		"drawLayer":   "D: never set on a creature or an NPC; zero as the constructor builds it",
		"done":        "W: the arrival callback, a function: nil on a chase's walk, and a no-op pace return on a paced fight's once returnAllPace has run, which it has whenever a save is allowed",
		"directioner": "W: the facing hook the NPC constructor installs (rotate)",
		"highlight":   "D: a render flag, cleared every frame",
	}}
}

func b2bCreatureClass() b2bMotionClass {
	return b2bMotionClass{Creature{}, map[string]string{
		"mapEntity":  "W: embedded; its fields are labelled as mapEntity's",
		"name":       "D: the bestiary's name, given by NewCreature at rebuild",
		"animations": "W: the sheets NewCreature loads",
		"animation":  "S:action_at", // while an action is held, its frame and the time into it (BUG-87); the sheet itself is set by setMode from mode and action, and a sheet that is not held restarts at its first frame
		"mode":       "S:mode",
		"direction":  "S:dir",
		"held":       "S:action",
		"heldMode":   "S:action",
		"finished":   "W: a held action's callback, a function; the game starts every action with nil",
		"corpse":     "S:corpse",
		"sheets":     "D: the path each mode was loaded from, set by NewCreature at rebuild",
		"creatureID": "D: the bestiary entry, saved in the world file's entity list (entities[].creature, M4.6 B3), not in the motion; the load rebuilds the creature from it",
	}}
}

func b2bNPCClass() b2bMotionClass {
	return b2bMotionClass{NPC{}, map[string]string{
		"mapEntity":      "W: embedded; its fields are labelled as mapEntity's",
		"Paths":          "D: a villager's patrol, rebuilt by the map with the villagers (build plan section 1)",
		"name":           "D: the monstat's name, given by NewNPC at rebuild",
		"composite":      "S:action_at", // rebuilt by NewNPC from the monstat; its mode and facing are saved as mode and dir, and while an action is held its mode's frame and time as action_at (BUG-87), and set back on it (needs MPQs: the playtests carry it; d2asset's TestACompositeResumesAtItsProgress the frame)
		"action":         "D: a villager's patrol step; only villagers patrol",
		"path":           "D: a villager's patrol index; only villagers patrol",
		"repetitions":    "D: a villager's patrol count; only villagers patrol",
		"rng":            "D: the per-NPC behaviour stream, re-seeded at rebuild; it has no effect (build plan section 1)",
		"monstatRecord":  "D: the monstat, given to NewNPC at rebuild",
		"monstatEx":      "D: the monstat's extra record, looked up by NewNPC at rebuild",
		"HasPaths":       "D: whether a villager patrols; rebuilt by the map",
		"isDone":         "D: a villager's patrol flag; only villagers patrol",
		"held":           "S:action",
		"heldMode":       "S:action",
		"onHeldFinished": "W: a held action's callback, a function; the game starts every action with nil",
		"corpse":         "S:corpse",
	}}
}

// b2bMotionKeys is Motion's top-level JSON keys.
func b2bMotionKeys(t *testing.T) map[string]bool {
	t.Helper()

	raw, err := json.Marshal(Motion{Action: "x", ActionAt: &ActionProgress{}, Corpse: true}) // the omitempty keys written too
	require.NoError(t, err)

	var tree map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &tree))

	keys := map[string]bool{}
	for k := range tree {
		keys[k] = true
	}

	return keys
}

// b2bMotionKeysChanged is the JSON key of every Motion field that differs
// between a and b.
func b2bMotionKeysChanged(a, b Motion) []string {
	var out []string

	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)

	for i := 0; i < va.NumField(); i++ {
		if !reflect.DeepEqual(va.Field(i).Interface(), vb.Field(i).Interface()) {
			out = append(out, strings.Split(va.Type().Field(i).Tag.Get("json"), ",")[0])
		}
	}

	return out
}

// b2bMotionClassified requires every field of the class's struct to carry a
// label with a reason, every S key to be one of Motion's, and every label to
// name a field.
func b2bMotionClassified(t *testing.T, c b2bMotionClass) {
	t.Helper()

	typ := reflect.TypeOf(c.kind)
	keys := b2bMotionKeys(t)
	have := map[string]bool{}

	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		have[name] = true

		class, ok := c.fields[name]

		switch {
		case !ok:
			t.Errorf("%s.%s is not labelled: decide whether the save carries it (S), derives it (D), or it is wiring (W)",
				typ.Name(), name)
		case strings.HasPrefix(class, "S:"):
			if !keys[strings.TrimPrefix(class, "S:")] {
				t.Errorf("%s.%s is saved at %q, which Motion does not write", typ.Name(), name, class)
			}
		case strings.HasPrefix(class, "D: "), strings.HasPrefix(class, "W: "):
			if len(strings.TrimSpace(class[3:])) < 10 {
				t.Errorf("%s.%s: a %c label needs its reason", typ.Name(), name, class[0])
			}
		default:
			t.Errorf("%s.%s: label %q is not S:, D: or W:", typ.Name(), name, class)
		}
	}

	for name := range c.fields {
		if !have[name] {
			t.Errorf("%s.%s is labelled but is not a field", typ.Name(), name)
		}
	}
}

// b2bMotionExercised requires every S key of the classes to be one a sweep's
// mutation changed (seen).
func b2bMotionExercised(t *testing.T, seen map[string]bool, classes ...b2bMotionClass) {
	t.Helper()

	for _, c := range classes {
		for name, class := range c.fields {
			if key := strings.TrimPrefix(class, "S:"); key != class && !seen[key] {
				t.Errorf("%s.%s is saved at %q, and no mutation in its sweep changed that key: the label is unchecked",
					reflect.TypeOf(c.kind).Name(), name, key)
			}
		}
	}
}

func TestMotionFieldsClassified(t *testing.T) {
	for _, c := range []b2bMotionClass{b2bMapEntityClass(), b2bCreatureClass(), b2bNPCClass()} {
		b2bMotionClassified(t, c)
	}
}
