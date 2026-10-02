# Strigoi — creature art spec

**Written 20 September 2026. Every number here was MEASURED, not guessed:**
`go run ./tools/spritescale` reads D2's own creature sprites out of the MPQs and
reports their real pixel dimensions, frame counts and facing counts. The log is
`strigoi-harness-runs\spritescale.txt`. If a number below looks wrong, re-run the
tool rather than arguing with the document.

This exists because the game is getting its own art, generated outside the repo,
and an image generator asked for "an isometric wolf" will hand back a beautiful
1024×1024 hero pose that is useless. What follows is what to ask for instead.

---

> **[1 Oct 2026 — the camera scale.]** The game is moving to a Dawn of War 2-scale
> camera: the world is drawn at about **0.5** on this 800×600 screen (`-zoom`, the
> wheel; see `docs/camera.md`), and later at 1.0 on a screen of about twice the
> resolution. **Author at the native scale, unchanged:** orthographic, 30° elevation,
> 45° azimuth, light upper-left, **42.7 px per metre** (the Janissary rig: 128 px cell,
> ortho scale 3); a 160×80 tile is about 2.65 m square. Every structure and prop goes
> through the same px-per-metre (the old house and hearth rigs measured ~2.1× and must
> be re-rendered). Judge every sheet at **50% (box filter)** and darkened: silhouette and
> big colour blocks carry it; detail under ~4 art px and lines under 2 px are lost.
> Record: the project's `claude/rulings-2026-10-01-camera-scale-and-fog.md`.

## 0. What the game is — give the generator this context first

Wallachia, June 1462, in the days after the Night Attack at Târgoviște. A
countryside Vlad scorched and the campaign left strewn with unburied dead. You
play a survivor preparing by day and surviving the night. Isometric, 2:1, seen
from above at a shallow angle. **Grim, muddy, period-real, and small** — not high
fantasy, no glowing runes, no oversized pauldrons, no saturated colour.

The three things that decide whether a piece of art is right:

1. **The night is the enemy.** Most of the time this creature is seen in the
   dark, by torchlight, at the edge of a light radius. Silhouette does all the
   work; interior detail does almost none.
2. **Grounded, then supernatural.** A wolf is a wolf. What is frightening is that
   it is on the corpse you came to bury. The supernatural things are *wrong*
   versions of real things, not invented creatures.
3. **One region done deeply.** Carpathian foothills, 15th century. Everything
   should look like it belongs to that place and that year.

---

## 1. The technical shape — measured

| Fact | Value | Where it comes from |
|---|---|---|
| Game resolution | **800 × 600** | the engine's native surface |
| Facings per creature | **8** — measured 8 of 8 on every creature and every mode | `spritescale` |
| Idle frame size | **19–32 px wide, 41–55 px tall** | `spritescale`: fallen1 32×55, zombie1 19×41, skeleton1 25×45 |
| Walk frame size | 20–35 × 43–59 | `spritescale` |
| Attack frame size | 22–62 × **69–96** | `spritescale` — attacks are the tall ones |
| Death frame size | 69–82 × 62–94 | `spritescale` — deaths are the *wide* ones |
| Frames per facing | idle 8–20 · walk 8–12 · attack 10–16 · death 19–20 | `spritescale` |

**Read the two surprising rows.** A creature's idle pose is about **30 × 50
pixels**. That is very small — smaller than a favicon is tall. And the *attack*
and *death* frames are much bigger than the idle ones, because a lunge and a fall
sweep outside the standing silhouette. So a creature does not have one frame
size; it has one per animation.

**What that means for generation: make it big, let the tool shrink it.** Image
generators are bad at tiny canvases and good at large ones. Generate each facing
at **512 × 512** with the creature filling most of the frame, and the import tool
scales it down to the cell size the engine wants. Asking for 32 × 55 directly
produces mush.

---

## 2. What to ask for, per creature

One creature, one animation, one facing at a time. Eight facings, in this order —
**the import tool remaps them to the engine's internal order, so use this one and
don't worry about what the engine does with it:**

```
1. S   (facing the camera, coming toward you)
2. SW
3. W   (facing left, profile)
4. NW
5. N   (facing away, back to camera)
6. NE
7. E   (facing right, profile)
8. SE
```

Animations needed, in build order. **Do not start with all four — get the idle
of one creature right first, then the walk.**

| Mode | What it is | Frames |
|---|---|---|
| `NU` | **idle** — standing, breathing, alert. Start here. | 8 |
| `WL` | **walk** — a full gait cycle that loops seamlessly | 8 |
| `A1` | **attack** — one lunge or bite, does not loop | 10 |
| `DT` | **death** — falls and stays down. Ends on the corpse pose. | 12 |

(The D2 sheets carry up to 20 frames for idle; 8 is enough and eight is far
cheaper to generate. Frame counts are ours to choose — the manifest declares
them.)

### The rules that make art usable rather than pretty

- **Transparent background.** PNG with real alpha. No checkerboard, no white, no
  colour to key out. If the generator will not do transparency, the import tool
  can remove a flat background — say so and use a pure magenta `#FF00FF`.
- **No baked shadow.** The engine draws its own shadow pass. A shadow painted
  into the sprite will be drawn twice and will not move with the light.
- **No ground, no scenery, no base.** The creature only. It stands on the game's
  own terrain.
- **One consistent light direction across every asset in the game: from the
  upper-left.** This is a convention we are choosing now and must not drift; two
  creatures lit from opposite sides in the same frame reads as broken.
- **Camera: looking down at roughly 30°**, the same angle for every creature and
  every facing. This is the single easiest thing to get wrong — a generator will
  quietly drift to a side-on or three-quarter view between facings.
- **Consistent size between facings.** The creature must not grow or shrink as it
  turns. Generate all eight from one description in one session.
- **No outline, no rim light, no glow.**

---

## 3. The creatures — grounded in the research, not invented

Taken from **N1 (Predators, signed 22 Aug)** and **R1 (Mythology & Region)**.
These are the descriptions to hand the generator.

### `feral-dog` — the near threat, and the first thing a player meets
Masterless village dogs gone half-wild in a depopulated, corpse-strewn land.
Lean, mangy, ribs showing, matted coats in mud-browns and greys — *not* wolves
and not menacing-looking in themselves. They are the **least wary of humans** of
any canid because they knew people. Medium dogs, not hounds. They should look
pitiable and dangerous at the same time. **They rout when hurt and a torch cows
them**, so they need to read as nervous.

### `wolf` — the woods' threat
A real grey wolf of the Carpathians: big, lean, greyish-brown, long-legged, heavy
through the chest and shoulders, with a low head carriage when moving. **Not a
monster** — no red eyes, no bared exaggerated fangs, no bulk. What is frightening
is that there are several and they have claimed a body. Should read as
*wild and indifferent*, not evil.

### `wild-boar` — the melee hazard
A European wild boar: dark bristled coat, heavy forequarters, low head, small
eyes, tusks. Built like a wedge. It **charges** and must be braced against, so its
silhouette should telegraph forward mass.

### `opportunist` — men
Stragglers and deserters from an atrocity-scarred campaign, drawn by a lit camp.
Period-correct and poor: mud-coloured wool, a hood or a rag cap, a spear, a
hatchet or a knife rather than a sword. Mismatched scavenged pieces of armour at
most. They should look like desperate men, not bandits from a fantasy game.

### `strigoi` — the premise
**The most important piece of art in the game, and the one to get right last.**
A recently dead peasant walking. The horror is that it is *recognisably a person
who died days ago* — not a monster, not a skeleton, not a zombie with rotted-off
features. Romanian folklore's strigoi is **human-faced**. So: ordinary 15th-
century peasant clothing, filthy and grave-stained; a body that has been in the
ground a short time; the face still a face. **Wrong in its stillness and its
carriage**, not in its anatomy. No claws, no glowing eyes, no exposed ribcage.

### `pricolici` — later, and worth knowing about now
The risen corpse of a cruel man returning as an oversized, **silent** black wolf
or great black dog. Deliberately almost indistinguishable from `wolf` at a
glance — **larger, blacker, unnaturally still**. The point is the player cannot
tell, on a dark road, whether the shape will flee his fire. Deferred, but if the
generator is producing wolves anyway, a black oversized one is nearly free.

---

## 4. Delivery — what to drop where

Put whatever comes back in a single folder. Any of these three shapes works and
the import tool sorts it out:

- **Eight separate images**, named `<creature>-<mode>-<n>.png` where `n` is 1–8 in
  the facing order above. Example: `wolf-NU-1.png` … `wolf-NU-8.png`.
- **One horizontal strip** of eight facings, named `<creature>-<mode>.png`.
- **One single image** — it becomes a one-facing still, which is enough to see the
  creature in the game and is a perfectly good first step.

Nothing needs to be the right size, the right aspect, or trimmed. The tool
handles scaling, padding, centring, background removal and the manifest.

**Licensing: every asset gets a line in `CREDITS.md`** — what made it, the date,
the prompt, and the licence. That is project law (Plan §5, M5.3) and it is not
optional, including for generated art.

---

## 5. What the engine does with it, for reference

`d2core/d2asset/png_animation.go`. A creature's art is a grid: **one row per
facing, one column per frame**. A sidecar manifest beside the PNG declares the
grid:

```json
{
  "directions": 8,
  "frames_per_direction": 8,
  "frame_width": 96,
  "frame_height": 96,
  "offset_x": -48,
  "offset_y": 32,
  "origin_at_bottom": true
}
```

With no manifest a PNG is a single still frame, which is why a one-image drop
works. `origin_at_bottom` is what makes a creature stand *on* its tile rather
than hang from it — ground creatures want it true.

**The offsets are not optional (26 Sep 2026).** A frame is placed ONLY through
them: with `origin_at_bottom` the frame hangs from the creature's foot point,
then moves by `offset_x`/`offset_y`. With zeros — what this example used to
show — the frame's bottom-LEFT corner sits on the foot point, so the creature
is drawn half a frame to the right of where it is and lifted by the empty rows
below its feet. That shipped for the Janissary and every creature (~62 px right
and ~31 px up for him), while his bar and his clicks used the true spot. The
rule:

* `offset_x` = minus the figure's centre (the mean x of its opaque pixels, the
  median over the frames) — about `-frame_width/2` for a figure drawn centred;
* `offset_y` = `frame_height` minus the feet row (the lowest opaque row, the
  median over the frames);
* a lying sheet (death, dead) takes its idle sheet's offsets, or the body jumps
  when it falls.

For a calibrated 3D export, the projected ground root is stronger evidence
than the lowest-alpha heuristic: a boot occupies an area of the ground and
its front toe projects below that area's centre. The rebuilt Janissary pins
his measured root to (64,96) in a 128px cell, offsets (-64,+32), for every
motion. His anchor test keeps those exact values and allows up to 8px for
the finite boot footprint; other assets retain their 4px heuristic. Source
projection evidence is retained with the hero, and wrong-offset controls
must still fail.

`tools/spritestitch` writes both from the frames it stitches (pass
`-anchor-from <the idle sheet's .png.json>` for death and dead). `TestGroundSheetsStandOnTheirFootPoint`
(`d2core/d2asset`, runs in the gate and on CI) measures every standing sheet in
`data/strigoi` and fails one that is off by more than 6 px across or 4 down.

Composites try `.png` first, then `.dcc`, then `.dc6`, so **a creature with our
art uses our art and everything else still falls back to D2's.** Replacing one
monster does not wait on replacing all of them.

---

## 6. The HUD's hands — two icons, drop-in

Not a creature, but the same loader. The two icons beside the globes are his
two hands (the left button the blade, the right the torch). Until their art
lands each draws the letter of its key (F, L). The art drops in with no code:

| hand | file | size | frames |
|---|---|---|---|
| blade (left) | `data/strigoi/ui/hands/blade.png` | 48×48 | 1 — no manifest |
| torch (right) | `data/strigoi/ui/hands/torch.png` + `torch.png.json` | 96×48 | 2 — **unlit, then lit**; the frame follows the carried torch |

The torch's manifest is the creatures' format, one row of two cells:

```json
{"directions": 1, "frames_per_direction": 2}
```

Each frame is drawn with its bottom-left corner where Diablo II's skill icon
sat, so the art fills its 48×48 cell exactly; no offsets, no `origin_at_bottom`.
Art that is there and does not fit — another size, a torch with one frame or no
manifest — is reported in the log (`hand art refused`) and the letter is drawn:
a wrong size would draw over the globes. The harness's `ui` state says which
each hand drew: `hand_icons` is `{"left": "F" | "art:blade", "right": "L" |
"art:torch", "right_frame": 0 | 1}`. Only Strigoi's game draws them; `-classic`
draws Diablo II's skill icons. Every file gets its `CREDITS.md` line (§4).

## 7. The combat marker — one icon, drop-in

Not a creature either, and the same loader. While he is **in combat** — his
fight live, a hostile chasing him, his own swing or hit/block reaction
playing, or the three seconds after the last of these (Josh, 30 Sep 2026: "you
can't save while in combat") — he cannot save, and a small marker sits over
the health globe saying so. Until its art lands it is a red `!` on a dark
square with a red edge, in the HUD's font (a glyph, not a letter: every letter
the HUD draws beside the globes is a key — F, L, and C opens his character
panel). The art drops in with no code:

| icon | file | size | frames |
|---|---|---|---|
| in combat | `data/strigoi/ui/combat.png` | 32×32 | 1 — no manifest |

**The brief for the artist:** a small alarm mark for "in combat — you cannot
save", read at a glance over a dark map at night and beside the red life
globe: crossed blades, a drawn sword, a bared-teeth wolf's head, or a
blood-red sigil; Wallachia 1462, not a modern warning triangle. It must read
at 32×32 and stand apart from the red globe under it (a pale or gold edge
helps). Transparent where it is not drawn.

It is drawn with its top-left corner at (54, 468): centred over the globe
(x 54–86), its foot at y 500, above the globe's stone frame, clear of the
globe and of the left hand's icon (x 117–165, y 552–600). Art that is there and
does not fit — another size, more than one frame — is reported in the log
(`combat marker art refused`) and the glyph is drawn. The harness's `ui` state
says what it drew this frame: `combat_marker.drew` is `"!"`, `"art:combat"`,
or `""` (nothing drawn), with the square it drew in (`x`, `y`, `w`, `h`; all
0 when nothing was drawn). It is drawn in both games
(`-classic` too: the status is the save's, and the save is both games').
Every file gets its `CREDITS.md` line (§4).
