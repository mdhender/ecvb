// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package units implements the mass and volume rules that decide what one
// inventory unit weighs and how much room it takes in each section. Both the
// kit loader and the orders that move units through inventory measure with
// these, so a unit weighs the same whoever is asking.
//
// The formulas here are provisional apart from the drives and sensors, which
// have defined masses of their own. See docs/units.md.
package units

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mdhender/ecvb/internal/jumpdrive"
	"github.com/mdhender/ecvb/internal/sensors"
)

// Metrics is what one unit masses and what it occupies in each of the sections
// it may be held in. Assembling a unit usually costs volume: a manufactured
// unit takes twice its cargo volume operational and four times it as a
// component.
type Metrics struct {
	Mass              int64
	CargoVolume       int64
	OperationalVolume int64
	ComponentVolume   int64
}

// PopulationMetrics is what one population unit -- a hundred people of any
// class -- masses and occupies. Population is never assembled, so all four
// numbers are the same.
var PopulationMetrics = Metrics{
	Mass:              2,
	CargoVolume:       2,
	OperationalVolume: 2,
	ComponentVolume:   2,
}

// ParseTag splits a unit tag into its unit code and technology level. A tag is
// either a bare code (FUEL) or a code and a level (HDRV-8); a level runs 0
// through 10 and a code is upper-case letters and digits.
func ParseTag(tag string) (unit string, techLevel int, hasTechLevel bool, err error) {
	if tag == "" {
		return "", 0, false, fmt.Errorf("unit code is required")
	}
	unit = tag
	if dash := strings.LastIndexByte(tag, '-'); dash >= 0 {
		unit = tag[:dash]
		if unit == "" || dash == len(tag)-1 {
			return "", 0, false, fmt.Errorf("invalid unit tag %q", tag)
		}
		techLevel, err = strconv.Atoi(tag[dash+1:])
		if err != nil || techLevel < 0 || techLevel > 10 {
			return "", 0, false, fmt.Errorf("invalid unit tag %q", tag)
		}
		hasTechLevel = true
	}
	for _, r := range unit {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return "", 0, false, fmt.Errorf("invalid unit code %q", unit)
		}
	}
	return unit, techLevel, hasTechLevel, nil
}

// MetricsFor is what one unit of the given code and technology level masses and
// occupies.
func MetricsFor(unit string, techLevel int, hasTechLevel bool) Metrics {
	// The bulk resources are measured, not manufactured: one unit masses 1 MU
	// and takes 1 VU wherever it is held, with none of the multipliers that
	// installing a manufactured unit costs.
	if IsBulkResource(unit) && !hasTechLevel {
		return Metrics{Mass: 1, CargoVolume: 1, OperationalVolume: 1, ComponentVolume: 1}
	}
	base := int64(6)
	if hasTechLevel {
		base = int64(2 * techLevel)
	}
	metrics := Metrics{
		Mass:              base,
		CargoVolume:       base,
		OperationalVolume: 2 * base,
		ComponentVolume:   4 * base,
	}
	// A jump drive and a sensor have defined masses. Their volumes remain
	// provisional.
	switch {
	case unit == jumpdrive.Unit && hasTechLevel:
		metrics.Mass = jumpdrive.UnitMass(techLevel)
	case unit == sensors.Unit && hasTechLevel:
		metrics.Mass = sensors.UnitMass(techLevel)
	}
	return metrics
}

// IsBulkResource reports whether a unit is one of the four raw resources. They
// share a mass and volume of 1, and as cargo on a COPN or CORB they sit in
// external depots rather than in enclosed space.
func IsBulkResource(unit string) bool {
	return unit == "GOLD" || unit == "FUEL" || unit == "METL" || unit == "MNRL"
}

// UsableEnclosedSpace is how much of an entity's raw enclosed volume it can
// actually hold things in. An open-air colony uses all of it; the others lose
// most of it to the structure that keeps the outside out.
func UsableEnclosedSpace(kind string, enclosedVolume int64) int64 {
	switch kind {
	case "COPN":
		return enclosedVolume
	case "CSFC":
		return enclosedVolume / 5
	case "CORB", "SHIP":
		return enclosedVolume / 10
	default:
		panic("unknown entity kind " + kind)
	}
}
