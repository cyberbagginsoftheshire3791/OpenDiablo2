package d2dialogue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func shipped(t *testing.T) *Book {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "data", "strigoi", "dialogue.json"))
	if err != nil {
		t.Fatal(err)
	}

	b, err := Load(data)
	if err != nil {
		t.Fatalf("the shipped dialogue must load: %v", err)
	}

	return b
}

func TestTheShippedLadderIsTheSignedOne(t *testing.T) {
	b := shipped(t)

	var ids []string
	for _, r := range b.Village.Ladder {
		ids = append(ids, r.ID)
	}

	// S1 §8.2's order, exactly.
	if got := strings.Join(ids, ">"); got != "water>trade>shelter>hearth>rite" {
		t.Fatalf("the ladder is water, trade, shelter, hearth, rite; got %s", got)
	}

	st := b.NewStanding()
	if b.Reached(st, "water") {
		t.Fatal("a stranger starts below the first rung: low-wary")
	}
}

func TestFloorAndCeiling(t *testing.T) {
	b := shipped(t)
	st := b.NewStanding()

	b.Move(st, -1000)
	if st.Rep != b.Village.Floor {
		t.Fatalf("the fear floor holds: %d", st.Rep)
	}

	b.Move(st, 1000)
	if st.Rep != b.Village.Ceiling {
		t.Fatalf("the Occupier's Shadow ceiling holds: %d", st.Rep)
	}

	if !b.Reached(st, "rite") {
		t.Fatal("the last rung is reachable at the ceiling")
	}
}

func TestLoadRefuses(t *testing.T) {
	good := `{"village":{"start":5,"floor":0,"ceiling":20,"watch":1,"ladder":[{"id":"a","name":"A","at":10}]},
	"speakers":[{"id":"s","role":"S","stand_in":"X","openings":[{"node":"n"}]}],
	"nodes":{"n":{"text":"hi","choices":[{"text":"bye"}]}}}`

	if _, err := Load([]byte(good)); err != nil {
		t.Fatalf("THE CONTROL: a well-formed table loads: %v", err)
	}

	bad := map[string]string{
		"a choice to a missing node":   strings.Replace(good, `{"text":"bye"}`, `{"text":"bye","next":"nowhere"}`, 1),
		"an unknown rung":              strings.Replace(good, `{"text":"bye"}`, `{"text":"bye","requires":{"rung":"z"}}`, 1),
		"a rung above the ceiling":     strings.Replace(good, `"at":10`, `"at":30`, 1),
		"a gated last opening":         strings.Replace(good, `{"node":"n"}`, `{"requires":{"flag":"f"},"node":"n"}`, 1),
		"a last opening gated by time": strings.Replace(good, `{"node":"n"}`, `{"requires":{"time":"day"},"node":"n"}`, 1),
		"a last opening gated by not_flags": strings.Replace(good, `{"node":"n"}`,
			`{"requires":{"not_flags":["watch_promised"]},"node":"n"}`, 1),
		"an unknown field":            strings.Replace(good, `"text":"hi"`, `"text":"hi","mood":"sad"`, 1),
		"a bad time":                  strings.Replace(good, `{"text":"bye"}`, `{"text":"bye","requires":{"time":"dusk"}}`, 1),
		"an opening to no node":       strings.Replace(good, `{"node":"n"}`, `{"node":"m"}`, 1),
		"floor above start":           strings.Replace(good, `"floor":0`, `"floor":6`, 1),
		"a ladder that does not rise": strings.Replace(good, `[{"id":"a","name":"A","at":10}]`, `[{"id":"a","name":"A","at":10},{"id":"b","name":"B","at":10}]`, 1),
		"a duplicate rung":            strings.Replace(good, `[{"id":"a","name":"A","at":10}]`, `[{"id":"a","name":"A","at":10},{"id":"a","name":"B","at":12}]`, 1),
		"a speaker with no openings":  strings.Replace(good, `"openings":[{"node":"n"}]`, `"openings":[]`, 1),
		"a duplicate stand-in": strings.Replace(good, `"speakers":[`,
			`"speakers":[{"id":"t","role":"T","stand_in":"X","openings":[{"node":"n"}]},`, 1),
		"a duplicate speaker id": strings.Replace(good, `"speakers":[`,
			`"speakers":[{"id":"s","role":"T","stand_in":"Y","openings":[{"node":"n"}]},`, 1),
		"an empty node id": strings.Replace(good, `"nodes":{`, `"nodes":{"":{"text":"x","choices":[]},`, 1),
		"too many answers": strings.Replace(good, `[{"text":"bye"}]`,
			`[{"text":"1"},{"text":"2"},{"text":"3"},{"text":"4"},{"text":"5"},{"text":"6"},{"text":"7"}]`, 1),
		"a negative effect":   strings.Replace(good, `{"text":"bye"}`, `{"text":"bye","effects":{"water":-5}}`, 1),
		"a negative watch":    strings.Replace(good, `"watch":1`, `"watch":-1`, 1),
		"a flag nothing sets": strings.Replace(good, `{"text":"bye"}`, `{"text":"bye","requires":{"flag":"typo"}}`, 1),
	}

	// And a flag some choice DOES set is fine -- the mirror of the typo case.
	setAndRead := strings.Replace(good, `{"text":"bye"}`,
		`{"text":"bye","effects":{"set":["seen"]}},{"text":"again","requires":{"flag":"seen"}}`, 1)
	if _, err := Load([]byte(setAndRead)); err != nil {
		t.Fatalf("a flag a choice sets may be required: %v", err)
	}

	for name, doc := range bad {
		if _, err := Load([]byte(doc)); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
}

// A first meeting with the headman, the ditch dug: the number moves, the
// flags are set, and the dug ditch cannot be dug twice for credit.
func TestAConversation(t *testing.T) {
	b := shipped(t)
	st := b.NewStanding()

	talk, err := b.Open(st, "headman", false)
	if err != nil || talk == nil || talk.NodeID != "headman_first" {
		t.Fatalf("a first meeting opens on the first-meeting node: %v %v", talk, err)
	}

	if _, err := talk.Choose(0); err != nil { // "Only to live..."
		t.Fatal(err)
	}

	if talk.NodeID != "headman_work" || st.Rep != b.Village.Start+3 || !st.Has("met_headman") {
		t.Fatalf("the civil answer: node %s, rep %d, flags %v", talk.NodeID, st.Rep, st.Flags)
	}

	e, err := talk.Choose(0) // dig
	if err != nil || e.Minutes != 60 || st.Rep != b.Village.Start+11 {
		t.Fatalf("the ditch costs an hour and earns 8: %+v rep %d %v", e, st.Rep, err)
	}

	talk.Leave()

	// Second time: the headman's plain opening, and the dig is gone.
	talk = mustOpen(t, b, st, "headman", false)
	if talk.NodeID != "headman" {
		t.Fatalf("the second meeting is the plain one: %s", talk.NodeID)
	}

	mustChoose(t, talk, 0) // "Is there work?"

	for _, c := range talk.Answers() {
		if strings.Contains(c.Text, "Dig") {
			t.Fatal("the ditch is dug; it is not offered again")
		}
	}
}

func TestTheftClosesTheGate(t *testing.T) {
	b := shipped(t)
	st := b.NewStanding()
	b.Move(st, 30)

	talk := mustOpen(t, b, st, "headman", false)
	mustChoose(t, talk, 1) // "Your bread."

	e, err := talk.Choose(0) // at sword-point
	if err != nil || e.Food != 50 {
		t.Fatalf("the bread is his: %+v %v", e, err)
	}

	if st.Rep != b.Village.Floor || !st.Has("gate_closed") {
		t.Fatalf("to the floor, and the gate closes: rep %d flags %v", st.Rep, st.Flags)
	}

	for _, sp := range b.Speakers {
		talk := mustOpen(t, b, st, sp.ID, false)
		if talk.NodeID != "closed" || len(talk.Answers()) != 0 {
			t.Fatalf("%s must not speak to him again: %s", sp.ID, talk.NodeID)
		}
	}
}

func TestNightAndTheLadderGateTheOpenings(t *testing.T) {
	b := shipped(t)
	st := b.NewStanding()

	if talk, _ := b.Open(st, "well_woman", false); talk.NodeID != "well_wary" {
		t.Fatalf("below water, the well is guarded: %s", talk.NodeID)
	}

	b.Move(st, 5) // 15: water

	if talk, _ := b.Open(st, "well_woman", false); talk.NodeID != "well" {
		t.Fatalf("at water, the trough: %s", talk.NodeID)
	}

	if talk, _ := b.Open(st, "well_woman", true); talk.NodeID != "night_barred" {
		t.Fatalf("at night, below shelter, the door stays barred: %s", talk.NodeID)
	}

	b.Move(st, 100) // ceiling: shelter and past

	if talk, _ := b.Open(st, "well_woman", true); talk.NodeID != "well" {
		t.Fatalf("with shelter, night is no bar: %s", talk.NodeID)
	}
}

func TestTheWatchIsPaidAtDawnOnce(t *testing.T) {
	b := shipped(t)
	st := b.NewStanding()

	if b.DawnWatch(st) != 0 {
		t.Fatal("THE CONTROL: no promise, nothing paid")
	}

	talk := mustOpen(t, b, st, "headman", false)
	mustChoose(t, talk, 2) // say nothing, and go -- met now
	talk = mustOpen(t, b, st, "headman", false)
	mustChoose(t, talk, 1) // the watch
	mustChoose(t, talk, 0) // promise

	if !st.Has(FlagWatch) {
		t.Fatalf("the promise is kept as a flag: %v", st.Flags)
	}

	before := st.Rep
	if got := b.DawnWatch(st); got != b.Village.Watch || st.Rep != before+got {
		t.Fatalf("the watch pays %d at dawn; paid %d", b.Village.Watch, got)
	}

	if b.DawnWatch(st) != 0 {
		t.Fatal("once per promise")
	}

	// And a promise cannot be made at night -- with shelter reached, so the
	// headman does talk after dark and the refusal is the choice's own.
	b.Move(st, 100)

	talk = mustOpen(t, b, st, "headman", true)
	if talk.NodeID != "headman" || len(talk.Answers()) == 0 {
		t.Fatalf("with shelter he talks at night: %s", talk.NodeID)
	}

	for _, c := range talk.Answers() {
		if strings.Contains(c.Text, "watch") {
			t.Fatal("the watch is promised by day")
		}
	}
}

func mustOpen(t *testing.T, b *Book, st *Standing, id string, night bool) *Talk {
	t.Helper()

	talk, err := b.Open(st, id, night)
	if err != nil {
		t.Fatalf("open %s: %v", id, err)
	}

	return talk
}

func mustChoose(t *testing.T, talk *Talk, i int) Effects {
	t.Helper()

	e, err := talk.Choose(i)
	if err != nil {
		t.Fatalf("choose %d at %s: %v", i+1, talk.NodeID, err)
	}

	return e
}

func TestNormaliseMendsASavedStanding(t *testing.T) {
	b := shipped(t)

	// Hand-edited or older: unsorted, doubled, and above today's ceiling.
	st := &Standing{Rep: b.Village.Ceiling + 40, Flags: []string{"met_headman", "gate_closed", "met_headman"}}

	b.Normalise(st)

	if !st.Has("gate_closed") || !st.Has("met_headman") || len(st.Flags) != 2 {
		t.Fatalf("flags sorted and unique: %v", st.Flags)
	}

	if st.Rep != b.Village.Ceiling {
		t.Fatalf("the number inside the ceiling: %d", st.Rep)
	}

	// THE CONTROL: unmended, the closed gate is not found.
	raw := &Standing{Flags: []string{"met_headman", "gate_closed"}}
	if raw.Has("gate_closed") {
		t.Fatal("control: an unsorted list must defeat the search, or this test proves nothing")
	}
}

func TestATalkFollowsTheClock(t *testing.T) {
	b := shipped(t)
	st := b.NewStanding()
	b.Move(st, 100)

	talk := mustOpen(t, b, st, "headman", false)
	mustChoose(t, talk, 2) // the first meeting: say nothing, and go

	talk = mustOpen(t, b, st, "headman", false)

	watch := func() bool {
		for _, c := range talk.Answers() {
			if strings.Contains(c.Text, "watch") {
				return true
			}
		}

		return false
	}

	if !watch() {
		t.Fatal("by day the watch is offered")
	}

	talk.SetNight(true)

	if watch() {
		t.Fatal("once the clock has turned to night mid-talk, it is not")
	}
}

func TestChooseRefuses(t *testing.T) {
	b := shipped(t)
	st := b.NewStanding()
	talk, _ := b.Open(st, "headman", false)

	if _, err := talk.Choose(99); err != ErrNoChoice {
		t.Fatalf("no such answer: %v", err)
	}

	talk.Leave()

	if _, err := talk.Choose(0); err != ErrOver {
		t.Fatalf("over: %v", err)
	}

	if _, err := b.Open(st, "nobody", false); err != ErrNoSpeaker {
		t.Fatalf("nobody: %v", err)
	}
}

func TestBarterNamesItsGoods(t *testing.T) {
	b := shipped(t)

	if got := strings.Join(b.ItemsNamed(), ","); got != "arrowheads,branches,feathers,wire" {
		t.Fatalf("the village hands over branches, feathers, arrowheads and wire: %s", got)
	}

	if got := strings.Join(b.SlotsNamed(), ","); got != "body" {
		t.Fatalf("the smith mends mail: %s", got)
	}

	st := b.NewStanding()
	b.Move(st, 30) // trade

	talk := mustOpen(t, b, st, "smith", false)

	c, err := talk.Peek(2)
	if err != nil || c.Effects.Give["arrowheads"] != 6 {
		t.Fatalf("peek shows the arrowheads without taking them: %+v %v", c, err)
	}

	if talk.NodeID != "smith" {
		t.Fatal("a peek moves nothing")
	}

	if _, err := talk.Peek(9); err != ErrNoChoice {
		t.Fatalf("peek past the answers: %v", err)
	}
}

func TestShelterIsANightThingAtTheShelterRung(t *testing.T) {
	b := shipped(t)
	st := b.NewStanding()

	talk := mustOpen(t, b, st, "headman", false)
	mustChoose(t, talk, 2) // met

	offered := func(night bool) bool {
		talk := mustOpen(t, b, st, "headman", night)
		for _, c := range talk.Answers() {
			if strings.Contains(c.Text, "sleep inside") {
				return true
			}
		}

		return false
	}

	b.Move(st, 100)

	if !offered(true) {
		t.Fatal("at shelter, by night, he may ask to sleep inside")
	}

	if offered(false) {
		t.Fatal("by day there is nothing to shelter from")
	}

	// Below the rung the door is barred at night anyway, so the CHOICE's own
	// rung requirement is tested directly (review finding: the old check could
	// not fail).
	var ask Choice
	for _, c := range b.Nodes["headman"].Choices {
		if strings.Contains(c.Text, "sleep inside") {
			ask = c
		}
	}

	below := &Standing{Rep: 30, Flags: []string{}}
	if b.Meets(below, ask.Requires, true) {
		t.Fatal("below the shelter rung the choice itself refuses")
	}

	promised := &Standing{Rep: 60, Flags: []string{FlagWatch}}
	if b.Meets(promised, ask.Requires, true) {
		t.Fatal("a promised watch is not slept through")
	}

	b.Move(st, 100)
	talk = mustOpen(t, b, st, "headman", true)

	for i, c := range talk.Answers() {
		if strings.Contains(c.Text, "sleep inside") {
			mustChoose(t, talk, i)
		}
	}

	e := mustChoose(t, talk, 0)
	if e.Rest != 60 || e.Minutes != 240 || !e.Shelter {
		t.Fatalf("four hours' sleep inside: %+v", e)
	}

	// Once a night: not offered again until dawn clears it.
	if offered(true) {
		t.Fatal("one sleep a night")
	}

	b.DawnWatch(st)

	if !offered(true) {
		t.Fatal("dawn gives the byre back")
	}
}
