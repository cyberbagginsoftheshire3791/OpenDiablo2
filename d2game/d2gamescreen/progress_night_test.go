package d2gamescreen

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2dialogue"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2hero"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2progress"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// A night lived through pays once, at the turn to dawn, and only to the living.
// The first case is the one the T3 review caught: a session opening AT dawn
// with lastStage still its zero value (night) must not pay -- bindProgress sets
// lastStage and dawnPaidDay from the clock, which is what the second case is.
func TestNightSurvived(t *testing.T) {
	night, dawn, fullDay, dusk := d2world.StageNight, d2world.StageDawn, d2world.StageDay, d2world.StageDusk

	cases := []struct {
		name          string
		last, now     d2world.Stage
		day, paid     int
		alive, expect bool
	}{
		{"the turn to dawn pays", night, dawn, 3, 2, true, true},
		{"a dawn already paid does not", night, dawn, 3, 3, true, false},
		{"the dead are not paid", night, dawn, 3, 2, false, false},
		{"dawn to dawn is no turn", dawn, dawn, 3, 2, true, false},
		{"night to night is no turn", night, night, 3, 2, true, false},

		// BUG-17: a labour or a sleep carries him over dawn between two
		// frames, so the frame after it sees night, then full day.
		{"a jump from the night into full day ends it too", night, fullDay, 3, 2, true, true},
		{"and a day already paid still does not", night, fullDay, 3, 3, true, false},
		{"nor does a dead man's", night, fullDay, 3, 2, false, false},
		{"day to day is no turn", fullDay, fullDay, 3, 2, true, false},
		{"dusk into the night is no end of it", dusk, night, 3, 2, true, false},

		// Review C6 (27 Sep): a jump from the night past the whole day to dusk
		// ended the night too. Without this case the rule "now is dawn or day"
		// passed every row above.
		{"a jump from the night over the day to dusk ends it too", night, dusk, 3, 2, true, true},
	}

	for _, c := range cases {
		if got := nightSurvived(c.last, c.now, c.day, c.paid, c.alive); got != c.expect {
			t.Errorf("%s: got %v", c.name, got)
		}
	}

	// The zero value of a Stage is night: the reason bindProgress must seed it.
	var zero d2world.Stage
	if zero != night {
		t.Fatal("the zero Stage is no longer night; revisit bindProgress's seeding comment")
	}
}

// BUG-17, the row's own reproduction on a real clock, through the real spend
// and the real frame: the watch promised and stood through the evening, then
// three hours at the smith's anvil from 01:30, then the next frame. Under the
// old rule (night to DAWN only) that frame saw night, then full day: no night's
// experience, and the watch left promised with its minutes about to be zeroed.
func TestALabourAcrossDawnStillEndsTheNight(t *testing.T) {
	dials := d2world.DefaultClockDials()
	clock := d2world.NewClock(dials)
	t.Cleanup(clock.Close)

	// From the epoch (02:45, dawn, day 0) to 01:30 the next morning: the day's
	// stages at the day rate up to NightStart, then the night's rate.
	const epoch, at = 2*60 + 45, 1*60 + 30.0

	since := math.Mod(at-epoch+1440, 1440)
	daylight := math.Min(since, dials.NightStart-epoch)
	clock.Advance(daylight / dials.DayRate)
	clock.Advance((since - daylight) / dials.NightRate)

	if clock.Stage() != d2world.StageNight || clock.DayIndex() != 1 || math.Abs(clock.MinuteOfDay()-at) > 1e-6 {
		t.Fatalf("set-up: want 01:30 of day 1 in the night; got minute %.3f of day %d, %s",
			clock.MinuteOfDay(), clock.DayIndex(), clock.Stage())
	}

	tree, err := d2progress.Load(readData(t, "talents.json"))
	if err != nil {
		t.Fatal(err)
	}

	book, err := d2dialogue.Load(readData(t, "dialogue.json"))
	if err != nil {
		t.Fatal(err)
	}

	standing := book.NewStanding()
	standing.Mark(d2dialogue.FlagWatch)
	rep := standing.Rep

	v := &Game{
		worldClock:  clock,
		localPlayer: &d2mapentity.Player{Stats: &d2hero.HeroStatsState{Health: 1}},
		progress:    &d2progress.Progress{},
		talents:     tree,
		dialogue:    book,
		standing:    standing,
		lastStage:   d2world.StageNight, // the frame before the work
		dawnPaidDay: 0,                  // day 0's dawn, the one he arrived at
		watchStood:  book.Village.WatchMinutes,
	}

	v.spendMinutes(180, 0, d2world.ActivityLabour)

	if clock.Stage() != d2world.StageDay {
		t.Fatalf("the anvil's three hours from 01:30 end in full day: %s at minute %.1f", clock.Stage(), clock.MinuteOfDay())
	}

	// Not this test's subject: earnExperience reads the fights' events off a
	// combat model, and a fresh one has none.
	v.combat = d2world.NewCombat(clock, nil, nil, nil, v, nil, nil, nil, nil, 1, d2world.DefaultCombatDials())
	t.Cleanup(v.combat.Close)

	v.earnExperience()

	if v.progress.XP != tree.XP.Night {
		t.Errorf("the night he worked through pays %d experience; he has %d", tree.XP.Night, v.progress.XP)
	}

	if standing.Has(d2dialogue.FlagWatch) || v.watchStood != 0 || standing.Rep != rep+book.Village.Watch {
		t.Errorf("the watch he stood is settled and paid (+%d): promised %v, stood %.0f, rep %d -> %d",
			book.Village.Watch, standing.Has(d2dialogue.FlagWatch), v.watchStood, rep, standing.Rep)
	}

	v.earnExperience()

	if v.progress.XP != tree.XP.Night {
		t.Errorf("and only once: %d experience after a second frame", v.progress.XP)
	}
}

func readData(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "data", "strigoi", name))
	if err != nil {
		t.Fatal(err)
	}

	return data
}
