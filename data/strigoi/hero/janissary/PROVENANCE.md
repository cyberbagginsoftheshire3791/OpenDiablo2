# Janissary game v1

Rendered 29 September 2026 from `strigoi-art/source/heroes/janissary-game-v1.blend`,
SHA-256 `6d09436302a3072453e32cc46df999dc28feac1fc36dcc0eea0a2e19ac78a769`.
This separate derivative preserves the refined anatomical Q3 checkpoint.
The rejected rebuilt V10 attack was not used.

The human foundation is byzmod3d's Bravos human from the CC0
[OpenGameArt 3D Character Pack](https://opengameart.org/content/3d-character-pack).
The uniform, fitted headwear, mail, equipment, skeleton and motion are project
work. No Blizzard-derived art is included. The source retains the protected
Q3/V9/V10 input hashes, packed textures and editable named parts.

Inherited rebuilt V9 idle/walk/run share the unchanged 17-bone rest skeleton.
New attack/hit/block/death use the same figure and drawn sword. New flexible
surfaces are inverse-skinned from Q3's neutral pose, with measured neutral
round-trip error below 2.7e-7 metres. The 1,241,760-vertex mail is baked to
packed appearance maps; hems follow their panels. The bend-induced shoulder
strap spike was fixed by disabling unbounded even-offset thickness miters.

The saved Cycles camera uses 128px cells, scale 3 and 30-degree elevation.
`Janissary Game Root` rotates the whole figure, with row yaws
0,-90,180,90,45,-45,-135,135 for SW,NW,NE,SE,S,W,N,E. Ground origin (64,96)
gives offsets (-64,+32) throughout. Neutral soles lie at z=0; their projected
toe envelope explains why the lowest opaque pixel is below the ground centre.
Standing height measures 72px. Gait correction removes the measured negative
ground penetration while retaining airborne running phases.

All 432 input frames were checked for emptiness, clipping and measured anchors.
The dead sheet reuses the terminal death frames exactly after checking the
separately rendered terminal pose; this avoids minute repeated-render colour
noise. `export-provenance.json` records the source/config/exporter and output
hashes; `source-projection.json` retains the neutral sole projection evidence.

Live screenshots and test logs stay outside the repository because they show
the running development game. The acceptance tests use default hero selection,
movement in every bearing and real combat actions, with no animation setter.

The source/export tooling is retained through art commit `cab1cf2`. Exported
JSON uses LF line endings; all 17 game asset hashes were checked against the
staged Git blobs as well as the working files, so checkout does not invalidate
their byte-level provenance. The final normalized pack passed both Janissary
tests inside the full 85-pass, two-opt-in-skip gameplay suite.
