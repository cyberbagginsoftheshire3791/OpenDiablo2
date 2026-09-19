package d2world

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Combat.Encounter, M4.4c-2a. The wish note asks which fight the player is in
// AT THE MOMENT HE WRITES, and the two other things that look like an answer
// are not one:
//
//   - LastRound().Encounter is the last CLOSED round, so it is empty for the
//     whole of round one and it keeps naming a fight that has ended
//   - LastPace().Encounter only exists after a fight is over
//
// Each assertion below therefore has the pair that makes it mean something:
// live-and-named against closed-and-empty, and the round-one case against the
// stand-in that fails it.
func TestEncounterIsEmptyWithNoFight(t *testing.T) {
	f := humanFight(t, 1462, nil)

	assert.Empty(t, f.c.Encounter(), "nothing is happening, so no fight is named")
	assert.False(t, f.c.Fighting())
}

// TestEncounterNamesTheFightInRoundOne is the one that rules out the stand-in.
// The fight is open, no round has closed, and the wish note still has to say
// which fight it is.
func TestEncounterNamesTheFightInRoundOne(t *testing.T) {
	f := humanFight(t, 1462, nil)
	f.add(t, "e:1", 40, Profile{})
	f.open(t)

	require.True(t, f.c.Fighting())

	live := f.c.Encounter()
	assert.NotEmpty(t, live, "an open fight must be nameable before any round closes")

	// THE PAIR. The stand-in cannot answer this, which is why the accessor
	// exists at all. If this ever stops being true the accessor is redundant
	// and should go, rather than sit here unexplained.
	assert.Empty(t, f.c.LastRound().Encounter,
		"no round has closed yet, so LastRound cannot be the source")
}

// TestEncounterGoesQuietWhenTheFightEnds is the other half, and it is the
// assertion negative-control row 9 breaks: a wish written after the fight is
// over must read "-", not the fight the player has just walked away from.
func TestEncounterGoesQuietWhenTheFightEnds(t *testing.T) {
	f := humanFight(t, 1462, nil)
	f.add(t, "e:1", 1, Profile{})
	f.open(t)

	live := f.c.Encounter()
	require.NotEmpty(t, live)

	// Kill the one enemy outright: the fight ends on the blow that lands.
	for i := 0; i < 20 && f.c.Fighting(); i++ {
		f.round()

		if f.c.Awaiting() {
			// The pace window opens on the first turn-open, and it opens
			// inside Wait, which the game screen calls every frame. A unit
			// test that never waits closes a fight that no pace row saw.
			f.c.Wait(0.1)

			require.NoError(t, f.c.Commit(CommitStrike, "e:1"))
		}
	}

	require.False(t, f.c.Fighting(), "the fight must have ended for this to assert anything")
	assert.Empty(t, f.c.Encounter(), "a fight that is over names nothing")
	assert.Zero(t, f.c.Round(), "and its round is gone with it")

	// THE PAIR. The pace row still remembers the fight, which is exactly why
	// the wish note must not read the pace row.
	assert.Equal(t, live, f.c.LastPace().Encounter,
		"LastPace keeps naming the finished fight -- the source row 9 breaks it with")
}
