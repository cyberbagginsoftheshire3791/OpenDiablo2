# Focus -- printed into every session by the SessionStart hook

Updated: 2026-09-01 (00:15 CT). Keep this to a screen; the full state
lives in `state.md` in the claude.ai project and in Notion.

**THREE SAVED SKILLS CARRY THIS PROJECT'S PROCEDURES. INVOKE THEM; DO NOT
RE-DERIVE THEM** (Josh saved them 3 Sep 2026). They live on Josh's
claude.ai account, so they are available on every surface -- Cowork,
Claude Code, chat -- and they are deliberately NOT copied into
`.claude/skills/`: two writable homes for one procedure drift, and the
stale one is what a cold session reads first. The spine of a burst, in
order:

1. `brief-then-attack` -- BEFORE any milestone step, research topic or
   finding is built or filed. One session writes it; a second,
   independent agent tries to break it. Four A-severity findings across
   three M4.5 steps, and C3's lead pass read its load-bearing sentence
   backwards.
2. `strigoi-measure-first` -- at the START of a build burst whose design
   depends on engine behaviour, before code or assertions. Throwaway
   playtest scaffold, deleted before the commit. Step 4's section 0
   changed THREE design decisions before they cost anything.
   **A measurement that contradicts the brief beats the brief.**
3. `strigoi-burst-closeout` -- gate, the FULL reachability register,
   every playtest script, negative controls, commit, CI, Notion, the
   tracker, state.md, the handoff.

They compose and do not substitute: closeout is not the gate, and the
brief is not section 0. **If a skill is not in the session's skill list,
say so** rather than improvising a shortened version of it.

**THIS FILE IS ONE STEP BEHIND: M4.5 STEP 4 SHIPPED 3 SEP (`040eea70`)**
and the body below still stops at step 3 -- the exact disease the next
paragraph warns about. The resolver, the player's body through
`Game.BodyOf`, D8 section 9's order and the thirteenth playtest are all in
the tree; read `claude/m4.5-step4-build-note.md`. Refreshing this file
properly is its own small burst.

**SINCE 28 AUG, AND THIS FILE HAD NOT SAID SO -- five heads landed while the
body of this document still read as current, which is the exact disease it
warns about further down.** In order: `68605cb2` THE SPAWN STALL FIXED
(daybreak sends home the packs that have not noticed you; groups were
hard-capped at 8 and nothing ever despawned, so the night stopped happening
after the first eight packs) - `dd2b7d99` `tools/mapfirecount` (ask 1 of the
map-fire note: the village places about TWENTY-ONE lit objects, and NOT ONE
is lit in the mode it is placed in) - `deadf8a8` A3 (the playtest scripts'
fail-open bool reads now fail closed; `num`/`str`/`pair` are still fail-open
at 177 sites and that is named, not fixed) - `84018f5a` + `502e4cef` M4.5
STEPS 1 AND 2 (the encounter model and its provider, built BEFORE the
resolver on purpose) - and this burst, M4.5 STEP 3.

**M4.5 IS WHOLE, WITH NAMED STEPS** (Josh, 31 Aug). Step 3 = the NPC body;
step 4 = the resolver (`band`, `advantage`-and-why, damage); step 5 = rout,
quick-resolve, `Pursuit.Release`'s first game caller, the eight-clause
playtest. The eight-clause DoD in the signed note's section 4.1 is the only
"done"; each step closes with a decision row.

**M4.5 STEP 3 IN ONE PARAGRAPH.** A monster had no health, no morale, no
alive flag and three usable animation modes; the player was a body and an NPC
was a sprite on a path. Now `npcBody` in `d2gamescreen` satisfies the same
`d2world.Body` the meters declared, health lives on an adapter the screen
owns (NOT on the entity - R2 section 1 signs no health bar, and
`d2mapentity` is engine-inherited code), and `Combat` reads it through a
two-method `Bodies` lookup. **HP is `MaxHPNormal` with NO ROLL**: a new RNG
draw would move every seeded measurement, which is what the worldgen fix cost
in August. `NPC.SetAnimationMode` is the first exported way to tell a monster
to do anything. **`tools/animcensus` measured that the three stand-in codes
all have A1, A2, GH, DT and DD** (fallen1 21-61, zombie1 101-181, skeleton1
86-129), so the resolver will find animations rather than errors. **The one
thing to know before step 4: `npc.go`'s `next()`/`rotate()` reset the mode
from the NPC's own state every tick, so an externally set A1 does not hold -
a held-mode path is step 4's to build.**

**THE PLAN IS v1.6 (28 Aug).** v1.5 corrected the plan's stale status;
v1.6 changes the SEQUENCE. Phase 4 now states an explicit build order,
M4.5 comes before M4.4, the corpse machine is named M4.7 and built after
M4.5, and M4.4 absorbs the meter HUD. **The M4.x numbers are identities,
not a schedule** -- the build-order line is the schedule.
Lesson still standing from v1.5: **a stable document that also carries
status has two writable homes for one truth, and the stale one is what a
cold session reads first.**

**PHASE 3 IS DONE** (M3.1-M3.4, DoD audit passed 26 Aug on 2c1b2124; the
harness has 33 tools, SEVEN playtest scripts, docs/harness.md v1 with a leak
register). **M4.1 "MAKE NIGHT REAL" IS DONE** -- the clock, the light model,
the renderer, and (27 Aug) placed sources, which reopened it. **M4.2
"SURVIVAL METERS v0" IS DONE** (27 Aug, build-shape note signed first).

**M4.2 in one paragraph.** d2core/d2world gains Meters beside Clock and
Light: Food and Water run 100 (full) to 0 (empty), **FATIGUE RUNS THE OTHER
WAY**, 0 (rested) to 100 (exhausted), because S1 5's thresholds read "fatigue
>= 75%" and "food = 0". Daylight costs more water, hunger tires you faster,
labour and the night watch cost more. ReactionAvailable (R2 1) and Shaken
(R2 3, its threshold lowered by thirst) are computed HERE so M4.5's resolver
reads the body's fact instead of recomputing it. Neglect spends health per
world hour per empty meter through a d2world.Body interface the game screen
satisfies -- playerBody in d2gamescreen is the first thing in this codebase to
write Stats.Health. **THE NOTE FOUND A SCOPE FAILURE BEFORE IT WAS BUILT:**
S1 5's signed assertion is three clauses and only ONE was buildable (no
combat state machine until M4.5; nothing wrote Stats.Health; no player death
or death screen; no item-consumption verb). The signed split gives M4.2
clause 1 whole, the METERS half of clause 2, and health-to-death without the
screen (M4.6's, per S1 6.5 and R2 3) -- every deferral named in the DoD.
NOT in M4.2, by signature: a meter HUD (wanted -- Josh confirmed -- home
undecided between M4.4 and its own milestone), inventory, combat use.

**The reopening, closed.** Josh saw the torch screenshot and said the light
came from the player; it did, structurally -- Light.Add always took a
position, but the only non-test caller was HarnessSet("carried_source"), which
hardcodes carried=true at the player, so source_list could never hold anything
but the player's torch. `light.place_source` is the verb that fills it: a
settable field whose value is an object, {"kind","x","y"} in world tiles, a
field rather than a tool because the three provider tools are the only ones
that know about providers and do not change as systems are added.
`light.remove_source` (value: a source id) is its other half -- Light.Remove
had been an exported method with no non-test caller, the same unreachable
shape one step earlier, and a fuel-fed hearth never burns down, so it is the
only way to show the dark closing back in around a placed light. The provider
now also reports `player_level` and per-source `level_here`, because `radius`
is player-centric and a light the player stands outside of moved nothing it
reported. Two rules to carry into M4.2+: **a provider that reports a
collection needs a verb that can put something in it**, and **a provider read
per-position must report it at the positions its assertion names.**
Lighting the MAP's own fires is now SCOPED but NOT decided -- see the
map-fire scoping note near the end of this file; still no engine code.

`d2core/d2world` is the world's own package -- plain arithmetic over the
delta the game screen already gets, no wall clock, no renderer.
- **Clock**: world minutes since 17 June 1462 dawn; Julian date + weekday
  (pinned by tests to 29 May 1453 = Tuesday and 17 Jun 1462 = Thursday, so
  the run's Saturday and Tuesday can never drift); stage dawn/day/dusk/night;
  compression by stage (day 4 world-min/s, night 2.5 -- the night dilated);
  moon thinning each night; D7's hearth-freeze flag (harness-only for now).
  **Stepped, never set** (P3 4.5) -- HarnessSet refuses the time.
- **Light**: ambient falls to a moon-set floor; sources restore a radius and
  burn per world minute; torch 5 tiles / 60 world-min, hearth 8 tiles
  fuel-fed, floor 1.5 tiles. Level() quantised into 16 bands for the eye;
  Radius() continuous so the dials stay exact. ONE source of truth -- M4.5's
  combat resolver and the renderer read these same values (S1 4).
- **The renderer dims to it.** All four map passes wrap their per-tile body
  in `PushBrightness(level)` / `Pop()`, fed by `d2maprenderer.LightSampler`
  (one method, `Level(x, y int) float64`) set on the MapRenderer -- so
  d2maprenderer imports no world code and links no ebiten. **No sampler set
  = 1.0 = the pre-M4.1 renderer, pixel for pixel.**
- Both are harness providers (`clock`, `light`); step_world{world_minutes}
  works (harness 0.6.0); `night_light_test.go` runs S1 4's assertion verbatim,
  `night_render_test.go` measures it off four screenshots (night 88% dimmer
  than noon; unlit night falls uniformly near x0.119 / far x0.118; a carried
  torch breaks that uniformity near x8.35 / far x1.63) and
  `night_placed_test.go` proves a PLACED hearth nine tiles west lights its own
  ground x6.65 while the player's stays at x1.00 (model: hearth tile 1.000,
  player tile 0.125, radius 1.5), then puts it out and watches the dark close
  back in (23.1 -> 3.5 of 255, the unlit night was 3.5). **Nine tiles, not three: the hearth's radius
  is 8, so anything closer engulfs the player -- and no placement puts a
  source's own tile on screen while leaving the player's ground dark, since 5
  tiles already spans the viewport.** Determinism proved across two launches
  on 27 Aug, now 5dc478f8b7c2 / 8b22345a3241 / c6e9b2317361. They MOVED TWICE
  today -- when the light provider gained player_level/level_here, and again
  when the meters provider registered -- because the digest's `systems` part
  hashes what providers REPORT. (remove_source did NOT move it: a settable
  field is capability, not state.) Digests are build-specific by design; the
  proof is that two launches agree, not that a value matches yesterday's.
- **`meters_test.go`** measures M4.2, seed 1462: over 4.01 night hours food
  87.98 / water 81.98 / fatigue 14.02, each on its dial; eating and sleeping
  move them back; Reaction lost at 75, Shaken at 90 and at 80 when thirsty;
  neglect took health 166 -> 156 in 2.54 starving+parched hours and ran the
  body to 0, where the meters stop. **Trap it found: getting to a deep night
  costs ~43 world hours, which empties every meter (water goes in 22 hours,
  food in 33) -- any script that walks to nightfall must top the body up
  before measuring a drain, and must measure against the clock's own elapsed
  minutes rather than the hours it asked step_world for.**

**M4.3a AND M4.3b ARE BOTH DONE. NEXT IS M4.5 -- COMBAT v0 -- AHEAD OF
M4.4, BY JOSH'S CALL, 28 AUG (decision 3caff9f3-d21e-8153-b34c-dfe6f018f47f;
the plan is now v1.6).**

**THE REASON IN ONE LINE: the chain is spawn -> notice -> approach ->
FIGHT, and only the last link is missing.** Something comes out of the
woods, works out that it can see you, routes around the palisade to reach
you -- and stands there. **You cannot lose.** Until that changes,
everything else in Phase 4 is presentation or content.

**M4.5 IS CHEAPER THAN IT LOOKS BECAUSE IT WAS BUILT TOWARD.** It READS
rather than recomputes: meters.reaction_available and meters.shaken
(M4.2), spawns.morale and spawns.routing (M4.3b), and light.Level(x,y)
(M4.1, already carrying R2 section 3's dark-into-light advantage). Three
milestones each named this one as the consumer of a value they exposed
but did not act on. R2 is the signed ruleset and its section 5 open-dials
list is the scope. **D8 (combat initiation and initiative order) is an
OPEN Research Topic, so initiative is unspecified rather than decided** --
R2 section 1 signed the GROUPED half of BG3's model (pack activation) and
refused the bonus-action economy.

**THE BUILD ORDER IS NOW EXPLICIT AND THE M4.x NUMBERS ARE IDENTITIES,
NOT A SCHEDULE:**

    M4.1 -> M4.2 -> M4.3a -> M4.3b -> M4.5 -> M4.7 -> M4.6
    with M4.4 floating -- slot it whenever.

**M4.7 IS NEW: the corpse machine finally has a name and a place**, after
M4.5, so a risen that paths toward the camp arrives into a night that can
resolve the encounter instead of reproducing the diorama shape the M4.3
split existed to avoid. It carries the corpse state machine, the per-band
rising roll against soul pressure, the edge-arrival floor and the rite
window; it shares content with M4.6; and it drives spawns.open_bodies,
which M4.3b built as a settable stand-in for exactly this.
**M4.4 IS NOW THE HUD MILESTONE** -- clock UI plus the meter HUD plus the
first scripted event -- and it floats. That closes the last open item
from M4.2's signature.

**TWO SHORT BURSTS ARE APPROVED OUTSIDE THE BUILD ORDER.**
(a) **The worldgen default branch** (3caff9f3-d21e-81a2-a0a7-edd4dcf0578c):
the wilderness generates only for the E1/S1/W1 town presets, so an
unseeded launch produced a village in a void. Seeded, so not a
determinism bug -- but Phase 4's DoD is a FRIEND cloning and surviving a
night, and a friend will not pass seed 1462. **READ THE BRANCH BEFORE
PROMISING THE FIX IS SMALL**: the default case may exist because those
presets have no wilderness stamp to place.
(b) **The map-fire counting pass** (3caff9f3-d21e-8102-a210-e69708c2e55c):
about an hour, changes no lighting, and reports how many placed objects
declare a light, whether Flicker separates fire from magic, and the
per-frame source count -- **that last number decides the design**, because
Light.Level loops EVERY source and the renderer samples it per tile across
four passes. Unblocks the map-fire note's asks 3, 4 and 5.

**M4.3b closed 28 Aug** (decision 3caff9f3-d21e-81f4-bdb0-fad1556d7ac5)
in five commits: fcd592c6 the notice model · 0f3cda8d the spawn tables ·
5a61b95a the wiring · 943ee278 the units fix + strigoi_watch + the ninth
script · ecba7943 docs. **Harness 0.9.0, 36 tools, SIX providers (clock,
light, meters, pursuit, spawns, ui), NINE playtest scripts.** Determinism
re-proved at 6a697780ce29 / f56f3f0edda2 / 7a7e9a004dff.

**MEASURED, seed 1462:** a watcher six tiles away with a clear line SEES
the player at 5.94 tiles; one behind cover at 6.00 tiles DOES NOT -- same
map, same tick, same radius, only the line differs, and that pairing is
the milestone's whole assertion. The memory window held a watcher coming
1.6 world-minutes after it lost sight. Light 1.000 took reach 3.0 -> 6.0
AND THE VERDICT WITH IT. Dawn -> night band 0 took the wolves row 0.000
-> 0.500. Four open bodies lifted the carrion weight and the beast rows
with it. A forced arrival routed at morale 5 and stopped routing at 90.

**AND IT CAUGHT AN M4.3a BUG. READ THIS BEFORE TRUSTING ANY M4.3a
MEASUREMENT.** chaser.HunterAt and prey.QuarryAt divided GetPositionF by
the subtile factor -- but **GetPositionF ALREADY RETURNS WORLD TILES**
(it is Position.World(); its own comment says "0.2 is one sub tile").
Every coordinate d2world saw from M4.3a onward was a fifth of the truth,
and because mapRouter.Route takes world tiles this did not shrink routes,
it **routed between the WRONG POINTS**: a hunter on tile 35.2 asked for a
path from tile 7.04 toward a player reported on tile 6.2 and walked at
the map's corner, while the reported distance went DOWN and the real gap
went UP. **So M4.3a's "a fallen closed to 2.80 tiles" was false in both
size and direction, and ArriveWithin 1.0 meant five tiles.** Fixed in
943ee278; the dials keep their signed values because those values were
chosen as TILES and only now mean it.
**Why it survived sign-off, and the rule that comes out of it:** every
consumer divided consistently, so unit tests on fakes agreed (a fake
returns whatever units the test puts in) and the old playtest agreed (it
asserted the distance SHRANK, which it did). **At least one assertion per
system must compare a number the TEST chose against a number the SYSTEM
reported** -- place a thing six tiles away and assert it reports about
six. Relationship assertions are invariant under a scale error and always
will be. It is now an explicit act in playtest/spawns_test.go.
**UNITS, once, plainly:** GetPositionF = world tiles. GetPosition =
floored tile. NewNPC and the entity factory = SUBTILES.

**THE SIGNED SHAPE M4.3b BUILT TO.** Scope is option (a): the two stage tables + the [3]
deep-night bands (computed in the SPAWNS system, not the clock) + the morale
STATE (`morale`/`routing`, read by M4.5's resolver -- no rout BEHAVIOUR here)
+ `open_bodies` reported and harness-settable + the notice model. **The corpse
state machine, and therefore the dead arriving, is split into its own
milestone shared with M4.6.**
**THE FENCE (ask 5, the hardest to move later): the notice model lives in
d2core/d2world beside Pursuit, behind a one-method sight interface, and may
know LINE OF SIGHT, DISTANCE and THE LIGHT LEVEL AT THE QUARRY -- nothing
else.** Not the meters, not the inventory, not reputation. That fence is what
makes the torch the trade: light against being seen.
**CONSIDERED AND NOT TAKEN:** adding the NOISE value. S1 section 4 already
signs "one noise value per action that beasts and humans react to", so a wolf
hearing you dig is signed design rather than new -- Josh took the tighter
fence, which leaves that clause with a consumer in the TABLE WEIGHTS but none
in the NOTICE MODEL. Recorded case against; additive to a one-method
interface if playtest says otherwise. **Do not quietly add it.**
**ASK 6 IS THE ONE TO HONOUR LITERALLY:** the provider reports a per-group
`notice` block (`sees`, `distance`, `light_at_quarry`, `noticed`) drivable
BOTH directions, because a chase that starts is NOT evidence that noticing
works -- a chase can start for the wrong reason and look identical. It exists
only because M4.3a's section 3.2 was signed with an assertion nothing could
write. The provider rule, fifth costume, caught before the build.
**ASK 7:** every rate in world minutes, but CHOSEN against DAY compression
with its frame cost written beside it, and each with a reported counter the
way Pursuit.solves is.
**THE NOTICE DIALS ARE INVENTED** -- N1 says nothing about detection radii,
and only the light threshold has a signed parent (R2 section 3). **But the
12-tile beast radius is DELIBERATE, ruled on by Josh 28 Aug after it was
flagged twice: it exceeds what is on screen (~5 tiles each way) ON PURPOSE,
so a wolf can decide to come for you while off-camera. "12 tile aggro will
keep it scary." DO NOT TUNE IT DOWN as an oversight** -- if a playtest makes
it look unfair, report the measurement and let Josh re-rule.

M4.3a closed 28 Aug (decision 3caff9f3-d21e-8102-aec3-c35f401ec6c8) in four
commits: cafe22f6 the three accessor bugs · f894e5df the A* · 309193a8
strigoi_find_path + the eighth playtest script · eea45074 pursuit.
**Harness 0.8.0, 35 tools, FIVE providers (clock, light, meters, pursuit,
ui), EIGHT playtest scripts, all passing.**

**THE MILESTONE IN ONE LINE:** MapEngine.PathFind was one line-of-sight
raycast returning a single point, so nothing in the world could route around
anything. It is now a bounded, deterministic A* over the subtile grid, and
d2core/d2world.Pursuit keeps a chase honest while its quarry moves.

**MEASURED, seed 1462:** a goal twelve tiles east reports
straight_line_clear=false and reachable=true in five waypoints, and the
player ARRIVED after 420 stepped ticks -- twelve tiles it could not
previously cross. The same query twice is byte-identical. A goal 400 tiles
off the map reports unreachable and still returns nine partial waypoints
toward it. A fallen spawned six tiles away chased the player through an
eight-tile move and closed to 2.80 tiles, re-pathing as it went. Determinism
re-proved at 13f523c11aad / 1ff9e341378e / b614542beb53.

**THREE CORRECTIONS TO THE SIGNED NOTE, all mine -- read them before writing
M4.3b's note, because two are about how a build-shape note goes wrong.**
1. Section 3.5's blast-radius prediction was WRONG: town_walk_test.go and
   determinism_test.go did NOT need rewriting. They were written to TOLERATE
   a blocked direction, not to REQUIRE one.
2. Section 3.2's own signed assertion was NOT WRITABLE when it was signed,
   because nothing reported a path -- the provider rule a FOURTH time: an
   assertion that names a path needs a provider that reports the path.
   strigoi_find_path is the fix; its straight_line_clear is the negative
   control, because a route that arrives proves nothing unless the straight
   line did not.
3. The pursuit dials were wrong in a way only the running game could show:
   MinRepathMinutes 0.25 against a COMPRESSING clock meant 218 route solves
   across 600 stepped frames. Fixed to 2.0 plus a ProgressTiles rule; 2
   solves afterward. Lesson filed.

**NOT DONE, NAMED:** a pursuer is not verified to end up ADJACENT to its
quarry. It closes distance and re-paths correctly, but the last couple of
tiles are a melee-range question and belong to M4.5. Awareness -- WHEN a
chase starts -- is M4.3b's by signature.

**PRIOR (the first half, for context):**
- `cafe22f6` -- the three accessor bugs, each with a unit test on a
  hand-built grid. tileCoordinateToIndex returns -1 off the map instead of
  wrapping x onto the neighbouring row; TileExists bounds half-open;
  SubTileAt floors its division (new floorDivMod) and returns nil rather
  than dereferencing a nil tile; checkLos treats off-map as blocked.
  **PROVEN BY REVERTING ALL THREE:** the tests then fail with exactly
  "index out of range [16] with length 16", "index out of range [-1]", and
  a nil pointer dereference.
- `f894e5df` -- **a real A\* replaces the raycast** (d2mapengine/astar.go).
  Subtile resolution; queue on the total key (f, h, y, x); fixed compass
  neighbour order; no map iteration anywhere; INTEGER step costs 10/14 so
  nothing floating-point can reach the digest; bounded at 4000 expanded
  nodes [DIAL] with a best-partial fallback; no corner cutting; routes
  collapsed to corners, final waypoint the caller's own dest.
  **Measured in the running game at seed 1462, and the digests moved as
  named in advance:** A after load UNCHANGED 5dc478f8b7c2; B after the walk
  8b22345a3241 -> 52c1559da938; C after 600 idle ticks c6e9b2317361 ->
  9931ffb10d91 -- all three agreeing across two separate launches.

**WHAT M4.3b OWNS.** The spawn tables (hostile arrivals keyed to the clock)
PLUS the notice/awareness model, which moved into it when M4.3a took pursuit
only. Nothing in the R-track gates it: N1 is Verified and closes the beast
content; E3 gates only the human row's kit; M11 and C3 do NOT gate it.
Pursuit gives it the seam it needs -- `strigoi_pursue` and `Game.Pursue`
start a chase; M4.3b decides WHEN.
**THE FINDING THAT ARRIVED WITH THE SIGNATURE, and it changes what this
milestone is FOR.** Josh asked how much of a priority it really is that you
cannot walk out of the village. Read from the repo rather than assumed:
**THE LAND OUTSIDE THE VILLAGE ALREADY EXISTS IN THE RUNNING BUILD.** There
is no missing level and no missing transition -- the game server builds
exactly ONE map engine (d2networking/d2server/game_server.go:106-115) and
never builds another, and the LevelWarp records are decoded by d2records and
then read by NOTHING (grep LevelWarp outside d2records/d2resource: zero
hits). It needs none, because MapGenerator.GenerateAct1Overworld
(d2core/d2map/d2mapgen/act1_overworld.go:37) resets the map to 150x150 and,
when the town stamp's region path contains E1, S1 or W1, places the town and
then generates Blood Moor wilderness beside it ON THE SAME CONTIGUOUS GRID.
**At seed 1462 -- the seed every playtest run uses -- the game log reads
"Region Path: Act1/Town/TownE1.ds1"**, so that branch runs: town at (0,0),
wilderness east, player at tile (31,14) with generated ground a short walk
away. It is already there and simply UNREACHABLE, because the raycast
returns the last walkable point before the first fence. **So M4.3a is not
mainly about wolves: it is the only thing between the player and terrain
this build already generates.**
**A SEPARATE DEFECT FROM THE SAME READ -- filed, not fixed, NOT in M4.3a's
scope, and it needs Josh's call:** the wilderness generates only for the E1,
S1 and W1 town presets. Every other preset falls to the switch's default
branch, which places the town and generates NOTHING around it. An unseeded
launch (wall-clock seed 1787854851626238800) drew TownN1 and produced a
village in a void. The preset pick IS seeded, so this is not a determinism
bug -- but WHETHER A WORLD EXISTS OUTSIDE THE PALISADE IS CURRENTLY A
PROPERTY OF THE SEED. That is worldgen content, not pathfinding.
**WHY M4.3 SPLIT (decision 3c9ff9f3-d21e-813d-afdd-d24d48b408f8):** told that
M4.3 as scoped would ship spawn tables with nothing that moves, Josh said "we
dont need a diorama we are trying to make a game that works", and chasing that
objection found a hole in the plan. A working night is spawn -> notice ->
approach -> fight; plan v1.4 5 owns only spawn (M4.3) and fight (M4.5);
nobody owned notice or approach, and **THE ENGINE CANNOT DO APPROACH.**
`MapEngine.PathFind` (d2core/d2map/d2mapengine/pathfind.go) is NOT a
pathfinder -- it is one line-of-sight raycast returning ONE point, the
destination if nothing blocks, else the last walkable point before the first
BlockWalk. No A*, no route around anything. The repo already knew and had
filed it as normal: harness_tools.go:657 calls "stuck" a normal outcome,
town_walk_test.go:64 tries directions until one moves the player, and the
determinism proof's "stuck at 33.80,14.00" IS that raycast meeting a fence.
It is load-bearing beyond wolves -- S1 12's M4.6 assertion ("paths toward
the camp"), R2 3 ("the dead pursue"), S1 9.1's palisaded village -- and it
is a works-TODAY defect: clicking across town gets stuck at the first fence.
**THREE LATENT BUGS in the same code, to be FIXED in M4.3a with unit tests:**
`SubTileAt` dereferences a possibly-nil tile (engine.go:212 -- an off-map move
target can panic today); `TileExists` is off by one (engine.go:315, index ==
length, the DT1 panic's signature, and strigoi_spawn_entity inherits it);
`tileCoordinateToIndex` does not bound x (engine.go:207, so x = -1 aliases
onto the previous row). That is the OPPOSITE call to parking addDT1, from the
same principle: d2mapengine links 0 ebiten, has tests, and its grid is
constructible with no MPQs, where addDT1 had no seam. **NAMED CONSEQUENCE:**
replacing PathFind changes PLAYER movement, so the digests move again and
town_walk_test.go and determinism_test.go -- which encode "stuck" as the
expected outcome -- get rewritten.
**THEN M4.3b, night spawns** (hostile tables keyed to the clock). Its note is
now v1.1 (28 Aug): it is the SECOND half, and notice/awareness is in it
because M4.3a ships pursuit only. **THE COMPRESSION RUNS OPPOSITE TO
INTUITION AND M4.3b SHIPS AT LEAST THREE RATES:** DayRate 4.0 / NightRate 2.5
(d2core/d2world/clock.go:98-99), so a world-minute-denominated rate is
TIGHTEST IN DAYLIGHT, not at night -- at the harness default tick of 1/60 s,
one world minute is **15 stepped frames by day and 24 at night**. The note
wrote its own fence against this backwards on the first pass and caught it
against the code: the 218-solve unit error, one costume on. Write every rate
down with its frame cost beside it.
**THREE M4.3b SEAMS M4.3a LEFT NAMED IN THE CODE:** pursuit.go:15 ("it is
deliberately NOT awareness"), pathfind.go:87-98 (`LineOfSight` -- "whether a
thing can SEE you is not whether it can WALK to you"; the sight test already
exists and is exported), and game.go:499 (`Game.Pursue` -- "the seam the
harness, and at M4.3b the awareness model, uses to start a chase"). M4.3b
supplies the DECISION, not the mechanism.
**VERIFIED BLAST RADIUS, read not predicted:** playtest/ui_inventory_test.go:51
pins the ABSENCE of the spawns system and WILL fail the moment a spawns
provider registers -- point it at "dead" (one word; the substring check still
passes). harness_providers.go:29 must lose its "spawns" row. The digest's
`systems` part WILL move by construction. Whether `entities` also moves is a
MEASUREMENT, not a prediction.
**THE RESEARCH GATE, checked 27 Aug against Notion and NOT what an earlier
note in this file said:** M4.3's BEAST table is UNBLOCKED -- N1 is Verified
and its 5 explicitly closes S1 6.4's beast content (feral dogs, wolves, boar,
bear, lynx-as-atmosphere; carrion- and noise-driven; every quantity a [DIAL]
"the M4.3 build sets"). E3 (arms) is In progress and gates only the HUMAN
row's kit. H4 (Radu's men) is Campaign priority and S1 3.3 has the riders as
rumor-level presence anyway. **M11 and C3 do NOT gate M4.3** -- S1 12's
"Must close before" column reads "M4.6 placeholder is fine; Phase 6 needs
M11", and the M11/C3 pairing comes from S1 12's research-ORDER line, which
is labelled "for the R-track lane, not a decision" and puts them in the
PHASE-6-WRITING group. The Phase-4 group is D4, E2, N1 (+E6) -- all four
signed. Inherited rules, unchanged: register a provider at construction, ship a
playtest script, stay inside the digest, build on the stepped clock. THREE
PROVIDER RULES now, all earned the hard way: (1) a provider that reports a
COLLECTION needs a verb that can put something in it -- and one that can take
it back out; (2) a provider read PER POSITION must report it at the positions
its assertion names; (3) a provider that reports a VALUE needs a verb that can
move it in BOTH directions, or a script can only ever watch the number fall.
Two more habits: read the engine BEFORE writing the build note (M4.2's found
a scope failure that would have cost a reopening), and check that a signed
assertion is actually buildable clause by clause.

**THE MAP-FIRE SCOPING NOTE'S ASK 2 IS ANSWERED (28 Aug): a map fire is A
HEARTH SOMEONE ELSE LIT** -- radius 8, Burn negative, no fuel cost, no new
category (decision 3caff9f3-d21e-8122-a038-c2c651f45768). The note's own
argument for the opposite, that most of those drawn fires should be OUT
because a countryside Vlad scorched should not glow, was read and NOT taken;
it is preserved as the recorded case against. **ASKS 1, 3, 4 AND 5 ARE STILL
OPEN and no engine code exists or may be written until 1 and 4 are answered.**
"Map Fires - Scoping Note.md". **Its headline: D2's object table ALREADY
declares which objects emit light** -- LightDiameter [8]int, LightRed/Green/
Blue, Flicker bool, BlocksLight [8]bool, all **PER ANIMATION MODE**, all read
by object_details_loader.go:72/82/146-148/196 and consumed by NOTHING outside
d2core/d2records. The map walk already exists (d2mapstamp/stamp.go:130-149)
and a map fire is structurally a hearth with Burn negative. **So the unit of
any fire list is (object id, MODE), not id** -- a lit and an unlit brazier are
usually one id in two modes. `Light.Level(x,y)` loops EVERY source and the
renderer samples it per tile across four passes, so the placed-source COUNT
decides the design, and it is UNKNOWN (objects.txt lives in the MPQs).
**Ask 1 is the cheap one and blocks nothing: a counting pass that reports how
many placed objects declare a light, how many also flicker, and what the
per-frame source count would be -- changing no lighting behaviour.**
Still Josh's call on timing: where the meter HUD lands.

Standing findings: the black floor is INTERMITTENT per launch with the cache
provably colored; fix stays parked (P3 5.3) -- and from here on a NIGHT
screenshot is useless as black-floor evidence (town_walk samples a daylight
frame, which still tells them apart). **New and intermittent, filed not
fixed, and it has TWO faces:** (a) 27 Aug, 1 full-suite run in 2 -- a nil
dereference in `d2mapengine.addDT1` (engine.go:107) from ResetMap during
start_game. One line causes it: addDT1 logs LoadDT1's error and then FALLS
THROUGH to `dt1.Tiles` instead of returning, so a failed load kills the
process rather than leaving a tileset missing. Downstream of the asset-cache
defect below. **DO NOT "just add the return" -- Josh parked it 27 Aug on two
counts: MapEngine.asset is a CONCRETE *d2asset.AssetManager (engine.go:26), so
there is no seam to test the error path without extracting an interface or
using real MPQs (Article V bars that from CI) -- a behaviour change with no
test, against VI.1; and it makes the game QUIET, not correct (a silently
wrong region instead of a crash, harder to tell from the parked black floor).
It goes in the DT1 milestone with the rest.** (b) 26 Aug, 1 run in 4 -- a panic in
`d2dt1.DecodeTileGfxData` from `generateWallCache` during
map load, upstream of all M4.1 code. Either way: a playtest run that dies in
~3 s with a game-output tail is one of these; RE-RUN before investigating. The
index EQUALS the length, which points at the source buffer, not the
destination: the decoder walks `block.EncodedData` off its end when an RLE
run claims more pixels than remain. Every access in that function is
unchecked. Two more real bugs sit beside it in tile_cache.go: the wall pixel
buffer is sized from whichever of two tiles is SHORTER and then both are
decoded into it (:178-218), and `newTileOptions` is indexed with a
RandomIndex chosen against a different options array (:174).
Benign log line "invalid frame index" allowlisted. Minimized window keeps
ticking (~63/s). Recipe: pause BEFORE start_game{seed}, advance only with
step/step_world; digests are build-specific. `gate.ps1` and `playtest.ps1`
need `-ExecutionPolicy Bypass` or they fail silently.

Parked for Josh: history rewrite before friends build #1; dead-file removals;
the skill trigger test; deep-decode test paths; the S1 sec. 5/9.1 game-scope
annotation; the "what Vlad knew" thread (his separate chat -- do not
pre-empt); the black-floor FIX; the locked-session tick test; **softening the
torch's tile-stepped edge** (drop FalloffStart) if the look ever matters.

Do not: `go get -u`; write any *.mpq/*.dc6/*.dcc/*.ds1/*.dt1/*.cof/*.pl2/
*.tbl/*.d2; write into /harness-runs/ or run dirs from Claude Code; create
a capitalised `Docs/`; commit CRLF; let a test binary link ebiten (check
`go list -deps`); claim "on disk"/"pushed" without a same-burst listing; put
gameplay logic in the harness; read the wall clock in a world system.
