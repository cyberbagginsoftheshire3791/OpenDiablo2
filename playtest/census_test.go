//go:build playtest

package playtest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestAssetCensus is M5.2 (23 Sep 2026): the asset census, taken over a
// player's first day and night -- the menus, the kit, talents and help, a
// talk, a forage, a night with the tables and the dead running. It writes
// the whole list to strigoi-harness-runs\asset-census.tsv and a summary by
// area to asset-census.md, and asserts only what an instrument must: that it
// saw both kinds of source, and that none of Strigoi's own files (data/strigoi)
// came out of an MPQ. The MPQ count is the ratchet's metric; it is REPORTED
// here, not pinned -- it is meant to fall.
func TestAssetCensus(t *testing.T) {
	s := start(t)
	s.call("strigoi_pause", map[string]any{})

	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Census", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})

	health := mustNum(t, metersState(s), "health")
	keepAlive := func() {
		setField(s, "meters", "food", 80.0)
		setField(s, "meters", "water", 80.0)
		setField(s, "meters", "fatigue", 10.0)
		setField(s, "meters", "health", health)
	}

	// The panels a player opens on day one.
	for _, k := range []string{"i", "i", "t", "t", "h", "h", "k"} {
		s.call("strigoi_key", map[string]any{"key": k})
		s.call("strigoi_step", map[string]any{"frames": 4})
	}

	headman := villager(t, s, "Warriv")
	walkNear(t, s, headman)
	openTalkWith(t, s, headman)
	s.call("strigoi_key", map[string]any{"key": "escape"})
	s.call("strigoi_step", map[string]any{"frames": 4})

	// A night, as shipped: the tables, the dead, whatever comes.
	for i := 0; i < 80; i++ {
		s.call("strigoi_step_world", map[string]any{"world_minutes": 15.0})
		keepAlive()
	}

	a := sub(s.call("strigoi_get_system_state", map[string]any{"system": "assets"}), "state")

	mpq, native := mustNum(t, a, "mpq_files"), mustNum(t, a, "native_files")
	if mpq == 0 || native == 0 {
		t.Fatalf("the census must see both kinds of source: mpq %.0f, native %.0f", mpq, native)
	}

	var (
		tsv   strings.Builder
		wrong []string
	)

	tsv.WriteString("path\tsource\tfrom\tloads\n")

	for _, raw := range asList(a["files"]) {
		f := raw.(map[string]any)
		fmt.Fprintf(&tsv, "%s\t%s\t%s\t%.0f\n", str(f, "path"), str(f, "source"), str(f, "from"), num(f, "loads"))

		if strings.Contains(str(f, "path"), "data/strigoi/") && str(f, "source") != "native" {
			wrong = append(wrong, str(f, "path"))
		}
	}

	if len(wrong) > 0 {
		t.Fatalf("Strigoi's own files must come from Strigoi's folder, not an MPQ: %v", wrong)
	}

	areas := sub(a, "by_area")
	names := make([]string, 0, len(areas))

	for name := range areas {
		names = append(names, name)
	}

	sort.Strings(names)

	var md strings.Builder

	fmt.Fprintf(&md, "# Asset census\n\nOne player's first day and night (`playtest/census_test.go`). **%.0f files from Diablo II's MPQs, %.0f of Strigoi's own.**\n\n", mpq, native)
	md.WriteString("| Area | From MPQs | Ours |\n|---|---:|---:|\n")

	for _, name := range names {
		m := areas[name].(map[string]any)
		fmt.Fprintf(&md, "| `%s` | %.0f | %.0f |\n", name, num(m, "mpq"), num(m, "native"))
	}

	for file, body := range map[string]string{"asset-census.tsv": tsv.String(), "asset-census.md": md.String()} {
		if err := os.WriteFile(filepath.Join(s.RunBase, file), []byte(body), 0o600); err != nil {
			t.Fatalf("writing %s: %v", file, err)
		}
	}

	t.Logf("asset census: %.0f MPQ files, %.0f native, %d areas -> %s", mpq, native, len(names), filepath.Join(s.RunBase, "asset-census.md"))
}
