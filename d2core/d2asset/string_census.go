package d2asset

import (
	"sort"
	"sync"
)

// THE STRING CENSUS (M5.3, 23 Sep 2026). Diablo II's three string tables
// (data/local/lng: string.tbl, expansionstring.tbl, patchstring.tbl) are
// read whole from the MPQs at boot, for the few hundred keys the UI asks
// for. Replacing them with Strigoi's own words starts with knowing which
// keys those are: every key TranslateString is asked for is recorded here,
// once, with whether a table had it and what it became. The harness's
// "assets" system reports it (strings_asked, strings_missing, strings).
//
// HOW OFTEN a key was asked is deliberately not kept: the HUD asks for its
// tooltips every frame it draws, so the count measured frames drawn, not
// anything about the game -- and, being in the harness state digest, it
// made two launches of the same seeded run differ (docs/harness.md, leak
// register #2).

// StringAsk is one key the game has asked for.
type StringAsk struct {
	Key   string
	Found bool
	Text  string // what it was translated to ("" when not found)
}

type stringCensus struct {
	mu    sync.Mutex
	byKey map[string]*StringAsk
}

func (c *stringCensus) record(key, text string, found bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.byKey == nil {
		c.byKey = map[string]*StringAsk{}
	}

	if _, ok := c.byKey[key]; ok {
		return
	}

	c.byKey[key] = &StringAsk{Key: key, Found: found, Text: text}
}

// StringsAsked is every key asked for so far, sorted by key (copies).
func (am *AssetManager) StringsAsked() []StringAsk {
	am.stringCensus.mu.Lock()
	defer am.stringCensus.mu.Unlock()

	out := make([]StringAsk, 0, len(am.stringCensus.byKey))
	for _, a := range am.stringCensus.byKey {
		out = append(out, *a)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })

	return out
}
