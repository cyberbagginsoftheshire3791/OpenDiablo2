// Command mapfirecount answers ask 1 of the map-fire scoping note: how many
// D2 objects declare a light, how many of those flicker, and how many lit
// objects an actual Act 1 town map places.
//
// IT CHANGES NO LIGHTING. It loads records, counts, and prints. Nothing here
// touches d2world, the renderer, or any dial. Josh's fence on that note,
// verbatim: "If you find yourself reaching into d2records object tables to
// BUILD something, you have left that note's scope." Reading them to count is
// the scope; building from them is not.
//
// ARTICLE V. This reads the MPQs at runtime and writes nothing. No extracted
// data may be committed, and this tool must never run on CI -- there are no
// MPQs there. It is a main package so `go build ./...` compiles it and never
// runs it.
//
// Usage, from the repository root:
//
//	go run ./tools/mapfirecount
//	go run ./tools/mapfirecount -mpq "C:\Program Files (x86)\Diablo II"
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2loader/asset/types"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2config"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2records"
)

// The Act 1 town presets. GenerateAct1Overworld places one of these and, for
// E1/S1/W1, generates wilderness beside it. Counting them individually is
// deliberate: which one a run draws is a property of the seed, so a single
// number would be a number for one seed rather than for the slice.
var townDS1s = []string{
	// Relative to the tiles root: LoadDS1 prepends /data/global/tiles/ itself,
	// which is worth knowing because passing the full path silently produces
	// /data/global/tiles//data/global/tiles/... and a "file not found" that
	// looks like a missing MPQ rather than a doubled prefix.
	"act1/town/townE1.ds1",
	"act1/town/townW1.ds1",
	"act1/town/townS1.ds1",
	"act1/town/townN1.ds1",
}

func main() {
	cfg := d2config.DefaultConfig()

	mpq := flag.String("mpq", cfg.MpqPath, "directory holding the D2 MPQs")
	verbose := flag.Bool("v", false, "list every lit record")
	flag.Parse()

	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		die("asset manager: %v", err)
	}

	for _, name := range cfg.MpqLoadOrder {
		src := filepath.Join(filepath.Clean(*mpq), name)
		if err := asset.AddSource(src, types.AssetSourceMPQ); err != nil {
			die("MPQ %q not found. Pass -mpq with the directory that holds them.\n  %v", src, err)
		}
	}

	if err := asset.LoadRecords(d2resource.ObjectDetails); err != nil {
		die("objects.txt: %v", err)
	}

	details := asset.Records.Object.Details
	if len(details) == 0 {
		die("objects.txt loaded but held no records -- the gate is broken, this is not a finding")
	}

	censusRecords(details, *verbose)
	censusMaps(asset, details)
}

// censusRecords is the headline: how much of objects.txt declares a light.
func censusRecords(details d2records.ObjectDetails, verbose bool) {
	var (
		litPairs   int // (id, MODE) pairs with a diameter
		litIDs     int // distinct records with at least one lit mode
		flickerIDs int // ...of which flicker
		modeHist   [8]int
		diameters  = map[int]int{}
		litRecords []int
	)

	for id, rec := range details {
		if rec == nil {
			continue
		}

		modes := 0

		for mode := 0; mode < len(rec.LightDiameter); mode++ {
			if rec.LightDiameter[mode] > 0 {
				modes++
				litPairs++
				modeHist[mode]++
				diameters[rec.LightDiameter[mode]]++
			}
		}

		if modes > 0 {
			litIDs++

			litRecords = append(litRecords, id)

			if rec.Flicker {
				flickerIDs++
			}
		}
	}

	sort.Ints(litRecords)

	fmt.Printf("OBJECT RECORDS (objects.txt)\n")
	fmt.Printf("  records loaded                      %d\n", len(details))
	fmt.Printf("  (id, MODE) pairs declaring a light  %d\n", litPairs)
	fmt.Printf("  distinct ids with any lit mode      %d\n", litIDs)
	fmt.Printf("  ...of those, Flicker is set on      %d\n", flickerIDs)
	fmt.Println()

	fmt.Printf("  lit modes, by animation mode index:\n")

	for mode, n := range modeHist {
		if n > 0 {
			fmt.Printf("    mode %d  %d\n", mode, n)
		}
	}

	fmt.Println()
	fmt.Printf("  light diameters seen (diameter: count):\n")

	keys := make([]int, 0, len(diameters))
	for d := range diameters {
		keys = append(keys, d)
	}

	sort.Ints(keys)

	for _, d := range keys {
		fmt.Printf("    %3d: %d\n", d, diameters[d])
	}

	if verbose {
		fmt.Println()
		fmt.Printf("  lit record ids: %v\n", litRecords)
	}

	fmt.Println()
	fmt.Printf("  NOTE, and it corrects the scoping note: LightDiameter is per\n")
	fmt.Printf("  animation MODE ([8]int), but Flicker is a single bool on the\n")
	fmt.Printf("  RECORD. So \"how many also flicker\" is a question about ids,\n")
	fmt.Printf("  not about (id, MODE) pairs, and a lit and an unlit mode of one\n")
	fmt.Printf("  object cannot disagree about flickering.\n")
	fmt.Println()
}

// censusMaps is the number that decides the design: how many lit objects an
// actual town map places. Light.Level loops EVERY source and the renderer
// samples it per tile across four passes, so the per-frame source count is
// what makes map fires cheap or ruinous.
func censusMaps(asset *d2asset.AssetManager, details d2records.ObjectDetails) {
	fmt.Printf("PLACED OBJECTS, per Act 1 town DS1\n")
	fmt.Printf("  %-34s %7s %7s %7s %9s\n", "file", "objects", "items", "resolved", "LIT")

	for _, path := range townDS1s {
		ds1, err := asset.LoadDS1(path)
		if err != nil {
			fmt.Printf("  %-34s  UNREADABLE: %v\n", filepath.Base(path), err)
			continue
		}

		var objects, items, resolved, lit, flicker int

		var byMode [8]int

		for i := range ds1.Objects {
			obj := &ds1.Objects[i]
			objects++

			// The same filter the map stamp applies (stamp.go:130).
			if obj.Type != int(d2enum.ObjectTypeItem) {
				continue
			}

			items++

			lookup := asset.Records.LookupObject(int(ds1.Act), obj.Type, obj.ID)
			if lookup == nil {
				continue
			}

			rec := details[lookup.ObjectsTxtId]
			if rec == nil {
				continue
			}

			resolved++

			any := false

			for mode := 0; mode < len(rec.LightDiameter); mode++ {
				if rec.LightDiameter[mode] > 0 {
					byMode[mode]++

					any = true
				}
			}

			if any {
				lit++

				if rec.Flicker {
					flicker++
				}
			}
		}

		fmt.Printf("  %-34s %7d %7d %7d %9d\n", filepath.Base(path), objects, items, resolved, lit)
		fmt.Printf("  %-34s lit in mode 0:%d  1:%d  2:%d   flicker:%d\n", "", byMode[0], byMode[1], byMode[2], flicker)
	}

	fmt.Println()
	fmt.Printf("  \"LIT\" counts a placed object whose record declares a light in\n")
	fmt.Printf("  ANY mode -- an upper bound, because an object standing in an\n")
	fmt.Printf("  unlit mode contributes nothing. The per-frame source count of a\n")
	fmt.Printf("  running map is therefore at most this, plus the wilderness,\n")
	fmt.Printf("  WHICH THIS TOOL DOES NOT COUNT: the wilderness DS1s are chosen\n")
	fmt.Printf("  by the generator from level presets at run time, and reading\n")
	fmt.Printf("  them faithfully means running the generator. Named, not guessed.\n")
}

func die(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "mapfirecount: "+format+"\n", args...)
	os.Exit(1)
}
