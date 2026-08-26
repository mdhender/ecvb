// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mdhender/ecvb/internal/jumpdrive"
	"github.com/mdhender/ecvb/internal/sensors"
	"github.com/mdhender/ecvb/internal/world"
)

// The orders the game understands. Each one lives here and nowhere else: its
// Spec is how the parser finds it and how `ec orders help` prints it, its
// Params is the line the player wrote, and its Bound is what the game does
// about it. Nothing else in the pipeline knows the order exists.

var errExpectedActor = syntaxErr{message: "expected ship or colony"}

func init() {
	register(&Spec{
		Verb:     "jump",
		Summary:  "send a ship to another stellium, within its drive's range",
		Syntax:   []string{"jump ship SHIP-ID to (X,Y,Z)"},
		Phase:    PhaseJump,
		Movement: true,
		Decode: func(actor int64, encoded string) (Params, error) {
			order := JumpParams{ShipID: actor}
			if err := json.Unmarshal([]byte(encoded), &order); err != nil {
				return nil, err
			}
			return order, nil
		},
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
		Phase:    PhaseMove,
		Movement: true,
		Decode: func(actor int64, encoded string) (Params, error) {
			order := MoveParams{ShipID: actor}
			if err := json.Unmarshal([]byte(encoded), &order); err != nil {
				return nil, err
			}
			return order, nil
		},
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
		Verb:    "name",
		Summary: "give a place or one of your ships or colonies a name only you see",
		Syntax: []string{
			`name ship SHIP-ID "NAME"`,
			`name colony COLONY-ID "NAME"`,
			`name (X,Y,Z) "NAME"`,
			`name (X,Y,Z) system SYSTEM "NAME"`,
			`name (X,Y,Z) system SYSTEM orbit ORBIT "NAME"`,
		},
		Phase: PhaseNaming,
		Decode: func(actor int64, encoded string) (Params, error) {
			order := NameParams{Entity: actor}
			if err := json.Unmarshal([]byte(encoded), &order); err != nil {
				return nil, err
			}
			return order, nil
		},
		Parse: func(line *Line) (Params, error) {
			var order NameParams
			var err error
			// A name is given either to a place, which opens with its
			// coordinates, or to an entity, which opens with what kind it is.
			if kind, ok := line.keyword("ship", "colony"); ok {
				order.Kind = kind
				if order.Entity, err = line.entityID(kind); err != nil {
					return nil, err
				}
			} else {
				place := &Place{}
				if place.X, place.Y, place.Z, err = line.coordinates(); err != nil {
					return nil, err
				}
				if _, ok := line.keyword("system"); ok {
					if place.System, err = line.systemLetter(); err != nil {
						return nil, err
					}
					// Only a system can hold a planet, so an orbit may only
					// follow one.
					if _, ok := line.keyword("orbit"); ok {
						if place.Orbit, err = line.number("orbit"); err != nil {
							return nil, err
						}
					}
				}
				order.Place = place
			}
			if order.Name, err = line.quoted("name"); err != nil {
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
		Decode: func(actor int64, encoded string) (Params, error) {
			order := ProbeParams{EntityID: actor}
			if err := json.Unmarshal([]byte(encoded), &order); err != nil {
				return nil, err
			}
			return order, nil
		},
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
	ShipID int64  `json:"-"`
	System string `json:"system,omitempty"`
	Orbit  int    `json:"orbit"`
}

// Actor is the ship the move carries.
func (p MoveParams) Actor() int64 { return p.ShipID }

// Input is the move as the player wrote it.
func (p MoveParams) Input() string { return orbitInput(p.System, p.Orbit) }

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

// Params is the move as it will be stored: the orbit asked for, and the system
// only if the player named one.
func (o *moveBound) Params() Params {
	return MoveParams{ShipID: o.ship.ID, System: o.system, Orbit: o.orbit}
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

// JUMP ------------------------------------------------------------------

// JumpParams is a JUMP as written: a ship and the point it is bound for.
type JumpParams struct {
	ShipID int64 `json:"-"`
	X      int   `json:"x"`
	Y      int   `json:"y"`
	Z      int   `json:"z"`
}

// Actor is the ship the jump carries.
func (p JumpParams) Actor() int64 { return p.ShipID }

// Input is the jump as the player wrote it.
func (p JumpParams) Input() string { return fmt.Sprintf("(%d,%d,%d)", p.X, p.Y, p.Z) }

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

// Params is the jump as it will be stored: the point it is bound for.
func (o *jumpBound) Params() Params {
	return JumpParams{ShipID: o.ship.ID, X: o.x, Y: o.y, Z: o.z}
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

// PROBE -----------------------------------------------------------------

// ProbeParams is a PROBE as written. Kind is the word the player used, "ship"
// or "colony"; a probe is the only order a colony may be given. An order read
// back out of the database carries no Kind, because which word was written was
// settled when it was written.
type ProbeParams struct {
	Kind     string `json:"-"`
	EntityID int64  `json:"-"`
	System   string `json:"system,omitempty"`
	// Orbits holds every orbit the line named. One probe order probes one or
	// more orbits and spends one probe on each, so a stored probe holds one.
	Orbits []int `json:"orbits"`
}

// Actor is the entity whose sensors launch the probes.
func (p ProbeParams) Actor() int64 { return p.EntityID }

// Input is the probe as the player wrote it, however many orbits it named.
func (p ProbeParams) Input() string { return orbitInput(p.System, p.Orbits...) }

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

// Params is one probe of one orbit, which is what a stored probe order is.
func (o *probeBound) Params() Params {
	return ProbeParams{EntityID: o.entity.ID, System: o.system, Orbits: []int{o.orbit}}
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
	item.Survey = &Survey{StelliumID: at.StelliumID, SystemID: o.systemID,
		PlanetID: o.planet.ID, Habitability: o.planet.Habitability}
	return item, nil
}

// NAME ------------------------------------------------------------------

// LongestName is how long a name may be, counted in characters and counting
// the spaces.
const LongestName = 24

// NameParams is a NAME as written. A name is given to an entity -- the ship or
// colony in Entity -- or to a place, and never to both.
type NameParams struct {
	// Entity is the ship or colony being named, and is 0 when a place is being
	// named instead. It is the order's actor, so it is a column rather than
	// part of the stored parameters.
	Entity int64 `json:"-"`
	// Kind is the word the player wrote, "ship" or "colony". An order read
	// back out of the database carries none, because which word was written
	// was settled when it was written.
	Kind string `json:"-"`
	// Place is the thing being named when it is not an entity: a stellium, or
	// a system or planet of one.
	Place *Place `json:"place,omitempty"`
	Name  string `json:"name"`
}

// Place is a stellium, or a system or a planet of one, as an order named it.
type Place struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Z      int    `json:"z"`
	System string `json:"system,omitempty"`
	Orbit  int    `json:"orbit,omitzero"`
}

// Actor is the entity being named, or 0 when the order names a place. A place
// belongs to nobody, so naming one is an order with no actor at all.
func (p NameParams) Actor() int64 { return p.Entity }

// Input is the name order as the player wrote it.
func (p NameParams) Input() string {
	if p.Place == nil {
		return fmt.Sprintf("%s %d %q", p.Kind, p.Entity, p.Name)
	}
	var out strings.Builder
	fmt.Fprintf(&out, "(%d,%d,%d)", p.Place.X, p.Place.Y, p.Place.Z)
	if p.Place.System != "" {
		fmt.Fprintf(&out, " system %s", p.Place.System)
	}
	if p.Place.Orbit != 0 {
		fmt.Fprintf(&out, " orbit %d", p.Place.Orbit)
	}
	fmt.Fprintf(&out, " %q", p.Name)
	return out.String()
}

// Bind checks the name and finds what it is for. Everything a NAME settles is
// settled here: nothing about naming can change during a turn, so Apply only
// writes the label down.
func (p NameParams) Bind(b *Binder) ([]Bound, error) {
	if err := checkName(p.Name); err != nil {
		return nil, err
	}
	if p.Place == nil {
		entity, err := b.actor(p.Entity, p.Kind)
		if err != nil {
			return nil, err
		}
		return []Bound{&nameBound{params: p, of: world.NamedEntity, id: entity.ID}}, nil
	}
	stelliumID := b.World.StelliumAt(p.Place.X, p.Place.Y, p.Place.Z)
	if stelliumID == 0 {
		return nil, fmt.Errorf("game %q has no stellium at (%d,%d,%d)",
			b.World.Game().Code, p.Place.X, p.Place.Y, p.Place.Z)
	}
	if p.Place.System == "" {
		return []Bound{&nameBound{params: p, of: world.NamedStellium, id: stelliumID}}, nil
	}
	systemID, err := b.World.System(stelliumID, p.Place.System)
	if err != nil {
		return nil, err
	}
	if systemID == 0 {
		return nil, fmt.Errorf("the stellium at (%d,%d,%d) has no system %s",
			p.Place.X, p.Place.Y, p.Place.Z, p.Place.System)
	}
	if p.Place.Orbit == 0 {
		return []Bound{&nameBound{params: p, of: world.NamedSystem, id: systemID}}, nil
	}
	planet, exists, err := b.World.Planet(systemID, p.Place.Orbit)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("system %s has no planet in orbit %d", p.Place.System, p.Place.Orbit)
	}
	return []Bound{&nameBound{params: p, of: world.NamedPlanet, id: planet.ID}}, nil
}

// checkName is what a name may be: printable, no longer than LongestName,
// and spaced the way the player would read it back.
func checkName(name string) error {
	if name == "" {
		return errors.New("a name cannot be empty")
	}
	if count := utf8.RuneCountInString(name); count > LongestName {
		return fmt.Errorf("a name may be %d characters and %q is %d", LongestName, name, count)
	}
	if strings.TrimSpace(name) != name {
		return fmt.Errorf("a name may not begin or end with a space")
	}
	if strings.Contains(name, "  ") {
		return fmt.Errorf("a name may not hold two spaces in a row")
	}
	for _, r := range name {
		if r != ' ' && !unicode.IsPrint(r) {
			return fmt.Errorf("a name may not hold control characters")
		}
	}
	return nil
}

type nameBound struct {
	params NameParams
	of     world.Subject
	id     int64
}

// Params is the name order as it will be stored.
func (o *nameBound) Params() Params { return o.params }

// Fuel is nothing: naming something costs no fuel.
func (o *nameBound) Fuel() int64 { return 0 }

// Apply writes the name down. Nothing about a name can fail once it is bound.
func (o *nameBound) Apply(t *Turn) (Outcome, error) {
	if err := t.World.SetName(t.FactionID, o.of, o.id, o.params.Name); err != nil {
		return Outcome{}, err
	}
	// A name moves nothing, so the order happened wherever its actor stands; a
	// place has no actor and no location at all.
	at := world.Location{}
	if entity := t.World.Entity(o.params.Entity); entity != nil {
		at = entity.Location
	}
	return succeeded(at, at, 0), nil
}
