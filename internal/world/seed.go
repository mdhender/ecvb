// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package world

import "math/rand/v2"

// Seed is a game's random seed. Everything the rules decide at random is drawn
// from it and from the identity of the order being resolved, so re-resolving a
// turn reaches the same answers.
type Seed struct {
	High, Low int64
}

// Rings a ship may settle into around a planet. Ring 0 is the surface and ring
// 1 belongs to orbital colonies, so a ship arriving under its own power takes
// one of the rings above them.
const (
	MinShipRing = 2
	MaxShipRing = 99
)

// RingFor picks the ring a ship settles into when it arrives at a planet. The
// draw is seeded from the game and the identity of the order rather than from
// a global source, so the same turn resolved twice puts the ship in the same
// ring, and no two orders share a stream.
func (s Seed) RingFor(turn int, factionID int64, sequence int) int {
	rng := rand.New(rand.NewPCG(
		mix(uint64(s.High), uint64(turn), uint64(factionID)),
		mix(uint64(s.Low), uint64(sequence), 0x9e3779b97f4a7c15),
	))
	return MinShipRing + rng.IntN(MaxShipRing-MinShipRing+1)
}

// mix folds several values into one well-distributed seed word using the
// SplitMix64 finalizer, so that seeds differing in one small field still start
// unrelated streams.
func mix(values ...uint64) uint64 {
	var state uint64
	for _, value := range values {
		state += value + 0x9e3779b97f4a7c15
		state ^= state >> 30
		state *= 0xbf58476d1ce4e5b9
		state ^= state >> 27
		state *= 0x94d049bb133111eb
		state ^= state >> 31
	}
	return state
}
