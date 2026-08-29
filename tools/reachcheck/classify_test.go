package main

import "testing"

// The strings below are VERBATIM deadcode output, captured on 28 Aug 2026 at
// 9:35 PM CT against HEAD 9d9611ba, by
//
//	strigoi-harness-runs\reach-controls.ps1
//
// which ran `deadcode -whylive=<symbol> .` and then
// `deadcode -tags=harness -whylive=<symbol> .` over four symbols chosen so
// that every branch of Classify has one case known true and one known false.
// They are not paraphrased and must not be tidied: Constitution VI.4(b) says
// an instrument that cannot tell a known-true case from a known-false one
// produces no findings, and these tests are where that check lives from now
// on. If a deadcode upgrade changes the wording, these fail first -- which is
// the point, because the alternative is a gate that silently stops working.

const (
	// CONTROL 1, positive. d2world.Clock.Advance, default tags, exit 0.
	// The game calls it once a frame from Game.advanceWorld.
	outLivePath = `                   github.com/OpenDiablo2/OpenDiablo2.main
  static@L0023 --> github.com/OpenDiablo2/OpenDiablo2/d2app.Create
  static@L0121 --> github.com/OpenDiablo2/OpenDiablo2/d2app.App.parseArguments
 dynamic@L0236 --> github.com/pkg/profile.Start$10
 dynamic@L0307 --> github.com/OpenDiablo2/OpenDiablo2/d2app.App.advance
  static@L0419 --> github.com/OpenDiablo2/OpenDiablo2/d2app.App.advanceOnce
  static@L0433 --> github.com/OpenDiablo2/OpenDiablo2/d2core/d2screen.ScreenManager.Advance
 dynamic@L0100 --> github.com/OpenDiablo2/OpenDiablo2/d2game/d2gamescreen.Game.Advance
  static@L0314 --> github.com/OpenDiablo2/OpenDiablo2/d2game/d2gamescreen.Game.advanceWorld
  static@L0388 --> github.com/OpenDiablo2/OpenDiablo2/d2core/d2world.Clock.Advance`

	// CONTROL 2, negative. d2gamescreen.Game.Notice, default tags, exit 1.
	// Under -tags=harness the same symbol returns exit 0 with a path whose
	// last edge is d2app.App.harnessAddActionTools$3$1 -> Game.Notice.
	outReflection = `deadcode: github.com/OpenDiablo2/OpenDiablo2/d2game/d2gamescreen.Game.Notice is reachable only through reflection`

	// CONTROL 2b. The same message as PowerShell 5.1 rendered it, wrapped
	// mid-sentence at column 80. Classify must give the same answer.
	outReflectionWrapped = `deadcode.exe : deadcode: github.com/OpenDiablo2/OpenDiablo2/d2game/d2gamescreen.Game.Notice is reachable only through
reflection`

	// CONTROL 4. A symbol that does not exist -- the rename case. Exit 1,
	// same as CONTROL 2, which is why the text and not the code decides.
	outNotFound = `deadcode: function "github.com/OpenDiablo2/OpenDiablo2/d2game/d2gamescreen.Game.ZzNoSuchMethodQq" not found in program`

	// The build-is-broken case. A gate that reads this as "dead" invents a
	// finding out of a compile error.
	outPackagesBroken = `deadcode: packages contain errors`
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		exit int
		out  string
		want ConfigVerdict
	}{
		{"live path, exit 0", 0, outLivePath, CfgReachable},
		{"reflection message, exit 1", 1, outReflection, CfgUnreachable},
		{"reflection message wrapped by the console", 1, outReflectionWrapped, CfgUnreachable},
		{"not found in program", 1, outNotFound, CfgNotFound},
		{"packages contain errors", 1, outPackagesBroken, CfgBroken},
		{"empty output with exit 0", 0, "", CfgBroken},
		{"unrecognised output", 3, "deadcode: some future message", CfgBroken},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Classify(c.exit, c.out); got != c.want {
				t.Fatalf("Classify(%d, %q) = %q, want %q", c.exit, c.out, got, c.want)
			}
		})
	}
}

// TestClassifyDoesNotTrustTheExitCodeAlone is the specific check that the
// remediation plan called out: dead and renamed both exit 1, so a gate keyed
// on the exit code cannot separate them and a rename passes silently.
func TestClassifyDoesNotTrustTheExitCodeAlone(t *testing.T) {
	unreachable := Classify(1, outReflection)
	notFound := Classify(1, outNotFound)

	if unreachable == notFound {
		t.Fatalf("exit 1 collapsed two different states into %q; a renamed symbol would pass the gate", unreachable)
	}
}

func TestCombine(t *testing.T) {
	cases := []struct {
		name         string
		def, harness ConfigVerdict
		want         Verdict
	}{
		{"game reaches it", CfgReachable, CfgReachable, VerdictLive},
		{"only the harness reaches it", CfgUnreachable, CfgReachable, VerdictHarnessOnly},
		{"nothing reaches it", CfgUnreachable, CfgUnreachable, VerdictDead},
		{"renamed under default tags", CfgNotFound, CfgReachable, VerdictMissing},
		{"renamed under harness tags", CfgReachable, CfgNotFound, VerdictMissing},
		{"gate broken", CfgBroken, CfgReachable, VerdictBroken},
		{"impossible: a build tag removed reachability", CfgReachable, CfgUnreachable, VerdictAnomaly},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Combine(c.def, c.harness); got != c.want {
				t.Fatalf("Combine(%q, %q) = %q, want %q", c.def, c.harness, got, c.want)
			}
		})
	}
}

// TestCombineSeparatesHarnessOnlyFromDead is the whole point of running
// deadcode twice. Both states are exit 1 under default tags; only the second
// configuration tells them apart, and only the harness-only one is a bug.
func TestCombineSeparatesHarnessOnlyFromDead(t *testing.T) {
	harnessOnly := Combine(Classify(1, outReflection), Classify(0, outLivePath))
	dead := Combine(Classify(1, outReflection), Classify(1, outReflection))

	if harnessOnly != VerdictHarnessOnly {
		t.Fatalf("harness-only case classified as %q", harnessOnly)
	}

	if dead != VerdictDead {
		t.Fatalf("dead case classified as %q", dead)
	}
}
