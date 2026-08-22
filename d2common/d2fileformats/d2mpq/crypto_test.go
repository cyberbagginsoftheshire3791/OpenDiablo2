package d2mpq

import (
	"testing"

	testify "github.com/stretchr/testify/assert"
)

// These exercise the MPQ crypto primitives directly. They rely only on the
// algorithm and its well-known constants, so no Blizzard-derived bytes enter
// the repo (Constitution, Article V).

// The hash-table and block-table decryption keys are fixed constants of the
// MPQ format: hashString of their names with hash type 3 (the file key). These
// are the values StormLib and every MPQ reader agree on, and are exactly what
// readHashTable/readBlockTable pass to decryptTable.
func TestHashString_KnownTableKeys(t *testing.T) {
	assert := testify.New(t)

	assert.Equal(uint32(0xC3AF3770), hashString("(hash table)", 3))
	assert.Equal(uint32(0xEC83B3A3), hashString("(block table)", 3))
}

func TestHashString_DeterministicAndCaseInsensitive(t *testing.T) {
	assert := testify.New(t)

	// Same input, same output.
	assert.Equal(hashString("data\\global\\ui.dc6", 1), hashString("data\\global\\ui.dc6", 1))

	// hashString upper-cases its key, so case does not matter.
	assert.Equal(hashString("ABC", 1), hashString("abc", 1))

	// Different hash types of the same key give different results.
	assert.NotEqual(hashString("(hash table)", 1), hashString("(hash table)", 2))
}

func TestHashFilename_CombinesTypeAAndB(t *testing.T) {
	assert := testify.New(t)

	const key = "data\\global\\excel\\misc.txt"

	want := uint64(hashString(key, 1))<<32 | uint64(hashString(key, 2))
	assert.Equal(want, hashFilename(key))
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	assert := testify.New(t)

	key := hashString("(block table)", 3)

	orig := []uint32{1, 2, 3, 4, 5, 0xDEADBEEF, 0, 0xFFFFFFFF, 0x12345678}

	work := make([]uint32, len(orig))
	copy(work, orig)

	encrypt(work, key)
	assert.NotEqual(orig, work, "encryption should change the data")

	decrypt(work, key)
	assert.Equal(orig, work, "decrypt(encrypt(x)) should recover x")
}
