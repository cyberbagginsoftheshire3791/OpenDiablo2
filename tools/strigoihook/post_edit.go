package main

import (
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
)

const (
	buildTimeout = 4 * time.Minute
	vetTimeout   = 2 * time.Minute
	fmtTimeout   = 30 * time.Second
)

const d2commonReminder = "Reminder: you edited d2common/. The d2-formats skill (.claude/skills/d2-formats/) documents the decoders with path:line citations at a pinned commit. If behaviour, constants, or version gates changed, update the matching reference and PROGRESS.md in this burst, or say why not."

const depReminder = "Reminder: go.mod/go.sum changed. One dependency bump per commit, never `go get -u` (CLAUDE.md law 7); build, vet, test, and play to the Rogue Encampment before the next bump."

// runPostEdit is the PostToolUse hook for Write/Edit/MultiEdit. Failures go
// to stderr with exit 2 (Claude Code shows them to Claude); a green result
// and any reminders go back as additionalContext.
func runPostEdit(root string, r io.Reader, w, errw io.Writer) int {
	in, err := readInput(r)
	if err != nil {
		fmt.Fprintf(errw, "strigoihook post-edit: %v\n", err)

		return 1
	}

	target := in.editedPath()
	if target == "" {
		return 0
	}

	target = strings.ReplaceAll(target, "\\", "/")

	rel, underRoot := relPath(root, target)
	if !underRoot {
		// A file outside the repo is not ours to build or vet.
		return 0
	}

	var notes, failures []string

	switch {
	case strings.HasSuffix(rel, ".go"):
		notes, failures = checkGoFile(root, rel)
	case rel == "go.mod" || rel == "go.sum":
		notes = append(notes, depReminder)

		build := runCmd(root, buildTimeout, "go", "build", "./...")
		if build.ok {
			notes = append(notes, fmt.Sprintf("go build ./... ok (%s)", build.duration.Round(100*time.Millisecond)))
		} else {
			failures = append(failures, "go build ./... failed after the go.mod/go.sum change:\n"+truncate(build.output, 6000))
		}
	}

	if strings.HasPrefix(rel, "d2common/") && !strings.Contains(rel, "_test.go") {
		notes = append(notes, d2commonReminder)
	}

	if len(failures) > 0 {
		fmt.Fprintf(errw, "strigoihook post-edit (%s):\n%s\n", rel, strings.Join(failures, "\n\n"))

		return 2
	}

	if len(notes) == 0 {
		return 0
	}

	var out postToolUseOutput
	out.HookSpecificOutput.HookEventName = "PostToolUse"
	out.HookSpecificOutput.AdditionalContext = "strigoihook (" + rel + "): " + strings.Join(notes, " · ")

	if err := json.NewEncoder(w).Encode(out); err != nil {
		fmt.Fprintf(errw, "strigoihook post-edit: %v\n", err)

		return 1
	}

	return 0
}

// checkGoFile formats the file, builds the module, and vets the package.
func checkGoFile(root, rel string) (notes, failures []string) {
	list := runCmd(root, fmtTimeout, "gofmt", "-l", rel)

	switch {
	case !list.ok:
		failures = append(failures, "gofmt -l failed (syntax error?):\n"+truncate(list.output, 4000))
	case list.output != "":
		write := runCmd(root, fmtTimeout, "gofmt", "-w", rel)
		if write.ok {
			notes = append(notes, "gofmt reformatted the file — re-read it before the next edit")
		} else {
			failures = append(failures, "gofmt -w failed:\n"+truncate(write.output, 4000))
		}
	default:
		notes = append(notes, "gofmt ok")
	}

	build := runCmd(root, buildTimeout, "go", "build", "./...")
	if build.ok {
		notes = append(notes, fmt.Sprintf("go build ./... ok (%s)", build.duration.Round(100*time.Millisecond)))
	} else {
		failures = append(failures, "go build ./... failed:\n"+truncate(build.output, 6000))
	}

	pkg := "./" + path.Dir(rel)
	if pkg == "./." {
		pkg = "."
	}

	vet := runCmd(root, vetTimeout, "go", "vet", pkg)
	if vet.ok {
		notes = append(notes, fmt.Sprintf("go vet %s ok", pkg))
	} else {
		failures = append(failures, "go vet "+pkg+" failed:\n"+truncate(vet.output, 6000))
	}

	return notes, failures
}
