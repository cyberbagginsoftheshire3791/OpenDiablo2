package d2mapedit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// THE WHOLE JSON TREE IS KEPT, IN THE ORDER THE FILE WROTE IT.
//
// The .tmj is the authoring file: there is no second format, so a save must
// give Tiled and the game back everything they put in -- nextlayerid,
// nextobjectid, compressionlevel, tiledversion, draworder, a tile object's
// width and height, renderorder, and whatever a later Tiled adds that this
// package has never heard of. A read model of the parts the editor understands
// would quietly drop the rest.
//
// Order is kept for a different reason: a diff. village.tmj is 1115 lines, and
// its key order is not one rule -- the parts Tiled (and tools/villagemap)
// write from a struct are in the struct's order, so a tile's "imagewidth"
// comes before its "imageheight", while the parts written from a map are
// alphabetical. Round-tripping through map[string]any sorts every object and
// rewrites the whole file, which turns "moved one house" into an unreviewable
// diff. jsonObject remembers the order it was given.

// jsonObject is a JSON object that remembers its key order.
type jsonObject struct {
	keys []string
	vals map[string]any
}

func newJSONObject() *jsonObject {
	return &jsonObject{vals: map[string]any{}}
}

// Get returns the value of k.
func (o *jsonObject) Get(k string) (any, bool) {
	v, ok := o.vals[k]

	return v, ok
}

// Set stores k, appending it to the key order the first time it is seen so an
// edit to an existing key leaves the file's order alone.
func (o *jsonObject) Set(k string, v any) {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}

	o.vals[k] = v
}

// Delete removes k.
func (o *jsonObject) Delete(k string) {
	if _, ok := o.vals[k]; !ok {
		return
	}

	delete(o.vals, k)

	for i, key := range o.keys {
		if key == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)

			break
		}
	}
}

// Keys returns the key order.
func (o *jsonObject) Keys() []string {
	return o.keys
}

// Clone copies the object and everything under it, so a command can hold a
// deleted object and put it back unchanged.
func (o *jsonObject) Clone() *jsonObject {
	out := newJSONObject()
	for _, k := range o.keys {
		out.Set(k, cloneValue(o.vals[k]))
	}

	return out
}

func cloneValue(v any) any {
	switch t := v.(type) {
	case *jsonObject:
		return t.Clone()
	case []any:
		out := make([]any, len(t))
		for i := range t {
			out[i] = cloneValue(t[i])
		}

		return out
	default:
		return v
	}
}

// parseTree reads JSON into jsonObjects, []any, json.Number, string, bool and
// nil. Numbers stay json.Number -- their exact literal -- so a save cannot
// turn Tiled's -1 into -1.0 or lose a large id to float64.
func parseTree(data []byte) (*jsonObject, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	v, err := parseValue(dec)
	if err != nil {
		return nil, err
	}

	obj, ok := v.(*jsonObject)
	if !ok {
		return nil, fmt.Errorf("the file's top level is %T, want a JSON object", v)
	}

	// Anything after the top-level value is a second document, not a map.
	if _, err := dec.Token(); err == nil {
		return nil, fmt.Errorf("trailing data after the map object")
	}

	return obj, nil
}

func parseValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}

	delim, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}

	switch delim {
	case '{':
		obj := newJSONObject()

		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return nil, err
			}

			key, ok := keyTok.(string)
			if !ok {
				return nil, fmt.Errorf("object key %v is not a string", keyTok)
			}

			val, err := parseValue(dec)
			if err != nil {
				return nil, err
			}

			obj.Set(key, val)
		}

		if _, err := dec.Token(); err != nil { // the closing brace
			return nil, err
		}

		return obj, nil
	case '[':
		arr := []any{}

		for dec.More() {
			val, err := parseValue(dec)
			if err != nil {
				return nil, err
			}

			arr = append(arr, val)
		}

		if _, err := dec.Token(); err != nil { // the closing bracket
			return nil, err
		}

		return arr, nil
	}

	return nil, fmt.Errorf("unexpected %v", delim)
}

// writeTree renders the tree the way Tiled and tools/villagemap write a .tmj:
// json.MarshalIndent(v, "", " ") followed by a newline. The compact form is
// assembled here, in the file's key order, and encoding/json's own Indent does
// the layout -- so the escaping and the indentation are exactly what
// json.MarshalIndent would have produced, and only the order is ours.
func writeTree(obj *jsonObject) ([]byte, error) {
	var compact bytes.Buffer

	if err := appendCompact(&compact, obj); err != nil {
		return nil, err
	}

	var out bytes.Buffer

	if err := json.Indent(&out, compact.Bytes(), "", " "); err != nil {
		return nil, err
	}

	out.WriteByte('\n')

	return out.Bytes(), nil
}

func appendCompact(b *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case *jsonObject:
		b.WriteByte('{')

		for i, k := range t.keys {
			if i > 0 {
				b.WriteByte(',')
			}

			key, err := json.Marshal(k)
			if err != nil {
				return err
			}

			b.Write(key)
			b.WriteByte(':')

			if err := appendCompact(b, t.vals[k]); err != nil {
				return err
			}
		}

		b.WriteByte('}')

		return nil
	case []any:
		b.WriteByte('[')

		for i := range t {
			if i > 0 {
				b.WriteByte(',')
			}

			if err := appendCompact(b, t[i]); err != nil {
				return err
			}
		}

		b.WriteByte(']')

		return nil
	}

	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}

	b.Write(raw)

	return nil
}

// ---- reading values out of the tree ---------------------------------------

func asInt(v any) (int, bool) {
	switch t := v.(type) {
	case json.Number:
		n, err := strconv.ParseFloat(t.String(), 64)
		if err != nil || n != float64(int(n)) {
			return 0, false
		}

		return int(n), true
	case float64:
		if t != float64(int(t)) {
			return 0, false
		}

		return int(t), true
	case int:
		return t, true
	}

	return 0, false
}

func asFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case json.Number:
		n, err := strconv.ParseFloat(t.String(), 64)
		if err != nil {
			return 0, false
		}

		return n, true
	case float64:
		return t, true
	case int:
		return float64(t), true
	}

	return 0, false
}

func asString(v any) (string, bool) {
	s, ok := v.(string)

	return s, ok
}

func asBool(v any) (bool, bool) {
	b, ok := v.(bool)

	return b, ok
}

func asArray(v any) ([]any, bool) {
	a, ok := v.([]any)

	return a, ok
}

func asObject(v any) (*jsonObject, bool) {
	o, ok := v.(*jsonObject)

	return o, ok
}

// field* read one key, giving the zero value when it is absent or the wrong
// type. The validator reports the wrong type; the read model does not have to
// carry an error for every field.
func fieldInt(o *jsonObject, k string) int {
	v, ok := o.Get(k)
	if !ok {
		return 0
	}

	n, _ := asInt(v)

	return n
}

func fieldFloat(o *jsonObject, k string) float64 {
	v, ok := o.Get(k)
	if !ok {
		return 0
	}

	n, _ := asFloat(v)

	return n
}

func fieldString(o *jsonObject, k string) string {
	v, ok := o.Get(k)
	if !ok {
		return ""
	}

	s, _ := asString(v)

	return s
}

func fieldBool(o *jsonObject, k string) bool {
	v, ok := o.Get(k)
	if !ok {
		return false
	}

	b, _ := asBool(v)

	return b
}

func fieldArray(o *jsonObject, k string) []any {
	v, ok := o.Get(k)
	if !ok {
		return nil
	}

	a, _ := asArray(v)

	return a
}

// num and fnum build the json.Number an edit writes, so every number in the
// tree is one type and a saved-then-reopened document is the tree it came
// from.
func num(i int) json.Number {
	return json.Number(strconv.Itoa(i))
}

func fnum(f float64) json.Number {
	return json.Number(strconv.FormatFloat(f, 'f', -1, 64))
}
