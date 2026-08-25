# Focus -- printed into every session by the SessionStart hook

Updated: 2026-08-24 (late evening CT). Keep this to a screen; the full state
lives in `state.md` in the claude.ai project and in Notion.

PHASE 3 -- the playtest harness -- P3 spec SIGNED by Josh 24 Aug (all seven
sec. 8 asks as recommended) and **M3.2 (the spine) SHIPPED the same night**:
the MCP Go SDK server behind build tag `harness` on 127.0.0.1:6670, update/
draw queues in d2app, 16 strigoi_* tools, .mcp.json + settings attach,
playtest/ Go tests, docs/harness.md. town_walk_test PASSES on the laptop;
Claude Code attached headless and drove the game (hero created, screenshot,
quit). Run artifacts: <Projects>\strigoi-harness-runs\ (never the repo).

FINDINGS (24 Aug, docs/harness.md): the black town floor is INTERMITTENT per
process launch -- two healthy and two black launches the same evening; in the
black runs strigoi_dump_surface proves the cached floor tiles are FULLY
COLORED, so content exists but DrawImage of NewSurface+ReplacePixels surfaces
composites nothing while NewImageFromImage sprites draw. Suspect the OnLoad-
goroutine image build (screens load off the main goroutine). Fix is NOT
Phase 3 work; the town-walk script records floor luminance every run.
Minimized window: the loop keeps ticking (~63/s). Benign known error line:
"[UI Manager][ERROR] ... invalid frame index" (allowlisted in the script).

Next: **M3.3 -- determinism** (P3 sec. 6.1): TimeSource (live|paused|
stepping, dt 1/60); seed into NewGameServer (E3) so start_game{seed} works;
world RNG on MapEngine for mapgen/mapstamp/npc (fixes the client/server
wilderness divergence -- rand.Seed is a no-op since Go 1.24); uuid.SetRand +
handles; tools pause/resume/step/step_world/set_seed/reseed_world/
get_state_digest; move_player_to{wait}; the town walk run TWICE with seed
1462 and matching digests. Then M3.4 (providers, input overlay, spawn).

Parked for Josh: history rewrite before friends build #1; removal of
d2logo.ico / d2discord.png / build.sh / tagdev.bat / .github/FUNDING.yml /
upstream issue templates / README rewrite; the skill trigger test; the
deep-decode test paths; the S1 sec. 5/9.1 game-scope annotation; the "what
Vlad knew" premise thread (his separate chat -- do not pre-empt); the
black-floor FIX (diagnostic-only in Phase 3, evidence now in hand).

Do not: `go get -u`; write any *.mpq/*.dc6/*.dcc/*.ds1/*.dt1/*.cof/*.pl2/
*.tbl/*.d2; write into /harness-runs/ or the run dirs from Claude Code;
create a capitalised `Docs/`; commit CRLF; claim "on disk" or "pushed"
without a same-burst listing; put gameplay logic in the harness.
