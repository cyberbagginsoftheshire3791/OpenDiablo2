package d2rand

import "hash/fnv"

// The gameplay streams that seed from the game's seed through Derive (M4.6
// B2a, B1 review C6). The world stream -- the map engine's -- is not here: it
// keeps the game seed itself, so the map and the digest's world part do not
// move.
const (
	StreamSpawns = "spawns"
	StreamCombat = "combat"
	StreamRising = "rising"
)

// int32max is math/rand's seed modulus (rngSource.Seed): 2^31 - 1.
const int32max = (1 << 31) - 1

// zeroSeed is what math/rand's rngSource.Seed uses in place of a seed whose
// residue is zero.
const zeroSeed = 89482311

// Derive is the seed of one named stream of a game seeded with seed (M4.6 B2a,
// B1 review C6).
//
// THE PROBLEM IT CLOSES. Spawns, Combat and the world all seeded from the game
// seed, and the rising from seed+4707. Three streams with one seed hand out one
// sequence: the spawn roll's n-th draw was combat's n-th draw, and a restore
// that swapped two streams' positions could not be told from a right one.
//
// WHY IT WORKS IN math/rand's RESIDUE SPACE rather than hashing to an int64.
// rngSource.Seed reduces every seed modulo 2^31-1 (and replaces a zero residue
// with 89482311), so two int64 seeds that agree modulo it ARE the same stream.
// A hash to 64 bits would make two streams sharing an effective seed unlikely;
// this rules it out: the derived seed is the game's effective seed moved by a
// per-stream offset in [1, 2^31-3] around the ring of the 2^31-2 non-zero
// residues, so for ANY game seed each derived stream runs on an effective seed
// different from the world stream's and from every other named stream whose
// offset differs (TestDeriveStreamsNeverCoincide pins that the offsets of
// every name on Derived do). The offset is a fixed function of the name
// (FNV-1a), so it is the same on every build and platform.
//
// WHAT IS PROVED, AND WHAT IS ONLY CHECKED (softened at the B2a review, 28
// Sep 2026, from "impossible"). Proved: four different effective seeds, for
// every game seed. Checked, not proved: that different effective seeds hand
// out different values. The test compares each stream's first three values at
// sixteen game seeds; math/rand's seeding makes a shared prefix vanishingly
// unlikely, but nothing here rules it out for every seed, nor rules out one
// stream's sequence turning up later inside another's. A swapped restore is
// refused by the seed itself (StreamState.Check), which does not rest on this.
//
// A derived seed is in [1, 2^31-1]: it survives a float64 JSON reader exactly,
// though a reader should still take seed_str, because the world seed is the
// game's own and a wall-clock game seed does not (B1 notes, section 2).
func Derive(seed int64, stream string) int64 {
	return 1 + (Effective(seed)-1+streamOffset(stream))%(int32max-1)
}

// Effective is the seed math/rand's source actually runs on for seed: its
// residue modulo 2^31-1, with a zero residue replaced as rngSource.Seed
// replaces it. Two seeds with one Effective value are one stream.
func Effective(seed int64) int64 {
	r := seed % int32max
	if r < 0 {
		r += int32max
	}

	if r == 0 {
		r = zeroSeed
	}

	return r
}

// streamOffset is the name's fixed distance around the residue ring, in
// [1, 2^31-3] -- never 0, so a derived stream is never the game seed's own.
func streamOffset(stream string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(stream)) // hash.Hash.Write never errs

	return 1 + int64(h.Sum64()%uint64(int32max-2))
}
