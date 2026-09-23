package d2items

import (
	"encoding/json"
	"testing"
)

func mustJSON(t *testing.T, v interface{}) []byte {
	t.Helper()

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func mustUnJSON(t *testing.T, data []byte, v interface{}) {
	t.Helper()

	if err := json.Unmarshal(data, v); err != nil {
		t.Fatal(err)
	}
}
