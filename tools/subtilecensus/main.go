// Command subtilecensus answers the one question BUG-9's fix turns on: is D2's
// dedicated BlockLOS bit actually POPULATED in the map art, or is it sparse?
//
// checkLos (d2core/d2map/d2mapengine/pathfind.go) consults flags.BlockWalk.
// D2 authored a separate BlockLOS bit, decoded at d2dt1/subtile.go:72 from
// data&2, and the engine reads it nowhere. Changing that one word made the
// 19 Sep unaided-chain probe go from "nothing ever notices the player" to
// "dead at 04:32" -- but there are two very different explanations, and they
// demand opposite decisions:
//
//	(a) sight is now CORRECT, honouring the blockers D2 authored for it; or
//	(b) BlockLOS is barely set in the real art, so sight became nearly UNBLOCKED.
//
// No amount of outcome measurement separates those. This counts the bits.
//
// IT CHANGES NOTHING. It loads records and DT1s, counts, and prints.
//
// ARTICLE V. This reads the MPQs at runtime and writes nothing. No extracted
// data may be committed, and this tool must never run on CI -- there are no
// MPQs there. It is a main package so `go build ./...` compiles it and never
// runs it.
//
// Usage, from the repository root:
//
//	go run ./tools/subtilecensus
//	go run ./tools/subtilecensus -v
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2loader/asset/types"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2config"
)

// The regions the slice actually draws. Act 1 town is where seed 1462 opens and
// where every measurement in the record was taken; the wilderness beside it is
// where the spawn tables put their arrivals.
var regions = []struct {
	name string
	id   d2enum.RegionIdType
}{
	{"Act 1 Town", d2enum.RegionAct1Town},
	{"Act 1 Wilderness", d2enum.RegionAct1Wilderness},
	{"Act 1 Cave", d2enum.RegionAct1Cave},
}

type counts struct {
	subtiles                int
	blockWalk, blockLOS     int
	both, walkOnly, losOnly int
	blockLight, blockJump   int
	playerWalk              int
}

func (c *counts) add(walk, los, jump, playerWalk, light bool) {
	c.subtiles++

	switch {
	case walk && los:
		c.both++
	case walk:
		c.walkOnly++
	case los:
		c.losOnly++
	}

	if walk {
		c.blockWalk++
	}

	if los {
		c.blockLOS++
	}

	if light {
		c.blockLight++
	}

	if jump {
		c.blockJump++
	}

	if playerWalk {
		c.playerWalk++
	}
}

func main() {
	cfg := d2config.DefaultConfig()

	mpq := flag.String("mpq", cfg.MpqPath, "directory holding the D2 MPQs")
	verbose := flag.Bool("v", false, "list per-DT1 tile counts")
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

	if err := asset.LoadRecords(d2resource.LevelType); err != nil {
		die("LvlTypes.txt: %v", err)
	}

	if len(asset.Records.Level.Types) == 0 {
		die("level types loaded but held no records -- the instrument is broken, this is not a finding")
	}

	overall := &counts{}
	seen := map[string]bool{}

	for _, region := range regions {
		if int(region.id) >= len(asset.Records.Level.Types) {
			fmt.Printf("%s: no such level type id %d\n\n", region.name, region.id)

			continue
		}

		rec := asset.Records.Level.Types[region.id]
		if rec == nil {
			fmt.Printf("%s: nil level type record\n\n", region.name)

			continue
		}

		per := &counts{}
		files := 0

		for _, f := range rec.Files {
			if f == "" || f == "0" {
				continue
			}

			name := strings.ToLower(strings.ReplaceAll(f, `\`, "/"))

			dt1, err := asset.LoadDT1(name)
			if err != nil {
				if *verbose {
					fmt.Printf("    %-46s UNREADABLE: %v\n", name, err)
				}

				continue
			}

			files++
			fresh := !seen[name]
			seen[name] = true

			for i := range dt1.Tiles {
				for _, sf := range dt1.Tiles[i].SubTileFlags {
					per.add(sf.BlockWalk, sf.BlockLOS, sf.BlockJump, sf.BlockPlayerWalk, sf.BlockLight)

					if fresh {
						overall.add(sf.BlockWalk, sf.BlockLOS, sf.BlockJump, sf.BlockPlayerWalk, sf.BlockLight)
					}
				}
			}

			if *verbose {
				fmt.Printf("    %-46s tiles=%d\n", name, len(dt1.Tiles))
			}
		}

		report(fmt.Sprintf("%s -- %q, %d DT1s read", region.name, rec.Name, files), per)
	}

	report("ALL REGIONS ABOVE, each DT1 counted once", overall)
	verdict(overall)
}

func report(title string, c *counts) {
	fmt.Printf("%s\n", title)
	fmt.Printf("  subtiles counted    %9d\n", c.subtiles)
	fmt.Printf("  BlockWalk           %9d  (%6.2f%% of subtiles)\n", c.blockWalk, pct(c.blockWalk, c.subtiles))
	fmt.Printf("  BlockLOS            %9d  (%6.2f%% of subtiles)\n", c.blockLOS, pct(c.blockLOS, c.subtiles))
	fmt.Printf("    both bits set     %9d\n", c.both)
	fmt.Printf("    BlockWalk ONLY    %9d   <- under the fix: cannot walk, CAN see through\n", c.walkOnly)
	fmt.Printf("    BlockLOS ONLY     %9d   <- under the fix: can walk, CANNOT see through\n", c.losOnly)
	fmt.Printf("  BlockLight          %9d\n", c.blockLight)
	fmt.Printf("  BlockJump           %9d\n", c.blockJump)
	fmt.Printf("  BlockPlayerWalk     %9d\n", c.playerWalk)
	fmt.Println()
}

func verdict(c *counts) {
	fmt.Println("VERDICT")

	// THE DISCRIMINATING STATISTIC IS losOnly, NOT THE RATIO. A first version of
	// this tool branched on BlockLOS-vs-BlockWalk share and called 11.6%
	// "populated", which reads as support for the swap. It is not: what decides
	// whether the swap can ever ADD a blocker is whether any subtile sets
	// BlockLOS without BlockWalk. If none does, BlockLOS is a strict subset and
	// the swap is a pure removal, whatever its share.
	switch {
	case c.blockLOS == 0:
		fmt.Println("  BlockLOS is NEVER SET in this art. Reading it in place of BlockWalk does not fix")
		fmt.Println("  sight -- it REMOVES sight blocking entirely.")
	case c.losOnly == 0:
		fmt.Printf("  BlockLOS IS A STRICT SUBSET OF BlockWalk: %d subtiles set it, and every single\n", c.blockLOS)
		fmt.Printf("  one also sets BlockWalk (BlockLOS-only = 0). So swapping the word can NEVER add\n")
		fmt.Printf("  a blocker -- it only removes them, taking sight blockers from %d to %d, a %.1f%%\n",
			c.blockWalk, c.blockLOS, 100-pct(c.blockLOS, c.blockWalk))
		fmt.Println("  reduction. Any increase in awareness that follows is UNBLOCKED sight, not")
		fmt.Println("  corrected sight, and the one-word remedy as filed is wrong.")
	default:
		fmt.Printf("  BlockLOS carries %d subtiles that BlockWalk does NOT (of %d set). The swap both\n",
			c.losOnly, c.blockLOS)
		fmt.Println("  adds and removes blockers, so it is a genuine rule change with real blockers")
		fmt.Println("  behind it rather than a removal, and the two counts below bound its effect.")
	}
}

func pct(n, of int) float64 {
	if of == 0 {
		return 0
	}

	return 100 * float64(n) / float64(of)
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "subtilecensus: "+format+"\n", args...)
	os.Exit(1)
}
