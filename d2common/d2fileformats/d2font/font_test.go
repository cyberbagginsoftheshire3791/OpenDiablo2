package d2font

import (
	"testing"

	testify "github.com/stretchr/testify/assert"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2datautils"
)

// The font bytes are assembled in-code with the same stream primitives the
// decoder reads with, so the test carries no Blizzard-derived data (Article V)
// and does not depend on the uint16 byte order.

type glyphSpec struct {
	code          rune
	width, height int
	frame         int
}

func buildFont(glyphs []glyphSpec) []byte {
	sw := d2datautils.CreateStreamWriter()

	sw.PushBytes([]byte(knownSignature)...) // "Woo!\x01" (5 bytes)
	sw.PushBytes(make([]byte, unknownHeaderBytesCount)...)

	for _, g := range glyphs {
		sw.PushUint16(uint16(g.code))
		sw.PushBytes(0)                // unknown1
		sw.PushBytes(byte(g.width))    //
		sw.PushBytes(byte(g.height))   //
		sw.PushBytes(1, 0, 0)          // unknown2
		sw.PushUint16(uint16(g.frame)) //
		sw.PushBytes(0, 0, 0, 0)       // unknown3
	}

	return sw.GetBytes()
}

func TestLoad_ParsesGlyphs(t *testing.T) {
	assert := testify.New(t)

	specs := []glyphSpec{
		{code: 'A', width: 7, height: 12, frame: 0},
		{code: 'b', width: 5, height: 10, frame: 3},
		{code: '1', width: 6, height: 11, frame: 42},
	}

	font, err := Load(buildFont(specs))
	assert.NoError(err)
	assert.Len(font.Glyphs, len(specs))

	for _, s := range specs {
		g, ok := font.Glyphs[s.code]
		assert.Truef(ok, "glyph %q should be present", s.code)
		assert.Equalf(s.width, g.Width(), "width of %q", s.code)
		assert.Equalf(s.height, g.Height(), "height of %q", s.code)
		assert.Equalf(s.frame, g.FrameIndex(), "frame of %q", s.code)
	}
}

func TestLoad_RejectsBadSignature(t *testing.T) {
	assert := testify.New(t)

	_, err := Load([]byte("NOPE!\x00\x00\x00\x00\x00\x00\x00"))
	assert.Error(err)
}

// Marshal writes the numHeaderBytes header Load reads back, so a full
// Marshal->Load round-trip preserves every glyph. Regression guard for the
// former 13-vs-12-byte header mismatch (Marshal used to be one byte too long,
// which shifted every glyph).
func TestMarshalLoad_RoundTrip(t *testing.T) {
	assert := testify.New(t)

	specs := []glyphSpec{
		{code: 'A', width: 7, height: 12, frame: 0},
		{code: 'b', width: 5, height: 10, frame: 3},
		{code: '1', width: 6, height: 11, frame: 42},
	}

	font, err := Load(buildFont(specs))
	assert.NoError(err)

	reloaded, err := Load(font.Marshal())
	assert.NoError(err)
	assert.Len(reloaded.Glyphs, len(specs))

	for _, s := range specs {
		g, ok := reloaded.Glyphs[s.code]
		assert.Truef(ok, "glyph %q should survive the round-trip", s.code)
		assert.Equalf(s.width, g.Width(), "width of %q", s.code)
		assert.Equalf(s.height, g.Height(), "height of %q", s.code)
		assert.Equalf(s.frame, g.FrameIndex(), "frame of %q", s.code)
	}
}
