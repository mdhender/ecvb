// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package cadre implements the CWKR rules that decide how much assembly and
// unassembly an entity's construction workers get through in a turn.
//
// A cadre is not a unit. It is a temporary assignment of population -- one
// CWKR is one SKW plus one USK -- so it has no mass and no volume of its own;
// the people in it are already counted in the entity's population. See
// docs/units.md.
package cadre

import "github.com/mdhender/ecvb/internal/units"

// Unit is the code of the construction worker cadre.
const Unit = "CWKR"

// Class is one population class a cadre assigns, and how many of them one unit
// of that cadre takes.
type Class struct {
	Name    string
	PerUnit int64
}

// Composition is the population one unit of a cadre assigns. One CWKR is one
// SKW plus one USK.
//
// It is what makes a cadre an assignment rather than a thing. The people in it
// are already counted in the entity's population, so a cadre adds no mass; but
// they are spoken for, so they are not available to be given a second job, and
// the cadre cannot outlive them. Both halves of that are internal/world's to
// enforce, and this is what it enforces them against.
//
// The other four cadres have names and nothing else -- docs/units.md says
// outright that what they permit and which population they assign is not
// settled -- so nothing can form one and this reports none.
func Composition(name string) []Class {
	if name != Unit {
		return nil
	}
	return []Class{{Name: units.ClassSkilled, PerUnit: 1}, {Name: units.ClassUnskilled, PerUnit: 1}}
}

// WorkPerUnit is the work in MU that one construction worker does in a turn.
// A worker does one task per turn, so this is the whole of what one is worth
// however many orders ask for it.
const WorkPerUnit = 500

// The two pools of construction work. Work of one kind is pooled across an
// entity, so the workers it needs are reckoned from one total rather than
// order by order and unit by unit -- an entity assembling 15,120 MU of HDRV
// and 100 MU of STRC-1 needs 31 workers and not 32.
//
// The two pools never pool with each other. An entity assembling 100 MU and
// unassembling 100 MU needs two workers and not one, because each total is
// rounded up on its own.
const (
	Assembly   = "assembly"
	Unassembly = "unassembly"
)

// WorkersFor is how many construction workers a pool of work needs. The
// rounding up is per pool, not per line, which is what makes an entity
// assembling many small lots cheaper than one charged a whole worker for each.
func WorkersFor(work int64) int64 {
	if work <= 0 {
		return 0
	}
	return (work + WorkPerUnit - 1) / WorkPerUnit
}

// WorkAllowed is how much more work of one kind an entity's cadre can still do
// this turn: workers is the cadre it has, done is what that pool has already
// got through, and other is what the other pool has.
//
// The other pool is subtracted as workers rather than as work, because it
// rounds up on its own: an entity with two workers that has unassembled 1 MU
// has one worker left and 500 MU of assembly in it, not 999.
//
// A shortage is a rate rather than a failure. An order that outruns the cadre
// assembles what the workers paid for and no more; it does not fail for want
// of them.
func WorkAllowed(workers, done, other int64) int64 {
	spare := workers - WorkersFor(other)
	if spare <= 0 {
		return 0
	}
	allowed := spare*WorkPerUnit - done
	if allowed < 0 {
		return 0
	}
	return allowed
}
