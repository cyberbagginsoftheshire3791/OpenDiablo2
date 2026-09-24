// Package d2journal is Strigoi's journal (J1, 24 Sep 2026): his own diary,
// in the first person, which replaces Diablo II's quest log.
//
// WHAT IS SIGNED (Josh, 24 Sep 2026): the journal is his diary; an entry
// appears as he learns it; the only numbers in it are counts, recipes and the
// sky data line under each day page; the keepsake cross is dropped. The words
// are drafts from claude/journal-content-map.md, which carries each entry's
// source and grounding label -- and so does data/strigoi/journal.json, in a
// "sources" field the game never shows.
//
// THE MODEL. The data holds parts (the tabs), entries, tasks and the day-page
// template. An entry has one condition, `when`; once it holds the entry is
// WRITTEN and stays written, whatever happens after (a rung reached and then
// lost keeps its line -- the loss has an entry of its own). A task has up to
// three conditions -- open, done, failed -- and moves between those states on
// the RISING EDGE of each (see Task). A day page is written for each slice
// date he sees by daylight.
//
// Conditions read facts the game owns (flags, rungs, states, the date) and
// events the game raises through Note. Every name a condition uses is checked
// at load against the lists the game passes in (Names), so a misspelt flag is
// a load error, not an entry that never appears.
//
// It is a LEAF package: no ebiten, no world, no logging, no harness. The game
// screen owns the provider, the save and the panel.
package d2journal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// The parts the code itself writes into: day pages go to PartDays, tasks to
// PartTasks. The data may add others; these two must exist.
const (
	PartDays  = "days"
	PartTasks = "tasks"

	// PartWritings holds the writings he has read (J2): each writing in the
	// table becomes an entry there, written the first time he reads it.
	PartWritings = "writings"
)

// Layout bounds the panel draws to; Load refuses text that would not fit.
const (
	// MaxTitle is the longest title, in characters, the entry list draws
	// beside a task's state mark.
	MaxTitle = 24

	// MaxPartTitle is the longest tab title.
	MaxPartTitle = 10

	// WrapWidth is the text column's width in characters, and MaxLines the
	// most wrapped lines an entry may take (the panel does not scroll text).
	// 54 was MEASURED (24 Sep, -classic's Diablo II font, the widest the
	// game draws): 60 characters ran 492 px, past the 454 px column.
	WrapWidth = 54
	MaxLines  = 20
)

// When is one condition: exactly one field is set.
type When struct {
	Start bool   `json:"start,omitempty"` // holds from the first frame
	Flag  string `json:"flag,omitempty"`  // a village standing flag is set
	Rung  string `json:"rung,omitempty"`  // this rung reached, at any time
	Event string `json:"event,omitempty"` // the game has raised this event
	State string `json:"state,omitempty"` // a state the game reports holds now
	Read  string `json:"read,omitempty"`  // a writing read (J2)
	Date  string `json:"date,omitempty"`  // he has seen this date by daylight

	All []When `json:"all,omitempty"` // every one holds
	Any []When `json:"any,omitempty"` // at least one holds
	Not *When  `json:"not,omitempty"` // this one does not hold (review B4: hearsay after the gate shuts)
}

// Part is one tab.
type Part struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Entry is one page of the journal.
type Entry struct {
	ID      string `json:"id"`
	Part    string `json:"part"`
	Title   string `json:"title"`
	Text    string `json:"text"`
	When    When   `json:"when"`
	Sources string `json:"sources,omitempty"` // provenance; never shown

	// Anchor makes the entry a PLACE (J2): the panel adds, live, which way
	// the place lies from where he stands and how far. It names something
	// the game can locate (Names.Anchors: a villager, for now).
	Anchor string `json:"anchor,omitempty"`
}

// Writing is a thing he can read (J2): a book, a charter, a mark. Reading it
// the first time pays XP and writes its entry -- the whole text, in his
// words -- into the writings part; conditions elsewhere may read it
// ({"read": "R03"}), which is how a writing gives a tip or opens a task.
type Writing struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Text    string `json:"text"`
	XP      int    `json:"xp"`
	Sources string `json:"sources,omitempty"`
}

// WritingEntry is the entry id a writing's text is written under.
func WritingEntry(id string) string { return "w_" + strings.ToLower(id) }

// Stage is one state of a task: what makes it so, and what the page says.
type Stage struct {
	When When   `json:"when"`
	Text string `json:"text"`
}

// Task is a promise or an errand.
//
// A task is absent until one of its stages fires. It moves on the RISING EDGE
// of a stage's condition -- false becoming true, or an event in it raised
// again -- and only along these moves:
//
//	(absent)       -> open, done
//	open           -> done, failed
//	done, failed   -> open, only if Repeats (the watch, promised every night)
//
// So a task already done is not failed by a later loss (the ditch stays dug
// when the gate closes on him), and a repeating task reopens each time the
// promise is made again. This refines the attack's "latest edge wins" (A1,
// 24 Sep) with the one thing it left open: which edges a state listens to.
type Task struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Repeats bool   `json:"repeats,omitempty"`
	Open    *Stage `json:"open,omitempty"`
	Done    *Stage `json:"done,omitempty"`
	Failed  *Stage `json:"failed,omitempty"`
	Sources string `json:"sources,omitempty"`
}

// Task states.
const (
	TaskOpen   = "open"
	TaskDone   = "done"
	TaskFailed = "failed"
)

// stageNames are the states in stages() order.
var stageNames = [3]string{TaskOpen, TaskDone, TaskFailed}

// stages is open, done and failed, in the order an edge in one frame is
// applied: open first, so a task opened and finished in the same frame ends
// done.
func (t *Task) stages() [3]*Stage { return [3]*Stage{t.Open, t.Done, t.Failed} }

// DayPage is the template for the page written for each slice date. Its
// strings may use the placeholders in dayFields.
type DayPage struct {
	Title string   `json:"title"`
	Lines []string `json:"lines"`
	Data  string   `json:"data"`

	// Events is the day's own line, by date, when something happened.
	Events map[string]string `json:"events,omitempty"`

	// FastWords and SaintWords say the day table's Typikon fast rule and
	// commemoration in his words (the table's are a scholar's: "xerophagy",
	// "Hieromart."). A day with none shows the table's own; the shipped-file
	// test requires one for every slice day.
	FastWords  map[string]string `json:"fast_words,omitempty"`
	SaintWords map[string]string `json:"saint_words,omitempty"`
}

// Book is the validated journal table.
type Book struct {
	Parts    []Part    `json:"parts"`
	Entries  []Entry   `json:"entries"`
	Tasks    []Task    `json:"tasks"`
	Writings []Writing `json:"writings,omitempty"`
	DayPage  DayPage   `json:"day_page"`

	parts    map[string]bool
	entries  map[string]*Entry
	tasks    map[string]*Task
	writings map[string]*Writing
	names    Names
}

// Names are what the game can report, for Load to check every condition
// against. Items are what a {count:item} placeholder may name.
type Names struct {
	Flags   []string
	Rungs   []string
	Events  []string
	States  []string
	Items   []string
	Anchors []string // what a place's anchor may name
}

var (
	datePattern  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	countPattern = regexp.MustCompile(`\{count:([a-z0-9-]+)\}`)
	fieldPattern = regexp.MustCompile(`\{([a-z_]+)\}`)
	idPattern    = regexp.MustCompile(`^[a-z0-9_:-]+$`)
)

// dayFields are the placeholders a day page may use.
var dayFields = map[string]bool{
	"weekday": true, "day": true, "month": true, "year": true, "ramazan": true,
	"feast": true, "saint": true, "fast": true, "event": true,
	"sunset": true, "dark_start": true, "dark_end": true,
	"moonrise": true, "lit": true, "moonless": true,
}

// Load parses and validates a journal table against the game's names.
func Load(data []byte, n Names) (*Book, error) {
	var b Book

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&b); err != nil {
		return nil, fmt.Errorf("decode journal: %w", err)
	}

	b.parts = map[string]bool{}
	b.entries = map[string]*Entry{}
	b.tasks = map[string]*Task{}
	b.writings = map[string]*Writing{}

	for i, p := range b.Parts {
		if p.ID == "" || p.Title == "" {
			return nil, fmt.Errorf("part %d needs an id and a title", i)
		}

		if b.parts[p.ID] {
			return nil, fmt.Errorf("two parts named %q", p.ID)
		}

		if utf8.RuneCountInString(p.Title) > MaxPartTitle {
			return nil, fmt.Errorf("part %s: title longer than %d characters", p.ID, MaxPartTitle)
		}

		b.parts[p.ID] = true
	}

	for _, need := range []string{PartDays, PartTasks} {
		if !b.parts[need] {
			return nil, fmt.Errorf("the journal needs a %q part", need)
		}
	}

	// The writings: each one is a name a read condition may use, and an
	// entry, written the first time he reads it, holding its text.
	if len(b.Writings) > 0 && !b.parts[PartWritings] {
		return nil, fmt.Errorf("the journal has writings and no %q part", PartWritings)
	}

	// The only names a read may use are the table's own writings: a read of
	// anything else could never be recorded (Journal.Read).
	var readable []string

	for i := range b.Writings {
		w := &b.Writings[i]

		if w.ID == "" || b.writings[w.ID] != nil {
			return nil, fmt.Errorf("writing %d: an id, and only once (%q)", i, w.ID)
		}

		if w.XP < 0 {
			return nil, fmt.Errorf("writing %s: xp cannot be negative", w.ID)
		}

		b.writings[w.ID] = w
		readable = append(readable, w.ID)
		b.Entries = append(b.Entries, Entry{
			ID: WritingEntry(w.ID), Part: PartWritings, Title: w.Title, Text: w.Text,
			When: When{Read: w.ID}, Sources: w.Sources,
		})
	}

	b.names = n

	known := func(list []string) map[string]bool {
		m := map[string]bool{}
		for _, s := range list {
			m[s] = true
		}

		return m
	}

	sets := map[string]map[string]bool{
		"flag": known(n.Flags), "rung": known(n.Rungs), "event": known(n.Events),
		"state": known(n.States), "read": known(readable),
	}
	items, anchors := known(n.Items), known(n.Anchors)

	checkText := func(where, text string) error {
		if strings.TrimSpace(text) == "" {
			return fmt.Errorf("%s has no text", where)
		}

		for _, m := range countPattern.FindAllStringSubmatch(text, -1) {
			if !items[m[1]] {
				return fmt.Errorf("%s counts %q, which is not an item", where, m[1])
			}
		}

		// Counts render wider than the placeholder never; the bound is
		// checked on the text as written, with each count as three digits.
		if lines := len(Wrap(countPattern.ReplaceAllString(text, "000"), WrapWidth)); lines > MaxLines {
			return fmt.Errorf("%s wraps to %d lines; the panel draws %d", where, lines, MaxLines)
		}

		return nil
	}

	checkTitle := func(where, title string) error {
		if title == "" {
			return fmt.Errorf("%s has no title", where)
		}

		if utf8.RuneCountInString(title) > MaxTitle {
			return fmt.Errorf("%s: title %q is longer than %d characters", where, title, MaxTitle)
		}

		return nil
	}

	ids := map[string]bool{}

	for i := range b.Entries {
		e := &b.Entries[i]
		where := fmt.Sprintf("entry %q", e.ID)

		if e.ID == "" || !idPattern.MatchString(e.ID) {
			return nil, fmt.Errorf("entry %d: id %q must be lower-case letters, digits, _ - :", i, e.ID)
		}

		if ids[e.ID] {
			return nil, fmt.Errorf("two entries or tasks named %q", e.ID)
		}

		ids[e.ID] = true

		if !b.parts[e.Part] || e.Part == PartTasks {
			return nil, fmt.Errorf("%s: no part %q for an entry", where, e.Part)
		}

		if err := checkTitle(where, e.Title); err != nil {
			return nil, err
		}

		if err := checkText(where, e.Text); err != nil {
			return nil, err
		}

		if err := checkWhen(where, e.When, sets); err != nil {
			return nil, err
		}

		if e.Anchor != "" && !anchors[e.Anchor] {
			return nil, fmt.Errorf("%s: no anchor %q to place it by", where, e.Anchor)
		}

		// A place's text carries two more lines, the where-line and a blank.
		if e.Anchor != "" && len(Wrap(e.Text, WrapWidth)) > MaxLines-2 {
			return nil, fmt.Errorf("%s: a place's text must leave two lines for where it lies", where)
		}

		b.entries[e.ID] = e
	}

	for i := range b.Tasks {
		t := &b.Tasks[i]
		where := fmt.Sprintf("task %q", t.ID)

		if t.ID == "" || !idPattern.MatchString(t.ID) {
			return nil, fmt.Errorf("task %d: id %q must be lower-case letters, digits, _ - :", i, t.ID)
		}

		if ids[t.ID] {
			return nil, fmt.Errorf("two entries or tasks named %q", t.ID)
		}

		ids[t.ID] = true

		if err := checkTitle(where, t.Title); err != nil {
			return nil, err
		}

		if t.Open == nil && t.Done == nil {
			return nil, fmt.Errorf("%s can never appear: it needs an open or a done stage", where)
		}

		if t.Failed != nil && t.Open == nil {
			return nil, fmt.Errorf("%s: only an open task can fail, and it has no open stage", where)
		}

		if t.Repeats && t.Open == nil {
			return nil, fmt.Errorf("%s repeats but has no open stage to repeat", where)
		}

		for k, s := range t.stages() {
			if s == nil {
				continue
			}

			sw := where + " " + stageNames[k]

			if err := checkText(sw, s.Text); err != nil {
				return nil, err
			}

			if err := checkWhen(sw, s.When, sets); err != nil {
				return nil, err
			}

			if s.When.Start {
				return nil, fmt.Errorf("%s: a task stage cannot be {start}; it is an entry", sw)
			}
		}

		b.tasks[t.ID] = t
	}

	if err := b.checkDayPage(); err != nil {
		return nil, err
	}

	return &b, nil
}

// checkWhen refuses a condition that is empty, ambiguous or names something
// the game cannot report.
func checkWhen(where string, w When, sets map[string]map[string]bool) error {
	kinds := 0

	for _, set := range []bool{w.Start, w.Flag != "", w.Rung != "", w.Event != "", w.State != "",
		w.Read != "", w.Date != "", w.All != nil, w.Any != nil, w.Not != nil} {
		if set {
			kinds++
		}
	}

	if kinds != 1 {
		return fmt.Errorf("%s: a condition needs exactly one of start, flag, rung, event, state, read, date, all, any, not (has %d)", where, kinds)
	}

	for kind, name := range map[string]string{"flag": w.Flag, "rung": w.Rung, "event": w.Event, "state": w.State, "read": w.Read} {
		if name != "" && !sets[kind][name] {
			return fmt.Errorf("%s: no %s %q", where, kind, name)
		}
	}

	if w.Date != "" && !datePattern.MatchString(w.Date) {
		return fmt.Errorf("%s: date %q is not YYYY-MM-DD", where, w.Date)
	}

	if w.Not != nil {
		if w.Not.Start {
			return fmt.Errorf("%s: not start can never hold", where)
		}

		if err := checkWhen(where, *w.Not, sets); err != nil {
			return err
		}
	}

	for _, list := range [][]When{w.All, w.Any} {
		if list != nil && len(list) == 0 {
			return fmt.Errorf("%s: an empty all or any", where)
		}

		for _, c := range list {
			if err := checkWhen(where, c, sets); err != nil {
				return err
			}
		}
	}

	return nil
}

func (b *Book) checkDayPage() error {
	d := b.DayPage

	if d.Title == "" || d.Data == "" || len(d.Lines) == 0 {
		return fmt.Errorf("the day page needs a title, lines and a data line")
	}

	for _, s := range append([]string{d.Title, d.Data}, d.Lines...) {
		for _, m := range fieldPattern.FindAllStringSubmatch(s, -1) {
			if !dayFields[m[1]] {
				return fmt.Errorf("the day page uses {%s}, which is not a day field", m[1])
			}
		}
	}

	for date, line := range d.Events {
		if !datePattern.MatchString(date) || strings.TrimSpace(line) == "" {
			return fmt.Errorf("day page event %q: a YYYY-MM-DD date and a line", date)
		}
	}

	return nil
}

// Writing is a writing by id, or nil.
func (b *Book) Writing(id string) *Writing { return b.writings[id] }

// WritingIDs is every writing, in table order: what a talk's read may name.
func (b *Book) WritingIDs() []string {
	out := make([]string, 0, len(b.Writings))
	for _, w := range b.Writings {
		out = append(out, w.ID)
	}

	return out
}

// Entry is an entry by id, or nil.
func (b *Book) Entry(id string) *Entry { return b.entries[id] }

// Task is a task by id, or nil.
func (b *Book) Task(id string) *Task { return b.tasks[id] }

// Events is every event name the book's conditions use, sorted: what the game
// must be able to raise for the journal to be complete.
func (b *Book) Events() []string {
	seen := map[string]bool{}

	var walk func(w When)
	walk = func(w When) {
		if w.Event != "" {
			seen[w.Event] = true
		}

		for _, c := range append(append([]When{}, w.All...), w.Any...) {
			walk(c)
		}

		if w.Not != nil {
			walk(*w.Not)
		}
	}

	for _, e := range b.Entries {
		walk(e.When)
	}

	for _, t := range b.Tasks {
		for _, s := range t.stages() {
			if s != nil {
				walk(s.When)
			}
		}
	}

	out := make([]string, 0, len(seen))
	for e := range seen {
		out = append(out, e)
	}

	sort.Strings(out)

	return out
}

// Wrap breaks text into lines of at most width characters (runes, not
// bytes: the journal is full of ş and ç), at spaces. A newline starts a new
// paragraph; an empty line between paragraphs is kept. A word longer than
// the width is put on a line of its own rather than cut.
func Wrap(text string, width int) []string {
	var lines []string

	for _, para := range strings.Split(text, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}

		line, n := "", 0

		for _, w := range words {
			wn := utf8.RuneCountInString(w)

			switch {
			case n == 0:
				line, n = w, wn
			case n+1+wn <= width:
				line, n = line+" "+w, n+1+wn
			default:
				lines = append(lines, line)
				line, n = w, wn
			}
		}

		lines = append(lines, line)
	}

	return lines
}
