package d2gamescreen

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maprenderer"
)

// TestTheZoomFlagIsTheNewGamesScale: -zoom's value (SetGameZoom, as d2app
// calls it) is the scale a new game's map renderer starts at, clamped to the
// game's 0.4..2.0; at 1.0 the renderer is left as it was made.
//
// Negative controls (1 Oct 2026, the review's M8 and M6): make applyGameZoom
// skip every zoom but 1.0 and this fails, "-zoom 0.5: a new game draws at 1"
// (nc24-zoom-flag-ignored.txt); store -zoom's value unclamped and it fails,
// "-zoom 0.1: a new game draws at 0.1; the game's range stops at 0.4"
// (nc25-zoom-unclamped.txt).
func TestTheZoomFlagIsTheNewGamesScale(t *testing.T) {
	was := GameZoom()
	t.Cleanup(func() { SetGameZoom(was) })

	for _, c := range []struct{ flag, want float64 }{{0.5, 0.5}, {0.4, 0.4}, {0.1, 0.4}, {1.5, 1.5}, {2, 2}, {3, 2}, {1, 1}} {
		SetGameZoom(c.flag)

		v := &Game{mapRenderer: d2maprenderer.NewViewOnlyMapRenderer(0, 0)}
		v.applyGameZoom()

		if got := v.mapRenderer.Scale(); got != c.want {
			if c.flag < 0.4 {
				t.Errorf("-zoom %v: a new game draws at %v; the game's range stops at 0.4", c.flag, got)
			} else {
				t.Errorf("-zoom %v: a new game draws at %v, want %v", c.flag, got, c.want)
			}
		}
	}
}
