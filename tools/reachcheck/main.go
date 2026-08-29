// Command reachcheck is Project Strigoi's reachability gate.
//
// # The bug class
//
// An exported symbol can be reachable from the playtest harness and not from
// the shipped game. The system then looks wired and is hollow: M4.1 shipped
// with Light.Remove callable only by a test, and M4.3b shipped with the whole
// notice->pursuit seam joined by the playtest script rather than by the game,
// so a spawned wolf saw the player and stood there while the suite went
// green. Both were found by an audit, one milestone late. A lesson did not
// stop it twice, so this is a gate.
//
// # Why plain deadcode does not find it
//
// Every world system registers itself into the process-global d2harness
// registry and reports through HarnessState() map[string]interface{}, which
// is JSON-encoded. That makes each receiver a reflection-live runtime type,
// and RTA soundly marks the entire method set of such a type reachable. Run
// bare, deadcode reports every orphaned accessor as "reachable only through
// reflection" -- the architecture built to make the systems observable is
// exactly what blinds the tool that would catch harness-only code.
//
// # What works
//
// Ask about one symbol at a time, in two build configurations, and read the
// pair. `deadcode -whylive=SYM .` and `deadcode -tags=harness -whylive=SYM .`
// disagree in exactly one direction -- unreachable by default, reachable with
// the harness compiled in -- and that disagreement is the bug class. See
// classify.go for the table and classify_test.go for the controls.
//
// # What this gate does NOT do
//
// It is a curated allowlist, not a sweep. Because the default report is
// reflection-blinded, deadcode cannot enumerate harness-only symbols for us,
// so register.go is maintained by hand and grows when a feature ships. A
// symbol that is not on the register is not checked, and its absence is not
// evidence of anything.
//
// Usage, from the repository root:
//
//	go run ./tools/reachcheck
//	go run ./tools/reachcheck -list
//	go run ./tools/reachcheck -only Notice -v
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	exitOK      = 0
	exitFailed  = 1
	exitBrokenG = 2
)

func main() {
	var (
		deadcodeBin = flag.String("deadcode", "deadcode", "path to the deadcode binary (golang.org/x/tools/cmd/deadcode)")
		pkg         = flag.String("pkg", ".", "the main package to analyse, in go list notation")
		only        = flag.String("only", "", "check only register entries whose symbol contains this substring")
		list        = flag.Bool("list", false, "print the register as a markdown table and exit without measuring")
		verbose     = flag.Bool("v", false, "print the full deadcode output for every entry")
		jobs        = flag.Int("jobs", 4, "how many deadcode invocations to run at once")
	)

	flag.Parse()

	if *list {
		fmt.Print(RegisterMarkdown())
		os.Exit(exitOK)
	}

	entries := Register
	if *only != "" {
		entries = nil

		for _, e := range Register {
			if strings.Contains(e.Symbol, *only) {
				entries = append(entries, e)
			}
		}

		if len(entries) == 0 {
			fmt.Fprintf(os.Stderr, "reachcheck: -only %q matched no register entry\n", *only)
			os.Exit(exitBrokenG)
		}
	}

	os.Exit(run(entries, *deadcodeBin, *pkg, *jobs, *verbose))
}

// result is one measured register entry.
type result struct {
	Entry   Entry
	Def     ConfigVerdict
	Harness ConfigVerdict
	Verdict Verdict
	DefOut  string
	HarnOut string
	Elapsed time.Duration
}

func run(entries []Entry, bin, pkg string, jobs int, verbose bool) int {
	if jobs < 1 {
		jobs = 1
	}

	start := time.Now()
	results := make([]result, len(entries))

	var (
		wg   sync.WaitGroup
		sem  = make(chan struct{}, jobs)
		lock sync.Mutex
		seen int
	)

	for i, e := range entries {
		wg.Add(1)

		go func(i int, e Entry) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			t0 := time.Now()
			defCode, defOut := invoke(bin, pkg, e.Symbol, "")
			harnCode, harnOut := invoke(bin, pkg, e.Symbol, "harness")

			r := result{
				Entry:   e,
				Def:     Classify(defCode, defOut),
				Harness: Classify(harnCode, harnOut),
				DefOut:  defOut,
				HarnOut: harnOut,
				Elapsed: time.Since(t0),
			}
			r.Verdict = Combine(r.Def, r.Harness)
			results[i] = r

			lock.Lock()
			seen++
			fmt.Fprintf(os.Stderr, "\rreachcheck: measured %d/%d", seen, len(entries))
			lock.Unlock()
		}(i, e)
	}

	wg.Wait()
	fmt.Fprintln(os.Stderr)

	return report(results, start, verbose)
}

// invoke runs deadcode once and returns its exit code and combined output.
// deadcode writes its verdict to stderr and its path table to stdout, so both
// streams are captured together and classified as one text.
func invoke(bin, pkg, symbol, tags string) (int, string) {
	args := make([]string, 0, 3)
	if tags != "" {
		args = append(args, "-tags="+tags)
	}

	args = append(args, "-whylive="+symbol, pkg)

	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()

	code := 0
	if err != nil {
		var ee *exec.ExitError
		if ok := asExitError(err, &ee); ok {
			code = ee.ExitCode()
		} else {
			// The binary is missing or could not start. That is a broken
			// gate, never a statement about the code, so say so in words
			// Classify will read as broken rather than as a verdict.
			return -1, "reachcheck: could not run deadcode: " + err.Error()
		}
	}

	return code, string(out)
}

func asExitError(err error, target **exec.ExitError) bool {
	ee, ok := err.(*exec.ExitError)
	if ok {
		*target = ee
	}

	return ok
}

func report(results []result, start time.Time, verbose bool) int {
	var (
		broken   []result
		mismatch []result
	)

	fmt.Printf("%-8s  %-12s  %-12s  %s\n", "BUCKET", "EXPECTED", "MEASURED", "SYMBOL")
	fmt.Println(strings.Repeat("-", 100))

	for _, r := range results {
		mark := "  "

		switch {
		case r.Verdict == VerdictBroken:
			mark = "!!"

			broken = append(broken, r)
		case r.Verdict != r.Entry.Expect:
			mark = "->"

			mismatch = append(mismatch, r)
		}

		fmt.Printf("%s%-6s  %-12s  %-12s  %s\n", mark, r.Entry.Bucket, r.Entry.Expect, r.Verdict, shortSymbol(r.Entry.Symbol))

		if verbose {
			fmt.Printf("      default: %s\n", firstLine(r.DefOut))
			fmt.Printf("      harness: %s\n", firstLine(r.HarnOut))
		}
	}

	fmt.Println(strings.Repeat("-", 100))
	fmt.Printf("%d entries in %s\n", len(results), time.Since(start).Round(time.Millisecond))

	if len(broken) > 0 {
		fmt.Println()
		fmt.Println("GATE BROKEN -- no verdict was produced for these, so nothing below is a finding:")

		for _, r := range broken {
			fmt.Printf("  %s\n    %s\n", shortSymbol(r.Entry.Symbol), firstLine(r.DefOut+" "+r.HarnOut))
		}

		return exitBrokenG
	}

	if len(mismatch) == 0 {
		fmt.Println("\nOK -- every registered symbol is where the register says it is.")

		return exitOK
	}

	fmt.Println()
	fmt.Println("REGISTER DISAGREES WITH THE PROGRAM:")

	sort.Slice(mismatch, func(i, j int) bool {
		return mismatch[i].Entry.Symbol < mismatch[j].Entry.Symbol
	})

	for _, r := range mismatch {
		fmt.Printf("\n  %s\n    expected %s, measured %s\n    %s\n", shortSymbol(r.Entry.Symbol), r.Entry.Expect, r.Verdict, explain(r))
	}

	return exitFailed
}

// explain says, in words a reader who cannot read the call graph can act on,
// which direction a symbol moved and what to do about it.
func explain(r result) string {
	switch {
	case r.Verdict == VerdictMissing:
		return "The symbol no longer exists. It was renamed or deleted and the register was not updated, so this row has been checking nothing. Fix the register."
	case r.Verdict == VerdictAnomaly:
		return "Reachable with default tags but not with -tags=harness. A build tag only ever adds files, so this should be impossible and the gate's assumptions need re-checking before anyone acts on it."
	case r.Entry.Expect == VerdictLive && r.Verdict == VerdictHarnessOnly:
		return "WENT HOLLOW. The game called this and now only the harness does. This is the M4.1 and M4.3b failure exactly: the playtest will still pass, because the script drives it directly. Find the game call site that disappeared."
	case r.Entry.Expect == VerdictLive && r.Verdict == VerdictDead:
		return "WENT DEAD. Nothing calls this in either configuration. Find the call site that disappeared."
	case r.Verdict == VerdictLive:
		return "Now wired. This is good news, but the register still says otherwise -- move the row to the wire bucket so the gate starts defending the new call site."
	default:
		return "Reachability changed. Re-derive the row's bucket and reason before changing the register to match; the register is the claim, not the measurement."
	}
}

func shortSymbol(s string) string {
	return strings.TrimPrefix(s, ModulePath+"/")
}

func firstLine(s string) string {
	line := normalise(s)
	if len(line) > 160 {
		line = line[:160] + "..."
	}

	return line
}
