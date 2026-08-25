//go:build playtest

package playtest

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// TestMinimizedTick records whether the game keeps ticking while its window
// is minimized (P3 spec §2.1 UNKNOWN; the answer lands in docs/harness.md).
// Opt-in — it minimizes every window on the desktop for ~5 seconds:
//
//	STRIGOI_TEST_MINIMIZED=1 go test -tags playtest ./playtest/ -run Minimized -v
func TestMinimizedTick(t *testing.T) {
	if os.Getenv("STRIGOI_TEST_MINIMIZED") == "" {
		t.Skip("set STRIGOI_TEST_MINIMIZED=1 to run (it minimizes your windows)")
	}

	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}

	s := start(t)

	before := s.call("strigoi_ping", map[string]any{})

	if err := exec.Command("powershell", "-NoProfile", "-Command",
		"(New-Object -ComObject Shell.Application).MinimizeAll()").Run(); err != nil {
		t.Fatalf("minimize: %v", err)
	}

	time.Sleep(5 * time.Second)

	during := s.call("strigoi_ping", map[string]any{})

	_ = exec.Command("powershell", "-NoProfile", "-Command",
		"(New-Object -ComObject Shell.Application).UndoMinimizeALL()").Run()

	time.Sleep(1 * time.Second)

	after := s.call("strigoi_ping", map[string]any{})

	t.Logf("tick before minimize: %.0f · after 5s minimized: %.0f (delta %.0f ≈ %.0f/s) · after restore: %.0f",
		num(before, "tick"), num(during, "tick"), num(during, "tick")-num(before, "tick"),
		(num(during, "tick")-num(before, "tick"))/5, num(after, "tick"))

	if num(during, "tick") == num(before, "tick") {
		t.Log("FINDING: the loop FREEZES while minimized — harness calls will stall in a minimized window")
	} else {
		t.Log("FINDING: the loop keeps ticking while minimized")
	}
}
