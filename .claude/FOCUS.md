# Focus -- printed into every session by the SessionStart hook

Updated: 2026-08-26 (evening CT). Keep this to a screen; the full state lives
in `state.md` in the claude.ai project and in Notion.

**PHASE 3 IS DONE** (M3.1-M3.4, DoD audit passed 26 Aug on 2c1b2124; the
harness has 33 tools, four playtest scripts, docs/harness.md v1 with a leak
register). **PHASE 4 IS OPEN: M4.1 "make night real" -- FIRST HALF SHIPPED.**

M4.1 so far (build note SIGNED 26 Aug evening, four asks as recommended):
d2core/d2world is the world's own package -- plain arithmetic over the delta
the game screen already gets, no wall clock, no renderer.
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
  combat resolver reads these same values (S1 4).
- Both are harness providers (`clock`, `light`); step_world{world_minutes}
  works (harness 0.5.0); `playtest/night_light_test.go` runs S1 4's assertion
  verbatim and passes; determinism re-proven with both in the digest
  (22b54a6ff64c / a9ada311d3f1 / 827268ad303d, identical across launches).

**NEXT: M4.1's second half -- the renderer.** The four map-render passes are
already per-tile loops; each wraps its existing body in
`target.PushBrightness(level)` / `Pop()`, fed by a one-method LightSampler
interface set on the MapRenderer (so d2maprenderer never imports d2world and
no import cycle appears). Verified hooks: PushBrightness reaches every draw
via ColorM.ChangeHSV (ebiten_surface.go:111, :153-163) and entity sprites
inherit it (animation.go:161-179). With no sampler set every call returns 1.0
and the renderer behaves exactly as today. Then the screenshots and the eye
test, and the readability question is D5's (campaign).

Standing findings: the black floor is INTERMITTENT per launch with the cache
provably colored; fix stays parked (P3 5.3) -- and from here on a NIGHT
screenshot is useless as black-floor evidence (town_walk samples a daylight
frame, which still tells them apart). Benign log line "invalid frame index"
allowlisted. Minimized window keeps ticking (~63/s). Recipe: pause BEFORE
start_game{seed}, advance only with step/step_world; digests are
build-specific.

Parked for Josh: history rewrite before friends build #1; dead-file removals;
the skill trigger test; deep-decode test paths; the S1 sec. 5/9.1 game-scope
annotation; the "what Vlad knew" thread (his separate chat -- do not
pre-empt); the black-floor FIX; the locked-session tick test; `rh.ini`.

Do not: `go get -u`; write any *.mpq/*.dc6/*.dcc/*.ds1/*.dt1/*.cof/*.pl2/
*.tbl/*.d2; write into /harness-runs/ or run dirs from Claude Code; create
a capitalised `Docs/`; commit CRLF; let a test binary link ebiten (check
`go list -deps`); claim "on disk"/"pushed" without a same-burst listing; put
gameplay logic in the harness; read the wall clock in a world system.
