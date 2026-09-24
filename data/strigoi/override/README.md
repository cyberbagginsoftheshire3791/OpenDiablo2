# Strigoi art in place of Diablo II's sprites (M5.3)

Any sprite the game still draws from Diablo II -- the cursor, the buttons,
the panels, the orbs, a villager -- can be replaced by a PNG **without code**:
put it here at the sprite's own path, lower-cased, with `.png` for `.dc6` /
`.dcc`:

    /data/global/ui/CURSOR/ohand.DC6  ->  data/strigoi/override/data/global/ui/cursor/ohand.png

- **One frame**: the PNG alone is the frame.
- **Several frames or directions**: a `.png.json` beside it, the creatures'
  sheet format -- rows are directions, columns are frames, one cell size:

      {"directions": 1, "frames_per_direction": 4, "frame_width": 48, "frame_height": 48}

  (and `offset_x` / `offset_y` / `origin_at_bottom` where the sprite hangs from
  a point, as characters do).
- **What size?** The harness tool `strigoi_describe_sprite {path}` answers:
  directions, frames, each frame's size, and the exact override path. The
  list of every Diablo II sprite a first day still draws, with sizes, is
  written by the art-needs probe (see `docs/asset-census.md`).
- Diablo II's frames within one sprite can differ in size; a sheet has one
  cell size. Draw each frame at the top-left of its cell and size the cell to
  the largest.
- Real alpha is fine (no palette). Keep the overall look dark.
- Directory names and file names **lower-case** (the game runs on Linux too) --
  except a language folder, which the game fills in: `{LANG}` becomes the
  language code in capitals (`ENG`), `{LANG_FONT}` the character set (`LATIN`).
  `strigoi_describe_sprite` names the exact path.
- A broken PNG or manifest is reported in the log and Diablo II's sprite is
  drawn instead; it never stops the game.
- A sprite with several frames or states (a button's up and down) needs them
  all: the game asks for frame 1, 2 ... Match the original's frame count.
