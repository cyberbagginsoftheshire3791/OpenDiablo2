package d2mapentity

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
)

// THE BUG-87 REVIEW FIXES (29 Sep 2026): its B1 (BUG-91), its B2 (BUG-92), and
// BUG-89. The dog's sheets: attack 10 frames, hit 6, death 12, idle and walk
// 8, dead 1 -- each played in the PNG default of one second.

// b89HeldTicks is how many ticks c holds a bite started now.
func b89HeldTicks(t *testing.T, c *Creature) int {
	t.Helper()

	require.NoError(t, c.StartAction(d2enum.MonsterAnimationModeAttack1, nil))

	n := 0
	for ; n < 1000 && c.held; n++ {
		c.Advance(b2bTick)
	}

	require.False(t, c.held, "the bite ended")

	return n
}

// BUG-89: A CREATURE'S BITE LASTS ITS SHEET, TURNED OR NOT. Standing, a feral
// dog's bite holds 61 ticks at 1/60 s (ten frames of 0.1 s, and the tick that
// plays it through). Walking, it reaches a waypoint during the bite -- the
// map calls its directioner there -- and turns (the route bends), or does not
// (the route runs straight on): either way the bite ends on the 61st tick.
// Before the fix, Creature.rotate put the sheet back to its first frame at
// every waypoint, and the bite started again.
func TestABiteAcrossAWaypointLastsItsSheet(t *testing.T) {
	standing := b89HeldTicks(t, b2bNewDog(t, b2bFactory(t), 50, 50))
	require.Equal(t, 61, standing, "a standing dog's bite")

	for _, c := range []struct {
		name  string
		route []d2vector.Position
		turns bool
	}{
		{"a waypoint where the route bends", []d2vector.Position{
			d2vector.NewPosition(51, 50), d2vector.NewPosition(51, 60), d2vector.NewPosition(60, 60),
		}, true},
		{"waypoints on a straight route", []d2vector.Position{
			d2vector.NewPosition(51, 50), d2vector.NewPosition(52, 50), d2vector.NewPosition(53, 50), d2vector.NewPosition(70, 50),
		}, false},
	} {
		dog := b2bNewDog(t, b2bFactory(t), 50, 50)
		dog.SetSpeed(7)
		dog.SetPath(c.route, nil)

		facing, ahead := dog.direction, len(dog.path)

		held := b89HeldTicks(t, dog)

		require.Less(t, len(dog.path), ahead, "%s: the dog passed a waypoint during the bite", c.name)
		require.Equal(t, c.turns, dog.direction != facing, "%s: turned (%d to %d)", c.name, facing, dog.direction)
		require.Equal(t, standing, held, "%s: the bite lasts its sheet", c.name)

		t.Logf("%-34s the bite held %d ticks (standing: %d); facing %d -> %d, %d waypoint(s) passed",
			c.name, held, standing, facing, dog.direction, ahead-len(dog.path))
	}
}

// BUG-89: A TURN KEEPS THE SHEET'S FRAME AND THE TIME INTO IT, and a
// directioner call on the facing the creature already has changes nothing.
func TestATurnKeepsTheSheetsFrame(t *testing.T) {
	dog := b2bNewDog(t, b2bFactory(t), 50, 50)
	require.NoError(t, dog.StartAction(d2enum.MonsterAnimationModeDeath, nil))

	for i := 0; i < 23; i++ {
		dog.Advance(b2bTick)
	}

	frame, elapsed := dog.animation.Progress()
	require.Positive(t, frame)

	dog.rotate(dog.direction)
	f, e := dog.animation.Progress()
	require.Equal(t, [2]interface{}{frame, elapsed}, [2]interface{}{f, e}, "the same facing: nothing")

	require.Zero(t, dog.direction)
	dog.rotate(8)
	require.Equal(t, 8, dog.direction)

	f, e = dog.animation.Progress()
	require.Equal(t, [2]interface{}{frame, elapsed}, [2]interface{}{f, e}, "a turn: the same frame and time")
}

// b92DogMidDeath is a dog ticks into its death, and its motion.
func b92DogMidDeath(t *testing.T, ticks int) (*Creature, Motion) {
	t.Helper()

	a := b2bNewDog(t, b2bFactory(t), 50, 50)
	require.NoError(t, a.StartAction(d2enum.MonsterAnimationModeDeath, nil))

	for i := 0; i < ticks; i++ {
		a.Advance(b2bTick)
	}

	require.True(t, a.held)

	return a, a.MotionSnapshot()
}

// b92Fresh is a dog a load rebuilds, in another factory, with its saved id;
// art, when not nil, changes its sheets first -- the art pass that landed
// between the save and the load.
func b92Fresh(t *testing.T, id string, art func(c *Creature)) *Creature {
	t.Helper()

	f := b2bFactory(t)
	require.NoError(t, f.SetNextEntityID(id))

	c := b2bNewDog(t, f, 0, 0)
	if art != nil {
		art(c)
	}

	return c
}

// B2 (BUG-92): A HELD ACTION THE ART NO LONGER FITS IS ENDED, NOT REFUSED --
// one case for each way an art pass between the save and the load can leave
// it: a sheet re-exported with FEWER frames (the saved frame gone), one with
// MORE (its frames shorter than the time saved into one), a sheet ADDED for
// an action that had fallen back to idle, and one TAKEN AWAY. ResumeMotion
// ends the action as it would have ended and says which and why; the motion
// it returns is the one the dog then has (the load's read-back); and that is
// the state the saved dog reaches on its own when the action plays out -- a
// bite or a blow back to idle, a death a corpse where it fell -- and the two
// are the same dog on every tick after. RestoreMotion, the strict restore,
// refuses a frame or a time the art lacks, changing nothing; an action drawn
// in another mode it restores, and the load's read-back refused that (it
// reads back another mode than the file's). BUG-87 refused all four.
func TestAHeldActionTheArtNoLongerFitsIsEnded(t *testing.T) {
	type art = func(c *Creature)

	for _, c := range []struct {
		name  string
		saved func(t *testing.T) (*Creature, Motion)
		art   art
		why   string
		mode  bool // the mode changed: the strict restore takes it and the read-back differs
	}{
		{
			"a death re-exported with fewer frames (12 to 6), saved at frame 8",
			func(t *testing.T) (*Creature, Motion) { return b92DogMidDeath(t, 40) },
			func(c *Creature) { c.animations[creatureDeath] = c.animations[creatureHit].Clone() },
			"saved at frame 8, and its art has 6", false,
		},
		{
			"a flinch re-exported with more frames (6 to 12), saved 0.15 s into a frame",
			func(t *testing.T) (*Creature, Motion) {
				a := b2bNewDog(t, b2bFactory(t), 50, 50)
				require.NoError(t, a.StartAction(d2enum.MonsterAnimationModeGetHit, nil))

				for i := 0; i < 19; i++ {
					a.Advance(b2bTick)
				}

				mo := a.MotionSnapshot()
				require.Greater(t, mo.ActionAt.Elapsed, 1.0/12, "longer than a frame of 12 in a second")

				return a, mo
			},
			func(c *Creature) { c.animations[creatureHit] = c.animations[creatureDeath].Clone() },
			"a frame of its art lasts", false,
		},
		{
			"a bite that fell back to idle (no sheet), a sheet added since",
			func(t *testing.T) (*Creature, Motion) {
				a := b2bNewDog(t, b2bFactory(t), 50, 50)
				delete(a.animations, creatureAttack)
				require.NoError(t, a.StartAction(d2enum.MonsterAnimationModeAttack1, nil))

				for i := 0; i < 10; i++ {
					a.Advance(b2bTick)
				}

				mo := a.MotionSnapshot()
				require.Equal(t, "idle", mo.Mode, "the bite is drawn on the idle sheet")

				return a, mo
			},
			nil,
			`saved drawn as "idle", and this build draws it as "attack"`, true,
		},
		{
			"a bite saved on its own sheet, the sheet taken away since",
			func(t *testing.T) (*Creature, Motion) {
				a := b2bNewDog(t, b2bFactory(t), 50, 50)
				require.NoError(t, a.StartAction(d2enum.MonsterAnimationModeAttack1, nil))

				for i := 0; i < 10; i++ {
					a.Advance(b2bTick)
				}

				return a, a.MotionSnapshot()
			},
			func(c *Creature) { delete(c.animations, creatureAttack) },
			`saved drawn as "attack", and this build draws it as "idle"`, true,
		},
	} {
		a, mo := c.saved(t)

		strict := b92Fresh(t, a.ID(), c.art)
		before := b2bSeen(t, strict)

		if err := strict.RestoreMotion(mo); c.mode {
			require.NoError(t, err, c.name)
			require.NotEqual(t, mo, strict.MotionSnapshot(), "%s: the read-back refused it", c.name)
		} else {
			require.Error(t, err, "%s: the strict restore refuses it", c.name)
			require.Equal(t, before, b2bSeen(t, strict), "%s: and changes nothing", c.name)
		}

		b := b92Fresh(t, a.ID(), c.art)
		restored, ended, err := b.ResumeMotion(mo)
		require.NoError(t, err, "%s: resumed, not refused", c.name)
		require.NotNil(t, ended, "%s: and said so", c.name)
		require.Equal(t, mo.Action, ended.Action, c.name)
		require.Contains(t, ended.Why, c.why, c.name)
		require.Contains(t, ended.String(), "an animation could not be resumed exactly", c.name)
		require.Equal(t, restored, b.MotionSnapshot(), "%s: the dog is what ResumeMotion said", c.name)
		require.False(t, b.held, "%s: the action ended", c.name)
		require.Empty(t, restored.Action, c.name)
		require.Equal(t, mo.Pos, restored.Pos, "%s: where the file put it", c.name)

		// The state the saved dog reaches on its own: played out to the
		// tick its action ends, it is the ended one, and the two are the
		// same dog on every tick after.
		played := 0
		for ; played < 1000 && a.held; played++ {
			a.Advance(b2bTick)
		}

		require.Equal(t, b2bSeen(t, a), b2bSeen(t, b), "%s: the state its end reaches (played %d ticks to it)", c.name, played)
		require.Equal(t, mo.Action == "death", b.corpse, "%s: a death ends a corpse", c.name)

		for i := 0; i < 60; i++ {
			a.Advance(b2bTick)
			b.Advance(b2bTick)
			require.Equal(t, b2bSeen(t, a), b2bSeen(t, b), "%s: tick %d after", c.name, i)
		}

		t.Logf("%-72s resumed ended as %q (the saved dog reached it %d ticks on); %s", c.name, restored.Mode, played, ended)
	}
}

// B1 AND B2 (BUG-91, BUG-92): A CORRUPT HELD ACTION IS STILL REFUSED by
// ResumeMotion as by RestoreMotion, changing nothing -- a time below the
// floor, a negative frame, a time at or past the whole play of the sheet (a
// creature's sheet plays in one second whatever its frames, so no re-export
// makes a saved time reach it) -- while a time the art could have written is
// resumed exactly, with nothing ended.
func TestACorruptHeldActionIsStillRefused(t *testing.T) {
	a, mo := b92DogMidDeath(t, 17)

	for name, edit := range map[string]func(at *ActionProgress){
		"a second before its frame (-1.0)": func(at *ActionProgress) { at.Elapsed = -1.0 },
		"a hair below the floor":           func(at *ActionProgress) { at.Elapsed = -2e-9 },
		"a negative frame":                 func(at *ActionProgress) { at.Frame = -1 },
		"the whole play (1.0 s)":           func(at *ActionProgress) { at.Elapsed = 1.0 },
		"1e17 s":                           func(at *ActionProgress) { at.Elapsed = 1e17 },
		"1e18 s":                           func(at *ActionProgress) { at.Elapsed = 1e18 },
		"1e300 s":                          func(at *ActionProgress) { at.Elapsed = 1e300 },
		"1e18 s at a frame the sheet lacks": func(at *ActionProgress) {
			at.Frame, at.Elapsed = 40, 1e18
		},
	} {
		bad := mo
		at := *mo.ActionAt
		edit(&at)
		bad.ActionAt = &at

		b := b92Fresh(t, a.ID(), nil)
		before := b2bSeen(t, b)

		_, ended, err := b.ResumeMotion(bad)
		require.Error(t, err, "%s: refused", name)
		require.Nil(t, ended, name)
		require.Equal(t, before, b2bSeen(t, b), "%s: a refusal changes nothing", name)

		require.Error(t, b92Fresh(t, a.ID(), nil).RestoreMotion(bad), "%s: the strict restore refuses it too", name)
		t.Logf("%-36s refused: %v", name, err)
	}

	for _, e := range []float64{-1e-17, 0, math.Nextafter(1.0/12, 0)} {
		ok := mo
		at := *mo.ActionAt
		at.Elapsed = e
		ok.ActionAt = &at

		b := b92Fresh(t, a.ID(), nil)
		restored, ended, err := b.ResumeMotion(ok)
		require.NoError(t, err, "%v s", e)
		require.Nil(t, ended, "%v s: resumed at its frame, nothing ended", e)
		require.Equal(t, ok, restored)
		require.Equal(t, ok, b.MotionSnapshot())
	}
}

// b89Spy is a sheet that counts the turns asked of it.
type b89Spy struct {
	d2interface.Animation
	turns int
}

func (s *b89Spy) SetDirection(direction int) error {
	s.turns++

	return s.Animation.SetDirection(direction)
}

// BUG-89, THE GUARD: A WAYPOINT ON THE FACING THE CREATURE ALREADY HAS IS NO
// TURN AT ALL -- the sheet is not even asked to turn (NPC.rotate has always
// had the guard; Creature.rotate did not, and the map calls the directioner
// at every waypoint). A dog biting while it walks a straight route passes
// its waypoints and its sheet is asked to turn no time; the bite lasts its
// sheet.
func TestAWaypointOnTheSameFacingDoesNotTurnTheSheet(t *testing.T) {
	dog := b2bNewDog(t, b2bFactory(t), 50, 50)
	spy := &b89Spy{Animation: dog.animations[creatureAttack]}
	dog.animations[creatureAttack] = spy

	dog.SetSpeed(7)
	dog.SetPath([]d2vector.Position{
		d2vector.NewPosition(51, 50), d2vector.NewPosition(52, 50), d2vector.NewPosition(53, 50), d2vector.NewPosition(70, 50),
	}, nil)

	require.NoError(t, dog.StartAction(d2enum.MonsterAnimationModeAttack1, nil))

	facing, ahead := dog.direction, len(dog.path)
	spy.turns = 0 // StartAction faced the sheet as it put the dog in it

	n := 0
	for ; n < 1000 && dog.held; n++ {
		dog.Advance(b2bTick)
	}

	require.Less(t, len(dog.path), ahead, "the dog passed a waypoint during the bite")
	require.Equal(t, facing, dog.direction, "on the same facing")
	require.Zero(t, spy.turns, "the sheet was asked to turn")
	require.Equal(t, 61, n, "the bite lasts its sheet")
	t.Logf("%d waypoint(s) passed on facing %d during a %d-tick bite; the sheet asked to turn %d times", ahead-len(dog.path), facing, n, spy.turns)
}
