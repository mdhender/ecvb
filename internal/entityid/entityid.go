// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package entityid turns a game's entity ordinal into the number the player
// sees. It exists because those two things must not be the same number.
//
// # Why not the ordinal
//
// An entity id has to be three things at once. It must be unique within the
// game and never reused, because it is a handle: a player writes `ship 482137`
// and a draw about that ship is addressed by it. It must be deterministic, so
// the same seeds rebuild the same game and a replay compares byte for byte
// against a golden. And it must say nothing about how many entities the game
// has made, because a player who can read a count off an opponent's ship id
// can count the opponent's fleet.
//
// The ordinal itself — first entity 0, second 1 — has the first two and fails
// the third outright. A monotonic id with a random gap between one and the next
// keeps determinism and uniqueness but not secrecy: the magnitude of a number
// still estimates the count.
//
// # The permutation
//
// [Number] is a keyed pseudorandom permutation of the ordinal. Being a
// permutation is what makes uniqueness structural: distinct ordinals give
// distinct numbers by construction, with no collision check and so no retry to
// make the result depend on the order rows were written. Being keyed by the
// game's own seeds is what makes it deterministic and, without those seeds,
// not invertible — so a number carries no information about the ordinal behind
// it.
//
// It is a four-round Feistel network over twenty bits, ten bits to a half, with
// [prng] supplying the round function; the tag is [prng.TagEntityNumber]. Twenty
// bits is 1048576 values, of which the 900000 six-digit ones are accepted, so
// the construction is closed by cycle walking: apply the network until the
// result lands in range. Cycle walking over a subset that is 86% of the domain
// terminates in about 1.2 rounds on average, and the network restricted to the
// accepted set is a permutation of it, which is what keeps the whole map a
// bijection.
//
// # Frozen surface
//
// The round count, the split, the domain, the accepted range, and the key path
// handed to [prng] are all part of the game's determinism the moment any game
// exists. Changing any of them renumbers every entity of every live game. The
// golden vectors in prng/testdata pin the round function; entityid_test.go pins
// the numbers themselves.
package entityid

import (
	"fmt"

	"github.com/mdhender/ecvb/internal/prng"
)

// The accepted range. Every entity number is six digits, so a number is
// recognizable as one on sight and the report columns are a fixed width.
const (
	MinNumber = 100_000
	MaxNumber = 999_999

	// Capacity is how many entities one game may ever create. The permutation
	// is a bijection onto the accepted range, so the range size is the limit.
	Capacity = MaxNumber - MinNumber + 1
)

// The Feistel parameters. domainBits must be wide enough to hold MaxNumber and
// even, so the two halves are the same width.
const (
	halfBits   = 10
	halfMask   = 1<<halfBits - 1
	domainSize = 1 << (2 * halfBits)
	rounds     = 4
)

// Number returns the public number of the ordinal-th entity of a game, counting
// from zero. Distinct ordinals give distinct numbers, and every number is in
// [MinNumber, MaxNumber].
//
// An ordinal at or past [Capacity] is an error rather than a wrap: the game has
// run out of numbers, and reusing one would break the handle every stored order
// and every prng address is written against.
func Number(seeds prng.Seeds, ordinal int64) (int64, error) {
	if ordinal < 0 {
		return 0, fmt.Errorf("entity ordinal %d is negative", ordinal)
	}
	if ordinal >= Capacity {
		return 0, fmt.Errorf("entity ordinal %d is past the %d numbers a game has", ordinal, Capacity)
	}
	// Seat the ordinal in the accepted range first, so that what the cycle walk
	// permutes is exactly the set it walks back into.
	value := uint32(MinNumber + ordinal)
	for {
		value = feistel(seeds, value)
		if MinNumber <= value && value <= MaxNumber {
			return int64(value), nil
		}
	}
}

// feistel is the permutation of [0, domainSize) that Number walks. Four rounds
// is the standard minimum for a strong pseudorandom permutation, and the cost
// here is four hashes per entity created rather than per draw made.
func feistel(seeds prng.Seeds, value uint32) uint32 {
	left, right := value>>halfBits, value&halfMask
	for round := range rounds {
		left, right = right, left^roundFunction(seeds, round, right)
	}
	return left<<halfBits | right
}

// roundFunction is the Feistel round's mixing function: any function of the
// round and the right half will do, because a Feistel network is a permutation
// whatever it returns. Addressing it through prng is what ties it to the game's
// seeds and keeps it identical on every machine.
func roundFunction(seeds prng.Seeds, round int, right uint32) uint32 {
	return uint32(seeds.Stream(prng.TagEntityNumber, prng.Key(round), prng.Key(right)).Uint64()) & halfMask
}
