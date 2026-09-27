package d2records

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2txt"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
)

// Item.All is whatever item tables are loaded, merged, as each loads (M5.3's
// tables burst): it used to wait for all three and then never change, which
// tied misc.txt -- read by nothing else -- to boot.
//
// Negative control: restore the all-three condition and All stays nil after
// misc alone, and never learns the weapon loaded after it.
func TestItemAllMergesWhateverIsLoaded(t *testing.T) {
	r := &RecordManager{boundLoaders: map[string][]recordLoader{}}

	fake := func(set func(r *RecordManager)) recordLoader {
		return func(r *RecordManager, _ *d2txt.DataDictionary) error {
			set(r)
			return nil
		}
	}

	if err := r.AddLoader(d2resource.Misc, fake(func(r *RecordManager) {
		r.Item.Misc = CommonItems{"tch": &ItemCommonRecord{Code: "tch"}}
	})); err != nil {
		t.Fatal(err)
	}

	if err := r.AddLoader(d2resource.Weapons, fake(func(r *RecordManager) {
		r.Item.Weapons = CommonItems{"jav": &ItemCommonRecord{Code: "jav"}}
	})); err != nil {
		t.Fatal(err)
	}

	if err := r.Load(d2resource.Misc, nil); err != nil {
		t.Fatal(err)
	}

	if r.Item.All["tch"] == nil || len(r.Item.All) != 1 {
		t.Fatalf("after misc alone: All %v, want the misc item", r.Item.All)
	}

	if err := r.Load(d2resource.Weapons, nil); err != nil {
		t.Fatal(err)
	}

	if r.Item.All["tch"] == nil || r.Item.All["jav"] == nil || len(r.Item.All) != 2 {
		t.Fatalf("after misc then weapons: All %v, want both", r.Item.All)
	}
}

// Without experience.txt the breakpoint is 0, not a nil dereference: Strigoi's
// game does not load it (its experience is d2progress's).
//
// Negative control: index the missing record directly and this panics.
func TestExperienceBreakpointWithoutTheTable(t *testing.T) {
	r := &RecordManager{}

	if got := r.GetExperienceBreakpoint(d2enum.HeroAmazon, 1); got != 0 {
		t.Fatalf("breakpoint %d without experience.txt, want 0", got)
	}

	r.Character.Experience = ExperienceBreakpoints{1: {HeroBreakpoints: map[d2enum.Hero]int{d2enum.HeroAmazon: 500}}}

	if got := r.GetExperienceBreakpoint(d2enum.HeroAmazon, 1); got != 500 {
		t.Fatalf("breakpoint %d with the table, want its 500", got)
	}
}
