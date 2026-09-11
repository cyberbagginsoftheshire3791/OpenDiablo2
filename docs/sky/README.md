# docs/sky — the Strigoi sky/calendar tooling

This directory holds the tooling that GENERATES the game's historical day table
rather than letting anyone type it. It exists because of Article II.5 (derived
facts live in the systems that produce them) and the signed C2/C4 P4 and S1
Appendix A clauses, which named M4.4 as the moment the sky scripts move into the
repo so the numbers are regenerated, not retyped.

Note the directory is lowercase `docs/` — the repo's docs dir. Signed documents
say "Docs/" informally; a capitalized `Docs/` aliases `docs/` on Windows and
collides on Linux (see `CLAUDE.md`), so the repo uses lowercase.

## What produces the day table

    python docs/sky/gen_daytable.py

writes `d2core/d2world/daytable_gen.go` — the seven rows of the slice (17–23
June 1462, Julian), Targoviste local mean time. The clock (`d2core/d2world`)
keys into it by date; the HUD reads the feast name and moon-phase name for
"today". Commit the generated `.go`; the generator **never runs on CI and
touches no MPQs** (Article V), so it is run by hand on the laptop.

Requirements: Python 3 and PyEphem (`pip install ephem`, 4.2.x). meeus.py needs
no third-party package.

## Instruments and provenance

- `meeus.py` — instrument B, hand-coded Meeus 1998 (*Astronomical Algorithms*),
  pure math, no ephem/astropy. Computes the sun's rise/set (ch. 25 + 15) and the
  moon's phase instants (ch. 49). Run `python docs/sky/meeus.py` to see its own
  worked-example controls pass. This is the accurate instrument the generator
  uses for sunrise, sunset and the moon-phase NAME.
- PyEphem (instrument A) — supplies the two columns meeus does not compute here:
  moonrise and illuminated %. These are grounding only; the HUD does not show
  them.
- The C3 computus — Gauss Julian Easter + Zeller weekday, reproduced inside
  `gen_daytable.py` from `C3 — Julian Calendar and Orthodox Feast Days 1462`
  §3.5/§3.6. The movable feasts are placed by their offset from Easter (E+60 =
  9th Thursday after Easter on 17 Jun; E+63 = 2nd Sunday after Pentecost on
  20 Jun), not by a hand-assigned day. The fixed commemorations and Typikon fast
  rules are carried verbatim from C3 (grounded, not computable).
- `calendar1462.py`, `sun1462.py`, `moon1462.py` — the original S1-era sky
  scripts (21 Aug 2026). Kept for provenance; superseded by `meeus.py` in the
  30 Aug C2/C4 precision pass. The generator does not import them.

## Controls (Constitution VI.4b)

`gen_daytable.py` refuses to write unless all pass, and prints each:

- instrument audit — `meeus.py`'s worked examples reproduce Meeus's book values.
- reproduction (positive) — 17 Jun 1462 sunset = 19:50 LMT, matching signed S1
  Appendix A.
- negative — 1 Jan 1462 sunset (16:39 LMT) differs from the slice, proving the
  sun instrument reads the date rather than emitting a constant.
- computus derive — Julian Easter 1462 = 18 April, and the two movable feasts
  land on 17 and 20 June by offset.
