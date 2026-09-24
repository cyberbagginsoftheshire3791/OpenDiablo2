# Strigoi heroes (M5.3)

A hero is a folder with a `hero.json` naming its sprite sheets:

    {"name": "...", "animations": {"idle": "idle.png", "walk": "walk.png",
     "run": "...", "attack": "...", "hit": "...", "block": "...",
     "death": "...", "dead": "..."}}

Only `idle` is required; a missing motion falls back (run → walk,
block → hit, dead → death, anything else → idle). Unknown keys are refused.

Timing, optional: `"fps": {"attack": 10, ...}` -- frames a second, per motion.
A motion without one plays its whole sheet in one second. A run drawn with the
walk sheet plays it 13/9 as fast (the run's speed over the walk's), so the
feet keep pace with the ground.

Each sheet is a PNG with a `.png.json` manifest beside it, the same format
as the creatures in `data/strigoi/creatures/`:

- rows are directions, **8**, in Diablo II's order: **SW, NW, NE, SE, S, W, N, E**;
- columns are frames; `frame_width`/`frame_height` give the cell;
- `origin_at_bottom: true`; `offset_x` / `offset_y` place the frame so the
  figure's feet land on its position (the placeholder: 96×128 cells,
  `offset_x` −48, `offset_y` 8, feet 8 px above the cell's bottom centre).

Height, optional: `"height": 90` -- how tall the figure stands in its cell,
in pixels (feet to the top of the head). The overhead bar, the hover label
and the click box measure by it; without it they use the whole cell. At most
the cell's height.

Play with a hero: `OpenDiablo2.exe -hero data/strigoi/hero/placeholder/hero.json`.

**`placeholder/` is NOT the real art** — a stick figure in a long coat and a
tall white cap, drawn by `go run ./tools/heroplaceholder` so the path could
be proved before the Janissary exists. The real hero is Josh's and GPT's to
make; replace these sheets, or add a new folder and point `-hero` at it.
