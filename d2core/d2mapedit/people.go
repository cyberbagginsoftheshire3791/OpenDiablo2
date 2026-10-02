package d2mapedit

import (
	"errors"
	"fmt"
	"sort"
)

// THE VILLAGE'S OBJECTS AS EDITS (the raid's R3a, 2 Oct 2026). The records the
// editor writes for a household, a hotar or a watch post, and for an npc's
// "household": the same commands a screen will run when the People tab
// places them (R3c), and the ones the raid's proposal for the village was
// written with. Like every edit they refuse only what they cannot represent;
// whether the game takes the result is Validate's (and Save's) question.

// Property is one object property as Tiled writes it: a name, a Tiled type
// ("string", "int" or "bool") and a value of that type.
type Property struct {
	Name  string
	Type  string
	Value any
}

// PlacePoint puts a point object of one of the village's classes on the map at
// x, y in world tiles (a person stands mid-tile: 20.5, 31.5), named name, with
// props. It takes the next object id the way PlaceStructure does.
func (d *Doc) PlacePoint(class, name string, x, y float64, props ...Property) (Cmd, error) {
	switch class {
	case ClassHousehold, ClassHotar, ClassWatchPost:
	default:
		return nil, fmt.Errorf("class %q is not one of the village's point objects (%s, %s, %s)", class, ClassHousehold, ClassHotar, ClassWatchPost)
	}

	if x < 0 || y < 0 || !d.m.onMap(int(x), int(y)) {
		return nil, fmt.Errorf("%.2f,%.2f is off the %dx%d map", x, y, d.m.width, d.m.height)
	}

	for _, p := range props {
		if _, err := propertyValue(p); err != nil {
			return nil, err
		}
	}

	return &placePointCmd{class: class, name: name, x: x, y: y, props: append([]Property(nil), props...)}, nil
}

type placePointCmd struct {
	class, name string
	x, y        float64
	props       []Property
	id          int
}

func (c *placePointCmd) Label() string {
	return fmt.Sprintf("place %s %q at %.1f,%.1f", c.class, c.name, c.x, c.y)
}

func (c *placePointCmd) Do(d *Doc) error {
	layer, ok := findLayer(d.tree, LayerObjects)
	if !ok {
		return errNoObjectLayer
	}

	if c.id == 0 {
		next := d.NextObjectID()
		if next <= 0 {
			return errors.New("the map's \"nextobjectid\" is missing or not a positive number, so a new object cannot be given an id Tiled will respect")
		}

		c.id = next
	}

	if _, clash := d.Object(c.id); clash {
		return fmt.Errorf("object %d already exists", c.id)
	}

	th := float64(d.m.tileHeight)

	o := newJSONObject()
	o.Set("id", num(c.id))
	o.Set("name", c.name)
	o.Set(d.m.classKey, c.class)
	o.Set("x", fnum(c.x*th))
	o.Set("y", fnum(c.y*th))
	o.Set("width", num(0))
	o.Set("height", num(0))
	o.Set("rotation", num(0))
	o.Set("visible", true)
	o.Set("point", true)

	if len(c.props) > 0 {
		list, err := propertyList(c.props)
		if err != nil {
			return err
		}

		o.Set("properties", list)
	}

	layer.Set("objects", append(fieldArray(layer, "objects"), o))
	d.tree.Set("nextobjectid", num(maxInt(d.NextObjectID(), c.id+1)))
	d.derive()

	return nil
}

func (c *placePointCmd) Undo(d *Doc) error {
	if err := d.removeObject(c.id); err != nil {
		return err
	}

	if d.NextObjectID() == c.id+1 {
		d.tree.Set("nextobjectid", num(c.id))
	}

	d.derive()

	return nil
}

// SetProperty sets one property of a point object -- replacing a property of
// that name, or adding it, the properties kept sorted by name as Tiled writes
// them. An npc's "household" is set this way.
func (d *Doc) SetProperty(id int, p Property) (Cmd, error) {
	o, ok := d.Object(id)
	if !ok {
		return nil, fmt.Errorf("no object %d", id)
	}

	if o.IsStructure() {
		return nil, fmt.Errorf("object %d is a structure; a structure takes no properties", id)
	}

	if _, err := propertyValue(p); err != nil {
		return nil, err
	}

	return &setPropertyCmd{id: id, p: p}, nil
}

type setPropertyCmd struct {
	id     int
	p      Property
	before []any // the properties as they were; nil for none
	had    bool
}

func (c *setPropertyCmd) Label() string {
	return fmt.Sprintf("set %s on object %d", c.p.Name, c.id)
}

func (c *setPropertyCmd) Do(d *Doc) error {
	raw, ok := d.rawObject(c.id)
	if !ok {
		return fmt.Errorf("no object %d", c.id)
	}

	value, err := propertyValue(c.p)
	if err != nil {
		return err
	}

	_, c.had = raw.Get("properties")
	c.before = append([]any(nil), fieldArray(raw, "properties")...)

	entry := newJSONObject()
	entry.Set("name", c.p.Name)
	entry.Set("type", c.p.Type)
	entry.Set("value", value)

	list := make([]any, 0, len(c.before)+1)

	for _, e := range c.before {
		if po, isObj := asObject(e); isObj && fieldString(po, "name") == c.p.Name {
			continue
		}

		list = append(list, e)
	}

	list = append(list, entry)
	sort.SliceStable(list, func(a, b int) bool {
		pa, _ := asObject(list[a])
		pb, _ := asObject(list[b])

		return fieldString(pa, "name") < fieldString(pb, "name")
	})

	raw.Set("properties", list)
	d.derive()

	return nil
}

func (c *setPropertyCmd) Undo(d *Doc) error {
	raw, ok := d.rawObject(c.id)
	if !ok {
		return fmt.Errorf("no object %d", c.id)
	}

	if c.had {
		raw.Set("properties", c.before)
	} else {
		raw.Delete("properties")
	}

	d.derive()

	return nil
}

// propertyValue is p's value as the tree holds it, refusing a type Tiled would
// not write for it.
func propertyValue(p Property) (any, error) {
	switch p.Type {
	case "string":
		if s, ok := p.Value.(string); ok {
			return s, nil
		}
	case "int":
		if n, ok := p.Value.(int); ok {
			return num(n), nil
		}
	case "bool":
		if b, ok := p.Value.(bool); ok {
			return b, nil
		}
	default:
		return nil, fmt.Errorf("property %q: type %q is not string, int or bool", p.Name, p.Type)
	}

	return nil, fmt.Errorf("property %q: %v is not a %s", p.Name, p.Value, p.Type)
}

func propertyList(props []Property) ([]any, error) {
	sorted := append([]Property(nil), props...)
	sort.SliceStable(sorted, func(a, b int) bool { return sorted[a].Name < sorted[b].Name })

	list := make([]any, 0, len(sorted))

	for _, p := range sorted {
		value, err := propertyValue(p)
		if err != nil {
			return nil, err
		}

		entry := newJSONObject()
		entry.Set("name", p.Name)
		entry.Set("type", p.Type)
		entry.Set("value", value)
		list = append(list, entry)
	}

	return list, nil
}
