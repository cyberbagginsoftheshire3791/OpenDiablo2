package d2mappalette

import (
	"path"
	"strings"
)

// Status is how far along the art is.
//
// THE ENGINE HAS NO APPROVAL FIELD. A search of the tree for "approved" or
// "approval" outside tests returns nothing; d2records, d2asset and d2maptiled
// carry no review, sign-off or status field on any asset, and the one
// machine-readable "not finished" record in the whole repository is the
// knownUnanchored allowlist in d2core/d2asset/png_sheet_anchor_test.go:47,
// which names sheet folders whose art is drawn off its foot point and has
// nothing to say about a map tile.
//
// So this package does not invent one. Status is derived from the signals that
// genuinely exist, by [DeriveStatus], and [Entry.Signals] records every signal
// found so the editor can show the user what the verdict rests on. An asset
// with no signal at all gets [StatusUnknown] and says so, because "we do not
// know" is a true answer and "approved" would not be.
type Status string

// The three states. There is no fourth: a signal this package cannot read
// honestly produces Unknown, not a guess.
const (
	// StatusApproved: something that speaks for this asset says the art is
	// finished. In practice that is one thing only -- a strigoi-art structure
	// manifest whose "integration" value is exactly "art-ready".
	StatusApproved Status = "approved"

	// StatusPreview: something says this is not final art. A "placeholder"
	// element in the path, a map note that says so, or an "integration" value
	// that is anything other than plain "art-ready".
	StatusPreview Status = "preview"

	// StatusUnknown: nothing anywhere says either way.
	StatusUnknown Status = "unknown"
)

// Signals is the evidence [DeriveStatus] works from. Every field is optional,
// and a zero Signals gives StatusUnknown.
//
// WHAT IS DELIBERATELY NOT HERE. The strigoi-art repository was read for this
// (43 files under provenance/, four structures/*.json, and the *-QUEUE.md
// tables) and most of what it says about readiness is prose that would be
// dishonest to parse:
//
//   - provenance/*.md: 20 of the 43 files carry a "- Status:" bullet, and its
//     value is free text with no vocabulary -- "draft v1", "locomotion
//     checkpoint. Combat actions...", "current integrated production source.",
//     "placeholder; not approved final game art". The 4 structure provenance
//     files do not use that bullet at all; they use "- Game-facing state:
//     art-ready. Live placement and collision await a ..." instead. Two
//     different conventions and no enumerated values: not parsed.
//   - STRUCTURE-QUEUE.md and the other *-QUEUE.md files: a Markdown table whose
//     Status column reads "Art-ready v1", "Queued", and for the monastery
//     "Art-ready v1; validated and independently reviewed; not installed".
//     Prose in a table is still prose: not parsed.
//   - modules.json's "walkability" and "rectangular_canvas_note" keys: free
//     text, and both are WRONG at HEAD d1d07434. "walkability" says a gate
//     needs "separate engine collision, not one blocked compound footprint",
//     and this burst measured a one-tile gap between two 3x3 modules walking
//     25/25 and passing sight while the module column blocked it. The note is
//     boilerplate repeated verbatim on all six modules, including the three
//     that are square. Not parsed, and not believed.
//
// The one key that IS read is "integration", because it is a real JSON key with
// a value that is exactly "art-ready" on all four installed structures. Even
// there the comparison is an exact match on that one literal and nothing else:
// anything longer -- the monastery's "art-ready; not installed; collision and
// occlusion need placement review" -- falls to Preview with the raw string shown
// to the user, rather than being substring-matched into an approval it does not
// claim.
type Signals struct {
	// Path is the art's path. A "placeholder" element anywhere in it (any
	// directory, or a prefix or infix of the filename) is the strongest signal
	// there is, because it is the artist's own label on his own file.
	Path string

	// MapNote is the "note" property of the .tmj the art was found in, verbatim.
	// The loader reads this property and throws it away (tiled.go:325), so this
	// package parses the .tmj itself to see it. village.tmj's says "ALL TILE ART
	// IS PLACEHOLDER (tools/villagemap)".
	MapNote string

	// MapNoteFrom is the .tmj MapNote came from, for the reason sentence.
	MapNoteFrom string

	// Integration is a strigoi-art manifest's "integration" value, verbatim, and
	// IntegrationFrom is the manifest it came from.
	Integration     string
	IntegrationFrom string
}

// integrationApproved is the one literal that means finished art. Exact match,
// trimmed, case-folded -- nothing else. See [Signals].
const integrationApproved = "art-ready"

// placeholderWord is matched against each element of the art's path.
const placeholderWord = "placeholder"

// PathSaysPlaceholder reports whether any element of p is or contains
// "placeholder", case-insensitively: "tiles/placeholder-church.png" and
// "hero/placeholder/hero.json" both do.
func PathSaysPlaceholder(p string) bool {
	for _, el := range strings.Split(path.Clean(strings.ReplaceAll(p, "\\", "/")), "/") {
		if strings.Contains(strings.ToLower(el), placeholderWord) {
			return true
		}
	}

	return false
}

// noteSaysPlaceholder reports whether a map's note admits its art is
// placeholder. The note is prose, so this is a single case-folded substring
// test for the one word, and the note is quoted back to the user in full rather
// than summarised -- see [DeriveStatus].
func noteSaysPlaceholder(note string) bool {
	return strings.Contains(strings.ToLower(note), placeholderWord)
}

// DeriveStatus turns the evidence into a verdict, a sentence for the user, and
// the list of every signal seen.
//
// PRECEDENCE, strongest first:
//
//  1. "placeholder" in the art's own path. Per-asset, written by whoever made
//     the file, and it beats an approval elsewhere: if a file is named
//     placeholder-church.png and a manifest calls it art-ready, the palette
//     says Preview and the reason names both, because that disagreement is
//     exactly what the user needs to see.
//  2. An "integration" value. Exactly "art-ready" is Approved; anything else
//     non-empty is Preview with the value quoted.
//  3. The map's note. Map-wide rather than per-asset, so it comes last of the
//     three: village.tmj's note says "ALL TILE ART IS PLACEHOLDER
//     (tools/villagemap)" and the four structure PNGs beside it are art-repo
//     renders that the note is not about. It still marks anything the first two
//     rules left undecided, because a map that says its art is placeholder is
//     evidence.
//  4. Nothing: Unknown.
//
// The returned list holds every signal found, in that order, whether or not it
// decided the verdict.
func DeriveStatus(s Signals) (Status, string, []string) {
	var (
		found  []string
		status = StatusUnknown
		why    string
	)

	placeholderPath := s.Path != "" && PathSaysPlaceholder(s.Path)
	integration := strings.TrimSpace(s.Integration)
	approved := strings.EqualFold(integration, integrationApproved)

	if placeholderPath {
		found = append(found, "its path says \"placeholder\": "+s.Path)
	}

	if integration != "" {
		found = append(found, "the art manifest "+s.IntegrationFrom+" says integration "+quote(integration))
	}

	if s.MapNote != "" && noteSaysPlaceholder(s.MapNote) {
		found = append(found, "the map "+s.MapNoteFrom+" carries the note "+quote(s.MapNote))
	}

	switch {
	case placeholderPath && approved:
		status = StatusPreview
		why = "Preview: the file is named as a placeholder (" + s.Path + ") even though " +
			s.IntegrationFrom + " calls it " + quote(integration) + ". The filename wins here, " +
			"and the disagreement is worth a look."
	case placeholderPath:
		status = StatusPreview
		why = "Preview: the art itself is named as a placeholder (" + s.Path + "), " +
			"so somebody meant to replace it."
	case approved:
		status = StatusApproved
		why = "Approved art: " + s.IntegrationFrom + " records integration " + quote(integration) +
			". That is the only machine-readable approval this project has; there is no approval " +
			"field in the engine."
	case integration != "":
		status = StatusPreview
		why = "Preview: " + s.IntegrationFrom + " records integration " + quote(integration) +
			", which is not the plain \"art-ready\" that means finished."
	case s.MapNote != "" && noteSaysPlaceholder(s.MapNote):
		status = StatusPreview
		why = "Preview: the map " + s.MapNoteFrom + " says of itself " + quote(s.MapNote) +
			" -- that note covers the whole map, not this one file."
	default:
		status = StatusUnknown
		why = "Unknown: nothing says whether this art is finished. There is no approval field in " +
			"the engine, the path does not say \"placeholder\", and no art manifest was found for it."
	}

	return status, why, found
}

func quote(s string) string { return "\"" + s + "\"" }
