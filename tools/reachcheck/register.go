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
	pkgWorld    = "d2core/d2world"
	pkgScreen   = "d2game/d2gamescreen"
	pkgBestiary = "d2core/d2bestiary"
	pkgItems    = "d2core/d2items"
	pkgProgress = "d2core/d2progress"
	// pkgEntity joined at M4.5 step 3, when the NPC gained its first two
	// exported members that combat cares about. Before that, nothing in
	// d2mapentity was worth a claim: the entity was a sprite on a path.
	pkgEntity = "d2core/d2map/d2mapentity"
	// pkgPlayer joined at M4.4c-2a, with the first keys that reach the world.
	// Before that, d2player held HUD drawing and nothing a register row could
	// usefully claim; F, L and E are the first three handlers whose going dark
	// would mean a person cannot play the game at all.
	pkgPlayer = "d2game/d2player"
	// pkgAsset and pkgMapGen joined on 23-24 Sep 2026 (M5.3's census): the
	// tables that load only when a generated world is built, and the world a
	// joining client builds from its host's.
	pkgAsset  = "d2core/d2asset"
	pkgMapGen = "d2core/d2map/d2mapgen"
	// pkgJournal joined at J1 (24 Sep 2026): his diary, which replaces the
	// quest log.
	pkgJournal = "d2core/d2journal"
	// pkgHero and pkgMapEngine joined at M5.3's tables burst (26 Sep 2026):
	// the hero's body read from his manifest instead of charstats.txt, and an
	// authored map's sound environment and name instead of levels.txt.
	pkgHero      = "d2core/d2hero"
	pkgMapEngine = "d2core/d2map/d2mapengine"
	// pkgSave joined at M4.6 B3 (28 Sep 2026): the world file.
	pkgSave = "d2core/d2save"
	// pkgClient, pkgServer and pkgAudio joined at the tables burst's review
	// (27 Sep 2026): a host refusing the other game's join and cast, the
	// refused client going back to the menu, and the village's sound read.
	pkgClient = "d2networking/d2client"
	pkgServer = "d2networking/d2server"
	pkgAudio  = "d2core/d2audio"
	// pkgApp joined at the polish burst (27 Sep 2026): the one place a game
	// is started, and what it does when the game screen cannot be built.
	pkgApp = "d2app"
	// pkgPalette joined at the editor burst (27 Sep 2026): the model behind the
	// map editor's asset palette. Every row of it was a DEFERRAL then, and not
	// even a measured one (PalettePending, deleted 28 Sep 2026), because the
	// editor screen that drives it did not exist and NOTHING in the shipped
	// program imported the package -- so every symbol measured "not found".
	// The World Editor screen landed on 28 Sep 2026 and imports it, so the rows
	// below are now measured like any other.
	pkgPalette = "d2core/d2mappalette"
	// pkgInput, pkgInputEbiten and pkgMapRenderer joined at the editor-zoom
	// burst (27 Sep 2026): the mouse wheel's read, which the engine had no path
	// for at all, and the viewport's scale.
	pkgInput       = "d2core/d2input"
	pkgInputEbiten = "d2core/d2input/ebiten"
	pkgMapRenderer = "d2core/d2map/d2maprenderer"
	// pkgMapEdit joined at the World Editor screen (28 Sep 2026). It is the
	// importer the block below spent a burst waiting for: the authoring
	// document was written first and nothing in the shipped program referred to
	// it, so the gate could not see the package at all.
	pkgMapEdit = "d2core/d2mapedit"
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
	// WIRE -- the drive train, all measured live.
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

	// M4.4c-1 MOVED these three from defer/dead: the selected-squad sheet reads
	// them (HUD.renderSquadSheet -> Squads.SheetCards -> Meters.Food/Water/
	// Fatigue). A drink verb is still Phase 6, but the SHEET is what makes the
	// number carry information now, and the unit is the squad.
	{sym(pkgWorld, "Meters.Food"), BucketWire, VerdictLive,
		"Read by the selected-squad sheet through Squads.SheetCards. Moved to wire at M4.4c-1.", ""},
	{sym(pkgWorld, "Meters.Water"), BucketWire, VerdictLive,
		"As Meters.Food -- read by the squad sheet.", ""},
	{sym(pkgWorld, "Meters.Fatigue"), BucketWire, VerdictLive,
		"As Meters.Food -- read by the squad sheet.", ""},
	{sym(pkgWorld, "Meters.Dying"), BucketWire, VerdictLive,
		"The overhead DYING cue reads it (Squads.cues -> Meters.Dying), drawn by the bar widget. Moved from defer/M4.6 at M4.4c-1: c-1 gives Dying a CUE, not a consequence -- the death path is still M4.6.", ""},

	// THE SQUADS OWNER (M4.4c-1): the player commands squads, and this type is
	// the "meters" provider now. These rows are the drive train of that -- the
	// game screen advances and binds it, combat looks up fitness through it, and
	// the HUD draws its bars and sheet and takes selection input.
	{sym(pkgWorld, "Squads.Advance"), BucketWire, VerdictLive,
		"Game.advanceWorld drains every squad's meters here, so a second squad drains independently of the player's.", ""},
	{sym(pkgWorld, "Squads.BindPlayer"), BucketWire, VerdictLive,
		"The metersBodied latch binds s:1 to the player's body and entity id on the first frame that has a player.", ""},
	{sym(pkgWorld, "Squads.Close"), BucketWire, VerdictLive,
		"Game.OnUnload unregisters the provider with the screen.", ""},
	{sym(pkgWorld, "Squads.PlayerMeters"), BucketWire, VerdictLive,
		"The game screen holds s:1's meters for the fighting-activity edge; construction reads it here.", ""},
	{sym(pkgWorld, "Squads.FitnessOf"), BucketWire, VerdictLive,
		"Combat looks up the player squad's fitness by entity id at five read sites (clause 4), through the FitnessSource interface this satisfies.", ""},
	{sym(pkgWorld, "Squads.Bars"), BucketWire, VerdictLive,
		"Game.OverheadBars reads it to draw a bar over every player squad model; the HUD projects and paints it every frame.", ""},
	{sym(pkgWorld, "Squads.SheetCards"), BucketWire, VerdictLive,
		"HUD.renderSquadSheet reads it to draw the selected-squad sheet's cards.", ""},
	{sym(pkgWorld, "Squads.SetSelected"), BucketWire, VerdictLive,
		"A select-click sets the selected squad (GameControls.selectSquad, from OnMouseButtonDown).", ""},
	{sym(pkgWorld, "Squads.Cycle"), BucketWire, VerdictLive,
		"The cycle key selects the next squad (GameControls.cycleSquad, from OnKeyDown).", ""},
	{sym(pkgWorld, "Squads.ModelEntities"), BucketWire, VerdictLive,
		"The selection hit test iterates squad model entities (GameControls.squadAtScreen).", ""},
	{sym(pkgWorld, "Squads.SquadOf"), BucketWire, VerdictLive,
		"Game.OverheadBars asks which squad owns a model entity, so a player model is barred as its squad and not as an enemy.", ""},
	{sym(pkgScreen, "Game.OverheadBars"), BucketWire, VerdictLive,
		"The bar source the HUD asks each frame: assembles a bar per player squad model and per beast/man body, never adopting a body. The HUD's refreshOverheadBars calls it.", ""},
	{sym(pkgScreen, "Game.ShowsBar"), BucketWire, VerdictLive,
		"R2 section 1's fence in code (ruled ask 4/7): beasts and men get a bar, the dead never. Game.OverheadBars calls it per candidate body. Keyed on the SPAWN ROW through Spawns.ProfileOf, not on the monstats group: N1 section 5's codes are stand-in sprites (the wolves wear zombie1), so the group answers what the art is, not what the thing is -- the c-1 review, 15 Sep 2026.", ""},
	{sym(pkgWorld, "Combat.Fighting"), BucketWire, VerdictLive,
		"Whether an encounter is live, asked in Go rather than read out of harness state. advanceWorld reads it every tick to apply and take back the labour activity. This is the row that stops Pursuit's arrived mistake happening twice.", ""},
	{sym(pkgScreen, "Game.BodyOf"), BucketWire, VerdictLive,
		"How the resolver reaches a body -- the PLAYER'S included, since step 4, which is what makes losing possible. Called from Combat.Advance, in Go, on every blow. It was harness-only for one milestone because only HarnessState read it.", ""},
	// ---------------------------------------------------------------
	// M4.4c-2a -- THE HANDS. A round can stop and wait for a person, and
	// these rows are the only reason that is true outside a script.
	//
	// The five MOVED rows below (Combat.Round, Light.Add, Light.Remove,
	// Light.Carried, Clock.TimeOfDay) were all measured harness-only or dead
	// on 18 Sep 2026 at dc7149d3 and are live now because a KEY reaches them,
	// not because a caller was manufactured. Light.Remove is the symbol M4.1
	// was reopened over; it has a shipped caller for the first time.
	//
	// Combat.Order did NOT move and c-2's brief expected it to -- see the
	// defer block below for why, and state.md for the finding.
	// ---------------------------------------------------------------
	{sym(pkgWorld, "Combat.Commit"), BucketWire, VerdictLive,
		"The player's slot, resolved by a person. GameControls.combatStrike/combatTorch/combatEndTurn all call it from OnKeyDown in a shipped build, and the harness's commit settable field calls the SAME method -- two paths, one implementation, which is what keeps commits_by_input honest.", ""},
	{sym(pkgWorld, "Combat.Awaiting"), BucketWire, VerdictLive,
		"Whether a turn is open and waiting for a person. The torch key refuses to spend when it is false, and T1's tactical Move and click-to-strike refuse out of turn on it. Since T1 the WORLD gate reads WorldHeld instead, which is Awaiting widened to the whole of a paced fight.", ""},
	{sym(pkgWorld, "Combat.WorldHeld"), BucketWire, VerdictLive,
		"T1: the game screen's world gate. A paced fight holds the world for its whole length and pays for each round with TakeRoundMinutes. If this goes dark the world runs in real time through a fight again and rounds stop costing exactly one world minute.", ""},
	{sym(pkgWorld, "Combat.SetKits"), BucketWire, VerdictLive,
		"T2: CreateGame attaches the screen as the resolver's Kits source, so his weapon sets his bite and his mail and shield answer a blow. Nil is legal (no gear), so losing this line is silent -- the action rows' weapon/absorbed/blocked fields are the instrument.", ""},
	{sym(pkgScreen, "Game.KitOf"), BucketWire, VerdictLive,
		"T2: the d2world.Kits seam -- the local hero's kit, read by resolveBlow and riposteAllowed on every blow.", ""},
	{sym(pkgScreen, "Game.ChooseLoadout"), BucketWire, VerdictLive,
		"T2: the one loadout choice, from the first-entry panel (keys 1/2 or a click) through the KitHolder seam; writes the kit sidecar and releases the held world.", ""},
	{sym(pkgScreen, "Game.EquipFromPack"), BucketWire, VerdictLive,
		"T2: a click on a pack row in the kit panel.", ""},
	{sym(pkgScreen, "Game.UnequipSlot"), BucketWire, VerdictLive,
		"T2: a click on a worn row in the kit panel; a torch put away takes its minutes out of the light model with it.", ""},
	{sym(pkgPlayer, "GameControls.SetKitHolder"), BucketWire, VerdictLive,
		"T2: bindGameControls hands the controls the kit owner; the I key, the L key and the loadout choice all read through it.", ""},
	{sym(pkgItems, "Load"), BucketWire, VerdictLive,
		"T2: loads and validates data/strigoi/items.json when a game screen is built; an invalid table stops the screen, like the bestiary.", ""},
	{sym(pkgItems, "Kit.MainBite"), BucketWire, VerdictLive,
		"T2: his weapon as the resolver rolls it -- range after make and condition, class, reaction.", ""},
	{sym(pkgItems, "Kit.Absorb"), BucketWire, VerdictLive,
		"T2: his mail's share off a blow of its class, and the wear it takes for it.", ""},
	{sym(pkgItems, "Kit.CanBlock"), BucketWire, VerdictLive,
		"T2: an unruined shield in the off-hand; resolveBlow steps one blow a round down a band when it answers true.", ""},
	{sym(pkgWorld, "Combat.EndedReason"), BucketWire, VerdictLive,
		"Death screen v0 (23 Sep 2026): Game.deathCause reads it on the frame he dies, to say a fight killed him.", ""},
	{sym(pkgScreen, "Game.TalkTo"), BucketWire, VerdictLive,
		"T4: the talk verb -- a left click on a villager in reach (GameControls.talkClick) opens a conversation.", ""},
	{sym(pkgScreen, "Game.dawnWatch"), BucketWire, VerdictLive,
		"T4: a watch promised to the headman is counted on the frame he sees the night end -- the stage has left the night, for dawn or, after a labour or a sleep, for full day (earnExperience; BUG-17).", ""},
	{sym(pkgScreen, "Game.Craft"), BucketWire, VerdictLive,
		"T5: a click on a make row in the kit panel (GameControls.kitClick) makes or mends a recipe and charges its minutes.", ""},
	{sym(pkgScreen, "Game.barter"), BucketWire, VerdictLive,
		"T5: what a villager hands over or mends for labour -- Game.applyTalk on every answer.", ""},
	{sym(pkgWorld, "Spawns.SetSheltered"), BucketWire, VerdictLive,
		"T6: a night's sleep inside the palisade (the shelter rung) holds arrivals and noticing for its hours -- Game.applyTalk.", ""},
	{sym(pkgItems, "Kit.Eat"), BucketWire, VerdictLive,
		"T6: a click on a pack row he eats (peksimet) in the kit panel -- Game.eatFromPack.", ""},
	{sym(pkgWorld, "Notice.SetHidden"), BucketWire, VerdictLive,
		"T6: asleep inside the palisade, nothing new sees him while memory fades on its own clock -- Game.applyTalk.", ""},
	{sym(pkgScreen, "Game.Forage"), BucketWire, VerdictLive,
		"T7: K gathers branches for half an hour from a stock the land does not renew -- and sets the forage stance for those minutes, so D8 section 9's caught-head-down branch is reachable in a shipped build at last.", ""},
	{sym(pkgScreen, "Game.keepWatch"), BucketWire, VerdictLive,
		"T8: once a frame from earnExperience -- a promised watch counts the night minutes stood at the headman's post and sets the WATCH stance there.", ""},
	{sym(pkgWorld, "Corpses.Fall"), BucketWire, VerdictLive,
		"M4.7 step 1: every enemy death the resolver leaves on the map (reachedZero, quick-resolve) becomes an open body where it fell.", ""},
	{sym(pkgWorld, "Corpses.Close"), BucketWire, VerdictLive,
		"M4.7 step 1: the stake (X) closes the open body of a man at his feet -- Game.Stake.", ""},
	{sym(pkgWorld, "Corpses.Bury"), BucketWire, VerdictLive,
		"M4.7 step 2: D lays the open body at his feet in a hasty grave -- Game.Dig.", ""},
	{sym(pkgWorld, "Corpses.Rise"), BucketWire, VerdictLive,
		"M4.7 step 2: a door that wins its roll leaves where it lay -- Rising.roll.", ""},
	{sym(pkgWorld, "Rising.Advance"), BucketWire, VerdictLive,
		"M4.7 step 2: every frame after the spawn tables -- the open dead roll once per deep-night band.", ""},
	{sym(pkgWorld, "Spawns.Raise"), BucketWire, VerdictLive,
		"M4.7 step 3: a body that wins its roll stands up where it lay as the risen row -- Game.raiseTheDead, through Rising.roll.", ""},
	{sym(pkgWorld, "Spawns.LayDownDead"), BucketWire, VerdictLive,
		"M4.7 step 3: at first light each risen lies down where it stands and its group goes -- Game.firstLight.", ""},
	{sym(pkgWorld, "Combat.BreakOff"), BucketWire, VerdictLive,
		"M4.7 step 3: at first light the dead leave any fight; a fight left empty ends dawn -- Game.firstLight.", ""},
	{sym(pkgWorld, "Corpses.Raised"), BucketWire, VerdictLive,
		"M4.7 step 3: a risen body remembers the member it walks as, so its fall returns to it -- Rising.roll.", ""},
	{sym(pkgScreen, "Game.riteAtDawn"), BucketWire, VerdictLive,
		"M4.7 step 4 (Q4a): with the rite granted, the priest closes hasty graves near the church at each first light -- Game.firstLight.", ""},
	{sym(pkgScreen, "Game.seenStaking"), BucketWire, VerdictLive,
		"M4.7 step 4: a staking seen by a villager by day costs standing, once -- Game.stake.", ""},
	{sym(pkgScreen, "Game.DeadName"), BucketWire, VerdictLive,
		"M4.7 step 4 (Q6a): what one of the dead is called -- what he was in life before the priest's tale, the dead after (27 Sep 2026) -- on the hover and in the combat log alike, through GameControls.nameFor.", ""},
	{sym(pkgWorld, "Corpses.BodyOf"), BucketWire, VerdictLive,
		"27 Sep 2026: the body a risen member walks as, so Game.DeadName can call him what his body was in life before the priest's tale.", ""},
	{sym(pkgBestiary, "Catalog.DeadWere"), BucketWire, VerdictLive,
		"27 Sep 2026: the bestiary's words for what Night 1's dead and the edge floor's wanderers were -- deadNamesFrom reads them when the game screen is built, and refuses a bestiary without them.", ""},
	{sym(pkgScreen, "Game.stakeInFight"), BucketWire, VerdictLive,
		"M4.7 step 3b: X in a fight, on his turn -- a stake through a Downed man at his feet as the Action.", ""},
	{sym(pkgWorld, "Spawns.RaiseWanderer"), BucketWire, VerdictLive,
		"M4.7 step 5: the edge floor -- a nameless dead man stands up at the edge of the night in band 3, whatever is closed (S1 6.3) -- Rising.wanderers.", ""},
	{sym(pkgWorld, "Combat.Rejoin"), BucketWire, VerdictLive,
		"M4.7 step 3b: a Downed man who stands again is back in the fight he fell in -- Game.raiseTheDead.", ""},
	{sym(pkgWorld, "Spawns.Member"), BucketWire, VerdictLive,
		"M4.7 step 3b: the new member's watcher, for Combat.Rejoin -- Game.raiseTheDead.", ""},
	{sym(pkgWorld, "Corpses.DownedMember"), BucketWire, VerdictLive,
		"M4.7 step 3b: a fight goes on while a Downed man of it may stand again -- Combat.stillIn.", ""},
	{sym(pkgWorld, "Corpses.LastWalker"), BucketWire, VerdictLive,
		"M4.7 step 3b: a Downed man standing again takes the fallen one's remains off the map -- Game.raiseTheDead.", ""},
	{sym(pkgWorld, "Rising.Rite"), BucketWire, VerdictLive,
		"M4.7 step 2: a stake lowers soul pressure -- Game.stake.", ""},
	{sym(pkgScreen, "Game.Dig"), BucketWire, VerdictLive,
		"M4.7 step 2: the D key -- half an hour head-down for a hasty grave.", ""},
	{sym(pkgScreen, "Game.placeTheDead"), BucketWire, VerdictLive,
		"M4.7 Q2a PLACEHOLDER: Night 1's dead laid near where he enters, when the controls bind.", ""},
	{sym(pkgItems, "LoadHero"), BucketWire, VerdictLive,
		"T2/T3: reads the hero's kit and progress from beside his save when the controls bind; its absence opens the loadout choice. Replaces T2's LoadSidecar (retired at T3: reach measured it dead once the game moved here).", ""},
	{sym(pkgItems, "SaveHero"), BucketWire, VerdictLive,
		"T2/T3: writes kit and progress on every change and on leaving the world alive (never for a dead hero, the 12 Sep ruling). Replaces T2's SaveSidecar (retired at T3: reach measured it harness-only).", ""},
	{sym(pkgWorld, "Combat.SetEdges"), BucketWire, VerdictLive,
		"T3: CreateGame attaches the screen as the resolver's Edges source -- his talents as numbers. Nil is legal (no talents); the progress provider's edge block is the instrument.", ""},
	{sym(pkgWorld, "Combat.TakeXPEvents"), BucketWire, VerdictLive,
		"T3: what fights did that earns experience (slain, routed), taken every live frame by Game.earnExperience.", ""},
	{sym(pkgWorld, "Meters.SetConditioning"), BucketWire, VerdictLive,
		"T3: Endurance's talents applied to his body -- fatigue and hunger rates, the Shaken and no-Reaction thresholds.", ""},
	{sym(pkgWorld, "Light.SetCarriedBurnRate"), BucketWire, VerdictLive,
		"T3: Tallow and Pitch -- his carried torch burns slower.", ""},
	{sym(pkgScreen, "Game.EdgeOf"), BucketWire, VerdictLive,
		"T3: the d2world.Edges seam, read on every blow of his; Fire and Iron counts only while his torch burns.", ""},
	{sym(pkgScreen, "Game.PickTalent"), BucketWire, VerdictLive,
		"T3: a second click on an open talent in the talent panel (the first only selects it: no respec).", ""},
	{sym(pkgPlayer, "GameControls.SetProgressHolder"), BucketWire, VerdictLive,
		"T3: bindGameControls hands the controls the progress owner; the skill-tree key opens the talent panel through it.", ""},
	{sym(pkgProgress, "Load"), BucketWire, VerdictLive,
		"T3: loads and validates data/strigoi/talents.json when a game screen is built; a talent whose effect nothing reads is refused at load.", ""},
	{sym(pkgProgress, "Progress.Gain"), BucketWire, VerdictLive,
		"T3: experience from kills, routs and dawn, and the level it crosses.", ""},
	{sym(pkgProgress, "Progress.Effect"), BucketWire, VerdictLive,
		"T3: a talent effect summed over what he has taken; every system his talents change reads through it.", ""},
	{sym(pkgWorld, "Combat.Paced"), BucketWire, VerdictLive,
		"T1: whether a paced fight would run now (dial on AND a person at the controls). The game screen gates the tactical Move, click-to-strike and the no-hold-walk rule on it, so an unpaced or policy fight keeps the old click behaviour (review finding, 23 Sep).", ""},
	{sym(pkgWorld, "Combat.Tick"), BucketWire, VerdictLive,
		"T1: drives a paced fight on real seconds -- the packs' walks, one blow a beat, the round's close. Called every live frame from Game.tacticalAdvance. If this goes dark a paced fight opens and never moves past the first enemy slot.", ""},
	{sym(pkgWorld, "Combat.TakeRoundMinutes"), BucketWire, VerdictLive,
		"T1: the world time closed paced rounds owe; Game.tacticalAdvance advances the world by exactly that. If it goes dark a fight costs no world time at all and torches never burn in one.", ""},
	{sym(pkgWorld, "Combat.Participates"), BucketWire, VerdictLive,
		"T1: startChasesForTheAware skips a participant in a paced fight, so a chase cannot walk it in real time between its turns.", ""},
	{sym(pkgWorld, "Combat.SetStepper"), BucketWire, VerdictLive,
		"T1: CreateGame attaches the screen as the Stepper, which is what walks a pack on its turn. A nil stepper is legal and means packs wait to be reached, so losing this line is silent -- steps_ordered in the combat provider is the instrument.", ""},
	{sym(pkgWorld, "Combat.Tactical"), BucketWire, VerdictLive,
		"T1: the one read the HUD's tactical overlay and the click handlers make -- phase, pips, enemies, the blow log.", ""},
	{sym(pkgScreen, "Game.StepToward"), BucketWire, VerdictLive,
		"T1: the Stepper half that walks an enemy on its turn to a free tile beside the player, cut to EnemyMoveTiles. Reached through the d2world.Stepper interface from Combat.Tick.", ""},
	{sym(pkgScreen, "Game.Halt"), BucketWire, VerdictLive,
		"T1: stops a body where it stands when a paced fight opens. A released chase leaves its path in flight, so without this a participant closes in real time through its first turn.", ""},
	{sym(pkgScreen, "Game.Moving"), BucketWire, VerdictLive,
		"T1: the Stepper half a paced fight waits on -- a pack's walk and the hero's own Move.", ""},
	{sym(pkgScreen, "Game.OnTacticalTarget"), BucketWire, VerdictLive,
		"T1: a click on an enemy in a paced fight, from GameControls.OnMouseButtonDown through the inputCallbackListener seam: strike it, or walk beside it and strike on arrival. This is c-2b's click-to-strike, built.", ""},
	{sym(pkgPlayer, "GameControls.TacticalNotice"), BucketWire, VerdictLive,
		"T1: the refusals a tactical click earns (not your turn, too far, Move spent) put on the combat panel. Before it a refused commit was silent.", ""},
	{sym(pkgWorld, "Combat.Wait"), BucketWire, VerdictLive,
		"The decision timer. Game.Advance hands it the frame's elapsed while a turn is open -- a delta, never a wall-clock read inside the simulation. It is also what opens the pace window, so a fight nobody waited in writes no PACE line.", ""},
	{sym(pkgWorld, "Combat.SpendMove"), BucketWire, VerdictLive,
		"Marks the Move pip spent when a walk is ordered during the player's turn. It is the THIRD entry point to finishRound: without a Go caller here, a strike-then-move turn could never close itself and AutoEndTurn would be a dial that does nothing.", ""},
	{sym(pkgWorld, "Combat.ActionSpent"), BucketWire, VerdictLive,
		"Which of the two things E means. GameControls.combatEndTurn asks it to choose between hold (Action unspent, its signed meaning) and end (Action spent). One key, two choices, and the player never learns the difference.", ""},
	{sym(pkgWorld, "Combat.LastRound"), BucketWire, VerdictLive,
		"The closed round's row. writeRoundLine reads it every frame and writes one ROUND line per round -- the edge is the ROW changing, not Combat.Round(), which is the correction that stopped a three-round fight writing two lines.", ""},
	{sym(pkgWorld, "Combat.LastPace"), BucketWire, VerdictLive,
		"The finished fight's row. writePaceLine reads it on the fight-closed edge. Captured inside end() rather than by a caller a frame later, because a fight that opened and closed between two frames is otherwise invisible.", ""},
	{sym(pkgWorld, "Combat.Round"), BucketWire, VerdictLive,
		"The current round. MOVED from defer/dead at M4.4c-2a: the wish note stamps the round the player is in, and the ROUND line is the readout that row was waiting for since M4.4.", ""},
	{sym(pkgWorld, "Light.Add"), BucketWire, VerdictLive,
		"Lights a torch. MOVED from defer/harness-only at M4.4c-2a: the L key is the verb that block said did not exist. In a fight it costs the Action; out of one it is free and real-time.", ""},
	{sym(pkgWorld, "Light.Remove"), BucketWire, VerdictLive,
		"Spends a burnt-out torch. MOVED from defer/harness-only at M4.4c-2a, and this is the symbol M4.1 WAS REOPENED OVER -- exported, callable only by a test, a placed fire that could not be put out in any playable build. Game.writeTorchOut is its first shipped caller: a torch at 0 minutes is gone, not carried as an empty stick (Josh, 12 Sep). DOUSE does not call it; douse keeps the burn.", ""},
	{sym(pkgWorld, "Light.Carried"), BucketWire, VerdictLive,
		"What is in the player's hand. MOVED from observe/harness-only at M4.4c-2a: the L key asks before it decides whether L means light, relight or douse, and the ROUND and PACE lines read the burn off it.", ""},
	{sym(pkgWorld, "Clock.TimeOfDay"), BucketWire, VerdictLive,
		"HH:MM. MOVED from observe/harness-only at M4.4c-2a: the PACE, TORCH_OUT and WISH lines all stamp it. The HUD still shows time-to-sunset and still does not call it -- the row moved because the instrument appeared, not because S1 section 3.4 changed.", ""},
	// The three verbs by key. The brief asks for a row per EVENT; an event is
	// an enum constant and deadcode classifies functions, so the rows are the
	// handlers behind them -- which is the executable claim anyway. If one of
	// these goes dark the key still exists in the table and does nothing,
	// which is the failure a player would report as "F is broken".
	{sym(pkgPlayer, "GameControls.combatStrike"), BucketWire, VerdictLive,
		"F. Commits the Action against the first living adjacent enemy in D8 order -- it passes an EMPTY target and lets Combat.Commit choose, so the key and the harness field cannot drift apart. Reached from OnKeyDown.", ""},
	{sym(pkgPlayer, "GameControls.combatTorch"), BucketWire, VerdictLive,
		"L, and the right mouse button (Josh, 25 Sep 2026: one implementation, two bindings). One verb: light, relight or douse, decided before anything is spent so that a refused commit changes nothing. Reached from OnKeyDown and, in Strigoi's game, OnMouseButtonDown.", ""},
	{sym(pkgPlayer, "GameControls.combatEndTurn"), BucketWire, VerdictLive,
		"E. Ends the turn -- hold when the Action is unspent, end when it is spent. Reached from OnKeyDown. Without it a turn with an unspent Move waits forever.", ""},
	{sym(pkgScreen, "Game.WorldHeldBy"), BucketWire, VerdictLive,
		"Names what holds the world (the escape menu, the loadout choice, a talk, the journal, a paced fight) or \"\" while it runs. worldRunning is WorldHeldBy() == \"\", and the ui provider reports it so step_world refuses a hold no number of ticks lifts (history item 121).", ""},
	{sym(pkgPlayer, "GameControls.SetWorldHolder"), BucketWire, VerdictLive,
		"Game.OnLoad attaches the game screen's WorldHeldBy to the controls, for the ui provider's world_held_by. Without it the field reads \"unknown\" and step_world refuses to step at all.", ""},
	{sym(pkgScreen, "Game.worldRunning"), BucketWire, VerdictLive,
		"The one boolean that stops the world, keeping BOTH of its original terms (menu closed OR not single-player) and adding the paced fight (Combat.WorldHeld, Awaiting widened); since history item 121 it is WorldHeldBy() == \"\". advanceWorld is gated on it; MapEngine.Advance deliberately is NOT, so sprites animate and in-flight walks finish while a person thinks.", ""},
	{sym(pkgWorld, "Combat.Encounter"), BucketWire, VerdictLive,
		"Which fight is live RIGHT NOW. The wish console verb asks it as the player types, from Game.commandWish, bound in Game.OnLoad in a shipped build. It exists because the two things that look like an answer are not one: LastRound names the last CLOSED round and is empty through the whole of round one, LastPace only exists after the fight is over and goes on naming it.", ""},
	// The class pin (ruled 11 Sep 2026, built in M4.4c-2a). Two rows, not one:
	// the second is the gate on the pin itself.
	{sym(pkgScreen, "getHeroRenderConfiguration"), BucketWire, VerdictLive,
		"The select-hero screen loads one sprite set per entry in the map this returns. CreateSelectHeroClass ranges over it, in a shipped build, with no harness in the path.", ""},
	{sym(pkgScreen, "pinRoster"), BucketWire, VerdictLive,
		"Reduces the seven-class roster to the one class a new game offers. This row is the pin's own gate: if someone unpins by returning the roster straight out of getHeroRenderConfiguration, nothing calls this any more and it goes DEAD here before anyone has to notice the screen.", ""},
	{sym(pkgScreen, "shippedCombatDials"), BucketWire, VerdictLive,
		"The dials a real launch runs on -- the signed defaults with PlayerControl flipped to human (ask 5). Called from CreateGame, so it is live in every shipped build. This row exists because the gate could NOT see the gap it closes: the keys call Combat.Commit either way, so every row stayed green while the seam was harness-only. If someone drops the flip, the register cannot catch it and TestTheShippedScreenTakesTheTurn is what does.", ""},
	{sym(pkgEntity, "NPC.MonStat"), BucketWire, VerdictLive,
		"The monstats record an NPC was built from. Reached from Game.BodyOf's on-demand adoption path, so it inherits that row's verdict exactly, as its own comment has said since step 3.", ""},
	{sym(pkgEntity, "NPC.StartAction"), BucketWire, VerdictLive,
		"Plays one animation and HOLDS it, then returns the monster to Neutral -- or, for a death, to a Dead that is held for the rest of the run. Called from Game.Animate on every swing, every blow taken and every death. Before it, nothing could make a monster's sprite survive the next tick.", ""},
	{sym(pkgEntity, "NPC.SetAnimationMode"), BucketWire, VerdictLive,
		"The first exported way to tell a monster to play a mode. Its only caller is NPC.StartAction, which is the honest reading: the row stays because the symbol stays, and it is wire because a real build now reaches it. tools/animcensus measured on 31 Aug that A1, GH, DT and DD all exist for the three codes the spawn tables use.", ""},
	{sym(pkgEntity, "MapEntityFactory.NewCreature"), BucketWire, VerdictLive,
		"Builds a project-owned PNG creature without a COF. gameSpawner calls it for the authored dogs row, so every natural dog arrival in a shipped game crosses this constructor.", ""},
	{sym(pkgEntity, "Creature.StartAction"), BucketWire, VerdictLive,
		"Maps the combat resolver's swing, hit and death acts onto Strigoi's own creature modes. Game.Animate reaches it through the same small interface NPC.StartAction satisfies.", ""},
	{sym(pkgEntity, "Creature.HarnessState"), BucketObserve, VerdictHarnessOnly,
		"Reports the creature name, Strigoi animation mode, direction and movement -- and since M5.1b the sheet the current mode is drawn from and the speed it walks at -- for get_entity. It is read-only and exists so the same observer can inspect inherited NPCs and project-owned creatures.", ""},
	{sym(pkgBestiary, "Load"), BucketWire, VerdictLive,
		"Loads and validates the shipped project-owned creature catalog when a game screen is constructed; an invalid bestiary prevents a half-authored game from starting.", ""},
	{sym(pkgBestiary, "Catalog.ByID"), BucketWire, VerdictLive,
		"Resolves player and art-review spawnmon commands through the same authored creature definition natural spawns use.", ""},
	{sym(pkgBestiary, "Catalog.ForSpawnRow"), BucketWire, VerdictLive,
		"Maps an authored spawn-table row to project art, its inherited stats stand-in and Strigoi health in a shipped game.", ""},
	{sym(pkgBestiary, "Entry.SpeedOr"), BucketWire, VerdictLive,
		"M5.1b: a creature's walking speed -- its authored bestiary speed, or its stand-in's SpeedBase when none is authored. gameSpawner calls it for every project creature a spawn row or a risen man stands up, so the shipped game walks at the authored number.", ""},
	{sym(pkgWorld, "Spawns.ProfileOf"), BucketWire, VerdictLive,
		"What one enemy fights as: its PACK (not its row -- two dog packs are two packs), its authored Speed and its bite. The resolver calls it through the Profiles interface to build D8's order and to draw damage. It is the seam that put speed and damage on the spawn row instead of reading them out of the D2 record. Since step 5 it also carries the pack's STARTING COUNT, which the rout decrement and quick-resolve's advantage are both measured against. Since M4.4c-1 it has a SECOND Go caller: Game.ShowsBar asks it what a spawned enemy IS, because the monstats code is only its sprite.", ""},

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
		"The only way a chase ends. Called from the resolver when an enemy dies, when a pack breaks, and when quick-resolve finishes a fight -- and, since the hardening burst (14 Sep 2026), from Spawns.Despawn: a pack sent home at daybreak releases its members' chases too, or they become ghost pursuits re-pathing an A* forever for a member no longer on the map (audit B2). IT IS HALF OF A PAIR: startChasesForTheAware runs every frame with no liveness filter, so a release on its own is undone on the very next frame, and the same death also calls Notice.Unwatch.", ""},

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

	// M4.6 B2a, C6 (28 Sep 2026): one seed per stream. WIRE because CreateGame
	// must keep seeding the spawn tables, combat and the rising through it: if
	// it went harness-only, the three streams would be back on one seed in the
	// shipped game and a restore that swapped two of them would pass the seed
	// check that now rests on it (d2rand.StreamState.Check) -- C6 hollow, with
	// its unit test still green. (Folded here from register_b2a.go at the B2a
	// review, C4: the register lives in one place.)
	{sym("d2common/d2rand", "Derive"), BucketWire, VerdictLive,
		"C6: CreateGame seeds the spawn tables, combat and the rising each on its own stream of the run's seed (the world keeps the seed itself), so no two gameplay streams hand out one sequence.", ""},

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
	// MOVED to wire at T4 (23 Sep 2026). Recorded before this row was edited:
	// the village's talk is the first game verb that fills a meter -- water
	// at the well (S1 §8.2's first rung) and bread for labour at the forge.
	// There is still no eat or drink verb from his own pack (Phase 6's
	// inventory); the peksimet in his kit is not eaten by anything.
	{sym(pkgWorld, "Meters.Consume"), BucketWire, VerdictLive,
		"T4: Game.applyTalk gives the meters what a villager's answer gives -- water at the trough, bread for an hour at the bellows.", ""},
	// Light.Add and Light.Remove MOVED to wire at M4.4c-2a (see the wire block
	// above). The torch key is the verb that lights a fire, and a torch at 0
	// minutes is removed -- the two call sites this block said did not exist.
	// Light.Remove was the symbol M4.1 was REOPENED over; it closes by being
	// used, not by being re-bucketed.
	// Meters.Food/Water/Fatigue and Meters.Dying MOVED to wire at M4.4c-1 (see
	// the wire block above): the selected-squad sheet reads the three meters,
	// and the overhead dying cue reads Dying, both through the Squads owner.
	//
	// Combat.Round MOVED to wire at M4.4c-2a (see the wire block above): the
	// ROUND line is the readout this row was waiting for.
	//
	// Combat.Order DID NOT MOVE, and c-2's brief expected it to. MEASURED 18
	// Sep 2026: still dead. The reason is a design call c-2a made and did not
	// announce -- the strike key commits with an EMPTY target and the target
	// is chosen inside Combat.Commit, one implementation for the key and the
	// settable field both, rather than the key asking for the order and
	// picking the first living adjacent enemy itself. That is the better
	// shape; it just leaves this accessor with no Go caller, which is the
	// truth and is what the row now says. Recorded in state.md before this
	// text was written, per this file's own rule.
	{sym(pkgWorld, "Combat.Order"), BucketWire, VerdictLive,
		"The activation sequence -- D8 section 9's since step 4. MOVED from defer/dead at J1 (24 Sep 2026): the journal's fight sampler (Game.sampleFight) reads the participants off it to note each kind of enemy he has met. The turn UI that DISPLAYS an order will be its second reader.", ""},

	// THE N-PATH IS HARNESS-DRIVEN, and the register says so rather than
	// claiming a green (brief §9 objection 3): squad_add/squad_remove are
	// settable provider fields handled inside HarnessSet, so addSquad/removeSquad
	// and the deployer they drive are harness-only one level down -- exactly
	// Meters.Consume's shape. A shipped build has ONE squad (the player) and
	// never deploys a second (ruled ask 10(b): s:1's model is the player, no
	// entity created, no RNG drawn).
	{sym(pkgWorld, "Squads.addSquad"), BucketDefer, VerdictHarnessOnly,
		"squad_add's implementation: deploys a second squad's model. Reached only from HarnessSet, so a shipped build never runs it.", "the campaign, when a found survivor is a selectable squad"},
	{sym(pkgWorld, "Squads.removeSquad"), BucketDefer, VerdictHarnessOnly,
		"squad_remove's implementation, harness-only for the same reason.", "the campaign"},
	{sym(pkgWorld, "Squads.Selected"), BucketDefer, VerdictHarnessOnly,
		"The selected squad id, read only by the ui provider's HarnessState. The game reads selection through Bars()'s Selected flag, not this accessor.", "M4.4c-2 or later, when a readout wants it"},
	{sym(pkgScreen, "squadDeployer.Deploy"), BucketDefer, VerdictHarnessOnly,
		"Places a second squad's model entity. Reached only from Squads.addSquad <- HarnessSet <- the harness.", "the campaign"},
	{sym(pkgScreen, "squadDeployer.Recall"), BucketDefer, VerdictHarnessOnly,
		"Removes a deployed model, harness-only for the same reason as Deploy.", "the campaign"},
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
	// MOVED to wire at M4.7 step 1 (23 Sep 2026), recorded before this row was
	// edited: the deferral named this milestone, and it landed.
	{sym(pkgWorld, "Spawns.OpenBodies"), BucketWire, VerdictLive,
		"The carrion count. M4.7: the corpse registry's change hook reads it to move it by one per body fallen or staked.", ""},

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

	// M4.6 B2a -- THE SNAPSHOTS WITH NO ENTITY IDS (clock, light, squads,
	// corpses, rising, combat; 28 Sep 2026). All DEFERRED and expected DEAD:
	// nothing in the shipped build takes a snapshot or restores one yet.
	// Game.SaveWorld (B3) takes the snapshots; the quiet-evening load (B4a)
	// validates and restores these -- except Squads, whose deployed models are
	// map entities and need the next-id seam, so its Restore and Validate are
	// the hunted-night load's (B4b; D3 of the B2 review, 28 Sep 2026). B4a
	// restores s:1 through it and refuses a snapshot holding a deployed squad,
	// so the day B4a calls it the row goes live early, and the burst that
	// moves it to wire must show that refusal. (These rows sat in a file of
	// their own, register_b2a.go, appended at init; folded here at the B2a
	// review, C4, because the register lives in one place -- RegisterMarkdown.)
	//
	// Every Restore has a Validate twin (D4): the same checks, changing
	// nothing, so a load checks every block of the file before it restores
	// any and a refusal anywhere sets the whole file aside (rule 7).
	//
	// M4.6 B3 (28 Sep 2026) BUILT Game.SaveWorld, and every Snapshot row
	// moved from "dead, for B3" to "HARNESS-ONLY, for B5": the save verb's
	// one caller today is the harness's strigoi_save_game, and the game's own
	// callers -- the escape menu's SAVE GAME and SAVE AND EXIT, the window's
	// close hook, the dawn autosave (rule 10) -- are B5's. A save the player
	// cannot make is the hollow class this register is for, so these rows
	// stay deferred until B5 wires them and moves them to wire. The Restore
	// and Validate rows are untouched: nothing loads yet.
	{sym(pkgWorld, "Clock.Snapshot"), BucketDefer, VerdictHarnessOnly,
		"The world minutes since the epoch, the clock's whole state. Game.SaveWorld calls it.", "M4.6 B5"},
	{sym(pkgWorld, "Clock.Restore"), BucketDefer, VerdictDead,
		"Puts the clock at the saved minute, right after NewClock and before bindProgress reads it (trap 1). The load calls it.", "M4.6 B4a"},
	{sym(pkgWorld, "Clock.Validate"), BucketDefer, VerdictDead,
		"Restore's check alone (D4): the load validates every block before it restores any.", "M4.6 B4a"},
	{sym(pkgWorld, "Light.Snapshot"), BucketDefer, VerdictHarnessOnly,
		"Every light source (no radius: a dial, D2) and the next id. Game.SaveWorld calls it.", "M4.6 B5"},
	{sym(pkgWorld, "Light.Restore"), BucketDefer, VerdictDead,
		"Puts every source back exactly, his carried torch included -- the light model is the truth on load, the kit torch's minutes are ignored, and nothing re-lights through the L path (D1). Radii come from the dials (D2). The load calls it.", "M4.6 B4a"},
	{sym(pkgWorld, "Light.Validate"), BucketDefer, VerdictDead,
		"Restore's check alone (D4), including a burn that fits the kind (the B2a review's B2).", "M4.6 B4a"},
	{sym(pkgWorld, "Squads.Snapshot"), BucketDefer, VerdictHarnessOnly,
		"Every squad with its members and meters, s:1's member written as the player. Game.SaveWorld calls it.", "M4.6 B5"},
	{sym(pkgWorld, "Squads.Restore"), BucketDefer, VerdictDead,
		"Puts the squads back into a fresh owner, s:1 in place so the game's meters pointer and his conditioning survive. Deployed squads' models are map entities rebuilt through the next-id seam, so this row is the hunted-night load's (D3); B4a restores s:1 alone and refuses a deployed squad.", "M4.6 B4b"},
	{sym(pkgWorld, "Squads.Validate"), BucketDefer, VerdictDead,
		"Restore's check alone (D4), including one man in one squad (the B2a review's B2).", "M4.6 B4b"},
	{sym(pkgWorld, "Corpses.Snapshot"), BucketDefer, VerdictHarnessOnly,
		"Every body in fall order and the three member maps. Game.SaveWorld calls it.", "M4.6 B5"},
	{sym(pkgWorld, "Corpses.Restore"), BucketDefer, VerdictDead,
		"Puts the bodies back into an empty registry WITHOUT the open-count callback (trap 5); the load skips placeTheDead and sets openBodies from the file.", "M4.6 B4a"},
	{sym(pkgWorld, "Corpses.Validate"), BucketDefer, VerdictDead,
		"Restore's check alone (D4), accepting only what the machine could have written (the B2a review's B2).", "M4.6 B4a"},
	{sym(pkgWorld, "Rising.Snapshot"), BucketDefer, VerdictHarnessOnly,
		"The accrued soul pressure (not the dial's constant, D2), the band and stage last seen, the counters and the stream. Game.SaveWorld calls it.", "M4.6 B5"},
	{sym(pkgWorld, "Rising.Restore"), BucketDefer, VerdictDead,
		"Puts the rising back after the corpses, its stream checked against the game seed and restored through d2rand. The load calls it.", "M4.6 B4a"},
	{sym(pkgWorld, "Rising.Validate"), BucketDefer, VerdictDead,
		"Restore's check alone (D4), against the file's corpses block, so it runs before the registry is restored.", "M4.6 B4a"},
	{sym(pkgWorld, "Combat.Snapshot"), BucketDefer, VerdictHarnessOnly,
		"The model between fights: counters, records and the stream. Refuses during a fight (ErrCombatFighting) and while experience or paced minutes wait to be taken. Game.SaveWorld calls it, and its refusal is the save's FIGHTING.", "M4.6 B5"},
	{sym(pkgWorld, "Combat.Restore"), BucketDefer, VerdictDead,
		"Puts the model back between fights, its stream checked against the game seed, with the other systems' counters. The load calls it.", "M4.6 B4a"},
	{sym(pkgWorld, "Combat.Validate"), BucketDefer, VerdictDead,
		"Restore's check alone (D4).", "M4.6 B4a"},

	// The saved stream's one shape (the B2 review: B2a's and B2b's private
	// copies made one, in d2rand) and its check. Dead with the snapshots that
	// write and read it.
	{sym("d2common/d2rand", "StateOf"), BucketDefer, VerdictHarnessOnly,
		"A counted stream's {seed, draws} for the world file. Every Snapshot with a stream calls it.", "M4.6 B5"},
	// StreamState.Check and SeedFor are harness-only since B3: d2save's
	// World.Check refuses a file whose streams are not the seed's before
	// SaveWorld writes it (strict at save). Their game caller is the load's
	// validation, B4a.
	{sym("d2common/d2rand", "StreamState.Check"), BucketDefer, VerdictHarnessOnly,
		"Refuses a saved stream on another stream's seed (a swapped block) or past MaxDraws (the B2a review's B1). Every Validate with a stream calls it.", "M4.6 B4a"},
	{sym("d2common/d2rand", "StreamState.RestoreInto"), BucketDefer, VerdictDead,
		"Puts a stream back through Stream.Restore, rand and counted source together. Every Restore with a stream calls it.", "M4.6 B4a"},
	{sym("d2common/d2rand", "SeedFor"), BucketDefer, VerdictHarnessOnly,
		"The seed a named stream runs on in a game of a given seed: the check's half.", "M4.6 B4a"},

	// M4.6 B2b -- THE SNAPSHOTS THAT CARRY ENTITY IDS (28 Sep 2026). All
	// DEFERRED and expected DEAD: B2b builds the verbs and tests them in their
	// own packages, and nothing in the shipped build calls them yet. The
	// snapshots are for B3's Game.SaveWorld, the restores, their Validate
	// twins (D4) and the next-id seam for B4b's hunted-night load. The day
	// either calls one, its row goes live and the gate goes red until the row
	// moves to wire -- a save that exists only in a unit test is the hollow
	// class this register was built for. (The first draft of these rows sat in
	// a file of their own, appended at init; they are here because the
	// register lives in one place -- see RegisterMarkdown.)
	{sym(pkgWorld, "Spawns.Snapshot"), BucketDefer, VerdictHarnessOnly,
		"The spawn tables' state for the world file, members by entity id with the dead marked gone. Game.SaveWorld calls it.", "M4.6 B5"},
	{sym(pkgWorld, "Spawns.Restore"), BucketDefer, VerdictDead,
		"Puts the tables back through a Resolver, after the entities are rebuilt with their ids and motion. The hunted-night load calls it.", "M4.6 B4b"},
	{sym(pkgWorld, "Notice.Snapshot"), BucketDefer, VerdictHarnessOnly,
		"Every watch, watcher by entity id and the player as the word player. Game.SaveWorld calls it.", "M4.6 B5"},
	{sym(pkgWorld, "Notice.Restore"), BucketDefer, VerdictDead,
		"Puts every watch back through a Resolver; an unresolved watch is an error, never a drop. The hunted-night load calls it.", "M4.6 B4b"},
	{sym(pkgWorld, "Pursuit.Snapshot"), BucketDefer, VerdictHarnessOnly,
		"Every chase, hunter by entity id and the player as the word player. Game.SaveWorld calls it.", "M4.6 B5"},
	{sym(pkgWorld, "Pursuit.Restore"), BucketDefer, VerdictDead,
		"Puts every chase back through a Resolver, solving nothing: the walk is the entity's motion. The hunted-night load calls it.", "M4.6 B4b"},
	{sym(pkgWorld, "Spawns.Validate"), BucketDefer, VerdictDead,
		"Restore's check alone (D4), through the same Resolver, stream seed included.", "M4.6 B4b"},
	{sym(pkgWorld, "Notice.Validate"), BucketDefer, VerdictDead,
		"Restore's check alone (D4), through the same Resolver.", "M4.6 B4b"},
	{sym(pkgWorld, "Pursuit.Validate"), BucketDefer, VerdictDead,
		"Restore's check alone (D4), through the same Resolver.", "M4.6 B4b"},
	{sym(pkgMapEngine, "MapEngine.SetNextEntityID"), BucketDefer, VerdictDead,
		"The next-id seam through the engine: the next entity construction takes a saved id (NewNPC and NewCreature wear it, the rest spend it); an id already on the map is refused. The load rebuilds every saved entity through it.", "M4.6 B4b"},
	{sym(pkgEntity, "MapEntityFactory.PendingEntityID"), BucketDefer, VerdictDead,
		"Whether a set id is still waiting. A load that has rebuilt everything asserts nothing is.", "M4.6 B4b"},
	{sym(pkgEntity, "Creature.MotionSnapshot"), BucketDefer, VerdictHarnessOnly,
		"A creature's walk and pose for the world file's entity list. Game.SaveWorld calls it.", "M4.6 B5"},
	{sym(pkgEntity, "Creature.RestoreMotion"), BucketDefer, VerdictDead,
		"Puts a rebuilt creature back mid-stride. The load calls it before Spawns.Restore, which checks each member stands where he was saved.", "M4.6 B4b"},
	{sym(pkgEntity, "NPC.MotionSnapshot"), BucketDefer, VerdictHarnessOnly,
		"An inherited monster's walk and pose for the world file. Game.SaveWorld calls it.", "M4.6 B5"},
	{sym(pkgEntity, "NPC.RestoreMotion"), BucketDefer, VerdictDead,
		"Puts a rebuilt inherited monster back mid-stride. The load calls it.", "M4.6 B4b"},
	{sym(pkgScreen, "gameSpawner.Snapshot"), BucketDefer, VerdictHarnessOnly,
		"The arrival count, which sets where the next pack comes from. Game.SaveWorld calls it.", "M4.6 B5"},
	{sym(pkgScreen, "gameSpawner.Restore"), BucketDefer, VerdictDead,
		"Puts the arrival count back. The load calls it with the spawns.", "M4.6 B4b"},

	// M4.6 B3 -- THE FILE AND THE SAVE VERB (28 Sep 2026). Game.SaveWorld is
	// harness-only until B5 gives it the game's callers (the menu, the close
	// hook, the dawn autosave); what the save needs that the game ALREADY
	// calls is wire -- the .od2's .bak generation, the sidecar's one
	// document, the bestiary id a creature is saved by.
	{sym(pkgScreen, "Game.SaveWorld"), BucketDefer, VerdictHarnessOnly,
		"THE save verb: the world file, then the .od2, then the sidecar, then the death screen's copy re-taken; refused (touching no file) while fighting, talking, reading, choosing, dead or networked. The harness's strigoi_save_game calls it today; the escape menu, the close hook and the dawn autosave are B5's.", "M4.6 B5"},
	{sym(pkgSave, "Encode"), BucketDefer, VerdictHarnessOnly,
		"Writes the world file's bytes, every block in order (omit is the negative controls'). Game.SaveWorld calls it.", "M4.6 B5"},
	{sym(pkgSave, "WriteWorld"), BucketDefer, VerdictHarnessOnly,
		"The world file's write: the previous version-1 file kept as .bak, a file this build cannot read set aside unread (rule 7), then tmp + rename. Game.SaveWorld calls it.", "M4.6 B5"},
	{sym(pkgSave, "Decode"), BucketDefer, VerdictHarnessOnly,
		"Reads a world file, refusing any version but 1 and any block missing, null or unknown. SaveWorld reads back what it is about to write (strict at save); the load reading the file before Open is its game caller.", "M4.6 B4a"},
	{sym(pkgSave, "World.Check"), BucketDefer, VerdictHarnessOnly,
		"The file's own refusals: streams on the seed, rng copies equal to their blocks, every live ref an entity in the list, hero and entities at their own pos. Decode calls it; the load is its game caller.", "M4.6 B4a"},
	{sym(pkgEntity, "Creature.CreatureID"), BucketDefer, VerdictHarnessOnly,
		"The bestiary entry a creature is saved by (entities[].creature). Game.SaveWorld reads it; the load rebuilds from it.", "M4.6 B5"},
	{sym(pkgEntity, "Creature.SetCreatureID"), BucketWire, VerdictLive,
		"The game spawner (and the terminal's spawnmon) records the bestiary entry beside every NewCreature, so every creature the game places can be saved and rebuilt. If it went dark, every creature would refuse the save.", ""},
	{sym(pkgItems, "KeepGeneration"), BucketWire, VerdictLive,
		"The .od2's previous generation, kept as .bak before the server's hero save replaces it (rule 5). Every hero save the game makes goes through it.", ""},
	{sym(pkgItems, "HeroBytes"), BucketWire, VerdictLive,
		"The sidecar's one document: saveKit writes it, and the world save embeds the same bytes it writes, so the two files are one moment.", ""},

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
	// Clock.TimeOfDay MOVED to wire at M4.4c-2a (see the wire block above).
	// The M4.4a note two blocks up is kept as written and is no longer true of
	// this symbol: the HUD still shows time-to-sunset and still never calls
	// it, but the PACE, TORCH_OUT and WISH lines stamp HH:MM and they are
	// written by the game screen in every shipped build. The row moved because
	// a caller appeared, not because the reasoning changed.
	{sym(pkgWorld, "Clock.SetFrozen"), BucketObserve, VerdictHarnessOnly,
		"Freezes the clock for a script. The game must never do this, so harness-only is the correct answer and not a deferral.", ""},
	{sym(pkgWorld, "Clock.SetMoon"), BucketObserve, VerdictHarnessOnly,
		"Sets the moon phase for a script. A dial, not a world change.", ""},
	{sym(pkgWorld, "Light.Radius"), BucketObserve, VerdictHarnessOnly,
		"Reports the lit radius around the player.", ""},
	// Light.Carried MOVED to wire at M4.4c-2a (see the wire block above): the
	// torch key asks what is in the player's hand before it decides what L
	// means, and the ROUND and PACE lines read the burn.
	// M4.4c-2a's read-only half. These two report the turn seam to a script
	// and the game never asks them, which is correct and not a deferral: the
	// game ACTS on the seam through Awaiting, ActionSpent and Commit, all
	// wired above. A script needs the numbers; a player needs the behaviour.
	{sym(pkgWorld, "Combat.MoveSpent"), BucketWire, VerdictLive,
		"Whether the Move pip is spent. Harness-only until T1 (23 Sep 2026): the game SET it (Combat.SpendMove) and never read it back. Game.tacticalMove now reads it to refuse a second Move in one turn, so it moved to wire/live -- recorded here before the row was edited, per docs/reachability.md.", ""},
	//
	// Combat.DecisionSeconds and Combat.DecisionSecondsRound had rows here for
	// about an hour on 18 Sep 2026 and are GONE: the gate measured them DEAD,
	// not harness-only, because HarnessState reads the two fields straight off
	// the struct in the same package. Two exported accessors with no reader in
	// either build is the hollow-class shape, so they were deleted rather than
	// given a caller. The register caught them the day they were written,
	// which is the fastest it has ever caught anything.
	{sym(pkgWorld, "Combat.CommitsRefused"), BucketObserve, VerdictHarnessOnly,
		"How many commits were refused because no turn was waiting. A script counts them; the game just refuses. This is how act 2's menu control proves a key was swallowed rather than silently doing nothing.", ""},
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
	{sym(pkgWorld, "Notice.Dials"), BucketWire, VerdictLive,
		"Harness-only until T3 (23 Sep 2026): Game.applyProgress reads the live radius to move it by Quiet Step's DELTA, so a pick never resets a radius something else set. Recorded before the row was edited, per docs/reachability.md.", ""},
	{sym(pkgWorld, "Notice.Report"), BucketObserve, VerdictHarnessOnly,
		"The per-watcher block -- sees, distance, light at the quarry, verdict -- that makes M4.3b's clause 5 assertable at all.", ""},
	{sym(pkgWorld, "Notice.SetRadius"), BucketWire, VerdictLive,
		"Harness-only until T3 (23 Sep 2026): Game.applyProgress now calls it for Quiet Step, which moves the radius beasts notice him at. Recorded here before the row was edited, per docs/reachability.md.", ""},
	{sym(pkgWorld, "Notice.SetLitLevel"), BucketObserve, VerdictHarnessOnly,
		"Writes the lit-level dial.", ""},
	{sym(pkgWorld, "Spawns.SetMorale"), BucketObserve, VerdictHarnessOnly,
		"Writes a group's morale so a script can drive it to the rout threshold. STILL HARNESS-ONLY AFTER STEP 5, and deliberately: the game hurts a pack through Spawns.Hurt, which subtracts and floors and writes the field itself. Routing this through here would have been the DRY refactor and would have flipped this row live transitively, which is the shape three earlier rows were caught by.", ""},
	// MOVED to wire at M4.7 step 1 (23 Sep 2026), recorded before this row was
	// edited: the corpse registry drives the count by delta on every fall and
	// every stake -- "the corpse machine drives it when it lands" (M4.3b ask 3).
	{sym(pkgWorld, "Spawns.SetOpenBodies"), BucketWire, VerdictLive,
		"M4.7: the corpse registry's change hook (game.go) moves the carrion count by one on every body that falls and every body staked.", ""},

	// 23-24 Sep 2026 (history items 111-113).
	{sym(pkgWorld, "Combat.Adjacent"), BucketWire, VerdictLive,
		"The fight's reach for two points: inReach uses it, and the stake in a fight (Game.stakeInFight) takes the Downed man it counts as beside him.", ""},
	{sym(pkgPlayer, "GameControls.inventoryAction"), BucketWire, VerdictLive,
		"The inventory key and the HUD mini-panel's inventory button open Strigoi's kit, not Diablo II's grid.", ""},
	{sym(pkgPlayer, "GameControls.skillsAction"), BucketWire, VerdictLive,
		"The skill key, the mini-panel's skill button and the add-skill button open Strigoi's talents, not Diablo II's skill tree.", ""},
	{sym(pkgAsset, "AssetManager.EnsureRecords"), BucketWire, VerdictLive,
		"The generated world's tables load when it is built (generateAct1World, LoadStamp, NewObject, and since the tables burst MapEngine.ResetMap's level types); without the call a generated world has no presets. The missiles and cast overlays load on the first -classic cast (GameClient.handleCastSkillPacket); without it a right-click casts nothing. Diablo II's item tables load with the first item made (diablo2item.ItemFactory.NewItem) and the grid's layout when the grid first opens (Inventory.Load).", ""},
	// M5.3's tables burst (26 Sep 2026): ten Diablo II tables left Strigoi's
	// boot, and these are what reads Strigoi's data in their place. If one
	// goes dark the game reads a zero where a table used to answer.
	{sym(pkgAsset, "AssetManager.Classic"), BucketWire, VerdictLive,
		"Which game this launch is: -classic loads Diablo II's fourteen tables at boot and keeps its hero, items, cast and levels; Strigoi's game loads four (d2app.initDataDictionaries) and reads its own data. Everything below asks it.", ""},
	{sym(pkgHero, "Body"), BucketWire, VerdictLive,
		"The hero's health, stamina and run drain from his manifest (hero.json), charstats.txt's amazon by default: HeroStateFactory.NewHeroStats for a new hero, StaminaRunDrain for every player built. The 240 pin is data here.", ""},
	{sym(pkgMapEngine, "MapEngine.SetAuthoredRegion"), BucketWire, VerdictLive,
		"An authored map's sound environment and name, from its map properties (MapGenerator.generateAuthored); without it the village is silent and nameless.", ""},
	{sym(pkgMapEngine, "MapEngine.AuthoredRegion"), BucketWire, VerdictLive,
		"What the game plays on the village instead of levels.txt's row: Game.Advance sets the sound environment from it once a second.", ""},
	{sym(pkgPlayer, "HUD.renderHand"), BucketWire, VerdictLive,
		"The HUD's two skill icons are his two hands in Strigoi's game -- the letters of the keys their verbs are on (F, L) until the art lands; renderLeftSkill and renderRightSkill draw them.", ""},
	// The tables burst's review (27 Sep 2026).
	{sym(pkgServer, "joinRefusal"), BucketWire, VerdictLive,
		"A host refuses a client of the other game (-classic or not), naming the flag to change: GameServer.registerConnection asks it of every join. Without it a -classic client's cast reached a Strigoi host's own client as a nil skill record.", ""},
	{sym(pkgClient, "GameClient.JoinRefused"), BucketWire, VerdictLive,
		"The host's refusal, read every frame by Game.Advance, which takes the player back to the main menu with it. Without the read a refused client sits on a world that never comes.", ""},
	{sym(pkgHero, "HeroState.onDisk"), BucketWire, VerdictLive,
		"An old save's Diablo II skills, kept out of Strigoi's game, go back into the file when he is saved (HeroStateFactory.Save), so a later -classic load still has them.", ""},
	{sym(pkgPlayer, "heroExperience"), BucketWire, VerdictLive,
		"The HUD's experience bar and its tooltip read Strigoi's own progression in his game (HUD.experience), not Diablo II's 0 / 0.", ""},
	{sym(pkgAudio, "SoundEnvironment.Current"), BucketObserve, VerdictHarnessOnly,
		"Reads the sound environment playing and its song, for the village provider (village.sound_env, village.music); changes nothing.", ""},
	{sym(pkgMapGen, "MapGenerator.GenerateHostWorld"), BucketWire, VerdictLive,
		"A client builds the world its host built, from the GenerateMap packet (GameClient.handleGenerateMapPacket) -- a local game's client too.", ""},
	{sym(pkgPlayer, "miniPanel.buttonsReport"), BucketObserve, VerdictHarnessOnly,
		"Reads the mini-panel's button rects for the harness ui state, so a script clicks where they are drawn. Changes nothing.", ""},
	// J1, the journal (24 Sep 2026): the drive train from a thing he does to
	// a line in his diary, and the key that opens it.
	{sym(pkgJournal, "Load"), BucketWire, VerdictLive,
		"Validates data/strigoi/journal.json against every flag, rung, event, state and item the game can report; CreateGame refuses a bad table (loadJournal).", ""},
	{sym(pkgJournal, "Journal.Evaluate"), BucketWire, VerdictLive,
		"Writes what now holds, once a live frame (Game.journalAdvance, after earnExperience so dawn is settled). If it goes dark nothing is ever written past the first frame.", ""},
	{sym(pkgJournal, "Journal.Note"), BucketWire, VerdictLive,
		"Raises an event the conditions read: the stake, the grave, a body rising, first light, a beast met, a talk, a craft, a forage, the torch out, the watch kept or broken (Game.note).", ""},
	{sym(pkgJournal, "Journal.View"), BucketWire, VerdictLive,
		"A part's rows as the panel draws them (Game.JournalRows, from GameControls.showJournalPart).", ""},
	{sym(pkgJournal, "Journal.Save"), BucketWire, VerdictLive,
		"The sidecar's journal block (Game.journalJSON, in saveKit). Without it a reload forgets everything written.", ""},
	{sym(pkgJournal, "Journal.Restore"), BucketWire, VerdictLive,
		"Reads the block back (Game.bindJournal, deferred first in bindKit so it binds last).", ""},
	{sym(pkgScreen, "Game.sampleFight"), BucketWire, VerdictLive,
		"Notes each kind of enemy the first frame it stands in a fight with him, and a risen man before the tale -- the only source of beast:<row> and risen_seen_untold.", ""},
	{sym(pkgScreen, "Game.SetJournalOpen"), BucketWire, VerdictLive,
		"The panel holds the world while he reads (WorldHeldBy journal) and saves what he has read when he closes it.", ""},
	{sym(pkgPlayer, "GameControls.SetJournalHolder"), BucketWire, VerdictLive,
		"bindGameControls attaches the game screen's journal. Without it Q opens Diablo II's quest log.", ""},
	{sym(pkgPlayer, "GameControls.questAction"), BucketWire, VerdictLive,
		"The Q key and the mini-panel's quest button open the journal whenever one is bound, Diablo II's quest log only with no holder.", ""},
	{sym(pkgPlayer, "GameControls.journalKey"), BucketWire, VerdictLive,
		"The open journal takes every key (A3): Q or Escape close it, the arrows turn parts and rows. Reached from OnKeyDown before every verb.", ""},
	{sym(pkgPlayer, "GameControls.journalClick"), BucketWire, VerdictLive,
		"A click on the open journal picks a tab or a row, and nothing reaches the world under it. Reached from OnMouseButtonDown.", ""},
	// J2, the writings (24 Sep 2026).
	{sym(pkgJournal, "Journal.Read"), BucketWire, VerdictLive,
		"Records a writing read and reports the first read, which alone pays (Game.readWriting, from a talk answer's read effect).", ""},
	{sym(pkgScreen, "Game.readWriting"), BucketWire, VerdictLive,
		"A talk answer that reads (Effects.Read) pays the first read's experience and opens his journal at the writing (Game.Answer, once the talk has ended).", ""},
	{sym(pkgPlayer, "GameControls.OpenJournalAt"), BucketWire, VerdictLive,
		"Opens the journal at a part and a row: the writing just read to him. Refused as Q is.", ""},
	{sym(pkgScreen, "Game.whereIs"), BucketWire, VerdictLive,
		"Where a place's anchor (a villager) stands from him, for the journal's which-way-and-how-far line (Game.JournalRows).", ""},
	// J2b, searching the dead (24 Sep 2026).
	{sym(pkgScreen, "Game.Search"), BucketWire, VerdictLive,
		"U: goes through the dead man at his feet; Night 1's first two carry the amulet (R09) and the Sultan's paper (R10), read through Game.readWriting. Reached from OnKeyDown.", ""},
	{sym(pkgPlayer, "GameControls.journalViewReport"), BucketObserve, VerdictHarnessOnly,
		"Reads the open journal's tabs, rows and text for the harness ui state, so a script clicks where they are drawn. Changes nothing.", ""},
	// The polish burst (27 Sep 2026).
	{sym(pkgApp, "startGame"), BucketWire, VerdictLive,
		"Every game starts through it (App.ToCreateGame): a CreateGame that fails closes the client and sends the player to the main menu with the reason, never a nil screen (BUG-25).", ""},
	{sym(pkgAsset, "AssetManager.LevelDetails"), BucketWire, VerdictLive,
		"levels.txt's row, read under the lock the lazy load writes under: Game.Advance's region check once a second (BUG-23). If it goes dark a reader has gone back to the RecordManager's map.", ""},
	{sym(pkgPlayer, "HUD.talkHoldsTheHUD"), BucketWire, VerdictLive,
		"An open talk closes and disables the HUD's mini-panel and the run button refuses, as under the journal; the talk's end gives them back (refreshTalk, every frame).", ""},
	{sym(pkgPlayer, "handArt"), BucketWire, VerdictLive,
		"The hands' icon art drops in: data/strigoi/ui/hands/blade.png and torch.png are drawn in place of the key letters when they are there and fit (HUD.loadHands).", ""},
	{sym(pkgPlayer, "GameControls.runButtonReport"), BucketObserve, VerdictHarnessOnly,
		"Reads where the HUD's run button is drawn for the harness ui state, so a script clicks it where a player would. Changes nothing.", ""},

	// d2core/d2mapedit and d2game/d2gamescreen's Editor -- THE WORLD EDITOR
	// (M5.4, 28 Sep 2026).
	//
	// WHAT THIS BLOCK SAID BEFORE, recorded per docs/reachability.md because the
	// reason is worth keeping: on 27 Sep 2026 there were NO rows here, and the
	// note explained that the gate analyses one program (`deadcode -whylive=SYM
	// .`), that nothing in that program imported d2core/d2mapedit because the
	// editor SCREEN had not been written, and that `deadcode
	// -whylive=...d2core/d2mapedit.Open .` therefore answered "not found in
	// program" -- which Classify reads as VerdictMissing, the STALE REGISTER
	// state, which TestRegisterIsWellFormed refuses as an Expect. It listed the
	// symbols that would go on as wire in the commit that landed the screen.
	// This is that commit; the list below is that list, measured, with the five
	// the v0 screen turned out NOT to call filed as deferrals rather than
	// quietly claimed.
	{sym(pkgScreen, "CreateEditor"), BucketWire, VerdictLive,
		"Opens a .tmj in the World Editor. App.ToWorldEditor calls it from the main menu's WORLD EDITOR button and from -editor at start-up; it REFUSES a map it cannot open rather than showing an empty grid.", ""},
	{sym(pkgScreen, "Editor.OnLoad"), BucketWire, VerdictLive,
		"Builds the editor's own MapEngine, lays the document into it through d2mapgen.LayAuthoredMap and allocates the Label pool. If it goes dark the editor opens on nothing.", ""},
	{sym(pkgScreen, "Editor.Render"), BucketWire, VerdictLive,
		"Draws the map with the real MapRenderer and then the editor's chrome over it: the toolbar, the palette column, the status area, the grid, the selection outline and the placement ghost.", ""},
	{sym(pkgScreen, "Editor.Advance"), BucketWire, VerdictLive,
		"Advances the editor's engine and renderer. MapRenderer.Advance returns nothing; this is the screen's ScreenAdvanceHandler.", ""},
	{sym(pkgScreen, "Editor.OnKeyDown"), BucketWire, VerdictLive,
		"Every editor verb: Escape, Ctrl+Z, Ctrl+Y, Ctrl+S, Delete, D, G, P and Tab. If it goes dark the editor is a viewer.", ""},
	{sym(pkgScreen, "Editor.OnMouseWheel"), BucketWire, VerdictLive,
		"Zoom about the cursor over the map, and scroll over the palette. The only caller of MapRenderer.ZoomAt in the shipped game.", ""},
	{sym(pkgScreen, "Editor.OnMouseButtonDown"), BucketWire, VerdictLive,
		"Select or place on the map, pick in the palette, and start a right-drag pan.", ""},
	{sym(pkgScreen, "Editor.OnMouseMove"), BucketWire, VerdictLive,
		"Tracks the cursor for the placement ghost and drags the camera while the right button is down.", ""},
	{sym(pkgScreen, "Editor.OnMouseButtonUp"), BucketWire, VerdictLive,
		"Ends a right-drag pan.", ""},
	{sym(pkgScreen, "Editor.OnUnload"), BucketWire, VerdictLive,
		"Unbinds the editor's input handler when the screen goes. A screen that does not leaves a handler on the input manager for ever.", ""},
	{sym(pkgApp, "App.ToWorldEditor"), BucketWire, VerdictLive,
		"The one way into the editor: the main menu's button and the -editor flag both come through here, and a map that will not open goes back to the menu with the reason.", ""},
	{sym(pkgApp, "App.ToPlaytest"), BucketWire, VerdictLive,
		"Starts a real game on the map the editor has just written: d2mapgen.SetAuthoredMap, a throwaway Playtest hero in a temporary folder, and ToCreateGame at once -- no hero screen since the 28 Sep review (B3). Keeps the editor screen for the way back. Reached from the editor's P.", ""},

	// The 28 Sep 2026 review of World Editor v0 (A1-B5, C): what its fixes
	// wired. Each is reached from the editor's own verbs or from App.ToMainMenu;
	// docs/editor.md "27-28 Sep review and fixes" has the list.
	{sym(pkgApp, "App.endPlaytest"), BucketWire, VerdictLive,
		"A2/B3: hands the editor screen back when a playtest's game ends, and puts the process's map setting back to the launch map. App.ToMainMenu calls it during a playtest -- the escape menu's exit, the death screen's menu button and a game that could not start all end there.", ""},
	{sym(pkgApp, "App.playtestHero"), BucketWire, VerdictLive,
		"B3: makes the throwaway Playtest hero, with the default loadout, in a temporary folder of his own -- never the player's Saves. ToPlaytest's first call.", ""},
	{sym(pkgApp, "App.advancePlaytestCleanup"), BucketWire, VerdictLive,
		"B3: removes a finished playtest's hero folder once no game can still be writing into it. Called every frame from advanceOnce.", ""},

	// The SECOND 28 Sep 2026 review of the World Editor (B, and C x5): what its
	// fixes wired. docs/editor.md "Second review (28 Sep)" has the list.
	{sym(pkgApp, "App.advancePlaytestEnd"), BucketWire, VerdictLive,
		"C: ends a playtest whose game has gone by any way that did not end it -- the net under the death screen's \"load last save\" giving up, which left the game on the REAL main menu with the scratch map set. Called every frame from advanceOnce, after advanceReload.", ""},
	{sym(pkgApp, "App.clearStalePlaytests"), BucketWire, VerdictLive,
		"C: clears, at start-up, the throwaway playtest heroes' folders earlier games left in %TEMP% -- never one whose game is still running. App.Run's first call.", ""},
	{sym(pkgApp, "clearOwnPlaytests"), BucketWire, VerdictLive,
		"C: removes this process's own playtest hero folders as it exits. App.Run defers it (the window closing) and the console's quit calls it; the harness's strigoi_quit too.", ""},
	{sym(pkgApp, "sweepPlaytestFolders"), BucketWire, VerdictLive,
		"C: the one walk of %TEMP%'s strigoi-playtest-* folders both clears go through. If it goes dark, every playtest that ends with the game leaves its hero behind.", ""},
	{sym(pkgApp, "playtestFolderStale"), BucketWire, VerdictLive,
		"C: the start-up clear's question -- is the folder's own game still running? -- so four games sharing the laptop never take each other's playtest hero.", ""},
	{sym(pkgApp, "processAlive"), BucketWire, VerdictLive,
		"C: whether the process a playtest folder's name carries is still running (OpenProcess and its exit code on Windows). playtestFolderStale's answer.", ""},
	{sym(pkgScreen, "Editor.ReturnFromPlaytest"), BucketWire, VerdictLive,
		"A2: what the editor says when the App hands it back after a playtest. The document, its unsaved changes and its undo history were never let go.", ""},
	{sym(pkgScreen, "Editor.PlaytestNotStarted"), BucketWire, VerdictLive,
		"A2: the editor's line when the App could not make the playtest hero, before it leaves the screen.", ""},
	{sym(pkgScreen, "Editor.zoomAbout"), BucketWire, VerdictLive,
		"Sets the zoom about a screen point, clamped. The wheel handler's whole effect -- and the harness's editor zoom field calls it too, so a scripted zoom goes down the wheel's own line.", ""},
	{sym(pkgScreen, "editorPlaceLabel"), BucketWire, VerdictLive,
		"C (the second 28 Sep review): where a person's name goes -- whole, inside the map's view, clear of every other mark and name -- or nowhere. drawPeople, every frame; it replaced the fit-to-the-palette placement that cut the headman's name short.", ""},
	{sym(pkgScreen, "Editor.drawPeople"), BucketWire, VerdictLive,
		"B5: marks every person the map places and the player_start, with names, sized with the zoom. renderChrome, every frame.", ""},
	{sym(pkgScreen, "Editor.drawNotice"), BucketWire, VerdictLive,
		"C: the engine's refusal of the map, in the middle of the map area. renderChrome, every frame the engine refuses the document.", ""},
	{sym(pkgScreen, "Editor.engine"), BucketWire, VerdictLive,
		"B2: the question Ctrl+S and P put to the game -- its own parser with its own loader -- over the exact bytes about to be written.", ""},
	{sym(pkgScreen, "editorResolve"), BucketWire, VerdictLive,
		"B1's hazard: fixes the file the editor reads and writes as an ABSOLUTE path at open, and the path the game reads it by. CreateEditor's first call.", ""},
	{sym(pkgScreen, "PinnedHeroClass"), BucketWire, VerdictLive,
		"The class a new game offers; the playtest hero is made of it (App.playtestHero).", ""},
	{sym(pkgScreen, "editorProvider.HarnessSet"), BucketObserve, VerdictHarnessOnly,
		"The harness's editor zoom field: calls Editor.zoomAbout, the wheel's function, because the harness has no wheel verb. Only the harness writes a provider field.", ""},
	{sym(pkgMapEdit, "Doc.Checked"), BucketWire, VerdictLive,
		"B2: the bytes a save writes and a playtest runs, once the validator AND the engine have taken them. Doc.Save and the editor's P.", ""},
	{sym(pkgMapEdit, "EngineParse"), BucketWire, VerdictLive,
		"B2: d2maptiled.Parse as an Engine. Editor.engine builds it with the game's loader.", ""},
	{sym(pkgMapEdit, "StructureName"), BucketWire, VerdictLive,
		"C: a placed structure is named for its kind (peasant-house), as the village's own are. placeStructureCmd.Do.", ""},
	{sym(pkgMapEdit, "Stack.MarkSaved"), BucketWire, VerdictLive,
		"C: records where in the history the file on disk is. Editor.save, after a save succeeds.", ""},
	{sym(pkgMapEdit, "Stack.Dirty"), BucketWire, VerdictLive,
		"C: the unsaved mark, from the history -- undo back to the saved map and it goes out. Editor.dirty: the status line, Escape's question and the harness.", ""},

	{sym(pkgMapEdit, "OpenFile"), BucketWire, VerdictLive,
		"Reads and opens the .tmj the editor edits. CreateEditor's first call, and the editor fails rather than opening an empty screen when it errors.", ""},
	{sym(pkgMapEdit, "Open"), BucketWire, VerdictLive,
		"Parses a .tmj into a document. OpenFile's own call, so the editor reaches it on every open.", ""},
	{sym(pkgMapEdit, "DirArt"), BucketWire, VerdictLive,
		"Reads the size of the art a map refers to, out of the directory the .tmj lives in. Validate and Save both need it and both refuse a nil one.", ""},
	{sym(pkgMapEdit, "BackupPath"), BucketWire, VerdictLive,
		"Where Save keeps the generation it is about to overwrite. The editor names it in the line it shows after a save, so a designer knows what to go back to.", ""},
	{sym(pkgMapEdit, "NewStack"), BucketWire, VerdictLive,
		"The editor's undo history over one document, built in CreateEditor.", ""},
	{sym(pkgMapEdit, "Doc.Bytes"), BucketWire, VerdictLive,
		"Renders the document back to .tmj bytes. The editor re-parses those bytes with the ENGINE's own parser after every edit, which is what makes the editing view the game's view.", ""},
	{sym(pkgMapEdit, "Doc.Save"), BucketWire, VerdictLive,
		"Validates and writes the map. Ctrl+S. It refuses to write a document the game would refuse, so the editor cannot save a map that will not load.", ""},
	{sym(pkgMapEdit, "Doc.Validate"), BucketWire, VerdictLive,
		"Every refusal the loader would make, not just the first. The editor runs it after every edit -- including an edit the engine has just refused -- and puts the count and the first problem in the status area.", ""},
	{sym(pkgMapEdit, "Doc.Reachable"), BucketWire, VerdictLive,
		"The flood fill from the player_start. The editor runs it after every edit and warns, loudly, when the start can no longer reach the edge of the map: a sealed village looks perfectly right on screen.", ""},
	{sym(pkgMapEdit, "Doc.PlaceStructure"), BucketWire, VerdictLive,
		"Puts a structure on the map, its footprint anchored at its bottom corner. The palette's place verb and D (duplicate).", ""},
	{sym(pkgMapEdit, "Doc.SetFloorTile"), BucketWire, VerdictLive,
		"Writes one gid onto the floor layer. The place verb takes this arm for a floor-layer palette entry.", ""},
	{sym(pkgMapEdit, "Doc.SetWallTile"), BucketWire, VerdictLive,
		"Writes one gid onto the walls layer. The place verb's default arm -- the village stands its well and its hearth on that layer.", ""},
	{sym(pkgMapEdit, "Doc.DeleteObject"), BucketWire, VerdictLive,
		"Removes an object, keeping its whole JSON so an undo puts back everything it had. The Delete key.", ""},
	{sym(pkgMapEdit, "Stack.Do"), BucketWire, VerdictLive,
		"Runs an edit and pushes it. Every editor edit goes through here; a command that fails is not pushed.", ""},
	{sym(pkgMapEdit, "Stack.Undo"), BucketWire, VerdictLive,
		"Ctrl+Z. Measured in the running editor, 28 Sep 2026: a placed house and then 'undid place structure 17 at 29,16'.", ""},
	{sym(pkgMapEdit, "Stack.Redo"), BucketWire, VerdictLive,
		"Ctrl+Y.", ""},
	{sym(pkgMapEdit, "Doc.MoveObject"), BucketDefer, VerdictDead,
		"Moves any object in world tiles, keeping its sub-tile offset. The v0 screen selects, places, deletes and duplicates; it has no drag, so nothing calls this yet. The 27 Sep note above promised it as wire, which this burst did not earn.",
		"World Editor v1: dragging a selected object, which is the verb this exists for."},
	{sym(pkgMapEdit, "Doc.SetGroup"), BucketDefer, VerdictDead,
		"Names a set of objects that move together. v0 has no group tool.",
		"World Editor v1: groups, whose whole point is MoveGroup."},
	{sym(pkgMapEdit, "Doc.DeleteGroup"), BucketDefer, VerdictDead,
		"Forgets a group, leaving its members where they are.",
		"World Editor v1: groups."},
	{sym(pkgMapEdit, "Doc.MoveGroup"), BucketDefer, VerdictDead,
		"Shifts a whole group by whole tiles as ONE undoable edit, which is the behaviour a designer expects and the thing that is easiest to get wrong.",
		"World Editor v1: groups."},
	{sym(pkgMapEdit, "SetAside"), BucketDefer, VerdictDead,
		"Moves a .tmj that cannot be parsed out of the way, once the designer has said to. v0's CreateEditor refuses the map and goes back to the menu with the reason instead of offering the choice, so nothing calls this.",
		"World Editor v1: the unreadable-map prompt, which is the only caller this should ever have."},

	// The editor-zoom burst (27 Sep 2026): mouse wheel input and a scale on the
	// viewport.
	//
	// WHAT THESE ROWS SAID BEFORE, recorded per docs/reachability.md: every one
	// of the six below except the wheel's own read was filed `defer` against
	// "the map editor burst", measured DEAD, with a note that the brief had
	// asked for wire-with-an-empty-milestone and that a wire row claims
	// VerdictLive, which no symbol without a caller can honestly be. The World
	// Editor screen landed on 28 Sep 2026 and is that caller, so they move to
	// wire -- "which is the whole point of them being here", as the old note
	// put it.
	{sym(pkgInputEbiten, "InputService.Wheel"), BucketWire, VerdictLive,
		"The mouse wheel reaches the engine here, read once per frame by inputManager.Advance through the InputService seam. If it goes dark the engine has no wheel at all again -- it had none before the editor-zoom burst.", ""},
	{sym(pkgInput, "ScriptedInputService.Wheel"), BucketObserve, VerdictHarnessOnly,
		"The playtest overlay's pass-through for the real wheel. Only harness builds put the overlay in the path (d2app/harness.go:193 against harness_off.go:35), and it reads the wheel without scripting one, so it changes nothing.", ""},
	{sym(pkgInput, "MouseWheelEvent.ScrollX"), BucketDefer, VerdictDead,
		"How far the wheel rolled sideways. A key event cannot carry an amount, which is why the wheel is its own event. The World Editor reads ScrollY only: it zooms about the cursor and scrolls the palette, and neither is a sideways gesture.",
		"World Editor v1: a horizontal scroll across a wide map if the editor ever wants one -- and a delete row if it does not."},
	{sym(pkgInput, "MouseWheelEvent.ScrollY"), BucketWire, VerdictLive,
		"Deferred and measured dead until the World Editor landed (28 Sep 2026); recorded before the row was edited. Editor.OnMouseWheel reads the amount, takes a zoom step from its sign and calls MapRenderer.ZoomAt -- or scrolls the palette when the cursor is over the column.", ""},
	{sym(pkgMapRenderer, "Viewport.SetScale"), BucketWire, VerdictLive,
		"Deferred and measured dead until the World Editor landed (28 Sep 2026); recorded before the row was edited. Both MapRenderer.SetScale (the editor's opening fit-the-whole-map) and ZoomAt (its wheel) set it. The shipped GAME still never leaves 1.0, and the fast path that keeps 1.0 byte for byte is what makes that safe.", ""},
	{sym(pkgMapRenderer, "Viewport.Scale"), BucketWire, VerdictLive,
		"Deferred and measured dead until the World Editor landed (28 Sep 2026); recorded before the row was edited. MapRenderer.Scale reads it, and the editor reads THAT on every wheel notch, every drag-pan step (ortho = screen / scale) and every frame of the status line.", ""},
	{sym(pkgMapRenderer, "Viewport.ZoomAtScreen"), BucketWire, VerdictLive,
		"Deferred and measured dead until the World Editor landed (28 Sep 2026); recorded before the row was edited. The camera position that holds the world point under a given pixel across a change of scale. Pure: it moves nothing, which is why ZoomAt is the pair.", ""},
	{sym(pkgMapRenderer, "MapRenderer.ZoomAt"), BucketWire, VerdictLive,
		"Deferred and measured dead until the World Editor landed (28 Sep 2026); recorded before the row was edited. Wheel-zoom about the cursor in one call -- set the scale and move the camera together, the only pairing that keeps the point under the cursor still. Editor.OnMouseWheel is its caller.", ""},
	{sym(pkgMapRenderer, "MapRenderer.SetScale"), BucketWire, VerdictLive,
		"Deferred and measured dead until the World Editor landed (28 Sep 2026); recorded before the row was edited. Zoom without moving the camera. Editor.centreOnStart uses it to open on the whole map before it puts the player_start in the middle of the view.", ""},
	{sym(pkgMapRenderer, "MapRenderer.Scale"), BucketWire, VerdictLive,
		"Deferred and measured dead until the World Editor landed (28 Sep 2026); recorded before the row was edited. Reads the viewport's zoom through the renderer, which is the only handle an editor outside that package has on it.", ""},
	{sym(pkgMapRenderer, "MapRenderer.drawTileArt"), BucketWire, VerdictLive,
		"A1 (28 Sep review): draws every floor, wall and shadow scaled with the viewport. At 1.0 it is the old target.Render and nothing else -- the game's own draw, every frame; at any other scale (the editor) it pushes the zoom so the art matches its scaled anchors.", ""},
	{sym(pkgMapRenderer, "MapRenderer.tileArtScale"), BucketWire, VerdictLive,
		"C (the second 28 Sep review): the scale drawTileArt pushes at any zoom but 1.0 -- the art's far corner on the floored screen position of its own far corner, plus a pixel -- which closes the one-pixel seams the first fix left at the editor's fit zoom (449 of 84,552 holes; 0 now). Never reached at 1.0.", ""},
	{sym(pkgMapRenderer, "MapRenderer.MoveCameraBy"), BucketWire, VerdictLive,
		"Moves the camera by an ORTHO vector. The editor's right-drag pan is this and nothing else: the screen delta divided by the scale, negated, because the camera goes the other way from the hand.", ""},

	// d2core/d2mappalette -- THE EDITOR'S ASSET PALETTE.
	//
	// WHAT THESE ROWS WERE BEFORE, recorded per docs/reachability.md: all 19
	// lived in a separate PalettePending block (27 Sep 2026) and were NOT
	// measured, because nothing in the shipped program imported the package and
	// every symbol answered "not found in program", which the gate collapses to
	// `missing` -- the stale-register verdict, which would have been a false
	// sentence. That block's note set out the three dishonest options and chose
	// a fourth: write the rows now, keep them out of the measured set, and move
	// them in with the commit that lands the editor screen. This is that commit.
	// PalettePending and TestPalettePendingIsWellFormed are deleted with it.
	//
	// Eight of the nineteen are still deferrals. The v0 screen catalogues the
	// OPEN MAP's own tilesets and nothing else, so the two other readers and the
	// six whole-catalog accessors have no caller yet, and saying otherwise here
	// would be the exact hollow claim the block was written to avoid.
	{sym(pkgPalette, "NewCatalog"), BucketWire, VerdictLive,
		"An empty palette catalog. CreateEditor builds one per map it opens.", ""},
	{sym(pkgPalette, "Catalog.ReadMap"), BucketWire, VerdictLive,
		"Catalogues a .tmj's embedded tilesets: what the open map can already place, measured from the real PNGs, plus the map's own note (which the loader discards) and the distinct-kind count against the 256 cap. The editor's only reader in v0.", ""},
	{sym(pkgPalette, "Catalog.InCategory"), BucketWire, VerdictLive,
		"One tab's entries. The palette's list is this, sorted placeable-first.", ""},
	{sym(pkgPalette, "Catalog.Tabs"), BucketWire, VerdictLive,
		"The five tabs with their counts, and the reason each of the three unavailable ones is empty. The editor draws the tab, the count and the reason together, greyed.", ""},
	{sym(pkgPalette, "Catalog.KindsUsed"), BucketWire, VerdictLive,
		"How many of the loader's 256 distinct kinds the open map already spends. The editor prints it in the palette header and in the status line, because a map at the cap is refused outright.", ""},
	{sym(pkgPalette, "Entry.Placeable"), BucketWire, VerdictLive,
		"Whether the engine will accept this art as it stands: the greying-out test for one palette row, and half of the placement ghost's answer.", ""},
	{sym(pkgPalette, "Entry.Why"), BucketWire, VerdictLive,
		"One line for the user: why the art has the status it has, and if it cannot be placed, the loader's own reason. The editor puts it on the row and in the status line.", ""},
	{sym(pkgPalette, "Tabs"), BucketWire, VerdictLive,
		"The tab strip with no catalog behind it. Catalog.Tabs is built on it, so the editor reaches it on every open.", ""},
	{sym(pkgPalette, "CheckArt"), BucketWire, VerdictLive,
		"The engine's art rules, transcribed and held against the real d2maptiled.Parse by TestPaletteAgreesWithTheEngine. Entry.resolve calls it, so every catalogued tile goes through it.", ""},
	{sym(pkgPalette, "DeriveStatus"), BucketWire, VerdictLive,
		"Approved / preview / unknown from the only signals that exist -- there is no approval field in the engine -- with the sentence the editor shows. Every tile the editor catalogues gets one.", ""},
	{sym(pkgPalette, "PNGSize"), BucketWire, VerdictLive,
		"A PNG's size from its IHDR without decoding it, so opening a map costs 24 bytes a file. The editor letterboxes each palette thumbnail at the size this reports.", ""},
	{sym(pkgPalette, "HumanName"), BucketWire, VerdictLive,
		"Human words from an art path, because no asset in this project carries a title anywhere. 'Church tower', not 'placeholder-church-tower.png'. It is what the palette rows are labelled with.", ""},
	{sym(pkgPalette, "Catalog.ReadStructureDir"), BucketDefer, VerdictDead,
		"Catalogues data/strigoi/structures: art-repo renders installed for the map, footprints derived from the loader's own width formula. v0 offers only what the OPEN MAP's tileset already carries, because adding a tile to an embedded tileset is a bigger change than v0 makes and offering art it cannot place would be the misleading preview the palette exists to avoid.",
		"World Editor v1: adding a new tile to the open map's embedded tileset, which is what makes this reader's entries placeable."},
	{sym(pkgPalette, "Catalog.ReadModules"), BucketDefer, VerdictDead,
		"Catalogues a strigoi-art module manifest, which is how the Dealu monastery reaches the palette. Same wall as ReadStructureDir, plus the monastery is not installed in the game at all.",
		"World Editor v1: adding a new tile to the open map's embedded tileset."},
	{sym(pkgPalette, "Catalog.Entries"), BucketDefer, VerdictDead,
		"Every catalogued piece, in tab order. The editor draws one TAB at a time, so it asks InCategory instead.",
		"World Editor v1: a search box across the whole palette, which is the one view that wants the flat list."},
	{sym(pkgPalette, "Catalog.Placeable"), BucketDefer, VerdictDead,
		"Only what the engine will accept. The editor tests one entry at a time with Entry.Placeable as it draws the row, so it never needs the filtered list.",
		"World Editor v1: a 'hide what I cannot place' filter."},
	{sym(pkgPalette, "Catalog.Refused"), BucketDefer, VerdictDead,
		"What the engine would refuse. Same as Placeable: the editor greys a row from Entry.Placeable rather than from a list.",
		"World Editor v1: a 'what is wrong with my art' report, which is the view this list is for."},
	{sym(pkgPalette, "Catalog.ByID"), BucketDefer, VerdictDead,
		"One entry by the id the loader itself uses for a kind. v0 holds the picked Entry by value and matches the map's tileset with its own index, so it never looks one up.",
		"World Editor v1: reopening a map with the last-picked entry still selected, which is a lookup by id."},
	{sym(pkgPalette, "Catalog.KindsFree"), BucketDefer, VerdictDead,
		"MaxKinds minus KindsUsed. The editor prints used-of-max instead, so the subtraction has no caller.",
		"World Editor v1: the warning that a placement would spend the last free kind, which is what this number is for."},
	{sym(pkgPalette, "Catalog.MapNote"), BucketDefer, VerdictDead,
		"The open map's 'note' property verbatim -- the loader reads it and throws it away, so this is the only place it survives. The editor shows the STATUS the note produces, through Entry.Why, rather than the note itself.",
		"World Editor v1: a note editor, since the note is also where the editor's own group data lives."},
}

// RegisterMarkdown renders the register as a table, so the register lives in
// one place -- here, next to the gate that enforces it -- and the document is
// generated rather than typed. A count in prose is a count that goes stale.
//
// The PalettePending block that used to sit here -- the editor palette's 19
// rows, written with the package and deliberately kept out of the measured set
// because nothing in the shipped program imported them -- was folded into
// Register on 28 Sep 2026, when the World Editor screen became that importer.
// Its reasoning is kept above the palette rows themselves.
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
