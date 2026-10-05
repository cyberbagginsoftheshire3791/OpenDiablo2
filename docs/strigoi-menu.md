# Strigoi front menu

The default game opens directly on an original village-at-dusk menu. Play
uses the existing hero selection flow. Multiplayer retains the engine's
local-network test path. Credits is a local Strigoi attribution page. Tools
contains the World Editor, Map Engine Test and upstream engine source link.
Quit exits. Escape returns from subpages; Escape on the main page stays there.

`-classic` retains the inherited menu. Menu identity follows the classic flag,
independently of a `-fonts` override. The character creation/selection screens,
loading screen, cursor, click sound and game HUD remain inherited in this slice.
The native front menu is silent; destination screens own their music.

The background is `data/strigoi/ui/menu/village-dusk-background-v1.png`.
Josh approved the original illustration on 4 October 2026. Its unmodified
1448x1086 source, exact generation prompt and provenance are preserved in
`strigoi-art/source/ui/menu-v1/`; see `CREDITS.md`. The game fits this 4:3
image into 800x600 logical UI coordinates. World camera and authoring scale
are unchanged. A missing background logs the error and uses a dark face
with the same working controls.

Menu buttons and the address box use original code-drawn faces. Complete
label images are cached before fitting, so text spacing and glyphs scale
together. The screen unbinds its own widgets on unload. A consumed mouse
release cannot activate the previous press again, and hidden/unfocused text
boxes ignore backspace repeat events.

Use the project's shared-lock launcher with `-Repo` pointing at this checkout
to inspect unmerged work with private saves. Runtime captures stay outside
the repository because retained cursor/loading/selection content is inherited.
This change is a front-menu replacement; it does not certify a Steam release.
