package d2loader

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// M5.2 (23 Sep 2026): the asset census. Every file the game loads, and which
// kind of source it came from -- a Blizzard MPQ, or Strigoi's own files. The
// MPQ count is the asset-independence ratchet's metric (plan §5; Constitution
// Article V.2): it only ever falls, and the game needs no copy of Diablo II
// when it reaches zero.

// Source kinds the census tells apart.
const (
	CensusMPQ    = "mpq"
	CensusNative = "native"
)

// CensusEntry is one file the game has loaded.
type CensusEntry struct {
	Path   string
	Source string // CensusMPQ or CensusNative
	From   string // the source's own path: which MPQ, or which folder
	Loads  int
}

// Census records loads. Loads come from more than one goroutine (a screen's
// OnLoad runs on its own -- BUG-5), so it is locked.
type Census struct {
	mu     sync.Mutex
	byPath map[string]*CensusEntry
}

// NewCensus is an empty census.
func NewCensus() *Census { return &Census{byPath: map[string]*CensusEntry{}} }

// Record notes one load of path from a source at sourcePath.
func (c *Census) Record(path, sourcePath string) {
	if c == nil {
		return
	}

	kind := CensusNative
	if strings.EqualFold(filepath.Ext(sourcePath), ".mpq") {
		kind = CensusMPQ
	}

	key := strings.ToLower(filepath.ToSlash(path))

	c.mu.Lock()
	defer c.mu.Unlock()

	if e, ok := c.byPath[key]; ok {
		e.Loads++
		return
	}

	c.byPath[key] = &CensusEntry{Path: key, Source: kind, From: filepath.Base(sourcePath), Loads: 1}
}

// Entries is every file loaded so far, sorted by path (copies).
func (c *Census) Entries() []CensusEntry {
	if c == nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]CensusEntry, 0, len(c.byPath))
	for _, e := range c.byPath {
		out = append(out, *e)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })

	return out
}

// Area is the census's grouping of a path: the first three folders it lives
// in, which in D2's layout say what the file is for (data/global/ui,
// data/global/tiles, data/local/font ...) and in Strigoi's likewise
// (data/strigoi/creatures). A file in a shallower folder groups by what
// folders it has.
func Area(path string) string {
	parts := strings.Split(strings.Trim(filepath.ToSlash(strings.ToLower(path)), "/"), "/")
	dirs := parts[:len(parts)-1]

	if len(dirs) > 3 {
		dirs = dirs[:3]
	}

	if len(dirs) == 0 {
		return "."
	}

	return strings.Join(dirs, "/")
}
