package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const testRoot = "/repo"

func TestClassifyWrite(t *testing.T) {
	cases := []struct {
		name string
		root string
		path string
		want decision
	}{
		{"go source", testRoot, "/repo/d2common/d2fileformats/d2dc6/dc6.go", allow},
		{"markdown", testRoot, "/repo/docs/architecture-as-found.md", allow},
		{"relative go source", testRoot, "d2core/d2asset/asset_manager.go", allow},
		{"skill reference", testRoot, "/repo/.claude/skills/d2-formats/references/mpq.md", allow},
		{"mpq", testRoot, "/repo/d2common/d2loader/testdata/D.mpq", deny},
		{"upper-case MPQ", testRoot, "/repo/D2DATA.MPQ", deny},
		{"dc6 outside the repo", testRoot, "/tmp/export/cursor.dc6", deny},
		{"dcc", testRoot, "/repo/x/y.dcc", deny},
		{"ds1", testRoot, "ds1/town.ds1", deny},
		{"dt1", testRoot, "tiles.DT1", deny},
		{"cof", testRoot, "/repo/a.cof", deny},
		{"pl2", testRoot, "/repo/a.pl2", deny},
		{"tbl", testRoot, "/repo/string.tbl", deny},
		{"d2 animdata", testRoot, "/repo/d2common/d2fileformats/d2animdata/testdata/AnimData.d2", deny},
		{"d2s is not protected", testRoot, "/repo/save.d2s", allow},
		{"extracted drop zone", testRoot, "/repo/extracted/data/global/readme.txt", deny},
		{"assets-d2 drop zone", testRoot, "/repo/assets-d2/notes.md", deny},
		{"harness-runs drop zone", testRoot, "/repo/harness-runs/20260824-220000/shot.png", deny},
		{"extracted nested elsewhere is fine", testRoot, "/repo/docs/extracted/notes.md", allow},
		{"gitignore asks", testRoot, "/repo/.gitignore", ask},
		{"settings asks", testRoot, "/repo/.claude/settings.json", ask},
		{"ci workflow asks", testRoot, "/repo/.github/workflows/ci.yml", ask},
		{"allowlist asks", testRoot, "/repo/docs/fixtures-allowlist.txt", ask},
		{"manifest asks", testRoot, "/repo/docs/fixtures-manifest.md", ask},
		{"guard source asks", testRoot, "/repo/tools/strigoihook/guard.go", ask},
		{"windows path to go file", `C:\Users\josht\Projects\OpenDiablo2`, `C:\Users\josht\Projects\OpenDiablo2\main.go`, allow},
		{"windows path to mpq", `C:\Users\josht\Projects\OpenDiablo2`, `C:\Users\josht\Projects\OpenDiablo2\patch_d2.mpq`, deny},
		{"windows mixed slashes to drop zone", `C:\Users\josht\Projects\OpenDiablo2`, `C:/Users/josht/Projects/OpenDiablo2/extracted/x.txt`, deny},
		{"windows path to guard file", `C:\Users\josht\Projects\OpenDiablo2`, `C:\Users\josht\Projects\OpenDiablo2\.claude\settings.json`, ask},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyWrite(tc.root, tc.path)
			if got.decision != tc.want {
				t.Fatalf("classifyWrite(%q, %q) = %s (%s), want %s", tc.root, tc.path, got.decision, got.reason, tc.want)
			}

			if got.decision != allow && got.reason == "" {
				t.Fatalf("a %s needs a reason", got.decision)
			}
		})
	}
}

func TestClassifyBash(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		want decision
	}{
		// Everyday commands stay open.
		{"build", "go build ./... && go vet ./... && go test ./...", allow},
		{"single bump", "go get github.com/google/uuid@v1.6.0", allow},
		{"go mod tidy", "go mod tidy", allow},
		{"git status", "git status -sb", allow},
		{"git add all", "git add -A && git commit -F msg.txt", allow},
		{"git push", "git push origin master", allow},
		{"git push -u sets upstream", "git push -u origin feature", allow},
		{"listing mpqs is fine", `ls "C:/Program Files (x86)/Diablo II"/*.mpq`, allow},
		{"reading an mpq is fine", "go run ./tmp-verify/main.go d2exp.mpq", allow},
		{"grep for the extension", "git ls-files | grep -iE '\\.(mpq|dc6)$'", allow},
		{"rm a build artefact", "rm -f OpenDiablo2.exe", allow},
		{"rm -rf a temp dir", "rm -rf tmp-verify-animdata", allow},
		{"git clean dry run", "git clean -n", allow},
		{"git checkout a branch", "git checkout -b m2.5-otto", allow},
		{"git restore one file", "git restore go.sum", allow},
		{"reading settings", "cat .claude/settings.json", allow},
		{"commit normally", "git commit -m 'x'", allow},
		{"unrelated -u flag", "git push -u origin master", allow},
		{"diff in d2common", "git diff -- d2common", allow},

		// Bulk upgrades are denied (CLAUDE.md law 7).
		{"go get -u", "go get -u ./...", deny},
		{"go get -u chained", "cd repo && go get -u ./... && go build", deny},
		{"go get -u=patch", "go get -u=patch ./...", deny},
		{"go get -u single module", "go get -u github.com/google/uuid", deny},
		{"go get -t -u", "go get -t -u ./...", deny},
		{"go get all", "go get all", deny},

		// Writing a protected format is denied (Article V).
		{"cp an mpq into the tree", `cp "/c/Program Files (x86)/Diablo II/d2data.mpq" ./testdata/`, deny},
		{"cp with explicit target", "cp d2data.mpq d2common/d2loader/testdata/E.mpq", deny},
		{"redirect into dc6", "go run ./tools/export > out/cursor.dc6", deny},
		{"redirect into DS1 upper", "./gen > town.DS1", deny},
		{"curl -o tbl", "curl -o string.tbl https://example.invalid/string.tbl", deny},
		{"powershell copy-item", `Copy-Item 'C:\Program Files (x86)\Diablo II\d2exp.mpq' .\`, deny},
		{"powershell set-content", "Set-Content -Path a.pl2 -Value x", deny},
		{"touch an animdata", "touch d2common/d2fileformats/d2animdata/testdata/AnimData.d2", deny},
		{"7z extract mpq", "7z x d2data.mpq -oextracted", deny},
		{"git add force", "git add -f d2common/d2loader/testdata/E.mpq", deny},
		{"git add --force anything", "git add --force rh.exe", deny},
		{"git add dc6", "git add sprites/cursor.dc6", deny},

		// Destructive or history-rewriting moves ask Josh.
		{"force push", "git push --force origin master", ask},
		{"force push short", "git push -f", ask},
		{"force-with-lease", "git push --force-with-lease origin master", ask},
		{"filter-repo", "git filter-repo --path d2common/d2fileformats/d2animdata/testdata --invert-paths", ask},
		{"filter-branch", "git filter-branch --index-filter 'x' HEAD", ask},
		{"reset hard", "git reset --hard origin/master", ask},
		{"clean -fd", "git clean -fd", ask},
		{"clean -xdf", "git clean -xdf", ask},
		{"checkout dot", "git checkout -- .", ask},
		{"restore dot", "git restore .", ask},
		{"branch -D", "git branch -D feature", ask},
		{"stash drop", "git stash drop", ask},
		{"no-verify", "git commit --no-verify -m x", ask},
		{"rm -rf .git", "rm -rf .git", ask},
		{"rm -rf dot", "rm -rf .", ask},
		{"rm -rf star", "rm -rf *", ask},
		{"sed into gitignore", "sed -i 's/^\\*\\.mpq$//' .gitignore", ask},
		{"redirect over ci", "cat new.yml > .github/workflows/ci.yml", ask},
		{"append to allowlist", "echo x.mpq >> docs/fixtures-allowlist.txt", ask},
		{"extract-mpq", "go run ./utils/extract-mpq d2data.mpq extracted/", ask},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyBash(tc.cmd)
			if got.decision != tc.want {
				t.Fatalf("classifyBash(%q) = %s (%s), want %s", tc.cmd, got.decision, got.reason, tc.want)
			}

			if got.decision != allow && got.reason == "" {
				t.Fatalf("a %s needs a reason", got.decision)
			}
		})
	}
}

func TestRelPath(t *testing.T) {
	cases := []struct {
		root, path string
		wantRel    string
		wantUnder  bool
	}{
		{"/repo", "/repo/a/b.go", "a/b.go", true},
		{"/repo", "a/b.go", "a/b.go", true},
		{"/repo", "/elsewhere/x.mpq", "/elsewhere/x.mpq", false},
		{"/repo", "/repo", ".", true},
	}

	for _, tc := range cases {
		rel, under := relPath(tc.root, tc.path)
		if rel != tc.wantRel || under != tc.wantUnder {
			t.Errorf("relPath(%q, %q) = (%q, %v), want (%q, %v)", tc.root, tc.path, rel, under, tc.wantRel, tc.wantUnder)
		}
	}
}

func TestIsAbs(t *testing.T) {
	for p, want := range map[string]bool{
		`C:\Users\josht`: true,
		`c:/Users/josht`: true,
		`/repo/x`:        true,
		`d2common/x.go`:  false,
		`C:`:             false,
		`relative.mpq`:   false,
	} {
		if got := isAbs(p); got != want {
			t.Errorf("isAbs(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestRunPreWriteEmitsDenyJSON(t *testing.T) {
	in := `{"hook_event_name":"PreToolUse","tool_name":"Write","tool_input":{"file_path":"C:\\repo\\patch.mpq","content":"x"},"cwd":"C:\\repo"}`

	var out, errOut bytes.Buffer

	code := runPreWrite(`C:\repo`, strings.NewReader(in), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}

	var got preToolUseOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}

	if got.HookSpecificOutput.HookEventName != "PreToolUse" || got.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("unexpected output %+v", got)
	}

	if !strings.Contains(got.HookSpecificOutput.PermissionDecisionReason, "Article V") {
		t.Fatalf("reason should cite Article V: %q", got.HookSpecificOutput.PermissionDecisionReason)
	}
}

func TestRunPreWriteAllowIsSilent(t *testing.T) {
	in := `{"tool_name":"Edit","tool_input":{"file_path":"/repo/main.go","old_string":"a","new_string":"b"}}`

	var out, errOut bytes.Buffer

	if code := runPreWrite(testRoot, strings.NewReader(in), &out, &errOut); code != 0 || out.Len() != 0 {
		t.Fatalf("allow should be exit 0 with no output; got %d %q", code, out.String())
	}
}

func TestRunPreBash(t *testing.T) {
	in := `{"tool_name":"Bash","tool_input":{"command":"go get -u ./...","description":"upgrade"}}`

	var out, errOut bytes.Buffer

	if code := runPreBash(strings.NewReader(in), &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}

	if !strings.Contains(out.String(), `"permissionDecision":"deny"`) {
		t.Fatalf("expected a deny, got %q", out.String())
	}

	out.Reset()

	if code := runPreBash(strings.NewReader(""), &out, &errOut); code != 0 || out.Len() != 0 {
		t.Fatalf("empty stdin should allow silently; got %d %q", code, out.String())
	}

	if code := runPreBash(strings.NewReader("{not json"), &out, &errOut); code != 1 {
		t.Fatalf("bad JSON should be a non-blocking error (exit 1), got %d", code)
	}
}

func TestDescribeTracking(t *testing.T) {
	for in, want := range map[string]string{
		"## master...origin/master":                     "in sync with origin/master",
		"## master...origin/master [ahead 2]":           "ahead 2 of master...origin/master",
		"## master...origin/master [ahead 1, behind 3]": "ahead 1, behind 3 of master...origin/master",
		"## feature": "no upstream",
		"garbage":    "tracking unknown",
	} {
		if got := describeTracking(in); got != want {
			t.Errorf("describeTracking(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDescribeTree(t *testing.T) {
	if got := describeTree("## master...origin/master"); got != "clean" {
		t.Errorf("clean tree described as %q", got)
	}

	got := describeTree("## master...origin/master\n M CLAUDE.md\n?? rh.ini\n")
	if !strings.HasPrefix(got, "1 modified/staged, 1 untracked") || !strings.Contains(got, "rh.ini") {
		t.Errorf("dirty tree described as %q", got)
	}
}

func TestReadAllowlist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "allow.txt")

	content := "# comment\n\nd2common/d2loader/testdata/D.mpq\n  docs\\x.tbl  \n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := readAllowlist(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 || !got["d2common/d2loader/testdata/D.mpq"] || !got["docs/x.tbl"] {
		t.Fatalf("unexpected allowlist %v", got)
	}

	if _, err := readAllowlist(filepath.Join(dir, "missing.txt")); err == nil {
		t.Fatal("a missing allowlist must be an error, not an empty list")
	}
}

// TestCheckFixtures builds a throwaway git repository: one allow-listed
// archive, one stray .dc6, and an ignored .mpq that must not count because
// it is untracked.
func TestCheckFixtures(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()

		res := runCmd(dir, gitTimeout, "git", args...)
		if !res.ok {
			t.Fatalf("git %v: %s", args, res.output)
		}
	}

	git("init", "-q")
	git("config", "user.email", "test@example.invalid")
	git("config", "user.name", "test")

	write := func(rel, content string) {
		t.Helper()

		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("docs/fixtures-allowlist.txt", "# allowed\nd2common/d2loader/testdata/D.mpq\nold/gone.tbl\n")
	write("d2common/d2loader/testdata/D.mpq", "not really an archive")
	write("sprites/stray.DC6", "stray")
	write("untracked.mpq", "ignored")
	write(".gitignore", "untracked.mpq\n")
	git("add", "docs", "d2common", "sprites", ".gitignore")
	git("commit", "-q", "-m", "fixtures")

	report, violations, err := checkFixtures(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(violations) != 1 || violations[0] != "sprites/stray.DC6" {
		t.Fatalf("violations = %v, want [sprites/stray.DC6]; report: %s", violations, report)
	}

	if !strings.Contains(report, "old/gone.tbl") {
		t.Fatalf("stale entry should be reported: %s", report)
	}

	var out, errOut bytes.Buffer
	if code := runCheckFixtures(dir, &out, &errOut); code != 1 {
		t.Fatalf("check-fixtures should exit 1 on a violation, got %d (%s)", code, errOut.String())
	}

	// Remove the stray file and the check goes green.
	git("rm", "-q", "sprites/stray.DC6")
	git("commit", "-q", "-m", "clean")

	out.Reset()
	errOut.Reset()

	if code := runCheckFixtures(dir, &out, &errOut); code != 0 {
		t.Fatalf("clean repo should pass, got %d: %s", code, errOut.String())
	}

	if !strings.Contains(out.String(), "1 tracked file(s)") {
		t.Fatalf("unexpected report: %s", out.String())
	}
}
