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
// started by hand instead of building + launching one. Launched games run in
// parallel (see startWith); go test -parallel N sets how many at once.
package playtest

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	connectTimeout = 90 * time.Second
	callTimeout    = 60 * time.Second
)

type session struct {
	t        *testing.T
	sess     *mcp.ClientSession
	cmd      *exec.Cmd
	exited   chan struct{} // closed when the launched game's process has exited
	attached bool
	stopped  bool
	RunBase  string
	LogPath  string // the launched game's stdout+stderr (panics land here)
	logFile  *os.File
}

// start launches Strigoi -- the game with no switches, the authored village,
// the Janissary, Strigoi's fonts and words -- and returns a connected session.
//
// THE SUITE TESTS STRIGOI (Josh, 25 Sep 2026). Until then start passed
// -classic and every script proved itself on Diablo II's generated Act 1,
// while the game that ships was covered only by a second, slower sweep.
// STRIGOI_PLAYTEST_GAME=classic now runs every script on Act 1 instead: the
// sweep is the other way round. A script whose subject only exists in Act 1
// (the generator, the font and word controls) says so and calls
// startWith(t, "-classic") itself.
func start(t *testing.T) *session {
	t.Helper()

	return startGame(t)
}

// startGame is start with extra flags for the game, still honouring
// STRIGOI_PLAYTEST_GAME. Any value but "" or "classic" fails the script: the
// retired "default" (the old sweep) would otherwise silently run Strigoi.
func startGame(t *testing.T, extra ...string) *session {
	t.Helper()

	switch game := os.Getenv("STRIGOI_PLAYTEST_GAME"); game {
	case "":
		return startWith(t, extra...)
	case "classic":
		return startWith(t, append([]string{"-classic"}, extra...)...)
	default:
		t.Fatalf("STRIGOI_PLAYTEST_GAME=%q: use \"classic\" or leave it unset (Strigoi is the default since 26 Sep 2026)", game)
		return nil
	}
}

// freePort returns a loopback port nothing is listening on right now, for a
// script that needs its game server on a REAL port (TestDeath: the reload's
// PortFree wait is vacuous under -server-port 0).
func freePort(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding a free port: %v", err)
	}

	defer l.Close()

	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
}

// startWith is start with extra command-line flags for the game (-fonts ...).
// An attached session was launched by someone else, so it cannot take them:
// the script is skipped rather than run against the wrong game.
//
// EVERY LAUNCHED SCRIPT RUNS IN PARALLEL (26 Sep 2026). Each game is its own
// process with its own harness port, game-server port, %APPDATA% (so its
// hero saves and config) and run directory, and the world is stepped rather
// than wall-clocked, so four games sharing the laptop report the same numbers
// one would. t.Parallel is taken here, once per test, so a new script is
// parallel without remembering to be. go test's -parallel sets the width;
// STRIGOI_PLAYTEST_SERIAL=1, an attached game, or keepSerial(t) keeps a
// script to itself.
func startWith(t *testing.T, flags ...string) *session {
	t.Helper()

	s := &session{t: t}

	addr := os.Getenv("STRIGOI_HARNESS_ADDR")
	if addr != "" {
		if len(flags) > 0 {
			t.Skipf("attached to a running game; this script launches its own with %v", flags)
		}

		s.attached = true
	} else {
		goParallel(t)

		repoRoot, err := filepath.Abs("..")
		if err != nil {
			t.Fatalf("repo root: %v", err)
		}

		exe, err := harnessBinary(repoRoot)
		if err != nil {
			t.Fatalf("%v", err)
		}

		home, err := testHome(t)
		if err != nil {
			t.Fatalf("a private %%APPDATA%% for this test: %v", err)
		}

		// Run artifacts land beside the repo, not in it (Article V), where the
		// device bridge can still reach them: <Projects>/strigoi-harness-runs.
		// Each test's harness runs go under pt/<test>, so parallel games never
		// share a run directory.
		s.RunBase = filepath.Join(filepath.Dir(repoRoot), "strigoi-harness-runs")
		name := safeName(t.Name())
		out := filepath.Join(s.RunBase, "pt", name)

		if err := os.MkdirAll(out, 0o750); err != nil {
			t.Fatalf("run dir: %v", err)
		}

		seq := launchSeq.Add(1)
		addrFile := filepath.Join(home, fmt.Sprintf("harness-addr-%d.txt", seq))

		args := []string{
			"-harness", "-harness-addr", "127.0.0.1:0", "-harness-addr-file", addrFile,
			"-harness-out", out, "-server-port", "0", "-harness-timeout", "30s", "-l", "4",
		}
		s.cmd = exec.Command(exe, append(args, flags...)...)
		s.cmd.Dir = repoRoot
		// os.UserConfigDir is %APPDATA% on Windows: the hero saves
		// (OpenDiablo2\Saves) and config.json follow it, so this test's heroes
		// are its own. The last entry wins when a key repeats.
		s.cmd.Env = append(os.Environ(), "APPDATA="+home, "XDG_CONFIG_HOME="+home)

		// Keep the game's own output: a script that dies with a transport
		// error usually died because the game panicked, and the panic is here.
		s.LogPath = filepath.Join(s.RunBase, fmt.Sprintf("game-%s-%s.log", time.Now().Format("20060102-150405.000"), name))

		if f, err := os.Create(s.LogPath); err == nil {
			s.logFile = f
			s.cmd.Stdout = f
			s.cmd.Stderr = f
		}

		if err := s.cmd.Start(); err != nil {
			if s.logFile != nil {
				_ = s.logFile.Close()
			}

			t.Fatalf("launching the game: %v", err)
		}

		s.exited = make(chan struct{})

		go func(cmd *exec.Cmd, done chan struct{}) {
			_ = cmd.Wait()
			close(done)
		}(s.cmd, s.exited)

		t.Logf("game output -> %s", s.LogPath)

		addr, err = s.waitForAddr(addrFile)
		if err != nil {
			s.kill()
			t.Fatalf("%v\n--- game output (tail) ---\n%s", err, s.gameTail(40))
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
			t.Fatalf("could not connect to %s within %v: %v\n--- game output (tail) ---\n%s", endpoint, connectTimeout, err, s.gameTail(40))
		}

		if s.exited != nil {
			select {
			case <-s.exited:
				t.Fatalf("the game exited before the harness session connected\n--- game output (tail) ---\n%s", s.gameTail(40))
			default:
			}
		}

		time.Sleep(500 * time.Millisecond)
	}

	t.Cleanup(s.stop)

	return s
}

// waitForAddr waits for the game to say which port its harness bound
// (-harness-addr-file). A game that exits first is reported at once rather
// than after the connect timeout.
func (s *session) waitForAddr(path string) (string, error) {
	deadline := time.Now().Add(connectTimeout)

	for {
		if data, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(data))) > 0 {
			return strings.TrimSpace(string(data)), nil
		}

		select {
		case <-s.exited:
			return "", fmt.Errorf("the game exited before its harness was listening")
		case <-time.After(100 * time.Millisecond):
		}

		if time.Now().After(deadline) {
			return "", fmt.Errorf("the game's harness did not report an address within %v (%s)", connectTimeout, path)
		}
	}
}

var (
	buildOnce sync.Once
	buildDir  string
	builtExe  string
	buildErr  error

	launchSeq atomic.Int64

	homeMu sync.Mutex
	homes  = map[*testing.T]string{}

	serialMu   sync.Mutex
	serialOnly = map[*testing.T]bool{}
	parallelOn = map[*testing.T]bool{}
)

// harnessBinary builds the harness game ONCE per go test run and mirrors
// Strigoi's own data beside it. The build used to run inside every launch:
// sixty-odd builds a suite, and with games running in parallel a rebuild
// would overwrite an exe another test is running from. The asset copy is
// therefore also a snapshot: art that changes mid-suite does not reach it.
//
// A failed build fails every test with the build's own output, not with a
// transport error from a game that never started.
func harnessBinary(repoRoot string) (string, error) {
	buildOnce.Do(func() {
		parent := filepath.Join(os.TempDir(), "strigoi-harness")
		if err := os.MkdirAll(parent, 0o750); err != nil {
			buildErr = fmt.Errorf("mkdir: %w", err)
			return
		}

		dir, err := os.MkdirTemp(parent, "run-")
		if err != nil {
			buildErr = fmt.Errorf("build dir: %w", err)
			return
		}

		exe := filepath.Join(dir, "od2-harness")
		if runtime.GOOS == "windows" {
			exe += ".exe"
		}

		build := exec.Command("go", "build", "-tags", "harness", "-o", exe, ".")
		build.Dir = repoRoot

		if out, err := build.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("go build -tags harness failed: %v\n%s", err, out)
			return
		}

		// The application loads loose project assets beside the executable. The
		// harness binary lives in a temp directory, so mirror only Strigoi's own
		// data there; without this, tests silently fall back to D2 art.
		if err := copyTree(
			filepath.Join(repoRoot, "data", "strigoi"),
			filepath.Join(dir, "data", "strigoi"),
		); err != nil {
			buildErr = fmt.Errorf("copying Strigoi assets beside harness: %w", err)
			return
		}

		buildDir, builtExe = dir, exe
	})

	return builtExe, buildErr
}

// testHome is this test's private %APPDATA%, shared by every game the test
// launches (a script that relaunches still finds its own saves). The player's
// config.json is copied in so the MPQ path and window settings are his.
func testHome(t *testing.T) (string, error) {
	homeMu.Lock()
	defer homeMu.Unlock()

	if home, ok := homes[t]; ok {
		return home, nil
	}

	parent := filepath.Join(os.TempDir(), "strigoi-harness")
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return "", err
	}

	home, err := os.MkdirTemp(parent, "home-"+safeName(t.Name())+"-")
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Join(home, "OpenDiablo2"), 0o750); err != nil {
		return "", err
	}

	if real, err := os.UserConfigDir(); err == nil {
		if data, err := os.ReadFile(filepath.Join(real, "OpenDiablo2", "config.json")); err == nil {
			if err := os.WriteFile(filepath.Join(home, "OpenDiablo2", "config.json"), data, 0o600); err != nil {
				return "", err
			}
		}
	}

	homes[t] = home

	// Registered before the session's own cleanup, so it runs after the game
	// is stopped. Best effort: Windows can hold a file a moment after exit.
	t.Cleanup(func() {
		homeMu.Lock()
		delete(homes, t)
		homeMu.Unlock()

		_ = os.RemoveAll(home)
	})

	return home, nil
}

// keepSerial keeps a script out of the parallel phase -- for a script whose
// subject is the whole desktop (minimized_test minimizes every window).
// Call it before start.
func keepSerial(t *testing.T) {
	serialMu.Lock()
	defer serialMu.Unlock()

	serialOnly[t] = true
}

func goParallel(t *testing.T) {
	serialMu.Lock()
	skip := serialOnly[t] || parallelOn[t] || os.Getenv("STRIGOI_PLAYTEST_SERIAL") == "1"
	parallelOn[t] = true
	// Unlock BEFORE t.Parallel: it pauses this test until every sequential test
	// has finished, and a lock held across that pause is one the next test to
	// start can never take (it did, 26 Sep, on the first parallel run).
	serialMu.Unlock()

	if !skip {
		t.Parallel()
	}
}

// safeName makes a test name usable in a file name.
func safeName(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, name)
}

func copyTree(source, target string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)

		if info.IsDir() {
			return os.MkdirAll(destination, 0o750)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Files copied out of the module cache are read-only; the copy must not be.
		return os.WriteFile(destination, data, info.Mode()|0o200)
	})
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

// kill waits up to five seconds for the game to leave on its own, then kills
// it, and closes its log. The process is waited on exactly once, by the
// goroutine startWith started; a second Wait would return at once and make a
// live game look exited.
func (s *session) kill() {
	if s.cmd == nil || s.cmd.Process == nil || s.exited == nil {
		return
	}

	select {
	case <-s.exited:
	case <-time.After(5 * time.Second):
		_ = s.cmd.Process.Kill()

		select {
		case <-s.exited:
		case <-time.After(5 * time.Second):
		}
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

	if name == startGameTool {
		s.dropToPolicy()
	}

	return out
}

// startGameTool is the one tool that brings a game screen -- and with it a
// Combat -- into existence.
const startGameTool = "strigoi_start_game"

// dropToPolicy puts a freshly started game back on the POLICY round.
//
// M4.4c-2a ask 5 made the shipped screen take the player's turn itself
// (shippedCombatDials), which is what makes the seam real for a person. Every
// script that steps world minutes therefore meets a round that waits, and a
// wait it never commits is a script that dies on AWAITING_PLAYER -- sixteen of
// the seventeen scripts were written against a world that resolves its own
// fights, and the spawn chance can open one in any of them (spawns_test and
// meters_test have met a fight on the 43-hour walk by their own comments).
//
// So the opt-out lives HERE, once, rather than in fourteen start_game call
// sites that would drift apart: a script runs on policy unless it asks for
// human, which is exactly what hands_test's acts do, after start_game. The GAME
// keeps one code path -- human, always -- and the divergence is test-side and
// visible in one place.
func (s *session) dropToPolicy() {
	s.t.Helper()

	// Not through s.call: that would recurse on the name check, and a session
	// attached to a game someone else started (STRIGOI_HARNESS_ADDR) has no
	// business being reconfigured either.
	if s.attached {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()

	res, err := s.sess.CallTool(ctx, &mcp.CallToolParams{
		Name: "strigoi_set_system_field",
		Arguments: map[string]any{
			"system": "combat", "field": "player_control", "value": "policy",
		},
	})
	if err != nil {
		s.t.Fatalf("player_control=policy after start_game: transport error: %v\n--- game output (tail) ---\n%s",
			err, s.gameTail(40))
	}

	if res.IsError {
		s.t.Fatalf("player_control=policy after start_game: %s", contentText(res))
	}
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
// assertion. num and str now have strict twins -- mustNum and mustStr, same
// contract, same failure message. pair is still fail-open, and that is A3's
// remaining named gap rather than an oversight: see docs/harness.md.
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

// num and str are the TOLERANT reads, and they stay tolerant on purpose: most
// of their 400-odd uses are logging, arithmetic on a value that is legitimately
// optional, or a setup step whose own assertion comes later. Converting those
// would be churn.
//
// USE mustNum / mustStr WHEREVER THE VALUE CARRIES THE ASSERTION. num reads an
// absent or RENAMED key as 0 and str reads it as "", so `num(row, "health")`
// against a field the provider has renamed says "dead" and the assertion
// passes while measuring nothing -- flag()'s defect in a second shape.
func num(m map[string]any, key string) float64 {
	v, _ := m[key].(float64)
	return v
}

func str(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

// mustNum is num with flag()'s contract: absent or wrong-typed is a FAILURE
// naming the key and the keys that ARE present, because the failure it usually
// reports is a rename and a rename is only legible beside its replacement.
//
// testing.TB rather than *testing.T for flag()'s reason: a helper whose whole
// job is to fail has to be shown failing, and flag_test.go's fakeTB is what
// shows it.
func mustNum(t testing.TB, m map[string]any, key string) float64 {
	t.Helper()

	// The returns after each Fatalf are flag()'s, for flag()'s reason: without
	// them the absent-key branch falls through into the type assertion and
	// reports "<nil> is not a number", losing the list of present keys.
	v, ok := m[key]
	if !ok {
		t.Fatalf("field %q is absent -- an assertion on a number that is not there proves nothing "+
			"(a missing key reads as 0, which is \"dead\", \"empty\" and \"at the origin\"). present: %v",
			key, keysOf(m))

		return 0
	}

	f, ok := v.(float64)
	if !ok {
		t.Fatalf("field %q is %T (%v), not a number", key, v, v)

		return 0
	}

	return f
}

// mustStr is str's strict twin. An absent key reads as "", which compares equal
// to no mode, no reason and no id -- and every one of those is an assertion
// somewhere in these scripts.
func mustStr(t testing.TB, m map[string]any, key string) string {
	t.Helper()

	v, ok := m[key]
	if !ok {
		t.Fatalf("field %q is absent -- an assertion on a string that is not there proves nothing "+
			"(a missing key reads as \"\"). present: %v", key, keysOf(m))

		return ""
	}

	s, ok := v.(string)
	if !ok {
		t.Fatalf("field %q is %T (%v), not a string", key, v, v)

		return ""
	}

	return s
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

// readSaved reads a file the game rewrites while the script runs (the kit
// sidecar, a hero save). The game replaces it atomically -- a temp file, then
// a rename (d2items.WriteFileAtomic) -- and on Windows a read that lands in
// that instant fails with a sharing violation ("being used by another
// process"). Under the parallel suite's load that instant was hit twice on 27
// Sep (TestKit, both times on a loaded machine; alone it passed). So a read
// that fails for any reason but absence is retried for up to two seconds.
// Absence is returned at once: several scripts assert that a file is NOT there.
func readSaved(path string) ([]byte, error) {
	var (
		data []byte
		err  error
	)

	for i := 0; i < 40; i++ {
		if data, err = os.ReadFile(path); err == nil || os.IsNotExist(err) {
			return data, err
		}

		time.Sleep(50 * time.Millisecond)
	}

	return data, err
}
