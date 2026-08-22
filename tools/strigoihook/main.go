// Command strigoihook is the Claude Code hook runner for Project Strigoi.
//
// It is wired up by .claude/settings.json and invoked as
//
//	go run ./tools/strigoihook <event>
//
// with the hook's JSON payload on stdin. Events:
//
//	pre-write      PreToolUse for Write/Edit/MultiEdit/NotebookEdit — refuses
//	               Diablo II binary formats and the extraction drop zones
//	               (Constitution, Article V); asks before the guardrails
//	               themselves are edited.
//	pre-bash       PreToolUse for Bash/PowerShell — refuses bulk dependency
//	               upgrades (CLAUDE.md law 7) and shell writes of protected
//	               formats; asks before destructive git operations.
//	post-edit      PostToolUse for Write/Edit/MultiEdit — gofmt, go build,
//	               go vet after a Go edit; reminds about the d2-formats skill
//	               after a d2common/ edit.
//	session-start  SessionStart — prints the live facts (branch, tree, build,
//	               Article V status, .claude/FOCUS.md) as context.
//	check-fixtures Article V compliance: every tracked file with a protected
//	               extension must be listed in docs/fixtures-allowlist.txt.
//	               Exit 1 on a violation (used by CI).
//
// The rules live in guard.go and are unit-tested; this file is plumbing.
// The program depends on the standard library only, so a broken package
// elsewhere in the module cannot take the guardrails down with it.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// hookInput is the subset of the Claude Code hook payload this program reads.
type hookInput struct {
	HookEventName string         `json:"hook_event_name"`
	ToolName      string         `json:"tool_name"`
	ToolInput     map[string]any `json:"tool_input"`
	Cwd           string         `json:"cwd"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: strigoihook <pre-write|pre-bash|post-edit|session-start|check-fixtures>")
		os.Exit(2)
	}

	root := projectDir()

	var code int

	switch os.Args[1] {
	case "pre-write":
		code = runPreWrite(root, os.Stdin, os.Stdout, os.Stderr)
	case "pre-bash":
		code = runPreBash(os.Stdin, os.Stdout, os.Stderr)
	case "post-edit":
		code = runPostEdit(root, os.Stdin, os.Stdout, os.Stderr)
	case "session-start":
		code = runSessionStart(root, os.Stdout, os.Stderr)
	case "check-fixtures":
		code = runCheckFixtures(root, os.Stdout, os.Stderr)
	default:
		fmt.Fprintf(os.Stderr, "strigoihook: unknown event %q\n", os.Args[1])
		code = 2
	}

	os.Exit(code)
}

// projectDir is the repository root: CLAUDE_PROJECT_DIR when Claude Code sets
// it, otherwise the working directory (settings.json cd's there first).
func projectDir() string {
	if dir := os.Getenv("CLAUDE_PROJECT_DIR"); dir != "" {
		return filepath.Clean(dir)
	}

	dir, err := os.Getwd()
	if err != nil {
		return "."
	}

	return dir
}

// readInput decodes the hook payload. An empty stdin is not an error: the
// hook then has nothing to judge and allows the call.
func readInput(r io.Reader) (hookInput, error) {
	var in hookInput

	data, err := io.ReadAll(r)
	if err != nil {
		return in, err
	}

	if len(strings.TrimSpace(string(data))) == 0 {
		return in, nil
	}

	if err := json.Unmarshal(data, &in); err != nil {
		return in, fmt.Errorf("hook payload is not JSON: %w", err)
	}

	return in, nil
}

// inputString returns a string field of tool_input, or "".
func (in hookInput) inputString(key string) string {
	if in.ToolInput == nil {
		return ""
	}

	s, _ := in.ToolInput[key].(string)

	return s
}

// editedPath is the file a Write/Edit/MultiEdit/NotebookEdit call targets.
func (in hookInput) editedPath() string {
	if p := in.inputString("file_path"); p != "" {
		return p
	}

	return in.inputString("notebook_path")
}

// preToolUseOutput is the JSON Claude Code reads from a PreToolUse hook.
type preToolUseOutput struct {
	HookSpecificOutput struct {
		HookEventName            string `json:"hookEventName"`
		PermissionDecision       string `json:"permissionDecision"`
		PermissionDecisionReason string `json:"permissionDecisionReason"`
	} `json:"hookSpecificOutput"`
}

// postToolUseOutput is the JSON Claude Code reads from a PostToolUse hook.
type postToolUseOutput struct {
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

// emitDecision writes a deny/ask decision for a PreToolUse hook. An allow is
// silence: no output, exit 0.
func emitDecision(w io.Writer, v verdict) int {
	if v.decision == allow {
		return 0
	}

	var out preToolUseOutput
	out.HookSpecificOutput.HookEventName = "PreToolUse"
	out.HookSpecificOutput.PermissionDecision = string(v.decision)
	out.HookSpecificOutput.PermissionDecisionReason = v.reason

	enc := json.NewEncoder(w)
	if err := enc.Encode(out); err != nil {
		// If the JSON cannot be written, fall back to the exit-code protocol
		// so a deny is never silently downgraded to an allow.
		fmt.Fprintln(os.Stderr, v.reason)

		return 2
	}

	return 0
}

func runPreWrite(root string, r io.Reader, w, errw io.Writer) int {
	in, err := readInput(r)
	if err != nil {
		fmt.Fprintf(errw, "strigoihook pre-write: %v\n", err)

		return 1
	}

	path := in.editedPath()
	if path == "" {
		return 0
	}

	return emitDecision(w, classifyWrite(root, path))
}

func runPreBash(r io.Reader, w, errw io.Writer) int {
	in, err := readInput(r)
	if err != nil {
		fmt.Fprintf(errw, "strigoihook pre-bash: %v\n", err)

		return 1
	}

	cmd := in.inputString("command")
	if cmd == "" {
		return 0
	}

	return emitDecision(w, classifyBash(cmd))
}
