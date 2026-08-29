# The reachability gate

`tools/reachcheck` answers one question about one symbol at a time: **can the
shipped game reach this, or only the playtest harness?**

## Why it exists

An exported symbol can be reachable from the harness and not from the game.
The system then looks wired and is hollow, and the playtest goes green because
the script drives both halves itself.

It has happened twice.

* **M4.1** shipped with `Light.Remove` exported and callable only by a test. A
  placed fire could not be put out in any playable build. The milestone was
  reopened.
* **M4.3b** shipped with `Notice` working out awareness, `Pursuit` able to
  route a chase, and **nothing joining them outside the harness**. A spawned
  wolf saw the player and stood there. The playtest passed because the M4.3b
  script called `strigoi_watch` and the M4.3a script called `strigoi_pursue`,
  so both halves were exercised separately, by scripts, and never by the game.
  Found by an audit, one milestone late.

A lesson did not stop it twice. So it is a gate.

## Why plain `deadcode` does not find it

Every world system registers itself into the process-global `d2harness`
registry and reports through `HarnessState() map[string]interface{}`, which is
JSON-encoded. That makes each receiver a **reflection-live** runtime type, and
RTA soundly marks the entire method set of such a type reachable. Run bare,
`deadcode` reports every one of our orphaned accessors as *"reachable only
through reflection"*.

The architecture built to make the systems observable is exactly what blinds
the tool that would catch harness-only code.

## What works

Ask about **one symbol at a time**, in **two build configurations**, and read
the pair:

```
deadcode -whylive=SYM .
deadcode -tags=harness -whylive=SYM .
```

| default tags | `-tags=harness` | verdict | meaning |
|---|---|---|---|
| exit 0, a path is printed | exit 0 | `live` | the game calls it |
| exit 1, *"is reachable only through reflection"* | **exit 0** | `harness-only` | **the bug class** |
| exit 1, reflection | exit 1, reflection | `dead` | nothing calls it in either build |
| either config, *"not found in program"* | — | `missing` | renamed or deleted; the register is stale |
| *"packages contain errors"* | — | `broken` | never a verdict about the code |

Two things that will bite, both encoded in `classify_test.go`:

* **The exit code alone cannot tell `dead` from `missing`** — both are exit 1.
  The message text decides, or a rename passes the gate silently.
* **The console may wrap the message.** PowerShell 5.1 broke *"is reachable
  only through reflection"* across a line while these controls were being
  measured, and did it at a different column for each symbol name. `Classify`
  collapses whitespace before matching for exactly this reason. An ad-hoc awk
  version of the same check, written the same evening without that step,
  mis-tagged 9 of 68 symbols.

## The four buckets

The tool measures reachability. The **register** (`register.go`) says what
each symbol's reachability *ought* to be, and that is a decision somebody made,
not a fact the tool discovered.

| bucket | expects | means |
|---|---|---|
| `wire` | `live` | the game must call this. These rows are the gate's teeth |
| `observe` | `harness-only` / `dead` | exists so the harness can read or tune it |
| `defer` | `harness-only` / `dead` | the game should drive it eventually; **`Milestone` names who** |
| `delete` | `harness-only` / `dead` | a seam nobody took, scheduled for removal |

**`observe` is the gate's only loophole**, so it has a rule: a symbol may be
`observe` **only if it neither changes world state nor is named in a signed
assertion**. Reading a value is `observe`. Writing a **dial** is `observe` —
tuning is what the harness is for. Adding, removing or releasing anything **in
the world** is not, however convenient it would be to say so, and goes in
`defer` with a milestone against its name.
`TestObserveHoldsNoWorldChangingVerb` is a name heuristic that defends this,
and it is a heuristic, not a proof.

The gate fails on **any** disagreement, in either direction. A symbol that
quietly becomes wired is also a register that has stopped being true.

## The register defends itself

Two guards worth knowing about, because they are the difference between a gate
and a green light:

* **`TestRegisterCarriesItsOwnPositiveControl`.** If the analysis ever
  silently stopped reaching anything — a build tag typo, a wrong `-pkg`, a
  `deadcode` upgrade that changes its entry-point rules — every symbol would
  measure unreachable, every `observe`, `defer` and `delete` row would still
  match, and the gate would report OK while measuring nothing at all. At least
  three `wire` rows must exist so that failure turns it red instead of quiet.
* **`TestTheM43bSeamIsDefended`.** `Notice.AwarePairs` and `Pursuit.Chase`
  must be registered `wire` by name. They are the two ends of
  `startChasesForTheAware`, and nothing else stops M4.3b going hollow a second
  time without anyone noticing.

## Running it

From the repository root:

```
go run ./tools/reachcheck                      # the gate
go run ./tools/reachcheck -list                # render the register as markdown
go run ./tools/reachcheck -only Notice -v      # one system, with full deadcode output
```

`deadcode` must be on `PATH`, or pass `-deadcode=<path>`. Install it with
`go install golang.org/x/tools/cmd/deadcode@v0.49.0` (the version these
controls were measured against).

There is also `strigoi-harness-runs\reach-gate.ps1`, which runs it into a log
the same way `gate.ps1` and `playtest.ps1` do. Launch minimized, poll the log.

**Measured cost, 28 Aug 2026, warm build cache on the laptop: a mean of 4.7 s
per `deadcode` invocation** over 136 invocations. Two invocations per symbol,
so the whole register is roughly ten minutes serial and about three at
`-jobs 4`. An earlier spike reported ~2.2 s and that number was optimistic.

## What this gate does not do

* **It is a curated allowlist, not a sweep.** Because the default report is
  reflection-blinded, `deadcode` cannot enumerate harness-only symbols for us.
  The register is hand-maintained and grows when a feature ships. **A symbol
  that is not on the register is not checked, and its absence is not evidence
  of anything.**
* **It does not run on CI.** `deadcode` is not installed on the runners, and
  the cold-cache cost was **not measured** — only the warm figure above. Do not
  promise a CI step on the strength of the warm number. What *does* run on CI
  is `go test ./tools/reachcheck/`, which checks the classifier's controls and
  the register's shape; neither needs the `deadcode` binary.
* **It says nothing about whether reachable code is correct**, only about
  whether it is reached.
* **It does not check unexported functions, package-level functions, struct
  fields, or any package outside `d2core/d2world` and `d2game/d2gamescreen`.**
  The register's current 68 symbols came from the exported method sets of
  `Game`, `Clock`, `Light`, `Meters`, `Pursuit`, `Notice` and `Spawns`.

## Adding a symbol

Add a row to `Register` in `register.go` with a bucket, the verdict you expect,
and a `Why` that someone who cannot read the call graph can check. Deferrals
need a milestone. Then run the gate: if your expectation is wrong, that is a
finding, and it belongs in `state.md` before the row is changed to match.

**The register is the claim. The measurement is not allowed to edit it
quietly.**
