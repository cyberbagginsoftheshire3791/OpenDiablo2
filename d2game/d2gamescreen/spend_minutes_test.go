package d2gamescreen

import (
	"math"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2hero"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// spendMinutes moves the world by exactly the minutes asked, even across a
// change of stage. The clock's rate is the stage's (day 4, night 2.5 world
// minutes a second), and a sleep or a labour that turned minutes into seconds
// at the rate it STARTED with was paid at the wrong rate once the stage
// turned: every step after dawn gave 1.6 times its minutes (four hours asleep
// from 23:00 slept 246, from 01:00 it slept 318), and half an
// hour's labour from 21:05 lost 7.5 minutes to the night (history item 117;
// found by the default-game sweep, where TestSurvive's sleep crossed dawn and
// slept 246.1 minutes).
func TestSpentMinutesAreExactAcrossAStage(t *testing.T) {
	dials := d2world.DefaultClockDials()

	for _, c := range []struct {
		name    string
		at      float64 // minute of day to start
		minutes float64
	}{
		{"a night's sleep into dawn", 23 * 60, 240},
		{"a long sleep into dawn", 60, 240},
		{"labour from dusk into night", 21*60 + 5, 30},
		{"a sleep within the day", 9 * 60, 240},
	} {
		clock := d2world.NewClock(dials)
		t.Cleanup(clock.Close)

		// From the epoch (02:45, dawn) to the start, on the first day or the
		// small hours after it: the day's stages run at the day rate up to
		// 21:15, then the night's until the next dawn.
		const epoch = 2*60 + 45

		since := math.Mod(c.at-epoch+1440, 1440)
		day := math.Min(since, dials.NightStart-epoch)
		clock.Advance(day / dials.DayRate)

		if night := since - day; night > 0 {
			clock.Advance(night / dials.NightRate)
		}

		if got := clock.MinuteOfDay(); math.Abs(got-c.at) > 1e-6 {
			t.Fatalf("%s: set-up put the clock at minute %.3f, want %.0f", c.name, got, c.at)
		}

		v := &Game{
			worldClock:  clock,
			localPlayer: &d2mapentity.Player{Stats: &d2hero.HeroStatsState{Health: 1}},
		}

		before := clock.WorldMinutes()
		v.spendMinutes(c.minutes, 0, "")

		if spent := clock.WorldMinutes() - before; math.Abs(spent-c.minutes) > 1e-6 {
			t.Errorf("%s: spent %.3f world minutes, want %.0f", c.name, spent, c.minutes)
		}
	}
}
