package d2dcc

import (
	"encoding/binary"
	"testing"

	testify "github.com/stretchr/testify/assert"
)

// These cover the DCC header and its error paths. The bytes are assembled
// in-code (no Blizzard-derived data, Article V). BitMuncher reads LSB-first,
// so a byte-aligned header is plain little-endian.
//
// NOT covered here: decoding an actual DCC direction. That is a variable
// bit-width pixel bitstream (see dcc_direction.go) whose faithful synthesis
// is a separate, larger job; a zero-direction file exercises only the header.

func dccHeader(signature byte, magic int32) []byte {
	buf := make([]byte, 15)
	buf[0] = signature
	buf[1] = 6                                              // version
	buf[2] = 0                                              // numberOfDirections
	binary.LittleEndian.PutUint32(buf[3:7], 0)              // FramesPerDirection
	binary.LittleEndian.PutUint32(buf[7:11], uint32(magic)) // must be 1
	binary.LittleEndian.PutUint32(buf[11:15], 0)            // TotalSizeCoded

	return buf
}

func TestLoad_MinimalZeroDirectionHeader(t *testing.T) {
	assert := testify.New(t)

	dcc, err := Load(dccHeader(dccFileSignature, 1))
	assert.NoError(err)
	assert.Equal(dccFileSignature, dcc.Signature)
	assert.Equal(0, dcc.NumberOfDirections)
	assert.Empty(dcc.Directions)
}

func TestLoad_RejectsBadSignature(t *testing.T) {
	assert := testify.New(t)

	_, err := Load([]byte{0x00})
	assert.Error(err)
}

func TestLoad_RejectsNonOneMagicField(t *testing.T) {
	assert := testify.New(t)

	// Same valid header but the field the format requires to be 1 is not.
	_, err := Load(dccHeader(dccFileSignature, 2))
	assert.Error(err)
}
