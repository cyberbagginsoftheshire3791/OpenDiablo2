package main

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// decision is what a PreToolUse hook tells Claude Code to do with a call.
type decision string

const (
	allow decision = "allow"
	deny  decision = "deny"
	ask   decision = "ask"
)

// verdict is a decision plus the reason Claude (or Josh, for ask) sees.
type verdict struct {
	decision decision
	reason   string
}

var allowed = verdict{decision: allow}

// protectedExt mirrors the Strigoi block of .gitignore: the Diablo II binary
// formats. Compared lower-cased, so the upper-case patterns in .gitignore are
// covered too.
var protectedExt = map[string]bool{
	".mpq": true,
	".dc6": true,
	".dcc": true,
	".ds1": true,
	".dt1": true,
	".cof": true,
	".pl2": true,
	".tbl": true,
	".d2":  true,
}

// protectedDirs mirrors the root-anchored directories of the same block: the
// drop zones for extracted Blizzard content, plus the playtest harness's
// output drop zone (screenshots are renders of Blizzard art — P3 spec §3.9).
// Nothing is written there from Claude Code.
var protectedDirs = []string{"extracted", "assets-d2", "harness-runs"}

// guardFiles are the guardrails themselves. Editing them is Josh's call, so
// the hook asks instead of deciding.
var guardFiles = []string{
	".gitignore",
	".gitattributes",
	".claude/settings.json",
	".github/workflows/ci.yml",
	"docs/fixtures-manifest.md",
	"docs/fixtures-allowlist.txt",
}

// guardDirs are guarded as a whole (prefix match on the slash-normalised
// repo-relative path).
var guardDirs = []string{"tools/strigoihook/"}

const articleV = "Constitution, Article V: Blizzard content never enters the repo — MPQs and anything extracted or derived from them — and new content never targets Diablo II binary formats (the asset ratchet only tightens)."

// normalise turns either slash convention into forward slashes and cleans
// the result. Pure string work, so the rules behave the same on Windows
// (where Claude Code runs) and on Linux (where CI runs the tests).
func normalise(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")

	return path.Clean(p)
}

// isAbs recognises both a POSIX root and a drive-letter path, whatever the
// host platform.
func isAbs(p string) bool {
	if strings.HasPrefix(p, "/") {
		return true
	}

	return len(p) >= 3 && p[1] == ':' && (p[2] == '/' || p[2] == '\\') &&
		((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z'))
}

// relPath returns p relative to root with forward slashes and whether p is
// inside root. A path outside root comes back normalised, so the extension
// checks still apply to it. The prefix comparison is case-insensitive: the
// repo lives on a Windows filesystem.
func relPath(root, p string) (rel string, underRoot bool) {
	root = normalise(root)
	p = normalise(p)

	if !isAbs(p) {
		return p, true
	}

	if strings.EqualFold(p, root) {
		return ".", true
	}

	prefix := root + "/"
	if len(p) > len(prefix) && strings.EqualFold(p[:len(prefix)], prefix) {
		return p[len(prefix):], true
	}

	return p, false
}

// classifyWrite judges a Write/Edit/MultiEdit/NotebookEdit target.
func classifyWrite(root, p string) verdict {
	rel, _ := relPath(root, p)

	ext := strings.ToLower(path.Ext(rel))
	if protectedExt[ext] {
		return verdict{
			decision: deny,
			reason: fmt.Sprintf("Refusing to write %s: *%s is a Diablo II binary format. %s If this write is deliberate, Josh makes it outside Claude Code.",
				rel, ext, articleV),
		}
	}

	first := rel
	if i := strings.Index(rel, "/"); i >= 0 {
		first = rel[:i]
	}

	for _, dir := range protectedDirs {
		if strings.EqualFold(first, dir) {
			return verdict{
				decision: deny,
				reason: fmt.Sprintf("Refusing to write %s: /%s/ is the git-ignored drop zone for extracted Blizzard content and nothing is written there from Claude Code. %s",
					rel, dir, articleV),
			}
		}
	}

	for _, f := range guardFiles {
		if strings.EqualFold(rel, f) {
			return verdict{
				decision: ask,
				reason:   fmt.Sprintf("%s is one of the repo's guardrails (hooks, CI, Article V compliance). Changing it is Josh's call — confirm before the edit goes through.", rel),
			}
		}
	}

	for _, d := range guardDirs {
		if len(rel) > len(d) && strings.EqualFold(rel[:len(d)], d) {
			return verdict{
				decision: ask,
				reason:   fmt.Sprintf("%s is part of strigoihook, the guard itself. Changing it is Josh's call — confirm before the edit goes through.", rel),
			}
		}
	}

	return allowed
}

// Shell-command rules. Each is a regexp over the raw command string; the
// first match wins, denies before asks. They are deliberately plain: the
// goal is to stop the obvious moves (a bulk upgrade, a copy of an MPQ into
// the tree, a force-push) and explain why, not to parse shell.
const protectedExtAlt = `(?:mpq|dc6|dcc|ds1|dt1|cof|pl2|tbl|d2)`

// cmdStart matches the beginning of a (sub)command.
const cmdStart = `(?:^|[\s;&|(]|\$\()`

type bashRule struct {
	re       *regexp.Regexp
	decision decision
	reason   string
}

var bashRules = []bashRule{
	{
		re:       regexp.MustCompile(cmdStart + `go\s+get\s+(?:[^\s;&|]+\s+)*-u(?:=|\s|$)`),
		decision: deny,
		reason:   "Bulk dependency upgrades are forbidden (CLAUDE.md law 7: one dependency bump per commit, never `go get -u`). Bump one module explicitly — `go get module@version` — and build, vet, test, and play before the next one.",
	},
	{
		re:       regexp.MustCompile(`(?i)` + cmdStart + `go\s+get\s+(?:[^\s;&|]+\s+)*all(?:\s|$)`),
		decision: deny,
		reason:   "`go get all` upgrades the whole dependency graph (CLAUDE.md law 7: one dependency bump per commit). Bump one module explicitly instead.",
	},
	{
		// A write verb or a redirection with a protected-format target.
		re:       regexp.MustCompile(`(?i)(?:` + cmdStart + `(?:cp|mv|install|tee|dd|touch|curl|wget|7z|7za|unzip|tar|copy|move|xcopy|robocopy|copy-item|move-item|new-item|set-content|out-file|add-content|invoke-webrequest|iwr|expand-archive|cpi|mi)\b[^;&|]*|>+\s*)["']?[^\s"'<>|;&]*\.` + protectedExtAlt + `\b`),
		decision: deny,
		reason:   "The command writes a Diablo II binary format (.mpq/.dc6/.dcc/.ds1/.dt1/.cof/.pl2/.tbl/.d2). " + articleV + " If this is deliberate, Josh does it outside Claude Code.",
	},
	{
		re:       regexp.MustCompile(cmdStart + `git\s+add\s+(?:[^\s;&|]+\s+)*(?:-f|--force)(?:\s|$)`),
		decision: deny,
		reason:   "`git add --force` is the one way an ignored file gets tracked; the Strigoi block of .gitignore is load-bearing (" + articleV + ") Stage files the normal way. If an ignored file must be tracked, that is a Constitution amendment and Josh's call.",
	},
	{
		re:       regexp.MustCompile(`(?i)` + cmdStart + `git\s+add\s+(?:[^\s;&|]+\s+)*["']?[^\s"';&|]*\.` + protectedExtAlt + `\b`),
		decision: deny,
		reason:   "Staging a Diablo II binary format is refused. " + articleV,
	},
	{
		re:       regexp.MustCompile(cmdStart + `git\s+push\b[^;&|]*\s(?:-f|--force|--force-with-lease|--force-if-includes)\b`),
		decision: ask,
		reason:   "Force-pushing rewrites shared history. History rewrites (including the deferred purge of inherited fixtures) are Josh's decision — confirm.",
	},
	{
		re:       regexp.MustCompile(cmdStart + `git\s+(?:filter-branch|filter-repo)\b`),
		decision: ask,
		reason:   "Rewriting history is Josh's decision (the fixture purge is parked until before the first friends build) — confirm.",
	},
	{
		re:       regexp.MustCompile(cmdStart + `git\s+reset\s+(?:[^\s;&|]+\s+)*--hard\b`),
		decision: ask,
		reason:   "`git reset --hard` discards uncommitted work in the tree — confirm with Josh.",
	},
	{
		re:       regexp.MustCompile(cmdStart + `git\s+clean\s+(?:[^\s;&|]+\s+)*-[A-Za-z]*f`),
		decision: ask,
		reason:   "`git clean -f` deletes untracked files (which can include rh.ini, local notes, and staged deliveries) — confirm with Josh.",
	},
	{
		re:       regexp.MustCompile(cmdStart + `git\s+(?:checkout\s+--|restore(?:\s+--worktree)?)\s+(?:\.|\*|:/)(?:\s|$)`),
		decision: ask,
		reason:   "Restoring the whole tree discards every uncommitted change — confirm with Josh.",
	},
	{
		re:       regexp.MustCompile(cmdStart + `git\s+(?:branch\s+-D|stash\s+(?:drop|clear))\b`),
		decision: ask,
		reason:   "Deleting a branch or dropping stashes loses work that may not be anywhere else — confirm with Josh.",
	},
	{
		re:       regexp.MustCompile(cmdStart + `git\s+commit\b[^;&|]*\s--no-verify\b`),
		decision: ask,
		reason:   "`--no-verify` bypasses the repo's checks — confirm with Josh.",
	},
	{
		re:       regexp.MustCompile(cmdStart + `rm\s+-[A-Za-z]*[rR][A-Za-z]*\s+(?:[^\s;&|]+\s+)*["']?(?:\.|\*|\.git|/|~)["']?(?:\s|$)`),
		decision: ask,
		reason:   "Recursive deletion of the tree, .git, or a root — confirm with Josh.",
	},
	{
		re:       regexp.MustCompile(`(?i)(?:sed\s+-i|>+\s*|tee\s+|set-content|out-file|add-content)[^;&|]*(?:\.gitignore|\.gitattributes|\.claude/settings\.json|workflows/ci\.yml|fixtures-allowlist\.txt|fixtures-manifest\.md|tools/strigoihook)`),
		decision: ask,
		reason:   "The command rewrites one of the repo's guardrails from the shell. Changing the guardrails is Josh's call — confirm.",
	},
	{
		re:       regexp.MustCompile(cmdStart + `(?:go\s+run\s+\S*|\S*)extract-mpq\b`),
		decision: ask,
		reason:   "extract-mpq writes extracted Blizzard content. Inside the repo it may only land under /extracted/ (git-ignored); confirm the output directory with Josh. " + articleV,
	},
}

// classifyBash judges a Bash/PowerShell command string.
func classifyBash(cmd string) verdict {
	for _, rule := range bashRules {
		if rule.re.MatchString(cmd) {
			return verdict{decision: rule.decision, reason: rule.reason}
		}
	}

	return allowed
}
