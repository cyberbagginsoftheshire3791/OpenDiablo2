package d2app

import (
	"fmt"
	"path/filepath"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2loader/asset/types"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2animdata"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2config"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2gui"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2screen"
)

func (a *App) initialize() error {
	if err := a.initConfig(a.config); err != nil {
		return err
	}

	a.initLanguage()

	// M5.3: before the first label, so no Diablo II font is ever loaded. A
	// refused set is reported and Diablo II's fonts stand.
	if a.Options.fontSet != nil && *a.Options.fontSet != "" {
		if err := a.asset.UseFontSet(*a.Options.fontSet); err != nil {
			a.Errorf("font set refused, drawing Diablo II's fonts: %v", err)
		}
	}

	if err := a.initDataDictionaries(); err != nil {
		return err
	}

	a.timeScale = 1.0
	a.lastTime = d2util.Now()
	a.lastScreenAdvance = a.lastTime

	a.renderer.SetWindowIcon("d2logo.png")
	a.terminal.BindLogger()
	a.initTerminalCommands()

	gui, err := d2gui.CreateGuiManager(a.asset, *a.Options.LogLevel, a.inputManager)
	if err != nil {
		return err
	}

	a.guiManager = gui

	a.screen = d2screen.NewScreenManager(a.ui, *a.Options.LogLevel, a.guiManager)

	a.audio.SetVolumes(a.config.BgmVolume, a.config.SfxVolume)

	if err := a.loadStrings(); err != nil {
		return err
	}

	a.ui.Initialize()

	return nil
}

const (
	fmtErrSourceNotFound = `file not found: %q

Please check your config file at %q

Also, verify that the MPQ files exist at %q

Capitalization in the file name matters.
`
)

func (a *App) initConfig(config *d2config.Configuration) error {
	a.config = config

	for _, mpqName := range a.config.MpqLoadOrder {
		cleanDir := filepath.Clean(a.config.MpqPath)
		srcPath := filepath.Join(cleanDir, mpqName)

		err := a.asset.AddSource(srcPath, types.AssetSourceMPQ)
		if err != nil {
			// nolint:stylecheck // we want a multiline error message here..
			return fmt.Errorf(fmtErrSourceNotFound, srcPath, a.config.Path(), a.config.MpqPath)
		}
	}

	return nil
}

func (a *App) initLanguage() {
	// With Strigoi's words AND fonts the install's language chooses nothing
	// -- it picks Diablo II's string tables and font folders -- so its file
	// (data/local/use, from the MPQs) is not read.
	if a.Options.strings != nil && *a.Options.strings != "" && a.Options.fontSet != nil && *a.Options.fontSet != "" {
		a.language = a.asset.UseDefaultLanguage()
	} else {
		a.language = a.asset.LoadLanguage(d2resource.LocalLanguage)
	}
	a.asset.Loader.SetLanguage(&a.language)

	a.charset = d2resource.GetFontCharset(a.language)
	a.asset.Loader.SetCharset(&a.charset)
}

func (a *App) initDataDictionaries() error {
	// THE TABLES THE GAME READS (23 Sep 2026, M5.3's census): 20 of the 83
	// Diablo II .txt tables this list used to load. 52 of the rest filled
	// RecordManager fields that nothing outside d2core/d2records ever reads --
	// LevelWarp, Books, MonProp, MonType, MonMode, ItemRatio, StorePage,
	// Hireling, Gems, QualityItems, Runes, DifficultyLevels, AutoMap,
	// LevelMaze, LevelSubstitutions, CubeRecipes, SuperUniques, SkillCalc,
	// MissileCalc, BodyLocations, AutoMagic, TreasureClassEx, States, Shrines,
	// ElemType, PlrMode, PetType, NPC, MonsterUniqueModifier, MonsterEquipment,
	// UniqueAppellation, MonsterLevel, MonsterSound, MonsterSequence,
	// PlayerClass, MonsterPlacement, ObjectGroup (parsed, then discarded),
	// CompCode, MonsterAI, Events, Colors, ArmorType, WeaponClass, PlayerType,
	// Composite, HitClass, UniquePrefix, UniqueSuffix, CubeModifier, CubeType,
	// HirelingDescription, LowQualityItems -- so each was a file read from an
	// MPQ for nothing. Their loaders stay registered (d2records), so one comes
	// back by adding it here; code that starts reading one of those fields
	// must add its table, or it reads an empty record set. How it was
	// checked: for each loader, the fields it assigns, grepped repo-wide
	// outside d2records and tests (history item 106).
	//
	// ELEVEN MORE GO (23 Sep 2026, history item 111): ItemTypes, UniqueItems,
	// MagicPrefix, MagicSuffix, ItemStatCost, Properties, Sets, SetItems,
	// TreasureClass, RarePrefix, RareSuffix. They fed Diablo II's item
	// generator (d2core/d2item/diablo2item) its affixes, uniques, sets, stats
	// and drops, and the one thing the game made with it was OpenDiablo2's
	// test items in the grid inventory -- gone, since Strigoi's kit is the
	// inventory. Without them the generator still makes plain items from
	// Weapons/Armor/Misc (the debug console's spawnitem, the harness's item
	// spawn); an affix, unique or set code it no longer knows is ignored. How
	// it was checked: every reader of those fields outside d2records, by grep
	// (the list in history item 111), and the playtest suite both ways.
	//
	// The order is the old list's, filtered.
	dictPaths := []string{
		d2resource.LevelType, d2resource.LevelPreset,
		d2resource.ObjectType, d2resource.ObjectDetails, d2resource.Weapons,
		d2resource.Armor, d2resource.Misc,
		d2resource.Missiles, d2resource.SoundSettings,
		d2resource.MonStats, d2resource.MonStats2, d2resource.MonPreset,
		d2resource.Overlays, d2resource.CharStats, d2resource.Experience,
		d2resource.LevelDetails, d2resource.Inventory, d2resource.Skills,
		d2resource.SkillDesc, d2resource.SoundEnvirons,
	}

	a.Info("Initializing asset manager")

	for _, path := range dictPaths {
		err := a.asset.LoadRecords(path)
		if err != nil {
			return err
		}
	}

	err := a.initAnimationData(d2resource.AnimationData)
	if err != nil {
		return err
	}

	return nil
}

const (
	fmtLoadAnimData = "loading animation data from: %s"
)

func (a *App) initAnimationData(path string) error {
	animDataBytes, err := a.asset.LoadFile(path)
	if err != nil {
		return err
	}

	a.Debugf(fmtLoadAnimData, path)

	animData, err := d2animdata.Load(animDataBytes)
	if err != nil {
		a.Error(err.Error())
	}

	a.Infof("Loaded %d animation data records", animData.GetRecordsCount())

	a.asset.Records.Animation.Data = animData

	return nil
}

func (a *App) loadStrings() error {
	// M5.3: Strigoi's own words, when asked for, INSTEAD of Diablo II's three
	// tables -- none of them is loaded. A refused table is reported and
	// Diablo II's are read as before.
	if a.Options.strings != nil && *a.Options.strings != "" {
		err := a.asset.UseStringTable(*a.Options.strings)
		if err == nil {
			return nil
		}

		a.Errorf("string table refused, reading Diablo II's: %v", err)
	}

	tablePaths := []string{
		d2resource.PatchStringTable,
		d2resource.ExpansionStringTable,
		d2resource.StringTable,
	}

	for _, tablePath := range tablePaths {
		_, err := a.asset.LoadStringTable(tablePath)
		if err != nil {
			return err
		}
	}

	return nil
}
