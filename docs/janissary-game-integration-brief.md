# Refined Janissary in the playable game

29 September 2026. User-authorized goal: replace the older default hero with the Q3-derived Janissary, anchored on the ground and animated by actual gameplay. The protected source and rejected V10 remain unchanged. Art construction/export review is recorded in the separate art repository's `provenance/janissary-game-v1-brief.md`.

Baseline at 1f60a66f: default hero path already selects Janissary, but old PNG manifests omit offsets. Live monastery measurement places the reported ground point at (399,300) while the visible old hero's feet are roughly (463,278). Exact new offsets come from the game camera and every-frame image measurements, not that old estimate.

## Necessary animation wiring

Source inspection found that Game.Animate handles only the player's swing, through StartCasting. Player.GetAnimationMode then substitutes SC (or town idle) for the requested mode. Hit and death are ignored; blocked blows emit the same visual act as hits. Merely replacing the sheets would leave the new states unreachable.

Implement a visual held-action path on Player for hit, block and death. Keep it separate from IsCasting, which has input/save semantics. Swings retain the existing StartCasting path and its input/save locks; storing the requested castMode fixes their pose. Death halts movement, interrupts pending skill callbacks, plays DT once and holds DD. Later visual actions cannot revive it. Health reaching zero through neglect follows the same path. Repeated same-mode actions restart the sheet. Nonfatal visual reactions preserve pending cast callbacks and restart the pending cast animation after the reaction ends.

[verifier] Held modes precede movement/town selection. Death begins before Step. Avoid repeatedly restarting death from the health check. MapEngine.Advance already runs beneath the death screen and a player's decision pause; only the escape menu freezes it. No new pause workaround is needed.

[verifier] Add ActBlock after existing enum values and emit it only where the resolver has already computed a nonlethal blocked blow. Leave damage, RNG, block spending and combat clocks unchanged. Lethal blocked blows still emit Die. NPC fallback for a block may use the existing hit mode.

[verifier] New visual fields are classified in the save field inventory. They normalize to idle when StandAt reconstructs the living hero, consistent with existing load rule 4. No save format or identity changes. Unit checks cover callback preservation/cancellation, same-mode restarts, death-before-movement and terminal corpse priority. Live acceptance triggers actions through combat, never a test-only animation setter. Attack must assert A1 as well as the attack sheet: SC currently maps to that same sheet and would hide the defect.

## Delivery and evidence

One fixed camera/canvas/ground anchor for all eight directions and all eight states; no empty or clipped frames; dead equals death's final frame. Measured height and offsets must satisfy the existing anchor test before removing BUG-18's exception. Native-size captures show actual walking/running in eight bearings and real combat states. Run positive and broken-offset controls, relevant unit tests, the full gate/reach register and playtest suite, then independent review. Stage and validate in this worktree before integrating with the active main checkout. Do not push without the user's separate standing approval.

Strongest case against: adding visual state can accidentally change input/cast/save behavior. Keep combat actions independent from IsCasting; test callbacks and reload normalization explicitly, and retain all health/combat mechanics.
