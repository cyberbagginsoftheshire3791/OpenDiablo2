package d2asset

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	slashpath "path"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2tbl"
)

// STRIGOI'S OWN WORDS (M5.3, the census's data/local/lng row). Diablo II's
// three string tables are read whole from the MPQs for the few hundred keys
// the UI asks for (string_census.go lists them). A STRING TABLE of Strigoi's
// own -- a JSON object, key to text, written for this game -- replaces all
// three: with one in use, Diablo II's are never loaded, and TranslateString
// answers from it alone. A key it lacks reads as the key itself, as a key
// missing from Diablo II's always did; the string census counts them
// (strings_missing), which is how a gap is found.

// UseStringTable makes TranslateString answer from the table at p
// (game-relative, e.g. data/strigoi/strings/strings.json) instead of Diablo
// II's. It replaces any table already loaded. A refused table changes
// nothing. It is for initialisation only (d2app loadStrings): the tables
// are read without a lock by every goroutine that translates, the game
// server's included, once the game is running.
//
// While it is in use, numbered labels (#1620 ...) are looked up with no
// language modifier: the table is written in one language, and a
// non-English Diablo II install's modifier would shift every #nnnn key off
// it.
func (am *AssetManager) UseStringTable(p string) error {
	p = strings.TrimSpace(strings.ReplaceAll(p, `\`, "/"))
	if p == "" {
		return errors.New("no string table named")
	}

	p = slashpath.Clean("/" + p)

	data, err := am.LoadFile(p)
	if err != nil {
		return fmt.Errorf("loading string table %s: %w", p, err)
	}

	table, err := parseStringTable(data, p)
	if err != nil {
		return err
	}

	am.tables = []d2tbl.TextDictionary{table}
	am.stringSetPath = p

	return nil
}

// StringSetPath is Strigoi's string table in use, or "" for Diablo II's.
func (am *AssetManager) StringSetPath() string { return am.stringSetPath }

// parseStringTable reads a key-to-text JSON object, refusing anything but
// one: a list, a number, a key given twice (a map would keep the last one
// silently), trailing data, an empty table, an empty key. A text may be
// empty -- some labels are deliberately blank.
func parseStringTable(data []byte, tablePath string) (d2tbl.TextDictionary, error) {
	table := map[string]string{}
	bad := func(what string) error { return fmt.Errorf("string table %s: %s", tablePath, what) }

	dec := json.NewDecoder(strings.NewReader(string(data)))

	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, bad("is not a JSON object of key to text")
	}

	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, bad(err.Error())
		}

		key := tok.(string) // an object's keys are always strings

		var text string
		if err := dec.Decode(&text); err != nil {
			return nil, bad(fmt.Sprintf("%q is not text: %v", key, err))
		}

		if _, twice := table[key]; twice {
			return nil, bad(fmt.Sprintf("%q is given twice", key))
		}

		table[key] = text
	}

	if _, err := dec.Token(); err != nil {
		return nil, bad(err.Error())
	}

	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("string table %s has something after its closing brace", tablePath)
	}

	if len(table) == 0 {
		return nil, fmt.Errorf("string table %s is empty", tablePath)
	}

	if _, blank := table[""]; blank {
		return nil, fmt.Errorf("string table %s has an empty key", tablePath)
	}

	return d2tbl.TextDictionary(table), nil
}
