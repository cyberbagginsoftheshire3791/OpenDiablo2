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

		if err := s.cmd.Start(); err != nil {
			t.Fatalf("launching the game: %v", err)
		}
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
}

// call invokes a tool and fails the test on transport or tool errors.
func (s *session) call(name string, args map[string]any) map[string]any {
	s.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()

	res, err := s.sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		s.t.Fatalf("%s: transport error: %v", name, err)
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
		s.t.Fatalf("%s: transport error: %v", name, err)
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
