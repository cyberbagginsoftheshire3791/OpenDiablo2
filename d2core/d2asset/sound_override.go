package d2asset

import (
	slashpath "path"
	"strings"
)

// STRIGOI'S SOUND AND MUSIC IN PLACE OF DIABLO II'S (24 Sep 2026). The sprites'
// override (sprite_override.go) for the ears: a .wav at the same path under
// data/strigoi/override, lower-cased
// (/data/global/sfx/cursor/button.wav -> data/strigoi/override/data/global/sfx/cursor/button.wav),
// is played instead of Diablo II's, and counts as present even where Diablo
// II's is not -- a friend without the MPQs hears Strigoi's.

// SoundOverridePath is where a .wav standing in for the Diablo II sound at p
// would live -- whether or not one does -- or "" for anything that is not a
// .wav.
func SoundOverridePath(p string) string {
	s := strings.ReplaceAll(strings.TrimSpace(p), `\`, "/")
	if strings.ToLower(slashpath.Ext(s)) != ".wav" {
		return ""
	}

	return spriteOverrideRoot + slashpath.Clean("/"+strings.ToLower(s))
}

// soundOverride is the Strigoi .wav that stands in for the sound at p, or "".
func (am *AssetManager) soundOverride(p string) string {
	candidate := SoundOverridePath(p)
	if candidate == "" {
		return ""
	}

	// The path asked for, cleaned and lower-cased: one that is already an
	// override has none of its own.
	if asked := candidate[len(spriteOverrideRoot):]; strings.HasPrefix(asked, spriteOverrideRoot+"/") {
		return ""
	}

	if am.Loader.Exists(candidate) {
		return candidate
	}

	return ""
}
