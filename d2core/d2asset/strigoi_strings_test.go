package d2asset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2tbl"
)

func TestParseStringTable(t *testing.T) {
	table, err := parseStringTable([]byte(`{"strOk":"Well enough","StrHelp16a":""}`), "/s.json")
	require.NoError(t, err)
	assert.Equal(t, "Well enough", table["strOk"])
	assert.Contains(t, table, "StrHelp16a", "an empty text is allowed")

	for name, c := range map[string]struct{ table, says string }{
		"a list":    {`["a"]`, "not a JSON object"},
		"twice":     {`{"a":"b","a":"c"}`, "given twice"},
		"trailing":  {`{"a":"b"} {"c":"d"}`, "after"},
		"empty":     {`{}`, "empty"},
		"empty key": {`{"":"x"}`, "empty key"},
		"not text":  {`{"a":1}`, "not text"},
	} {
		_, err := parseStringTable([]byte(c.table), "/s.json")
		if assert.Error(t, err, name) {
			assert.Contains(t, err.Error(), c.says, name)
		}
	}
}

// With a string table in use, TranslateString answers from it ALONE --
// Diablo II's tables, loaded or not, are gone -- numbered labels included;
// a refused table changes nothing.
func TestAStringTableReplacesDiabloIIs(t *testing.T) {
	dir, err := os.MkdirTemp("", "strings")
	require.NoError(t, err)

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	require.NoError(t, os.WriteFile(filepath.Join(dir, "words.json"), []byte(`{"#971":"Aye","strClose":"Shut it"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{}`), 0o600))

	am := fontAssets(t, dir)
	am.tables = append(am.tables, d2tbl.TextDictionary{"strClose": "Close", "strOnlyD2": "Diablo"})

	require.Error(t, am.UseStringTable("bad.json"))
	assert.Equal(t, "Close", am.TranslateString("strClose"), "a refused table changes nothing")
	assert.Empty(t, am.StringSetPath())

	require.NoError(t, am.UseStringTable("words.json"))
	assert.Equal(t, "/words.json", am.StringSetPath())

	assert.Equal(t, "Shut it", am.TranslateString("strClose"))
	assert.Equal(t, "Aye", am.TranslateString(d2enum.OKLabel), "a numbered label, through BaseLabelNumbers")
	assert.Equal(t, "strOnlyD2", am.TranslateString("strOnlyD2"), "Diablo II's table no longer answers")

	// A non-English install's label modifier does not shift the numbered
	// labels off a table written in one language.
	am.languageModifier = 1
	assert.Equal(t, "Aye", am.TranslateString(d2enum.OKLabel), "numbered labels ignore the language modifier")
}

// The shipped table covers what the UI is known to ask for, and keeps the
// printf verbs the game fills in.
func TestTheShippedStringTable(t *testing.T) {
	am := fontAssets(t, filepath.Join("..", ".."))
	require.NoError(t, am.UseStringTable("data/strigoi/strings/strings.json"))

	for key, verbs := range map[string]string{
		"panelhealth": "%d%d", "panelmana": "%d%d", "panelstamina": "%d%d", "panelexp": "%u%u",
		"StrHelp2": "%s", "StrHelp3": "%s", "StrHelp4": "%s", "StrHelp5": "%s", "StrHelp8a": "%s",
	} {
		text := am.TranslateString(key)
		got := ""

		for i := 0; i+1 < len(text); i++ {
			if text[i] == '%' {
				got += text[i : i+2]
			}
		}

		assert.Equal(t, verbs, got, "%s keeps its verbs: %q", key, text)
	}

	for _, key := range []string{"#1620", "#971", "Amazon", "Warriv", "strchrstr", "minipanelinv"} {
		assert.NotEqual(t, key, am.TranslateString(key), "%s has a text", key)
	}

	assert.False(t, strings.Contains(am.TranslateString("#1625"), "DIABLO"), "the exit button does not say Diablo II")
	assert.Contains(t, am.TranslateString("#1613"), "Blizzard", "while Diablo II's data, art and sound are used, the credit stays")
}
