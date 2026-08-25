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

import "sync"

// Provider exposes a named system's observable state to the playtest harness.
type Provider interface {
	// HarnessName identifies the system: "clock", "meters", "light", "spawns", ...
	HarnessName() string

	// HarnessState returns the system's observable state. Values must be
	// JSON-encodable. Called on the game goroutine.
	HarnessState() map[string]interface{}
}

// Settable is optionally implemented by providers that allow test setup to
// write named fields (an explicit allow-list inside the system).
type Settable interface {
	Provider

	// HarnessSet writes one allow-listed field. Called on the game goroutine.
	HarnessSet(field string, value interface{}) error
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
