package d2journal

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Facts is what the game reports for a condition to read. It is asked every
// frame the screen is live, so each answer must be cheap.
type Facts interface {
	Flag(name string) bool
	Rung(id string) bool // reached NOW; the journal remembers it was
	State(name string) bool

	// Date is the slice date he can see by daylight, "YYYY-MM-DD", and false
	// in the deep night: the date turns at midnight, inside the dark, and a
	// page is written when he sees the day (A8, 24 Sep).
	Date() (string, bool)
}

// Lookups are what the panel's text asks the game, live: how many of an item
// he carries (a {count:item}), and where an anchored place lies from him,
// in tiles -- east is +x and north is -y, the generated map's own north (its
// north fence is at the low y, d2mapgen/act1_overworld.go:236-239). On the
// isometric screen that north is up and to the LEFT: a villager straight up
// the screen reads "north-west".
type Lookups struct {
	Count func(item string) int
	Where func(anchor string) (dx, dy float64, ok bool)
}

// Day is one slice date's calendar and sky, from the game's day table. The
// minutes are minute-of-day; Moonrise is -1 when the moon does not rise
// before dawn.
type Day struct {
	Date               string // YYYY-MM-DD
	Year, Month, Dom   int
	Weekday            string
	Feast, Saint       string
	Fast               string
	Sunset, Moonrise   int
	Lit                float64
	DarkStart, DarkEnd int
}

// condMem is what a task stage remembers between frames, to see its edge.
type condMem struct {
	Held   bool `json:"held,omitempty"`
	Events int  `json:"events,omitempty"`
}

type taskState struct {
	State string     `json:"state,omitempty"`
	Seq   int        `json:"seq,omitempty"`
	Mem   [3]condMem `json:"mem"`
}

// state is everything the journal saves. Order is a monotonic sequence
// number, never world time: the clock is not saved (B4, 24 Sep).
type state struct {
	Seq      int                   `json:"seq"`
	Written  map[string]int        `json:"written"`
	Pages    map[string]int        `json:"pages"`
	Events   map[string]int        `json:"events"`
	Reads    map[string]int        `json:"reads,omitempty"`
	Rungs    map[string]bool       `json:"rungs"`
	Tasks    map[string]*taskState `json:"tasks"`
	Seen     map[string]int        `json:"seen"`
	LastDate string                `json:"last_date,omitempty"`
}

func newState() state {
	return state{
		Written: map[string]int{}, Pages: map[string]int{}, Events: map[string]int{},
		Reads: map[string]int{}, Rungs: map[string]bool{}, Tasks: map[string]*taskState{},
		Seen: map[string]int{},
	}
}

// Journal is one hero's journal: the book and what he has written in it.
type Journal struct {
	book   *Book
	days   map[string]Day
	events map[string]bool
	st     state
	last   string // the id last written, for the notice and the provider
}

// New is an empty journal over a book and the slice's days.
func New(b *Book, days []Day) *Journal {
	j := &Journal{book: b, days: map[string]Day{}, events: map[string]bool{}, st: newState()}

	for _, d := range days {
		j.days[d.Date] = d
	}

	for _, e := range b.names.Events {
		j.events[e] = true
	}

	return j
}

// Restore reads a saved journal. A block that cannot be read leaves the
// journal empty and says why: the start entries are written again at once,
// and nothing else is lost that the standing does not rebuild.
func (j *Journal) Restore(raw json.RawMessage) error {
	j.st = newState()

	if len(raw) == 0 {
		return nil
	}

	st := newState()
	if err := json.Unmarshal(raw, &st); err != nil {
		return fmt.Errorf("journal: %w", err)
	}

	// A hand-edited or older block may lack a map; nil maps would panic on
	// the first write.
	fresh := newState()
	if st.Written == nil {
		st.Written = fresh.Written
	}

	if st.Pages == nil {
		st.Pages = fresh.Pages
	}

	if st.Events == nil {
		st.Events = fresh.Events
	}

	if st.Reads == nil {
		st.Reads = fresh.Reads
	}

	if st.Rungs == nil {
		st.Rungs = fresh.Rungs
	}

	if st.Tasks == nil {
		st.Tasks = fresh.Tasks
	}

	if st.Seen == nil {
		st.Seen = fresh.Seen
	}

	for id, t := range st.Tasks {
		if t == nil {
			delete(st.Tasks, id)
		}
	}

	j.st = st

	return nil
}

// Save is the block the sidecar carries.
func (j *Journal) Save() (json.RawMessage, error) {
	return json.Marshal(j.st)
}

// Note raises an event. It reports false, and records nothing, for a name
// the game did not declare -- a typo at a call site must not fill the save
// with events no condition can read. A nil journal notes nothing: events can
// be raised before the journal is bound (A11).
func (j *Journal) Note(event string) bool {
	if j == nil || !j.events[event] {
		return false
	}

	j.st.Events[event]++

	return true
}

// Read records a writing read (J2) and reports whether it was the first
// time: only a first read pays. A writing the table does not hold is not
// read at all.
func (j *Journal) Read(id string) (first bool) {
	if j == nil || j.book.writings[id] == nil {
		return false
	}

	first = j.st.Reads[id] == 0
	j.st.Reads[id]++

	return first
}

// Events is how many times an event has been raised.
func (j *Journal) Events(event string) int {
	if j == nil {
		return 0
	}

	return j.st.Events[event]
}

// Written is what one Evaluate wrote.
type Written struct {
	Entries []string // entry ids, in the order written
	Pages   []string // dates
	Tasks   []string // task ids whose state changed
}

// Any reports whether anything was written.
func (w Written) Any() bool { return len(w.Entries)+len(w.Pages)+len(w.Tasks) > 0 }

// Evaluate writes whatever now holds. It is cheap enough to run every frame
// (B2): about two hundred conditions over maps and a handful of facts.
func (j *Journal) Evaluate(f Facts) Written {
	var out Written

	for _, r := range j.book.names.Rungs {
		if f.Rung(r) {
			j.st.Rungs[r] = true
		}
	}

	// Only a slice date counts: past the slice there is no page and no dated
	// entry to write, and a stray date must not run LastDate on. The date
	// moves first, so a dated entry is written on its day; the page is
	// numbered LAST, so it heads what the same frame wrote (newest first).
	newPage := ""

	if d, ok := f.Date(); ok {
		if _, known := j.days[d]; known {
			if j.st.Pages[d] == 0 {
				newPage = d
			}

			if d > j.st.LastDate {
				j.st.LastDate = d
			}
		}
	}

	for i := range j.book.Entries {
		e := &j.book.Entries[i]
		if j.st.Written[e.ID] != 0 || !j.holds(e.When, f) {
			continue
		}

		j.st.Seq++
		j.st.Written[e.ID] = j.st.Seq
		j.last = e.ID
		out.Entries = append(out.Entries, e.ID)
	}

	for i := range j.book.Tasks {
		t := &j.book.Tasks[i]

		ts := j.st.Tasks[t.ID]
		if ts == nil {
			ts = &taskState{}
			j.st.Tasks[t.ID] = ts
		}

		changed := false

		for k, s := range t.stages() {
			if s == nil {
				continue
			}

			held, ev := j.holds(s.When, f), j.eventTotal(s.When)
			mem := &ts.Mem[k]
			edge := held && (!mem.Held || ev > mem.Events)
			mem.Held, mem.Events = held, ev

			if !edge {
				continue
			}

			to := stageNames[k]
			if !canMove(ts.State, to, t.Repeats) {
				continue
			}

			ts.State = to
			changed = true
		}

		if changed {
			j.st.Seq++
			ts.Seq = j.st.Seq
			j.last = t.ID
			out.Tasks = append(out.Tasks, t.ID)
		}
	}

	if newPage != "" {
		j.st.Seq++
		j.st.Pages[newPage] = j.st.Seq
		j.last = "page:" + newPage
		out.Pages = append(out.Pages, newPage)
	}

	return out
}

// canMove is Task's table of moves.
func canMove(from, to string, repeats bool) bool {
	switch from {
	case "":
		return to == TaskOpen || to == TaskDone
	case TaskOpen:
		return to == TaskDone || to == TaskFailed
	default: // done or failed
		return to == TaskOpen && repeats
	}
}

func (j *Journal) holds(w When, f Facts) bool {
	switch {
	case w.Start:
		return true
	case w.Flag != "":
		return f.Flag(w.Flag)
	case w.Rung != "":
		return j.st.Rungs[w.Rung]
	case w.Event != "":
		return j.st.Events[w.Event] > 0
	case w.State != "":
		return f.State(w.State)
	case w.Read != "":
		return j.st.Reads[w.Read] > 0
	case w.Date != "":
		return j.st.LastDate != "" && j.st.LastDate >= w.Date
	case w.All != nil:
		for _, c := range w.All {
			if !j.holds(c, f) {
				return false
			}
		}

		return true
	case w.Any != nil:
		for _, c := range w.Any {
			if j.holds(c, f) {
				return true
			}
		}
	case w.Not != nil:
		return !j.holds(*w.Not, f)
	}

	return false
}

// eventTotal is how many times the events (and reads) in a condition have
// been raised: a rise in it is an edge even while the condition holds. An
// event under a not is left out -- raising it can only make the condition
// fail.
func (j *Journal) eventTotal(w When) int {
	n := j.st.Events[w.Event] + j.st.Reads[w.Read]

	for _, c := range w.All {
		n += j.eventTotal(c)
	}

	for _, c := range w.Any {
		n += j.eventTotal(c)
	}

	return n
}

// --- what the panel shows ----------------------------------------------------

// View is one row of a part: an entry, a task or a day page.
type View struct {
	ID     string
	Title  string
	Text   string
	Mark   string // a task's state; "" for anything else
	Unread bool
	Seq    int
}

// Parts are the tabs.
func (j *Journal) Parts() []Part { return j.book.Parts }

// View is a part's rows, newest first, with counts filled in.
func (j *Journal) View(part string, lk Lookups) []View {
	var out []View

	seen := j.st.Seen[part]

	if part == PartDays {
		for date, seq := range j.st.Pages {
			d, ok := j.days[date]
			if !ok {
				continue
			}

			title, text := j.book.DayPage.Render(d)
			out = append(out, View{ID: "page:" + date, Title: title, Text: text, Seq: seq, Unread: seq > seen})
		}
	}

	if part == PartTasks {
		for i := range j.book.Tasks {
			t := &j.book.Tasks[i]

			ts := j.st.Tasks[t.ID]
			if ts == nil || ts.State == "" {
				continue
			}

			out = append(out, View{
				ID: t.ID, Title: t.Title, Text: fillCounts(j.taskText(t, ts.State), lk.Count),
				Mark: ts.State, Seq: ts.Seq, Unread: ts.Seq > seen,
			})
		}
	}

	for i := range j.book.Entries {
		e := &j.book.Entries[i]

		seq := j.st.Written[e.ID]
		if e.Part != part || seq == 0 {
			continue
		}

		text := fillCounts(e.Text, lk.Count)
		if e.Anchor != "" {
			text += "\n\n" + whereIs(e.Anchor, lk.Where)
		}

		out = append(out, View{ID: e.ID, Title: e.Title, Text: text, Seq: seq, Unread: seq > seen})
	}

	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Seq != out[b].Seq {
			return out[a].Seq > out[b].Seq
		}

		return out[a].ID < out[b].ID
	})

	return out
}

func (j *Journal) taskText(t *Task, state string) string {
	switch state {
	case TaskDone:
		return t.Done.Text
	case TaskFailed:
		return t.Failed.Text
	default:
		return t.Open.Text
	}
}

// Unread is how many rows of a part are unread.
func (j *Journal) Unread(part string) int {
	n := 0

	for _, v := range j.View(part, Lookups{}) {
		if v.Unread {
			n++
		}
	}

	return n
}

// Leave marks every row of a part read: he has turned away from it.
func (j *Journal) Leave(part string) { j.st.Seen[part] = j.st.Seq }

// Last is the id last written ("page:<date>" for a page), or "".
func (j *Journal) Last() string { return j.last }

// Title is the title of an entry, a task or a page, for the notice.
func (j *Journal) Title(id string) string {
	if date, ok := strings.CutPrefix(id, "page:"); ok {
		if d, ok := j.days[date]; ok {
			title, _ := j.book.DayPage.Render(d)
			return title
		}

		return ""
	}

	if e := j.book.entries[id]; e != nil {
		return e.Title
	}

	if t := j.book.tasks[id]; t != nil {
		return t.Title
	}

	return ""
}

func fillCounts(text string, counts func(string) int) string {
	return countPattern.ReplaceAllStringFunc(text, func(m string) string {
		if counts == nil {
			return "0"
		}

		return strconv.Itoa(counts(countPattern.FindStringSubmatch(m)[1]))
	})
}

// HarnessState is what the "journal" provider reports: every id written,
// each task's state, the pages, and the events raised.
func (j *Journal) HarnessState() map[string]interface{} {
	written := make([]string, 0, len(j.st.Written))
	for id := range j.st.Written {
		written = append(written, id)
	}

	sort.Strings(written)

	pages := make([]string, 0, len(j.st.Pages))
	for d := range j.st.Pages {
		pages = append(pages, d)
	}

	sort.Strings(pages)

	tasks := map[string]interface{}{}

	for id, t := range j.st.Tasks {
		if t.State != "" {
			tasks[id] = t.State
		}
	}

	events := map[string]interface{}{}
	for e, n := range j.st.Events {
		events[e] = n
	}

	reads := map[string]interface{}{}
	for r, n := range j.st.Reads {
		reads[r] = n
	}

	unread := map[string]interface{}{}
	for _, p := range j.book.Parts {
		unread[p.ID] = j.Unread(p.ID)
	}

	return map[string]interface{}{
		"written":      written,
		"pages":        pages,
		"tasks":        tasks,
		"events":       events,
		"reads":        reads,
		"unread":       unread,
		"last_written": j.last,
		"last_date":    j.st.LastDate,
		"seq":          j.st.Seq,
	}
}
