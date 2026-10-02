//go:build playtest

package playtest

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestSaveResumeFieldSweep is the omit sweep's companion one level down (the
// M4.6 B6 review's probe, adopted): EVERY LEAF of T's world file -- the first
// element of each list, and the first entity the map does not build --
// perturbed in turn (a number +1 or +0.37, a bool flipped, a known word
// swapped; ids, keys and paths left alone), the file resumed, and the outcome
// classed REFUSED, DIVERGED (where) or SAME. Act 9 proves each BLOCK is read;
// this asks it of each FIELD, and every SAME is red unless the field is on
// fieldSameByDesign, each with its reason.
//
// OPT-IN, like TestSaveResumeNegatives: it runs against a kept evening
// (STRIGOI_SAVE_RESUME_FROM) and takes about seven minutes (some 275 loads).
// FIELD_SWEEP=<prefix> runs only the paths under it.
func TestSaveResumeFieldSweep(t *testing.T) {
	from := os.Getenv("STRIGOI_SAVE_RESUME_FROM")
	if from == "" {
		t.Skip("opt-in: set STRIGOI_SAVE_RESUME_FROM to an evening a green TestSaveResume kept")
	}

	only := os.Getenv("FIELD_SWEEP")

	ev := keptEvening(t, from)
	s := start(t)
	s.call("strigoi_pause", map[string]any{})
	eveningHarness(t, s, ev)

	if sT := negLoad(t, s, ev, "the field sweep's control", ev.fileT); sT.Resume != ev.sT.Resume {
		sameWorld(t, "the field sweep's control (the untouched file)", ev.sT, sT)
	}

	var paths [][]any
	fieldPaths(decodeNumbers(t, ev.fileT), nil, true, &paths)

	// A key written only when true is a leaf this walk never meets in a file
	// where it is false: each such key is put in, true, as its own leaf
	// (version 5: pursuit.chases[].from_watch).
	absentTrue := [][]any{{"pursuit", "chases", 0, "from_watch"}}
	for _, p := range absentTrue {
		if !hasLeaf(paths, p) {
			paths = append(paths, p)
		}
	}

	tally := map[string]int{}
	allowed := map[string]bool{}
	began := time.Now()

	for _, p := range paths {
		ps := fieldPathString(p)
		if only != "" && !strings.HasPrefix(ps, only) {
			continue
		}

		file := decodeNumbers(t, ev.fileT)

		desc, ok := fieldPerturb(file, p)
		if !ok {
			continue
		}

		data, err := json.MarshalIndent(file, "", "  ")
		if err != nil {
			t.Fatal(err)
		}

		snap := negLoad(t, s, ev, "field "+ps, append(data, '\n'))
		load := sub(s.call("strigoi_get_game_info", map[string]any{}), "load")
		resumed, _ := load["resumed"].(bool)

		var class, verdict string

		switch {
		case !resumed:
			class, verdict = "REFUSED", fmt.Sprintf("%v/%v: %s", load["refused"], load["rule"], cut(fmt.Sprint(load["reason"])))
		case snap.Resume == ev.sT.Resume:
			class, verdict = "SAME", "resumed the saved moment"
		default:
			class, verdict = "DIVERGED", strings.Join(diffOf(ev.sT, snap), ", ")
		}

		tally[class]++

		if class == "SAME" {
			why := sameByDesign(ps)
			if why == "" {
				t.Errorf("FIELD SAME %s (%s): a change to it resumed the saved moment -- a field no part of the digest shows", ps, desc)

				continue
			}

			allowed[why] = true
			verdict += " -- by design: " + why
		}

		t.Logf("FIELD %-8s %s (%s) :: %s", class, ps, desc, verdict)
	}

	for _, a := range fieldSameByDesign {
		if !allowed[a.why] && only == "" {
			t.Logf("FIELD allow-list entry never needed this run: %s (%s)", a.pattern, a.why)
		}
	}

	t.Logf("FIELD TALLY %v in %.0f s", tally, time.Since(began).Seconds())
}

// fieldSameByDesign are the fields whose change a resume does not show, BY
// DESIGN, each with why. Anything else that comes out SAME is red.
var fieldSameByDesign = []struct {
	pattern *regexp.Regexp
	why     string
}{
	{regexp.MustCompile(`^light\.sources\.0\.[xy]$`),
		"the carried torch's place is his: the light model sets it from where he stands every frame (BUG-110)"},
	{regexp.MustCompile(`^sidecar\.kit\.worn\.off\.burn_left$`),
		"a lit torch's minutes are the light model's carried source; the load zeroes the kit's (D1)"},
	{regexp.MustCompile(`^entities\.\d+\.motion\.velocity\.\d+$`),
		"a velocity at a standstill: an entity with no path to walk has none, whatever was written"},
	{regexp.MustCompile(`^squads\.squads\.\d+\.meters\.activity$`),
		"the stance follows the post: keepWatch sets watch or idle every frame (watch.go)"},
	{regexp.MustCompile(`^scene\.(last_stage|watch_clock_set)$`),
		"spent on the first frame, which samples the stage and the watch clock again; act 9 shows each against its twin, " +
			"at a moment it decides (an unpaid dawn; the watch clock behind)"},
	{regexp.MustCompile(`^sidecar\.journal\.(rungs\.[a-z_]+|tasks\.[a-z_]+\.mem\.\d+\.(held|events))$`),
		"recomputed by the journal's first Evaluate: a rung he still stands at is reached again, and a task stage's edge " +
			"memory is the condition as it holds now"},
}

func sameByDesign(path string) string {
	for _, a := range fieldSameByDesign {
		if a.pattern.MatchString(path) {
			return a.why
		}
	}

	return ""
}

var fieldSkipTop = map[string]bool{"version": true, "build": true, "saved_at": true, "map": true, "seed": true}

var fieldSkipKey = map[string]bool{"id": true, "watcher": true, "target": true, "hunter": true, "quarry": true,
	"entity": true, "sha": true, "path": true, "generation": true, "seed": true, "monstat": true, "name_key": true,
	"name": true, "class": true, "code": true, "item": true, "loadout": true, "explored": true, "map": true}

var fieldSwap = map[string]string{"watch": "idle", "idle": "watch", "night": "day", "day": "night", "dusk": "dawn",
	"closed": "fresh", "fresh": "closed", "hasty": "closed", "hostile": "living", "living": "hostile", "none": "held",
	"issue": "fine", "sound": "worn", "player": "s:9", "disengaged": "routed", "NU": "WL", "human": "beast"}

func fieldPaths(v any, path []any, top bool, out *[][]any) {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if top && fieldSkipTop[k] {
				continue
			}
			fieldPaths(x[k], append(append([]any{}, path...), k), false, out)
		}
	case []any:
		idx := []int{}
		if len(x) > 0 {
			idx = append(idx, 0)
		}
		if len(path) == 1 && path[0] == "entities" {
			for i, e := range x {
				if m, _ := e.(map[string]any); m["native"] != true {
					idx = append(idx, i)
					break
				}
			}
		}
		for _, i := range idx {
			fieldPaths(x[i], append(append([]any{}, path...), i), false, out)
		}
	default:
		*out = append(*out, path)
	}
}

func fieldPathString(p []any) string {
	parts := make([]string, len(p))
	for i, e := range p {
		parts[i] = fmt.Sprint(e)
	}
	return strings.Join(parts, ".")
}

// fieldPerturb changes the leaf at p in file; false when it is not one to try.
func fieldPerturb(file map[string]any, p []any) (string, bool) {
	var parent any = file
	for _, e := range p[:len(p)-1] {
		switch c := parent.(type) {
		case map[string]any:
			parent = c[e.(string)]
		case []any:
			parent = c[e.(int)]
		}
	}
	last := p[len(p)-1]
	get := func() any {
		if m, ok := parent.(map[string]any); ok {
			return m[last.(string)]
		}
		return parent.([]any)[last.(int)]
	}
	set := func(v any) {
		if m, ok := parent.(map[string]any); ok {
			m[last.(string)] = v
			return
		}
		parent.([]any)[last.(int)] = v
	}
	key, _ := last.(string)

	if m, ok := parent.(map[string]any); ok {
		if _, has := m[key]; !has && key != "" {
			m[key] = true

			return "absent->true", true
		}
	}

	// The watch clock is perturbed BEHIND: ahead of the clock the first
	// frame's keepWatch credits nothing either way, so +0.37 cannot show.
	if fieldPathString(p) == "scene.watch_clock" {
		f, err := get().(json.Number).Float64()
		if err != nil {
			return "", false
		}

		set(json.Number(strconv.FormatFloat(f-4, 'g', -1, 64)))

		return fmt.Sprintf("%g->%g", f, f-4), true
	}

	switch v := get().(type) {
	case json.Number:
		s := v.String()
		if !strings.ContainsAny(s, ".eE") {
			n, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return "", false
			}
			set(json.Number(strconv.FormatInt(n+1, 10)))
			return fmt.Sprintf("%d->%d", n, n+1), true
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return "", false
		}
		nf := f + 0.37
		set(json.Number(strconv.FormatFloat(nf, 'g', -1, 64)))
		return fmt.Sprintf("%g->%g", f, nf), true
	case bool:
		set(!v)
		return fmt.Sprintf("%v->%v", v, !v), true
	case string:
		if fieldSkipKey[key] {
			return "", false
		}
		if w, ok := fieldSwap[v]; ok {
			set(w)
			return fmt.Sprintf("%q->%q", v, w), true
		}
		return "", false
	}
	return "", false
}

// hasLeaf is whether paths holds p.
func hasLeaf(paths [][]any, p []any) bool {
	for _, q := range paths {
		if fieldPathString(q) == fieldPathString(p) {
			return true
		}
	}

	return false
}
