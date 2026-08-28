package d2mapstamp

import (
	"math"
	"math/rand"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2ds1"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
)

const logPrefix = "Map Stamp"

// NewStampFactory creates a MapStamp factory instance
func NewStampFactory(asset *d2asset.AssetManager, l d2util.LogLevel, entity *d2mapentity.MapEntityFactory) *StampFactory {
	result := &StampFactory{
		asset:  asset,
		entity: entity,
	}

	result.Logger = d2util.NewLogger()
	result.Logger.SetLevel(l)
	result.Logger.SetPrefix(logPrefix)

	return result
}

// StampFactory is responsible for loading map stamps. A stamp can be thought of like a
// preset map configuration, like the various configurations of Act 1 town.
type StampFactory struct {
	asset  *d2asset.AssetManager
	entity *d2mapentity.MapEntityFactory
	rng    *rand.Rand // the world RNG (P3 E4); nil falls back to the global generator

	*d2util.Logger
}

// SetRand hands the factory the world RNG, so random stamp selection is
// seeded by the map seed instead of the (unseeded) global generator.
// MapEngine.SetSeed calls it.
func (f *StampFactory) SetRand(r *rand.Rand) {
	f.rng = r
}

func (f *StampFactory) randFloat64() float64 {
	if f.rng != nil {
		return f.rng.Float64()
	}

	// nolint:gosec // not cryptographic; pre-seed fallback only
	return rand.Float64()
}

// LoadStamp loads the Stamp data from file, using the given level type, level preset index, and
// level file index.
// PresetFileNames returns a level preset's usable DS1 file names, in the order
// LoadStamp indexes them -- so a caller that needs a SPECIFIC layout can find
// its index here and pass it back as LoadStamp's fileIndex.
//
// It exists because GenerateAct1Overworld has to choose a town layout it can
// actually build a world around, and until now it had no way to ask what the
// choices were: it drew blind and then discovered, in a switch, that it could
// not generate wilderness for what it got. The filter lives here rather than
// being written twice, because two copies of "which files count" would drift.
func (f *StampFactory) PresetFileNames(levelPreset int) []string {
	var names []string

	for _, fileRecord := range f.asset.Records.Level.Presets[levelPreset].Files {
		if fileRecord != "" && fileRecord != "0" {
			names = append(names, fileRecord)
		}
	}

	return names
}

func (f *StampFactory) LoadStamp(levelType d2enum.RegionIdType, levelPreset, fileIndex int) *Stamp {
	stamp := &Stamp{
		factory:     f,
		entity:      f.entity,
		regionID:    levelType,
		levelType:   *f.asset.Records.Level.Types[levelType],
		levelPreset: f.asset.Records.Level.Presets[levelPreset],
	}

	for _, levelTypeDt1 := range &stamp.levelType.Files {
		if levelTypeDt1 == "" || levelTypeDt1 == "0" {
			continue
		}

		dt1, err := f.asset.LoadDT1(levelTypeDt1)
		if err != nil {
			f.Error(err.Error())
			return nil
		}

		stamp.tiles = append(stamp.tiles, dt1.Tiles...)
	}

	levelFilesToPick := f.PresetFileNames(levelPreset)

	levelIndex := int(math.Round(float64(len(levelFilesToPick)-1) * f.randFloat64()))
	if fileIndex >= 0 && fileIndex < len(levelFilesToPick) {
		levelIndex = fileIndex
	}

	if levelFilesToPick == nil {
		panic("no level files to pick from")
	}

	stamp.regionPath = levelFilesToPick[levelIndex]
	fileData, err := f.asset.LoadFile("/data/global/tiles/" + stamp.regionPath)

	if err != nil {
		panic(err)
	}

	stamp.ds1, err = d2ds1.Unmarshal(fileData)
	if err != nil {
		f.Error(err.Error())
		return nil
	}

	return stamp
}
