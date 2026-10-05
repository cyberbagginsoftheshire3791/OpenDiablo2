# F8 feedback and crash reports (Level 1, 5 Oct 2026)

Ruled by Josh on 5 Oct 2026: the game tells us what happened without anyone
having to ask him.

## For Josh: F8

Press **F8** anywhere -- in a game, on the main menu (Strigoi's and
`-classic`'s), in the World Editor. The game freezes the picture you are
looking at, pauses (in a single-player game the world stops exactly as it does
under the escape menu), and opens one line:

> What happened, or what did you want?

Type a sentence. **Enter** saves it with the picture; **Esc** cancels and saves
nothing. A short "Feedback saved" line confirms it. That is all: you never need
to find the file, because a Claude session can read it (below).

F8 was free: Diablo II bound it to "skill 8", which nothing in this game uses,
and the World Editor uses no F-key.

## If the game crashes

If the game crashes, it writes a crash report before it closes. The next time
you start it, the main menu says so on one line across the top:

> The last run crashed -- a report was saved to ...\crashes\20261005-211502

If the game ended some other way -- it froze and you ended it from Task
Manager, or the power went -- the next launch notices that too:

> The last run did not close cleanly -- a report was saved to ...

Nothing to do; the line goes away once you leave the menu for a game. (It
comes back on the next launch only if that run also ends badly.)

## Where the files go

Everything is under `%LOCALAPPDATA%\Strigoi\` (`C:\Users\josht\AppData\Local\Strigoi`):

| Folder | What is in it |
|---|---|
| `feedback\<yyyyMMdd-HHmmss>\` | `note.txt` (what you typed), `shot.png` (the frame as you saw it, before the box), `state.json` (build, screen, game day and clock, where you stood, zoom, fog, what was open, window size, the last 200 lines of the log) |
| `crashes\<yyyyMMdd-HHmmss>\` | a crash the game caught: `stack.txt` (every goroutine's stack, the one that panicked first), `log-tail.txt` (last 300 log lines), `state.json` (what was on screen) |
| `crashes\<stamp>-unclean\` | a run the NEXT launch found had not closed cleanly: `marker.json` (when it started, its pid and build), `log-tail.txt`, `state.json`, and `stack.txt` when the Go runtime wrote a fatal error (a panic nothing caught, a concurrent map write) |
| `crashes\<stamp>-unclean-harness\` | the same for a `-harness` game (a playtest or a hand-started harness game); kept apart so a killed playtest is never called your crash |
| `running\` | one marker per game running now (`<pid>.json`, `<pid>.crash`); a clean exit removes its own |

The log line `FEEDBACK dir=<folder> text="<note>"` goes into `strigoi.log`
beside the ROUND, PACE and WISH lines, so a note lands next to the numbers it
belongs to. (`wish <text>` in the console still works; F8 is the same idea with
a picture and no console.)

## For a Claude session: reading them

```
powershell -NoProfile -ExecutionPolicy Bypass -File C:\Users\josht\Projects\strigoi-harness-runs\collect-feedback.ps1
```

lists every feedback and crash folder newer than the script's last run (its
state file is `strigoi-harness-runs\collect-feedback.state.json`) and prints
each note, its screen/day/clock, and each crash's first lines of stack.
`-Since 2026-10-05` (or any date/time) lists from then instead; `-All` lists
everything; `-IncludeHarness` adds the `-unclean-harness` folders; `-NoSave`
does not move the "last run" stamp. `shot.png` is beside each note -- open it.

From a harness game, the `feedback` provider (`strigoi_get_system_state
feedback`) reports the box, the last folder saved and the menu's notice.

## How it works

- **The box** is the App's (`d2app/feedback.go`), above every screen and the
  console in input priority (`d2enum.PriorityTop`): F8 opens it, and open it
  takes every key and click, so nothing typed reaches the game. The frame is
  read back (`Surface.Screenshot`, on the draw goroutine) in `App.render` after
  the screen, the UI and the gui manager are drawn and before the box is. The
  text box is the native menu's (`d2ui.NewMenuTextboxWide`, 400 characters).
  In a game, `Game.SetFeedbackHold` holds the world: `screenLive` is false,
  `WorldHeldBy` says `feedback`.
- **Crash reports** (`d2common/d2report`, ebiten-free, tested headless). ebiten
  runs the game's Update and Draw on a goroutine of its own, and its errgroup
  does not hand a panic back to `main` -- so `main.go`'s `recover()` never saw a
  game-loop panic (it does not, and never did; read at master `dc91255f`).
  `App.advance` and `App.update` now defer `crashGuard`, which writes the
  folder and exits 1. A panic on any other goroutine is uncaught; for that the
  runtime's fatal-error output is pointed at `running\<pid>.crash`
  (`runtime/debug.SetCrashOutput`), and the next launch turns it into the
  report.
- **The running marker** is one file per process, held open for the whole run.
  On Windows a file another process holds open cannot be renamed, so a
  start-up that CAN rename a marker has proved its owner is gone, however it
  ended, and no pid is trusted. Harness and player runs only report their own
  kind. `STRIGOI_REPORT_HOME` moves the whole tree; the playtest launcher sets
  it to each test's private home.
- **Clean exits** -- the window's close, QUIT, the console's `quit`,
  `strigoi_quit` -- remove the marker (`d2report.Exit` / `EndRun`). A
  `Logger.Fatal` does not: it is reported next launch as "did not close
  cleanly", with the fatal line in the log tail.

## Not done (Level 1)

- No last-frame picture in a crash folder (the draw goroutine is the one that
  may have broken).
- The menu notice is not dismissed by a key; it shows on the main menu until
  a game starts.
- A panic on a goroutine other than the game loop and main (the in-process
  game server, the harness's MCP server) gets the runtime's stack only, not a
  state.json.
