# Focus -- printed into every session by the SessionStart hook

Updated: 2026-09-05 (13:55 CT). **This file is capped at one screen.** It
held 528 lines until today and told a session more about finished
milestones than about the next hour; the body was frozen into
`docs/focus-archive-2026-09-05.md` and is not appended to again. **If you
add a paragraph here, delete one.** If you edit this file at all, bump the
date on the line above -- the old one drifted two days and nobody noticed.

## Where status actually lives -- read this before anything else

**This file is NOT the status.** Status is `next-session-prompt.md`,
rewritten every burst, in `OneDrive\Desktop\Video Game\Claude doc outputs\`
and mirrored to the claude.ai project as `claude/next-session-prompt.md`.

| File | What it is | How it changes |
|---|---|---|
| `next-session-prompt.md` | **STATUS.** What is true now, what is next, what is owed, the live traps | Rewritten every burst |
| `state.md` | Durable reference: paths, standing engine facts, the provider rules, parked items, Josh's constraints | Rarely, only when a fact changes |
| `state-history.md` | The numbered burst log | Appended, never edited, never read at session start |

Then Notion (workstreams, gates, decisions, lessons) and the Constitution,
which lives at the **root** of the Video Game folder, not under
`Claude doc outputs\`.

## Three saved skills carry the procedures -- invoke them, do not re-derive

They live on Josh's claude.ai account, so they reach every surface, and they
are deliberately NOT copied into `.claude/skills/` (two writable homes for
one procedure drift). The spine of a burst, in order:

1. `brief-then-attack` -- before any milestone step, research topic or
   finding is built or filed. One session writes it; a second, independent
   agent tries to break it. **It has changed the answer every time it has
   run**, most recently ten A-severity findings across two briefs on 4 Sep.
2. `strigoi-measure-first` -- at the START of a build burst whose design
   depends on engine behaviour. Throwaway scaffold, deleted before the
   commit. **A measurement that contradicts the brief beats the brief.**
3. `strigoi-burst-closeout` -- gate, the FULL reachability register, every
   playtest script, negative controls, commit, CI, Notion, the tracker,
   the handoff, the history log.

They compose and do not substitute: closeout is not the gate, and the brief
is not section 0. **If a skill is not in the session's skill list, say so**
rather than improvising a shortened version of it.

## The build order -- identities, not a schedule

    M4.1 -> M4.2 -> M4.3a -> M4.3b -> M4.5 -> M4.7 -> M4.6
    with M4.4 floating -- slot it whenever.

**M4.5 is whole, with named steps**, and steps 1-4 are done. **M4.7 is the
corpse machine** -- the corpse state machine, the per-band rising roll
against soul pressure, the edge-arrival floor and the rite window; it shares
content with M4.6 and drives `spawns.open_bodies`, which M4.3b built as a
settable stand-in for exactly this. **M4.4 is the HUD milestone** (clock UI,
meter HUD, first scripted event) and it floats.

**Two short bursts are approved outside the build order.**
(a) **The worldgen default branch** (3caff9f3-d21e-81a2-a0a7-edd4dcf0578c):
the wilderness generates only for the E1/S1/W1 town presets, so an unseeded
launch produces a village in a void. Phase 4's DoD is a FRIEND cloning and
surviving a night, and a friend will not pass seed 1462. **Read the branch
before promising the fix is small** -- the default case may exist because
those presets have no wilderness stamp to place.
(b) **Map fires.** Ask 1 is DONE (`dd2b7d99`, `tools/mapfirecount`: the
village places about twenty-one lit objects and **not one is lit in the mode
it is placed in**). Ask 2 is ruled: a map fire is a hearth someone else lit,
radius 8, Burn negative, no new category. **Asks 3, 4 and 5 are open and no
lighting engine code may be written until 4 is answered.**

## Do not

`go get -u` · write any *.mpq/*.dc6/*.dcc/*.ds1/*.dt1/*.cof/*.pl2/*.tbl/*.d2
· write into `/harness-runs/` or run dirs from Claude Code · create a
capitalised `Docs/` · commit CRLF · let a test binary link ebiten (check
`go list -deps`) · claim "on disk" or "pushed" without a same-burst listing
· put gameplay logic in the harness · read the wall clock in a world system
· run a writing git command (`git status` included -- it takes the index
lock) from the mounted Linux shell.

**Parked items and Josh's standing constraints are in `state.md`**, which is
where they belong and where they will not go stale. Do not reopen one from
memory.
