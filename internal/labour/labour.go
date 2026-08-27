// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package labour implements the production-labour rules that decide how much
// freight an entity's STOW and UNSTOW orders move in a turn.
//
// Production labour is not a cadre. It is an entity's unassigned USK plus t
// for every assembled AUTO it carries: automation stands in for an unskilled
// worker wherever unskilled work is done, and moving freight is unskilled work
// however the term reads. Nothing is drafted for it and nothing is assigned
// into it -- automation is a pool and never a member -- so the total is read
// off what the entity holds rather than kept anywhere.
//
// A worker already assigned to a cadre is not production labour at all, having
// been spoken for. Automation is the other way round: it is production labour
// and never a cadre, because a cadre is an assignment of people. See
// docs/units.md.
package labour

// Automation is the code of the unit that does unskilled work without anyone
// doing it. Only assembled units count; an AUTO held in any other section is
// freight, the same as a drive or a sensor.
const Automation = "AUTO"

// PerUnit is the freight in MU that one unit of production labour moves in a
// turn. docs/units.md gives it as the rate a construction worker assembles at,
// which is where the number comes from; it is written here rather than read
// out of internal/cadre because the two are separate rules that happen to
// agree, and because a cadre is exactly what production labour is not.
const PerUnit = 500

// The two pools of freight handling. One unit of production labour does one
// task per turn, so labour that stowed cannot also unstow: the two pools round
// up on their own and never pool with each other, the way assembly and
// unassembly do for a CWKR.
const (
	Stowing   = "stowing"
	Unstowing = "unstowing"
)

// Automates is the production labour that a lot of assembled AUTO stands in
// for: one unit at technology level t does the work of t USK, so 20 AUTO-4
// replace 80 unskilled workers.
func Automates(techLevel int, count int64) int64 { return int64(techLevel) * count }
