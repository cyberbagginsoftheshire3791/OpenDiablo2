package d2mappalette

import (
	"strings"
	"testing"
)

// The village's real note, verbatim from data/strigoi/maps/village.tmj's "note"
// map property. The loader reads this property and discards it (tiled.go:325),
// so a copy here is the only way a test can assert on it -- and
// TestShippedVillageCatalog checks the real file still says it.
const villageNote = "M5.4 v0 village. ALL TILE ART IS PLACEHOLDER (tools/villagemap); " +
	"the layout is a proposal from S1 section 9.1 and G4, Josh decides."

// The real "integration" values, verbatim from strigoi-art: the four installed
// structures all say the first, and all six Dealu modules say the second.
const (
	artReady    = "art-ready"
	dealuReady  = "art-ready; not installed; collision and occlusion need placement review"
	queuedValue = "queued"
)

func TestPathSaysPlaceholder(t *testing.T) {
	yes := []string{
		"maps/tiles/placeholder-grass.png",
		"hero/placeholder/hero.json",
		"PLACEHOLDER-church.png",
		"a/b/Placeholder_thing.png",
		`maps\tiles\placeholder-ditch.png`,
	}

	no := []string{
		"structures/peasant-house/intact.png",
		"renders/structures/dealu-monastery-v1/church-intact-960x768.png",
		"maps/tiles/grass.png",
		"",
	}

	for _, p := range yes {
		if !PathSaysPlaceholder(p) {
			t.Errorf("PathSaysPlaceholder(%q) = false", p)
		}
	}

	for _, p := range no {
		if PathSaysPlaceholder(p) {
			t.Errorf("PathSaysPlaceholder(%q) = true", p)
		}
	}
}

// TestDeriveStatusPrecedence is the whole honesty argument in one table. There
// is no approval field in the engine, so every row here is a real signal doing
// the work, and the last row is the one that matters most: no signal means
// Unknown, never Approved.
func TestDeriveStatusPrecedence(t *testing.T) {
	cases := []struct {
		name    string
		in      Signals
		want    Status
		saysAll []string
		signals int
	}{
		{
			name: "a village tile: placeholder filename and a map note that agrees",
			in: Signals{
				Path:        "maps/tiles/placeholder-church.png",
				MapNote:     villageNote,
				MapNoteFrom: "maps/village.tmj",
			},
			want:    StatusPreview,
			saysAll: []string{"placeholder", "maps/tiles/placeholder-church.png"},
			signals: 2,
		},
		{
			name: "an installed structure the art repo calls art-ready",
			in: Signals{
				Path:            "structures/peasant-house/intact.png",
				Integration:     artReady,
				IntegrationFrom: "structures/peasant-house.json",
				MapNote:         villageNote,
				MapNoteFrom:     "maps/village.tmj",
			},
			want: StatusApproved,
			// The map note is still RECORDED even though it did not decide:
			// the user can see the map calls its tile art placeholder while
			// this particular file is an art-repo render.
			saysAll: []string{"Approved", "structures/peasant-house.json", artReady},
			signals: 2,
		},
		{
			name: "a Dealu module: integration says more than art-ready",
			in: Signals{
				Path:            "renders/structures/dealu-monastery-v1/church-intact-960x768.png",
				Integration:     dealuReady,
				IntegrationFrom: "structures/dealu-monastery-v1/modules.json",
			},
			want:    StatusPreview,
			saysAll: []string{"Preview", dealuReady, "not the plain"},
			signals: 1,
		},
		{
			name: "the conflict: a placeholder filename the art repo calls art-ready",
			in: Signals{
				Path:            "maps/tiles/placeholder-well.png",
				Integration:     artReady,
				IntegrationFrom: "structures/village-well.json",
			},
			want:    StatusPreview,
			saysAll: []string{"Preview", "placeholder", "structures/village-well.json", "disagreement"},
			signals: 2,
		},
		{
			name: "only a map note",
			in: Signals{
				Path:        "maps/tiles/grass.png",
				MapNote:     villageNote,
				MapNoteFrom: "maps/village.tmj",
			},
			want:    StatusPreview,
			saysAll: []string{"Preview", "maps/village.tmj", "covers the whole map"},
			signals: 1,
		},
		{
			name: "a map note that says nothing about placeholders",
			in: Signals{
				Path:        "maps/tiles/grass.png",
				MapNote:     "The layout is a proposal; Josh decides.",
				MapNoteFrom: "maps/village.tmj",
			},
			want:    StatusUnknown,
			saysAll: []string{"Unknown", "no approval field"},
			signals: 0,
		},
		{
			name:    "no signal at all",
			in:      Signals{Path: "structures/some-new-thing/intact.png"},
			want:    StatusUnknown,
			saysAll: []string{"Unknown", "no approval field"},
			signals: 0,
		},
		{
			name: "an integration value that is not art-ready at all",
			in: Signals{
				Path:            "structures/barn/intact.png",
				Integration:     queuedValue,
				IntegrationFrom: "structures/barn.json",
			},
			want:    StatusPreview,
			saysAll: []string{"Preview", queuedValue},
			signals: 1,
		},
		{
			name: "art-ready with whitespace and odd case is still art-ready",
			in: Signals{
				Path:            "structures/barn/intact.png",
				Integration:     "  Art-Ready  ",
				IntegrationFrom: "structures/barn.json",
			},
			want:    StatusApproved,
			signals: 1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, why, signals := DeriveStatus(c.in)

			if got != c.want {
				t.Errorf("status %q, want %q (why: %s)", got, c.want, why)
			}

			if why == "" {
				t.Error("no reason given; the editor has nothing to show the user")
			}

			for _, want := range c.saysAll {
				if !strings.Contains(why, want) {
					t.Errorf("the reason does not mention %q: %s", want, why)
				}
			}

			if len(signals) != c.signals {
				t.Errorf("%d signals recorded, want %d: %v", len(signals), c.signals, signals)
			}
		})
	}
}

// TestIntegrationIsAnExactMatchAndNotASubstring is the anti-fragility test.
//
// "art-ready" is matched exactly, trimmed and case-folded, and NOTHING ELSE. A
// substring match would read the Dealu monastery's "art-ready; not installed;
// collision and occlusion need placement review" as an approval, which is the
// opposite of what it says. Every string below contains the literal and must
// still come back Preview.
func TestIntegrationIsAnExactMatchAndNotASubstring(t *testing.T) {
	notApprovals := []string{
		dealuReady,
		"art-ready but the collision is wrong",
		"not art-ready",
		"art-ready?",
		"pre-art-ready",
		"art-readyish",
	}

	for _, v := range notApprovals {
		got, why, _ := DeriveStatus(Signals{Integration: v, IntegrationFrom: "some.json"})
		if got != StatusPreview {
			t.Errorf("integration %q gave %q; only the bare literal %q means approved (why: %s)",
				v, got, artReady, why)
		}
	}

	if got, _, _ := DeriveStatus(Signals{Integration: artReady, IntegrationFrom: "x.json"}); got != StatusApproved {
		t.Errorf("integration %q gave %q, want approved", artReady, got)
	}
}
