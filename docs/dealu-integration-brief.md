# Dealu model integration — 29 September 2026

Goal: install the finished monastery modules in the playable game and world editor, with honest footprints, an open gateway the player can traverse, and a ready-to-launch authored monastery scene.

Base measured from code: 78a6ffe4. Primary checkout was clean. Work occurs in the separate art/dealu-integration worktree; other save/raid branches and their playtest lock are preserved. No remote push is authorized by the current standing handoff.

## Evidence and design

VERIFIED: `d2core/d2map/d2maptiled/tiled.go:879` rejects unequal footprint sides, `:887` rejects blocked=false, `:810` demands (w+h)*80 image width. `d2core/d2map/d2mapgen/authored.go:368` uses a single n for both front faces. The editor palette reads only tilesets embedded in the current map (`docs/editor.md`, The palette). Existing village house, ruin, hearth and well art is already installed (`data/strigoi/structures/README.md`).

MEASURED from final PNG alpha: cells 1120x720 bbox(383,152,1057,668), declared 3x7; refectory 960x640 bbox(299,110,901,590), declared 3x6; wall 480x384 bbox(15,77,305,366), declared 3x1. The current art uses width 160*max(w,h), with the footprint front corner at bottom-center. Its unused left padding is (max(w,h)-w)*80: 320, 240, 0 respectively; used image spans are (w+h)*80. Keep these exact final pixels and anchors rather than stretch images or invent oversized square collision.

RECOMMENDED: permit rectangular structure footprints, preserve the padded bottom-center canvas contract as 160*max(w,h), and slice only the occupied image span. Left face has w strips; right face h strips. Left image padding is (max(w,h)-w)*80. Existing squares have zero padding and retain the identical strip calculation. Validate any unused side padding as transparent in the actual image loader, rather than silently drop opaque art.

RECOMMENDED: add an optional strict `collision_mask` tileset string, rows separated by `/`, row-major across the declared footprint, `1` solid and `0` traversable. It applies only to structures and must exactly match footprint dimensions. Absent mask retains current fully solid behavior. Structure reservation/selection remains the whole footprint; movement and sight use the same per-cell mask (solid cells respect blocks_sight). Open gate uses a centered three-tile channel (orientation to be measured against the live image); closed gate remains solid. Do not claim a runtime door toggle: open and closed are two placeable variants.

Keep engine parser, editor validator/occupancy and palette in agreement. Reuse strict mask validation where practical. A traversable cell must be standable and reachable; it must still prevent overlapping a second building. Malformed masks, non-structure masks, and mismatched dimensions must be refused before editor save or playtest. Existing ordinary tiles retain their behavior.

## Delivery and acceptance

- All six v2 modules, seven states, installed with source hashes/credits. Keep unrelated unfinished character art untouched.
- Preserve village.tmj byte-for-byte: saves compare its exact SHA (`d2game/d2gamescreen/load.go:552`), including otherwise unused tilesets. Add `dealu-monastery.tmj` with both village and monastery pieces in its palette, a walled courtyard, honest open gate approach, separate buildings, existing ground assets, and an appropriate starting point. This is a separate playable scene, not a world-transition or sanctuary-mechanics feature.
- Supply clear local launchers for playing and editing the monastery; build the current game after integrating changes. Preserve existing saves and the standing no-push decision.
- Measure authored coordinates, strip positions and collision in a temporary playtest scaffold before relying on the new contract. Do not run concurrently with another agent's live playtest lock. Remove scaffold before closeout.
- Meaningful tests: rectangular footprints in both orientations; existing square behavior; malformed mask/padding refusals; gate channel vs blocked jambs; palette/validator/engine agreement; editor place/save/reopen/play; live navigation through gate with side-wall negative control; visible native scale and occlusion screenshots. Every new load-bearing assertion has a exercised negative control.
- Run project gates and the full reachable-symbol register and playtest suite on final merged state. Record exact limitations rather than call a loaded PNG a gameplay test.

Strongest case against: generic structure features could turn an art integration into a large engine detour and alter saves or existing maps. Bound the change to rectangular sprite slicing and explicit per-tile structure collision, preserve all legacy defaults, use a separate scene, and prove existing square/render behavior unchanged. A fake square around a long building or an impassable open gate is not an acceptable shortcut.

UNKNOWN pending independent review and measurement: precise map-axis orientation of the gate channel; strip ordering while a player walks under the belfry; current playtest/build slot availability. No new historical authenticity claim is made.

## Independent attack folded in

[verifier] No A blocker after amendments. Independent enumeration of 3x7, 7x3, 3x1, 1x3 and 3x3 confirms the strip placement and vertical cancellation. Engine WorldToOrtho gives Blender X -> map X and Blender Y -> negative map Y; 101/101/101 therefore opens the right gate axis. A transposed 111/000/111 must fail the navigation orientation test.

[verifier] Preserve the existing village bytes, not merely its positions, because any TMJ hash change refuses a saved world. New scene and launchers satisfy this task without changing that identity.

[verifier] Keep the header-only Art API. Alpha-padding rejection belongs to actual engine Parse and the editor's Checked/Save/playtest boundary; do not claim header-only Validate measures pixel transparency. Exercise an opaque padding pixel that passes header dimensions and is refused before save/play.

[verifier] Coverage remains the entire footprint for selection and overlap; collision is per mask in both Map and Doc. Test both jambs, all channel cells, blocked underlying floor, sight, overlapping a second building into the opening, and unmasked legacy solids. Strictly reject duplicate/empty/nonbinary/non-string/whitespace/mis-sized masks or a mask without a footprint. Update all three separate footprint rule implementations and their existing refusal tests.

[verifier] Occlusion requires actual walking screenshots before, within and after the three channel cells in both directions. Correct strip math alone does not prove heads stay behind the lintel. This remains an explicit release check.
