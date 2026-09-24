package d2asset

import (
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2loader"
)

// assetsProvider is the "assets" harness system (M5.2): the census of every
// file loaded, by source and by area. The ratchet's number is mpq_files.
type assetsProvider struct {
	loader *d2loader.Loader
	am     *AssetManager
}

func (p assetsProvider) HarnessName() string { return "assets" }

func (p assetsProvider) HarnessState() map[string]interface{} {
	entries := p.loader.Census.Entries()

	files := make([]map[string]interface{}, 0, len(entries))
	areas := map[string]map[string]int{}
	counts := map[string]int{d2loader.CensusMPQ: 0, d2loader.CensusNative: 0}
	loads := map[string]int{d2loader.CensusMPQ: 0, d2loader.CensusNative: 0}

	for _, e := range entries {
		files = append(files, map[string]interface{}{"path": e.Path, "source": e.Source, "from": e.From, "loads": e.Loads})
		counts[e.Source]++
		loads[e.Source] += e.Loads

		area := d2loader.Area(e.Path)
		if areas[area] == nil {
			areas[area] = map[string]int{}
		}

		areas[area][e.Source]++
	}

	asked := p.am.StringsAsked()
	strs := make([]map[string]interface{}, 0, len(asked))
	missing := 0

	for _, a := range asked {
		strs = append(strs, map[string]interface{}{"key": a.Key, "found": a.Found, "text": a.Text})

		if !a.Found {
			missing++
		}
	}

	byArea := map[string]interface{}{}
	for a, m := range areas {
		byArea[a] = map[string]interface{}{"mpq": m[d2loader.CensusMPQ], "native": m[d2loader.CensusNative]}
	}

	return map[string]interface{}{
		"mpq_files":    counts[d2loader.CensusMPQ],
		"native_files": counts[d2loader.CensusNative],
		"mpq_loads":    loads[d2loader.CensusMPQ],
		"native_loads": loads[d2loader.CensusNative],
		"by_area":      byArea,
		"files":        files,
		"font_set":     p.am.FontSetPath(), // "" is Diablo II's fonts
		// The string census (string_census.go): every key the UI asked for.
		"string_set":      p.am.StringSetPath(), // "" is Diablo II's string tables
		"strings_asked":   len(asked),
		"strings_missing": missing,
		"strings":         strs,
	}
}
