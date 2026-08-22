package d2txt

import (
	"testing"

	testify "github.com/stretchr/testify/assert"
)

// All inputs are built in-code (tab-separated text) so the test carries no
// Blizzard-derived bytes (Constitution, Article V).

func TestDataDictionary_LoadAndIterate(t *testing.T) {
	assert := testify.New(t)

	data := []byte(
		"Name\tCode\tFlag\tItems\n" +
			"alpha\t10\t1\ta,b,c\n" +
			"Expansion\t0\t0\t\n" + // skipped: first column is "Expansion"
			"beta\t20\t0\tx\n")

	dd := LoadDataDictionary(data)

	// first data row
	assert.True(dd.Next(), "expected a first row")
	assert.Equal("alpha", dd.String("Name"))
	assert.Equal(10, dd.Number("Code"))
	assert.True(dd.Bool("Flag"))
	assert.Equal([]string{"a", "b", "c"}, dd.List("Items"))

	// the "Expansion" row is transparently skipped, so the next row is beta
	assert.True(dd.Next(), "expected the beta row after the skipped Expansion row")
	assert.Equal("beta", dd.String("Name"))
	assert.Equal(20, dd.Number("Code"))
	assert.False(dd.Bool("Flag"))
	assert.Equal([]string{"x"}, dd.List("Items"))

	// end of data
	assert.False(dd.Next(), "expected no further rows")
	assert.NoError(dd.Err)
}

func TestDataDictionary_Gotchas(t *testing.T) {
	assert := testify.New(t)

	dd := LoadDataDictionary([]byte("Name\tVal\nrow1\t5\n"))
	assert.True(dd.Next())

	// An unknown column name maps to 0 in the lookup, so String silently
	// returns column 0 rather than erroring (documented d2-formats gotcha).
	assert.Equal("row1", dd.String("NoSuchColumn"))

	// Number returns 0 when the cell will not parse as an int.
	assert.Equal(0, dd.Number("Name"))

	// Bool panics on any value greater than 1.
	assert.Panics(func() { dd.Bool("Val") })
}
