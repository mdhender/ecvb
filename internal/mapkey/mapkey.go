// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package mapkey is how the map is named in an [prng] key path.
//
// A draw about a map object is addressed by what is intrinsic to it -- where it
// is -- and never by a row id, which depends only on the order rows happened to
// be written. The shapes here are the ones [prng]'s tag registry reserves, and
// they are a frozen surface for the same reason the registry is: once a game
// exists, its outcomes are welded to them.
//
// This package exists because two callers need the same addresses and cannot
// share them any other way. cmd/ecgen builds a map and never opens a database;
// internal/world draws against one and does nothing else. Putting the A=1 rule
// in either would either link SQLite into the generator or duplicate a frozen
// surface, which is the one thing tags.go warns against. It goes here instead,
// depending on nothing but [prng].
package mapkey

import (
	"fmt"

	"github.com/mdhender/ecvb/internal/prng"
)

// MinCoordinate and MaxCoordinate bound every axis of a stellium's position.
const (
	MinCoordinate = -15
	MaxCoordinate = 15
)

// MinOrbit and MaxOrbit bound a planet's orbit within its system.
const (
	MinOrbit = 1
	MaxOrbit = 10
)

// Sequence converts a system's sequence letter to the Key that stands for it in
// an address: A is 1 through E is 5. It is the one place that rule exists.
//
// The letter is what the schema stores and what a player reads; the number is
// what a key path carries, because a path element is an int64. Changing the
// mapping would move every draw about every system, planet, and deposit in
// every game, so it does not change.
func Sequence(letter string) (prng.Key, error) {
	if len(letter) != 1 || letter[0] < 'A' || letter[0] > 'E' {
		return 0, fmt.Errorf("system sequence %q is not a letter A through E", letter)
	}
	return prng.Key(letter[0]-'A') + 1, nil
}

// Letter is the inverse of [Sequence].
func Letter(sequence prng.Key) (string, error) {
	if sequence < 1 || sequence > 5 {
		return "", fmt.Errorf("system sequence %d is not in 1 through 5", sequence)
	}
	return string(rune('A' + sequence - 1)), nil
}

// Stellium returns the address of a stellium: its coordinates, behind the tag.
//
// The tag leads every path here, and not only for domain separation: an axis may
// legitimately be zero -- (0, 5, 3) is a real place -- and prng rejects a path
// whose first element is zero. Leading with the tag means a coordinate never has
// to be.
func Stellium(x, y, z int) []prng.Key {
	return []prng.Key{prng.TagStellium, prng.Key(x), prng.Key(y), prng.Key(z)}
}

// System returns the address of a system: its stellium's coordinates and its
// own sequence, from [Sequence].
func System(x, y, z int, sequence prng.Key) []prng.Key {
	return []prng.Key{prng.TagSystem, prng.Key(x), prng.Key(y), prng.Key(z), sequence}
}

// Planet returns the address of a planet: its system's address and its orbit.
func Planet(x, y, z int, sequence prng.Key, orbit int) []prng.Key {
	return []prng.Key{prng.TagPlanet, prng.Key(x), prng.Key(y), prng.Key(z), sequence, prng.Key(orbit)}
}

// Deposit returns the address of a deposit: its planet's address and its number
// on that planet, which is assigned at generation and never reused.
func Deposit(x, y, z int, sequence prng.Key, orbit, number int) []prng.Key {
	return []prng.Key{prng.TagDeposit, prng.Key(x), prng.Key(y), prng.Key(z), sequence, prng.Key(orbit), prng.Key(number)}
}
