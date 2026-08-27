// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package lifesupport implements the LFSU rules that decide how many people an
// entity can keep alive.
//
// Only units assembled in component inventory support anyone; LFSU held in any
// other section is freight, the same as a drive or a sensor. That is what makes
// life support worth its mass and what a create order's population gate is
// measured against: unsupported people never leave the entity handing them
// over. See docs/units.md.
package lifesupport

// Unit is the inventory unit code of life support.
const Unit = "LFSU"

// Capacity is what an entity's assembled LFSU supports, in population units of
// a hundred persons each. The zero Capacity is an entity that supports nobody.
type Capacity struct {
	Units      int64 // assembled LFSU units
	Population int64 // population units they keep alive
}

// Add installs quantity LFSU units at techLevel. One unit supports t squared
// population units, so 5 LFSU-3 support 45 of them, which is 4,500 persons.
func (c Capacity) Add(techLevel int, quantity int64) Capacity {
	if quantity <= 0 {
		return c
	}
	c.Units += quantity
	c.Population += quantity * int64(techLevel) * int64(techLevel)
	return c
}

// Installed reports whether the entity carries life support.
func (c Capacity) Installed() bool { return c.Units > 0 }

// OpenAir reports whether an entity of a kind breathes the air outside and so
// needs no life support at all.
//
// A COPN sits on a planet whose habitability is above 0, which is what "open
// air" means and what its enclosed-space efficiency of 1 is paying for. It is
// the one entity that carries no LFSU and the one whose population is not
// capped by one.
func OpenAir(entityKind string) bool { return entityKind == "COPN" }
