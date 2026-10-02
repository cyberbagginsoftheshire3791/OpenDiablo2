package d2journal

import (
	"encoding/json"
	"testing"
)

// M4.6 B6 review, BUG-117: THE PROVIDER REPORTS EVERYTHING SAVE WRITES. Two
// journals restored from blocks that differ in one saved field -- the order
// two entries were written in, a page's number, a rung, a task's number or
// edge memory, where a part was last read -- report differently. Before it
// the provider reported which ids were written and the tasks' states, and a
// resume that lost the rest compared equal. THE CONTROL: the same block twice
// reports the same.
func TestTheProviderReportsWhatSaveWrites(t *testing.T) {
	b, j := mustLoad(t)
	f := newFacts()
	f.flags["met"], f.date = true, "1462-06-17"
	j.Note("staked")
	j.Evaluate(f)
	j.Leave("self")

	raw, err := j.Save()
	if err != nil {
		t.Fatal(err)
	}

	report := func(block []byte) string {
		k := New(b, testDays)
		if err := k.Restore(block); err != nil {
			t.Fatal(err)
		}

		out, err := json.Marshal(k.HarnessState())
		if err != nil {
			t.Fatal(err)
		}

		return string(out)
	}

	base := report(raw)
	if again := report(raw); again != base {
		t.Fatalf("the control: one block, two reports\n%s\n%s", base, again)
	}

	edit := func(change func(st *state)) []byte {
		var st state
		if err := json.Unmarshal(raw, &st); err != nil {
			t.Fatal(err)
		}

		change(&st)

		out, err := json.Marshal(st)
		if err != nil {
			t.Fatal(err)
		}

		return out
	}

	if len(j.st.Written) < 2 || len(j.st.Pages) == 0 || len(j.st.Tasks) == 0 {
		t.Fatalf("the premise: two entries, a page and a task written: %+v", j.st)
	}

	for name, change := range map[string]func(st *state){
		"the written order": func(st *state) {
			var a, b string
			for id := range st.Written {
				if a == "" {
					a = id
				} else if b == "" {
					b = id
				}
			}
			st.Written[a], st.Written[b] = st.Written[b], st.Written[a]
		},
		"a page's number": func(st *state) {
			for d := range st.Pages {
				st.Pages[d]++
			}
		},
		"a rung": func(st *state) { st.Rungs["a-rung-never-reached"] = true },
		"a task's number": func(st *state) {
			for _, ts := range st.Tasks {
				ts.Seq += 7
			}
		},
		"a task's edge memory": func(st *state) {
			for _, ts := range st.Tasks {
				ts.Mem[0].Held = !ts.Mem[0].Held
				ts.Mem[0].Events += 3
			}
		},
		"where a part was read to": func(st *state) { st.Seen["self"] += 5 },
	} {
		if got := report(edit(change)); got == base {
			t.Errorf("%s: a journal restored with it changed reports the same", name)
		}
	}

}
