// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package fuel implements the FUEL rules that decide what an entity's fuel is
// worth and which of its fuel is spent first.
//
// Taking the units out of an entity is not here. internal/world is the only
// thing that writes the inventory table, so that what an entity holds and what
// it masses cannot drift apart; this package is the rule world spends by.
package fuel

import (
	"github.com/mdhender/ecvb/internal/units"
)

// Unit is the inventory unit code of fuel. Unlike a drive or a sensor, fuel
// works from any section: it is burned, not assembled.
const Unit = "FUEL"

// UnitMass is the mass in MU of one FUEL unit. FUEL is a bulk resource, so it
// masses 1 MU like the other three. See docs/units.md.
const UnitMass = 1

// drawOrder is the order the sections are emptied in. Fuel that is already
// plumbed into the entity goes first and freight goes last, so a hold of spare
// fuel survives until the working supply is gone.
//
// Fuel is not held in component inventory: it is burned rather than assembled,
// so there is nothing to install.
var drawOrder = [...]string{units.SectionOperational, units.SectionUnassembled, units.SectionCargo}

// DrawOrder is the order an entity's fuel is emptied in.
func DrawOrder() []string { return drawOrder[:] }
