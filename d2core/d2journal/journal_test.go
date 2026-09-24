package d2journal

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2dialogue"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

// facts is a hand-set world for Evaluate.
type facts struct {
	flags, rungs, states map[string]bool
	date                 string
	night                bool
}

func newFacts() *facts {
	return &facts{flags: map[string]bool{}, rungs: map[string]bool{}, states: map[string]bool{}}
}

func (f *facts) Flag(n string) bool  { return f.flags[n] }
func (f *facts) Rung(n string) bool  { return f.rungs[n] }
func (f *facts) State(n string) bool { return f.states[n] }
func (f *facts) Date() (string, bool) {
	return f.date, f.date != "" && !f.night
}

var testNames = Names{
	Flags:   []string{"met", "promised", "gate", "dug"},
	Rungs:   []string{"water", "trade"},
	Events:  []string{"kept", "broken", "staked", "asked"},
	States:  []string{"hungry"},
	Items:   []string{"arrows"},
	Anchors: []string{"priest"},
}

// minimal is a valid book the refusal cases each break one way.
const minimal = `{
 "parts": [{"id": "days", "title": "Days"}, {"id": "tasks", "title": "Tasks"}, {"id": "self", "title": "Myself"},
  {"id": "writings", "title": "Writings"}],
 "entries": [
  {"id": "start", "part": "self", "title": "Taken", "text": "I was taken.", "when": {"start": true}},
  {"id": "met", "part": "self", "title": "The headman", "text": "An old man.", "when": {"flag": "met"}},
  {"id": "water", "part": "self", "title": "Water", "text": "They let me drink.", "when": {"rung": "water"}},
  {"id": "stake", "part": "self", "title": "The stake", "text": "I staked one.", "when": {"event": "staked"}},
  {"id": "hungry", "part": "self", "title": "Hunger", "text": "Empty.", "when": {"state": "hungry"}},
  {"id": "later", "part": "days", "title": "Later", "text": "The army goes.", "when": {"date": "1462-06-19"}},
  {"id": "both", "part": "self", "title": "Both", "text": "Met and staked.", "when": {"all": [{"flag": "met"}, {"event": "staked"}]}},
  {"id": "arrows", "part": "self", "title": "Arrows", "text": "I have {count:arrows} left.", "when": {"start": true}},
  {"id": "church", "part": "self", "title": "The church", "text": "A wooden church.", "when": {"read": "R02"}, "anchor": "priest"}
 ],
 "tasks": [
  {"id": "watch", "title": "The watch", "repeats": true,
   "open": {"when": {"flag": "promised"}, "text": "I promised."},
   "done": {"when": {"event": "kept"}, "text": "I kept it."},
   "failed": {"when": {"event": "broken"}, "text": "I broke it."}},
  {"id": "ditch", "title": "The ditch",
   "open": {"when": {"event": "asked"}, "text": "Dig it."},
   "done": {"when": {"flag": "dug"}, "text": "Dug."},
   "failed": {"when": {"flag": "gate"}, "text": "Shut out."}},
  {"id": "deed", "title": "A deed", "done": {"when": {"flag": "dug"}, "text": "Done at once."}}
 ],
 "writings": [{"id": "R02", "title": "A loose leaf", "text": "He read me the verses.", "xp": 5}],
 "day_page": {"title": "{weekday}, {day} {month}", "lines": ["The {ramazan} day of Ramazan.", "{event}"],
  "data": "Sunset {sunset} · moonrise {moonrise}, {lit} lit · moonless dark {moonless}",
  "events": {"1462-06-17": "The morning after."}, "fast_words": {"dry": "dry food only"}}
}`

var testDays = []Day{
	{Date: "1462-06-17", Year: 1462, Month: 6, Dom: 17, Weekday: "Thursday", Fast: "dry", Sunset: 1190, Moonrise: 1410, Lit: 69.3, DarkStart: 1275, DarkEnd: 165},
	{Date: "1462-06-18", Year: 1462, Month: 6, Dom: 18, Weekday: "Friday", Sunset: 1190, Moonrise: 1431, Lit: 60.3, DarkStart: 1275, DarkEnd: 165},
	{Date: "1462-06-19", Year: 1462, Month: 6, Dom: 19, Weekday: "Saturday", Sunset: 1190, Moonrise: 11, Lit: 50.8, DarkStart: 1275, DarkEnd: 165},
}

func mustLoad(t *testing.T) (*Book, *Journal) {
	t.Helper()

	b, err := Load([]byte(minimal), testNames)
	if err != nil {
		t.Fatalf("minimal book refused: %v", err)
	}

	return b, New(b, testDays)
}

func TestLoadRefuses(t *testing.T) {
	cases := map[string][2]string{
		"unknown field":            {`"repeats": true,`, `"repeats": true, "colour": "red",`},
		"two kinds":                {`"when": {"flag": "met"}}`, `"when": {"flag": "met", "rung": "water"}}`},
		"no kind":                  {`"when": {"state": "hungry"}}`, `"when": {}}`},
		"unknown flag":             {`{"flag": "met"}}`, `{"flag": "mett"}}`},
		"unknown rung":             {`{"rung": "water"}}`, `{"rung": "wine"}}`},
		"unknown event":            {`"when": {"event": "staked"}}`, `"when": {"event": "stakd"}}`},
		"unknown state":            {`{"state": "hungry"}}`, `{"state": "hungri"}}`},
		"bad date":                 {`"1462-06-19"}}`, `"19 June"}}`},
		"duplicate id":             {`{"id": "deed"`, `{"id": "met"`},
		"empty text":               {`"text": "I was taken."`, `"text": "  "`},
		"long title":               {`"title": "Taken"`, `"title": "Taken far from home as a boy"`},
		"no days part":             {`{"id": "days", "title": "Days"}, `, ``},
		"entry in tasks":           {`"part": "self", "title": "Taken"`, `"part": "tasks", "title": "Taken"`},
		"no such part":             {`"part": "self", "title": "Taken"`, `"part": "selves", "title": "Taken"`},
		"fail without open":        {`{"id": "deed", "title": "A deed", "done"`, `{"id": "deed", "title": "A deed", "failed": {"when": {"flag": "gate"}, "text": "x"}, "done"`},
		"repeats without open":     {`{"id": "deed", "title": "A deed",`, `{"id": "deed", "title": "A deed", "repeats": true,`},
		"start in a task":          {`"done": {"when": {"flag": "dug"}, "text": "Done at once."}`, `"done": {"when": {"start": true}, "text": "Done at once."}`},
		"unknown count":            {`{count:arrows}`, `{count:bolts}`},
		"unknown day field":        {`{moonless}`, `{moonset}`},
		"not start":                {`{"state": "hungry"}}`, `{"not": {"start": true}}}`},
		"bad not":                  {`{"state": "hungry"}}`, `{"not": {"flag": "nope"}}}`},
		"empty all":                {`{"all": [{"flag": "met"}, {"event": "staked"}]}`, `{"all": []}`},
		"unknown read in any":      {`{"all": [{"flag": "met"}, {"event": "staked"}]}`, `{"any": [{"flag": "met"}, {"read": "R99"}]}`},
		"bad id":                   {`{"id": "start"`, `{"id": "Start Here"`},
		"task never appears":       {`{"id": "deed", "title": "A deed", "done": {"when": {"flag": "dug"}, "text": "Done at once."}}`, `{"id": "deed", "title": "A deed"}`},
		"long text":                {`"text": "I was taken."`, `"text": "` + strings.Repeat("word ", 300) + `"`},
		"unknown anchor":           {`"title": "The headman", "text": "An old man.",`, `"title": "The headman", "text": "An old man.", "anchor": "bishop",`},
		"writing twice":            {`"writings": [`, `"writings": [{"id": "R02", "title": "Again", "text": "x", "xp": 1},`},
		"negative xp":              {`"xp": 5`, `"xp": -5`},
		"writing title too long":   {`"title": "A loose leaf"`, `"title": "A loose leaf from the Book of Hours"`},
		"writing text too long":    {`"text": "He read me the verses."`, `"text": "` + strings.Repeat("word ", 300) + `"`},
		"writing's entry collides": {`{"id": "start", "part": "self"`, `{"id": "w_r02", "part": "self"`},
		"writings with no part": {`,
  {"id": "writings", "title": "Writings"}]`, `]`},
		"long place text":       {`"text": "A wooden church."`, `"text": "` + strings.Repeat("word ", 200) + `"`},
		"read of no writing":    {`{"read": "R02"}`, `{"read": "R09"}`},
		"day page without data": {`"data": "Sunset {sunset} · moonrise {moonrise}, {lit} lit · moonless dark {moonless}"`, `"data": ""`},
	}

	for name, c := range cases {
		if !strings.Contains(minimal, c[0]) {
			t.Fatalf("%s: the mutation's anchor %q is not in the minimal book", name, c[0])
		}

		broken := strings.Replace(minimal, c[0], c[1], 1)
		if _, err := Load([]byte(broken), testNames); err == nil {
			t.Errorf("%s: loaded; want a refusal", name)
		}
	}
}

func TestEvaluateWritesWhatHolds(t *testing.T) {
	_, j := mustLoad(t)
	f := newFacts()

	w := j.Evaluate(f)
	if got := strings.Join(w.Entries, ","); got != "start,arrows" {
		t.Fatalf("first evaluate wrote %q; want the start entries", got)
	}

	if len(w.Pages) != 0 {
		t.Fatalf("a page with no date: %v", w.Pages)
	}

	// Nothing new holds: nothing new is written.
	if w := j.Evaluate(f); w.Any() {
		t.Fatalf("a second evaluate wrote %+v", w)
	}

	f.flags["met"] = true
	if w := j.Evaluate(f); strings.Join(w.Entries, ",") != "met" {
		t.Fatalf("the flag wrote %v", w.Entries)
	}

	// A rung reached and lost keeps its line.
	f.rungs["water"] = true
	j.Evaluate(f)
	f.rungs["water"] = false
	j.Evaluate(f)

	if !written(j, "self", "water") {
		t.Fatal("the rung's line was lost with the rung")
	}

	// An undeclared event records nothing.
	if j.Note("stakd") {
		t.Fatal("an undeclared event was noted")
	}

	if !j.Note("staked") {
		t.Fatal("a declared event was refused")
	}

	if w := j.Evaluate(f); strings.Join(w.Entries, ",") != "stake,both" {
		t.Fatalf("the event wrote %v; want stake and the all", w.Entries)
	}

	f.states["hungry"] = true
	j.Evaluate(f)
	f.states["hungry"] = false
	j.Evaluate(f)

	if !written(j, "self", "hungry") {
		t.Fatal("a state edge did not latch")
	}
}

func TestNot(t *testing.T) {
	b, err := Load([]byte(strings.Replace(minimal, `"when": {"state": "hungry"}}`,
		`"when": {"all": [{"state": "hungry"}, {"not": {"flag": "gate"}}]}}`, 1)), testNames)
	if err != nil {
		t.Fatal(err)
	}

	j, f := New(b, testDays), newFacts()
	f.flags["gate"], f.states["hungry"] = true, true
	j.Evaluate(f)

	if written(j, "self", "hungry") {
		t.Fatal("a not held while its flag was set")
	}

	f.flags["gate"] = false
	j.Evaluate(f)

	if !written(j, "self", "hungry") {
		t.Fatal("a not failed with its flag clear")
	}
}

func TestWritingsAndPlaces(t *testing.T) {
	b, j := mustLoad(t)
	f := newFacts()
	j.Evaluate(f)

	if b.Writing("R02") == nil || b.Entry(WritingEntry("R02")) == nil {
		t.Fatal("a writing is in the table, with its entry")
	}

	if j.Read("R09") {
		t.Fatal("a writing the table does not hold was read")
	}

	if !j.Read("R02") {
		t.Fatal("the first read is not first")
	}

	if j.Read("R02") {
		t.Fatal("a second read counted as first")
	}

	w := j.Evaluate(f)
	if strings.Join(w.Entries, ",") != "church,w_r02" && strings.Join(w.Entries, ",") != "w_r02,church" {
		t.Fatalf("reading wrote %v; want the writing's entry and the place it reveals", w.Entries)
	}

	where := func(dx, dy float64) string {
		for _, v := range j.View("self", Lookups{Where: func(string) (float64, float64, bool) { return dx, dy, true }}) {
			if v.ID == "church" {
				return v.Text
			}
		}

		return ""
	}

	for _, c := range []struct {
		dx, dy float64
		want   string
	}{
		{0, -10, "north of me, about 10 paces"},
		{10, 0, "east of me"},
		{7, 7, "south-east of me, about 10 paces"},
		{-7, -7, "north-west"},
		{1, 1, "here, a few paces"},
	} {
		if got := where(c.dx, c.dy); !strings.Contains(got, c.want) {
			t.Errorf("a place at %v,%v reads %q; want %q", c.dx, c.dy, got, c.want)
		}
	}

	if got := where(0, 0); !strings.HasPrefix(got, "A wooden church.") {
		t.Errorf("the place keeps its own text first: %q", got)
	}
}

func TestDayPagesAndDates(t *testing.T) {
	_, j := mustLoad(t)
	f := newFacts()

	// The deep night writes no page, even on a slice date.
	f.date, f.night = "1462-06-17", true
	if w := j.Evaluate(f); len(w.Pages) != 0 {
		t.Fatalf("a page written in the dark: %v", w.Pages)
	}

	f.night = false
	if w := j.Evaluate(f); strings.Join(w.Pages, ",") != "1462-06-17" {
		t.Fatalf("pages %v; want the 17th", w.Pages)
	}

	// A date outside the slice writes nothing.
	f.date = "1462-07-01"
	if w := j.Evaluate(f); len(w.Pages) != 0 {
		t.Fatalf("a page outside the slice: %v", w.Pages)
	}

	// The 19th's entry waits for the 19th.
	f.date = "1462-06-18"
	j.Evaluate(f)

	if written(j, "days", "later") {
		t.Fatal("a dated entry written a day early")
	}

	f.date = "1462-06-19"
	j.Evaluate(f)

	if !written(j, "days", "later") {
		t.Fatal("a dated entry not written on its day")
	}

	// A new session opens on the 17th again (the clock is not saved): the
	// pages written stay, none twice.
	f.date = "1462-06-17"
	if w := j.Evaluate(f); len(w.Pages) != 0 {
		t.Fatalf("a page written twice: %v", w.Pages)
	}

	// The page heads what its own frame wrote: newest first.
	rows := j.View(PartDays, Lookups{})
	if len(rows) != 4 || rows[0].ID != "page:1462-06-19" || rows[1].ID != "later" {
		t.Fatalf("days part %+v", rows)
	}
}

func TestTaskMoves(t *testing.T) {
	_, j := mustLoad(t)
	f := newFacts()

	state := func(id string) string {
		if ts := j.st.Tasks[id]; ts != nil {
			return ts.State
		}

		return ""
	}

	j.Evaluate(f)

	if state("watch") != "" {
		t.Fatal("a task shown before any stage fired")
	}

	// The watch: promised, kept, promised again, broken.
	f.flags["promised"] = true
	j.Evaluate(f)

	if state("watch") != TaskOpen {
		t.Fatalf("promised: %q", state("watch"))
	}

	f.flags["promised"] = false
	j.Note("kept")
	j.Evaluate(f)

	if state("watch") != TaskDone {
		t.Fatalf("kept: %q", state("watch"))
	}

	f.flags["promised"] = true
	j.Evaluate(f)

	if state("watch") != TaskOpen {
		t.Fatalf("promised again: %q; a repeating task reopens", state("watch"))
	}

	f.flags["promised"] = false
	j.Note("broken")
	j.Evaluate(f)

	if state("watch") != TaskFailed {
		t.Fatalf("broken: %q", state("watch"))
	}

	// A kept event while failed does not overturn it: only a new promise
	// reopens the watch.
	j.Note("kept")
	j.Evaluate(f)

	if state("watch") != TaskFailed {
		t.Fatalf("kept while failed: %q", state("watch"))
	}

	// The ditch: asked, dug, and the gate closing after does not fail it.
	j.Note("asked")
	j.Evaluate(f)

	if state("ditch") != TaskOpen {
		t.Fatalf("asked: %q", state("ditch"))
	}

	f.flags["dug"] = true
	j.Evaluate(f)

	if state("ditch") != TaskDone || state("deed") != TaskDone {
		t.Fatalf("dug: ditch %q, deed %q", state("ditch"), state("deed"))
	}

	f.flags["gate"] = true
	j.Evaluate(f)

	if state("ditch") != TaskDone {
		t.Fatalf("the gate failed a task already done: %q", state("ditch"))
	}

	// Asked again (the node reached once more): not a repeating task.
	j.Note("asked")
	j.Evaluate(f)

	if state("ditch") != TaskDone {
		t.Fatalf("asked again reopened a done task: %q", state("ditch"))
	}
}

func TestTaskFailsFromOpen(t *testing.T) {
	_, j := mustLoad(t)
	f := newFacts()

	j.Note("asked")
	j.Evaluate(f)

	f.flags["gate"] = true
	j.Evaluate(f)

	if s := j.st.Tasks["ditch"].State; s != TaskFailed {
		t.Fatalf("gate closed on an open ditch: %q", s)
	}

	rows := j.View(PartTasks, Lookups{})
	if len(rows) != 1 || rows[0].Mark != TaskFailed || rows[0].Text != "Shut out." {
		t.Fatalf("tasks part %+v", rows)
	}
}

func TestSaveRestore(t *testing.T) {
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

	k := New(b, testDays)
	if err := k.Restore(raw); err != nil {
		t.Fatal(err)
	}

	// Restored, nothing is written again.
	if w := k.Evaluate(f); w.Any() {
		t.Fatalf("a restored journal rewrote %+v", w)
	}

	if k.Events("staked") != 1 || k.Unread("self") != 0 {
		t.Fatalf("restored events %d, unread %d", k.Events("staked"), k.Unread("self"))
	}

	// A block that cannot be read: an error, and an empty journal that works.
	if err := k.Restore(json.RawMessage(`{"written": 7}`)); err == nil {
		t.Fatal("a bad block restored")
	}

	if w := k.Evaluate(f); !w.Any() {
		t.Fatal("an emptied journal wrote nothing")
	}

	// A block with maps missing is repaired, not a panic.
	if err := k.Restore(json.RawMessage(`{"seq": 3}`)); err != nil {
		t.Fatal(err)
	}

	k.Note("staked")
	k.Evaluate(f)
}

func TestUnreadAndLeave(t *testing.T) {
	_, j := mustLoad(t)
	f := newFacts()
	j.Evaluate(f)

	if j.Unread("self") != 2 {
		t.Fatalf("unread %d; want the two start entries", j.Unread("self"))
	}

	j.Leave("self")

	if j.Unread("self") != 0 {
		t.Fatal("leaving a part did not mark it read")
	}

	f.flags["met"] = true
	j.Evaluate(f)

	rows := j.View("self", Lookups{Count: func(string) int { return 12 }})
	if !rows[0].Unread || rows[0].ID != "met" {
		t.Fatalf("newest first and unread: %+v", rows[0])
	}

	for _, r := range rows {
		if r.ID == "arrows" && r.Text != "I have 12 left." {
			t.Fatalf("count not filled: %q", r.Text)
		}
	}
}

func TestRamazan(t *testing.T) {
	for _, c := range []struct{ m, d, want int }{{5, 30, 0}, {5, 31, 1}, {6, 17, 18}, {6, 22, 23}, {6, 23, 24}, {6, 29, 30}, {6, 30, 0}} {
		if got := RamazanDay(1462, c.m, c.d); got != c.want {
			t.Errorf("RamazanDay(1462-%02d-%02d) = %d; want %d", c.m, c.d, got, c.want)
		}
	}

	for n, want := range map[int]string{1: "first", 18: "eighteenth", 20: "twentieth", 21: "twenty-first", 24: "twenty-fourth", 30: "thirtieth"} {
		if got := Ordinal(n); got != want {
			t.Errorf("Ordinal(%d) = %q; want %q", n, got, want)
		}
	}
}

func TestDayPageRender(t *testing.T) {
	b, _ := mustLoad(t)

	title, text := b.DayPage.Render(testDays[0])
	if title != "Thursday, 17 June" {
		t.Fatalf("title %q", title)
	}

	for _, want := range []string{"The eighteenth day of Ramazan.", "The morning after.", "Sunset 19:50", "moonrise 23:30, 69% lit", "moonless dark 2.2 h"} {
		if !strings.Contains(text, want) {
			t.Errorf("page lacks %q:\n%s", want, text)
		}
	}

	// No event on the 18th: the event line is left out, not left blank.
	_, text = b.DayPage.Render(testDays[1])
	if strings.Contains(text, "\n\n\n") || strings.HasPrefix(strings.Split(text, "\n")[1], " ") {
		t.Errorf("an empty event line:\n%q", text)
	}

	// A moon that rises after the dark ends: the whole dark is moonless.
	late := testDays[0]
	late.Moonrise = 200 // 03:20

	if _, text := b.DayPage.Render(late); !strings.Contains(text, "moonless dark 5.5 h") {
		t.Errorf("a moon after the dark:\n%s", text)
	}

	// A moonrise after midnight is counted on into the next day.
	_, text = b.DayPage.Render(testDays[2])
	if !strings.Contains(text, "moonrise 00:11") || !strings.Contains(text, "moonless dark 2.9 h") {
		t.Errorf("after-midnight moonrise:\n%s", text)
	}
}

func TestWrapCountsRunes(t *testing.T) {
	lines := Wrap("kılıç kılıç kılıç", 11)
	if len(lines) != 2 || lines[0] != "kılıç kılıç" {
		t.Fatalf("wrap %q; ş and ı are one character each", lines)
	}

	if got := Wrap("one\n\ntwo", 10); len(got) != 3 || got[1] != "" {
		t.Fatalf("paragraphs %q", got)
	}
}

// TestShippedJournal loads data/strigoi/journal.json against the game's real
// names -- the same lists the game screen builds -- and checks every slice day
// renders in his words.
func TestShippedJournal(t *testing.T) {
	b, names := shippedBook(t)

	if len(b.Entries) < 80 || len(b.Tasks) < 6 {
		t.Fatalf("%d entries, %d tasks: the shipped journal is thinner than J1's Tier 1", len(b.Entries), len(b.Tasks))
	}

	// Every event the book reads is one the game declares (Load checks it);
	// and no entry mentions the dropped keepsake (ruled 24 Sep).
	cross := regexp.MustCompile(`(?i)\bcross(es)?\b`)
	for _, e := range b.Entries {
		text := strings.NewReplacer("sign of the cross", "", "in a cross", "", "wooden crosses", "").Replace(e.Text) // grave-markers, not the dropped keepsake
		if cross.MatchString(text) {
			t.Errorf("entry %s mentions a cross: %q", e.ID, e.Text)
		}
	}

	days := gameDays()
	if len(days) != 7 {
		t.Fatalf("%d slice days", len(days))
	}

	for _, d := range days {
		if _, ok := b.DayPage.FastWords[d.Fast]; !ok {
			t.Errorf("%s: the fast rule %q has no words of his", d.Date, d.Fast)
		}

		if _, ok := b.DayPage.SaintWords[d.Saint]; !ok {
			t.Errorf("%s: the commemoration %q has no words of his", d.Date, d.Saint)
		}

		title, text := b.DayPage.Render(d)
		if title == "" || strings.Contains(text, "{") || !strings.Contains(text, "day of Ramazan") {
			t.Errorf("%s renders %q / %q", d.Date, title, text)
		}

		if n := len(Wrap(text, WrapWidth)); n > MaxLines {
			t.Errorf("%s page wraps to %d lines", d.Date, n)
		}
	}

	_ = names
}

// shippedBook loads the shipped table with the names the game builds.
func shippedBook(t *testing.T) (*Book, Names) {
	t.Helper()

	read := func(p string) []byte {
		data, err := os.ReadFile("../../data/strigoi/" + p)
		if err != nil {
			t.Fatal(err)
		}

		return data
	}

	dialogue, err := d2dialogue.Load(read("dialogue.json"))
	if err != nil {
		t.Fatal(err)
	}

	var items struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}

	var recipes struct {
		Recipes []struct {
			ID string `json:"id"`
		} `json:"recipes"`
	}

	if err := json.Unmarshal(read("items.json"), &items); err != nil {
		t.Fatal(err)
	}

	if err := json.Unmarshal(read("recipes.json"), &recipes); err != nil {
		t.Fatal(err)
	}

	var itemIDs, recipeIDs []string
	for _, it := range items.Items {
		itemIDs = append(itemIDs, it.ID)
	}

	for _, r := range recipes.Recipes {
		recipeIDs = append(recipeIDs, r.ID)
	}

	names := Names{
		Flags:   append(dialogue.FlagsNamed(), "seen_staking"),
		Rungs:   dialogue.RungIDs(),
		Anchors: dialogue.SpeakerIDs(),
		States:  GameStates(),
		Items:   itemIDs,
		Events: GameEvents(Vocabulary{
			Rows: gameRows(), Recipes: recipeIDs, Speakers: dialogue.SpeakerIDs(), Nodes: dialogue.NodeIDs(),
		}),
	}

	b, err := Load(read("journal.json"), names)
	if err != nil {
		t.Fatalf("the shipped journal is refused: %v", err)
	}

	return b, names
}

// gameRows and gameDays are what the game screen passes, from d2world; the
// conversion is the game screen's (journalDays), repeated here so this
// package stays a leaf.
func gameRows() []string {
	var rows []string
	for _, r := range d2world.DefaultSpawnDials().Rows {
		rows = append(rows, r.Name)
	}

	return append(rows, d2world.RisenRow)
}

func gameDays() []Day {
	dials := d2world.DefaultClockDials()

	var out []Day

	for _, e := range d2world.SliceDays() {
		out = append(out, Day{
			Date: fmt.Sprintf("%04d-%02d-%02d", e.Year, e.Month, e.Day), Year: e.Year, Month: e.Month, Dom: e.Day,
			Weekday: e.Weekday, Feast: e.Feast, Saint: e.FeastDetail, Fast: e.FastRule,
			Sunset: e.SunsetMinute, Moonrise: e.MoonriseMinute, Lit: e.MoonLitPercent,
			DarkStart: int(dials.NightStart), DarkEnd: int(dials.DawnStart),
		})
	}

	return out
}

func written(j *Journal, part, id string) bool {
	for _, v := range j.View(part, Lookups{}) {
		if v.ID == id {
			return true
		}
	}

	return false
}
