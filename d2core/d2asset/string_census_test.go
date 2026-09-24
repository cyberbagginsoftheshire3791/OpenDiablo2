package d2asset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2tbl"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
)

// Every key the UI asks for is on record: once per key, whether a table had
// it, and what it became.
func TestTheStringCensusRecordsEveryAsk(t *testing.T) {
	am, err := NewAssetManager(d2util.LogLevelError)
	require.NoError(t, err)

	am.tables = append(am.tables, d2tbl.TextDictionary{"strOk": "Well enough"})

	assert.Equal(t, "Well enough", am.TranslateString("strOk"))
	assert.Equal(t, "Well enough", am.TranslateString("strOk"))
	assert.Equal(t, "strNope", am.TranslateString("strNope"), "a missing key reads as itself, as before")

	asked := am.StringsAsked()
	require.Len(t, asked, 2)

	assert.Equal(t, StringAsk{Key: "strNope", Found: false, Text: ""}, asked[0])
	assert.Equal(t, StringAsk{Key: "strOk", Found: true, Text: "Well enough"}, asked[1])
}

// Asking again for a key already on record changes nothing the harness can
// see. The HUD asks for its tooltips every frame it draws, and how many
// frames a launch draws while loading is the wall clock's business: a
// census that counted asks made two launches of one seeded run digest
// differently (docs/harness.md, leak register #2).
func TestAskingAgainLeavesTheCensusAlone(t *testing.T) {
	am, err := NewAssetManager(d2util.LogLevelError)
	require.NoError(t, err)

	am.tables = append(am.tables, d2tbl.TextDictionary{"panelexp": "Experience"})

	am.TranslateString("panelexp")
	once := am.StringsAsked()

	for i := 0; i < 5; i++ {
		am.TranslateString("panelexp")
	}

	assert.Equal(t, once, am.StringsAsked(), "a key asked again, frame after frame, must not change the census")
}
