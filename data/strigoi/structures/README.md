# Strigoi structures (art-ready renders, installed for the map)

These PNGs are the production renders from `strigoi-art` (Codex and Josh's
art pipeline), copied here so the authored village (M5.4) can stand them on
the map. **Do not edit them here** — edit the Blender source in strigoi-art,
rebuild with its `build-*.cmd`, and copy the new render over.

| Here | From strigoi-art | Footprint | Used as |
|---|---|---|---|
| `peasant-house/intact.png` | `renders/structures/peasant-house-v1/intact-480x448.png` | 3×3 | structure (tile object) |
| `burned-house/cold-ruin.png` | `renders/structures/burned-house-v1/cold-ruin-480x448.png` | 3×3 | structure (tile object) |
| `village-well/intact.png` | `renders/structures/village-well-v1/intact-160x256.png` | 1×1 | wall tile |
| `village-hearth/unlit.png` | `renders/structures/village-hearth-v1/unlit-160x128.png` | 1×1 | wall tile (unlit; the fire states wait for map fires) |

Provenance and sources: `strigoi-art/provenance/` and `strigoi-art/structures/*.json`.
The game's rules for structure art are in `data/strigoi/maps/README.md`.
