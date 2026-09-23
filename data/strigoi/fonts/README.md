# Strigoi fonts (M5.3)

`fonts.json` is a **font set**: it replaces every Diablo II font (the `.tbl` +
`.dc6` pairs in the MPQs) with a TrueType/OpenType face. Play with it:

    OpenDiablo2.exe -fonts data/strigoi/fonts/fonts.json

    {"default": {"face": "gofont:regular", "size": 13},
     "fonts": {"fontexocet10": {"face": "IMFeENsc28P.ttf", "size": 13}, ...}}

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
  reproduced in `LICENSE-go-fonts.txt`; the OFL faces' licences are the
  `OFL-*.txt` files. All of them must ship with any build. A face added here
  brings its own licence file beside it.

**The faces (Josh's pick, 23 Sep 2026)**, all SIL Open Font License 1.1,
each licence beside it:

- **IM Fell English** (`IMFeENrm28P.ttf`, Igino Marini's revival of the Fell
  types) -- body text; it has Romanian's ă â î ș ț.
- **IM Fell English SC** (`IMFeENsc28P.ttf`) -- small capitals, where Diablo
  II used its Exocet caps (buttons, titles).
- **Uncial Antiqua** (`UncialAntiqua-Regular.ttf`, Astigmatic) -- the big
  headings (font30, font42). It has no ș/ț; headings that need them must
  use IM Fell. "Uncial Antiqua" is a Reserved Font Name: the file ships
  unmodified.

The Go fonts stay compiled in (`gofont:...`) as a fallback anyone can name.
