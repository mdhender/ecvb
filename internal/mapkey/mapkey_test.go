// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package mapkey_test

import (
	"slices"
	"testing"

	"github.com/mdhender/ecvb/internal/mapkey"
	"github.com/mdhender/ecvb/internal/prng"
)

func TestSequenceRoundTrips(t *testing.T) {
	for want, letter := range []string{"A", "B", "C", "D", "E"} {
		sequence, err := mapkey.Sequence(letter)
		if err != nil {
			t.Fatalf("Sequence(%q): %v", letter, err)
		}
		if got := int(sequence); got != want+1 {
			t.Errorf("Sequence(%q) = %d; want %d", letter, got, want+1)
		}
		back, err := mapkey.Letter(sequence)
		if err != nil {
			t.Fatalf("Letter(%d): %v", sequence, err)
		}
		if back != letter {
			t.Errorf("Letter(Sequence(%q)) = %q", letter, back)
		}
	}
}

func TestSequenceRejectsWhatIsNotALetter(t *testing.T) {
	for _, letter := range []string{"", "F", "a", "AA", "1", " A"} {
		if _, err := mapkey.Sequence(letter); err == nil {
			t.Errorf("Sequence(%q) was accepted", letter)
		}
	}
	for _, sequence := range []prng.Key{-1, 0, 6, 100} {
		if _, err := mapkey.Letter(sequence); err == nil {
			t.Errorf("Letter(%d) was accepted", sequence)
		}
	}
}

// The shapes are a frozen surface, so they are spelled out here rather than
// computed: a test that rebuilt them the way the code does could not catch the
// code changing.
func TestAddressShapesAreTheOnesTheRegistryReserves(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  []prng.Key
		want []prng.Key
	}{
		{"stellium", mapkey.Stellium(3, -7, 12), []prng.Key{2, 3, -7, 12}},
		{"system", mapkey.System(3, -7, 12, 1), []prng.Key{3, 3, -7, 12, 1}},
		{"planet", mapkey.Planet(3, -7, 12, 1, 5), []prng.Key{4, 3, -7, 12, 1, 5}},
		{"deposit", mapkey.Deposit(3, -7, 12, 1, 5, 2), []prng.Key{5, 3, -7, 12, 1, 5, 2}},
	} {
		if !slices.Equal(tc.got, tc.want) {
			t.Errorf("%s address = %v; want %v (frozen surface changed?)", tc.name, tc.got, tc.want)
		}
	}
}

// An axis may be zero -- (0, 5, 3) is a real place -- and prng panics on a path
// whose first element is zero. Leading with the tag is what keeps that from
// being a crash, so it is worth a test of its own.
func TestAZeroCoordinateIsAddressable(t *testing.T) {
	seeds := prng.New(19, 12)
	for _, address := range [][]prng.Key{
		mapkey.Stellium(0, 5, 3),
		mapkey.Stellium(0, 0, 0),
		mapkey.System(-3, 6, 0, 1),
		mapkey.Planet(0, 0, 0, 1, 1),
		mapkey.Deposit(0, 0, 0, 1, 1, 1),
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Stream(%v) panicked: %v", address, r)
				}
			}()
			seeds.Stream(address...).Uint64()
		}()
	}
}

// Every address must name one thing and no other.
func TestAddressesAreDistinct(t *testing.T) {
	seeds := prng.New(19, 12)
	seen := make(map[uint64][]prng.Key)
	add := func(address []prng.Key) {
		t.Helper()
		draw := seeds.Stream(address...).Uint64()
		if other, ok := seen[draw]; ok {
			t.Errorf("%v and %v share a stream", address, other)
		}
		seen[draw] = address
	}
	for x := -1; x <= 1; x++ {
		for y := -1; y <= 1; y++ {
			add(mapkey.Stellium(x, y, 0))
			for sequence := prng.Key(1); sequence <= 2; sequence++ {
				add(mapkey.System(x, y, 0, sequence))
				for orbit := 1; orbit <= 2; orbit++ {
					add(mapkey.Planet(x, y, 0, sequence, orbit))
					add(mapkey.Deposit(x, y, 0, sequence, orbit, 1))
				}
			}
		}
	}
}
