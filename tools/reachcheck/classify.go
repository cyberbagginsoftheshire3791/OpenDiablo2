package main

import "strings"

// ConfigVerdict is what one deadcode invocation, in one build configuration,
// says about one symbol.
type ConfigVerdict string

const (
	// CfgReachable: deadcode printed a path from a main function to the symbol.
	CfgReachable ConfigVerdict = "reachable"
	// CfgUnreachable: the symbol exists but no path reaches it. deadcode says
	// "reachable only through reflection" rather than "dead" because every
	// world system registers itself with d2harness and is JSON-encoded, which
	// makes its receiver a reflection-live type; RTA then soundly marks the
	// type's whole method set live. That message is therefore our word for
	// unreachable, not a hint that reflection actually calls it.
	CfgUnreachable ConfigVerdict = "unreachable"
	// CfgNotFound: no such symbol in the program. The register is stale --
	// something was renamed or removed. This shares an exit code with
	// CfgUnreachable and is separated only by the message text, so a rename
	// would otherwise pass the gate silently.
	CfgNotFound ConfigVerdict = "not-found"
	// CfgBroken: the tool could not produce a verdict (packages did not load,
	// binary missing, unrecognised output). Never treat this as a finding.
	CfgBroken ConfigVerdict = "broken"
)

// Verdict is the pair of ConfigVerdicts collapsed into one answer.
type Verdict string

const (
	// VerdictLive: the shipped game reaches it.
	VerdictLive Verdict = "live"
	// VerdictHarnessOnly: the test harness reaches it and the shipped game
	// does not. This is the bug class -- it cost M4.1 (Light.Remove) and
	// M4.3b (the whole notice->pursuit seam), both of which looked wired and
	// were hollow in every playable build.
	VerdictHarnessOnly Verdict = "harness-only"
	// VerdictDead: nothing reaches it in either configuration.
	VerdictDead Verdict = "dead"
	// VerdictMissing: the symbol does not exist. The register is stale.
	VerdictMissing Verdict = "missing"
	// VerdictBroken: the gate failed to measure. Not a verdict about the code.
	VerdictBroken Verdict = "broken"
	// VerdictAnomaly: reachable by default but not under -tags=harness.
	// Adding a build tag only ever adds files, so this should be impossible;
	// if it happens the gate's assumptions are wrong and it says so rather
	// than picking whichever answer looks tidier.
	VerdictAnomaly Verdict = "anomaly"
)

// Classify turns one deadcode invocation into a ConfigVerdict.
//
// Order matters. "not found in program" and "reachable only through
// reflection" both exit 1, so the exit code alone cannot tell a renamed
// symbol from an unreachable one, and a rename must never read as a pass.
// The message text is matched first, and the exit code only breaks ties.
func Classify(exitCode int, output string) ConfigVerdict {
	text := normalise(output)

	switch {
	case strings.Contains(text, "not found in program"):
		return CfgNotFound
	case strings.Contains(text, "packages contain errors"):
		return CfgBroken
	case strings.Contains(text, "is reachable only through reflection"):
		return CfgUnreachable
	case exitCode == 0 && strings.Contains(text, "-->"):
		return CfgReachable
	default:
		return CfgBroken
	}
}

// normalise collapses every run of whitespace to a single space.
//
// deadcode itself does not wrap its messages, but the console that runs it
// may: PowerShell 5.1 broke "is reachable only through reflection" across two
// lines at width 80 while these controls were being measured on 28 Aug 2026.
// A gate that matched the raw text would have read that as an unrecognised
// message and reported the gate broken.
func normalise(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Combine collapses the two configurations into one verdict.
func Combine(def, harness ConfigVerdict) Verdict {
	switch {
	case def == CfgNotFound || harness == CfgNotFound:
		return VerdictMissing
	case def == CfgBroken || harness == CfgBroken:
		return VerdictBroken
	case def == CfgReachable && harness == CfgReachable:
		return VerdictLive
	case def == CfgUnreachable && harness == CfgReachable:
		return VerdictHarnessOnly
	case def == CfgUnreachable && harness == CfgUnreachable:
		return VerdictDead
	default:
		return VerdictAnomaly
	}
}
