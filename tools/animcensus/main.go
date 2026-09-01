// Command animcensus answers the question the signed M4.5 build note left
// UNKNOWN and its §6.6 said must be measured before the resolver depends on
// it: whether the composite even HAS A1 / GH / DT animations for the stand-in
// monstats codes the signed M4.3b spawn tables use.
//
// The reason it matters is stated in that note: a missing animation for a
// stand-in code would look exactly like a broken resolver. This makes it a
// measurement instead of a debugging session.
//
// IT CHANGES NO GAME BEHAVIOUR. It loads records, builds composites, asks
// each one for a mode, and prints what happened. It touches no dial, no
// world system and no renderer.
//
// ARTICLE V. This reads the MPQs at runtime and writes nothing. No extracted
// data may be committed, and this tool MUST NEVER RUN ON CI -- there are no
// MPQs there. It is a main package, so `go build ./...` compiles it and never
// runs it, which is the same fence tools/mapfirecount sits behind.
//
// Usage, from the repository root:
//
//	go run ./tools/animcensus
//	go run ./tools/animcensus -mpq "C:\Program Files (x86)\Diablo II"
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2animdata"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2loader/asset/types"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2config"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2records"
)

// standIns are the monster codes the signed spawn tables actually use:
// d2core/d2world/spawns.go:196 (dogs), :208 (wolves), :220 (boar) and :231
// (opportunists, which reuses fallen1).
//
// They are HARDCODED HERE, and that is worth saying rather than hiding: there
// is no exported accessor that returns the spawn table, so this list is a
// copy and can go stale. If a row's Code changes, this list must change with
// it. The alternative -- exporting the table so the census could read it --
// would grow the API surface of a signed system to serve a tool, which is a
// worse trade than a comment.
var standIns = []string{
	"fallen1",   // dogs, and opportunists
	"zombie1",   // wolves
	"skeleton1", // boar
}

// probeModes is what each code is asked for, in this order.
//
// MonsterAnimationModeKnockback is deliberately absent: its token is "GH",
// the same as GetHit (monster_animation_mode.go), so probing it would
// re-measure GetHit and report a second answer to one question.
var probeModes = []d2enum.MonsterAnimationMode{
	d2enum.MonsterAnimationModeNeutral, // NU -- positive control; the game plays it constantly
	d2enum.MonsterAnimationModeWalk,    // WL -- positive control; npc.go rotate() sets it
	d2enum.MonsterAnimationModeAttack1, // A1 -- THE RESOLVER NEEDS THIS
	d2enum.MonsterAnimationModeAttack2, // A2
	d2enum.MonsterAnimationModeGetHit,  // GH -- THE RESOLVER NEEDS THIS
	d2enum.MonsterAnimationModeDeath,   // DT -- M4.6 / M4.7 need this
	d2enum.MonsterAnimationModeDead,    // DD -- likewise
	d2enum.MonsterAnimationModeBlock,   // BL -- R2 §3's Brace reaction, if it is ever shown
	d2enum.MonsterAnimationModeRun,     // RN
	d2enum.MonsterAnimationModeCast,    // SC
	d2enum.MonsterAnimationModeSkill1,  // S1 -- the third mode npc.go already uses
}

func main() {
	cfg := d2config.DefaultConfig()

	mpq := flag.String("mpq", cfg.MpqPath, "directory holding the D2 MPQs")
	flag.Parse()

	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		die("asset manager: %v", err)
	}

	fmt.Printf("MPQ path: %s\n", filepath.Clean(*mpq))

	for _, name := range cfg.MpqLoadOrder {
		src := filepath.Join(filepath.Clean(*mpq), name)
		if err := asset.AddSource(src, types.AssetSourceMPQ); err != nil {
			die("MPQ %q not found. Pass -mpq with the directory that holds them.\n  %v", src, err)
		}
	}

	// The three loads a composite needs, and they are not all LoadRecords.
	// d2app/initialization.go loads MonStats and MonStats2 through
	// LoadRecords (:98) but animation data through its own path (:145-161),
	// because animdata is a binary .d2 rather than an excel .txt. A census
	// that forgot the third would report "could not find Animation data" for
	// every mode and read exactly like a monster with no animations at all.
	for _, path := range []string{d2resource.MonStats, d2resource.MonStats2} {
		if err := asset.LoadRecords(path); err != nil {
			die("%s: %v", path, err)
		}
	}

	animDataBytes, err := asset.LoadFile(d2resource.AnimationData)
	if err != nil {
		die("%s: %v", d2resource.AnimationData, err)
	}

	animData, err := d2animdata.Load(animDataBytes)
	if err != nil {
		die("animdata: %v", err)
	}

	asset.Records.Animation.Data = animData

	fmt.Printf("Loaded: %d MonStat records, %d MonStat2 records, %d animation data records\n\n",
		len(asset.Records.Monster.Stats), len(asset.Records.Monster.Stats2),
		animData.GetRecordsCount())

	if len(asset.Records.Monster.Stats) == 0 {
		die("monstats.txt loaded but held no records -- the instrument is broken, this is not a finding")
	}

	if !controls(asset) {
		die("a control failed. No findings are reported from an instrument that cannot tell present from absent.")
	}

	census(asset)
}

// controls runs one case known true and one known false before any output is
// believed -- Constitution Article VI.4(b). An instrument that cannot tell
// those two apart produces no findings, so a failure here aborts the run
// rather than degrading it.
func controls(asset *d2asset.AssetManager) bool {
	fmt.Println("=== CONTROLS (VI.4(b)) — run before any finding is believed ===")

	ok := true

	// POSITIVE: fallen1's Neutral and Walk. The game plays both every night
	// (npc.go rotate()), so anything but OK here means the census is broken.
	rec := asset.Records.Monster.Stats["fallen1"]
	if rec == nil {
		fmt.Println("  POSITIVE  FAILED: no monstats record for fallen1")
		return false
	}

	ex := asset.Records.Monster.Stats2[rec.ExtraDataKey]
	if ex == nil {
		fmt.Printf("  POSITIVE  FAILED: no monstats2 record for fallen1 (MonStatsEx %q)\n", rec.ExtraDataKey)
		return false
	}

	equip := equipmentFor(ex)

	for _, mode := range []d2enum.MonsterAnimationMode{
		d2enum.MonsterAnimationModeNeutral,
		d2enum.MonsterAnimationModeWalk,
	} {
		got, detail := probe(asset, rec.AnimationDirectoryToken, ex.BaseWeaponClass, equip, mode)
		fmt.Printf("  POSITIVE  fallen1 %-2s -> %-5v  (expected OK) %s\n", mode.String(), got, detail)

		if !got {
			ok = false
		}
	}

	// NEGATIVE 1: a token no monster has. Must fail, or a "yes" means nothing.
	got, detail := probe(asset, "ZZZZ", "HTH", nil, d2enum.MonsterAnimationModeNeutral)
	fmt.Printf("  NEGATIVE  bogus token ZZZZ NU -> %-5v  (expected false) %s\n", got, truncate(detail))

	if got {
		ok = false
	}

	// NEGATIVE 2: a real token asked for the Sequence mode, whose token is
	// "xx" and which is not a real animation. Must fail.
	got, detail = probe(asset, rec.AnimationDirectoryToken, ex.BaseWeaponClass, equip,
		d2enum.MonsterAnimationModeSequence)
	fmt.Printf("  NEGATIVE  fallen1 %-2s -> %-5v  (expected false) %s\n",
		d2enum.MonsterAnimationModeSequence.String(), got, truncate(detail))

	if got {
		ok = false
	}

	fmt.Printf("  controls: %s\n\n", map[bool]string{true: "PASS", false: "FAIL"}[ok])

	return ok
}

// census is the measurement proper.
func census(asset *d2asset.AssetManager) {
	fmt.Println("=== THE STAND-IN CODES ===")

	type summary struct {
		code string
		have map[string]bool
	}

	summaries := make([]summary, 0, len(standIns))

	for _, code := range standIns {
		rec := asset.Records.Monster.Stats[code]
		if rec == nil {
			fmt.Printf("\n%s: NO MONSTATS RECORD. The spawn tables name this code every night, so this\n"+
				"  is a broken census rather than a finding about the monster.\n", code)

			continue
		}

		ex := asset.Records.Monster.Stats2[rec.ExtraDataKey]
		if ex == nil {
			fmt.Printf("\n%s: no monstats2 record (MonStatsEx %q) -- cannot build a composite.\n",
				code, rec.ExtraDataKey)

			continue
		}

		fmt.Printf("\n%s\n", code)
		fmt.Printf("  Key=%s  hcIdx(ID)=%d  Code(token)=%q  MonStatsEx=%q  BaseWeaponClass=%q\n",
			rec.Key, rec.ID, rec.AnimationDirectoryToken, rec.ExtraDataKey, ex.BaseWeaponClass)
		fmt.Printf("  HIT POINTS (normal): MinHPNormal=%d  MaxHPNormal=%d   [what the NPC body will read]\n",
			rec.MinHPNormal, rec.MaxHPNormal)
		fmt.Printf("  SpeedBase=%d  ThreatLevel=%d\n", rec.SpeedBase, rec.ThreatLevel)

		equip := equipmentFor(ex)
		have := make(map[string]bool, len(probeModes))

		for _, mode := range probeModes {
			got, detail := probe(asset, rec.AnimationDirectoryToken, ex.BaseWeaponClass, equip, mode)
			have[mode.String()] = got

			verdict := "no "
			if got {
				verdict = "YES"
			}

			fmt.Printf("    %-2s  %s  %s\n", mode.String(), verdict, truncate(detail))
		}

		summaries = append(summaries, summary{code: code, have: have})
	}

	fmt.Println("\n=== SUMMARY — what M4.5's resolver can and cannot show ===")

	for _, s := range summaries {
		fmt.Printf("  %-10s A1=%-3v GH=%-3v DT=%-3v DD=%-3v BL=%-3v\n",
			s.code, s.have["A1"], s.have["GH"], s.have["DT"], s.have["DD"], s.have["BL"])
	}

	fmt.Println(`
NOT MEASURED, named rather than guessed (VI.4(d)):
  - whether a mode that LOADS also looks right on screen; this reads the COF
    and the animation data, it does not render a frame.
  - any weapon class but the record's own BaseWeaponClass.
  - any monster but the three the spawn tables name.
  - whether MinHPNormal/MaxHPNormal are the numbers a full D2 damage model
    would use; MonsterLevelRecord scaling is decoded and deliberately unread.`)
}

// probe asks a FRESH composite for one mode and reports whether it loaded.
//
// Fresh, and this is the whole reason the function exists: Composite.SetMode
// short-circuits when the requested mode and weapon class already match the
// current ones (d2core/d2asset/composite.go:115-117), so asking a composite
// for the mode it is already in returns nil having measured nothing. A
// composite straight from LoadComposite has a nil mode, which no short-circuit
// can match. LoadComposite does no I/O of its own -- it builds a struct -- so
// one composite per probe costs a few allocations and buys an unambiguous
// answer.
//
// What a failure means: createMode checks the COF exists and then that
// animation data exists for the key (composite.go:257-273), so "no" here is
// "this monster has no such animation", and the detail string says which of
// the two was missing.
func probe(asset *d2asset.AssetManager, token, weaponClass string,
	equipment *[d2enum.CompositeTypeMax]string, mode d2enum.MonsterAnimationMode) (bool, string) {
	c, err := asset.LoadComposite(d2enum.ObjectTypeCharacter, token, d2resource.PaletteUnits)
	if err != nil {
		return false, "LoadComposite: " + err.Error()
	}

	// Equip before SetMode, not after as the factory does: Composite.Equip
	// stores the equipment and returns early while the mode is nil, so this
	// is the same state the factory reaches, one step sooner.
	if equipment != nil {
		if err := c.Equip(equipment); err != nil {
			return false, "Equip: " + err.Error()
		}
	}

	if err := c.SetMode(mode, weaponClass); err != nil {
		return false, err.Error()
	}

	return true, ""
}

// equipmentFor mirrors d2mapentity's factory (factory.go:226-228) except that
// it takes the FIRST option rather than a random one, because a census must
// give the same answer twice.
func equipmentFor(ex *d2records.MonStat2Record) *[d2enum.CompositeTypeMax]string {
	var equipment [d2enum.CompositeTypeMax]string

	for compType, opts := range ex.EquipmentOptions {
		if len(opts) != 0 {
			equipment[compType] = opts[0]
		}
	}

	return &equipment
}

func truncate(s string) string {
	const max = 96
	if len(s) <= max {
		return s
	}

	return s[:max] + "..."
}

func die(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "animcensus: "+format+"\n", args...)
	os.Exit(1)
}
