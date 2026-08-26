# Focus -- printed into every session by the SessionStart hook

Updated: 2026-08-26 (midday CT). Keep this to a screen; the full state lives
in `state.md` in the claude.ai project and in Notion.

**PHASE 3 IS DONE.** M3.1 (design, SIGNED 24 Aug) + M3.2 (spine, 24 Aug) +
M3.3 (determinism, 26 Aug) + **M3.4 (providers, input, spawn, 26 Aug)**. The
harness has 33 tools, three playtest scripts (town_walk live, determinism
two-launch proof, ui_inventory scripted input), and docs/harness.md v1 with
the leak register. The DoD audit (P3 spec sec. 6.4) is in the closeout of the
26 Aug burst; WS-Harness closes, WS-Wallachia re-enters ("harness usable").

What M3.4 landed: the d2harness registry grew Stateful (entity half),
FieldLister, Unregister; Player/NPC expose HarnessState; GameControls is the
first real provider ("ui": panel/menu states) and keeps its OWN clock for
click-repeat timing (no wall clock in input handling); the
d2input.ScriptedInputService overlay (E6) at NewInputManagerWithService --
a scripted tap is just-pressed for exactly one poll, merged with the real
devices; tools list_systems / get_system_state / set_system_field / key /
click / move_cursor / type_text / spawn_entity / remove_entity; get_entity
gained a `screen` pixel position for aiming clicks; start_game now returns
after the first game frame (leak register #1, closed same day).

Two engine bugs the harness found, fixed as their own commits: MainMenu
never unbound its input handler (#792) -- a second Escape in-game hit the
stale menu's exit branch and killed the process silently; main.go swallowed
Run()'s error. Neither reproduces now.

Next: **Phase 4 -- M4.1 (light/night)** on the stepped clock, per plan v1.4
and the signed D4/D7/E6 briefs. Every Phase 4 system registers a provider
at construction (the rule: not done until the provider exposes what its
S1 sec. 12 assertion needs) and ships with a playtest script. Interleave
research when low-energy: M11 first.

Standing findings: the black floor is INTERMITTENT per launch with the
cache provably colored (init-race suspect, OnLoad-goroutine image builds);
fix stays parked (spec 5.3) -- town_walk logs floor luminance every run
(96-98% on 26 Aug). Benign log line "invalid frame index" allowlisted.
Minimized window keeps ticking (~63/s). Recipe: pause BEFORE
start_game{seed}, advance only with step; digests are build-specific.

Parked for Josh: history rewrite before friends build #1; dead-file
removals; the skill trigger test; deep-decode test paths; the S1
sec. 5/9.1 game-scope annotation; the "what Vlad knew" thread (his
separate chat -- do not pre-empt); the black-floor FIX; the locked-session
tick test.

Do not: `go get -u`; write any *.mpq/*.dc6/*.dcc/*.ds1/*.dt1/*.cof/*.pl2/
*.tbl/*.d2; write into /harness-runs/ or run dirs from Claude Code; create
a capitalised `Docs/`; commit CRLF; claim "on disk"/"pushed" without a
same-burst listing; put gameplay logic in the harness.
