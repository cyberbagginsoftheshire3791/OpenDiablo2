package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	gitTimeout         = 20 * time.Second
	sessionBuildEnvVar = "STRIGOI_HOOK_SKIP_BUILD"
	focusFile          = ".claude/FOCUS.md"
	maxDirtyListed     = 8
)

const sessionLaw = "Law (Constitution v1.1, condensed): one goal per burst, stated in one sentence before working · verify against the live system before asserting (code outranks memory and docs) · closeout = state.md + Notion + tracker + a same-burst listing · Blizzard content never enters the repo · one dependency bump per commit · case-against before any direction change · Josh decides, Claude advises."

// runSessionStart prints the live facts as plain text; Claude Code adds a
// SessionStart hook's stdout to Claude's context.
func runSessionStart(root string, w, errw io.Writer) int {
	var b strings.Builder

	fmt.Fprintf(&b, "strigoihook session-start — live facts for Project Strigoi (read before CLAUDE.md's frozen ones)\n")
	fmt.Fprintf(&b, "repo: %s\n", root)

	branch := runCmd(root, gitTimeout, "git", "rev-parse", "--abbrev-ref", "HEAD")
	head := runCmd(root, gitTimeout, "git", "rev-parse", "--short", "HEAD")
	status := runCmd(root, gitTimeout, "git", "status", "-sb", "--porcelain=v1")
	last := runCmd(root, gitTimeout, "git", "log", "-1", "--format=%s (%cr)")

	if !branch.ok || !head.ok || !status.ok {
		fmt.Fprintf(&b, "git: unavailable (%s)\n", firstLine(branch.output+status.output))
	} else {
		fmt.Fprintf(&b, "branch: %s @ %s · %s\n", branch.output, head.output, describeTracking(status.output))
		fmt.Fprintf(&b, "tree: %s\n", describeTree(status.output))
		fmt.Fprintf(&b, "last commit: %s\n", last.output)
	}

	goVersion := runCmd(root, gitTimeout, "go", "version")
	if goVersion.ok {
		fmt.Fprintf(&b, "go: %s", strings.TrimPrefix(goVersion.output, "go version "))

		if os.Getenv(sessionBuildEnvVar) == "" {
			build := runCmd(root, buildTimeout, "go", "build", "./...")
			if build.ok {
				fmt.Fprintf(&b, " · go build ./... OK (%s)\n", build.duration.Round(100*time.Millisecond))
			} else {
				fmt.Fprintf(&b, " · go build ./... FAILED — fix before anything else:\n%s\n", indent(truncate(build.output, 2000)))
			}
		} else {
			fmt.Fprintf(&b, " · build skipped (%s set)\n", sessionBuildEnvVar)
		}
	} else {
		fmt.Fprintf(&b, "go: unavailable — %s\n", firstLine(goVersion.output))
	}

	report, violations, err := checkFixtures(root)
	switch {
	case err != nil:
		fmt.Fprintf(&b, "Article V: check failed (%v)\n", err)
	case len(violations) > 0:
		fmt.Fprintf(&b, "Article V: VIOLATION — %s\n", report)
	default:
		fmt.Fprintf(&b, "Article V: %s\n", report)
	}

	if focus, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(focusFile))); err == nil {
		fmt.Fprintf(&b, "focus (%s):\n%s\n", focusFile, indent(strings.TrimSpace(string(focus))))
	} else {
		fmt.Fprintf(&b, "focus: %s missing — state.md in the claude.ai project is the fallback\n", focusFile)
	}

	fmt.Fprintln(&b, sessionLaw)

	if _, err := io.WriteString(w, b.String()); err != nil {
		fmt.Fprintf(errw, "strigoihook session-start: %v\n", err)

		return 1
	}

	return 0
}

// describeTracking reads the first line of `git status -sb`:
// "## master...origin/master [ahead 1, behind 2]".
func describeTracking(status string) string {
	line := firstLine(status)
	if !strings.HasPrefix(line, "## ") {
		return "tracking unknown"
	}

	line = strings.TrimPrefix(line, "## ")

	switch {
	case strings.Contains(line, "["):
		i := strings.Index(line, "[")

		return strings.TrimSuffix(strings.TrimSpace(line[i+1:]), "]") + " of " + strings.TrimSpace(line[:i])
	case strings.Contains(line, "..."):
		return "in sync with " + line[strings.Index(line, "...")+3:]
	default:
		return "no upstream"
	}
}

// describeTree summarises the porcelain lines after the branch header.
func describeTree(status string) string {
	lines := strings.Split(status, "\n")
	if len(lines) <= 1 {
		return "clean"
	}

	var modified, untracked int

	var listed []string

	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "" {
			continue
		}

		if strings.HasPrefix(l, "??") {
			untracked++
		} else {
			modified++
		}

		if len(listed) < maxDirtyListed {
			listed = append(listed, strings.TrimSpace(l))
		}
	}

	if modified+untracked == 0 {
		return "clean"
	}

	s := fmt.Sprintf("%d modified/staged, %d untracked — %s", modified, untracked, strings.Join(listed, "; "))
	if modified+untracked > maxDirtyListed {
		s += "; …"
	}

	return s
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}

	return strings.TrimSpace(s)
}

func indent(s string) string {
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}
