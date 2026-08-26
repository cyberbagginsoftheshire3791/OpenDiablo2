# Focus -- printed into every session by the SessionStart hook

Updated: 2026-08-26 (morning CT). Keep this to a screen; the full state lives
in `state.md` in the claude.ai project and in Notion.

PHASE 3: M3.1 (design, SIGNED 24 Aug) + M3.2 (spine, 24 Aug) + **M3.3
(determinism, 26 Aug) DONE. The harness is deterministic and PROVEN:**
TestTownWalkDeterministic ran the seeded town walk (seed 1462) in two
separate process launches -- identical spawn, identical walk (stuck at the
same wall after exactly 150 ticks), byte-identical state digests at all
three checkpoints. **The leak register opened EMPTY** (docs/harness.md).

What M3.3 landed (commits 36a514c8 engine + 005e31de harness): E1 the
paused/stepped clock (d2util.FrameDeltas pinned by test; advanceOnce driven
directly at dt 1/60, 600 ticks/frame batches); E3 one-shot server seed
(d2server.SetNextGameSeed; wall clock unchanged when unset); E4 the world
RNG on MapEngine behind a draw-counting source, feeding mapgen, stamp
selection, NPC equip variants, and per-NPC behaviour rngs -- ALSO fixes the
old client/server wilderness divergence (rand.Seed no-op since Go 1.24);
E5 uuid.SetRand for reproducible entity IDs (crypto-random restored when
unseeded). Tools: get_time_mode, pause, resume, step, step_world,
set_seed, reseed_world, get_state_digest (per-part sha256: sim/world/
entities/rng/systems; NO raw frame ticks -- comparable across launches);
move_player_to{wait,max_ticks}. Recipe for reproducible runs: pause BEFORE
start_game{seed}, then advance only with step.

Next: **M3.4 -- providers, input, spawn** (P3 sec. 6.1, the last Phase 3
milestone): the d2harness Provider tools (list_systems, get_system_state,
set_system_field), the InputService overlay (key/click/move_cursor/
type_text at the d2input seam; game_controls wall-clock reads move to the
injected clock), spawn_entity/remove_entity, one UI script (open inventory
with 'i'), docs/harness.md v1, then the Phase 3 DoD audit (spec 6.4) ->
WS-Harness closes, WS-Wallachia re-entry ("harness usable") unlocks M4.1.

Standing findings: the black floor is INTERMITTENT per launch with the
cache provably colored (init-race suspect, OnLoad-goroutine image builds);
fix stays parked (spec 5.3) -- town_walk logs floor luminance every run
(98% on 26 Aug's run). Benign log line "invalid frame index" allowlisted.
Minimized window keeps ticking (~63/s).

Parked for Josh: history rewrite before friends build #1; dead-file
removals; the skill trigger test; deep-decode test paths; the S1
sec. 5/9.1 game-scope annotation; the "what Vlad knew" thread (his
separate chat -- do not pre-empt); the black-floor FIX.

Do not: `go get -u`; write any *.mpq/*.dc6/*.dcc/*.ds1/*.dt1/*.cof/*.pl2/
*.tbl/*.d2; write into /harness-runs/ or run dirs from Claude Code; create
a capitalised `Docs/`; commit CRLF; claim "on disk"/"pushed" without a
same-burst listing; put gameplay logic in the harness.
