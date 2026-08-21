# Test fixture manifest

**Project Strigoi · started 21 August 2026 (M2.3) · canonical home for the rule and the list.** Governed by the Constitution, Article V.1: *Blizzard content never enters the repo — MPQs and anything extracted or derived from them.* `CLAUDE.md` law 4 adds: *new fixtures need explicit justification in a manifest.* This is that manifest. When the `d2-formats` skill lands (M2.2), its `assets/fixture-manifest.md` points here; it does not mirror this file.

## The rule

1. **Decoder tests synthesize their bytes in code.** A test that needs a file builds it from the format spec with the package's own writer (`d2datautils.StreamWriter`, a decoder's `Marshal`), or a hand-assembled byte slice. This is the default and, under Article V.1 as written, the only option for Diablo II formats: an "extracted slice" of an MPQ is derived Blizzard content, however small.
2. **A file under any `testdata/` directory is listed here before it is committed**, with: origin (who made it, how), size, what it exercises, and why it is safe. A fixture not in this list is a bug in the commit that added it.
3. **Compliance is checked with git, not by reading `.gitignore`** — `.gitignore` does not untrack files that were already tracked. The check, run at every phase audit:

       git ls-files | grep -iE '\.(mpq|dc6|dcc|ds1|dt1|cof|pl2|tbl|d2)$'

   Expected output today: exactly one line, `d2common/d2loader/testdata/D.mpq` (row 2 below). Anything else is a violation to fix in the same burst.
4. **Changing this rule** (for example, to allow a justified extracted slice under a fair-use posture, as the original d2-formats scaffold contemplated) is a Constitution amendment — a logged decision first, then the edit, in the same burst.

## Tracked fixtures

| # | Path | Size | Origin | Exercises | Why it is safe |
|---|---|---|---|---|---|
| 1 | `d2common/d2loader/testdata/{A,B,C}/common.txt`, `A/exclusive_a.txt`, `B/exclusive_b.txt`, `C/exclusive_c.txt` | 2 bytes each (6 files) | Upstream, commit `50d40fb5` ("D2loader (#714)", 2020). Hand-written text. | Loader source precedence: the same path present in several filesystem sources; exclusive files per source (`d2common/d2loader/loader_test.go`). | Plain text written by a contributor; no game content. |
| 2 | `d2common/d2loader/testdata/D.mpq` | 891 bytes | Upstream, commit `50d40fb5`. A hand-built minimal MPQ archive containing `exclusive_d.txt` and `dir\common.txt` (asserted by `loader_test.go`, constants `exclusiveD`, `subdirCommonD`). | The MPQ source in the loader precedence chain; subdirectory path handling inside an archive. | **Synthesized**, not extracted: the archive was assembled by a contributor around two tiny text files; it matches the `.gitignore` pattern by extension only. Kept because the loader test needs a real archive and the repo has no MPQ *writer* to build one at test time. If one is ever written, replace this file with in-test synthesis and delete the row. VERIFY on the next M2.3 pass: confirm its two entries with `utils/extract-mpq`. |

## In-code fixtures (no files; listed so the coverage is visible)

| Package | How the bytes are made | Test |
|---|---|---|
| `d2common/d2fileformats/d2animdata` | `synthesizeAnimData()` builds a full 256-block file from a small record set, placing records in their hash blocks; bad-data cases are built by hand (truncated, trailing byte, missing null terminator, over-capacity block, empty). Names are invented (`STRNU1A`, `VLGDD1A`, …), not D2 tokens. | `animdata_test.go` |
| `d2common/d2fileformats/d2cof` | Constructed in code, marshal/unmarshal round-trip. | `cof_test.go` |
| `d2common/d2fileformats/d2dc6` | Byte slices in code. | `dc6_test.go` |
| `d2common/d2fileformats/d2ds1` | Constructed in code, round-trip and layer operations. | `ds1_test.go`, `ds1_layers_test.go`, `layer_test.go` |
| `d2common/d2fileformats/d2pl2` | Constructed in code, round-trip. | `pl2_test.go` |
| `d2common/d2fileformats/d2tbl` | Constructed in code, marshal. | `text_dictionary_test.go` |
| `d2common/d2fileformats/d2dt1` | Subtile constructor only. | `subtile_test.go` |

Decoders with **no test at all** as of `87fe2aa0`: DCC, MPQ, DAT, TXT, font, and DT1 beyond the subtile. Each gets an in-code synthesized test before or with its next change (Constitution VI.1).

## Removed fixtures (history)

| Path | Removed | Why |
|---|---|---|
| `d2common/d2fileformats/d2animdata/testdata/AnimData.d2` (570,304 B) | 21 Aug 2026, M2.3 | Blizzard-derived. Added upstream in `65cce60e` ("adding animdata loader (#718)", 2020-09-08). Verified on 21 Aug 2026 against the installed 1.14b MPQs with the engine's own decoder: exactly the size of `data\global\animdata.d2` in `d2exp.mpq`, different SHA-256 — another patch's copy or a re-marshaled derivative; either way derived. Contained 3,558 real animation tokens. Replaced by `synthesizeAnimData()`. |
| `d2common/d2fileformats/d2animdata/testdata/BadData.d2` (570,368 B) | 21 Aug 2026, M2.3 | A scrambled sibling of the file above (same size ±64 bytes); derived from it. Replaced by the hand-built bad-data cases. |

These files remain in git history as inherited upstream content (every fork and mirror of OpenDiablo2 carries them). Whether to rewrite the fork's history before the first friends build is a separate decision, not taken here; the public repo's HEAD is clean of them from this commit on.
