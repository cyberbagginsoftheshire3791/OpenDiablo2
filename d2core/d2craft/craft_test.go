package d2craft

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
)

func load(t *testing.T) (*d2items.Catalog, *Book) {
	t.Helper()

	root := filepath.Join("..", "..", "data", "strigoi")

	itemData, err := os.ReadFile(filepath.Join(root, "items.json"))
	if err != nil {
		t.Fatal(err)
	}

	cat, err := d2items.Load(itemData)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(root, "recipes.json"))
	if err != nil {
		t.Fatal(err)
	}

	b, err := Load(data, cat)
	if err != nil {
		t.Fatalf("the shipped recipes must load: %v", err)
	}

	return cat, b
}

func kit(t *testing.T, cat *d2items.Catalog) *d2items.Kit {
	t.Helper()

	k, err := cat.NewKit(d2items.LoadoutTorchAndBlade)
	if err != nil {
		t.Fatal(err)
	}

	return k
}

func TestFletching(t *testing.T) {
	cat, b := load(t)
	k := kit(t, cat)

	arrows := k.Count("arrows")

	// THE CONTROL: with nothing to make them from, he makes nothing.
	if err := b.Check(k, "fletch-arrows"); !errors.Is(err, d2items.ErrNotEnough) {
		t.Fatalf("no materials, no arrows: %v", err)
	}

	for id, n := range map[string]int{"branches": 2, "feathers": 5, "arrowheads": 3} {
		if err := k.Give(id, n); err != nil {
			t.Fatal(err)
		}
	}

	r, err := b.Make(k, "fletch-arrows")
	if err != nil {
		t.Fatal(err)
	}

	if k.Count("arrows") != arrows+3 || r.Minutes != 45 {
		t.Fatalf("three arrows in 45 minutes: %d -> %d, %v min", arrows, k.Count("arrows"), r.Minutes)
	}

	if k.Count("branches") != 1 || k.Count("feathers") != 2 || k.Count("arrowheads") != 0 {
		t.Fatalf("the inputs are spent: %d %d %d", k.Count("branches"), k.Count("feathers"), k.Count("arrowheads"))
	}

	// Refused makes change nothing.
	before := len(k.Pack)
	if _, err := b.Make(k, "fletch-arrows"); err == nil || len(k.Pack) != before || k.Count("branches") != 1 {
		t.Fatalf("a refused make takes nothing: %v", err)
	}
}

func TestToolsAreNeededAndKept(t *testing.T) {
	cat, b := load(t)
	k := kit(t, cat)

	if err := k.Give("branches", 1); err != nil {
		t.Fatal(err)
	}

	delete(k.Worn, d2items.SlotBelt1) // no knife

	if err := b.Check(k, "whittle-stake"); !errors.Is(err, ErrNoTool) {
		t.Fatalf("no knife, no stake: %v", err)
	}

	if err := k.Give("knife", 1); err != nil { // a knife in the pack will do
		t.Fatal(err)
	}

	if _, err := b.Make(k, "whittle-stake"); err != nil || k.Count("stake") != 1 || k.Count("knife") != 1 {
		t.Fatalf("the knife is kept, the stake made: %v stake=%d knife=%d", err, k.Count("stake"), k.Count("knife"))
	}

	if k.Count("branches") != 0 {
		t.Fatalf("the branch is spent: %d left", k.Count("branches"))
	}

	// An unbound kit is refused before anything is taken.
	if err := b.Check(&d2items.Kit{}, "whittle-stake"); !errors.Is(err, d2items.ErrUnboundKit) {
		t.Fatalf("an unbound kit: %v", err)
	}
}

func TestFieldMendIsWorseThanNew(t *testing.T) {
	cat, b := load(t)
	k := kit(t, cat)

	if err := k.Give("wire", 3); err != nil {
		t.Fatal(err)
	}

	_, mail, _ := k.ItemIn(d2items.SlotBody)
	full := mail.Points

	// Undamaged mail: nothing to mend, and the wire is not spent on it.
	if err := b.Check(k, "field-mend-mail"); !errors.Is(err, d2items.ErrNothingToMend) {
		t.Fatalf("sound mail needs no mending: %v", err)
	}

	mail.Points = 2

	if _, err := b.Make(k, "field-mend-mail"); err != nil || mail.Points != 6 {
		t.Fatalf("four points closed: %v, %d", err, mail.Points)
	}

	if _, err := b.Make(k, "field-mend-mail"); err != nil {
		t.Fatal(err)
	}

	// Cap 0.75: a field mend stops short of new (14 -> 10) ...
	if want := int(float64(full) * 0.75); mail.Points != want {
		t.Fatalf("field mending stops at three quarters: %d, want %d of %d", mail.Points, want, full)
	}

	// ... and a mend past it is refused and keeps its wire.
	if _, err := b.Make(k, "field-mend-mail"); !errors.Is(err, d2items.ErrNothingToMend) || k.Count("wire") != 1 {
		t.Fatalf("at the cap: %v, %d wire left", err, k.Count("wire"))
	}
}

func TestLoadRefuses(t *testing.T) {
	cat, _ := load(t)

	good := `{"recipes":[{"id":"r","name":"R","inputs":{"branches":1},"outputs":{"stake":1},"minutes":5}]}`
	if _, err := Load([]byte(good), cat); err != nil {
		t.Fatalf("THE CONTROL: %v", err)
	}

	bad := map[string]string{
		"an unknown input":     strings.Replace(good, `"branches":1`, `"moonstone":1`, 1),
		"an unknown output":    strings.Replace(good, `"stake":1`, `"crown":1`, 1),
		"a zero count":         strings.Replace(good, `"branches":1`, `"branches":0`, 1),
		"no minutes":           strings.Replace(good, `"minutes":5`, `"minutes":0`, 1),
		"makes and mends":      strings.Replace(good, `"minutes":5`, `"mend":{"slot":"body","points":1,"cap":1},"minutes":5`, 1),
		"makes nothing":        strings.Replace(good, `,"outputs":{"stake":1}`, ``, 1),
		"a cap above new":      strings.Replace(good, `"outputs":{"stake":1}`, `"mend":{"slot":"body","points":1,"cap":1.5}`, 1),
		"a slot not worn":      strings.Replace(good, `"outputs":{"stake":1}`, `"mend":{"slot":"pack","points":1,"cap":1}`, 1),
		"a duplicate recipe":   strings.Replace(good, `]}`, `,{"id":"r","name":"R","inputs":{"branches":1},"outputs":{"stake":1},"minutes":5}]}`, 1),
		"an unknown field":     strings.Replace(good, `"minutes":5`, `"minutes":5,"xp":3`, 1),
		"an unknown tool":      strings.Replace(good, `"minutes":5`, `"tools":["lute"],"minutes":5`, 1),
		"a tool it eats":       strings.Replace(good, `"minutes":5`, `"tools":["branches"],"minutes":5`, 1),
		"a non-material input": strings.Replace(good, `"branches":1`, `"torch":1`, 1),
		"no inputs":            strings.Replace(good, `"inputs":{"branches":1}`, `"inputs":{}`, 1),
		"a mend of a hand":     strings.Replace(good, `"outputs":{"stake":1}`, `"mend":{"slot":"main","points":1,"cap":1}`, 1),
	}

	for name, doc := range bad {
		if _, err := Load([]byte(doc), cat); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
}
