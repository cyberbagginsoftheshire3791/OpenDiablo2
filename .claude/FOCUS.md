# Focus — printed into every session by the SessionStart hook

Updated: 2026-08-21 (M2.4 hooks + CI). Keep this to a screen; the full
state lives in `state.md` in the claude.ai project and in Notion.

Phase 2 (ground truth), lane WS-GroundTruth. Done: M2.1 archaeology,
M2.3 fixtures, M2.2 d2-formats skill, M2.4 hooks + CI (this file,
`.claude/settings.json`, `tools/strigoihook`, `.github/workflows/ci.yml`).

Next: **M2.5 dependency bumps** — one module per commit (otto, uuid,
profile, testify, restruct, blast; akara remove-or-keep is Josh's call),
build + vet + test after each, then a play-to-town pass. Then the
remaining M2.3 tests (synthesized DCC, MPQ, DAT, TXT, font, DT1).

Parked for Josh: history rewrite before friends build #1; removal of
d2logo.ico / d2discord.png / build.sh / tagdev.bat / .github/FUNDING.yml /
the upstream issue templates / README rewrite; plan v1.4 wording; the
"what Vlad knew" premise thread (his separate chat — do not pre-empt).

Do not: `go get -u`; write any *.mpq/*.dc6/*.dcc/*.ds1/*.dt1/*.cof/*.pl2/
*.tbl/*.d2; create a capitalised `Docs/`; commit CRLF; claim "on disk" or
"pushed" without a same-burst listing.
