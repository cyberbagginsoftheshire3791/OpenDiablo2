package main

import (
	"fmt"
	"strings"
)

// ModulePath is this repository's module path, the prefix deadcode wants on
// every -whylive symbol.
const ModulePath = "github.com/OpenDiablo2/OpenDiablo2"

// Bucket is the claim this register makes about a symbol. Every row is a
// decision somebody made, not a fact the tool discovered -- the tool only
// measures reachability, and reachability on its own says nothing about
// whether a symbol is where it belongs.
type Bucket string

const (
	// BucketWire: the shipped game must call this. These rows are the gate's
	// teeth. If one goes harness-only, a system that used to run in the game
	// now runs only when a script drives it, the playtest still passes, and
	// the milestone that claimed it is hollow. That is M4.1 and M4.3b, twice.
	BucketWire Bucket = "wire"

	// BucketObserve: this exists so the harness can read or tune the system,
	// and the shipped game is not expected to call it. Harness-only is the
	// correct answer for these.
	//
	// THIS BUCKET IS THE GATE'S ONLY LOOPHOLE, so it has a rule: a symbol may
	// be observe ONLY if it neither changes world state nor is named in a
	// signed assertion. Reading a value is observe. Writing a DIAL is observe
	// -- tuning is what the harness is for. Adding, removing or releasing
	// anything IN THE WORLD is not, however convenient it is to say so, and
	// goes in defer with a milestone against its name.
	BucketObserve Bucket = "observe"

	// BucketDefer: the game should drive this eventually and does not yet.
	// Milestone names who picks it up. A deferral without a milestone is a
	// leak with better manners, so the register's tests reject one.
	BucketDefer Bucket = "defer"

	// BucketDelete: a seam nobody took. Scheduled for removal.
	BucketDelete Bucket = "delete"
)

// Entry is one row.
type Entry struct {
	// Symbol as deadcode -whylive takes it: pkgpath.Type.Method.
	Symbol string
	Bucket Bucket
	// Expect is the verdict this row claims. The gate fails on any
	// disagreement, in EITHER direction -- a symbol that quietly becomes
	// wired is also a register that has stopped being true.
	Expect Verdict
	// Why must be checkable by someone who cannot read the call graph.
	Why string
	// Milestone is required for defer rows and forbidden on wire rows.
	Milestone string
}

func sym(pkg, name string) string {
	return ModulePath + "/" + pkg + "." + name
}

const (
	pkgWorld  = "d2core/d2world"
	pkgScreen = "d2game/d2gamescreen"
)

// Register is the allowlist. It is hand-maintained on purpose: deadcode's
// default report is blinded by the d2harness registry's reflection-liveness,
// so it cannot enumerate these for us. A symbol absent from this list is not
// checked, and its absence means nothing.
//
// Every Expect below was MEASURED on 28 Aug 2026 at 9:35-10:06 PM CT against
// HEAD 9d9611ba by strigoi-harness-runs\reach-census.ps1, which ran the same
// two deadcode invocations this tool runs, over these same 68 symbols. Mean
// 4.7 s per invocation. The raw output is in reach-census.tsv.
var Register = []Entry{
	// ---------------------------------------------------------------
	// WIRE -- the drive train. Sixteen rows, all measured live.
	//
	// These are also the gate's positive control: if the analysis ever
	// silently stops reaching anything, these go red instead of the whole
	// run passing while measuring nothing.
	// ---------------------------------------------------------------
	{sym(pkgWorld, "Clock.Advance"), BucketWire, VerdictLive,
		"Game.advanceWorld turns the frame delta into world minutes here. If this goes dark, time stops.", ""},
	{sym(pkgWorld, "Light.Advance"), BucketWire, VerdictLive,
		"Burns down a carried source and moves the ambient level with the clock.", ""},
	{sym(pkgWorld, "Light.Level"), BucketWire, VerdictLive,
		"The renderer samples this per tile through the LightSampler seam; it is what makes night dark on screen.", ""},
	{sym(pkgWorld, "Light.SetPlayer"), BucketWire, VerdictLive,
		"Moves the light model's idea of where the player is, every frame.", ""},
	{sym(pkgWorld, "Meters.Advance"), BucketWire, VerdictLive,
		"Drains food and water and accrues fatigue against the stepped clock.", ""},
	{sym(pkgWorld, "Meters.SetBody"), BucketWire, VerdictLive,
		"Hands the meters the player's stats so neglect can spend real health.", ""},
	{sym(pkgWorld, "Pursuit.Advance"), BucketWire, VerdictLive,
		"Re-paths live chases as the quarry moves.", ""},
	{sym(pkgWorld, "Pursuit.Chase"), BucketWire, VerdictLive,
		"Starts a chase. Reached from startChasesForTheAware, in ordinary game code -- this is half of the seam M4.3b was reopened over.", ""},
	{sym(pkgWorld, "Pursuit.Chasing"), BucketWire, VerdictLive,
		"Stops a chase being restarted every tick, which would reset the re-path clock and reproduce M4.3a's 218-solve bug.", ""},
	{sym(pkgWorld, "Notice.Watch"), BucketWire, VerdictLive,
		"The spawn tables call this for every arriving member, so a new wolf can see you.", ""},
	{sym(pkgWorld, "Notice.AwarePairs"), BucketWire, VerdictLive,
		"The other half of the M4.3b seam: it is what startChasesForTheAware reads to decide who comes for you. If this goes harness-only, M4.3b is hollow again and the playtest will not notice.", ""},
	{sym(pkgWorld, "Notice.Advance"), BucketWire, VerdictLive,
		"Re-evaluates sight lines and expires the memory window. Stepped by Spawns.Advance, deliberately not twice.", ""},
	{sym(pkgWorld, "Spawns.Advance"), BucketWire, VerdictLive,
		"Rolls the stage tables on the clock. Without it nothing ever arrives.", ""},
	{sym(pkgWorld, "Spawns.SetTarget"), BucketWire, VerdictLive,
		"Tells the tables who the prey is, every frame.", ""},
	{sym(pkgWorld, "Spawns.Weight"), BucketWire, VerdictLive,
		"Computes a row's weight from band, carrion and light -- the milestone's signed assertion.", ""},
	{sym(pkgWorld, "Spawns.Band"), BucketWire, VerdictLive,
		"The deep-night band the tables key on.", ""},

	// ---------------------------------------------------------------
	// DELETE -- proposed 28 Aug, awaiting Josh's ruling. Six rows.
	// The four Game system accessors have never had a call site in ANY
	// commit (git log -S returns nothing for each), because the harness
	// reaches every one of these systems through the d2harness registry
	// instead.
	// ---------------------------------------------------------------
	{sym(pkgScreen, "Game.WorldClock"), BucketDelete, VerdictDead,
		"Never called, in any commit. The harness reads the clock through d2harness.Lookup(\"clock\").", ""},
	{sym(pkgScreen, "Game.Light"), BucketDelete, VerdictDead,
		"Never called, in any commit. The renderer gets the light model handed to it as a field at CreateGame; the harness uses the registry.", ""},
	{sym(pkgScreen, "Game.Meters"), BucketDelete, VerdictDead,
		"Never called, in any commit. The harness uses the \"meters\" provider.", ""},
	{sym(pkgScreen, "Game.Spawns"), BucketDelete, VerdictDead,
		"Never called, in any commit. The harness uses the \"spawns\" provider.", ""},
	{sym(pkgScreen, "Game.HarnessLocalPlayerID"), BucketDelete, VerdictDead,
		"Never called since the commit that added it. The harness already holds the game client and reads client.PlayerID directly in about twenty places.", ""},
	{sym(pkgWorld, "Clock.Frozen"), BucketDelete, VerdictDead,
		"Zero call sites. The clock's own HarnessState publishes the frozen flag from the field without going through this getter, and SetFrozen is the half that is used.", ""},

	// ---------------------------------------------------------------
	// DEFER -- the game should drive these and does not yet.
	// ---------------------------------------------------------------
	// Meters consumption: RULED by Josh on 28 Aug (decision
	// 3caff9f3-d21e-81f9-b029-f1394aa131a3). In a real build the meters can
	// only drain; eating, drinking and resting are harness verbs. That is
	// correct as signed -- M4.2's DoD put inventory in Phase 6 -- and it is
	// recorded here so the gate reports it as a deliberate exclusion rather
	// than finding it again every burst.
	{sym(pkgWorld, "Meters.Activity"), BucketDefer, VerdictDead,
		"Nothing in the game sets what the body is doing, so the activity is permanently idle and the labour and watch multipliers never run.", "Phase 6 inventory"},
	{sym(pkgWorld, "Meters.SetActivity"), BucketDefer, VerdictDead,
		"The write half of the same dial. HarnessSet assigns the field directly rather than calling this, which is why it reads dead rather than harness-only.", "Phase 6 inventory"},
	{sym(pkgWorld, "Meters.Consume"), BucketDefer, VerdictHarnessOnly,
		"The only way to refill food or water or to rest off fatigue. No eat, drink or sleep verb exists in the game, so the meters are a one-way ratchet in every playable build.", "Phase 6 inventory"},
	{sym(pkgWorld, "Light.Add"), BucketDefer, VerdictHarnessOnly,
		"Places a light source. The game has no verb that lights a fire -- place_source is a harness field.", "Phase 6 inventory / the first interaction verbs"},
	{sym(pkgWorld, "Light.Remove"), BucketDefer, VerdictHarnessOnly,
		"Puts a placed light out. This is the symbol M4.1 was reopened over; remove_source gave it a caller, and that caller is HarnessSet, so it is still harness-only one level down.", "Phase 6 inventory / the first interaction verbs"},
	{sym(pkgWorld, "Meters.Food"), BucketDefer, VerdictDead,
		"A read accessor with only test callers. The meters provider reads the field directly. The HUD is what will want these.", "M4.4 (the HUD milestone)"},
	{sym(pkgWorld, "Meters.Water"), BucketDefer, VerdictDead,
		"As Meters.Food.", "M4.4 (the HUD milestone)"},
	{sym(pkgWorld, "Meters.Fatigue"), BucketDefer, VerdictDead,
		"As Meters.Food.", "M4.4 (the HUD milestone)"},
	{sym(pkgWorld, "Meters.ReactionAvailable"), BucketDefer, VerdictHarnessOnly,
		"R2 section 1's reaction flag. Computed and reported, read by nothing -- M4.2 built it for a resolver that does not exist yet.", "M4.5 (combat v0)"},
	{sym(pkgWorld, "Meters.Shaken"), BucketDefer, VerdictHarnessOnly,
		"R2 section 3's shaken flag. Same shape: built for M4.5 to read.", "M4.5 (combat v0)"},
	{sym(pkgWorld, "Meters.ShakenThreshold"), BucketDefer, VerdictHarnessOnly,
		"The threshold thirst lowers. Reported, unread.", "M4.5 (combat v0)"},
	{sym(pkgWorld, "Meters.Thirsty"), BucketDefer, VerdictHarnessOnly,
		"Feeds ShakenThreshold and nothing else.", "M4.5 (combat v0)"},
	{sym(pkgWorld, "Meters.Dying"), BucketDefer, VerdictHarnessOnly,
		"Neglect can already run the body to zero, but nothing acts on it: there is no death path and no death screen.", "M4.6 (death and the dead)"},
	{sym(pkgWorld, "Spawns.Routing"), BucketDefer, VerdictDead,
		"M4.3b built the rout STATE and signed the rout BEHAVIOUR over to M4.5, so this flag is deliberately read by nobody yet.", "M4.5 (combat v0)"},
	{sym(pkgWorld, "Spawns.Groups"), BucketDefer, VerdictDead,
		"The live group list. A combat resolver needs it; nothing else does yet.", "M4.5 (combat v0)"},
	{sym(pkgWorld, "Spawns.OpenBodies"), BucketDefer, VerdictDead,
		"The carrion count. Settable as a stand-in because the corpse machine that will drive it is not built.", "M4.7 (the corpse machine)"},

	// The three rows below are the spawn stall, found by this register on
	// 28 Aug. Nothing in a shipped build ever despawns a group, unwatches a
	// watcher or releases a chase. Groups are hard-capped at MaxGroups 8
	// (spawns.go:435, dial at :179, not settable), so this is not a leak --
	// it is worse in a quieter way: once eight groups exist, every later roll
	// hits the cap and NOTHING EVER ARRIVES AGAIN for the life of the screen.
	// The milestone named here is a PROPOSAL, not a ruling.
	{sym(pkgWorld, "Spawns.Despawn"), BucketDefer, VerdictHarnessOnly,
		"The only way a group leaves. Its one caller is HarnessSet, so in a shipped build no pack ever departs and the group cap becomes a permanent spawn stall.", "M4.5 (PROPOSED -- awaiting Josh's ruling on the spawn stall)"},
	{sym(pkgWorld, "Notice.Unwatch"), BucketDefer, VerdictHarnessOnly,
		"The only way a watcher stops watching. Reached from Despawn and from Game.Unwatch, both harness-only, so awareness entries are never dropped in a shipped build.", "M4.5 (PROPOSED -- awaiting Josh's ruling on the spawn stall)"},
	{sym(pkgWorld, "Pursuit.Release"), BucketDefer, VerdictHarnessOnly,
		"The only way a chase ends. Same shape: a shipped build starts chases and never ends one.", "M4.5 (PROPOSED -- awaiting Josh's ruling on the spawn stall)"},

	// ---------------------------------------------------------------
	// OBSERVE -- harness surface. Reads and dial writes only; see the
	// BucketObserve comment for the rule that keeps this from becoming the
	// gate's escape hatch.
	// ---------------------------------------------------------------
	{sym(pkgScreen, "Game.Pursuit"), BucketObserve, VerdictHarnessOnly,
		"A typed handle for the harness. Ordinary game code uses the field, and d2app cannot name the type because it does not import d2world.", ""},
	{sym(pkgScreen, "Game.Notice"), BucketObserve, VerdictHarnessOnly,
		"As Game.Pursuit. Notice is the one world system with no provider of its own; its reporting rides on the spawns provider by signature.", ""},
	{sym(pkgScreen, "Game.HarnessMapRenderer"), BucketObserve, VerdictHarnessOnly,
		"The only exported route to the map renderer, used by dump_surface and by world-to-screen conversion in input scripts.", ""},
	{sym(pkgScreen, "Game.HarnessControlsBound"), BucketObserve, VerdictHarnessOnly,
		"The harness's readiness signal: true once the world has run a frame with the player in it.", ""},
	{sym(pkgScreen, "Game.Pursue"), BucketObserve, VerdictHarnessOnly,
		"A handle-to-entity wrapper so a script can start a chase by id. The game starts its own chases through startChasesForTheAware, which is wired.", ""},
	{sym(pkgScreen, "Game.Watch"), BucketObserve, VerdictHarnessOnly,
		"The same wrapper for awareness. The spawn tables call Notice.Watch directly, which is wired.", ""},
	{sym(pkgScreen, "Game.Unwatch"), BucketObserve, VerdictHarnessOnly,
		"The wrapper's other half. Note that the game's own unwatch path is missing entirely -- see Notice.Unwatch above.", ""},
	{sym(pkgWorld, "Clock.Date"), BucketObserve, VerdictHarnessOnly,
		"Reports the Julian civil date for the clock provider.", ""},
	{sym(pkgWorld, "Clock.Weekday"), BucketObserve, VerdictHarnessOnly,
		"Reports the weekday for the clock provider.", ""},
	{sym(pkgWorld, "Clock.TimeOfDay"), BucketObserve, VerdictHarnessOnly,
		"Reports the time of day for the clock provider.", ""},
	{sym(pkgWorld, "Clock.SetFrozen"), BucketObserve, VerdictHarnessOnly,
		"Freezes the clock for a script. The game must never do this, so harness-only is the correct answer and not a deferral.", ""},
	{sym(pkgWorld, "Clock.SetMoon"), BucketObserve, VerdictHarnessOnly,
		"Sets the moon phase for a script. A dial, not a world change.", ""},
	{sym(pkgWorld, "Light.Radius"), BucketObserve, VerdictHarnessOnly,
		"Reports the lit radius around the player.", ""},
	{sym(pkgWorld, "Light.Carried"), BucketObserve, VerdictHarnessOnly,
		"Reports the carried source's state.", ""},
	{sym(pkgWorld, "Pursuit.Count"), BucketObserve, VerdictHarnessOnly,
		"Reports how many chases are live.", ""},
	{sym(pkgWorld, "Pursuit.Solves"), BucketObserve, VerdictHarnessOnly,
		"Reports how many route solves have run. This is the counter that caught M4.3a's 218-solve bug.", ""},
	{sym(pkgWorld, "Notice.Noticed"), BucketObserve, VerdictHarnessOnly,
		"Reports whether one watcher has noticed the player.", ""},
	{sym(pkgWorld, "Notice.Count"), BucketObserve, VerdictHarnessOnly,
		"Reports how many watchers exist.", ""},
	{sym(pkgWorld, "Notice.Checks"), BucketObserve, VerdictHarnessOnly,
		"Reports how many sight evaluations have run.", ""},
	{sym(pkgWorld, "Notice.Notices"), BucketObserve, VerdictHarnessOnly,
		"Reports how many times something has noticed the player.", ""},
	{sym(pkgWorld, "Notice.Aware"), BucketObserve, VerdictHarnessOnly,
		"Reports how many watchers are currently aware.", ""},
	{sym(pkgWorld, "Notice.Dials"), BucketObserve, VerdictHarnessOnly,
		"Reports the notice dials so a script can derive its bounds from them instead of hardcoding a fossil.", ""},
	{sym(pkgWorld, "Notice.Report"), BucketObserve, VerdictHarnessOnly,
		"The per-watcher block -- sees, distance, light at the quarry, verdict -- that makes M4.3b's clause 5 assertable at all.", ""},
	{sym(pkgWorld, "Notice.SetRadius"), BucketObserve, VerdictHarnessOnly,
		"Writes the notice radius dial. Josh ruled 12 tiles deliberate; a script may move it, the game may not.", ""},
	{sym(pkgWorld, "Notice.SetLitLevel"), BucketObserve, VerdictHarnessOnly,
		"Writes the lit-level dial.", ""},
	{sym(pkgWorld, "Spawns.SetMorale"), BucketObserve, VerdictHarnessOnly,
		"Writes a group's morale so a script can drive it to the rout threshold. The behaviour that would move it on its own is M4.5's, and Spawns.Routing carries that deferral.", ""},
	{sym(pkgWorld, "Spawns.SetOpenBodies"), BucketObserve, VerdictHarnessOnly,
		"Writes the carrion stand-in. Spawns.OpenBodies carries the deferral to the corpse machine.", ""},
}

// RegisterMarkdown renders the register as a table, so the register lives in
// one place -- here, next to the gate that enforces it -- and the document is
// generated rather than typed. A count in prose is a count that goes stale.
func RegisterMarkdown() string {
	var b strings.Builder

	counts := map[Bucket]int{}
	for _, e := range Register {
		counts[e.Bucket]++
	}

	fmt.Fprintf(&b, "# Reachability register\n\n")
	fmt.Fprintf(&b, "Generated by `go run ./tools/reachcheck -list`. Do not hand-edit; edit `tools/reachcheck/register.go`.\n\n")
	fmt.Fprintf(&b, "%d symbols: %d wire, %d observe, %d defer, %d delete.\n\n",
		len(Register), counts[BucketWire], counts[BucketObserve], counts[BucketDefer], counts[BucketDelete])

	for _, bucket := range []Bucket{BucketWire, BucketDefer, BucketDelete, BucketObserve} {
		fmt.Fprintf(&b, "## %s (%d)\n\n", bucket, counts[bucket])
		fmt.Fprintf(&b, "| symbol | expects | milestone | why |\n|---|---|---|---|\n")

		for _, e := range Register {
			if e.Bucket != bucket {
				continue
			}

			milestone := e.Milestone
			if milestone == "" {
				milestone = "-"
			}

			fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", shortSymbol(e.Symbol), e.Expect, milestone, e.Why)
		}

		fmt.Fprintln(&b)
	}

	return b.String()
}
