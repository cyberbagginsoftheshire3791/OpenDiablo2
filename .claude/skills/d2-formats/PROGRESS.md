# d2-formats — progress (delete when every box is ticked)

Populated 21 August 2026 (M2.2) from the scaffold `d2-formats-skill-scaffold.md`,
by extraction passes over the decoders at commit `c26dc732`, with every
load-bearing claim spot-checked against source and every cited commit hash
verified in `git log`.

```
[x] Patch version confirmed and recorded in SKILL.md (1.14b; decoder gates listed)
[x] Decoder inventory complete (Prompt 1) — SKILL.md table
[x] mpq.md          — incl. archive precedence
[x] sprites.md      — DC6
[x] sprites.md      — DCC
[x] animation.md    — COF + composite system
[x] animation.md    — layer→path naming convention (Prompt 3)
[x] palette.md      — incl. quantization target for new art (error metric: UNKNOWN, a design decision)
[x] maps.md         — DS1 + DT1
[x] tables.md       — the tables this fork actually reads (86 loaders, 83 bootstrapped)
[x] gotchas.md      — seeded from Prompt 4 (45 entries, every hash verified)
[x] fixture-manifest.md — points to docs/fixtures-manifest.md (canonical)
[x] All <!-- VERIFY --> markers checked against code (resolutions recorded in each reference)
[x] All <!-- FILL --> markers resolved
[x] Worked examples (Prompt 5) — from in-code test fixtures and D.mpq; no Blizzard bytes
[ ] Skill tested (scaffold §6): trigger test in a fresh Claude Code session with the
    three un-named prompts, and one known-answer + one unknown-answer correctness check
```

Scaffold VERIFY resolutions, for the record:
- "DC6 = UI/fonts/icons; DCC = animated units" — broadly right; corrected in
  `sprites.md` (dispatch is by extension; composites are DCC-only in practice
  because the DC6 fallback is unreachable).
- "DS1 has several versions with differing layer counts" — confirmed and
  extended in `maps.md` (v3–v18; header fields and trailing records differ too).
- "Direction counts 1/4/8/16/32" — the code's tables are 4/8/16/32/64; 1 falls
  to `default: return 0` (`animation.md`, `sprites.md`).
- "Animation mode tokens NU, WL, RN, A1, A2, BL, SC, TH, KK, GH, DD, DT, S1–S4"
  — incomplete: player also has TN/TW; monsters lack TH/KK and have `xx`;
  objects are NU/OP/ON/S1–S5 (`animation.md`).
- "Font TBL = glyph metrics; string TBL = key→string" — confirmed (`tables.md`).
- "TXT compiled to BIN" — the fork reads TXT only; no code opens a `.bin`.
