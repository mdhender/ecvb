// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package entityid_test

import (
	"testing"

	"github.com/mdhender/ecvb/internal/entityid"
	"github.com/mdhender/ecvb/internal/prng"
)

// TestNumberIsABijection is the whole contract in one test: every ordinal a
// game can reach gives a six-digit number, and no two ordinals give the same
// one. Uniqueness is not checked at the database and not retried for, so this
// is what holds it.
func TestNumberIsABijection(t *testing.T) {
	seeds := prng.New(0x0123456789abcdef, 0xfedcba9876543210)
	seen := make(map[int64]int64, entityid.Capacity)
	for ordinal := int64(0); ordinal < entityid.Capacity; ordinal++ {
		number, err := entityid.Number(seeds, ordinal)
		if err != nil {
			t.Fatalf("ordinal %d: %v", ordinal, err)
		}
		if number < entityid.MinNumber || number > entityid.MaxNumber {
			t.Fatalf("ordinal %d: number %d is outside [%d, %d]",
				ordinal, number, entityid.MinNumber, entityid.MaxNumber)
		}
		if other, ok := seen[number]; ok {
			t.Fatalf("ordinals %d and %d both give number %d", other, ordinal, number)
		}
		seen[number] = ordinal
	}
	if len(seen) != entityid.Capacity {
		t.Fatalf("got %d distinct numbers, want %d", len(seen), entityid.Capacity)
	}
}

// TestNumberHidesTheOrdinal is the reason the permutation is here rather than a
// counter: a number must not order with, or sit near, the ordinal behind it.
// Two adjacent ordinals landing adjacent is a coincidence; a run of them is the
// leak.
func TestNumberHidesTheOrdinal(t *testing.T) {
	seeds := prng.New(19, 12)
	const sample = 1000
	ascending, near := 0, 0
	previous, _ := entityid.Number(seeds, 0)
	for ordinal := int64(1); ordinal < sample; ordinal++ {
		number, err := entityid.Number(seeds, ordinal)
		if err != nil {
			t.Fatalf("ordinal %d: %v", ordinal, err)
		}
		if number > previous {
			ascending++
		}
		if number-previous < 100 && previous-number < 100 {
			near++
		}
		previous = number
	}
	// A permutation puts each of these near chance: half ascending, and a gap
	// under 100 about one time in 4500. The bounds are loose enough that only a
	// broken permutation trips them.
	if ascending < sample/4 || ascending > 3*sample/4 {
		t.Errorf("%d of %d consecutive ordinals ascend; the numbers track the ordinal", ascending, sample)
	}
	if near > sample/50 {
		t.Errorf("%d of %d consecutive ordinals land within 100 of each other", near, sample)
	}
}

// TestNumberIsSeeded is what makes a replay reproducible and two games in one
// database independent.
func TestNumberIsSeeded(t *testing.T) {
	one, two := prng.New(19, 12), prng.New(19, 13)
	same := 0
	for ordinal := int64(0); ordinal < 100; ordinal++ {
		a, err := entityid.Number(one, ordinal)
		if err != nil {
			t.Fatalf("ordinal %d: %v", ordinal, err)
		}
		again, err := entityid.Number(one, ordinal)
		if err != nil {
			t.Fatalf("ordinal %d: %v", ordinal, err)
		}
		if a != again {
			t.Fatalf("ordinal %d gave %d and then %d from the same seeds", ordinal, a, again)
		}
		b, err := entityid.Number(two, ordinal)
		if err != nil {
			t.Fatalf("ordinal %d: %v", ordinal, err)
		}
		if a == b {
			same++
		}
	}
	if same > 5 {
		t.Errorf("%d of 100 ordinals gave the same number under different seeds", same)
	}
}

func TestNumberRefusesAnOrdinalItHasNoNumberFor(t *testing.T) {
	seeds := prng.New(19, 12)
	for _, ordinal := range []int64{-1, entityid.Capacity, entityid.Capacity + 1} {
		if _, err := entityid.Number(seeds, ordinal); err == nil {
			t.Errorf("ordinal %d: want an error, got none", ordinal)
		}
	}
}
