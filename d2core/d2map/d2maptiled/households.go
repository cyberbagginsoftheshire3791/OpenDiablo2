package d2maptiled

import (
	"encoding/json"
	"fmt"
	"image"
	"strings"
)

// THE VILLAGE'S OWN OBJECTS (the raid's R3a, 2 Oct 2026; the raid brief's P1,
// "The map"): where each household keeps its door, the boundary the village
// carries its dead out to, and where its watch stands. They are point objects
// on the objects layer, refused with a named reason the way every other object
// is (see "What a map must look like"):
//
//   - "household": a point object on the household's DOOR TILE -- a standable
//     tile orthogonally beside its building: a structure's footprint, or a
//     building drawn in walls tiles (the church, the smithy), which is a wall
//     tile that is both blocked and blocks sight, so a low fence (blocked,
//     seen through) is no building. Its name is what an npc's "household"
//     names. Properties, all optional: "members" (string, the household's
//     people by role, comma-separated, from Roles; none is an empty house),
//     "incense" and "stakes" (ints from 0 to MaxHouseholdStock: what the house
//     starts with) and "church" (bool: the church, the one house that is always
//     kept; at most one). Refused: no name, a name twice, a door beside no
//     building or beside two, two households on one door tile or of one
//     building, an unknown property or role.
//   - "hotar": the village's boundary (M11's hotar), where its dead are
//     carried out to. At most one, standable, outside every inside area, no
//     properties.
//   - "watch_post": where the watch stands: one string property "post",
//     "gate" or "corner". Standable.
//   - an "npc" may carry a string "household" naming the household it belongs
//     to, so the four speakers are members of a house.
//
// The parser reads them and the game builds the households from them; it does
// not decide what a role does (the people table, R3b).

// Roles are the words a household's "members" may use (H7's cast; the raid
// brief's Q7 (b): children are full entities, and boys stand the watch).
var Roles = []string{"man", "woman", "old", "youth", "child"}

// Post kinds a watch_post may name.
const (
	PostGate   = "gate"
	PostCorner = "corner"
)

// MaxHouseholdStock bounds a household's authored incense and stakes, so a
// typo of 300 is refused, not obeyed.
const MaxHouseholdStock = 99

// Household is one household the map places.
type Household struct {
	// Name is the object's name; an npc's "household" property names it.
	Name string
	// Door is the door tile, where the household's object stands.
	Door image.Point
	// Members is the household's people by role, in the order authored; the
	// npcs that name the household are members too, and are not listed here.
	Members []string
	// Incense and Stakes are what the house starts with.
	Incense, Stakes int
	// Church marks the church.
	Church bool
}

// Post is one watch post: its tile and which way in it watches.
type Post struct {
	At   image.Point
	Post string
}

// building is what a door tile is beside: a structure's index, or the label of
// a building drawn in walls tiles.
type building struct {
	structure int // index into Map.Structures, or -1
	walls     int // a walls building's label (wallBuildings), or 0
}

func isRole(s string) bool {
	for _, r := range Roles {
		if r == s {
			return true
		}
	}

	return false
}

// household reads a household object standing at x, y.
func (p *parser) household(o *tmjObject, x, y float64) error {
	name := strings.TrimSpace(o.Name)
	if name == "" {
		return fmt.Errorf("household (object %d) has no name; an npc's \"household\" property names it", o.ID)
	}

	h := Household{Name: name}

	for _, prop := range o.Properties {
		switch prop.Name {
		case "members":
			var s string
			if prop.Type != "string" || json.Unmarshal(prop.Value, &s) != nil {
				return fmt.Errorf("household %q (object %d): members must be a string", name, o.ID)
			}

			members, err := ParseMembers(s)
			if err != nil {
				return fmt.Errorf("household %q (object %d): %v", name, o.ID, err)
			}

			h.Members = members
		case "incense", "stakes":
			var n int
			if prop.Type != "int" || json.Unmarshal(prop.Value, &n) != nil || n < 0 || n > MaxHouseholdStock {
				return fmt.Errorf("household %q (object %d): %s must be an int from 0 to %d", name, o.ID, prop.Name, MaxHouseholdStock)
			}

			if prop.Name == "incense" {
				h.Incense = n
			} else {
				h.Stakes = n
			}
		case "church":
			if prop.Type != "bool" || json.Unmarshal(prop.Value, &h.Church) != nil {
				return fmt.Errorf("household %q (object %d): church must be a bool", name, o.ID)
			}
		default:
			return fmt.Errorf("household %q (object %d): unknown property %q; a household takes \"members\", \"incense\", \"stakes\" and \"church\"",
				name, o.ID, prop.Name)
		}
	}

	if err := p.standable("household "+name, o.ID, x, y); err != nil {
		return err
	}

	h.Door = image.Pt(int(x), int(y))

	for _, other := range p.out.Households {
		switch {
		case other.Name == name:
			return fmt.Errorf("household %q (object %d): two households are named %q", name, o.ID, name)
		case other.Door == h.Door:
			return fmt.Errorf("household %q (object %d) stands on household %q's door tile %d,%d", name, o.ID, other.Name, h.Door.X, h.Door.Y)
		case h.Church && other.Church:
			return fmt.Errorf("household %q (object %d) is a second church; household %q is the church", name, o.ID, other.Name)
		}
	}

	p.out.Households = append(p.out.Households, h)
	p.householdIDs = append(p.householdIDs, o.ID)

	return nil
}

// ParseMembers reads a household's "members": roles from Roles, comma-separated
// (spaces around a role are ignored). An empty string is an empty house.
func ParseMembers(s string) ([]string, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}

	var out []string

	for _, part := range strings.Split(s, ",") {
		role := strings.TrimSpace(part)

		switch {
		case role == "":
			return nil, fmt.Errorf("members %q has an empty role; separate roles with one comma", s)
		case !isRole(role):
			return nil, fmt.Errorf("members names %q, which is not a role; the roles are %s", role, strings.Join(Roles, ", "))
		}

		out = append(out, role)
	}

	return out, nil
}

// hotar reads the boundary object.
func (p *parser) hotar(o *tmjObject, x, y float64) error {
	if len(o.Properties) > 0 {
		return fmt.Errorf("hotar (object %d) takes no properties", o.ID)
	}

	if p.out.HasHotar {
		return fmt.Errorf("hotar (object %d) is a second hotar; a village has one boundary its dead are carried to", o.ID)
	}

	if err := p.standable("hotar", o.ID, x, y); err != nil {
		return err
	}

	p.out.HasHotar, p.out.Hotar, p.hotarID = true, image.Pt(int(x), int(y)), o.ID

	return nil
}

// watchPost reads a watch post.
func (p *parser) watchPost(o *tmjObject, x, y float64) error {
	post := ""

	for _, prop := range o.Properties {
		if prop.Name != "post" {
			return fmt.Errorf("watch_post (object %d): unknown property %q; a watch_post takes \"post\"", o.ID, prop.Name)
		}

		if prop.Type != "string" || json.Unmarshal(prop.Value, &post) != nil {
			return fmt.Errorf("watch_post (object %d): post must be a string", o.ID)
		}
	}

	if post != PostGate && post != PostCorner {
		return fmt.Errorf("watch_post (object %d) has post %q; a watch_post watches the %q or the %q", o.ID, post, PostGate, PostCorner)
	}

	if err := p.standable("watch_post", o.ID, x, y); err != nil {
		return err
	}

	p.out.Posts = append(p.out.Posts, Post{At: image.Pt(int(x), int(y)), Post: post})

	return nil
}

// village is every check on the village's objects that needs the whole
// objects layer read: a hotar outside every inside area (an inside area may
// come later in the file), each household beside exactly one building and no
// two of one building, and each npc's household one the map has.
func (p *parser) village() error {
	if p.out.HasHotar && p.out.IsInside(p.out.Hotar.X, p.out.Hotar.Y) {
		return fmt.Errorf("hotar (object %d) at tile %d,%d is inside the village; the boundary is outside every inside area",
			p.hotarID, p.out.Hotar.X, p.out.Hotar.Y)
	}

	owner := map[building]int{}

	for i, h := range p.out.Households {
		bs := p.buildingsBeside(h.Door)

		switch len(bs) {
		case 0:
			return fmt.Errorf("household %q (object %d): its door tile %d,%d is beside no building; put it beside a house's footprint or a building's walls",
				h.Name, p.householdIDs[i], h.Door.X, h.Door.Y)
		case 1:
		default:
			return fmt.Errorf("household %q (object %d): its door tile %d,%d is beside %d buildings; put it beside one",
				h.Name, p.householdIDs[i], h.Door.X, h.Door.Y, len(bs))
		}

		if j, taken := owner[bs[0]]; taken {
			return fmt.Errorf("household %q (object %d) keeps the building household %q keeps; one building is one household",
				h.Name, p.householdIDs[i], p.out.Households[j].Name)
		}

		owner[bs[0]] = i
	}

	for i, n := range p.out.NPCs {
		if n.Household == "" {
			continue
		}

		found := false

		for _, h := range p.out.Households {
			found = found || h.Name == n.Household
		}

		if !found {
			return fmt.Errorf("npc %s (object %d) names household %q, which the map does not have", n.Monstat, p.npcIDs[i], n.Household)
		}
	}

	return nil
}

// buildingsBeside is every building orthogonally beside tile t, in a fixed
// order (west, east, north, south), each once.
func (p *parser) buildingsBeside(t image.Point) []building {
	var out []building

	for _, d := range []image.Point{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		n := t.Add(d)
		if n.X < 0 || n.Y < 0 || n.X >= p.out.Width || n.Y >= p.out.Height {
			continue
		}

		var b building

		switch c := p.out.At(n.X, n.Y); {
		case c.Structure > 0:
			b = building{structure: c.Structure - 1}
		case p.buildingWall(n.X, n.Y):
			b = building{structure: -1, walls: p.wallBuilding(n.X, n.Y)}
		default:
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

// buildingWall is whether x, y holds a wall tile that is part of a building:
// blocked AND blocks sight (a fence or a well, seen through, is not).
func (p *parser) buildingWall(x, y int) bool {
	c := p.out.At(x, y)

	return c.Wall >= 0 && p.out.Kinds[c.Wall].Blocked && p.out.Kinds[c.Wall].BlocksSight
}

// wallBuilding labels the building wall tile x, y belongs to: the building is
// the 4-connected run of building walls around it. Labels start at 1.
func (p *parser) wallBuilding(x, y int) int {
	if p.wallLabels == nil {
		p.wallLabels = make([]int, p.out.Width*p.out.Height)
		next := 0

		for sy := 0; sy < p.out.Height; sy++ {
			for sx := 0; sx < p.out.Width; sx++ {
				if p.wallLabels[sx+sy*p.out.Width] != 0 || !p.buildingWall(sx, sy) {
					continue
				}

				next++
				stack := []image.Point{{sx, sy}}
				p.wallLabels[sx+sy*p.out.Width] = next

				for len(stack) > 0 {
					t := stack[len(stack)-1]
					stack = stack[:len(stack)-1]

					for _, d := range []image.Point{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
						n := t.Add(d)
						if n.X < 0 || n.Y < 0 || n.X >= p.out.Width || n.Y >= p.out.Height ||
							p.wallLabels[n.X+n.Y*p.out.Width] != 0 || !p.buildingWall(n.X, n.Y) {
							continue
						}

						p.wallLabels[n.X+n.Y*p.out.Width] = next
						stack = append(stack, n)
					}
				}
			}
		}
	}

	return p.wallLabels[x+y*p.out.Width]
}
