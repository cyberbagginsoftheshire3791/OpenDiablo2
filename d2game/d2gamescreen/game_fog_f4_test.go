package d2gamescreen

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2progress"
)

// TestHisSightTermsAreHisKitAndTalents (fog of war F4): his eye carries what
// his kit and talents give, read fresh each frame -- nothing with neither; +2
// with the composite bow taken up (Q9 on its default); +1 dark radius with
// Night Eyes taken (Q8 on its default) -- and fogEyes puts them on s:1's eye.
//
// Negative control (2 Oct 2026, strigoi-harness-runs\wt-fog4\nc\): read the
// talent by the wrong key (d2progress.AdvantageBonus) and this fails, "with
// Night Eyes his dark talent is +10" (nc-wrong-talent-key.txt).
func TestHisSightTermsAreHisKitAndTalents(t *testing.T) {
	cat, err := d2items.Load(readData(t, "items.json"))
	if err != nil {
		t.Fatal(err)
	}

	kit, err := cat.NewKit("torch-and-blade")
	if err != nil {
		t.Fatal(err)
	}

	tree, err := d2progress.Load(readData(t, "talents.json"))
	if err != nil {
		t.Fatal(err)
	}

	v := &Game{kit: kit, progress: &d2progress.Progress{}, talents: tree}

	if gear, dark := v.heroSightTerms(); gear != 0 || dark != 0 {
		t.Fatalf("a fresh hero's sight terms are gear %v, dark %v; want none", gear, dark)
	}

	bow := -1

	for i := range kit.Pack {
		if kit.Pack[i].Item == "composite-bow" {
			bow = i
		}
	}

	if err := kit.Equip(bow, false, false); err != nil {
		t.Fatalf("taking up the bow: %v", err)
	}

	v.progress.Talents = append(v.progress.Talents, "night-eyes")

	gear, dark := v.heroSightTerms()
	if gear != 2 {
		t.Fatalf("with the bow in his hands his gear is +%v; Q9 is +2", gear)
	}

	if dark != 1 {
		t.Fatalf("with Night Eyes his dark talent is +%v; Q8 is +1", dark)
	}

	// No kit, no progress: nothing, and no panic.
	if gear, dark := (&Game{}).heroSightTerms(); gear != 0 || dark != 0 {
		t.Fatalf("a game with no kit or talents gives gear %v, dark %v", gear, dark)
	}
}
