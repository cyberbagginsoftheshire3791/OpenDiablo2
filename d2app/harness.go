//go:build harness

package d2app

// Phase 3 playtest harness — the spine (M3.2, P3 spec §3, §4).
//
// This file: state, lifecycle, the update/draw command queues, the log ring,
// handles, and the run directory. The MCP server and tools live in
// harness_tools.go. Everything here compiles only with `-tags harness`; the
// call sites in app.go are no-ops otherwise (harness_off.go).
//
// One goroutine, one truth (§3.2): every read or write of game state is
// marshalled onto the game goroutine via the queues, drained at the top of
// advance() and at the end of render().

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2logfile"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2input"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2gamescreen"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client"
)

const (
	harnessVersion     = "0.12.0"         // c-2b: strigoi_click{hold_frames} (BUG-7's instrument half), spawns.max_groups settable
	harnessDefaultAddr = "127.0.0.1:6670" // the game server owns 6669
	harnessToolTimeout = 5 * time.Second  // [DIAL] P3 §3.2
	harnessQueueDepth  = 64
	harnessRingCap     = 5000
	harnessQuitDelay   = 300 * time.Millisecond
)

type harnessCmd struct {
	fn   func()
	done chan struct{}
}

type harnessLogLine struct {
	Seq  int    `json:"seq"`
	Text string `json:"text"`
}

// harnessRing is the log ring buffer tee'd onto the stdlib log writer, which
// every d2util.Logger writes through (logger.go: l.Writer = log.Writer()).
type harnessRing struct {
	mu      sync.Mutex
	partial string
	seq     int
	lines   []harnessLogLine
	dropped int
}

var harnessANSI = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func (r *harnessRing) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	text := r.partial + string(p)
	parts := strings.Split(text, "\n")
	r.partial = parts[len(parts)-1]

	for _, line := range parts[:len(parts)-1] {
		line = harnessANSI.ReplaceAllString(strings.TrimRight(line, "\r"), "")
		if line == "" {
			continue
		}

		r.seq++
		r.lines = append(r.lines, harnessLogLine{Seq: r.seq, Text: line})

		if len(r.lines) > harnessRingCap {
			over := len(r.lines) - harnessRingCap
			r.lines = r.lines[over:]
			r.dropped += over
		}
	}

	return len(p), nil
}

func (r *harnessRing) since(cursor int, pattern *regexp.Regexp, limit int) (out []harnessLogLine, next, dropped int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	next = cursor

	for _, l := range r.lines {
		if l.Seq <= cursor {
			continue
		}

		next = l.Seq

		if pattern != nil && !pattern.MatchString(l.Text) {
			continue
		}

		out = append(out, l)

		if len(out) >= limit {
			break
		}
	}

	return out, next, r.dropped
}

type harnessState struct {
	enabled *bool
	addr    *string
	outFlag *string

	app    *App
	runDir string
	ring   *harnessRing
	input  *d2input.ScriptedInputService // the E6 overlay (harness_input.go)

	updateQ    chan harnessCmd
	drawQ      chan harnessCmd
	drawTarget d2interface.Surface // valid only while draining the draw queue

	tick    int64 // atomic: frames advanced since boot
	started time.Time

	mu         sync.Mutex
	screenHint string
	game       *d2gamescreen.Game
	client     *d2client.GameClient
	handles    map[string]string // entity uuid -> handle
	rhandles   map[string]string // handle -> entity uuid
	handleSeq  int
	toolCalls  []string

	// M3.3 time + seed state (harness_time.go). Guarded by mu; the sim
	// fields are written only on the game goroutine.
	timeMode    string  // "live" | "paused"
	timeDT      float64 // seconds per stepped tick, default 1/60 [DIAL]
	simNow      float64 // the frozen/stepped clock handed to advanceOnce
	simSeconds  float64 // simulated seconds accumulated by stepping (digest input)
	stepping    bool    // a step call is currently executing batches
	pendingSeed int64   // set_seed value awaiting the next start_game (0 = none)
	currentSeed int64   // the seed the current game was started with (0 = unseeded)
}

// nolint:gochecknoglobals // one App per process; the harness mirrors that
var harness harnessState

func (a *App) harnessEarlyInit() {
	harness.app = a
	harness.ring = &harnessRing{}
	harness.updateQ = make(chan harnessCmd, harnessQueueDepth)
	harness.drawQ = make(chan harnessCmd, harnessQueueDepth)
	harness.handles = make(map[string]string)
	harness.rhandles = make(map[string]string)
	harness.screenHint = "boot"
	harness.started = time.Now()
	harness.timeMode = "live"
	harness.timeDT = 1.0 / 60 // [DIAL] P3 §3.4

	// Tee everything the loggers write into the ring. d2util.NewLogger copies
	// log.Writer() at construction, so this must run before other subsystems
	// build their loggers (Create calls it first).
	log.SetOutput(io.MultiWriter(log.Writer(), harness.ring))
}

// harnessInputService installs the scripted overlay at the d2input seam
// (P3 spec §3.7, E6). With nothing scripted it is a pass-through, so the
// harness build plays normally even without -harness.
func (a *App) harnessInputService(real d2interface.InputService) d2interface.InputService {
	harness.input = d2input.NewScriptedInputService(real)

	return harness.input
}

func (a *App) harnessRegisterFlags() {
	harness.enabled = flag.Bool("harness", false, "start the playtest-harness MCP server (loopback only)")
	harness.addr = flag.String("harness-addr", harnessDefaultAddr, "harness listen address (must be loopback)")
	harness.outFlag = flag.String("harness-out", "", "harness output directory (default: %LOCALAPPDATA%\\Strigoi\\harness\\runs)")
}

func (a *App) harnessStart() {
	if harness.enabled == nil || !*harness.enabled {
		return
	}

	host := *harness.addr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}

	if host != "127.0.0.1" && host != "localhost" && host != "::1" && host != "[::1]" {
		a.Errorf("harness: refusing non-loopback address %q; use 127.0.0.1", *harness.addr)
		return
	}

	base := *harness.outFlag
	if base == "" {
		// Reuse the one place the root is derived (logfile.go), so the harness
		// run dirs and the crash log agree on %LOCALAPPDATA%\Strigoi.
		base = filepath.Join(d2logfile.DataDir(), "harness", "runs")
	}

	harness.runDir = filepath.Join(base, time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(harness.runDir, 0o750); err != nil {
		a.Errorf("harness: cannot create run dir %q: %v", harness.runDir, err)
		return
	}

	go a.harnessServe() // harness_tools.go

	a.Infof("harness: MCP server on http://%s/mcp · run dir %s", *harness.addr, harness.runDir)
}

func (a *App) harnessDrainUpdate() {
	atomic.AddInt64(&harness.tick, 1)

	for {
		select {
		case c := <-harness.updateQ:
			c.fn()
			close(c.done)
		default:
			return
		}
	}
}

func (a *App) harnessDrainDraw(target d2interface.Surface) {
	for {
		select {
		case c := <-harness.drawQ:
			harness.drawTarget = target
			c.fn()
			harness.drawTarget = nil

			close(c.done)
		default:
			return
		}
	}
}

func (a *App) harnessNoteScreen(name string) {
	harness.mu.Lock()
	defer harness.mu.Unlock()

	harness.screenHint = name

	if name != "game" {
		harness.game = nil
		harness.client = nil
	}
}

func (a *App) harnessNoteGame(c *d2client.GameClient, g *d2gamescreen.Game) {
	harness.mu.Lock()
	defer harness.mu.Unlock()

	if c == nil {
		harness.game = nil
		harness.client = nil

		return
	}

	harness.client = c
	harness.game = g
	harness.screenHint = "game"
}

var errGameNotTicking = fmt.Errorf("GAME_NOT_TICKING: the game loop did not service the request within %v — is the process alive and unblocked?", harnessToolTimeout)

func harnessRunOn(q chan harnessCmd, fn func()) error {
	c := harnessCmd{fn: fn, done: make(chan struct{})}
	before := atomic.LoadInt64(&harness.tick)

	select {
	case q <- c:
	case <-time.After(harnessToolTimeout):
		return harnessNotTicking(before)
	}

	select {
	case <-c.done:
		return nil
	case <-time.After(harnessToolTimeout):
		return harnessNotTicking(before)
	}
}

// harnessNotTickingStackCap bounds the goroutine dump.
const harnessNotTickingStackCap = 1 << 20

// harnessNotTicking is the error for a request the loop did not service in
// time -- and the instrument for finding out why. The cause is otherwise
// invisible from outside (docs/bugs.md: TownWalk's screenshot stall, seen
// twice, the log ending mid-load each time), so it writes every goroutine's
// stack to a file in the run directory and names the file in the log and the
// error. It also says how many update ticks ran during the wait: none is a
// loop that stopped; some, on a draw request, is a loop updating without
// drawing.
//
// The stacks go to a FILE, not the log: a dump is thousands of lines, and in
// the log's 5000-line ring it would evict exactly the lines that show what the
// game was doing when it stalled.
func harnessNotTicking(before int64) error {
	now := atomic.LoadInt64(&harness.tick)
	ticked := now - before
	file := harnessDumpStacks(now)

	log.Printf("harness: GAME_NOT_TICKING -- %d update ticks ran during the wait; every goroutine's stack is in %s", ticked, file)

	return fmt.Errorf("%w (%d update ticks ran during the wait; stacks in %s)", errGameNotTicking, ticked, file)
}

// harnessStall remembers the last stack dump, so a loop stuck on one tick
// answers every tool call waiting on it with ONE dump, not one each.
//
// nolint:gochecknoglobals // one loop per process, one stall at a time
var harnessStall struct {
	sync.Mutex
	tick int64
	file string
}

// harnessDumpStacks writes every goroutine's stack to a file in the run
// directory (the system temp directory when there is none) and returns its
// name -- or the earlier dump's, if the loop has not ticked since.
func harnessDumpStacks(tick int64) string {
	harnessStall.Lock()
	defer harnessStall.Unlock()

	if harnessStall.file != "" && harnessStall.tick == tick {
		return harnessStall.file
	}

	buf := make([]byte, harnessNotTickingStackCap)
	n := runtime.Stack(buf, true)
	dump := buf[:n]

	if n == len(buf) {
		dump = append(dump, fmt.Sprintf("\n... TRUNCATED at %d bytes\n", len(buf))...)
	}

	dir := harness.runDir
	if dir == "" {
		dir = os.TempDir()
	}

	file := filepath.Join(dir, fmt.Sprintf("stall-tick%d-%s.txt", tick, time.Now().Format("150405.000")))

	if err := os.WriteFile(file, dump, 0o640); err != nil {
		// No file, so the log is the only place left to put them.
		log.Printf("harness: cannot write the stall's stacks to %s (%v); they follow\n%s", file, err, dump)

		file = "the game log (the stack file could not be written)"
	}

	harnessStall.tick, harnessStall.file = tick, file

	return file
}

func harnessOnUpdate(fn func()) error { return harnessRunOn(harness.updateQ, fn) }

func harnessOnDraw(fn func()) error { return harnessRunOn(harness.drawQ, fn) }

// harnessHandleFor assigns stable sequential handles: p:1 for the local
// player, e:N for everything else in first-seen order. Caller holds no lock.
func harnessHandleFor(id, localPlayerID string) string {
	harness.mu.Lock()
	defer harness.mu.Unlock()

	if h, ok := harness.handles[id]; ok {
		return h
	}

	var h string

	if id != "" && id == localPlayerID {
		h = "p:1"
	} else {
		harness.handleSeq++
		h = fmt.Sprintf("e:%d", harness.handleSeq)
	}

	harness.handles[id] = h
	harness.rhandles[h] = id

	return h
}

func harnessIDForHandle(handle string) (string, bool) {
	harness.mu.Lock()
	defer harness.mu.Unlock()

	id, ok := harness.rhandles[handle]

	return id, ok
}

func harnessLogCall(name string) {
	harness.mu.Lock()
	defer harness.mu.Unlock()

	harness.toolCalls = append(harness.toolCalls, fmt.Sprintf("%s @tick %d", name, atomic.LoadInt64(&harness.tick)))
}

func harnessGame() (*d2client.GameClient, *d2gamescreen.Game) {
	harness.mu.Lock()
	defer harness.mu.Unlock()

	return harness.client, harness.game
}

func harnessScreenHint() string {
	harness.mu.Lock()
	defer harness.mu.Unlock()

	return harness.screenHint
}
