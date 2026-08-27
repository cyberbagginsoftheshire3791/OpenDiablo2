# Focus -- printed into every session by the SessionStart hook

Updated: 2026-08-27 (late morning CT). Keep this to a screen; the full state
lives in `state.md` in the claude.ai project and in Notion.

**PHASE 3 IS DONE** (M3.1-M3.4, DoD audit passed 26 Aug on 2c1b2124; the
harness has 33 tools, six playtest scripts, docs/harness.md v1 with a leak
register). **M4.1 "MAKE NIGHT REAL" IS DONE** -- the clock, the light model,
the renderer, and (27 Aug) placed sources, which reopened it.

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
Still out of scope and UNSCOPED: lighting the MAP's own fires -- which D2
object ids count as fire is content work against E6; ask Josh before starting.

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
  works (harness 0.5.2); `night_light_test.go` runs S1 4's assertion verbatim,
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
  on 27 Aug: b9d8e7168236 / ac13cd808406 / ec66a1ed93d1 (they MOVED from
  22b54a6ff64c / a9ada311d3f1 / 827268ad303d because the light provider
  reports more -- digests are build-specific by design; the proof is that two
  launches agree, not that a value matches yesterday's).

**NEXT: M4.2, the meters** (hunger/warmth/fatigue, D4). Inherited rules,
unchanged: register a provider at construction, ship a playtest script, stay
inside the digest, build on the stepped clock -- and expose every value the
assertion names, at the positions it names them.

Standing findings: the black floor is INTERMITTENT per launch with the cache
provably colored; fix stays parked (P3 5.3) -- and from here on a NIGHT
screenshot is useless as black-floor evidence (town_walk samples a daylight
frame, which still tells them apart). **New and intermittent, filed not
fixed, and it has TWO faces:** (a) 27 Aug, 1 full-suite run in 2 -- a nil
dereference in `d2mapengine.addDT1` (engine.go:107) from ResetMap during
start_game. One line causes it: addDT1 logs LoadDT1's error and then FALLS
THROUGH to `dt1.Tiles` instead of returning, so a failed load kills the
process rather than leaving a tileset missing. Downstream of the asset-cache
defect below. (b) 26 Aug, 1 run in 4 -- a panic in
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
