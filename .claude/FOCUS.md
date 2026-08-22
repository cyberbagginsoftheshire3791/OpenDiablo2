# Focus — printed into every session by the SessionStart hook

Updated: 2026-08-22 (M2.5 done, M2.4 proven). Keep this to a screen; the full
state lives in `state.md` in the claude.ai project and in Notion.

Phase 2 (ground truth) — WS-GroundTruth CLOSED (Complete), 22 Aug. Done:
M2.1 archaeology, M2.3 fixtures (+ decoder tests below), M2.2 d2-formats
skill, M2.4 hooks + CI, M2.5 dep bumps (akara KEPT). Both lane exit
criteria are met: the hooks fired inside a live Claude Code session —
SessionStart printed, a planted Write to extracted/x.txt was refused, a
clean write passed.

Next: research bursts per S1 §12 (D4, E2, N1, E6) before Phase 4 content.
Deep-decode follow-ups (parked, pull into a future burst): DCC direction
bitstream, MPQ full-archive, DT1 block graphics.

Done 22 Aug (post-M2.5): GlyphPrinter out of d2util → d2common ebiten-free,
CI xvfb dropped (both CI-green); akara KEPT (decision logged); synthesized
tests added for d2txt, d2dat, d2mpq crypto, d2font, d2dt1, d2dcc — every
decoder package now has tests (deep decode paths per the follow-ups above);
fixed d2font.Marshal's 12-byte header, round-trip test now passing.

Parked for Josh: history rewrite before friends build #1; removal of
d2logo.ico / d2discord.png / build.sh / tagdev.bat / .github/FUNDING.yml /
the upstream issue templates / README rewrite; plan v1.4 wording; the
"what Vlad knew" premise thread (his separate chat — do not pre-empt).

Do not: `go get -u`; write any *.mpq/*.dc6/*.dcc/*.ds1/*.dt1/*.cof/*.pl2/
*.tbl/*.d2; create a capitalised `Docs/`; commit CRLF; claim "on disk" or
"pushed" without a same-burst listing.
