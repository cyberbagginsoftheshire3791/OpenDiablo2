# Strigoi's words (M5.3)

`strings.json` is a **string table**: key to text, written for Strigoi. With

    OpenDiablo2.exe -strings data/strigoi/strings/strings.json

Diablo II's three string tables (`data/local/lng`, from the MPQs) are not
loaded at all, and every label, button and panel the UI asks for by key is
answered from here.

- **Keys** are the ones the engine's UI asks for (`#1620`, `strchrstr`,
  `minipanelinv`, ...). Which keys a run asked for, and which this table
  lacked, is in the harness's string census (`strings`, `strings_missing` in
  the "assets" system; `TestAssetCensus` writes `string-census.tsv` beside
  the repo). A key missing here shows as the key itself -- add it.
- **Texts** are ours. Keep any `%d`, `%u`, `%s` the original key carried:
  the game fills them in (`panelhealth` is "Life: %d / %d").
- `\n` breaks a line (the character panel's resistances).
- While the game still uses Diablo II's data, art and sound, `#1613`/`#1614`
  credit Blizzard and say Strigoi is not theirs. Change them when it is all ours.
- **Not all of it is ours yet**: where Strigoi has no word of its own, the
  table re-types Diablo II's plain terms -- the other classes' names, the
  difficulties, the skill-tree categories, item names like "Buckler". They
  go as those screens go or get Strigoi's own.
- **Known gaps**: the tested day (`words_test.go`) covers the menu, a new
  game, the kit/talents/help/character panels, a talk and world time.
  Screens outside it -- character creation, the quest log, skill and item
  tooltips, trade -- ask for keys this table lacks and show them raw (and a
  missing format key can show `%!(EXTRA ...)`). The string census names
  every one a run meets.
- Numbered labels (`#1620`) ignore a non-English install's language
  modifier while this table is in use; `data/local/use` is that language
  code.
