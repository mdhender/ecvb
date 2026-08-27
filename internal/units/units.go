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

// The inventory sections a unit may be held in. A unit is assembled out of
// unassembled inventory and into one of the two working sections; cargo is
// freight, which is what a transport sets down.
const (
	SectionComponent   = "component"
	SectionOperational = "operational"
	SectionUnassembled = "unassembled"
	SectionCargo       = "cargo"
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

// VolumeIn is what one unit occupies in a section. Unassembled inventory and
// cargo are both freight and take the same room; only assembling a unit costs
// anything extra.
func (m Metrics) VolumeIn(section string) int64 {
	switch section {
	case SectionComponent:
		return m.ComponentVolume
	case SectionOperational:
		return m.OperationalVolume
	default:
		return m.CargoVolume
	}
}

// componentUnits are the codes that only do their work assembled in component
// inventory. docs/units.md says of each of them that a unit held in any other
// section is freight, which is the same statement six times over; this is that
// statement in one place.
var componentUnits = map[string]bool{
	jumpdrive.Unit: true, // HDRV
	sensors.Unit:   true, // SNSR
	"LFSU":         true,
	"SDRV":         true,
	"STRC":         true,
	"STRL":         true,
}

// The population classes. One unit of any of them stands for 100 persons.
var populationClasses = map[string]bool{"USK": true, "SKW": true, "SOL": true, "NAS": true}

// The cadres. A cadre is a temporary assignment of population rather than a
// unit, so it is held in none of the inventory sections.
var cadres = map[string]bool{"CWKR": true, "LABR": true, "PLCF": true, "SPCF": true, "TRNE": true}

// IsPopulation reports whether a code names a population class.
func IsPopulation(code string) bool { return populationClasses[code] }

// IsCadre reports whether a code names a cadre.
func IsCadre(code string) bool { return cadres[code] }

// AssembledSection is where a unit lands when it is assembled, and where it is
// taken from when it is unassembled.
//
// assemblable is false for three kinds of thing. The bulk resources are
// measured rather than manufactured, so there is nothing to put together.
// Population is people, and a cadre is an assignment of people; neither is
// inventory at all.
//
// Nothing an order writes chooses the section. Which one a unit works in is a
// property of the unit, so a player writes what to assemble and the unit code
// says where it goes.
func AssembledSection(unit string) (section string, assemblable bool) {
	if IsBulkResource(unit) || IsPopulation(unit) || IsCadre(unit) {
		return "", false
	}
	if componentUnits[unit] {
		return SectionComponent, true
	}
	return SectionOperational, true
}

// IsStructural reports whether a unit is one of the two that enclose space.
func IsStructural(unit string) bool { return unit == "STRC" || unit == "STRL" }

// EnclosedVolumePerUnit is the raw volume one unit encloses. Only assembled
// STRC and STRL enclose anything, and a unit at technology level t encloses
// t squared VU.
func EnclosedVolumePerUnit(section, unit string, techLevel int) int64 {
	if section != SectionComponent || !IsStructural(unit) {
		return 0
	}
	return int64(techLevel) * int64(techLevel)
}

// OccupiedVolumePerUnit is the enclosed space one unit consumes on an entity
// of a kind. Two things consume none: assembled structure, which creates the
// space rather than filling it, and bulk resources in the cargo of a COPN or a
// CORB, which sit in external depots outside the hull.
func OccupiedVolumePerUnit(entityKind, section, unit string, techLevel int, hasTechLevel bool) int64 {
	if section == SectionComponent && IsStructural(unit) {
		return 0
	}
	if section == SectionCargo && (entityKind == "COPN" || entityKind == "CORB") && IsBulkResource(unit) {
		return 0
	}
	return MetricsFor(unit, techLevel, hasTechLevel).VolumeIn(section)
}

// StoredHasTechLevel reports whether an inventory row read back out of the
// database carries a technology level. A row is written with the level
// ParseTag read off the player's tag, and a unit with no level is written as
// zero, so zero is what "no level" reads as coming back. Nothing in the game
// has a use for a genuine level-zero unit: it would mass nothing and do
// nothing.
func StoredHasTechLevel(techLevel int) bool { return techLevel > 0 }

// MetricsForStored is MetricsFor for a unit read off a database row, where the
// technology level is a column rather than part of a tag.
func MetricsForStored(unit string, techLevel int) Metrics {
	return MetricsFor(unit, techLevel, StoredHasTechLevel(techLevel))
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
//
// An unknown kind is an error rather than a panic. This was private to the kit
// loader, which had already checked the kind against the four the game has;
// order code calls it with a unit code read off a database row, and a row that
// says something else is a corrupt database rather than a programming mistake.
func UsableEnclosedSpace(kind string, enclosedVolume int64) (int64, error) {
	switch kind {
	case "COPN":
		return enclosedVolume, nil
	case "CSFC":
		return enclosedVolume / 5, nil
	case "CORB", "SHIP":
		return enclosedVolume / 10, nil
	default:
		return 0, fmt.Errorf("unknown entity kind %q", kind)
	}
}
