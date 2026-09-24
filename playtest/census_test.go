//go:build playtest

package playtest

import (
	"encoding/json"
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

	// The string census: every key the UI asked for, and what it became --
	// the list Strigoi's own words must cover to replace Diablo II's string
	// tables. (Their text is Blizzard's: it is written beside the repo, never
	// into it.)
	var strs strings.Builder

	strs.WriteString("key\tfound\tasks\ttext\n")

	for _, raw := range asList(a["strings"]) {
		k := raw.(map[string]any)
		fmt.Fprintf(&strs, "%s\t%v\t%.0f\t%q\n", str(k, "key"), k["found"], num(k, "asks"), str(k, "text"))
	}

	if mustNum(t, a, "strings_asked") == 0 {
		t.Fatalf("the string census saw no key asked for; the menus and panels ask for dozens")
	}

	// Strigoi's own words (data/strigoi/strings/strings.json) must carry the
	// same printf verbs as the text they replace, key for key: the game fills
	// them in, and a verb too many or too few prints %!(EXTRA ...) or
	// %!s(MISSING). This run reads Diablo II's tables, so their text is here
	// to compare against.
	ours := map[string]string{}

	if data, err := os.ReadFile(filepath.Join("..", "data", "strigoi", "strings", "strings.json")); err != nil {
		t.Fatalf("reading Strigoi's string table: %v", err)
	} else if err := json.Unmarshal(data, &ours); err != nil {
		t.Fatalf("Strigoi's string table: %v", err)
	}

	verbs := func(s string) string {
		out := ""

		for i := 0; i+1 < len(s); i++ {
			if s[i] == '%' {
				out += s[i : i+2]
				i++
			}
		}

		return out
	}

	var mismatched []string

	for _, raw := range asList(a["strings"]) {
		k := raw.(map[string]any)
		text, mine := ours[str(k, "key")]

		if k["found"] == true && mine && verbs(text) != verbs(str(k, "text")) {
			mismatched = append(mismatched, fmt.Sprintf("%s: ours %q, the game's %q", str(k, "key"), verbs(text), verbs(str(k, "text"))))
		}
	}

	if len(mismatched) > 0 {
		t.Fatalf("Strigoi's words drop or add format verbs: %v", mismatched)
	}

	for file, body := range map[string]string{"asset-census.tsv": tsv.String(), "asset-census.md": md.String(), "string-census.tsv": strs.String()} {
		if err := os.WriteFile(filepath.Join(s.RunBase, file), []byte(body), 0o600); err != nil {
			t.Fatalf("writing %s: %v", file, err)
		}
	}

	t.Logf("asset census: %.0f MPQ files, %.0f native, %d areas -> %s", mpq, native, len(names), filepath.Join(s.RunBase, "asset-census.md"))
	t.Logf("string census: %.0f keys asked, %.0f not found -> %s", num(a, "strings_asked"), num(a, "strings_missing"), filepath.Join(s.RunBase, "string-census.tsv"))
}
