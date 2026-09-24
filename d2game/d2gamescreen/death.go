package d2gamescreen

import (
	"errors"
	"os"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2player"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client/d2clientconnectiontype"
)

// Death screen v0 (23 Sep 2026). The game screen's half: noticing the death,
// putting his files back, and the two ways out.
//
// "LAST SAVE" IS THE MOMENT HE LAST ENTERED THE WORLD. His .od2 is written
// only on leaving alive (OnUnload's shouldSaveOnUnload, the 12 Sep ruling),
// so after a death it already holds that moment. His kit-and-progress file is
// written far more often -- on every equip, level and fight's end -- so on the
// frame he dies it is put back to the bytes it held when he entered. Doing it
// AT DEATH rather than on the way out means every way out agrees, including
// the window's close button, which skips OnUnload entirely.

// heroSnapshot is his kit-and-progress file as it was when he entered.
type heroSnapshot struct {
	taken  bool
	exists bool
	data   []byte
}

// snapshotHero records the file before anything this session writes it.
func (v *Game) snapshotHero() {
	v.heroAtEntry = heroSnapshot{taken: true}

	if v.kitPath == "" {
		return
	}

	data, err := os.ReadFile(v.kitPath)

	switch {
	case err == nil:
		v.heroAtEntry.exists, v.heroAtEntry.data = true, data
	case errors.Is(err, os.ErrNotExist):
		// A hero who has never chosen: rolling back removes whatever this
		// session wrote, and he chooses again.
	default:
		v.Errorf("death: cannot snapshot %s: %v", v.kitPath, err)
		v.heroAtEntry.taken = false
	}
}

// restoreHero puts his file back to the snapshot.
func (v *Game) restoreHero() {
	if !v.heroAtEntry.taken || v.kitPath == "" {
		return
	}

	if !v.heroAtEntry.exists {
		if err := os.Remove(v.kitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			v.Errorf("death: %v", err)
		}

		return
	}

	// The same locked-file-safe write the saves use (d2items.WriteFileAtomic).
	if err := d2items.WriteFileAtomic(v.kitPath, v.heroAtEntry.data); err != nil {
		v.Errorf("death: %v", err)
	}
}

// heroDead is the one death fact: his body at 0 (Stats.IsDead, the same field
// Meters.Dead and the resolver's player_dead read).
func (v *Game) heroDead() bool {
	return v.localPlayer != nil && v.localPlayer.Stats.IsDead()
}

// noticeDeath runs once a frame. On the first frame he is dead it records how,
// and rolls his file back.
func (v *Game) noticeDeath() {
	if v.died || !v.heroDead() {
		return
	}

	v.died = true

	// A talk does not outlive him, nor an open journal's hold on the world
	// (J1 review B8: the death screen does not hold the world).
	v.EndTalk()
	v.journalOpen = false
	v.death = d2player.Death{Cause: v.deathCause()}

	if v.worldClock != nil {
		year, month, day := v.worldClock.Date()
		v.death.Weekday, v.death.Year, v.death.Month, v.death.Day = v.worldClock.Weekday(), year, month, day
		v.death.Time = v.worldClock.TimeOfDay()
		v.death.Night = v.worldClock.Stage() == d2world.StageNight
	}

	v.restoreHero()
	v.Infof("death: %+v -- his file is back to how he entered", v.death)
}

// deathCause names what killed him: the fight that just ended on his death,
// or the meter that was taking his health.
func (v *Game) deathCause() string {
	switch {
	case v.combat != nil && v.combat.EndedReason() == "player_dead":
		return d2player.DeathCauseFight
	case v.meters != nil && v.meters.Parched():
		return d2player.DeathCauseThirst
	case v.meters != nil && v.meters.Starving():
		return d2player.DeathCauseHunger
	}

	return ""
}

// Death is the d2player.DeathHolder seam.
func (v *Game) Death() (bool, d2player.Death) { return v.died, v.death }

// LoadLastSave reopens his save: the .od2 as he last left the world alive, and
// the kit file as it was then (already restored at death).
func (v *Game) LoadLastSave() {
	if v.navigator == nil || v.gameClient == nil || v.gameClient.SaveFilePath == "" {
		v.QuitToMenu()
		return
	}

	// The App opens it once this game has gone (its server holds the port
	// until then). A navigator without that -- a test fake -- gets the
	// character select, where the same save is one click away.
	if r, ok := v.navigator.(gameReloader); ok {
		r.ReloadGame(v.gameClient.SaveFilePath)
		return
	}

	v.navigator.ToCharacterSelect(d2clientconnectiontype.Local, "")
}

// gameReloader is the App's deferred open (d2app.ReloadGame).
type gameReloader interface {
	ReloadGame(filePath string)
}

// QuitToMenu leaves. OnUnload saves nothing for a dead hero.
func (v *Game) QuitToMenu() {
	if v.navigator != nil {
		v.navigator.ToMainMenu()
	}
}
