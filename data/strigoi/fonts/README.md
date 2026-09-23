# Strigoi fonts (M5.3)

`fonts.json` is a **font set**: it replaces every Diablo II font (the `.tbl` +
`.dc6` pairs in the MPQs) with a TrueType/OpenType face. Play with it:

    OpenDiablo2.exe -fonts data/strigoi/fonts/fonts.json

    {"default": {"face": "gofont:regular", "size": 13},
     "fonts": {"fontexocet10": {"face": "gofont:smallcaps", "size": 12}, ...}}

- **Keys** are the fonts Diablo II's UI asks for: `font6`, `font8`, `font16`,
  `font24`, `font30`, `font42`, `fontformal10/11/12`, `fontexocet8/10`,
  `fontridiculous`. Any the set does not list gets `default`, so no Diablo II
  font is ever loaded. A key that is not one of these is refused (a typo would
  otherwise fall silently to the default).
- **face**: `gofont:regular|medium|bold|italic|smallcaps|mono` (the Go fonts,
  compiled into the game, BSD licence) or a `.ttf`/`.otf` file, relative to
  this folder.
- **size**: the em size in pixels (a 13 px Go face stands about 16 px tall; a
  line is as tall as the face's tallest glyph and deepest descender).
- **color**: `#rrggbb`, the ink; default `#f0e8d8`. Labels that tint their
  text (gold headings, grey disabled buttons) MULTIPLY over it, so keep it
  near white -- a gold ink under a gold tint comes out ochre.
- **Characters**: ASCII, Latin-1, Latin Extended-A and Romanian's ș/ț. A face
  without some of them simply lacks those glyphs.
- **Known limit**: Diablo II drew one font in different palettes for
  different screens (a "static" and a "units" colouring); a set has one ink
  per font, and labels' own tints carry the rest.
- **Licence**: the Go fonts are Bigelow & Holmes's under the Go BSD licence,
  reproduced in `LICENSE-go-fonts.txt` (it must ship with any build). A face
  added here brings its own licence file beside it.

**These choices are PLACEHOLDERS** -- the Go fonts are clean and legible and
look nothing like 1462. The real faces are Josh's call; a free (SIL OFL)
face with the licence file beside it drops in by naming it here.
