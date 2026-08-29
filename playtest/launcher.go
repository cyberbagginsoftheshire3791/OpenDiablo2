//go:build playtest

// Package playtest holds the Phase 3 playtest scripts (P3 spec §3.8, §5).
// They are Go tests behind the `playtest` build tag, run on the laptop only:
//
//	go test -tags playtest ./playtest/... -v -count=1
//
// Each script builds the game with `-tags harness`, launches it headful
// against the real MPQs, attaches over the harness's Streamable HTTP endpoint
// with the MCP Go SDK client, and asserts on structured tool output. Never
// run in CI (no MPQs there — Constitution, Article V).
//
// Set STRIGOI_HARNESS_ADDR (e.g. 127.0.0.1:6670) to attach to a game you
// started by hand instead of building + launching one.
package playtest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultAddr    = "127.0.0.1:6670"
	connectTimeout = 90 * time.Second
	callTimeout    = 60 * time.Second
)

type session struct {
	t        *testing.T
	sess     *mcp.ClientSession
	cmd      *exec.Cmd
	attached bool
	stopped  bool
	RunBase  string
	LogPath  string // the launched game's stdout+stderr (panics land here)
	logFile  *os.File
}

// start builds and launches the harness build (or attaches to a running one)
// and returns a connected session. It registers cleanup on t.
func start(t *testing.T) *session {
	t.Helper()

	s := &session{t: t}

	addr := os.Getenv("STRIGOI_HARNESS_ADDR")
	if addr != "" {
		s.attached = true
	} else {
		addr = defaultAddr

		repoRoot, err := filepath.Abs("..")
		if err != nil {
			t.Fatalf("repo root: %v", err)
		}

		exe := filepath.Join(os.TempDir(), "strigoi-harness", "od2-harness")
		if runtime.GOOS == "windows" {
			exe += ".exe"
		}

		if err := os.MkdirAll(filepath.Dir(exe), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		build := exec.Command("go", "build", "-tags", "harness", "-o", exe, ".")
		build.Dir = repoRoot

		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("go build -tags harness failed: %v\n%s", err, out)
		}

		// Run artifacts land beside the repo, not in it (Article V), where the
		// device bridge can still reach them: <Projects>/strigoi-harness-runs.
		s.RunBase = filepath.Join(filepath.Dir(repoRoot), "strigoi-harness-runs")

		s.cmd = exec.Command(exe, "-harness", "-harness-addr", addr, "-harness-out", s.RunBase, "-l", "4")
		s.cmd.Dir = repoRoot

		// Keep the game's own output: a script that dies with a transport
		// error usually died because the game panicked, and the panic is here.
		if err := os.MkdirAll(s.RunBase, 0o750); err != nil {
			t.Fatalf("run base: %v", err)
		}

		s.LogPath = filepath.Join(s.RunBase, "game-"+time.Now().Format("20060102-150405")+".log")

		if f, err := os.Create(s.LogPath); err == nil {
			s.logFile = f
			s.cmd.Stdout = f
			s.cmd.Stderr = f
		}

		if err := s.cmd.Start(); err != nil {
			t.Fatalf("launching the game: %v", err)
		}

		t.Logf("game output -> %s", s.LogPath)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "strigoi-playtest", Version: "0.1.0"}, nil)
	endpoint := fmt.Sprintf("http://%s/mcp", addr)
	deadline := time.Now().Add(connectTimeout)

	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		sess, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint}, nil)

		cancel()

		if err == nil {
			s.sess = sess
			break
		}

		if time.Now().After(deadline) {
			s.kill()
			t.Fatalf("could not connect to %s within %v: %v", endpoint, connectTimeout, err)
		}

		time.Sleep(500 * time.Millisecond)
	}

	t.Cleanup(s.stop)

	return s
}

func (s *session) stop() {
	if s.stopped {
		return
	}

	s.stopped = true

	if s.sess != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _ = s.sess.CallTool(ctx, &mcp.CallToolParams{
			Name:      "strigoi_quit",
			Arguments: map[string]any{"confirm": true},
		})

		cancel()
		_ = s.sess.Close()
	}

	s.kill()
}

func (s *session) kill() {
	if s.cmd == nil || s.cmd.Process == nil {
		return
	}

	done := make(chan struct{})

	go func() {
		_, _ = s.cmd.Process.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = s.cmd.Process.Kill()
	}

	if s.logFile != nil {
		_ = s.logFile.Close()
		s.logFile = nil
	}
}

// gameTail returns the last n lines of the launched game's output, for
// failure messages. Empty when attached to a hand-started game.
func (s *session) gameTail(n int) string {
	if s.LogPath == "" {
		return ""
	}

	data, err := os.ReadFile(s.LogPath)
	if err != nil {
		return ""
	}

	lines := strings.Split(strings.TrimRight(string(data), "\r\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	return strings.Join(lines, "\n")
}

// call invokes a tool and fails the test on transport or tool errors.
func (s *session) call(name string, args map[string]any) map[string]any {
	s.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()

	res, err := s.sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		s.t.Fatalf("%s: transport error: %v\n--- game output (tail) ---\n%s", name, err, s.gameTail(40))
	}

	if res.IsError {
		s.t.Fatalf("%s: tool error: %s", name, contentText(res))
	}

	out := map[string]any{}

	if res.StructuredContent != nil {
		raw, err := json.Marshal(res.StructuredContent)
		if err != nil {
			s.t.Fatalf("%s: structured content: %v", name, err)
		}

		if err := json.Unmarshal(raw, &out); err != nil {
			s.t.Fatalf("%s: structured content decode: %v", name, err)
		}
	}

	s.t.Logf("%s -> %s", name, contentText(res))

	return out
}

// callErr invokes a tool and returns the tool-error text ("" on success).
func (s *session) callErr(name string, args map[string]any) string {
	s.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()

	res, err := s.sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		s.t.Fatalf("%s: transport error: %v\n--- game output (tail) ---\n%s", name, err, s.gameTail(40))
	}

	if res.IsError {
		return contentText(res)
	}

	return ""
}

func contentText(res *mcp.CallToolResult) string {
	text := ""

	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text += tc.Text
		}
	}

	return text
}

// flag reads a bool an assertion depends on, and FAILS when the key is absent
// or is not a bool.
//
// This is A3 of the Phase 4 audit. The pattern it replaces --
// `if v, _ := m[k].(bool); !v` -- reads a missing or RENAMED field as false,
// so a negative assertion passes while measuring nothing at all. The provider
// surface changed in four consecutive milestones, and a script that asserts
// "the watcher did NOT notice" against a field that no longer exists is the
// exact shape of a green suite over a broken system.
//
// Prefer this over the tolerant helpers below wherever the value carries an
// assertion. num, str and pair are still fail-open, and that is A3's named
// remainder rather than an oversight: see docs/harness.md.
// It takes a testing.TB rather than a *testing.T so that its own failure path
// can be exercised by a fake -- a helper whose whole job is to fail has to be
// shown failing, or it is the same act of faith it was written to replace.
func flag(t testing.TB, m map[string]any, key string) bool {
	t.Helper()

	// The returns after each Fatalf are deliberate. A real *testing.T's Fatalf
	// ends the goroutine, so they are unreachable in a run -- but without them
	// the absent-key branch falls through into the type assertion and reports
	// "<nil> is not a bool", losing the message that says WHICH names were
	// actually present. That is the message a rename is read from. Found by
	// this helper's own negative control, which is the argument for writing
	// one: an instrument whose failure path has never been executed has not
	// been shown to work.
	v, ok := m[key]
	if !ok {
		t.Fatalf("field %q is absent -- a negative assertion on a field that is not there proves nothing. present: %v",
			key, keysOf(m))

		return false
	}

	b, ok := v.(bool)
	if !ok {
		t.Fatalf("field %q is %T (%v), not a bool", key, v, v)

		return false
	}

	return b
}

// keysOf makes the failure above readable: the point of the message is to show
// that a field was RENAMED, which needs the names that are actually there.
func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}

func num(m map[string]any, key string) float64 {
	v, _ := m[key].(float64)
	return v
}

func str(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func sub(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	if v == nil {
		return map[string]any{}
	}

	return v
}

func pair(m map[string]any, key string) (x, y float64) {
	arr, _ := m[key].([]any)
	if len(arr) == 2 {
		x, _ = arr[0].(float64)
		y, _ = arr[1].(float64)
	}

	return x, y
}
