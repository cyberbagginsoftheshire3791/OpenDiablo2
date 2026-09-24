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
// with whether a table had it and how often it was asked. The harness's
// "assets" system reports it (strings_asked, strings_missing, strings).

// StringAsk is one key the game has asked for.
type StringAsk struct {
	Key   string
	Found bool
	Text  string // what it was translated to ("" when not found)
	Asks  int
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

	if a, ok := c.byKey[key]; ok {
		a.Asks++
		return
	}

	c.byKey[key] = &StringAsk{Key: key, Found: found, Text: text, Asks: 1}
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
