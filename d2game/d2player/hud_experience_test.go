package d2player

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2progress"
)

// progressOf is a ProgressHolder with a fixed standing.
type progressOf struct {
	p    *d2progress.Progress
	tree *d2progress.Tree
}

func (h progressOf) Progress() (*d2progress.Progress, *d2progress.Tree) { return h.p, h.tree }
func (h progressOf) PickTalent(string) error                            { return nil }

// The HUD's experience bar and its tooltip show Strigoi's own progression in
// his game -- the talent panel's numbers: his experience, and the total his
// next level needs -- not Diablo II's, which Strigoi's game never writes and
// whose table it does not load, so the tooltip read "0 / 0" (the tables
// burst's review, 27 Sep 2026). At the top level the bar is full. -classic
// keeps Diablo II's.
//
// Negative control (27 Sep 2026): make heroExperience return Diablo II's
// numbers whatever the game and the Strigoi cases read 0 / 0.
func TestTheHUDShowsStrigoisExperience(t *testing.T) {
	tree := &d2progress.Tree{Levels: []int{0, 50, 150}}

	for _, c := range []struct {
		name      string
		classic   bool
		holder    ProgressHolder
		xp, next  int
		d2, d2Nxt int
	}{
		{"level 2, toward 3", false, progressOf{&d2progress.Progress{XP: 60}, tree}, 60, 150, 0, 0},
		{"fresh", false, progressOf{&d2progress.Progress{}, tree}, 0, 50, 0, 0},
		{"the top level: a full bar", false, progressOf{&d2progress.Progress{XP: 400}, tree}, 400, 400, 0, 0},
		{"no progress bound: Diablo II's", false, nil, 7, 500, 7, 500},
		{"-classic: Diablo II's", true, progressOf{&d2progress.Progress{XP: 60}, tree}, 7, 500, 7, 500},
	} {
		xp, next := heroExperience(c.classic, c.holder, c.d2, c.d2Nxt)
		if xp != c.xp || next != c.next {
			t.Errorf("%s: %d / %d, want %d / %d", c.name, xp, next, c.xp, c.next)
		}
	}
}
