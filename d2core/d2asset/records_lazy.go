package d2asset

import "sync"

// TABLES THAT LOAD WHEN THEY ARE NEEDED (23 Sep 2026, history item 113).
// Boot loads the tables every game reads (d2app's initDataDictionaries). A
// table only Diablo II's generated world reads -- its DS1 presets and the
// objects and monsters they place -- is loaded by EnsureRecords when that
// world is first built, so the default game, which builds the authored
// village, never reads it from the MPQs. Likewise the tables only a Diablo
// II skill cast reads (d2resource.CastRecords, history item 116) load on the
// first cast.
type recordsLoaded struct {
	mu   sync.Mutex
	done map[string]bool
}

// LoadRecords loads the data dictionary at path into the RecordManager.
func (am *AssetManager) LoadRecords(path string) error {
	am.recordsLoaded.mu.Lock()
	defer am.recordsLoaded.mu.Unlock()

	return am.loadRecords(path)
}

// EnsureRecords loads each table at paths that is not loaded yet, in order,
// and stops at the first that fails (a failed table is not marked, so a
// later call tries it again). Safe from any goroutine: the server and the
// client both build the world.
func (am *AssetManager) EnsureRecords(paths ...string) error {
	am.recordsLoaded.mu.Lock()
	defer am.recordsLoaded.mu.Unlock()

	for _, p := range paths {
		if am.recordsLoaded.done[p] {
			continue
		}

		if err := am.loadRecords(p); err != nil {
			return err
		}
	}

	return nil
}

// RecordsLoaded reports whether the table at path has been loaded.
func (am *AssetManager) RecordsLoaded(path string) bool {
	am.recordsLoaded.mu.Lock()
	defer am.recordsLoaded.mu.Unlock()

	return am.recordsLoaded.done[path]
}

// loadRecords is LoadRecords with the lock held.
func (am *AssetManager) loadRecords(path string) error {
	dict, err := am.LoadDataDictionary(path)
	if err != nil {
		return err
	}

	if err := am.Records.Load(path, dict); err != nil {
		return err
	}

	if am.recordsLoaded.done == nil {
		am.recordsLoaded.done = map[string]bool{}
	}

	am.recordsLoaded.done[path] = true

	return nil
}
