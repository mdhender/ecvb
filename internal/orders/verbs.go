// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"errors"
	"fmt"

	"github.com/mdhender/ecvb/internal/jumpdrive"
	"github.com/mdhender/ecvb/internal/sensors"
	"github.com/mdhender/ecvb/internal/world"
	"zombiezen.com/go/sqlite/sqlitex"
)

// The orders the game understands. Each one lives here and nowhere else: its
// Spec is how the parser finds it and how `ec orders help` prints it, its
// Params is the line the player wrote, and its Bound is what the game does
// about it. Nothing else in the pipeline knows the order exists.

var errExpectedActor = syntaxErr{message: "expected ship or colony"}

func init() {
	register(&Spec{
		Verb:    "jump",
		Summary: "send a ship to another stellium, within its drive's range",
		Syntax:  []string{"jump ship SHIP-ID to (X,Y,Z)"},
		Phase:   PhaseJump,
		Parse: func(line *Line) (Params, error) {
			var order JumpParams
			if err := line.expect("ship"); err != nil {
				return nil, err
			}
			var err error
			if order.ShipID, err = line.entityID("ship"); err != nil {
				return nil, err
			}
			if err := line.expect("to"); err != nil {
				return nil, err
			}
			if order.X, order.Y, order.Z, err = line.coordinates(); err != nil {
				return nil, err
			}
			return order, nil
		},
	})

	register(&Spec{
		Verb:    "move",
		Summary: "move a ship inside its stellium, to a planet or to the stellium orbit",
		Syntax: []string{
			"move ship SHIP-ID to orbit ORBIT",
			"move ship SHIP-ID to system SYSTEM orbit ORBIT",
		},
		Phase: PhaseMove,
		Parse: func(line *Line) (Params, error) {
			var order MoveParams
			if err := line.expect("ship"); err != nil {
				return nil, err
			}
			var err error
			if order.ShipID, err = line.entityID("ship"); err != nil {
				return nil, err
			}
			if err := line.expect("to"); err != nil {
				return nil, err
			}
			// A move may name a system of the ship's stellium, or leave the
			// system out and mean the one the ship is already in.
			if _, ok := line.keyword("system"); ok {
				if order.System, err = line.systemLetter(); err != nil {
					return nil, err
				}
			}
			if err := line.expect("orbit"); err != nil {
				return nil, err
			}
			if order.Orbit, err = line.number("orbit"); err != nil {
				return nil, err
			}
			return order, nil
		},
	})

	register(&Spec{
		Verb:    "probe",
		Summary: "read planets with a ship's or a colony's sensors, one probe per orbit",
		Syntax: []string{
			"probe ship SHIP-ID orbit ORBIT ...",
			"probe colony COLONY-ID orbit ORBIT ...",
			"probe ship SHIP-ID system SYSTEM orbit ORBIT ...",
			"probe colony COLONY-ID system SYSTEM orbit ORBIT ...",
		},
		Phase: PhaseProbe,
		Parse: func(line *Line) (Params, error) {
			var order ProbeParams
			kind, ok := line.keyword("ship", "colony")
			if !ok {
				return nil, errExpectedActor
			}
			order.Kind = kind
			var err error
			if order.EntityID, err = line.entityID(kind); err != nil {
				return nil, err
			}
			// A probe that names a system reads any system of the entity's
			// stellium; one that does not reads the system it is in.
			if _, ok := line.keyword("system"); ok {
				if order.System, err = line.systemLetter(); err != nil {
					return nil, err
				}
			}
			if err := line.expect("orbit"); err != nil {
				return nil, err
			}
			if order.Orbits, err = line.orbitList(); err != nil {
				return nil, err
			}
			return order, nil
		},
	})
}

// MOVE ------------------------------------------------------------------

// MoveParams is a MOVE as written: a ship and an orbit, which a system of the
// ship's own stellium may qualify.
type MoveParams struct {
	ShipID int64
	System string
	Orbit  int
}

// Actor is the ship the move carries.
func (p MoveParams) Actor() int64 { return p.ShipID }

// Bind resolves the destination and measures the drive against the ship. Every
// move inside a stellium is well within the range of any drive, so only the
// drive's presence and the mass it propels matter.
func (p MoveParams) Bind(b *Binder) ([]Bound, error) {
	ship, err := b.actor(p.ShipID, "ship")
	if err != nil {
		return nil, err
	}
	order := &moveBound{ship: ship, system: p.System, orbit: p.Orbit, stelliumID: ship.Location.StelliumID}
	// Orbit 11 is the stellium orbit rather than a planet, so it resolves to
	// no system and no planet and cannot be qualified by a letter.
	if p.Orbit == StelliumOrbit {
		if p.System != "" {
			return nil, fmt.Errorf("orbit %d is the stellium orbit and belongs to no system", StelliumOrbit)
		}
	} else {
		systemID, err := b.system(ship, p.System)
		if err != nil {
			return nil, err
		}
		if systemID == 0 {
			return nil, errors.New("ship has no current system; specify a destination system")
		}
		planet, exists, err := b.World.Planet(systemID, p.Orbit)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, fmt.Errorf("system %s has no planet in orbit %d", displaySystem(p.System), p.Orbit)
		}
		order.systemID, order.planetID = systemID, planet.ID
	}
	if !ship.Drive.Installed() {
		return nil, fmt.Errorf("ship %d has no assembled %s and cannot move", ship.ID, jumpdrive.Unit)
	}
	if !ship.Drive.CanPropel(ship.Mass) {
		return nil, fmt.Errorf("ship %d masses %d MU and its drive propels %d MU",
			ship.ID, ship.Mass, ship.Drive.Capacity)
	}
	order.kind = jumpdrive.KindOfMove(ship.Location.SystemID, order.systemID)
	order.cost = ship.Drive.FuelForMove(order.kind)
	return []Bound{order}, nil
}

type moveBound struct {
	ship *world.Entity
	// system and orbit are what the player asked for, kept so the order the
	// report shows is the order they wrote.
	system string
	orbit  int
	// The destination, settled when the order bound. A move never leaves its
	// stellium, so the stellium is the one the ship was in; a system and a
	// planet of zero is the stellium orbit.
	stelliumID int64
	systemID   int64
	planetID   int64
	kind       jumpdrive.MoveKind
	cost       int64
}

func (o *moveBound) Fuel() int64 { return o.cost }

func (o *moveBound) Apply(t *Turn) (Outcome, error) {
	start := o.ship.Location
	if o.kind == jumpdrive.MoveNowhere {
		// The ship is already in the stellium orbit and stays exactly as it
		// was: nothing to burn, nothing to write.
		return succeeded(start, start, 0), nil
	}
	message, err := spendFuel(t, o.ship, "move", o.cost)
	if err != nil {
		return Outcome{}, err
	}
	if message != "" {
		return failed(start, message), nil
	}
	// A ship that moves to a planet settles into a ring the game draws; a ship
	// ordered to the stellium orbit has no ring at all.
	final := world.Location{StelliumID: o.stelliumID, SystemID: o.systemID, PlanetID: o.planetID}
	if o.systemID != 0 {
		final.Ring = t.World.Game().Seed.RingFor(t.Number, t.FactionID, t.Sequence)
	}
	if err := t.World.Move(o.ship, final); err != nil {
		return Outcome{}, err
	}
	return succeeded(start, final, o.cost), nil
}

func (o *moveBound) store(at row) error {
	if err := sqlitex.ExecuteTransient(at.conn, `
		INSERT INTO move_order (
			game_id, turn, faction_id, sequence, source_line, ship_id,
			requested_system, requested_orbit,
			destination_stellium_id, destination_system_id, destination_planet_id, fuel_spent
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`, &sqlitex.ExecOptions{
		Args: []any{at.gameID, at.turn, at.factionID, at.sequence, at.line, o.ship.ID,
			nullableString(o.system), o.orbit,
			o.stelliumID, nullableID(o.systemID), nullableID(o.planetID), o.cost},
	}); err != nil {
		return fmt.Errorf("insert move order: %w", err)
	}
	return nil
}

// JUMP ------------------------------------------------------------------

// JumpParams is a JUMP as written: a ship and the point it is bound for.
type JumpParams struct {
	ShipID  int64
	X, Y, Z int
}

// Actor is the ship the jump carries.
func (p JumpParams) Actor() int64 { return p.ShipID }

// Bind finds the destination stellium and measures the jump against the drive.
func (p JumpParams) Bind(b *Binder) ([]Bound, error) {
	ship, err := b.actor(p.ShipID, "ship")
	if err != nil {
		return nil, err
	}
	destinationID := b.World.StelliumAt(p.X, p.Y, p.Z)
	if destinationID == 0 {
		return nil, fmt.Errorf("game %q has no stellium at (%d,%d,%d)", b.World.Game().Code, p.X, p.Y, p.Z)
	}
	if !ship.Drive.Installed() {
		return nil, fmt.Errorf("ship %d has no assembled %s and cannot jump", ship.ID, jumpdrive.Unit)
	}
	if !ship.Drive.CanPropel(ship.Mass) {
		return nil, fmt.Errorf("ship %d masses %d MU and its jump drive propels %d MU",
			ship.ID, ship.Mass, ship.Drive.Capacity)
	}
	from := b.World.Coordinates(ship.Location.StelliumID)
	if !ship.Drive.Reaches(jumpdrive.SquaredDistance(from.X, from.Y, from.Z, p.X, p.Y, p.Z)) {
		return nil, fmt.Errorf("jump of %d units exceeds ship %d jump range of %d units",
			jumpdrive.Distance(from.X, from.Y, from.Z, p.X, p.Y, p.Z), ship.ID, ship.Drive.Range)
	}
	return []Bound{&jumpBound{
		ship: ship, x: p.X, y: p.Y, z: p.Z, destinationID: destinationID,
		cost: ship.Drive.FuelForJump(jumpdrive.Distance(from.X, from.Y, from.Z, p.X, p.Y, p.Z)),
	}}, nil
}

type jumpBound struct {
	ship          *world.Entity
	x, y, z       int
	destinationID int64
	cost          int64
}

func (o *jumpBound) Fuel() int64 { return o.cost }

func (o *jumpBound) Apply(t *Turn) (Outcome, error) {
	start := o.ship.Location
	message, err := spendFuel(t, o.ship, "jump", o.cost)
	if err != nil {
		return Outcome{}, err
	}
	if message != "" {
		return failed(start, message), nil
	}
	// A jump arrives in the destination's stellium orbit: a ship crosses to a
	// planet under its own power, with a MOVE.
	final := world.Location{StelliumID: o.destinationID}
	if err := t.World.Move(o.ship, final); err != nil {
		return Outcome{}, err
	}
	return succeeded(start, final, o.cost), nil
}

func (o *jumpBound) store(at row) error {
	if err := sqlitex.ExecuteTransient(at.conn, `
		INSERT INTO jump_order (
			game_id, turn, faction_id, sequence, source_line, ship_id,
			destination_x, destination_y, destination_z, destination_stellium_id, fuel_spent
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`, &sqlitex.ExecOptions{
		Args: []any{at.gameID, at.turn, at.factionID, at.sequence, at.line, o.ship.ID,
			o.x, o.y, o.z, o.destinationID, o.cost},
	}); err != nil {
		return fmt.Errorf("insert jump order: %w", err)
	}
	return nil
}

// PROBE -----------------------------------------------------------------

// ProbeParams is a PROBE as written. Kind is the word the player used, "ship"
// or "colony"; a probe is the only order a colony may be given. An order read
// back out of the database carries no Kind, because which word was written was
// settled when it was written.
type ProbeParams struct {
	Kind     string
	EntityID int64
	System   string
	// Orbits holds every orbit the line named. One probe order probes one or
	// more orbits and spends one probe on each.
	Orbits []int
}

// Actor is the entity whose sensors launch the probes.
func (p ProbeParams) Actor() int64 { return p.EntityID }

// Bind spends one probe from the entity's budget for each orbit it can read.
// The budget is settled here rather than in Apply because it is fixed by the
// order file itself: nothing that happens during a turn adds a probe.
func (p ProbeParams) Bind(b *Binder) ([]Bound, error) {
	entity, err := b.actor(p.EntityID, p.Kind)
	if err != nil {
		return nil, err
	}
	if !entity.Sensors.Installed() {
		return nil, fmt.Errorf("%s %d has no assembled %s and cannot probe", noun(entity), entity.ID, sensors.Unit)
	}
	// A probe that names a system reads any system of the entity's stellium. A
	// probe that does not reads the system the entity is in, which is why an
	// entity orbiting the stellium has to name one.
	systemID, err := b.system(entity, p.System)
	if err != nil {
		return nil, err
	}
	if systemID == 0 {
		return nil, fmt.Errorf("%s %d is orbiting the stellium; name a system to probe", noun(entity), entity.ID)
	}
	var bounds []Bound
	var found bindErrors
	for _, orbit := range p.Orbits {
		if b.World.ProbesSpent(entity.ID) >= entity.Sensors.Probes {
			found = append(found, fmt.Errorf("%s %d has only %d probes this turn",
				noun(entity), entity.ID, entity.Sensors.Probes))
			break
		}
		if orbit < 1 || orbit > 10 {
			found = append(found, fmt.Errorf("orbit %d is not between 1 and 10", orbit))
			continue
		}
		planet, exists, err := b.World.Planet(systemID, orbit)
		if err != nil {
			return nil, err
		}
		if !exists {
			found = append(found, fmt.Errorf("system %s has no planet in orbit %d", displaySystem(p.System), orbit))
			continue
		}
		b.World.SpendProbe(entity.ID)
		bounds = append(bounds, &probeBound{entity: entity, system: p.System, orbit: orbit,
			systemID: systemID, planet: planet})
	}
	if len(found) != 0 {
		return nil, found
	}
	return bounds, nil
}

type probeBound struct {
	entity   *world.Entity
	system   string
	orbit    int
	systemID int64
	planet   world.Planet
}

// Fuel is nothing: a probe is launched, not flown.
func (o *probeBound) Fuel() int64 { return 0 }

// Apply reads the planet and records what it found. The finding is stored now
// rather than derived at report time, so it survives the ship jumping away
// later in the same turn.
func (o *probeBound) Apply(t *Turn) (Outcome, error) {
	at := o.entity.Location
	if err := t.World.RecordProbe(t.Number, t.FactionID, o.planet.ID); err != nil {
		return Outcome{}, err
	}
	item := succeeded(at, at, 0)
	item.Result = ProbeResult{StelliumID: at.StelliumID, SystemID: o.systemID,
		PlanetID: o.planet.ID, Habitability: o.planet.Habitability}
	return item, nil
}

func (o *probeBound) store(at row) error {
	if err := sqlitex.ExecuteTransient(at.conn, `
		INSERT INTO probe_order (
			game_id, turn, faction_id, sequence, source_line, entity_id, requested_system, requested_orbit
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?);`, &sqlitex.ExecOptions{
		Args: []any{at.gameID, at.turn, at.factionID, at.sequence, at.line, o.entity.ID,
			nullableString(o.system), o.orbit},
	}); err != nil {
		return fmt.Errorf("insert probe order: %w", err)
	}
	return nil
}
