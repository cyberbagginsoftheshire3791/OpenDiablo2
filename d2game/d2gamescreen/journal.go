package d2gamescreen

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2craft"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2dialogue"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2journal"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"
)

// J1, the journal (24 Sep 2026): the game screen's half. It loads the table
// with the others, keeps his journal in the sidecar beside his kit, raises
// the events the table's conditions read, reports the facts they ask about,
// and runs the journal once a live frame -- after dawn has been settled, so a
// watch kept or broken lands on the new day (A8). See d2core/d2journal for
// the model, and claude/journal-build-brief.md for the design and its attack.

// journalPath is the shipped journal table.
const journalPath = "/data/strigoi/journal.json"

// Refusals of the journal.
var (
	errJournalNone  = errors.New("no journal")
	errJournalFight = errors.New("not in a fight")
)

// journalNoticeSeconds is how long "written in my journal" stays up.
const journalNoticeSeconds = 5.0

// loadJournal reads and validates the table against everything the game can
// report. A bad table is a CreateGame error like every other table (A2): a
// journal that silently lost an entry is worse than none.
func loadJournal(asset *d2asset.AssetManager, dialogue *d2dialogue.Book, items *d2items.Catalog, recipes *d2craft.Book) (*d2journal.Book, error) {
	data, err := asset.LoadFile(journalPath)
	if err != nil {
		return nil, fmt.Errorf("load Strigoi journal: %w", err)
	}

	book, err := d2journal.Load(data, journalNames(dialogue, items, recipes))
	if err != nil {
		return nil, err
	}

	return book, nil
}

// journalNames is every name a journal condition may use.
func journalNames(dialogue *d2dialogue.Book, items *d2items.Catalog, recipes *d2craft.Book) d2journal.Names {
	var rows, recipeIDs []string

	for _, r := range d2world.DefaultSpawnDials().Rows {
		rows = append(rows, r.Name)
	}

	rows = append(rows, d2world.RisenRow)

	for _, r := range recipes.Recipes {
		recipeIDs = append(recipeIDs, r.ID)
	}

	return d2journal.Names{
		Flags:   append(dialogue.FlagsNamed(), flagSeenStaking),
		Rungs:   dialogue.RungIDs(),
		Anchors: dialogue.SpeakerIDs(),
		States:  d2journal.GameStates(),
		Items:   items.IDs(),
		Events: d2journal.GameEvents(d2journal.Vocabulary{
			Rows: rows, Recipes: recipeIDs, Speakers: dialogue.SpeakerIDs(), Nodes: dialogue.NodeIDs(),
		}),
	}
}

// journalDays is the slice's day table as the journal reads it. True dark is
// the clock's own dials, 21:15 to 02:45, not C2's per-night minutes: the page
// shows the night the game plays (B9).
func journalDays() []d2journal.Day {
	dials := d2world.DefaultClockDials()

	var out []d2journal.Day

	for _, e := range d2world.SliceDays() {
		out = append(out, d2journal.Day{
			Date: fmt.Sprintf("%04d-%02d-%02d", e.Year, e.Month, e.Day), Year: e.Year, Month: e.Month, Dom: e.Day,
			Weekday: e.Weekday, Feast: e.Feast, Saint: e.FeastDetail, Fast: e.FastRule,
			Sunset: e.SunsetMinute, Moonrise: e.MoonriseMinute, Lit: e.MoonLitPercent,
			DarkStart: int(dials.NightStart), DarkEnd: int(dials.DawnStart),
		})
	}

	return out
}

// bindJournal reads his journal from the sidecar (nil: none yet). Called from
// bindKit, deferred FIRST so it runs LAST -- after the standing and progress
// it reads are bound (A9).
func (v *Game) bindJournal(raw json.RawMessage) {
	if v.journalBook == nil {
		return
	}

	j := d2journal.New(v.journalBook, journalDays())

	if err := j.Restore(raw); err != nil {
		v.Errorf("%v -- starting a fresh journal", err)
	}

	v.journal = j
	v.journalQuiet = true

	d2harness.Register(journalProvider{v})
}

// journalJSON is the block the sidecar carries.
func (v *Game) journalJSON() json.RawMessage {
	if v.journal == nil {
		return nil
	}

	data, err := v.journal.Save()
	if err != nil {
		v.Errorf("journal: %v", err)
		return nil
	}

	return data
}

// note raises a journal event. Safe before the journal is bound (A11): the
// journal's Note is nil-safe, and nothing before bindKit raises one.
func (v *Game) note(event string) { v.journal.Note(event) }

// journalAdvance runs once a live frame, after earnExperience: it samples
// the fight for what he has seen, writes what now holds, and says so.
func (v *Game) journalAdvance() {
	if v.journal == nil {
		return
	}

	v.sampleFight()

	w := v.journal.Evaluate(journalFacts{v})
	quiet := v.journalQuiet
	v.journalQuiet = false

	if !w.Any() {
		return
	}

	// The first frame writes the start entries, the first page and whatever
	// an older save already earned -- silently (B12).
	if !quiet && v.gameControls != nil {
		titles := make([]string, 0, len(w.Entries)+len(w.Pages)+len(w.Tasks))

		for _, d := range w.Pages {
			titles = append(titles, v.journal.Title("page:"+d))
		}

		for _, id := range append(append([]string{}, w.Entries...), w.Tasks...) {
			titles = append(titles, v.journal.Title(id))
		}

		text := fmt.Sprintf(d2player.JournalWritten, titles[0])
		if len(titles) > 1 {
			text = fmt.Sprintf(d2player.JournalWrittenMore, titles[0], len(titles)-1)
		}

		v.gameControls.JournalNotice(text, journalNoticeSeconds)
	}

	// B3 -- but not on the quiet first frame: it writes only what every load
	// writes again, and saving then changed his file the moment he entered,
	// which death puts back byte for byte (TestDeath act 4, measured 24 Sep).
	if !quiet {
		v.saveKit()
	}
}

// sampleFight notes each kind of enemy the first frame it stands in a fight
// with him, and a risen man before he knows what they are (A6). There is no
// hook for "a participant joined", so this is an edge over the participant
// list, remembered per encounter.
func (v *Game) sampleFight() {
	if v.combat == nil || v.spawns == nil || !v.combat.Fighting() {
		v.journalFight, v.journalRows = "", nil
		return
	}

	if enc := v.combat.Encounter(); enc != v.journalFight {
		v.journalFight, v.journalRows = enc, map[string]bool{}
	}

	for _, id := range v.combat.Order() {
		p, ok := v.spawns.ProfileOf(id)
		if !ok || p.Row == "" || v.journalRows[p.Row] {
			continue
		}

		v.journalRows[p.Row] = true
		v.note("beast:" + p.Row)

		if p.Row == d2world.RisenRow && !v.knowsTheDead() {
			v.note("risen_seen_untold")
		}
	}
}

// kitCount is how many of an item he carries, for a {count:item}.
func (v *Game) kitCount(item string) int {
	if v.kit == nil {
		return 0
	}

	return v.kit.Count(item)
}

// journalFacts is the world as the journal's conditions read it.
type journalFacts struct{ v *Game }

func (f journalFacts) Flag(name string) bool {
	return f.v.standing != nil && f.v.standing.Has(name)
}

func (f journalFacts) Rung(id string) bool {
	return f.v.dialogue != nil && f.v.standing != nil && f.v.dialogue.Reached(f.v.standing, id)
}

func (f journalFacts) State(name string) bool {
	v := f.v

	switch name {
	case "night":
		return v.night()
	case "fighting":
		return v.inFight()
	case "torch_lit":
		return v.torchLit()
	case "carries_torch":
		return v.kit != nil && v.kit.Carries("torch")
	case "shield":
		return v.kit != nil && v.kit.Carries("kalkan")
	}

	if v.meters == nil {
		return false
	}

	switch name {
	case "hungry":
		return v.meters.Hungry()
	case "thirsty":
		return v.meters.Thirsty()
	case "starving":
		return v.meters.Starving()
	case "parched":
		return v.meters.Parched()
	case "shaken":
		return v.meters.Shaken()
	}

	return false
}

// Date is today by daylight: nothing in the deep night, when the date has
// turned but he has not seen the day (A8).
func (f journalFacts) Date() (string, bool) {
	c := f.v.worldClock
	if c == nil || c.Stage() == d2world.StageNight {
		return "", false
	}

	e, ok := c.Today()
	if !ok {
		return "", false
	}

	return fmt.Sprintf("%04d-%02d-%02d", e.Year, e.Month, e.Day), true
}

// --- d2player.JournalHolder -------------------------------------------------

// JournalParts are the tabs, or none before the journal is bound.
func (v *Game) JournalParts() []d2journal.Part {
	if v.journal == nil {
		return nil
	}

	return v.journal.Parts()
}

// JournalRows is a part's rows, newest first.
func (v *Game) JournalRows(part string) []d2journal.View {
	if v.journal == nil {
		return nil
	}

	return v.journal.View(part, d2journal.Lookups{Count: v.kitCount, Where: v.whereIs})
}

// whereIs is where a place's anchor stands from him, in tiles (J2): east is
// +x and north is -y. The anchors are villagers, found by their stand-in.
func (v *Game) whereIs(anchor string) (dx, dy float64, ok bool) {
	e := v.speakerEntity(anchor)
	if e == nil || v.localPlayer == nil {
		return 0, 0, false
	}

	ex, ey := e.GetPositionF()
	px, py := v.localPlayer.GetPositionF()

	return ex - px, ey - py, true
}

// readWriting is J2's read effect, after the talk that read it has ended: the
// first read pays its experience, and every read opens his journal at the
// writing, so he can read it again.
func (v *Game) readWriting(id string) {
	if v.journal == nil || v.journalBook == nil {
		return
	}

	w := v.journalBook.Writing(id)
	if w == nil {
		return
	}

	first := v.journal.Read(id)

	// Written now, not next frame, so the page is there to open.
	written := v.journal.Evaluate(journalFacts{v})

	if first && v.progress != nil && v.talents != nil {
		v.gainXP(w.XP)
	}

	v.saveKit()

	if v.gameControls == nil {
		return
	}

	v.gameControls.OpenJournalAt(d2journal.PartWritings, d2journal.WritingEntry(id))

	// What else the read wrote -- a tip, a place -- is said when he closes
	// the journal (the notice waits while it is open; it is set after the
	// opening, which clears an old one).
	var also []string

	for _, e := range append(append([]string{}, written.Entries...), written.Tasks...) {
		if e != d2journal.WritingEntry(id) {
			also = append(also, v.journal.Title(e))
		}
	}

	switch {
	case len(also) == 1:
		v.gameControls.JournalNotice(fmt.Sprintf(d2player.JournalWritten, also[0]), journalNoticeSeconds)
	case len(also) > 1:
		v.gameControls.JournalNotice(fmt.Sprintf(d2player.JournalWrittenMore, also[0], len(also)-1), journalNoticeSeconds)
	}
}

// JournalLeave marks a part read.
func (v *Game) JournalLeave(part string) {
	if v.journal != nil {
		v.journal.Leave(part)
	}
}

// JournalAllowed is why the journal cannot open now, or nil. The panel asks
// before it opens: not while dead, talking, choosing a loadout, or in ANY
// fight (A3, B5) -- a paced fight holds the world already, and an unpaced one
// must not be paused by reading.
func (v *Game) JournalAllowed() error {
	switch {
	case v.journal == nil:
		return errJournalNone
	case v.died || !v.alive():
		return errCraftDead
	case v.talk != nil && !v.talk.Done(), v.choosingLoadout:
		return errCraftBusy
	case v.inFight():
		return errJournalFight
	}

	return nil
}

// SetJournalOpen holds the world while he reads, and saves what he has read
// when he closes it.
func (v *Game) SetJournalOpen(open bool) {
	v.journalOpen = open && v.journal != nil

	if !open {
		v.saveKit()
	}
}

// journalProvider is the "journal" harness system: read-only.
type journalProvider struct{ v *Game }

func (p journalProvider) HarnessName() string { return "journal" }

func (p journalProvider) HarnessState() map[string]interface{} {
	if p.v.journal == nil {
		return map[string]interface{}{"bound": false}
	}

	state := p.v.journal.HarnessState()
	state["bound"] = true
	state["open"] = p.v.journalOpen

	return state
}
