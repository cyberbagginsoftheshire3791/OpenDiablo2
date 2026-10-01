package d2world

// SEEK'S SNAPSHOT, THE FIVE WAYS (the raid's R2; docs/m4.6-world-save-notes.md,
// "B2a", and "The raid's R2"): the classification of every field, the round
// trip and the run on, the sweep over every leaf, and the probes -- over a
// fake night in which six wolves hunt him and three villagers (one a
// stand-in), two more arrive late, and the quarries walk.

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// seekEntities is the fake night's entities by id: the Resolver a restore
// resolves the notice block through, and what a stand-in resolves to.
type seekEntities struct {
	watchers map[string]*fakeWatcher
	quarries map[string]*fakeQuarry
}

func (e *seekEntities) Watcher(id string) (Watcher, bool) {
	w, ok := e.watchers[id]
	if !ok || w == nil {
		return nil, false
	}

	return w, true
}

func (e *seekEntities) Quarry(ref string) (Quarry, bool) {
	if ref == PlayerRef {
		ref = "p:1"
	}

	q, ok := e.quarries[ref]
	if !ok || q == nil {
		return nil, false
	}

	return q, true
}

// copy is every entity where it stands now, as new values.
func (e *seekEntities) copy() *seekEntities {
	c := &seekEntities{watchers: map[string]*fakeWatcher{}, quarries: map[string]*fakeQuarry{}}

	for id, w := range e.watchers {
		cw := *w
		c.watchers[id] = &cw
	}

	for id, q := range e.quarries {
		cq := *q
		c.quarries[id] = &cq
	}

	return c
}

// seekSnapLayout is the night's cast: him, two villagers and a stand-in, and
// eight wolves (m:7 and m:8 watch only late; m:6 stands far off).
func seekSnapLayout() *seekEntities {
	e := &seekEntities{watchers: map[string]*fakeWatcher{}, quarries: map[string]*fakeQuarry{}}

	for _, q := range []*fakeQuarry{
		{id: "p:1", x: 20, y: 24}, {id: "v:1", x: 27, y: 20}, {id: "v:2", x: 20, y: 28}, {id: "v:3", x: 14, y: 21},
	} {
		e.quarries[q.id] = q
	}

	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("m:%d", i+1)
		e.watchers[id] = &fakeWatcher{id: id, x: 10 + 4*float64(i), y: 12 + float64(i%3)*6}
	}

	e.watchers["m:6"] = &fakeWatcher{id: "m:6", x: 60, y: 60}
	e.watchers["m:7"] = &fakeWatcher{id: "m:7", x: 24, y: 30}
	e.watchers["m:8"] = &fakeWatcher{id: "m:8", x: 16, y: 16}

	return e
}

// seekSnapWorld is one fake night: its entities, the notice model and Seek.
type seekSnapWorld struct {
	ents   *seekEntities
	notice *Notice
	seek   *Seek
	frame  int
}

func newSeekSnapWorld(t *testing.T, ents *seekEntities, frame int) *seekSnapWorld {
	t.Helper()

	w := &seekSnapWorld{ents: ents, frame: frame}
	w.notice = NewNotice(&fakeSight{clear: true}, &fakeIllumination{}, DefaultNoticeDials())
	w.seek = NewSeek(w.notice, nil, nil, DefaultSeekDials())
	w.seek.SetResolver(ents)
	w.seek.SetQuarries(func() []Quarry {
		return []Quarry{ents.quarries["p:1"], ents.quarries["v:1"], ents.quarries["v:2"]}
	})

	t.Cleanup(w.seek.Close)

	return w
}

// step is one night frame: he and a villager walk, then the notice model and
// Seek step, in the game's order.
func (w *seekSnapWorld) step() {
	w.frame++
	g := float64(w.frame)

	p := w.ents.quarries["p:1"]
	p.x, p.y = 20+4*math.Sin(g/17), 20+4*math.Cos(g/23)

	v := w.ents.quarries["v:1"]
	v.x = 27 - float64(w.frame%80)/8

	w.notice.Advance(1.0 / 24)
	w.seek.Advance(1.0 / 24)
}

// record is what can be seen of the night: Seek's provider, the notice rows
// and where everyone stands -- first before a step (what a load shows on its
// first frame is the save's to get right), then every twenty frames, sixty.
func (w *seekSnapWorld) record(t *testing.T) string {
	t.Helper()

	var out []string

	for i := 0; i <= 3; i++ {
		if i > 0 {
			for f := 0; f < 20; f++ {
				w.step()
			}
		}

		at := map[string][2]float64{}
		for id, q := range w.ents.quarries {
			at[id] = [2]float64{q.x, q.y}
		}

		b, err := json.Marshal(map[string]interface{}{
			"seek":   w.seek.HarnessState(),
			"notice": w.notice.Report(),
			"at":     at,
		})
		require.NoError(t, err)

		out = append(out, string(b))
	}

	return strings.Join(out, "\n")
}

// seekFilled is the night at its saved moment: its Seek and notice
// snapshots, the entities as they stood, and the frame.
type seekFilled struct {
	a      *seekSnapWorld
	saved  *seekEntities
	seek   SeekSnapshot
	notice NoticeSnapshot
	frame  int
}

func seekFill(t *testing.T) seekFilled {
	t.Helper()

	a := newSeekSnapWorld(t, seekSnapLayout(), 0)
	require.NoError(t, a.seek.addStandIn("v:3"))

	for i := 1; i <= 6; i++ {
		id := fmt.Sprintf("m:%d", i)
		a.notice.Watch(a.ents.watchers[id], a.ents.quarries["p:1"])
	}

	for i := 0; i < 90; i++ {
		a.step()
	}

	// Two late arrivals, made on the saved frame: pending, their phases to come.
	a.notice.Watch(a.ents.watchers["m:7"], a.ents.quarries["p:1"])
	a.notice.Watch(a.ents.watchers["m:8"], a.ents.quarries["v:2"])
	a.step()

	ns, err := a.notice.Snapshot(a.ents)
	require.NoError(t, err)

	f := seekFilled{a: a, saved: a.ents.copy(), seek: a.seek.Snapshot(), notice: ns, frame: a.frame}

	reasons := map[string]int{}
	for _, r := range f.seek.Rows {
		reasons[r.Reason]++
	}

	require.NotZero(t, reasons[SeekLiving], "the fixture: a row on the nearest living (%v)", reasons)
	require.NotZero(t, reasons[SeekNone], "the fixture: a row with nothing in sight (%v)", reasons)
	require.NotZero(t, reasons[SeekPending], "the fixture: a row not yet looked (%v)", reasons)
	require.NotZero(t, f.seek.Retargets, "the fixture: watches moved")
	require.Less(t, f.seek.Retargets, f.seek.Looks)
	require.NotEmpty(t, f.seek.StandIns)

	return f
}

// resume restores a snapshot of Seek, beside the saved notice block, into a
// fresh night whose entities stand where they stood.
func (f seekFilled) resume(t *testing.T, snap SeekSnapshot) (*seekSnapWorld, error) {
	t.Helper()

	b := newSeekSnapWorld(t, f.saved.copy(), f.frame)

	if err := b.notice.Restore(b2aThroughJSON(t, f.notice), b.ents); err != nil {
		return nil, err
	}

	if err := b.seek.Restore(snap); err != nil {
		return nil, err
	}

	return b, nil
}

func seekClasses() []b2aClass {
	return []b2aClass{
		{Seek{}, map[string]string{
			"dials":     "D: Seek's dials, never saved (trap 7)",
			"notice":    "W: the notice model it reads and retargets through",
			"spawns":    "W: the spawn tables, asked which row a watcher is of",
			"combat":    "W: the combat model, asked who fights, who is dead and who is protected",
			"quarries":  "W: the game's living, a function of the live map",
			"resolver":  "W: the game's Resolver, finding stand-ins and naming him",
			"standIns":  "S:stand_ins",
			"rows":      "S:rows",
			"slots":     "S:slots",
			"looks":     "S:looks",
			"retargets": "S:retargets",
			"holds":     "S:holds",
			"rays":      "S:rays",
		}},
		{seekRow{}, map[string]string{
			"target":     "S:rows[0].target",
			"reason":     "S:rows[0].reason",
			"untilLook":  "S:rows[0].until_look_minutes",
			"candidates": "S:rows[0].candidates",
			"dwell":      "S:rows[0].dwell_minutes",
		}},
	}
}

func TestSeekSnapshotFieldsClassified(t *testing.T) {
	f := seekFill(t)

	for _, c := range seekClasses() {
		b2aClassified(t, c.kind, f.seek, c.fields)
	}
}

// Round trip and run on: through JSON into a fresh night, the copy is the
// original -- snapshot for snapshot, and sixty frames on.
func TestSeekSnapshotRoundTrip(t *testing.T) {
	f := seekFill(t)
	ref := f.a.record(t)

	b, err := f.resume(t, b2aThroughJSON(t, f.seek))
	require.NoError(t, err)
	require.Equal(t, f.seek, b.seek.Snapshot(), "restored, it snapshots as it was saved")
	require.Equal(t, ref, b.record(t), "and runs on as the saved night does")

	// Refused into a Seek in use, and the refusal changes nothing.
	before := f.a.seek.Snapshot()
	require.Error(t, f.a.seek.Restore(f.seek))
	require.Equal(t, before, f.a.seek.Snapshot())
	require.NoError(t, f.a.seek.CheckSnapshot(before), "the save's check takes the live Seek's own snapshot")
}

// The sweep: every leaf zeroed (or set), every element dropped -- each one
// refused, or seen.
func TestSeekSnapshotEveryFieldIsSeen(t *testing.T) {
	f := seekFill(t)
	ref := f.a.record(t)

	try := func(raw []byte) (string, error) {
		var snap SeekSnapshot
		if err := json.Unmarshal(raw, &snap); err != nil {
			return "", err
		}

		b, err := f.resume(t, snap)
		if err != nil {
			return "", err
		}

		return b.record(t), nil
	}

	b2aExercised(t, b2aSweep(t, f.seek, ref, nil, try), seekClasses()...)

	// The probes: valid-but-wrong, each accepted and seen.
	b2aMustDiverge(t, f.seek, ref, map[string]func(s *SeekSnapshot){
		"the last row under another watcher's id": func(s *SeekSnapshot) { s.Rows[len(s.Rows)-1].Watcher = "m:99" },
		"a row's target another quarry": func(s *SeekSnapshot) {
			if s.Rows[0].Target == PlayerRef {
				s.Rows[0].Target = "v:2"
			} else {
				s.Rows[0].Target = PlayerRef
			}
		},
		"a row's reason another": func(s *SeekSnapshot) {
			for i := range s.Rows {
				if s.Rows[i].Reason == SeekNone {
					s.Rows[i].Reason = SeekLiving

					return
				}
			}
		},
		"a row on another phase": func(s *SeekSnapshot) {
			if s.Rows[0].UntilLook > 0.5 {
				s.Rows[0].UntilLook -= 0.25
			} else {
				s.Rows[0].UntilLook += 0.25
			}
		},
		"one more phase given out": func(s *SeekSnapshot) { s.Slots++ },
		"another stand-in":         func(s *SeekSnapshot) { s.StandIns[0] = "v:9" },
	}, try)
}

// What the restore refuses, one at a time, of an otherwise good snapshot.
func TestSeekSnapshotRefusals(t *testing.T) {
	f := seekFill(t)

	for name, bad := range map[string]func(s *SeekSnapshot){
		"a row out of order":           func(s *SeekSnapshot) { s.Rows[0], s.Rows[1] = s.Rows[1], s.Rows[0] },
		"a row saved twice":            func(s *SeekSnapshot) { s.Rows[1].Watcher = s.Rows[0].Watcher },
		"a row on the word player":     func(s *SeekSnapshot) { s.Rows[0].Watcher = PlayerRef },
		"a row that is its own target": func(s *SeekSnapshot) { s.Rows[0].Target = s.Rows[0].Watcher },
		"a reason Seek does not give":  func(s *SeekSnapshot) { s.Rows[0].Reason = "hungry" },
		"a look already past":          func(s *SeekSnapshot) { s.Rows[0].UntilLook = -0.1 },
		"a look at NaN":                func(s *SeekSnapshot) { s.Rows[0].UntilLook = math.NaN() },
		"a pending row that counted": func(s *SeekSnapshot) {
			for i := range s.Rows {
				if s.Rows[i].Reason == SeekPending {
					s.Rows[i].Candidates = 2
				}
			}
		},
		"more rows than phases given": func(s *SeekSnapshot) { s.Slots = len(s.Rows) - 1 },
		"a retarget with no look":     func(s *SeekSnapshot) { s.Looks = s.Retargets - 1 },
		"a stand-in named twice":      func(s *SeekSnapshot) { s.StandIns = append(s.StandIns, s.StandIns[0]) },
		"him as a stand-in":           func(s *SeekSnapshot) { s.StandIns = append(s.StandIns, PlayerRef) },
		"a negative count":            func(s *SeekSnapshot) { s.Rays = -1 },
	} {
		s := b2aThroughJSON(t, f.seek)
		bad(&s)

		fresh := NewSeek(nil, nil, nil, DefaultSeekDials())
		require.Error(t, fresh.Restore(s), name)
		require.Empty(t, fresh.rows, "%s: a refused restore changes nothing", name)
		require.Empty(t, fresh.standIns, name)
		fresh.Close()
	}
}
