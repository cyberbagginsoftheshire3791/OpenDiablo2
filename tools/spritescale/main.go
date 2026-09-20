// Command spritescale measures how big D2's creature sprites actually are, in
// pixels, per animation mode and per facing.
//
// WHY THIS EXISTS. The game is getting its own art (Plan §5, M5.1 shipped the
// PNG path on 20 Sep 2026), and the art is being generated outside the repo. An
// image generator will cheerfully hand back a beautiful 1024x1024 hero-pose
// wolf, which is useless for an eight-facing isometric sprite -- so the spec
// handed to it has to state the real numbers. Guessing them would put every
// creature in the game at the wrong scale, and "the art looks wrong" is the
// hardest class of bug to attribute after the fact.
//
// So this is measured rather than assumed, and the numbers it prints are what
// docs/art-spec.md quotes.
//
// IT CHANGES NO GAME BEHAVIOUR. It loads records, builds composites, sets modes
// and reads frame geometry. No renderer is created and no surface is made --
// frame width, height and offsets are filled by the DC6/DCC init pass, before
// any pixels are decoded, which is why this runs headless.
//
// ARTICLE V. This reads the MPQs at runtime and writes nothing. No extracted
// data may be committed, and this tool MUST NEVER RUN ON CI -- there are no
// MPQs there. It is a main package, so `go build ./...` compiles it and never
// runs it, the same fence tools/animcensus and tools/mapfirecount sit behind.
//
// Usage, from the repository root:
//
//	go run ./tools/spritescale
//	go run ./tools/spritescale -mpq "C:\Program Files (x86)\Diablo II"
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2animdata"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2loader/asset/types"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2config"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2records"
)

// subjects are the codes worth measuring: the three stand-ins the spawn tables
// actually use, plus the player token, because a creature has to read as the
// right size NEXT TO THE HERO and the hero is the only fixed reference in the
// frame.
//
// HARDCODED, and for the reason tools/animcensus states about the same list:
// there is no exported accessor for the spawn table, so this is a copy that can
// go stale. If a row's creature changes, this list changes with it.
var subjects = []struct {
	code string
	why  string
}{
	{"fallen1", "the dogs row, and the opportunists row"},
	{"zombie1", "the wolves row"},
	{"skeleton1", "the boar row"},
}

// modes worth measuring. NU is the idle pose an art spec is written against;
// WL is the one that has to read at a glance from across a dark field; A1 and DT
// are the two the resolver shows.
var modes = []d2enum.MonsterAnimationMode{
	d2enum.MonsterAnimationModeNeutral,
	d2enum.MonsterAnimationModeWalk,
	d2enum.MonsterAnimationModeAttack1,
	d2enum.MonsterAnimationModeDeath,
}

func main() {
	cfg := d2config.DefaultConfig()

	mpq := flag.String("mpq", cfg.MpqPath, "directory holding the D2 MPQs")
	flag.Parse()

	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		die("asset manager: %v", err)
	}

	fmt.Printf("MPQ path: %s\n", filepath.Clean(*mpq))

	for _, name := range cfg.MpqLoadOrder {
		src := filepath.Join(filepath.Clean(*mpq), name)
		if err := asset.AddSource(src, types.AssetSourceMPQ); err != nil {
			die("MPQ %q not found. Pass -mpq with the directory that holds them.\n  %v", src, err)
		}
	}

	for _, path := range []string{d2resource.MonStats, d2resource.MonStats2} {
		if err := asset.LoadRecords(path); err != nil {
			die("%s: %v", path, err)
		}
	}

	animDataBytes, err := asset.LoadFile(d2resource.AnimationData)
	if err != nil {
		die("%s: %v", d2resource.AnimationData, err)
	}

	animData, err := d2animdata.Load(animDataBytes)
	if err != nil {
		die("animdata: %v", err)
	}

	asset.Records.Animation.Data = animData

	if len(asset.Records.Monster.Stats) == 0 {
		die("monstats.txt loaded but held no records -- the instrument is broken, this is not a finding")
	}

	if !controls(asset) {
		die("a control failed. No findings are reported from an instrument that cannot tell present from absent.")
	}

	measure(asset)
}

// controls runs one case known true and one known false before any number is
// believed (Constitution VI.4(b)). The failure this guards is the one that
// matters most here: geometry read as 0x0 because the composite silently did
// not build would look exactly like a tiny sprite, and a spec written from it
// would be wrong in the most expensive possible way.
func controls(asset *d2asset.AssetManager) bool {
	fmt.Println("=== CONTROLS (VI.4(b)) — run before any number is believed ===")

	ok := true

	rec := asset.Records.Monster.Stats["fallen1"]
	if rec == nil {
		fmt.Println("  KNOWN-TRUE FAILED: no monstats record for fallen1")

		return false
	}

	ex := asset.Records.Monster.Stats2[rec.ExtraDataKey]
	if ex == nil {
		fmt.Println("  KNOWN-TRUE FAILED: no monstats2 record for fallen1")

		return false
	}

	w, h, frames, dirs, err := sizeOf(asset, rec.AnimationDirectoryToken, ex,
		d2enum.MonsterAnimationModeNeutral)
	switch {
	case err != nil:
		fmt.Printf("  KNOWN-TRUE FAILED: fallen1 NU did not build: %v\n", err)

		ok = false
	case w <= 0 || h <= 0:
		fmt.Printf("  KNOWN-TRUE FAILED: fallen1 NU measured %dx%d -- a zero is what a silently "+
			"broken composite looks like, so no numbers below would be trustworthy\n", w, h)

		ok = false
	default:
		fmt.Printf("  known-true  ok: fallen1 NU is %dx%d px, %d frames, %d facings\n", w, h, frames, dirs)
	}

	// KNOWN-FALSE: a token that cannot exist must not measure as anything.
	//
	// THE FIRST DRAFT USED "zz" AND THIS CONTROL FAILED, which is worth keeping in
	// the file rather than quietly fixing: D2's animation tokens are two
	// characters, so most short strings ARE real tokens and "nonsense" has to be
	// longer than the namespace. A control that fails because it was badly chosen
	// looks exactly like a control that fails because the instrument is broken,
	// and the only way to tell them apart is to go and check whether the case
	// really is known-false.
	const impossible = "zzqq7"

	if _, _, _, _, err := sizeOf(asset, impossible, ex, d2enum.MonsterAnimationModeNeutral); err == nil {
		fmt.Printf("  KNOWN-FALSE FAILED: the impossible token %q produced a measurement -- a\n"+
			"    composite that builds for art which does not exist would make every number\n"+
			"    below meaningless\n", impossible)

		ok = false
	} else {
		fmt.Printf("  known-false ok: the impossible token %q refuses to build\n", impossible)
	}

	fmt.Println()

	return ok
}

func measure(asset *d2asset.AssetManager) {
	fmt.Println("=== D2 CREATURE SPRITE SCALE — the numbers the art spec quotes ===")
	fmt.Println("(width x height of one frame, at the game's native 800x600)")
	fmt.Println()

	type row struct {
		code, mode string
		w, h       int
		frames     int
		dirs       int
	}

	rows := make([]row, 0, len(subjects)*len(modes))

	for _, subject := range subjects {
		rec := asset.Records.Monster.Stats[subject.code]
		if rec == nil {
			fmt.Printf("%-10s NO MONSTATS RECORD (the spawn tables name it every night)\n", subject.code)

			continue
		}

		ex := asset.Records.Monster.Stats2[rec.ExtraDataKey]
		if ex == nil {
			fmt.Printf("%-10s no monstats2 record (%q)\n", subject.code, rec.ExtraDataKey)

			continue
		}

		fmt.Printf("%s — %s\n", subject.code, subject.why)
		fmt.Printf("  token=%q  weapon=%q  hp(normal)=%d..%d\n",
			rec.AnimationDirectoryToken, ex.BaseWeaponClass, rec.MinHPNormal, rec.MaxHPNormal)

		for _, mode := range modes {
			w, h, frames, dirs, err := sizeOf(asset, rec.AnimationDirectoryToken, ex, mode)
			if err != nil {
				fmt.Printf("    %-2s  -- %v\n", mode.String(), err)

				continue
			}

			fmt.Printf("    %-2s  %3d x %3d px   %2d frames   %2d facings\n",
				mode.String(), w, h, frames, dirs)

			rows = append(rows, row{subject.code, mode.String(), w, h, frames, dirs})
		}

		fmt.Println()
	}

	if len(rows) == 0 {
		die("nothing measured")
	}

	// The spec needs one number per axis, and the honest one is the range plus
	// the idle pose, because idle is what a generator is asked for first.
	minW, minH, maxW, maxH := rows[0].w, rows[0].h, rows[0].w, rows[0].h
	dirCounts := map[int]int{}
	frameCounts := map[string][]int{}

	for _, r := range rows {
		if r.w < minW {
			minW = r.w
		}

		if r.h < minH {
			minH = r.h
		}

		if r.w > maxW {
			maxW = r.w
		}

		if r.h > maxH {
			maxH = r.h
		}

		dirCounts[r.dirs]++
		frameCounts[r.mode] = append(frameCounts[r.mode], r.frames)
	}

	fmt.Println("=== SUMMARY ===")
	fmt.Printf("  frame size across %d (code, mode) pairs: %d..%d wide, %d..%d tall\n",
		len(rows), minW, maxW, minH, maxH)

	dirs := make([]int, 0, len(dirCounts))
	for d := range dirCounts {
		dirs = append(dirs, d)
	}

	sort.Ints(dirs)

	for _, d := range dirs {
		fmt.Printf("  %2d facings: %d of %d pairs\n", d, dirCounts[d], len(rows))
	}

	modeNames := make([]string, 0, len(frameCounts))
	for m := range frameCounts {
		modeNames = append(modeNames, m)
	}

	sort.Strings(modeNames)

	for _, m := range modeNames {
		fmt.Printf("  %-2s frames per facing: %v\n", m, frameCounts[m])
	}

	fmt.Println(`
READ IT LIKE THIS. A creature's art is a GRID: one row per facing, one column
per frame of that facing. The numbers above are the size of ONE CELL, and the
facing count is how many rows a sheet needs. Anything drawn larger than a cell
and scaled down loses its readability at this size long before it loses detail,
which is the thing to warn an image generator about -- these sprites are small.`)
}

// sizeOf builds one composite, sets one mode, and reports the frame geometry.
//
// It reads GetSize rather than a layer's own bounds because GetSize is what the
// game's own renderer uses (updateSize walks the COF's per-frame priority list
// and takes the largest layer), so it is the size the creature actually occupies
// on screen rather than the size of one body part.
func sizeOf(asset *d2asset.AssetManager, token string, ex *d2records.MonStat2Record,
	mode d2enum.MonsterAnimationMode) (w, h, frames, dirs int, err error) {
	c, err := asset.LoadComposite(d2enum.ObjectTypeCharacter, token, d2resource.PaletteUnits)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("LoadComposite: %w", err)
	}

	var equipment [d2enum.CompositeTypeMax]string

	for compType, opts := range ex.EquipmentOptions {
		if len(opts) != 0 {
			equipment[compType] = opts[0]
		}
	}

	if err := c.Equip(&equipment); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("Equip: %w", err)
	}

	if err := c.SetMode(mode, ex.BaseWeaponClass); err != nil {
		return 0, 0, 0, 0, err
	}

	w, h = c.GetSize()

	return w, h, c.GetFrameCount(), c.DirectionCount(), nil
}

func die(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "spritescale: "+format+"\n", args...)
	os.Exit(1)
}
