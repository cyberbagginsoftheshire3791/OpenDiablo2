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
		"T4: a watch promised to the headman is counted on the night-to-dawn edge he lived through (earnExperience).", ""},
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
		"L. One key: light, relight or douse, decided before anything is spent so that a refused commit changes nothing. Reached from OnKeyDown.", ""},
	{sym(pkgPlayer, "GameControls.combatEndTurn"), BucketWire, VerdictLive,
		"E. Ends the turn -- hold when the Action is unspent, end when it is spent. Reached from OnKeyDown. Without it a turn with an unspent Move waits forever.", ""},
	{sym(pkgScreen, "Game.worldRunning"), BucketWire, VerdictLive,
		"The one boolean that stops the world for an open turn, keeping BOTH of its original terms (menu closed OR not single-player) and adding !Awaiting. advanceWorld is gated on it; MapEngine.Advance deliberately is NOT, so sprites animate and in-flight walks finish while a person thinks.", ""},
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
		"Reports the creature name, Strigoi animation mode, direction and movement for get_entity. It is read-only and exists so the same observer can inspect inherited NPCs and project-owned creatures.", ""},
	{sym(pkgBestiary, "Load"), BucketWire, VerdictLive,
		"Loads and validates the shipped project-owned creature catalog when a game screen is constructed; an invalid bestiary prevents a half-authored game from starting.", ""},
	{sym(pkgBestiary, "Catalog.ByID"), BucketWire, VerdictLive,
		"Resolves player and art-review spawnmon commands through the same authored creature definition natural spawns use.", ""},
	{sym(pkgBestiary, "Catalog.ForSpawnRow"), BucketWire, VerdictLive,
		"Maps an authored spawn-table row to project art, its inherited stats stand-in and Strigoi health in a shipped game.", ""},
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
	{sym(pkgWorld, "Combat.Order"), BucketDefer, VerdictDead,
		"The activation sequence -- D8 section 9's since step 4. Reported and asserted by playtests; still read by nothing in Go. c-2a did NOT give it a caller: the strike key commits an empty target and Combat.Commit resolves D8 order internally, so the turn UI that DISPLAYS an order is what will want this.", "M4.4c-2b (the squad strip) or the first order readout"},

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
