package d2mappalette

import (
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// The Dealu monastery is the test case, and it is ART ONLY.
//
// Nothing of it is installed in the engine: grep the tree for "dealu" across
// .go, .tmj, .json and .txt at HEAD d1d07434 and there are zero hits. So this
// file judges it exactly as the palette must -- on whether its renders satisfy
// the loader's rules, and on nothing its own manifest claims about itself.
//
// dealuManifest is the real structures/dealu-monastery-v1/modules.json from
// C:\Users\josht\Projects\strigoi-art, reduced to the keys this package reads
// and with every number copied verbatim. It is here so CI, which has no art
// repository, still tests the logic; TestTheRealDealuManifest re-runs the same
// assertions against the real file on a machine that has it, and skips where it
// does not.
const dealuManifest = `{
  "id": "dealu-monastery-v1",
  "version": 1,
  "tile_pixels": [160, 80],
  "modules": [
    {"id": "church",    "footprint_tiles": [6, 6], "world_canvas_pixels": [960, 768],
     "states": ["intact"],
     "outputs": {"intact": "renders/structures/dealu-monastery-v1/church-intact-960x768.png"},
     "integration": "art-ready; not installed; collision and occlusion need placement review"},
    {"id": "gate",      "footprint_tiles": [3, 3], "world_canvas_pixels": [480, 576],
     "states": ["open", "closed"],
     "outputs": {"open": "renders/structures/dealu-monastery-v1/gate-open-480x576.png",
                 "closed": "renders/structures/dealu-monastery-v1/gate-closed-480x576.png"},
     "integration": "art-ready; not installed; collision and occlusion need placement review"},
    {"id": "cells",     "footprint_tiles": [3, 7], "world_canvas_pixels": [1120, 720],
     "states": ["intact"],
     "outputs": {"intact": "renders/structures/dealu-monastery-v1/cells-intact-1120x720.png"},
     "integration": "art-ready; not installed; collision and occlusion need placement review"},
    {"id": "refectory", "footprint_tiles": [3, 6], "world_canvas_pixels": [960, 640],
     "states": ["intact"],
     "outputs": {"intact": "renders/structures/dealu-monastery-v1/refectory-intact-960x640.png"},
     "integration": "art-ready; not installed; collision and occlusion need placement review"},
    {"id": "well",      "footprint_tiles": [2, 2], "world_canvas_pixels": [320, 384],
     "states": ["intact"],
     "outputs": {"intact": "renders/structures/dealu-monastery-v1/well-intact-320x384.png"},
     "integration": "art-ready; not installed; collision and occlusion need placement review"},
    {"id": "wall",      "footprint_tiles": [3, 1], "world_canvas_pixels": [480, 384],
     "states": ["intact"],
     "outputs": {"intact": "renders/structures/dealu-monastery-v1/wall-intact-480x384.png"},
     "integration": "art-ready; not installed; collision and occlusion need placement review"}
  ],
  "walkability": "NOT IMPLEMENTED: gate and yard need separate engine collision, not one blocked compound footprint."
}`

// dealuWant is the true table, and every row was checked against the engine's
// own rules by TestPaletteAgreesWithTheEngine's six dealu cases, which run the
// real d2maptiled.Parse over these exact footprints and sizes.
//
//	module     declared  render     (fw+fh)*80  square  <=768  verdict
//	church     6x6       960x768    960 = w     yes     yes    LEGAL
//	gate       3x3       480x576    480 = w     yes     yes    LEGAL
//	well       2x2       320x384    320 = w     yes     yes    LEGAL
//	cells      3x7       1120x720   800 != 1120 NO      yes    refused, twice over
//	refectory  3x6       960x640    720 != 960  NO      yes    refused, twice over
//	wall       3x1       480x384    320 != 480  NO      yes    refused, twice over
var dealuWant = []struct {
	module   string
	state    string
	name     string
	fp       image.Point
	w, h     int
	legal    bool
	problems []string
}{
	{"church", "intact", "Dealu monastery church", image.Pt(6, 6), 960, 768, true, nil},
	{"gate", "open", "Dealu monastery gate (open)", image.Pt(3, 3), 480, 576, true, nil},
	{"gate", "closed", "Dealu monastery gate (closed)", image.Pt(3, 3), 480, 576, true, nil},
	{"well", "intact", "Dealu monastery well", image.Pt(2, 2), 320, 384, true, nil},
	{"cells", "intact", "Dealu monastery cells", image.Pt(3, 7), 1120, 720, false,
		[]string{"not square", "wants exactly 800"}},
	{"refectory", "intact", "Dealu monastery refectory", image.Pt(3, 6), 960, 640, false,
		[]string{"not square", "wants exactly 720"}},
	{"wall", "intact", "Dealu monastery wall", image.Pt(3, 1), 480, 384, false,
		[]string{"not square", "wants exactly 320"}},
}

// dealuFS is the manifest plus a render of the declared size for each state.
func dealuFS(t *testing.T) fstest.MapFS {
	t.Helper()

	fsys := fstest.MapFS{
		"structures/dealu-monastery-v1/modules.json": &fstest.MapFile{Data: []byte(dealuManifest)},
	}

	for _, w := range dealuWant {
		name := "renders/structures/dealu-monastery-v1/" + w.module + "-" + w.state + "-" +
			itoa(w.w) + "x" + itoa(w.h) + ".png"
		fsys[name] = pngFile(t, w.w, w.h)
	}

	return fsys
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}

	return string(b)
}

// TestDealuMonasteryCatalog is the brief's test case. Three of the six modules
// must be offered, three must be excluded with a reason, and the reason must be
// the engine's own.
func TestDealuMonasteryCatalog(t *testing.T) {
	c := NewCatalog()
	if err := c.ReadModules(dealuFS(t), "structures/dealu-monastery-v1/modules.json"); err != nil {
		t.Fatalf("reading the monastery manifest: %v", err)
	}

	// Six modules, seven entries: the gate has two states and each is its own
	// piece of art the editor can place.
	if c.Len() != len(dealuWant) {
		t.Fatalf("%d entries, want %d: %v", c.Len(), len(dealuWant), ids(c))
	}

	for _, w := range dealuWant {
		id := "dealu-monastery-v1/" + w.module + "/" + w.state

		e, ok := c.ByID(id)
		if !ok {
			t.Errorf("no entry %q; got %v", id, ids(c))

			continue
		}

		if e.DisplayName != w.name {
			t.Errorf("%s is called %q, want %q", id, e.DisplayName, w.name)
		}

		if e.PixelWidth != w.w || e.PixelHeight != w.h {
			t.Errorf("%s measures %dx%d, want %dx%d", id, e.PixelWidth, e.PixelHeight, w.w, w.h)
		}

		// THE DECLARED FOOTPRINT IS KEPT AS DECLARED. Deriving 7x7 for the
		// cells from its 1120-pixel width would make the entry placeable and
		// the catalog a liar: the art is three tiles deep.
		if e.Footprint != w.fp {
			t.Errorf("%s footprint %v, want the manifest's declared %v", id, e.Footprint, w.fp)
		}

		if !strings.Contains(e.FootprintWhy, "declared") {
			t.Errorf("%s does not say its footprint was declared: %q", id, e.FootprintWhy)
		}

		if e.Placeable() != w.legal {
			t.Errorf("%s: placeable = %v, want %v (problems: %v)", id, e.Placeable(), w.legal, e.Problems)
		}

		for _, want := range w.problems {
			if !strings.Contains(strings.Join(e.Problems, " | "), want) {
				t.Errorf("%s: no problem mentions %q; got %v", id, want, e.Problems)
			}
		}

		// The whole footprint is solid whatever the manifest's free-text
		// walkability string says (tiled.go:890).
		if !e.Blocked {
			t.Errorf("%s is not blocked; a structure's footprint is solid", id)
		}

		// The manifest's "integration" says more than a plain "art-ready", so
		// every module is Preview and the string is quoted back.
		if e.Status != StatusPreview {
			t.Errorf("%s is %q, want preview (why: %s)", id, e.Status, e.StatusWhy)
		}

		if !strings.Contains(e.StatusWhy, "not installed") {
			t.Errorf("%s does not quote the manifest's integration value: %s", id, e.StatusWhy)
		}
	}

	// Exactly three placeable, four refused (the gate counting twice).
	if n := len(c.Placeable()); n != 4 {
		t.Errorf("%d placeable entries, want 4 (church, gate open, gate closed, well): %v", n, names(c.Placeable()))
	}

	if n := len(c.Refused()); n != 3 {
		t.Errorf("%d refused entries, want 3 (cells, refectory, wall): %v", n, names(c.Refused()))
	}

	// The three legal modules are three distinct MODULES, which is the claim the
	// brief makes.
	legal := map[string]bool{}

	for _, e := range c.Placeable() {
		legal[strings.Split(e.ID, "/")[1]] = true
	}

	for _, m := range []string{"church", "gate", "well"} {
		if !legal[m] {
			t.Errorf("the %s module is not offered and it satisfies every rule", m)
		}
	}

	for _, m := range []string{"cells", "refectory", "wall"} {
		if legal[m] {
			t.Errorf("the %s module is offered and the engine would refuse it", m)
		}
	}
}

// TestDealuFreeTextIsNotParsed: the manifest's "walkability" string says the
// engine cannot do a gate, and it is wrong. This burst measured a one-tile gap
// between two 3x3 modules walking 25/25 and passing sight while a module column
// blocked sight, and a generated 6x6 module blocking exactly its 36 tiles with
// the ring around it open (playtest log zz-gate-measure4.txt, PASS). The string
// is boilerplate, repeated verbatim on all six modules including the three that
// are square, so it must have no effect on anything.
func TestDealuFreeTextIsNotParsed(t *testing.T) {
	base := dealuFS(t)

	// Same manifest, with the walkability claim inverted and a status field
	// invented. Neither may change a single verdict.
	alt := strings.ReplaceAll(dealuManifest,
		"NOT IMPLEMENTED: gate and yard need separate engine collision, not one blocked compound footprint.",
		"FULLY IMPLEMENTED: everything works and every module is approved final art.")
	base2 := dealuFS(t)
	base2["structures/dealu-monastery-v1/modules.json"] = &fstest.MapFile{Data: []byte(alt)}

	a, b := NewCatalog(), NewCatalog()
	if err := a.ReadModules(base, "structures/dealu-monastery-v1/modules.json"); err != nil {
		t.Fatal(err)
	}

	if err := b.ReadModules(base2, "structures/dealu-monastery-v1/modules.json"); err != nil {
		t.Fatal(err)
	}

	for _, e := range a.Entries() {
		other, ok := b.ByID(e.ID)
		if !ok {
			t.Fatalf("%s vanished when the free text changed", e.ID)
		}

		if other.Status != e.Status || other.Placeable() != e.Placeable() {
			t.Errorf("%s changed when only free text changed: %q/%v became %q/%v",
				e.ID, e.Status, e.Placeable(), other.Status, other.Placeable())
		}

		// The REASONS have to be identical too, not just the verdicts. A first
		// draft compared only Status and Placeable, and a negative control that
		// fed the walkability string into the reason left it green -- free text
		// reaching the user as an engine fact is exactly the failure this test
		// is for.
		if other.StatusWhy != e.StatusWhy {
			t.Errorf("%s: the status reason changed when only free text changed: %q became %q",
				e.ID, e.StatusWhy, other.StatusWhy)
		}

		if strings.Join(other.Problems, "|") != strings.Join(e.Problems, "|") {
			t.Errorf("%s: the problems changed when only free text changed: %v became %v",
				e.ID, e.Problems, other.Problems)
		}

		if strings.Join(other.Signals, "|") != strings.Join(e.Signals, "|") {
			t.Errorf("%s: the signals changed when only free text changed: %v became %v",
				e.ID, e.Signals, other.Signals)
		}
	}

	// And nothing anywhere in the catalog repeats either free-text claim at the
	// user: not the reason, not a problem, not a signal.
	for _, e := range a.Entries() {
		blob := e.StatusWhy + " " + strings.Join(e.Problems, " ") + " " + strings.Join(e.Signals, " ")
		for _, leak := range []string{"NOT IMPLEMENTED", "separate engine collision", "rectangular_canvas"} {
			if strings.Contains(blob, leak) {
				t.Errorf("%s repeats the manifest's free text %q at the user: %s", e.ID, leak, blob)
			}
		}
	}
}

// TestDealuManifestSizesAreCheckedAgainstTheRenders: a manifest that declares a
// canvas its render does not match is reporting a stale number, and the palette
// must measure the file and say so rather than believe the JSON.
func TestDealuManifestSizesAreCheckedAgainstTheRenders(t *testing.T) {
	fsys := dealuFS(t)
	// Replace the church's render with one a pixel narrower than declared.
	fsys["renders/structures/dealu-monastery-v1/church-intact-960x768.png"] = pngFile(t, 959, 768)

	c := NewCatalog()
	if err := c.ReadModules(fsys, "structures/dealu-monastery-v1/modules.json"); err != nil {
		t.Fatal(err)
	}

	e, ok := c.ByID("dealu-monastery-v1/church/intact")
	if !ok {
		t.Fatal("no church entry")
	}

	if e.PixelWidth != 959 {
		t.Errorf("the palette reports %d wide; it must measure the file, not read the manifest", e.PixelWidth)
	}

	if e.Placeable() {
		t.Error("a 959-wide 6x6 church is offered as placeable")
	}

	if !strings.Contains(strings.Join(e.Problems, " | "), "declares world_canvas_pixels 960x768 but") {
		t.Errorf("the mismatch is not reported: %v", e.Problems)
	}

	// And a module whose render is missing altogether.
	gone := dealuFS(t)
	delete(gone, "renders/structures/dealu-monastery-v1/well-intact-320x384.png")

	c2 := NewCatalog()
	if err := c2.ReadModules(gone, "structures/dealu-monastery-v1/modules.json"); err != nil {
		t.Fatal(err)
	}

	well, ok := c2.ByID("dealu-monastery-v1/well/intact")
	if !ok {
		t.Fatal("the well vanished because its render is missing; it must be catalogued and refused")
	}

	if well.Placeable() {
		t.Error("a module with no render is offered as placeable")
	}

	if !strings.Contains(strings.Join(well.Problems, " | "), "could not be read") {
		t.Errorf("the missing render is not reported: %v", well.Problems)
	}
}

// TestTheRealDealuManifest runs the same table against EVERY real
// dealu-monastery-vN manifest in the strigoi-art repository, on a machine that
// has it. It skips on CI, which does not, and it is the check that dealuManifest
// above has not drifted from the files it was copied out of.
//
// It walks the versions rather than naming v1 because a v2 appeared in the art
// repository while this package was being written -- same six modules, same
// declared footprints, same canvases -- and a test that named one version would
// have said nothing about it. A vN that changes any module's footprint or render
// size fails here, which is the point: this package's whole claim about the
// monastery is arithmetic on those numbers.
//
// It is read-only: the art repository is not this package's to change.
func TestTheRealDealuManifest(t *testing.T) {
	root := filepath.FromSlash("C:/Users/josht/Projects/strigoi-art")

	versions, err := filepath.Glob(filepath.Join(root, "structures", "dealu-monastery-v*", "modules.json"))
	if err != nil {
		t.Fatal(err)
	}

	if len(versions) == 0 {
		t.Skipf("the art repository is not on this machine (%s); dealuManifest covers the logic", root)
	}

	fsys := os.DirFS(root)

	for _, manifest := range versions {
		version := filepath.Base(filepath.Dir(manifest))

		t.Run(version, func(t *testing.T) {
			c := NewCatalog()
			if err := c.ReadModules(fsys, "structures/"+version+"/modules.json"); err != nil {
				t.Fatalf("reading %s: %v", manifest, err)
			}

			if c.Len() != len(dealuWant) {
				t.Errorf("%s yields %d entries, the copy in this file yields %d: %v",
					version, c.Len(), len(dealuWant), ids(c))
			}

			for _, w := range dealuWant {
				id := version + "/" + w.module + "/" + w.state

				e, ok := c.ByID(id)
				if !ok {
					t.Errorf("%s has no %q; got %v", version, id, ids(c))

					continue
				}

				// The name drops the -vN: a version is not part of a thing's
				// name, so v1's church and v2's church read the same.
				if e.DisplayName != w.name {
					t.Errorf("%s is called %q, want %q", id, e.DisplayName, w.name)
				}

				if e.Footprint != w.fp {
					t.Errorf("%s: %s declares %v, this file's copy says %v", id, version, e.Footprint, w.fp)
				}

				if e.PixelWidth != w.w || e.PixelHeight != w.h {
					t.Errorf("%s: the real render measures %dx%d, this file's copy says %dx%d",
						id, e.PixelWidth, e.PixelHeight, w.w, w.h)
				}

				if e.Placeable() != w.legal {
					t.Errorf("%s: the real module is placeable=%v, this file says %v (problems %v)",
						id, e.Placeable(), w.legal, e.Problems)
				}
			}

			// Four entries offered (the gate twice), three refused, in every
			// version.
			if n := len(c.Placeable()); n != 4 {
				t.Errorf("%s offers %d entries, want 4: %v", version, n, names(c.Placeable()))
			}

			// And the engine still does not know about it.
			if _, err := os.Stat(filepath.Join("..", "..", "data", "strigoi", "structures", version)); err == nil {
				t.Errorf("%s has been installed under data/strigoi/structures; this test's premise (art only) is stale", version)
			}
		})
	}
}

// TestDealuManifestCopyIsValidJSON guards the copy above from a typo that would
// make every assertion in this file vacuous.
func TestDealuManifestCopyIsValidJSON(t *testing.T) {
	var m map[string]any
	if err := json.Unmarshal([]byte(dealuManifest), &m); err != nil {
		t.Fatalf("dealuManifest is not valid JSON: %v", err)
	}

	mods, ok := m["modules"].([]any)
	if !ok || len(mods) != 6 {
		t.Fatalf("dealuManifest holds %v modules, want 6", len(mods))
	}
}

func names(es []Entry) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.DisplayName)
	}

	return out
}
