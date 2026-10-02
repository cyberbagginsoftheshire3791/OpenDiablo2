package d2world

import (
	"encoding/base64"
	"fmt"
	"math/bits"
)

// FOG OF WAR F3, "KEPT" (1 Oct 2026; claude/fog-of-war-build-plan.md §4 F3
// and §3.7): the explored grid is saved, so the ground he has seen is still
// remembered after a load.
//
// WHAT IS SAVED AND WHAT IS NOT (D2, 28 Sep 2026: a value derived from dials,
// or from other saved state, is never saved):
//   - SAVED: the explored grid -- every tile he has ever seen -- its size, and
//     the map it was explored on.
//   - NOT SAVED: what he sees NOW (visible): it is recomputed from his eyes on
//     the first frame after a load, as every frame does. The dials (day sight,
//     the dark radii, the look): this build's tuning, never the file's. The
//     cost counters and the probe: this process's.
//
// THE GRID IS KEYED ON THE MAP (the F1 review's C6): a grid of the right size
// explored on another map is not this map's ground. A block whose map is not
// the game's is refused, as D5 refuses a world file saved on another map.
//
// AN EMPTY BLOCK (w = h = 0, no map, no grid) is a fog that has seen nothing:
// a game with fog off (it never looks), or one saved before its first look.
// Restored, the fog starts black, as a new game does.

// fogMaxSide is the largest grid side a snapshot may claim, in tiles: far
// past any map this project builds (the village is 48), and small enough that
// a file cannot make a load allocate without bound.
const fogMaxSide = 4096

// FogSnapshot is fog's saved state (F3): the explored grid of one map.
//
// Explored is the grid as bits, row-major, tile (x, y) at bit y*W+x, eight
// tiles to a byte with the lowest bit first; the bits past W*H in the last
// byte are zero. Base64 (standard, padded): the village's 48 x 48 is 288
// bytes, 384 characters.
//
// Map is the map the grid was explored on: the authored map's SHA-256 (the
// world file's map.sha). Diablo II's generated Act 1 has none -- its identity
// is the seed, which the world file's own checks hold -- and says "".
type FogSnapshot struct {
	Map      string `json:"map"`
	W        int    `json:"w"`
	H        int    `json:"h"`
	Explored string `json:"explored"`
}

// Empty is whether the snapshot is of a fog that has seen nothing on no map.
func (s FogSnapshot) Empty() bool { return s.W == 0 && s.H == 0 }

// Check is every refusal a fog snapshot can make on its own, with no map to
// hand: a size that is none or a real grid, and a grid of exactly that size,
// written as Snapshot writes it (canonical base64, its padding bits zero). It
// is what the world file's own check (d2save) and Validate both ask.
func (s FogSnapshot) Check() error {
	switch {
	case s.W < 0 || s.H < 0:
		return fmt.Errorf("fog snapshot: a grid of %d x %d tiles", s.W, s.H)
	case s.Empty():
		if s.Map != "" || s.Explored != "" {
			return fmt.Errorf("fog snapshot: an empty grid (0 x 0) names map %q and holds %d characters; an empty grid has neither",
				s.Map, len(s.Explored))
		}

		return nil
	case s.W == 0 || s.H == 0:
		return fmt.Errorf("fog snapshot: a grid of %d x %d tiles; a grid is empty (0 x 0) or has both sides", s.W, s.H)
	case s.W > fogMaxSide || s.H > fogMaxSide:
		return fmt.Errorf("fog snapshot: a grid of %d x %d tiles is past the %d-tile side any map has", s.W, s.H, fogMaxSide)
	}

	_, err := fogDecode(s.Explored, s.W*s.H)

	return err
}

// fogDecode is the explored bits of n tiles from their base64, refusing
// anything Snapshot would not have written.
func fogDecode(text string, n int) ([]byte, error) {
	raw, err := base64.StdEncoding.Strict().DecodeString(text)
	if err != nil {
		return nil, fmt.Errorf("fog snapshot: the explored grid is not base64: %v", err)
	}

	// The decoder skips line breaks; Snapshot writes none. Only the one
	// spelling Snapshot writes is taken.
	if base64.StdEncoding.EncodeToString(raw) != text {
		return nil, fmt.Errorf("fog snapshot: the explored grid is not written as a save writes it (one line of padded base64)")
	}

	if want := (n + 7) / 8; len(raw) != want {
		return nil, fmt.Errorf("fog snapshot: the explored grid is %d bytes; %d tiles are %d", len(raw), n, want)
	}

	if extra := n % 8; extra != 0 && raw[len(raw)-1]>>uint(extra) != 0 {
		return nil, fmt.Errorf("fog snapshot: the explored grid sets bits past its last tile")
	}

	return raw, nil
}

// Snapshot is the explored grid as it stands, on the map mapID (the world
// file's map.sha). A fog that has never been sized -- fog off, or no look yet
// -- is the empty snapshot, whatever mapID says.
func (f *Fog) Snapshot(mapID string) FogSnapshot {
	if f.w <= 0 || f.h <= 0 {
		return FogSnapshot{}
	}

	n := f.w * f.h
	raw := make([]byte, (n+7)/8)

	for i := 0; i < n; i++ {
		if f.explored[i/64]&(uint64(1)<<(uint(i)%64)) != 0 {
			raw[i/8] |= 1 << (uint(i) % 8)
		}
	}

	return FogSnapshot{Map: mapID, W: f.w, H: f.h, Explored: base64.StdEncoding.EncodeToString(raw)}
}

// Validate is Restore's check and nothing else (D4): the snapshot is one
// Snapshot could have written (Check), and -- unless it is empty -- of a grid
// of the map the game runs (w x h tiles, map mapID). A grid explored on
// another map, or of another size, is refused (the F1 review's C6; D5).
func (f *Fog) Validate(s FogSnapshot, w, h int, mapID string) error {
	if err := s.Check(); err != nil {
		return err
	}

	switch {
	case s.Empty():
		return nil
	case s.Map != mapID:
		return fmt.Errorf("fog snapshot: the grid was explored on map %q and this game runs map %q (D5)", s.Map, mapID)
	case s.W != w || s.H != h:
		return fmt.Errorf("fog snapshot: the grid is %d x %d tiles and this map is %d x %d", s.W, s.H, w, h)
	}

	return nil
}

// Restore puts the explored grid back (after Validate; a refused snapshot
// leaves the fog as it was). The visible set is not saved: nothing is visible
// until the next Update, which recomputes -- it is marked to -- and sees what
// the eyes see from where they stand. An empty snapshot leaves the fog
// unsized and unexplored: the next Update sizes it to the map, black. The
// dials and the counters are untouched.
func (f *Fog) Restore(s FogSnapshot, w, h int, mapID string) error {
	if err := f.Validate(s, w, h, mapID); err != nil {
		return err
	}

	if s.Empty() {
		f.resize(0, 0)

		return nil
	}

	raw, err := fogDecode(s.Explored, s.W*s.H)
	if err != nil {
		return err // Validate took it; this cannot happen
	}

	f.resize(s.W, s.H)

	count := 0

	for i, b := range raw {
		for b != 0 {
			j := bits.TrailingZeros8(b)
			b &^= 1 << uint(j)

			t := i*8 + j
			f.explored[t/64] |= uint64(1) << (uint(t) % 64)
			count++
		}
	}

	f.exploredCount = count
	f.dirty = true

	return nil
}
