package main

import (
	"strings"
	"testing"
)

// The register is hand-maintained, so these tests are the only thing standing
// between it and quiet rot. They check shape, not reachability -- reachability
// needs the deadcode binary and the whole program, and this file has to pass
// on CI, where neither is available.

func TestRegisterIsWellFormed(t *testing.T) {
	if len(Register) == 0 {
		t.Fatal("the register is empty, so the gate checks nothing")
	}

	seen := make(map[string]bool, len(Register))

	for _, e := range Register {
		if seen[e.Symbol] {
			t.Errorf("%s: listed twice", e.Symbol)
		}

		seen[e.Symbol] = true

		if !strings.HasPrefix(e.Symbol, ModulePath+"/") {
			t.Errorf("%s: not a symbol of this module, so deadcode will report it not found", e.Symbol)
		}

		if strings.TrimSpace(e.Why) == "" {
			t.Errorf("%s: no reason given. A bucket with no reason is a guess that will outrank the next person's judgement", e.Symbol)
		}

		switch e.Expect {
		case VerdictLive, VerdictHarnessOnly, VerdictDead:
		default:
			t.Errorf("%s: expects %q, which is a gate state rather than a claim about the code", e.Symbol, e.Expect)
		}
	}
}

func TestBucketsMatchTheirExpectedVerdict(t *testing.T) {
	for _, e := range Register {
		switch e.Bucket {
		case BucketWire:
			if e.Expect != VerdictLive {
				t.Errorf("%s: in the wire bucket but expects %q. Wire means the game must call it", e.Symbol, e.Expect)
			}

			if e.Milestone != "" {
				t.Errorf("%s: wired symbols do not name a milestone; they are already done", e.Symbol)
			}
		case BucketDefer:
			if e.Expect == VerdictLive {
				t.Errorf("%s: deferred but expected live. If the game already calls it, it is wired, not deferred", e.Symbol)
			}

			if strings.TrimSpace(e.Milestone) == "" {
				t.Errorf("%s: deferred without naming the milestone that picks it up. That is the whole difference between a deferral and a leak", e.Symbol)
			}
		case BucketObserve:
			if e.Expect == VerdictLive {
				t.Errorf("%s: in the observe bucket but the game calls it. Move it to wire so the gate defends the call site", e.Symbol)
			}

			if e.Milestone != "" {
				t.Errorf("%s: observe means nobody is ever going to wire it. A milestone here means it is a deferral", e.Symbol)
			}
		case BucketDelete:
			if e.Expect == VerdictLive {
				t.Errorf("%s: marked for deletion but the game calls it", e.Symbol)
			}
		default:
			t.Errorf("%s: unknown bucket %q", e.Symbol, e.Bucket)
		}
	}
}

// TestObserveHoldsNoWorldChangingVerb defends the gate's only loophole.
//
// "observe" says harness-only is the right answer for a symbol, which is
// exactly the sentence a hollow milestone would like to write about itself.
// The rule is that observe covers reads and DIAL writes and nothing else, so
// a name that announces a change to the world's contents does not belong
// there however convenient it would be.
//
// This is a heuristic on names, not a proof, and it will not catch a
// world-changing verb with a quiet name. It is here because the alternative
// -- trusting that nobody ever files an inconvenient symbol under observe to
// make the gate go green -- is the failure this whole tool exists to stop.
func TestObserveHoldsNoWorldChangingVerb(t *testing.T) {
	mutating := []string{"Add", "Remove", "Release", "Despawn", "Consume", "Spawn", "Chase", "Kill"}

	for _, e := range Register {
		if e.Bucket != BucketObserve {
			continue
		}

		// Take the text after the LAST dot. Cutting at the first one leaves
		// the receiver type on the front, and "Spawns.SetMorale" then reads
		// as a method beginning "Spawn" -- which is exactly what this check
		// did on its first run.
		full := shortSymbol(e.Symbol)
		method := full[strings.LastIndex(full, ".")+1:]

		for _, verb := range mutating {
			if strings.HasPrefix(method, verb) {
				t.Errorf("%s: %q changes the world, so it is a deferral with a milestone, not harness surface", e.Symbol, method)
			}
		}
	}
}

// TestRegisterCarriesItsOwnPositiveControl guards the failure mode that would
// make this gate worthless without ever going red: if the analysis silently
// stopped reaching anything -- a build tag typo, a wrong -pkg, a deadcode
// upgrade that changes its entry-point rules -- then every symbol would
// measure unreachable, every observe, defer and delete row would still match,
// and the gate would report OK while measuring nothing at all.
//
// At least one symbol the game demonstrably calls must be on the register, so
// that failure turns the gate red instead of quiet. Same reason
// strigoi_find_path reports straight_line_clear: a route that arrives proves
// nothing unless the straight line did not.
func TestRegisterCarriesItsOwnPositiveControl(t *testing.T) {
	live := 0

	for _, e := range Register {
		if e.Bucket == BucketWire {
			live++
		}
	}

	if live == 0 {
		t.Fatal("no symbol on the register is expected live, so a gate that reached nothing at all would still report OK")
	}

	if live < 3 {
		t.Errorf("only %d wire entries; one accessor moving could remove the gate's whole positive control", live)
	}
}

// TestTheM43bSeamIsDefended is a named regression guard rather than a general
// rule. M4.3b shipped, was audited, and was found hollow because nothing
// joined awareness to pursuit outside the harness. The fix was
// startChasesForTheAware, and the only thing that stops it being deleted or
// orphaned again without anyone noticing is these two symbols being on the
// register as wire.
func TestTheM43bSeamIsDefended(t *testing.T) {
	need := map[string]bool{
		sym(pkgWorld, "Notice.AwarePairs"): false,
		sym(pkgWorld, "Pursuit.Chase"):     false,
	}

	for _, e := range Register {
		if _, ok := need[e.Symbol]; ok && e.Bucket == BucketWire {
			need[e.Symbol] = true
		}
	}

	for s, found := range need {
		if !found {
			t.Errorf("%s is not registered as wire; the seam M4.3b was reopened over is undefended", s)
		}
	}
}

func TestRegisterMarkdownRendersEveryEntry(t *testing.T) {
	md := RegisterMarkdown()

	for _, e := range Register {
		if !strings.Contains(md, shortSymbol(e.Symbol)) {
			t.Errorf("%s: missing from the rendered register", e.Symbol)
		}
	}
}
