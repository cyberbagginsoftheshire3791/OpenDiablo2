package d2asset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2tbl"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
)

// Every key the UI asks for is on record: once per key, how often, whether a
// table had it, and what it became.
func TestTheStringCensusRecordsEveryAsk(t *testing.T) {
	am, err := NewAssetManager(d2util.LogLevelError)
	require.NoError(t, err)

	am.tables = append(am.tables, d2tbl.TextDictionary{"strOk": "Well enough"})

	assert.Equal(t, "Well enough", am.TranslateString("strOk"))
	assert.Equal(t, "Well enough", am.TranslateString("strOk"))
	assert.Equal(t, "strNope", am.TranslateString("strNope"), "a missing key reads as itself, as before")

	asked := am.StringsAsked()
	require.Len(t, asked, 2)

	assert.Equal(t, StringAsk{Key: "strNope", Found: false, Text: "", Asks: 1}, asked[0])
	assert.Equal(t, StringAsk{Key: "strOk", Found: true, Text: "Well enough", Asks: 2}, asked[1])
}
