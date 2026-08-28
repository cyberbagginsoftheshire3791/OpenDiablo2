//go:build playtest

package playtest

import (
	"strings"
	"testing"
)

// TestWorldgenAlwaysGeneratesAWorld is the tenth script, and it exists for a
// bug that only ever showed itself on seeds nobody used.
//
// GenerateAct1Overworld drew an Act 1 town layout blind. Three of those
// layouts have a wilderness generator -- East, South, West -- and one does
// not. A draw that landed on the north layout fell through to the switch's
// default branch, which places the town and generates NOTHING around it:
// a village in a void. It was seeded, so it was reproducible; it was just
// reproducible per seed, and every playtest in this repo pins 1462, which
// draws East. **Whether a world existed outside the palisade was a property
// of the seed.**
//
// That matters at Phase 4's own definition of done -- "a friend who owns D2
// can clone, build, and survive one full night" -- because a friend does not
// pass a seed.
//
// So this walks SEVERAL seeds rather than one. A single-seed assertion is
// exactly the assertion that missed this, and re-running it on 1462 would have
// gone on passing forever.
func TestWorldgenAlwaysGeneratesAWorld(t *testing.T) {
	s := start(t)

	s.call("strigoi_pause", map[string]any{})

	// Seeds picked to spread the draw, not to be lucky: 1462 is the one every
	// other script uses (and the one that always worked), and the rest are
	// arbitrary. If the constraint is wrong, one of them lands on the layout
	// with no wilderness.
	seeds := []int{1462, 7, 99, 1234, 20260828, 555}

	seen := map[string]int{}

	for i, seed := range seeds {
		if i > 0 {
			// Back to the menu so the next start regenerates the map.
			s.call("strigoi_navigate", map[string]any{"screen": "main_menu"})
		}

		s.call("strigoi_start_game", map[string]any{
			"hero_name": "Surveyor", "hero_class": "amazon",
			"seed": seed, "wait_seconds": 90,
		})

		region := lastRegionPath(s.gameTail(400))
		if region == "" {
			t.Fatalf("seed %d: the generator logs its Region Path at every map load and none was found", seed)
		}

		seen[region]++

		if !hasWilderness(region) {
			t.Fatalf("seed %d drew %q, which has no wilderness generator -- "+
				"that is a village in a void, and it is the bug this script exists for",
				seed, region)
		}

		t.Logf("seed %-9d -> %s", seed, region)
	}

	t.Logf("layouts drawn across %d seeds: %v", len(seeds), seen)

	// A weak but real guard against the constraint being implemented as
	// "always pick index 0": if every seed drew the same layout, the choice is
	// not being made, it is being hardcoded, and a future edit that broke the
	// draw would look identical to one that did not.
	if len(seen) < 2 {
		t.Logf("NOTE: all %d seeds drew the same layout (%v). That is possible by "+
			"chance with three choices, but if it persists, check that the draw "+
			"is still a draw.", len(seeds), seen)
	}
}

// lastRegionPath pulls the most recent "Region Path: ..." the generator logged.
func lastRegionPath(tail string) string {
	const marker = "Region Path:"

	out := ""

	for _, line := range strings.Split(tail, "\n") {
		if i := strings.Index(line, marker); i >= 0 {
			out = strings.TrimSpace(line[i+len(marker):])
		}
	}

	return out
}

// hasWilderness mirrors the generator's own list. Deliberately written out
// here rather than imported: the script asserts the OUTCOME the engine is
// supposed to produce, and sharing the constant would let one edit move both
// the behaviour and the thing checking it.
func hasWilderness(regionPath string) bool {
	for _, want := range []string{"E1", "S1", "W1"} {
		if strings.Contains(regionPath, want) {
			return true
		}
	}

	return false
}
