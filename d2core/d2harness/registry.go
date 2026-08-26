// Package d2harness holds the state-provider registry for the Project Strigoi
// playtest harness (P3 spec §3.5). The registry compiles into every build
// configuration; the MCP server that reads it exists only behind the `harness`
// build tag (in d2app). A gameplay system registers a Provider at construction
// so the harness can observe it the day it exists; in release builds the
// registry is a slice nobody reads.
//
// The Phase 4 rule (Constitution VI.2, made mechanical): a system is not done
// until its provider exposes every value its S1 §12 playtest assertion needs.
package d2harness

import (
	"sort"
	"sync"
)

// Stateful is the entity-level half of the contract: anything that exposes
// kind-specific observable state as a JSON-encodable map (P3 spec §3.5 —
// Player, NPC, and every future entity kind). The harness reads it for
// strigoi_get_entity and hashes it into the state digest, so "observable" and
// "deterministic" are one checklist. Called on the game goroutine.
type Stateful interface {
	HarnessState() map[string]interface{}
}

// Provider exposes a named system's observable state to the playtest harness.
type Provider interface {
	Stateful

	// HarnessName identifies the system: "clock", "meters", "light", "spawns", ...
	HarnessName() string
}

// Settable is optionally implemented by providers that allow test setup to
// write named fields (an explicit allow-list inside the system).
type Settable interface {
	Provider

	// HarnessSet writes one allow-listed field. Called on the game goroutine.
	HarnessSet(field string, value interface{}) error
}

// FieldLister is optionally implemented by a Settable provider to advertise
// its allow-listed field names (strigoi_list_systems shows them).
type FieldLister interface {
	HarnessSettableFields() []string
}

// nolint:gochecknoglobals // the registry is deliberately process-global
var (
	mu        sync.Mutex
	providers []Provider
)

// Register adds a provider. Safe to call from any goroutine at construction
// time. Registering a name twice is allowed; Lookup returns the newest.
func Register(p Provider) {
	mu.Lock()
	defer mu.Unlock()

	providers = append(providers, p)
}

// Unregister removes a previously registered provider (all occurrences). A
// screen that owns a provider calls it on unload so a stale registration
// never shadows a live one.
func Unregister(p Provider) {
	mu.Lock()
	defer mu.Unlock()

	kept := providers[:0]

	for _, q := range providers {
		if q != p {
			kept = append(kept, q)
		}
	}

	providers = kept
}

// Providers returns a snapshot of the registered providers.
func Providers() []Provider {
	mu.Lock()
	defer mu.Unlock()

	out := make([]Provider, len(providers))
	copy(out, providers)

	return out
}

// Lookup returns the most recently registered provider with the given name.
func Lookup(name string) (Provider, bool) {
	mu.Lock()
	defer mu.Unlock()

	for i := len(providers) - 1; i >= 0; i-- {
		if providers[i].HarnessName() == name {
			return providers[i], true
		}
	}

	return nil, false
}

// Names returns the distinct registered system names, sorted.
func Names() []string {
	mu.Lock()
	defer mu.Unlock()

	seen := map[string]bool{}
	names := make([]string, 0, len(providers))

	for _, p := range providers {
		n := p.HarnessName()
		if seen[n] {
			continue
		}

		seen[n] = true
		names = append(names, n)
	}

	sort.Strings(names)

	return names
}
