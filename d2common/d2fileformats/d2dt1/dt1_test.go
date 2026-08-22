package d2dt1

import (
	"testing"

	testify "github.com/stretchr/testify/assert"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2datautils"
)

// The DT1 bytes are produced by the package's own Marshal from an in-code DT1,
// so nothing Blizzard-derived enters the repo (Constitution, Article V). This
// exercises the header and per-tile header decode/encode. Block graphics
// decoding (RLE/isometric) is a separate, larger synthesis job and is not
// covered here.

func sampleTile(seed int32) Tile {
	return Tile{
		Direction:        seed,
		RoofHeight:       int16(seed + 1),
		MaterialFlags:    MaterialFlags{Dirt: true, Wood: true, Lava: seed%2 == 0},
		Height:           -160 + seed,
		Width:            160 + seed,
		Type:             seed + 2,
		Style:            seed + 3,
		Sequence:         seed + 4,
		RarityFrameIndex: seed + 5,
		// unknown2 must be numUnknownTileBytes2 long or Marshal/Load misalign.
		unknown2: []byte{byte(seed), 8, 7, 6},
		SubTileFlags: [25]SubTileFlags{
			0:  NewSubTileFlags(0x01),
			5:  NewSubTileFlags(0x2a),
			24: NewSubTileFlags(0xff),
		},
		blockHeaderPointer: 0,
		blockHeaderSize:    20 + seed,
		Blocks:             nil, // zero blocks: no block-offset arithmetic needed
	}
}

func TestMarshalLoad_RoundTrip(t *testing.T) {
	assert := testify.New(t)

	orig := &DT1{
		majorVersion: knownMajorVersion,
		minorVersion: knownMinorVersion,
		bodyPosition: 276, // header is 8 + 260 + 8 bytes; tiles follow immediately
	}
	orig.Tiles = []Tile{sampleTile(1), sampleTile(2)}
	orig.numberOfTiles = int32(len(orig.Tiles))

	got, err := LoadDT1(orig.Marshal())
	assert.NoError(err)
	assert.Len(got.Tiles, len(orig.Tiles))

	for i := range orig.Tiles {
		o, g := orig.Tiles[i], got.Tiles[i]
		assert.Equalf(o.Direction, g.Direction, "tile %d Direction", i)
		assert.Equalf(o.RoofHeight, g.RoofHeight, "tile %d RoofHeight", i)
		assert.Equalf(o.MaterialFlags, g.MaterialFlags, "tile %d MaterialFlags", i)
		assert.Equalf(o.Height, g.Height, "tile %d Height", i)
		assert.Equalf(o.Width, g.Width, "tile %d Width", i)
		assert.Equalf(o.Type, g.Type, "tile %d Type", i)
		assert.Equalf(o.Style, g.Style, "tile %d Style", i)
		assert.Equalf(o.Sequence, g.Sequence, "tile %d Sequence", i)
		assert.Equalf(o.RarityFrameIndex, g.RarityFrameIndex, "tile %d RarityFrameIndex", i)
		assert.Equalf(o.SubTileFlags, g.SubTileFlags, "tile %d SubTileFlags", i)
		assert.Equalf(o.unknown2, g.unknown2, "tile %d unknown2", i)
		assert.Emptyf(g.Blocks, "tile %d should have no blocks", i)
	}
}

func TestLoadDT1_RejectsWrongVersion(t *testing.T) {
	assert := testify.New(t)

	sw := d2datautils.CreateStreamWriter()
	sw.PushInt32(1) // major, not 7
	sw.PushInt32(0) // minor, not 6

	_, err := LoadDT1(sw.GetBytes())
	assert.Error(err)
}

func TestLoadDT1_RejectsTruncatedInput(t *testing.T) {
	assert := testify.New(t)

	_, err := LoadDT1([]byte{})
	assert.Error(err)
}
