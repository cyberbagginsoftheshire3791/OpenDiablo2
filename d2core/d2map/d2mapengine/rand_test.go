package d2mapengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorldRandDeterministicAndCounted(t *testing.T) {
	a := &MapEngine{}
	a.SetSeed(1462)

	b := &MapEngine{}
	b.SetSeed(1462)

	seqA := make([]int, 0, 16)
	seqB := make([]int, 0, 16)

	for i := 0; i < 16; i++ {
		seqA = append(seqA, a.Rand().Intn(1000))
		seqB = append(seqB, b.Rand().Intn(1000))
	}

	assert.Equal(t, seqA, seqB, "two engines with one seed draw identical sequences")
	assert.Equal(t, a.RandDraws(), b.RandDraws(), "draw counts match")
	assert.NotZero(t, a.RandDraws())

	c := &MapEngine{}
	c.SetSeed(1463)
	diverged := false

	for i := 0; i < 16; i++ {
		if c.Rand().Intn(1000) != seqA[i] {
			diverged = true
		}
	}

	assert.True(t, diverged, "a different seed draws a different sequence")

	a.ReseedRand(1462)
	assert.Zero(t, a.RandDraws(), "reseeding resets the draw count")
	assert.Equal(t, seqA[0], a.Rand().Intn(1000), "reseeding replays the sequence")
}
