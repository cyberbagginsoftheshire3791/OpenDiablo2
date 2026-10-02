package d2mapedit

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
)

// THE VILLAGE'S OWN OBJECTS, MIRRORED (the raid's R3a, 2 Oct 2026; the raid
// brief's A8): the loader reads household, hotar and watch_post point objects
// and an npc's "household" (d2maptiled/households.go), so the validator refuses
// what it refuses -- otherwise the first household on the village turns every
// editor test that saves the shipped map red, and a map the game would refuse
// could be saved. Each rule names the loader's own refusal.

// The village's object classes (d2maptiled/households.go).
const (
	ClassHousehold = "household"
	ClassHotar     = "hotar"
	ClassWatchPost = "watch_post"
)

// The rules the village's objects add.
const (
	RuleHousehold Rule = "household"
	RuleHotar     Rule = "hotar"
	RuleWatchPost Rule = "watch-post"
)

// npcProperties is the loader's npcProperties (d2maptiled): "monstat"
// (required) and "household" (optional; checked against the households once
// the whole layer is read).
func npcProperties(props []any) (monstat, household string, err error) {
	for _, raw := range props {
		p, ok := asObject(raw)
		if !ok {
			return "", "", errors.New("a property is not a JSON object")
		}

		value, _ := p.Get("value")
		s, isString := asString(value)

		switch name := fieldString(p, "name"); name {
		case "monstat":
			if fieldString(p, "type") != "string" || !isString {
				return "", "", errors.New("monstat must be a string")
			}

			monstat = s
		case "household":
			if fieldString(p, "type") != "string" || !isString {
				return "", "", errors.New("household must be a string")
			}

			household = strings.TrimSpace(s)
			if household == "" {
				return "", "", errors.New("household is empty; name a household or remove the property")
			}
		default:
			return "", "", fmt.Errorf("unknown property %q; an npc takes \"monstat\" and \"household\"", name)
		}
	}

	monstat = strings.TrimSpace(monstat)
	if monstat == "" {
		return "", "", errors.New("no monstat property naming who stands here")
	}

	return monstat, household, nil
}

// householdProperties is the loader's reading of a household's properties.
func householdProperties(o *Object) error {
	for _, raw := range o.props {
		p, ok := asObject(raw)
		if !ok {
			return errors.New("a property is not a JSON object")
		}

		value, _ := p.Get("value")
		typ := fieldString(p, "type")

		switch name := fieldString(p, "name"); name {
		case "members":
			s, isString := asString(value)
			if typ != "string" || !isString {
				return errors.New("members must be a string")
			}

			members, err := d2maptiled.ParseMembers(s)
			if err != nil {
				return err
			}

			o.Members = members
		case "incense", "stakes":
			n, isInt := asStrictInt(value)
			if typ != "int" || !isInt || n < 0 || n > d2maptiled.MaxHouseholdStock {
				return fmt.Errorf("%s must be an int from 0 to %d", name, d2maptiled.MaxHouseholdStock)
			}

			if name == "incense" {
				o.Incense = n
			} else {
				o.Stakes = n
			}
		case "church":
			b, isBool := asBool(value)
			if typ != "bool" || !isBool {
				return errors.New("church must be a bool")
			}

			o.Church = b
		default:
			return fmt.Errorf("unknown property %q; a household takes \"members\", \"incense\", \"stakes\" and \"church\"", name)
		}
	}

	return nil
}

// asStrictInt is an int written as the loader's json.Unmarshal into an int
// takes it: digits, no fraction and no exponent ("3.0" and "3e0" are refused
// there, the R3a review's C1).
func asStrictInt(v any) (int, bool) {
	if n, ok := v.(json.Number); ok {
		t := n.String()
		if strings.HasPrefix(t, "-") {
			t = t[1:]
		}

		if t == "" || strings.Trim(t, "0123456789") != "" {
			return 0, false
		}
	}

	return asInt(v)
}

// pointOnly is the loader's: the village's objects are points (C2).
func (v *validation) pointOnly(class string, o Object) bool {
	shaped := o.W != 0 || o.H != 0 || o.Ellipse

	if raw, ok := v.doc.rawObject(o.ID); ok {
		for _, key := range []string{"polygon", "polyline"} {
			if val, has := raw.Get(key); has && val != nil {
				shaped = true
			}
		}
	}

	if shaped {
		v.add(Problem{Rule: RuleObjectSize, Object: o.ID, Msg: class + " is drawn as a shape; place it as a point (Tiled's point tool)"})
	}

	return !shaped
}

// standsWell is whether o stands on the map on a tile someone can stand on:
// the loader stops at the first refusal, so a door that is off the map or
// blocked is checked no further (the R3a review's C3).
func (v *validation) standsWell(o Object) bool {
	t := o.Tile()

	return o.X >= 0 && o.Y >= 0 && v.doc.m.onMap(t.X, t.Y) && !v.doc.Blocked(t.X, t.Y)
}

// postProperty is the loader's reading of a watch post's one property.
func postProperty(props []any) (string, error) {
	post := ""

	for _, raw := range props {
		p, ok := asObject(raw)
		if !ok {
			return "", errors.New("a property is not a JSON object")
		}

		if name := fieldString(p, "name"); name != "post" {
			return "", fmt.Errorf("unknown property %q; a watch_post takes \"post\"", name)
		}

		value, _ := p.Get("value")

		s, isString := asString(value)
		if fieldString(p, "type") != "string" || !isString {
			return "", errors.New("post must be a string")
		}

		post = s
	}

	if post != d2maptiled.PostGate && post != d2maptiled.PostCorner {
		return "", fmt.Errorf("has post %q; a watch_post watches the %q or the %q", post, d2maptiled.PostGate, d2maptiled.PostCorner)
	}

	return post, nil
}

// villageCheck gathers the village's objects as the validator meets them, for
// the checks the loader makes once the whole layer is read.
type villageCheck struct {
	households []Object
	hotars     []Object
	npcs       []Object
}

// household is the loader's household(): its properties, its name, where it
// stands, and no second of the same name, door tile or church.
func (v *validation) household(o Object, vc *villageCheck) {
	name := strings.TrimSpace(o.Name)
	if name == "" {
		v.add(Problem{Rule: RuleHousehold, Object: o.ID, Msg: "household has no name; an npc's \"household\" property names it"})

		return
	}

	v.pointOnly("household", o)

	if o.propErr != nil {
		v.add(Problem{Rule: RuleHousehold, Object: o.ID, Msg: fmt.Sprintf("household %q: %v", name, o.propErr)})
	}

	v.standable("household "+name, o)

	door := o.Tile()

	for _, other := range vc.households {
		switch {
		case strings.TrimSpace(other.Name) == name:
			v.add(Problem{Rule: RuleHousehold, Object: o.ID, Msg: fmt.Sprintf("two households are named %q", name)})
		case other.Tile() == door:
			v.tile(Problem{Rule: RuleHousehold, Object: o.ID, Msg: fmt.Sprintf("household %q stands on household %q's door tile", name, other.Name)},
				door.X, door.Y)
		case o.Church && other.Church:
			v.add(Problem{Rule: RuleHousehold, Object: o.ID, Msg: fmt.Sprintf("household %q is a second church; household %q is the church", name, other.Name)})
		}
	}

	vc.households = append(vc.households, o)
}

// hotar is the loader's hotar(): no properties, one of them, standable.
func (v *validation) hotar(o Object, vc *villageCheck) {
	v.pointOnly("hotar", o)

	if len(o.props) > 0 {
		v.add(Problem{Rule: RuleObjectProps, Object: o.ID, Msg: "hotar takes no properties"})
	}

	if len(vc.hotars) > 0 {
		v.add(Problem{Rule: RuleHotar, Object: o.ID, Msg: "a second hotar; a village has one boundary its dead are carried to"})
	}

	v.standable("hotar", o)
	vc.hotars = append(vc.hotars, o)
}

// watchPost is the loader's watchPost(): its post, standable.
func (v *validation) watchPost(o Object) {
	v.pointOnly("watch_post", o)

	if o.propErr != nil {
		v.add(Problem{Rule: RuleWatchPost, Object: o.ID, Msg: fmt.Sprintf("watch_post %v", o.propErr)})
	}

	v.standable("watch_post", o)
}

// village is the loader's village(): the checks that need the whole layer.
func (v *validation) village(vc *villageCheck) {
	for _, h := range vc.hotars {
		if t := h.Tile(); v.standsWell(h) && v.doc.Inside(t.X, t.Y) {
			v.tile(Problem{Rule: RuleHotar, Object: h.ID, Msg: "hotar is inside the village; the boundary is outside every inside area"}, t.X, t.Y)
		}
	}

	owner := map[building]Object{}

	for _, h := range vc.households {
		if !v.standsWell(h) {
			continue // reported where it stands; the loader looks no further (C3)
		}

		door := h.Tile()

		bs := v.buildingsBeside(door)

		switch len(bs) {
		case 0:
			v.tile(Problem{Rule: RuleHousehold, Object: h.ID, Msg: fmt.Sprintf(
				"household %q: its door tile is beside no building; put it beside a house's footprint or a building's walls", h.Name)}, door.X, door.Y)

			continue
		case 1:
		default:
			v.tile(Problem{Rule: RuleHousehold, Object: h.ID, Msg: fmt.Sprintf(
				"household %q: its door tile is beside %d buildings; put it beside one", h.Name, len(bs))}, door.X, door.Y)

			continue
		}

		if other, taken := owner[bs[0]]; taken {
			v.add(Problem{Rule: RuleHousehold, Object: h.ID, Msg: fmt.Sprintf(
				"household %q keeps the building household %q keeps; one building is one household", h.Name, other.Name)})

			continue
		}

		owner[bs[0]] = h
	}

	for _, n := range vc.npcs {
		if n.Household == "" {
			continue
		}

		found := false

		for _, h := range vc.households {
			found = found || strings.TrimSpace(h.Name) == n.Household
		}

		if !found {
			v.add(Problem{Rule: RuleNPC, Object: n.ID, Msg: fmt.Sprintf("npc %s names household %q, which the map does not have", n.Monstat, n.Household)})
		}
	}
}

// building is what a door tile is beside: a structure (by its object id), or
// a building drawn in walls tiles (by any one of its tiles, the first found).
type building struct {
	structure int
	walls     image.Point
}

// buildingsBeside is the loader's buildingsBeside: the buildings orthogonally
// beside t, each once.
func (v *validation) buildingsBeside(t image.Point) []building {
	var out []building

	for _, d := range []image.Point{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		n := t.Add(d)
		if !v.doc.m.onMap(n.X, n.Y) {
			continue
		}

		var b building

		if s, on := v.doc.StructureOn(n.X, n.Y); on {
			if k, known := v.doc.Kind(s.GID); !known || !k.Building {
				continue // a tower, or a structure that says it is none
			}

			b = building{structure: s.ID}
		} else if v.buildingWall(n.X, n.Y) {
			b = building{walls: v.wallBuildingOf(n)}
		} else {
			continue
		}

		dup := false
		for _, o := range out {
			dup = dup || o == b
		}

		if !dup {
			out = append(out, b)
		}
	}

	return out
}

// buildingWall is the loader's: a wall tile whose tile says "building".
func (v *validation) buildingWall(x, y int) bool {
	gid := v.doc.WallTile(x, y)
	if gid == 0 {
		return false
	}

	if _, on := v.doc.StructureOn(x, y); on {
		return false
	}

	k, ok := v.doc.Kind(gid)

	return ok && k.Building
}

// wallBuildingOf names the walls building tile t belongs to by the first of
// its tiles in row order (the 4-connected run of building walls around t).
func (v *validation) wallBuildingOf(t image.Point) image.Point {
	seen := map[image.Point]bool{t: true}
	stack := []image.Point{t}
	first := t

	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if p.Y < first.Y || (p.Y == first.Y && p.X < first.X) {
			first = p
		}

		for _, d := range []image.Point{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
			n := p.Add(d)
			if seen[n] || !v.doc.m.onMap(n.X, n.Y) || !v.buildingWall(n.X, n.Y) {
				continue
			}

			seen[n] = true
			stack = append(stack, n)
		}
	}

	return first
}
