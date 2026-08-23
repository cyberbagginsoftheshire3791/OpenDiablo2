# Focus -- printed into every session by the SessionStart hook

Updated: 2026-08-22 (evening CT; Phase 3 opened by decision). Keep this to a
screen; the full state lives in `state.md` in the claude.ai project and in
Notion.

PHASE 3 -- the playtest harness -- is OPEN. WS-Harness: Planning, moving to
In-Progress on Josh's signature of the M3.1 design. Phase 2 (ground truth)
CLOSED 22 Aug. Research: the five slice-critical Phase-4 briefs (D4 v2, D7,
E2, N1, E6) SIGNED 22 Aug; Project Plan is v1.4 (survival-first with horror,
two modes: story campaign + open-ended survival sandbox). The Phase-4 content
runway is clear. Phase-6-writing research (G4, H7, E1, M11, C3) interleaves
when bursts go low-energy; M11 and C3 land before M4.3/M4.6 content.

M3.1 (design, no code): "Claude doc outputs/P3 - Playtest Harness Design
Spec.md", DRAFT v0.1 awaiting Josh's signature (seven sign-off asks, sec. 8).
Shape: the official Go MCP SDK compiled in behind build tag `harness`,
Streamable HTTP on 127.0.0.1:6670 (the game server owns 6669); a command
queue drained at the top of d2app.advance (one goroutine, one truth); a
TimeSource (live | paused | stepping, dt 1/60) replacing the wall-clock
delta -- the same step M4.4's world clock will run on; a world RNG on
MapEngine replacing the global math/rand for simulation (rand.Seed is a no-op
since Go 1.24, so the map seed does not seed the overworld today and the
client's and server's wilderness can differ -- fixed as a side effect);
uuid.SetRand in harness mode; a Provider contract every Phase-4 system
registers into (a system is not done until its provider exposes what its S1
sec. 12 assertion needs); outputs outside the repo (Article V -- screenshots
are Blizzard pixels). Scripts = Go tests in playtest/ behind a `playtest`
tag; laptop-only, never CI.

Next: Josh signs -> M3.2, the spine (tag + flags, the SDK dependency as one
commit, the server, the queues, the first 16 tools, playtest/town_walk_test.go,
docs/harness.md v0, .mcp.json, the black-floor floor-tile dump run once and
recorded). Then M3.3 (determinism: TimeSource, seed into NewGameServer, world
RNG, handles, step/digest tools; the town walk run twice with matching
digests) and M3.4 (providers, input overlay, spawn; WS-Harness closed).
Phase 3 owns the black-floor DIAGNOSTIC, not the fix (spec sec. 5.3).

Parked for Josh: history rewrite before friends build #1; removal of
d2logo.ico / d2discord.png / build.sh / tagdev.bat / .github/FUNDING.yml /
the upstream issue templates / README rewrite; the skill trigger test; the
deep-decode test paths (DCC direction bitstream, MPQ full-archive, DT1 block
graphics); the S1 sec. 5 / 9.1 game-scope annotation; the "what Vlad knew"
premise thread (his separate chat -- do not pre-empt).

Do not: `go get -u`; write any *.mpq/*.dc6/*.dcc/*.ds1/*.dt1/*.cof/*.pl2/
*.tbl/*.d2; create a capitalised `Docs/`; commit CRLF; claim "on disk" or
"pushed" without a same-burst listing; put gameplay logic in the harness.
