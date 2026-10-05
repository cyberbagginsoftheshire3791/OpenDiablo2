//go:build playtest

package playtest

import (
	"context"
	"encoding/json"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// LEVEL 1: F8 FEEDBACK AND CRASH REPORTS (ruled by Josh, 5 Oct 2026;
// docs/feedback.md). F8 anywhere freezes the frame, holds the world as the
// escape menu does and opens one line; Enter saves feedback\<stamp>\{note.txt,
// shot.png, state.json}, Escape writes nothing. A panic on the game goroutine
// writes crashes\<stamp>\, and the next launch says so on the main menu; a run
// that ends any other way -- a fatal error nothing recovers, a kill -- is
// reported by the next launch from the marker it left.
//
// The launcher points STRIGOI_REPORT_HOME at the test's home, so every folder
// here is the test's own; the provider's reports_root says which.
//
// NEGATIVE CONTROLS (run by overlay, STRIGOI_HARNESS_OVERLAY; the build
// agent's REPORT.md lists each with its result): the box not holding the game
// (SetFeedbackHold a no-op), the frame read after the box is drawn, Escape
// saving, the crash guard not deferred in advance, and the marker not removed
// on a clean quit.

// feedbackNote is what the script types. It has punctuation and digits in it:
// the box takes a sentence, not a hero's name.
const feedbackNote = "the torch went out at 21:15, why? (day 1)"

func feedbackState(s *session) map[string]any {
	return sub(s.call("strigoi_get_system_state", map[string]any{"system": "feedback"}), "state")
}

// reportDirs is the folders under home\<kind>, sorted.
func reportDirs(t *testing.T, home, kind string) []string {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(home, kind))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("reading %s: %v", kind, err)
	}

	var dirs []string

	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(home, kind, e.Name()))
		}
	}

	sort.Strings(dirs)

	return dirs
}

func readText(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", filepath.Base(path), err)
	}

	return string(data)
}

func readStateJSON(t *testing.T, dir string) map[string]any {
	t.Helper()

	var m map[string]any
	if err := json.Unmarshal([]byte(readText(t, filepath.Join(dir, "state.json"))), &m); err != nil {
		t.Fatalf("state.json in %s: %v", dir, err)
	}

	return m
}

// waitFeedback polls the feedback provider until ok, for a live (unpaused)
// game: the menus tick on their own.
func waitFeedback(t *testing.T, s *session, what string, ok func(map[string]any) bool) map[string]any {
	t.Helper()

	deadline := time.Now().Add(60 * time.Second)

	for {
		fb := feedbackState(s)
		if ok(fb) {
			return fb
		}

		if time.Now().After(deadline) {
			t.Fatalf("waiting for %s: feedback %v", what, fb)
		}

		time.Sleep(200 * time.Millisecond)
	}
}

func waitMainMenu(t *testing.T, s *session) map[string]any {
	t.Helper()

	waitFeedback(t, s, "the main menu", func(fb map[string]any) bool { return str(fb, "screen") == "main_menu" })

	// A few frames on the menu, so the start notice has been drawn.
	time.Sleep(time.Second)

	return feedbackState(s)
}

// callDying sends a verb that ends the process: the answer may never come.
func (s *session) callDying(name string, args map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	_, _ = s.sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
}

// waitExit waits for the launched game to end and returns its exit code.
func (s *session) waitExit(t *testing.T) int {
	t.Helper()

	select {
	case <-s.exited:
	case <-time.After(30 * time.Second):
		t.Fatalf("the game did not exit\n--- game output (tail) ---\n%s", s.gameTail(40))
	}

	return s.cmd.ProcessState.ExitCode()
}

// rectMean is a rectangle's mean colour, 0-255 per channel.
func rectMean(img image.Image, r image.Rectangle) [3]float64 {
	var sum [3]float64

	n := 0

	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			sum[0] += float64(cr >> 8)
			sum[1] += float64(cg >> 8)
			sum[2] += float64(cb >> 8)
			n++
		}
	}

	for i := range sum {
		sum[i] /= float64(n)
	}

	return sum
}

func meanDelta(a, b [3]float64) float64 {
	return math.Max(math.Abs(a[0]-b[0]), math.Max(math.Abs(a[1]-b[1]), math.Abs(a[2]-b[2])))
}

func decodePNG(t *testing.T, path string) image.Image {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}

	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("%s is not a PNG: %v", path, err)
	}

	return img
}

// feedbackPanel is where the box draws its panel (d2app/feedback.go).
var feedbackPanel = image.Rect(110, 240, 690, 340)

// TestFeedbackF8 is F8 in a game: the frame frozen before the box, the world
// held while he types, the three files and the log line, and Escape writing
// nothing.
func TestFeedbackF8(t *testing.T) {
	s := start(t)

	home, err := testHome(t)
	if err != nil {
		t.Fatal(err)
	}

	s.call("strigoi_pause", map[string]any{})
	s.call("strigoi_start_game", map[string]any{
		"hero_name": "Note", "hero_class": "amazon", "seed": 1462, "wait_seconds": 90,
	})
	s.call("strigoi_step", map[string]any{"frames": 30})

	fb := feedbackState(s)
	if got := mustStr(t, fb, "reports_root"); got != home {
		t.Fatalf("reports_root %q, want the test's home %q (the launcher's STRIGOI_REPORT_HOME)", got, home)
	}

	if flag(t, fb, "open") {
		t.Fatal("the box is open before F8")
	}

	// POSITIVE CONTROL: stepped frames move the clock while nothing holds it,
	// so "unchanged under the box" cannot pass on a frozen clock.
	w0 := mustNum(t, clockState(s), "world_minutes")
	s.call("strigoi_step", map[string]any{"frames": 120})

	if w1 := mustNum(t, clockState(s), "world_minutes"); w1 <= w0 {
		t.Fatalf("positive control: 120 frames moved the clock %.4f -> %.4f", w0, w1)
	}

	before := s.frame(t, "feedback-before")

	s.call("strigoi_key", map[string]any{"key": "f8"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	fb = feedbackState(s)
	if !flag(t, fb, "open") || !flag(t, fb, "held_game") || !flag(t, fb, "shot_taken") {
		t.Fatalf("F8 in a game: want open, held_game and shot_taken, got %v", fb)
	}

	ui := uiState(s)
	if held := mustStr(t, ui, "world_held_by"); held != "feedback" {
		t.Fatalf("world_held_by %q under the box, want \"feedback\"", held)
	}

	wA := mustNum(t, clockState(s), "world_minutes")
	s.call("strigoi_step", map[string]any{"frames": 600})

	if wB := mustNum(t, clockState(s), "world_minutes"); math.Abs(wB-wA) > 1e-9 {
		t.Fatalf("the world ran under the box: world_minutes %.4f -> %.4f over 600 frames", wA, wB)
	}

	if msg := s.callErr("strigoi_step_world", map[string]any{"world_minutes": 10.0}); !strings.Contains(msg, "WORLD_HELD") ||
		!strings.Contains(msg, `"feedback"`) {
		t.Fatalf("step_world under the box: %q, want WORLD_HELD naming \"feedback\"", msg)
	}

	live := s.frame(t, "feedback-box-open")

	// The box takes the keys: Q would open his journal.
	s.call("strigoi_key", map[string]any{"key": "q"})
	s.call("strigoi_type_text", map[string]any{"text": feedbackNote})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if ui := uiState(s); flag(t, ui, "journal_open") {
		t.Fatal("Q reached the game under the box: the journal opened")
	}

	if got := mustStr(t, feedbackState(s), "text"); got != feedbackNote {
		t.Fatalf("the box holds %q, want %q", got, feedbackNote)
	}

	s.call("strigoi_key", map[string]any{"key": "enter"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	fb = feedbackState(s)
	if flag(t, fb, "open") || mustNum(t, fb, "saved") != 1 || mustStr(t, fb, "notice") != "Feedback saved" {
		t.Fatalf("after Enter: want closed, saved 1, the notice; got %v", fb)
	}

	dir := mustStr(t, fb, "last_dir")
	if dirs := reportDirs(t, home, "feedback"); len(dirs) != 1 || dirs[0] != dir {
		t.Fatalf("feedback folders %v, want exactly %s", dirs, dir)
	}

	if got := readText(t, filepath.Join(dir, "note.txt")); got != feedbackNote+"\n" {
		t.Fatalf("note.txt %q, want %q", got, feedbackNote)
	}

	shot := decodePNG(t, filepath.Join(dir, "shot.png"))
	if b := shot.Bounds(); b.Dx() != 800 || b.Dy() != 600 {
		t.Fatalf("shot.png is %dx%d, want the 800x600 frame", b.Dx(), b.Dy())
	}

	// THE FRAME IS THE ONE HE SAW, NOT THE BOX: where the panel draws, the
	// saved shot matches the frame before F8 and not the frame with the box up.
	mBefore, mShot, mLive := rectMean(before, feedbackPanel), rectMean(shot, feedbackPanel), rectMean(live, feedbackPanel)
	if d := meanDelta(mBefore, mShot); d > 3 {
		t.Fatalf("shot.png's panel area differs from the frame before F8 by %.1f (before %v, shot %v)", d, mBefore, mShot)
	}

	if d := meanDelta(mLive, mShot); d < 10 {
		t.Fatalf("instrument: the box's panel barely changes the frame (%.1f; live %v, shot %v) -- the comparison above proves nothing", d, mLive, mShot)
	}

	state := readStateJSON(t, dir)
	if str(state, "screen") != "game" || str(state, "note") != feedbackNote || state["shot"] != true {
		t.Fatalf("state.json: screen %v note %v shot %v", state["screen"], state["note"], state["shot"])
	}

	game := sub(state, "game")
	for _, k := range []string{"day", "clock", "world_minutes", "player_tile", "view_scale", "fog", "open", "world_held_by"} {
		if _, ok := game[k]; !ok {
			t.Fatalf("state.json's game has no %q: %v", k, keysOf(game))
		}
	}

	if game["world_held_by"] != "feedback" || game["view_scale"] != 0.5 && os.Getenv("STRIGOI_PLAYTEST_GAME") == "" {
		t.Fatalf("state.json's game: world_held_by %v view_scale %v", game["world_held_by"], game["view_scale"])
	}

	if frame := sub(state, "frame"); num(frame, "w") != 800 || num(frame, "h") != 600 {
		t.Fatalf("state.json's frame %v", frame)
	}

	if v := sub(state, "version"); str(v, "commit") == "" {
		t.Fatalf("state.json's version %v", v)
	}

	if tail, _ := state["log_tail"].([]any); len(tail) == 0 || len(tail) > 200 {
		t.Fatalf("state.json's log_tail has %d lines, want 1..200", len(tail))
	}

	if lines := ringLines(s, `FEEDBACK dir=`); len(lines) != 1 || !strings.Contains(lines[0], dir) ||
		!strings.Contains(lines[0], `text="the torch went out at 21:15, why? (day 1)"`) {
		t.Fatalf("the log's FEEDBACK line: %v", lines)
	}

	// Let go: the world runs again.
	if held := mustStr(t, uiState(s), "world_held_by"); held != "" {
		t.Fatalf("world_held_by %q after Enter, want \"\"", held)
	}

	s.call("strigoi_step_world", map[string]any{"world_minutes": 5.0})

	// ESCAPE CANCELS AND WRITES NOTHING -- and is the box's, not the game's.
	s.call("strigoi_key", map[string]any{"key": "f8"})
	s.call("strigoi_step", map[string]any{"frames": 2})
	s.call("strigoi_type_text", map[string]any{"text": "never mind"})
	s.call("strigoi_key", map[string]any{"key": "escape"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	fb = feedbackState(s)
	if flag(t, fb, "open") || mustNum(t, fb, "cancelled") != 1 || mustNum(t, fb, "saved") != 1 {
		t.Fatalf("after Escape: want closed, cancelled 1, saved still 1; got %v", fb)
	}

	if dirs := reportDirs(t, home, "feedback"); len(dirs) != 1 {
		t.Fatalf("Escape wrote a folder: %v", dirs)
	}

	ui = uiState(s)
	if flag(t, ui, "escape_menu_open") {
		t.Fatal("the Escape that cancelled the box also opened the escape menu")
	}

	if held := mustStr(t, ui, "world_held_by"); held != "" {
		t.Fatalf("world_held_by %q after Escape, want \"\"", held)
	}

	// POSITIVE CONTROL for the Q above: with the box shut, Q opens his journal.
	s.call("strigoi_key", map[string]any{"key": "q"})
	s.call("strigoi_step", map[string]any{"frames": 2})

	if !flag(t, uiState(s), "journal_open") {
		t.Fatal("positive control: Q did not open the journal with the box shut -- the check under the box proved nothing")
	}
}

// TestFeedbackF8OnTheMenu is F8 on the main menu: Strigoi's native menu, or
// -classic's under STRIGOI_PLAYTEST_GAME=classic. TestFeedbackF8OnClassicMenu
// is -classic's in every run.
func TestFeedbackF8OnTheMenu(t *testing.T) {
	feedbackOnTheMenu(t, start(t), os.Getenv("STRIGOI_PLAYTEST_GAME") == "classic")
}

func TestFeedbackF8OnClassicMenu(t *testing.T) {
	feedbackOnTheMenu(t, startWith(t, "-classic"), true)
}

func feedbackOnTheMenu(t *testing.T, s *session, classic bool) {
	t.Helper()

	home, err := testHome(t)
	if err != nil {
		t.Fatal(err)
	}

	fb := waitMainMenu(t, s)
	if str(fb, "start_notice") != "" || flag(t, fb, "start_drawn") {
		t.Fatalf("a first launch in a fresh home shows a start notice: %v", fb)
	}

	s.call("strigoi_key", map[string]any{"key": "f8"})
	waitFeedback(t, s, "the box open with its frame", func(fb map[string]any) bool {
		return fb["open"] == true && fb["shot_taken"] == true
	})

	if fb := feedbackState(s); fb["held_game"] != false {
		t.Fatalf("held_game on the menu: %v", fb["held_game"])
	}

	s.call("strigoi_type_text", map[string]any{"text": "the menu, " + feedbackNote})
	s.call("strigoi_key", map[string]any{"key": "enter"})

	fb = waitFeedback(t, s, "the note saved", func(fb map[string]any) bool { return num(fb, "saved") == 1 })

	dir := mustStr(t, fb, "last_dir")
	if dirs := reportDirs(t, home, "feedback"); len(dirs) != 1 || dirs[0] != dir {
		t.Fatalf("feedback folders %v, want exactly %s", dirs, dir)
	}

	for _, f := range []string{"note.txt", "shot.png", "state.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}

	state := readStateJSON(t, dir)
	if str(state, "screen") != "main_menu" || state["classic"] != classic || state["note"] != "the menu, "+feedbackNote {
		t.Fatalf("state.json on the menu: screen %v classic %v (want %v) note %v", state["screen"], state["classic"], classic, state["note"])
	}

	if _, ok := state["game"]; ok {
		t.Fatalf("state.json on the menu has a game: %v", state["game"])
	}

	// The menu was not pressed underneath: still the main menu.
	if str(feedbackState(s), "screen") != "main_menu" {
		t.Fatal("the menu moved under the box")
	}
}

// TestCrashReports forces each way a run can end badly and reads what the
// next launch finds: a panic on the game goroutine (update, then draw) writes
// its own folder and the next menu names it; a panic on a goroutine nothing
// recovers is the runtime's fatal error, which the next launch reports from
// the marker and the runtime's crash output; a kill is "did not close
// cleanly"; and a clean quit leaves nothing to report.
func TestCrashReports(t *testing.T) {
	home, err := testHome(t)
	if err != nil {
		t.Fatal(err)
	}

	crashes := func() []string { return reportDirs(t, home, "crashes") }

	// 1. A panic in the frame's update.
	s1 := start(t)
	waitMainMenu(t, s1)
	s1.callDying("strigoi_run_console", map[string]any{"command": "crashtest update"})

	if code := s1.waitExit(t); code != 1 {
		t.Fatalf("a caught panic exits 1, got %d", code)
	}

	dirs := crashes()
	if len(dirs) != 1 || strings.HasSuffix(dirs[0], "-unclean") {
		t.Fatalf("crash folders %v, want one written by the crash itself", dirs)
	}

	dir1 := dirs[0]
	assertCrashFolder(t, dir1, "update", true)

	// 2. The next launch names it on the menu.
	s2 := start(t)

	fb := waitMainMenu(t, s2)
	if want := "The last run crashed -- a report was saved to " + dir1; str(fb, "start_notice") != want ||
		str(fb, "start_dir") != dir1 || !flag(t, fb, "start_drawn") {
		t.Fatalf("the menu after a crash: notice %q drawn %v, want %q", str(fb, "start_notice"), fb["start_drawn"], want)
	}

	if len(crashes()) != 1 {
		t.Fatalf("the launch after a caught crash wrote a second report: %v", crashes())
	}

	// The notice is on the frame: the strip it draws across the top.
	if c := rectMean(s2.frame(t, "crash-notice"), image.Rect(792, 0, 800, 24)); c[0] > 45 || c[1] > 45 || c[2] > 45 {
		t.Fatalf("the notice strip's right end is %v, want the dark strip", c)
	}

	// 3. A panic in the frame's draw.
	s2.callDying("strigoi_run_console", map[string]any{"command": "crashtest draw"})

	if code := s2.waitExit(t); code != 1 {
		t.Fatalf("a caught draw panic exits 1, got %d", code)
	}

	dirs = crashes()
	if len(dirs) != 2 {
		t.Fatalf("crash folders after the draw crash: %v", dirs)
	}

	dir2 := dirs[1]
	assertCrashFolder(t, dir2, "draw", true)

	// 4. A panic on a goroutine nothing recovers: the runtime's fatal error.
	s3 := start(t)

	fb = waitMainMenu(t, s3)
	if str(fb, "start_dir") != dir2 {
		t.Fatalf("the menu after the draw crash names %q, want %q", str(fb, "start_dir"), dir2)
	}

	s3.callDying("strigoi_run_console", map[string]any{"command": "crashtest goroutine"})

	if code := s3.waitExit(t); code != 2 {
		t.Fatalf("an uncaught panic exits 2 (the runtime's), got %d", code)
	}

	if len(crashes()) != 2 {
		t.Fatalf("an uncaught panic wrote its own folder (nothing should catch it): %v", crashes())
	}

	s4 := start(t)

	fb = waitMainMenu(t, s4)

	dir3 := str(fb, "start_dir")
	if !strings.HasSuffix(dir3, "-unclean-harness") || str(fb, "start_notice") != "The last run crashed -- a report was saved to "+dir3 {
		t.Fatalf("the menu after an uncaught panic: %v", fb)
	}

	stack := readText(t, filepath.Join(dir3, "stack.txt"))
	if !strings.Contains(stack, "crashtest: a crash forced on the goroutine goroutine") || !strings.Contains(stack, "harnessForcedCrash") {
		t.Fatalf("the runtime's stack.txt does not name the forced crash:\n%.2000s", stack)
	}

	if !strings.Contains(readText(t, filepath.Join(dir3, "marker.json")), `"harness": true`) {
		t.Fatal("the unclean report's marker.json is not the harness run's")
	}

	// 5. A kill: did not close cleanly, no stack.
	pid4 := s4.cmd.Process.Pid
	_ = s4.cmd.Process.Kill()
	s4.waitExit(t)

	s5 := start(t)

	fb = waitMainMenu(t, s5)

	dir4 := str(fb, "start_dir")
	if !strings.HasSuffix(dir4, "-unclean-harness") || str(fb, "start_notice") != "The last run did not close cleanly -- a report was saved to "+dir4 {
		t.Fatalf("the menu after a kill: %v", fb)
	}

	if _, err := os.Stat(filepath.Join(dir4, "stack.txt")); !os.IsNotExist(err) {
		t.Fatalf("a killed run's report has a stack.txt (%v): nothing crashed", err)
	}

	var marker map[string]any
	if err := json.Unmarshal([]byte(readText(t, filepath.Join(dir4, "marker.json"))), &marker); err != nil || int(num(marker, "pid")) != pid4 {
		t.Fatalf("the killed run's marker.json %v (%v), want pid %d", marker, err, pid4)
	}

	for _, f := range []string{"log-tail.txt", "state.json"} {
		if _, err := os.Stat(filepath.Join(dir4, f)); err != nil {
			t.Fatalf("the killed run's %s: %v", f, err)
		}
	}

	// 6. A clean quit leaves nothing to report.
	s5.stop()

	if code := s5.cmd.ProcessState.ExitCode(); code != 0 {
		t.Fatalf("strigoi_quit exited %d", code)
	}

	s6 := start(t)

	fb = waitMainMenu(t, s6)
	if str(fb, "start_notice") != "" || flag(t, fb, "start_drawn") {
		t.Fatalf("the launch after a clean quit shows a notice: %v", fb)
	}

	if n := len(crashes()); n != 4 {
		t.Fatalf("crash folders after the clean quit: %d, want the 4 above", n)
	}

	running, _ := os.ReadDir(filepath.Join(home, "running"))

	var names []string
	for _, e := range running {
		names = append(names, e.Name())
	}

	if len(names) != 2 { // s6's marker and its fatal-error file
		t.Fatalf("running\\ holds %v, want only the live game's two files", names)
	}
}

// assertCrashFolder reads a crash folder a caught panic wrote.
func assertCrashFolder(t *testing.T, dir, where string, onMenu bool) {
	t.Helper()

	stack := readText(t, filepath.Join(dir, "stack.txt"))
	for _, want := range []string{
		"panic: crashtest: a crash forced on the " + where + " goroutine",
		"goroutine: " + where,
		"harnessForcedCrash",
		"--- all goroutines ---",
	} {
		if !strings.Contains(stack, want) {
			t.Fatalf("%s stack.txt lacks %q:\n%.2000s", where, want, stack)
		}
	}

	if tail := readText(t, filepath.Join(dir, "log-tail.txt")); !strings.Contains(tail, "OpenDiablo2 PANIC on the "+where+" goroutine") {
		t.Fatalf("%s log-tail.txt does not hold the PANIC line (%d bytes)", where, len(tail))
	}

	state := readStateJSON(t, dir)
	if str(state, "goroutine") != where || !strings.Contains(str(state, "panic"), "crashtest") {
		t.Fatalf("%s state.json: %v", where, state)
	}

	if onMenu && str(state, "screen") != "main_menu" {
		t.Fatalf("%s state.json screen %q, want main_menu", where, str(state, "screen"))
	}

	if run := sub(state, "run"); run["harness"] != true || num(run, "pid") == 0 {
		t.Fatalf("%s state.json's run %v", where, run)
	}
}
