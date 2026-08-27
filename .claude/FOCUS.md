# Focus -- printed into every session by the SessionStart hook

Updated: 2026-08-26 (late night CT). Keep this to a screen; the full state lives
in `state.md` in the claude.ai project and in Notion.

**PHASE 3 IS DONE** (M3.1-M3.4, DoD audit passed 26 Aug on 2c1b2124; the
harness has 33 tools, five playtest scripts, docs/harness.md v1 with a leak
register). **PHASE 4 IS OPEN: M4.1 "make night real" -- THE CLOCK, THE LIGHT AND
THE RENDERER SHIPPED; M4.1 IS REOPENED FOR PLACED LIGHT SOURCES.**

**The gap, found by Josh looking at the torch screenshot:** the light follows
the PLAYER because a carried source is the only kind the game can make. S1 4
says "carried AND PLACED sources restore a radius around themselves", and the
build note's scope fence never excluded placed ones. Light.Add(kind, carried,
x, y) supports them and light_test.go exercises them -- but the only non-test
caller is light.go:415 inside HarnessSet("carried_source"), which hardcodes
carried=true at the player. Nothing in the game or the harness can put a light
anywhere else, so the camp's own fires stay dark and the provider reports a
source_list that can never hold anything but the player's torch. A unit test
passing on an unreachable path is exactly what the provider contract exists to
prevent. NEXT BURST FINISHES IT: a harness place-light verb, a script
assertion that a placed hearth lights where it stands and not on the player.
Lighting the MAP's own fires is a separate, larger job (which D2 object ids
count as fire is content work against E6) and is scoped later.

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
  works (harness 0.5.0); `night_light_test.go` runs S1 4's assertion verbatim
  and `night_render_test.go` measures it off four screenshots (night 88%
  dimmer than noon; unlit night falls uniformly near x0.116 / far x0.118; a
  torch breaks that uniformity near x8.59 / far x1.63). Determinism unmoved
  by the renderer (22b54a6ff64c / a9ada311d3f1 / 827268ad303d, identical
  across launches on both halves) -- presentation is outside the digest.

**NEXT: finish M4.1** (the place-light verb + its script assertion), THEN M4.2,
the meters (hunger/warmth/fatigue, D4). Same inherited rules for both: register
a provider at construction, ship a playtest script, stay inside the digest,
build on the stepped clock.

Standing findings: the black floor is INTERMITTENT per launch with the cache
provably colored; fix stays parked (P3 5.3) -- and from here on a NIGHT
screenshot is useless as black-floor evidence (town_walk samples a daylight
frame, which still tells them apart). **New and intermittent, filed not
fixed:** a panic in `d2dt1.DecodeTileGfxData` from `generateWallCache` during
map load (1 full-suite run in 4 on 26 Aug), upstream of all M4.1 code. The
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
