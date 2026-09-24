package d2asset

import (
	"fmt"
	slashpath "path"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
)

// STRIGOI'S ART IN PLACE OF DIABLO II'S, FILE BY FILE (M5.3). Every sprite the
// game still draws from the MPQs -- the cursor, the buttons, the panels, the
// orbs -- is asked for by its Diablo II path (/data/global/ui/CURSOR/ohand.DC6).
// A PNG at the same path under data/strigoi/override, lower-cased, with .png
// for .dc6/.dcc (data/strigoi/override/data/global/ui/cursor/ohand.png), is
// drawn instead, with its .png.json sheet manifest beside it when it has more
// than one frame (the creatures' format: rows are directions, columns
// frames, one cell size). No code changes per sprite: the art drops in by
// name. DescribeSprite, and the harness's strigoi_describe_sprite, say what
// a sprite is -- directions, frames, each frame's size -- so it can be drawn
// to fit.

// spriteOverrideRoot is where Strigoi's replacements for Diablo II's sprites live.
const spriteOverrideRoot = "/data/strigoi/override"

// SpriteOverridePath is where a PNG standing in for the Diablo II sprite at p
// would live -- whether or not one does -- or "" for anything that is not a
// .dc6 or .dcc. The loader's language tokens ({LANG}, {LANG_FONT}) are kept.
func SpriteOverridePath(p string) string {
	s := strings.ReplaceAll(strings.TrimSpace(p), `\`, "/")
	ext := strings.ToLower(slashpath.Ext(s))

	if ext != ".dc6" && ext != ".dcc" {
		return ""
	}

	s = strings.ToLower(strings.TrimSuffix(s, slashpath.Ext(s)))
	s = strings.ReplaceAll(s, strings.ToLower(d2resource.LanguageFontToken), d2resource.LanguageFontToken)
	s = strings.ReplaceAll(s, strings.ToLower(d2resource.LanguageTableToken), d2resource.LanguageTableToken)

	return spriteOverrideRoot + slashpath.Clean("/"+s) + ".png"
}

// spriteOverride is the Strigoi PNG that stands in for the sprite at p, or "".
func (am *AssetManager) spriteOverride(p string) string {
	candidate := SpriteOverridePath(p)
	if candidate == "" {
		return ""
	}

	if exists, err := am.FileExists(candidate); err == nil && exists {
		return candidate
	}

	return ""
}

// SpriteInfo is what a sprite is, as drawn: which file, and its cells.
type SpriteInfo struct {
	Path       string   `json:"path"`
	Override   string   `json:"override,omitempty"` // the Strigoi PNG drawn instead, if any
	Directions int      `json:"directions"`
	Frames     int      `json:"frames_per_direction"`
	Sizes      [][2]int `json:"frame_sizes"` // direction 0's frames, [w, h] each
	MaxW       int      `json:"max_w"`
	MaxH       int      `json:"max_h"`
}

// DescribeSprite loads the sprite at p (with palette for a Diablo II one;
// "" is the units palette) and reports its directions, frames and frame sizes.
func (am *AssetManager) DescribeSprite(p, palette string) (SpriteInfo, error) {
	if palette == "" {
		palette = d2resource.PaletteUnits
	}

	anim, err := am.LoadAnimation(p, palette)
	if err != nil {
		return SpriteInfo{}, fmt.Errorf("sprite %s: %w", p, err)
	}

	info := SpriteInfo{
		Path:       p,
		Override:   am.spriteOverride(p),
		Directions: anim.GetDirectionCount(),
		Frames:     anim.GetFrameCount(),
	}

	for i := 0; i < info.Frames; i++ {
		w, h, err := anim.GetFrameSize(i)
		if err != nil {
			return SpriteInfo{}, fmt.Errorf("sprite %s frame %d: %w", p, i, err)
		}

		info.Sizes = append(info.Sizes, [2]int{w, h})

		if w > info.MaxW {
			info.MaxW = w
		}

		if h > info.MaxH {
			info.MaxH = h
		}
	}

	return info, nil
}
