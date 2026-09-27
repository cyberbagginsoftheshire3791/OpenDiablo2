package d2audio

import (
	"math/rand"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2records"
)

const assumedFPS = 25

// SoundEnvironment represents the audio environment for map areas
type SoundEnvironment struct {
	environment *d2records.SoundEnvironRecord
	engine      *SoundEngine
	bgm         *Sound
	ambiance    *Sound
	eventTimer  float64
}

// NewSoundEnvironment creates a SoundEnvironment using the given SoundEngine
func NewSoundEnvironment(soundEngine *SoundEngine) SoundEnvironment {
	r := SoundEnvironment{
		// Start with env NONE
		environment: soundEngine.asset.Records.Sound.Environment[0],
		engine:      soundEngine,
	}

	return r
}

// SetEnv sets the sound environment using the given record index
func (s *SoundEnvironment) SetEnv(environmentIdx int) {
	if s.environment.Index != environmentIdx {
		newEnv := s.engine.asset.Records.Sound.Environment[environmentIdx]

		if s.environment.Song != newEnv.Song {
			if s.bgm != nil {
				s.bgm.Stop()
			}

			s.bgm = s.engine.PlaySoundID(newEnv.Song)
		}

		if s.environment.DayAmbience != newEnv.DayAmbience {
			if s.ambiance != nil {
				s.ambiance.Stop()
			}

			s.ambiance = s.engine.PlaySoundID(newEnv.DayAmbience)
		}

		s.environment = newEnv
	}
}

// Current reports the sound environment playing -- its soundenviron.txt row
// -- and the file of the song it started ("" when none plays). Read-only, for
// the harness: the village's environment comes from its map (the tables
// burst's review, B4, 27 Sep 2026: nothing asserted the game fed it in).
func (s *SoundEnvironment) Current() (env int, music string) {
	if s.environment != nil {
		env = s.environment.Index
	}

	if s.bgm != nil && s.bgm.entry != nil {
		music = s.bgm.entry.FileName
	}

	return env, music
}

// Advance advances the sound engine and plays sounds when necessary
func (s *SoundEnvironment) Advance(elapsed float64) {
	s.eventTimer -= elapsed
	if s.eventTimer < 0 {
		s.eventTimer = float64(s.environment.EventDelay) / assumedFPS

		snd := s.engine.PlaySoundID(s.environment.DayEvent)
		if snd != nil {
			// nolint:gosec,gomnd // client-side, no big deal if rand number isn't securely generated
			pan := (rand.Float64() * 2) - 1
			snd.SetPan(pan)
		}
	}
}
