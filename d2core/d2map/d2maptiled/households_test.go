package d2maptiled

import (
	"encoding/json"
	"image"
	"reflect"
	"strings"
	"testing"
)

// THE RAID'S R3a (households.go): the village's own objects -- households,
// the hotar and the watch posts -- and an npc's household.

// The fixture's one building is the wall tile at 2,0 (house art: blocked, and
// so blocks sight). Tile 2,1 is beside it and nothing else; 1,1 is mud (a
// blocked FLOOR, no building); 3,2 has no floor.

func villageObject(id int, class, name string, x, y float64, props ...map[string]any) map[string]any {
	o := map[string]any{"id": id, "type": class, "name": name, "x": x * 80, "y": y * 80, "point": true}
	if len(props) > 0 {
		list := make([]any, len(props))
		for i, p := range props {
			list[i] = p
		}

		o["properties"] = list
	}

	return o
}

func prop(name, typ string, value any) map[string]any {
	return map[string]any{"name": name, "type": typ, "value": value}
}

func addObjects(f *fixture, objects ...map[string]any) {
	for _, o := range objects {
		f.layer(2)["objects"] = append(f.objects(), o)
	}
}

// markBuilding marks the fixture's house wall (tile 2) a building, as the
// shipped village marks its house, smithy and church walls (the R3a
// review's B2).
func markBuilding(f *fixture) {
	t := f.tile(2)
	props, _ := t["properties"].([]any)
	t["properties"] = append(props, map[string]any{"name": "building", "type": "bool", "value": true})
}

// withVillage adds a household at the wall's door (2,1) with the npc as its
// member, a hotar at 0,0 and a gate post at 3,0: every class, valid.
func withVillage(f *fixture) {
	markBuilding(f)
	addObjects(f,
		villageObject(10, "household", "the wall house", 2.5, 1.5,
			prop("members", "string", "man, woman,child"), prop("incense", "int", 3),
			prop("stakes", "int", 2), prop("church", "bool", false)),
		villageObject(11, "hotar", "the boundary", 0.5, 0.5),
		villageObject(12, "watch_post", "the gate", 3.5, 0.5, prop("post", "string", "gate")),
	)

	npc := f.objects()[1].(map[string]any)
	npc["properties"] = append(npc["properties"].([]any), prop("household", "string", "the wall house"))
}

func TestTheVillagesObjectsAreRead(t *testing.T) {
	f := newFixture(t)
	withVillage(f)

	m, err := f.parse(t)
	if err != nil {
		t.Fatalf("a valid village is refused: %v", err)
	}

	want := []Household{{Name: "the wall house", Door: image.Pt(2, 1), Members: []string{"man", "woman", "child"}, Incense: 3, Stakes: 2}}
	if !reflect.DeepEqual(m.Households, want) {
		t.Errorf("households %+v, want %+v", m.Households, want)
	}

	if !m.HasHotar || m.Hotar != image.Pt(0, 0) {
		t.Errorf("hotar %v (has %v), want 0,0", m.Hotar, m.HasHotar)
	}

	if want := []Post{{At: image.Pt(3, 0), Post: PostGate}}; !reflect.DeepEqual(m.Posts, want) {
		t.Errorf("posts %+v, want %+v", m.Posts, want)
	}

	if len(m.NPCs) != 1 || m.NPCs[0].Household != "the wall house" {
		t.Errorf("npcs %+v: the npc belongs to the wall house", m.NPCs)
	}

	// The control: a map with none of them reads none, as every map did.
	plain, err := newFixture(t).parse(t)
	if err != nil {
		t.Fatal(err)
	}

	if len(plain.Households) != 0 || plain.HasHotar || len(plain.Posts) != 0 || plain.NPCs[0].Household != "" {
		t.Errorf("a map without the village's objects reads %+v %v %+v", plain.Households, plain.HasHotar, plain.Posts)
	}

	// An empty house and a church are households too.
	g := newFixture(t)
	markBuilding(g)
	addObjects(g, villageObject(10, "household", "church", 2.5, 1.5, prop("church", "bool", true)))

	m, err = g.parse(t)
	if err != nil {
		t.Fatalf("an empty church is refused: %v", err)
	}

	if len(m.Households) != 1 || !m.Households[0].Church || m.Households[0].Members != nil {
		t.Errorf("the church reads %+v", m.Households)
	}
}

// TestTheVillagesObjectsRefuse: every refusal households.go makes, each fired
// by a map that breaks exactly it (the R3a controls). Each case starts from
// withVillage's valid map, which TestTheVillagesObjectsAreRead takes.
func TestTheVillagesObjectsRefuse(t *testing.T) {
	household := func(f *fixture) map[string]any { return f.objects()[2].(map[string]any) }
	setWall := func(f *fixture, x, y, gid int) { f.layer(1)["data"].([]any)[x+y*4] = gid }

	cases := []struct {
		name    string
		breakIt func(f *fixture)
		want    string
	}{
		{"a household with no name", func(f *fixture) { household(f)["name"] = " " }, "household (object 10) has no name"},
		{"members that are not a string", func(f *fixture) {
			household(f)["properties"].([]any)[0] = prop("members", "int", 3)
		}, "members must be a string"},
		{"a member who is no role", func(f *fixture) {
			household(f)["properties"].([]any)[0] = prop("members", "string", "man,dog")
		}, "members names \"dog\", which is not a role"},
		{"an empty role", func(f *fixture) {
			household(f)["properties"].([]any)[0] = prop("members", "string", "man,,woman")
		}, "has an empty role"},
		{"incense below zero", func(f *fixture) {
			household(f)["properties"].([]any)[1] = prop("incense", "int", -1)
		}, "incense must be an int from 0 to 99"},
		{"stakes past the stock", func(f *fixture) {
			household(f)["properties"].([]any)[2] = prop("stakes", "int", 100)
		}, "stakes must be an int from 0 to 99"},
		{"incense as a string", func(f *fixture) {
			household(f)["properties"].([]any)[1] = prop("incense", "string", "3")
		}, "incense must be an int"},
		{"church as a string", func(f *fixture) {
			household(f)["properties"].([]any)[3] = prop("church", "string", "yes")
		}, "church must be a bool"},
		{"a household with an unknown property", func(f *fixture) {
			household(f)["properties"] = append(household(f)["properties"].([]any), prop("garlic", "int", 1))
		}, "unknown property \"garlic\"; a household takes"},
		{"a household on a blocked tile", func(f *fixture) { household(f)["x"], household(f)["y"] = 1.5*80, 1.5*80 },
			"household the wall house (object 10) stands on tile 1,1, which is blocked"},
		{"a household off the map", func(f *fixture) { household(f)["x"] = 9.5 * 80 }, "is off the 4x3 map"},
		{"a door beside no building", func(f *fixture) { household(f)["x"], household(f)["y"] = 0.5*80, 2.5*80 },
			"its door tile 0,2 is beside no building"},
		{"a door beside a fence alone", func(f *fixture) {
			// mud as a wall: blocked, seen through -- a fence, not a building.
			setWall(f, 0, 1, 2)
			household(f)["x"], household(f)["y"] = 0.5*80, 0.5*80
			f.objects()[3].(map[string]any)["x"] = 1.5 * 80 // the hotar off the door
			f.objects()[3].(map[string]any)["y"] = 2.5 * 80
		}, "its door tile 0,0 is beside no building"},
		{"a door beside a wall that is no building", func(f *fixture) {
			// The house wall without "building": blocked and seen-blocking
			// (a tree is the same), but not marked -- the review's B2.
			f.tile(2)["properties"] = []any{prop("blocked", "bool", true)}
		}, "its door tile 2,1 is beside no building"},
		{"two households of one walls building two tiles wide", func(f *fixture) {
			// The wall runs 2,0-3,0; the second door is beside its other
			// tile (the review's B1). The post moves off the new wall.
			setWall(f, 3, 0, 3)
			f.objects()[4].(map[string]any)["x"], f.objects()[4].(map[string]any)["y"] = 0.5*80, 2.5*80
			addObjects(f, villageObject(13, "household", "another", 3.5, 1.5))
		}, "household \"another\" (object 13) keeps the building household \"the wall house\" keeps"},
		{"building as a string", func(f *fixture) { f.tile(2)["properties"].([]any)[1] = prop("building", "string", "yes") },
			"property \"building\" must be a bool"},
		{"a building that is not blocked", func(f *fixture) {
			f.tile(0)["properties"] = []any{prop("building", "bool", true)}
		}, "\"building\" marks a solid building; this tile is not blocked"},
		{"a building on the floor", func(f *fixture) {
			// mud, blocked, on the floor at 1,1, marked a building.
			f.tile(1)["properties"] = append(f.tile(1)["properties"].([]any), prop("building", "bool", true))
		}, "carries \"building\" but is placed on the floor layer"},
		{"incense null", func(f *fixture) {
			household(f)["properties"].([]any)[1] = prop("incense", "int", nil)
		}, "household \"the wall house\" (object 10): incense is null"},
		{"incense written 3.0", func(f *fixture) {
			household(f)["properties"].([]any)[1] = prop("incense", "int", json.RawMessage("3.0"))
		}, "incense must be an int from 0 to 99"},
		{"members null", func(f *fixture) {
			household(f)["properties"].([]any)[0] = prop("members", "string", nil)
		}, "members is null"},
		{"an npc's household null", func(f *fixture) {
			props := f.objects()[1].(map[string]any)["properties"].([]any)
			props[1] = prop("household", "string", nil)
		}, "npc (object 2): household must be a string"},
		{"a household drawn as a rectangle", func(f *fixture) {
			household(f)["width"], household(f)["height"] = 80.0, 80.0
		}, "household (object 10) is drawn as a shape"},
		{"a hotar drawn as an ellipse", func(f *fixture) { f.objects()[3].(map[string]any)["ellipse"] = true },
			"hotar (object 11) is drawn as a shape"},
		{"a watch post drawn as a polygon", func(f *fixture) {
			f.objects()[4].(map[string]any)["polygon"] = []any{map[string]any{"x": 0, "y": 0}, map[string]any{"x": 80, "y": 0}}
		}, "watch_post (object 12) is drawn as a shape"},
		{"a door beside two buildings", func(f *fixture) { setWall(f, 2, 2, 3) },
			"its door tile 2,1 is beside 2 buildings"},
		{"two households on one door tile", func(f *fixture) {
			addObjects(f, villageObject(13, "household", "another", 2.5, 1.5))
		}, "household \"another\" (object 13) stands on household \"the wall house\"'s door tile 2,1"},
		{"two households of one building", func(f *fixture) {
			addObjects(f, villageObject(13, "household", "another", 1.5, 0.5))
		}, "household \"another\" (object 13) keeps the building household \"the wall house\" keeps"},
		{"two households of one name", func(f *fixture) {
			setWall(f, 0, 1, 3)
			addObjects(f, villageObject(13, "household", "the wall house", 0.5, 2.5))
		}, "two households are named \"the wall house\""},
		{"two churches", func(f *fixture) {
			setWall(f, 0, 1, 3)
			household(f)["properties"].([]any)[3] = prop("church", "bool", true)
			addObjects(f, villageObject(13, "household", "another", 0.5, 2.5, prop("church", "bool", true)))
		}, "is a second church; household \"the wall house\" is the church"},
		{"a hotar with a property", func(f *fixture) {
			f.objects()[3].(map[string]any)["properties"] = []any{prop("post", "string", "gate")}
		}, "hotar (object 11) takes no properties"},
		{"two hotars", func(f *fixture) { addObjects(f, villageObject(13, "hotar", "another", 1.5, 2.5)) },
			"hotar (object 13) is a second hotar"},
		{"a hotar on a blocked tile", func(f *fixture) {
			f.objects()[3].(map[string]any)["x"], f.objects()[3].(map[string]any)["y"] = 1.5*80, 1.5*80
		}, "hotar (object 11) stands on tile 1,1, which is blocked"},
		{"a hotar inside the village", func(f *fixture) {
			// The inside area comes AFTER the hotar in the file: the check
			// waits for the whole layer.
			addObjects(f, map[string]any{"id": 13, "type": "inside", "x": 0.0, "y": 0.0, "width": 80.0, "height": 80.0})
		}, "hotar (object 11) at tile 0,0 is inside the village"},
		{"a watch post with no post", func(f *fixture) { delete(f.objects()[4].(map[string]any), "properties") },
			"watch_post (object 12) has post \"\""},
		{"a watch post of another post", func(f *fixture) {
			f.objects()[4].(map[string]any)["properties"] = []any{prop("post", "string", "tower")}
		}, "watch_post (object 12) has post \"tower\""},
		{"a watch post's post as an int", func(f *fixture) {
			f.objects()[4].(map[string]any)["properties"] = []any{prop("post", "int", 1)}
		}, "post must be a string"},
		{"a watch post with an unknown property", func(f *fixture) {
			f.objects()[4].(map[string]any)["properties"] = append(f.objects()[4].(map[string]any)["properties"].([]any), prop("men", "int", 2))
		}, "unknown property \"men\"; a watch_post takes \"post\""},
		{"a watch post on a blocked tile", func(f *fixture) {
			f.objects()[4].(map[string]any)["x"], f.objects()[4].(map[string]any)["y"] = 1.5*80, 1.5*80
		}, "watch_post (object 12) stands on tile 1,1, which is blocked"},
		{"an npc of a household the map does not have", func(f *fixture) {
			props := f.objects()[1].(map[string]any)["properties"].([]any)
			props[1] = prop("household", "string", "the mill")
		}, "npc warriv1 (object 2) names household \"the mill\", which the map does not have"},
		{"an npc's household as an int", func(f *fixture) {
			props := f.objects()[1].(map[string]any)["properties"].([]any)
			props[1] = prop("household", "int", 1)
		}, "npc (object 2): household must be a string"},
		{"an npc's household empty", func(f *fixture) {
			props := f.objects()[1].(map[string]any)["properties"].([]any)
			props[1] = prop("household", "string", " ")
		}, "npc (object 2): household is empty"},
		{"a class the game does not read", func(f *fixture) { f.objects()[4].(map[string]any)["type"] = "watchpost" },
			"has class \"watchpost\"; the game reads player_start, npc, inside, household, hotar and watch_post"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			withVillage(f)
			c.breakIt(f)

			_, err := f.parse(t)
			if err == nil {
				t.Fatalf("broken map (%s) was accepted", c.name)
			}

			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not say %q", err, c.want)
			}
		})
	}
}
