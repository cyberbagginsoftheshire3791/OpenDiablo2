package d2server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNextGameSeedIsOneShot(t *testing.T) {
	SetNextGameSeed(1462)
	assert.Equal(t, int64(1462), takeNextGameSeed(), "the set seed is consumed")

	second := takeNextGameSeed()
	assert.NotEqual(t, int64(1462), second, "the override is one-shot")
	assert.NotZero(t, second, "with no override the wall-clock default applies")

	SetNextGameSeed(7)
	SetNextGameSeed(0) // clearing
	assert.NotEqual(t, int64(7), takeNextGameSeed(), "0 clears the override")
}
