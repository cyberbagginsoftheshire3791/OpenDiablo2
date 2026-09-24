package d2asset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2loader"
)

func TestSoundOverridePath(t *testing.T) {
	assert.Equal(t, "/data/strigoi/override/data/global/sfx/cursor/button.wav",
		SoundOverridePath(`\data\global\SFX\Cursor\button.WAV`))
	assert.Equal(t, "/data/strigoi/override/data/global/music/act1/town1.wav",
		SoundOverridePath("data/global/music/act1/town1.wav"))
	assert.Empty(t, SoundOverridePath("/data/global/ui/cursor/ohand.dc6"), "a sprite is the sprites' override")
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
}

// A Strigoi .wav at the override path is what a sound loads: INSTEAD of
// Diablo II's where both are there, and where Diablo II's is not (a friend
// without the MPQs) -- asked for as the game asks, backslashes and case and
// all -- and the census counts it as ours.
func TestASoundOverrideIsPlayedInstead(t *testing.T) {
	dir, err := os.MkdirTemp("", "sounds")
	require.NoError(t, err)

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	writeFile(t, filepath.Join(dir, "data", "global", "sfx", "cursor", "button.wav"), "RIFF diablo")
	writeFile(t, filepath.Join(dir, "data", "strigoi", "override", "data", "global", "sfx", "cursor", "button.wav"), "RIFF strigoi")
	writeFile(t, filepath.Join(dir, "data", "strigoi", "override", "data", "global", "music", "act1", "town1.wav"), "RIFF town")

	am := fontAssets(t, dir)

	// Both there: Strigoi's is played.
	data, err := am.LoadFile(`data\global\SFX\Cursor\Button.wav`)
	require.NoError(t, err)
	assert.Equal(t, "RIFF strigoi", string(data))

	// Only Strigoi's there: it exists, and loads.
	exists, err := am.FileExists(`data\global\Music\Act1\town1.wav`)
	require.NoError(t, err)
	assert.True(t, exists, "the override stands in for a sound Diablo II's MPQs would have")

	data, err = am.LoadFile("data/global/music/act1/town1.wav")
	require.NoError(t, err)
	assert.Equal(t, "RIFF town", string(data))

	// Neither: nothing is invented.
	exists, _ = am.FileExists("data/global/sfx/cursor/select.wav")
	assert.False(t, exists)

	// The census: both loads are ours.
	native := 0

	for _, e := range am.Loader.Census.Entries() {
		if strings.Contains(e.Path, "/strigoi/override/") {
			assert.Equal(t, d2loader.CensusNative, e.Source, e.Path)
			native++
		}
	}

	assert.Equal(t, 2, native, "each override load is on the census as ours")
}
