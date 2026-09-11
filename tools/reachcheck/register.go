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
	// pkgEntity joined at M4.5 step 3, when the NPC gained its first two
	// exported members that combat cares about. Before that, nothing in
	// d2mapentity was worth a claim: the entity was a sprite on a path.
	pkgEntity = "d2core/d2map/d2mapentity"
)

// Register is the allowlist. It is hand-maintained on purpose: deadcode's
// default report is blinded by the d2harness registry's reflection-liveness,
// so it cannot enumerate these for us. A symbol absent from this list is not
// checked, and its absence means nothing.
//
// Every Expect below was MEASURED on 28 Aug 2026 at 9:35-10:06 PM CT against
// HEAD 9d9611ba by strigoi-harness-runs\reach-census.ps1, which ran the same
// two deadcode invocations this tool runs, over 68 symbols -- these, plus the
// six deleted the same evening. Mean 4.7 s per invocation. The raw output is
// in reach-census.tsv, and the gate re-measures all of it on every run, so
// nothing here has to be taken on trust.
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
	{sym(pkgWorld, "Spawns.Despawn"), BucketWire, VerdictLive,
		"The only way a group leaves. Wired 29 Aug by clearAtDaybreak, which sends home every pack that never noticed you once the sun is up. Before that its one caller was HarnessSet, and with the group cap at 8 that made a permanent spawn stall: the eighth pack was the last one the game would ever produce.", ""},
	// MOVED observe -> wire ON 1 SEP 2026, AND THE MOVE IS THE FINDING.
	//
	// The gate went red on this row, and it was RIGHT: Spawns.aware reads
	// Noticed at spawns.go:468 to decide which packs go home at daybreak, so
	// a shipped build has called it since 68605cb2 on 28 Aug -- git blame
	// names that commit for the line, and the burst that found this had not
	// touched spawns.go at all.
	//
	// IT WENT UNNOTICED FOR THREE DAYS BECAUSE THE FULL REGISTER WAS NOT
	// RE-RUN. The spawn-stall burst moved Spawns.Despawn and Notice.Unwatch
	// by hand and checked those; M4.5 steps 1 and 2 ran `-only Combat` and
	// `-only Meters.Shaken`. Every one of those passed. A gate that is only
	// ever run on the symbols you expect to have changed measures your
	// expectations, which is the same failure as the grep census A2 replaced
	// -- an instrument and a hypothesis sharing an assumption cannot
	// disconfirm it. Run the whole register before the commit that closes a
	// milestone step.
	{sym(pkgWorld, "Notice.Noticed"), BucketWire, VerdictLive,
		"Whether one watcher has noticed the player. The spawn tables read it every daybreak: a pack that has noticed you is NOT sent home, which is the whole distinction the stall fix turned on. Wire, so the gate goes red if that read is ever removed.", ""},
	{sym(pkgWorld, "Notice.Unwatch"), BucketWire, VerdictLive,
		"The only way a watcher stops watching. Reached from Despawn, so it went live with it. Before 29 Aug nothing in a shipped build ever stopped watching anything.", ""},
	{sym(pkgWorld, "Combat.Advance"), BucketWire, VerdictLive,
		"Opens an encounter when something aware of the player is also in reach, and spends world time on rounds. Stepped from advanceWorld after startChasesForTheAware, so a thing that noticed you this tick and is already beside you fights this tick.", ""},

	// M4.5 STEP 4 -- THE RESOLVER. TEN ROWS MOVED TO WIRE AND TWO ARE NEW,
	// and this is the largest single move this register has made.
	//
	// Every one of them had sat in defer with "M4.5 (the resolver)" against
	// its name, which is what a deferral is FOR: the row was written when
	// the symbol was built, it named who would pick it up, and the gate
	// went red the moment that happened. Nine were harness-only or dead in
	// every shipped build one commit ago -- the meters two derived flags
	// were computed for a reader that did not exist, a monster body could
	// be read only by a script, and no monster in this codebase had ever
	// been told to swing.
	//
	// THREE OF THEM ARE LIVE TRANSITIVELY, and that is worth stating
	// because it is not visible at any call site: deadcode -whylive follows
	// calls, so Meters.ShakenThreshold and Meters.Thirsty go live through
	// Shaken(), and NPC.MonStat through Game.BodyOf. A first draft of the
	// step-4 brief filed all three as observe, and the gate would have gone
	// red on the closing run.
	{sym(pkgWorld, "Meters.ReactionAvailable"), BucketWire, VerdictLive,
		"R2 section 1's reaction flag. The resolver reads it to decide whether a graze on the player buys a Riposte -- clause 5 of M4.5's DoD, read from the meters rather than recomputed. Wire, so the gate goes red if the resolver ever stops asking.", ""},
	{sym(pkgWorld, "Meters.Shaken"), BucketWire, VerdictLive,
		"R2 section 3's shaken condition. The resolver reads it for the accuracy penalty on the player's own blows, and to withhold his Reaction.", ""},
	{sym(pkgWorld, "Meters.ShakenThreshold"), BucketWire, VerdictLive,
		"The threshold thirst lowers. Live TRANSITIVELY: Shaken() calls it, so it went live the moment the resolver called Shaken(). Nothing calls it directly and it is wire anyway, because a call graph is what reachability means.", ""},
	{sym(pkgWorld, "Meters.Thirsty"), BucketWire, VerdictLive,
		"Feeds ShakenThreshold, which feeds Shaken, which the resolver reads. Live for the same transitive reason as the row above.", ""},
	{sym(pkgWorld, "Meters.Activity"), BucketWire, VerdictLive,
		"What the body is doing. The resolver reads it ONCE at the start of a fight, through the Fitness interface, to decide D8 section 9's caught-head-down branch -- a player caught foraging or labouring loses round one's initiative and his Reaction with it.", ""},
	{sym(pkgWorld, "Meters.SetActivity"), BucketWire, VerdictLive,
		"The write half. The game screen sets labour while a fight runs and puts back what the body was doing when it ends, which is S1 section 5's signed food drain -- faster when digging, fighting, carrying -- finally getting a consumer.", ""},
	{sym(pkgWorld, "Combat.Fighting"), BucketWire, VerdictLive,
		"Whether an encounter is live, asked in Go rather than read out of harness state. advanceWorld reads it every tick to apply and take back the labour activity. This is the row that stops Pursuit's arrived mistake happening twice.", ""},
	{sym(pkgScreen, "Game.BodyOf"), BucketWire, VerdictLive,
		"How the resolver reaches a body -- the PLAYER'S included, since step 4, which is what makes losing possible. Called from Combat.Advance, in Go, on every blow. It was harness-only for one milestone because only HarnessState read it.", ""},
	{sym(pkgEntity, "NPC.MonStat"), BucketWire, VerdictLive,
		"The monstats record an NPC was built from. Reached from Game.BodyOf's on-demand adoption path, so it inherits that row's verdict exactly, as its own comment has said since step 3.", ""},
	{sym(pkgEntity, "NPC.StartAction"), BucketWire, VerdictLive,
		"Plays one animation and HOLDS it, then returns the monster to Neutral -- or, for a death, to a Dead that is held for the rest of the run. Called from Game.Animate on every swing, every blow taken and every death. Before it, nothing could make a monster's sprite survive the next tick.", ""},
	{sym(pkgEntity, "NPC.SetAnimationMode"), BucketWire, VerdictLive,
		"The first exported way to tell a monster to play a mode. Its only caller is NPC.StartAction, which is the honest reading: the row stays because the symbol stays, and it is wire because a real build now reaches it. tools/animcensus measured on 31 Aug that A1, GH, DT and DD all exist for the three codes the spawn tables use.", ""},
	{sym(pkgWorld, "Spawns.ProfileOf"), BucketWire, VerdictLive,
		"What one enemy fights as: its PACK (not its row -- two dog packs are two packs), its authored Speed and its bite. The resolver calls it through the Profiles interface to build D8's order and to draw damage. It is the seam that put speed and damage on the spawn row instead of reading them out of the D2 record. Since step 5 it also carries the pack's STARTING COUNT, which the rout decrement and quick-resolve's advantage are both measured against.", ""},

	// M4.5 STEP 5 -- ROUT, QUICK-RESOLVE AND WITHDRAWAL. Three rows arrive
	// wired and two move up from defer, and the pair below is the trigger
	// M4.3b's signature left belonging to neither milestone: it built the
	// morale STATE and signed the rout BEHAVIOUR over to M4.5, and nothing
	// owned the thing that MOVES the number.
	{sym(pkgWorld, "Spawns.Hurt"), BucketWire, VerdictLive,
		"Takes morale off a group -- the DECREMENT SetMorale deliberately was not. Called from the resolver, in Go, every time an enemy reaches zero, through the Morale interface. It writes the field directly rather than calling SetMorale, so that row's harness-only claim stays true; deadcode is transitive and routing it through would have flipped it.", ""},
	{sym(pkgWorld, "Spawns.Morale"), BucketWire, VerdictLive,
		"One group's nerve, and whether the group is known at all. The second return is what quick-resolve's mundane test turns on: an enemy the tables never placed is not an enemy with zero morale.", ""},
	{sym(pkgWorld, "Spawns.Routing"), BucketWire, VerdictLive,
		"The rout THRESHOLD, read rather than recomputed. M4.3b built this flag and signed its behaviour over to M4.5; step 5 is where something finally reads it, from routeIfBroken, after a death has hurt the pack. Testing morale <= 25 on the combat side instead would have been a second home for a runtime-settable dial.", ""},
	{sym(pkgWorld, "Pursuit.Release"), BucketWire, VerdictLive,
		"The only way a chase ends, and step 5 is the step that ends one. Called from the resolver when an enemy dies, when a pack breaks, and when quick-resolve finishes a fight. IT IS HALF OF A PAIR: startChasesForTheAware runs every frame with no liveness filter, so a release on its own is undone on the very next frame, and the same death also calls Notice.Unwatch.", ""},

	// M4.4a -- THE EYES. Five Clock reads for the HUD's always-visible clock
	// strip, which is d2player's first d2core/d2world import (threaded in at
	// construction, NewGameControls -> NewHUD). Clock.Date and Clock.Weekday
	// moved up from observe; MinuteOfDay, HoursToDusk and Today are new. The
	// strip changes the state digest by construction (the "ui" provider gained
	// clock_strip_* fields), and only the full reach-gate catches these -- the
	// register's unit test checks shape, not reachability.
	//
	// Clock.TimeOfDay did NOT move; it stays observe below. The strip shows
	// time-to-SUNSET, not the HH:MM clock time (S1 section 3.4, "nothing
	// else"), so the shipped game never calls it. The 11 Sep ruling expected
	// all three clock rows to wire, but wiring TimeOfDay would need the strip
	// to draw the clock time (scope creep against a signed fence) or a
	// manufactured caller (the exact hollow-class shape this gate exists to
	// catch -- see the Spawns.Groups note). The honest move is two rows moved
	// and three added, not three moved; the code, and reach-gate, outrank the
	// ruling's incidental count.
	{sym(pkgWorld, "Clock.Date"), BucketWire, VerdictLive,
		"The clock strip draws the Julian civil date. Wire since M4.4a; also reached transitively through Clock.Today. Was observe (harness-only) until the HUD read it.", ""},
	{sym(pkgWorld, "Clock.Weekday"), BucketWire, VerdictLive,
		"The clock strip draws the weekday beside the date. Wire since M4.4a; was observe until the HUD read it.", ""},
	{sym(pkgWorld, "Clock.MinuteOfDay"), BucketWire, VerdictLive,
		"The HUD refreshes the strip when int(MinuteOfDay) changes -- once a world minute, not every frame -- and Clock.HoursToDusk derives from it. Wire since M4.4a.", ""},
	{sym(pkgWorld, "Clock.HoursToDusk"), BucketWire, VerdictLive,
		"The strip's time-to-sunset readout: world hours to the clock's DuskStart (19:45). Built for M4.4a; the HUD is its only caller.", ""},
	{sym(pkgWorld, "Clock.Today"), BucketWire, VerdictLive,
		"Looks up today's generated day-table row for the strip's feast/fast name and moon-phase text. Built for M4.4a; the HUD is its only caller.", ""},

	// ---------------------------------------------------------------
	// DELETE -- empty, and that is the bucket working rather than an
	// oversight.
	//
	// Six symbols sat here on 28 Aug 2026, proposed for deletion because
	// `git log -S` found that not one of them had ever had a call site in
	// any commit: Game.WorldClock, Game.Light, Game.Meters, Game.Spawns,
	// Game.HarnessLocalPlayerID and Clock.Frozen. Each was a second door
	// onto a system the harness already reached through the d2harness
	// registry. Josh ruled delete-all-six and they went in the same burst.
	//
	// A row leaves the register when its symbol leaves the program,
	// because a register entry for a symbol that no longer exists measures
	// nothing and reports `missing` forever. The record of the deletion is
	// the commit, this comment, and the note at each old site.
	// ---------------------------------------------------------------

	// ---------------------------------------------------------------
	// DEFER -- the game should drive these and does not yet.
	// ---------------------------------------------------------------
	// Meters consumption: RULED by Josh on 28 Aug (decision
	// 3caff9f3-d21e-81f9-b029-f1394aa131a3). In a real build the meters can
	// only drain; eating, drinking and resting are harness verbs. That is
	// correct as signed -- M4.2's DoD put inventory in Phase 6 -- and it is
	// recorded here so the gate reports it as a deliberate exclusion rather
	// than finding it again every burst.
	{sym(pkgWorld, "Meters.Consume"), BucketDefer, VerdictHarnessOnly,
		"The only way to refill food or water or to rest off fatigue. No eat, drink or sleep verb exists in the game, so the meters are a one-way ratchet in every playable build.", "Phase 6 inventory"},
	{sym(pkgWorld, "Light.Add"), BucketDefer, VerdictHarnessOnly,
		"Places a light source. The game has no verb that lights a fire -- place_source is a harness field.", "Phase 6 inventory / the first interaction verbs"},
	{sym(pkgWorld, "Light.Remove"), BucketDefer, VerdictHarnessOnly,
		"Puts a placed light out. This is the symbol M4.1 was reopened over; remove_source gave it a caller, and that caller is HarnessSet, so it is still harness-only one level down.", "Phase 6 inventory / the first interaction verbs"},
	{sym(pkgWorld, "Meters.Food"), BucketDefer, VerdictDead,
		"A read accessor with only test callers. The meters provider reads the field directly. HELD at M4.4a (the eyes): the bar waits until a drink verb or a water-dial change makes it carry information -- water empties at 18:09 in every run today -- and the unit of every bar is now the squad, so it is redesigned in M4.4c, not drawn beside the clock strip.", "M4.4c (the meters HUD)"},
	{sym(pkgWorld, "Meters.Water"), BucketDefer, VerdictDead,
		"As Meters.Food.", "M4.4c (the meters HUD)"},
	{sym(pkgWorld, "Meters.Fatigue"), BucketDefer, VerdictDead,
		"As Meters.Food.", "M4.4c (the meters HUD)"},
	// M4.5 step 1 wired the ENCOUNTER, not the resolver, and the two rows
	// below are the honest consequence. Combat reads the meters' flags only
	// inside HarnessState, which the registry dispatches and only harness-
	// tagged code consumes -- so the flags are still harness-only, one level
	// down, exactly as Light.Remove was after M4.1's remove_source. They flip
	// to wire when the RESOLVER reads them, and not a commit before: marking
	// them wired now would be the lie this register exists to catch.
	{sym(pkgWorld, "Combat.Round"), BucketDefer, VerdictDead,
		"The current round. The resolver spends world time on rounds but reads it off the encounter directly; the HUD is what will want the accessor.", "M4.4 (the HUD milestone)"},
	{sym(pkgWorld, "Combat.Order"), BucketDefer, VerdictDead,
		"The activation sequence -- D8 section 9's since step 4, and no longer provisional. Reported and asserted by playtests; still read by nothing in Go, and the HUD is what will.", "M4.4 (the HUD milestone)"},
	{sym(pkgWorld, "Meters.Dying"), BucketDefer, VerdictHarnessOnly,
		"Neglect can already run the body to zero, but nothing acts on it: there is no death path and no death screen.", "M4.6 (death and the dead)"},
	// SPAWNS.GROUPS DID NOT FLIP AT STEP 5, AND THE ROW SAYS WHY RATHER THAN
	// BEING QUIETLY MOVED. Josh signed ask 7 as "wire it rather than
	// re-point it", and the build could not honour that without inventing a
	// call site: what rout needs is the PACK's starting size, not how many
	// packs are live, and that arrives through Spawns.ProfileOf -- which is
	// precisely the route this row's old text called the step-4 workaround.
	// Manufacturing a caller to turn the row green is the exact failure this
	// register exists to catch, so the honest move is to leave it deferred,
	// say so out loud, and put the amendment of M4.5 section 3.9's all-seven
	// clause back to Josh.
	{sym(pkgWorld, "Spawns.Groups"), BucketDefer, VerdictDead,
		"The live group COUNT. Nothing in the game wants it: the resolver reaches a pack through Spawns.ProfileOf, which answers per member and now carries the pack's starting size, and rout is measured against that. It is reported by the provider, so a script can see it; no Go caller needs it.", "unclaimed -- see the note above; M4.5 section 3.9's all-seven clause needs amending or a real caller"},
	{sym(pkgWorld, "Spawns.OpenBodies"), BucketDefer, VerdictDead,
		"The carrion count. Settable as a stand-in because the corpse machine that will drive it is not built.", "M4.7 (the corpse machine)"},

	// M4.5 STEP 3 -- THE NPC BODY. Three rows, and every verdict below was
	// measured on 31 Aug 2026 at 11:47 PM CT against HEAD 502e4cef plus this
	// burst's working tree, by strigoi-harness-runs\step3-reach.ps1, with
	// Combat.Advance (live) and Meters.Shaken (harness-only) run in the same
	// batch as controls so the instrument's two known answers were confirmed
	// before these three were believed.
	//
	// None of them is wire, and that is the honest reading rather than a
	// disappointing one. The game DOES adopt a body for every monster the
	// spawn tables send -- gameSpawner.Spawn calls Game.adoptNPCBody, which
	// Spawns.Advance drives from advanceWorld -- but adoptNPCBody is
	// unexported and this register covers exported symbols only, so the row
	// that would say so cannot be written. What CAN be named is the reading
	// side, and the reading side is still harness-only or dead, because the
	// only thing that reads a monster's health today is the combat
	// provider's HarnessState. That flips at step 4 when the resolver reads
	// it in Go, and not a commit before -- the same reasoning as the two
	// Meters rows above, in a third costume.
	{sym(pkgScreen, "Game.BodiesKnown"), BucketObserve, VerdictHarnessOnly,
		"How many monsters currently have a body. Observe rather than defer, and it is a READ: it exists so a playtest can watch the game adopt bodies when the spawn tables fire, which is the only way to tell eager adoption from BodyOf's on-demand fallback. Without it, deleting the eager path would break no test -- the M4.1 and M4.3b shape.", ""},

	// Spawns.Despawn, Notice.Unwatch AND Pursuit.Release all used to sit here,
	// deferred. All three are wired now -- see the wire block above -- and
	// the moves are the register doing exactly what it is for: it named the
	// holes, Josh ruled them bursts, and the gate stayed red until they were
	// filled. Pursuit.Release was the last of the three and it waited for
	// exactly what its row said it was waiting for: something that ends a
	// fight.

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
	// Clock.Date and Clock.Weekday moved to the M4.4a WIRE block: the HUD's
	// clock strip draws both since M4.4a. TimeOfDay stayed here -- the strip
	// shows time-to-sunset, not the HH:MM clock time, so only the clock
	// provider's HarnessState calls it. See the M4.4a note in the wire block.
	{sym(pkgWorld, "Clock.TimeOfDay"), BucketObserve, VerdictHarnessOnly,
		"Reports the time of day (HH:MM) for the clock provider. Harness-only: the HUD shows time-to-sunset, not the clock time (S1 section 3.4).", ""},
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
		"Writes a group's morale so a script can drive it to the rout threshold. STILL HARNESS-ONLY AFTER STEP 5, and deliberately: the game hurts a pack through Spawns.Hurt, which subtracts and floors and writes the field itself. Routing this through here would have been the DRY refactor and would have flipped this row live transitively, which is the shape three earlier rows were caught by.", ""},
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
