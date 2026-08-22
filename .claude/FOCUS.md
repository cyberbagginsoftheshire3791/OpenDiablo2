# Focus — printed into every session by the SessionStart hook

Updated: 2026-08-22 (M2.5 done, M2.4 proven). Keep this to a screen; the full
state lives in `state.md` in the claude.ai project and in Notion.

Phase 2 (ground truth), lane WS-GroundTruth. Done: M2.1 archaeology,
M2.3 fixtures, M2.2 d2-formats skill, M2.4 hooks + CI, M2.5 dep bumps
(otto, uuid, profile, testify, restruct, blast — one commit each, gate
green after each; akara left pinned). BOTH lane exit criteria are met:
the hooks have now fired inside a live Claude Code session — SessionStart
printed, a planted Write to extracted/x.txt was refused, a clean write
passed.

Next (Josh's calls, or pick one): akara remove-or-keep; the remaining
synthesized decoder tests (DCC, MPQ, DAT, TXT, font, DT1); research
bursts per S1 §12 (D4, E2, N1, E6) before Phase 4 content; optional
one-liner — move debug_print.go out of d2util (drops CI's xvfb need).

Parked for Josh: history rewrite before friends build #1; removal of
d2logo.ico / d2discord.png / build.sh / tagdev.bat / .github/FUNDING.yml /
the upstream issue templates / README rewrite; plan v1.4 wording; the
"what Vlad knew" premise thread (his separate chat — do not pre-empt).

Do not: `go get -u`; write any *.mpq/*.dc6/*.dcc/*.ds1/*.dt1/*.cof/*.pl2/
*.tbl/*.d2; create a capitalised `Docs/`; commit CRLF; claim "on disk" or
"pushed" without a same-burst listing.
