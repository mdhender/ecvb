// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mdhender/ecvb/internal/cadre"
	"github.com/mdhender/ecvb/internal/jumpdrive"
	"github.com/mdhender/ecvb/internal/labour"
	"github.com/mdhender/ecvb/internal/sensors"
	"github.com/mdhender/ecvb/internal/transport"
	"github.com/mdhender/ecvb/internal/units"
	"github.com/mdhender/ecvb/internal/world"
)

// The orders the game understands. Each one lives here and nowhere else: its
// Spec is how the parser finds it and how `ec orders help` prints it, its
// Params is the line the player wrote, and its Bound is what the game does
// about it. Nothing else in the pipeline knows the order exists.

func init() {
	register(&Spec{
		Verb:     "jump",
		Subjects: []string{SubjectShip},
		Summary:  "send a ship from the stellium orbit to another stellium",
		Syntax:   []string{"ship SHIP-ID jump to (X,Y,Z)"},
		Phase:    PhaseJump,
		Movement: true,
		Decode: func(actor int64, encoded string) (Params, error) {
			order := JumpParams{ShipID: actor}
			if err := json.Unmarshal([]byte(encoded), &order); err != nil {
				return nil, err
			}
			return order, nil
		},
		Parse: func(subject Subject, line *Line) (Params, error) {
			order := JumpParams{ShipID: subject.ID}
			if err := line.expect("to"); err != nil {
				return nil, err
			}
			var err error
			if order.X, order.Y, order.Z, err = line.coordinates(); err != nil {
				return nil, err
			}
			return order, nil
		},
	})

	register(&Spec{
		Verb:     "move",
		Subjects: []string{SubjectShip},
		Summary:  "move a ship inside its stellium, to a planet or to the stellium orbit",
		Syntax: []string{
			"ship SHIP-ID move to orbit ORBIT",
			"ship SHIP-ID move to system SYSTEM orbit ORBIT",
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
		Parse: func(subject Subject, line *Line) (Params, error) {
			order := MoveParams{ShipID: subject.ID}
			var err error
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
		Verb:     "name",
		Subjects: []string{SubjectShip, SubjectColony, SubjectFaction},
		Summary:  "give a place, a faction, or a ship or colony a name only you see",
		Syntax: []string{
			`ship SHIP-ID name "NAME"`,
			`colony COLONY-ID name "NAME"`,
			`we name (X,Y,Z) "NAME"`,
			`we name (X,Y,Z) system SYSTEM "NAME"`,
			`we name (X,Y,Z) system SYSTEM orbit ORBIT "NAME"`,
			`we name (player | faction) FACTION-ID "NAME"`,
			`we name (player | faction) FACTION-ID (ship | colony) ID "NAME"`,
		},
		Phase: PhaseNaming,
		Decode: func(actor int64, encoded string) (Params, error) {
			order := NameParams{Entity: actor}
			if err := json.Unmarshal([]byte(encoded), &order); err != nil {
				return nil, err
			}
			return order, nil
		},
		Parse: func(subject Subject, line *Line) (Params, error) {
			order := NameParams{Kind: subject.Kind, Entity: subject.ID}
			var err error
			// Naming something you own is an order to the thing itself, so the
			// subject is the thing. Naming a place is a faction order, because
			// no ship or colony carries it out, and the place follows the verb.
			if subject.Kind == SubjectFaction {
				// A faction, or one of its ships or colonies, is named the same
				// way a place is: by `we`, because nothing of the player's
				// carries the order out. What is not the same is that it can be
				// refused for a faction never encountered, and no encounter is
				// recorded yet -- so those two forms parse and do not yet act.
				if as, ok := line.keyword("player", "faction"); ok {
					other := &FactionRef{As: as}
					if other.ID, err = line.entityID("faction"); err != nil {
						return nil, err
					}
					if word, ok := line.peek(); ok && !word.quoted &&
						(strings.EqualFold(word.text, SubjectShip) || strings.EqualFold(word.text, SubjectColony)) {
						entity, err := line.entityRef()
						if err != nil {
							return nil, err
						}
						other.Entity = &entity
					}
					order.Faction = other
					order.Name, err = line.quoted("name")
					return order, err
				}
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
		Verb:     "create",
		Subjects: []string{SubjectShip, SubjectColony},
		Summary:  "begin building a ship or a colony, as fast as the materials, transports, and workers allow",
		Syntax: []string{
			"ship SHIP-ID create ship using QUANTITY UNIT, ... transfering QUANTITY UNIT, ... with QUANTITY CWKR end",
			"ship SHIP-ID create (open-air | enclosed | orbital) colony [as trade-station] using QUANTITY UNIT, ... transfering QUANTITY UNIT, ... with QUANTITY CWKR end",
			"colony COLONY-ID create ship using QUANTITY UNIT, ... transfering QUANTITY UNIT, ... with QUANTITY CWKR end",
			"colony COLONY-ID create (open-air | enclosed | orbital) colony [as trade-station] using QUANTITY UNIT, ... transfering QUANTITY UNIT, ... with QUANTITY CWKR end",
			"colony COLONY-ID create factory-group with QUANTITY UNIT, ... making UNIT",
			"ship SHIP-ID create farm-group with QUANTITY UNIT",
			"colony COLONY-ID create farm-group with QUANTITY UNIT",
			"colony COLONY-ID create mine-group with QUANTITY UNIT working deposit DEPOSIT-NO",
		},
		Phase: PhaseCreate,
		// A ship or colony create is long enough to want breaking over several
		// lines, so it runs to `end` rather than to the end of a line. A group
		// create is one line and takes no terminator.
		Terminator: func(form string) string {
			if _, ok := createKinds[strings.ToLower(form)]; ok {
				return "end"
			}
			return ""
		},
		// One Spec, two completion models. A ship or colony create is a
		// commitment that runs for turns; a group create is kill-and-fill and
		// closes out inside stage 5. They bind to different Bounds, which is
		// why Decode has to tell them apart, and it tells them apart by the
		// group the JSON names.
		Decode: func(actor int64, encoded string) (Params, error) {
			var form struct {
				Group string `json:"group"`
			}
			if err := json.Unmarshal([]byte(encoded), &form); err != nil {
				return nil, err
			}
			if form.Group != "" {
				return decode(encoded, CreateGroupParams{actorOf: actorOf{EntityID: actor}})
			}
			order := CreateParams{EntityID: actor}
			if err := json.Unmarshal([]byte(encoded), &order); err != nil {
				return nil, err
			}
			return order, nil
		},
		Parse: func(subject Subject, line *Line) (Params, error) {
			if group, ok := line.keyword(groupKinds...); ok {
				return parseCreateGroup(subject, line, group)
			}
			order := CreateParams{Kind: subject.Kind, EntityID: subject.ID}
			form, ok := line.keyword(buildsShip, buildsOpenAir, buildsEnclosed, buildsOrbital)
			if !ok {
				return nil, badSyntax("expected ship, a kind of colony, or a kind of group")
			}
			order.Builds = form
			if form != buildsShip {
				if err := line.expect("colony"); err != nil {
					return nil, err
				}
				// What a trade station confers is stage 11's business and is
				// not written. The grammar accepts it now so that a build
				// begun today is the thing the player asked for when it is.
				if _, ok := line.keyword("as"); ok {
					if err := line.expect("trade-station"); err != nil {
						return nil, err
					}
					order.TradeStation = true
				}
			}
			var err error
			if err = line.expect("using"); err != nil {
				return nil, err
			}
			if order.Using, err = line.unitList(); err != nil {
				return nil, err
			}
			if err = line.expect("transfering"); err != nil {
				return nil, err
			}
			if order.Transfering, err = line.unitList(); err != nil {
				return nil, err
			}
			if err = line.expect("with"); err != nil {
				return nil, err
			}
			if order.Workers, err = line.quantity("a quantity of " + cadre.Unit); err != nil {
				return nil, err
			}
			if err = line.expect(cadre.Unit); err != nil {
				return nil, err
			}
			return order, line.expect("end")
		},
	})

	register(&Spec{
		Verb:     "assemble",
		Subjects: []string{SubjectShip, SubjectColony},
		Summary:  "put unassembled units to work, as much of them as the construction workers manage",
		Syntax: []string{
			"ship SHIP-ID assemble QUANTITY UNIT, QUANTITY UNIT, ...",
			"colony COLONY-ID assemble QUANTITY UNIT, QUANTITY UNIT, ...",
		},
		Phase: PhaseAssemble,
		Decode: func(actor int64, encoded string) (Params, error) {
			order := AssembleParams{EntityID: actor}
			if err := json.Unmarshal([]byte(encoded), &order); err != nil {
				return nil, err
			}
			return order, nil
		},
		Parse: func(subject Subject, line *Line) (Params, error) {
			order := AssembleParams{Kind: subject.Kind, EntityID: subject.ID}
			var err error
			if order.Units, err = line.unitList(); err != nil {
				return nil, err
			}
			return order, nil
		},
	})

	register(&Spec{
		Verb:     "unassemble",
		Subjects: []string{SubjectShip, SubjectColony},
		Summary:  "take working units apart again, optionally stowing them in cargo",
		Syntax: []string{
			"ship SHIP-ID unassemble QUANTITY UNIT, QUANTITY UNIT, ...",
			"colony COLONY-ID unassemble QUANTITY UNIT, QUANTITY UNIT, ...",
			"ship SHIP-ID unassemble and stow QUANTITY UNIT, QUANTITY UNIT, ...",
			"colony COLONY-ID unassemble and stow QUANTITY UNIT, QUANTITY UNIT, ...",
		},
		Phase: PhaseUnassemble,
		Decode: func(actor int64, encoded string) (Params, error) {
			order := UnassembleParams{EntityID: actor}
			if err := json.Unmarshal([]byte(encoded), &order); err != nil {
				return nil, err
			}
			return order, nil
		},
		Parse: func(subject Subject, line *Line) (Params, error) {
			order := UnassembleParams{Kind: subject.Kind, EntityID: subject.ID}
			// Stowing puts the units down in cargo instead of leaving them in
			// unassembled inventory, which is what a transfer needs.
			if _, ok := line.keyword("and"); ok {
				if err := line.expect("stow"); err != nil {
					return nil, err
				}
				order.Stow = true
			}
			var err error
			if order.Units, err = line.unitList(); err != nil {
				return nil, err
			}
			return order, nil
		},
	})

	register(&Spec{
		Verb:     "stow",
		Subjects: []string{SubjectShip, SubjectColony},
		Summary:  "move units out of unassembled inventory into cargo, ready to be carried",
		Syntax: []string{
			"ship SHIP-ID stow QUANTITY UNIT, QUANTITY UNIT, ...",
			"colony COLONY-ID stow QUANTITY UNIT, QUANTITY UNIT, ...",
		},
		Phase: PhaseStow,
		Decode: func(actor int64, encoded string) (Params, error) {
			order := StowParams{EntityID: actor}
			if err := json.Unmarshal([]byte(encoded), &order); err != nil {
				return nil, err
			}
			return order, nil
		},
		Parse: func(subject Subject, line *Line) (Params, error) {
			order := StowParams{Kind: subject.Kind, EntityID: subject.ID}
			var err error
			if order.Units, err = line.unitList(); err != nil {
				return nil, err
			}
			return order, nil
		},
	})

	register(&Spec{
		Verb:     "unstow",
		Subjects: []string{SubjectShip, SubjectColony},
		Summary:  "move units out of cargo into unassembled inventory, ready to be sold",
		Syntax: []string{
			"ship SHIP-ID unstow QUANTITY UNIT, QUANTITY UNIT, ...",
			"colony COLONY-ID unstow QUANTITY UNIT, QUANTITY UNIT, ...",
		},
		Phase: PhaseUnstow,
		Decode: func(actor int64, encoded string) (Params, error) {
			order := UnstowParams{EntityID: actor}
			if err := json.Unmarshal([]byte(encoded), &order); err != nil {
				return nil, err
			}
			return order, nil
		},
		Parse: func(subject Subject, line *Line) (Params, error) {
			order := UnstowParams{Kind: subject.Kind, EntityID: subject.ID}
			var err error
			if order.Units, err = line.unitList(); err != nil {
				return nil, err
			}
			return order, nil
		},
	})

	register(&Spec{
		Verb:     "transfer",
		Subjects: []string{SubjectShip, SubjectColony},
		Summary:  "hand units or population to another entity at the same place, by transport",
		Syntax: []string{
			"ship SHIP-ID transfer QUANTITY UNIT, QUANTITY UNIT, ... to ship SHIP-ID",
			"ship SHIP-ID transfer QUANTITY UNIT, QUANTITY UNIT, ... to colony COLONY-ID",
			"colony COLONY-ID transfer QUANTITY UNIT, QUANTITY UNIT, ... to ship SHIP-ID",
			"colony COLONY-ID transfer QUANTITY UNIT, QUANTITY UNIT, ... to colony COLONY-ID",
		},
		Phase: PhaseTransfer,
		Decode: func(actor int64, encoded string) (Params, error) {
			order := TransferParams{EntityID: actor}
			if err := json.Unmarshal([]byte(encoded), &order); err != nil {
				return nil, err
			}
			return order, nil
		},
		Parse: func(subject Subject, line *Line) (Params, error) {
			order := TransferParams{Kind: subject.Kind, EntityID: subject.ID}
			var err error
			if order.Units, err = line.unitList(); err != nil {
				return nil, err
			}
			if err := line.expect("to"); err != nil {
				return nil, err
			}
			kind, ok := line.keyword(SubjectShip, SubjectColony)
			if !ok {
				return nil, badSyntax("expected ship or colony")
			}
			order.RecipientAs = kind
			if order.Recipient, err = line.entityID(kind); err != nil {
				return nil, err
			}
			return order, nil
		},
	})

	register(&Spec{
		Verb:     "probe",
		Subjects: []string{SubjectShip, SubjectColony},
		Summary:  "read planets with a ship's or a colony's sensors, one probe per orbit",
		Syntax: []string{
			"ship SHIP-ID probe orbit ORBIT ...",
			"colony COLONY-ID probe orbit ORBIT ...",
			"ship SHIP-ID probe system SYSTEM orbit ORBIT ...",
			"colony COLONY-ID probe system SYSTEM orbit ORBIT ...",
		},
		Phase: PhaseProbe,
		Decode: func(actor int64, encoded string) (Params, error) {
			order := ProbeParams{EntityID: actor}
			if err := json.Unmarshal([]byte(encoded), &order); err != nil {
				return nil, err
			}
			return order, nil
		},
		Parse: func(subject Subject, line *Line) (Params, error) {
			order := ProbeParams{Kind: subject.Kind, EntityID: subject.ID}
			var err error
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
//
// A ship moves once a turn. It may still jump in the same turn, which is what
// a ship at a planet does to leave, but the one move is the whole of what it
// does inside its stellium.
func (p MoveParams) Bind(b *Binder) ([]Bound, error) {
	ship, err := b.actor(p.ShipID, "ship")
	if err != nil {
		return nil, err
	}
	if err := b.once("move", ship); err != nil {
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
//
// A jump begins from the stellium orbit, so where the ship stands is part of
// what binds. It binds in the jump phase, which is after every MOVE has
// resolved, so a ship ordered out to the stellium orbit and then away in one
// file is measured as the move left it. A move that failed leaves the ship at
// its planet, and the jump behind it fails for the same reason.
//
// Nothing here measures the jump against a range. A drive's technology level
// does not cap how far it goes; it divides the distance to give the turns the
// crossing takes, so the only limit on a long jump is the FUEL it burns, which
// is linear in the distance.
func (p JumpParams) Bind(b *Binder) ([]Bound, error) {
	ship, err := b.actor(p.ShipID, "ship")
	if err != nil {
		return nil, err
	}
	// A ship jumps once a turn. A jump that departs says so by itself, because
	// the ship is then in transit and b.actor turns away everything; this is
	// what catches the second jump after a first that failed for want of fuel,
	// which leaves the ship where it was.
	if err := b.once("jump", ship); err != nil {
		return nil, err
	}
	destinationID := b.World.StelliumAt(p.X, p.Y, p.Z)
	if destinationID == 0 {
		return nil, fmt.Errorf("game %q has no stellium at (%d,%d,%d)", b.World.Game().Code, p.X, p.Y, p.Z)
	}
	if ship.Location.SystemID != 0 {
		return nil, fmt.Errorf("ship %d is at a planet and a jump begins from the stellium orbit; move it to orbit %d first",
			ship.ID, StelliumOrbit)
	}
	if !ship.Drive.Installed() {
		return nil, fmt.Errorf("ship %d has no assembled %s and cannot jump", ship.ID, jumpdrive.Unit)
	}
	if !ship.Drive.CanPropel(ship.Mass) {
		return nil, fmt.Errorf("ship %d masses %d MU and its jump drive propels %d MU",
			ship.ID, ship.Mass, ship.Drive.Capacity)
	}
	from := b.World.Coordinates(ship.Location.StelliumID)
	lightYears := jumpdrive.Distance(from.X, from.Y, from.Z, p.X, p.Y, p.Z)
	return []Bound{&jumpBound{
		ship: ship, x: p.X, y: p.Y, z: p.Z, destinationID: destinationID,
		cost:  ship.Drive.FuelForJump(lightYears),
		turns: ship.Drive.TurnsForJump(lightYears),
	}}, nil
}

type jumpBound struct {
	ship          *world.Entity
	x, y, z       int
	destinationID int64
	cost          int64
	// turns is how long the crossing takes, never fewer than one. It is the
	// distance divided by the drive's technology level and rounded up, which is
	// the whole of what a better drive buys.
	turns int
}

// Params is the jump as it will be stored: the point it is bound for.
func (o *jumpBound) Params() Params {
	return JumpParams{ShipID: o.ship.ID, X: o.x, Y: o.y, Z: o.z}
}

func (o *jumpBound) Fuel() int64 { return o.cost }

// Apply departs. The whole fuel bill is drawn here, however many turns the
// crossing takes, so a ship that cannot pay for all of it never leaves; then
// the ship goes off the board and the crossing continues without it.
//
// What the order records as its final location is the destination's stellium
// orbit, which is where the jump sent the ship -- a jump arrives in the
// stellium orbit, because a ship crosses to a planet under its own power, with
// a MOVE. That is where the ship ends up rather than where it stands tonight:
// order_movement says what the order did, and what the order did was send the
// ship there. Where the ship is in the meantime is the crossing's business, and
// the turn report is what answers that.
func (o *jumpBound) Apply(t *Turn) (Outcome, error) {
	start := o.ship.Location
	message, err := spendFuel(t, o.ship, "jump", o.cost)
	if err != nil {
		return Outcome{}, err
	}
	if message != "" {
		return failed(start, message), nil
	}
	// The crossing finishes on the last of its turns, so a one-turn crossing is
	// due in the turn it departed and the arrival step lands it before the turn
	// is out. That is what every jump did before a crossing could span turns.
	if err := t.World.Depart(o.ship, o.destinationID, t.Number+o.turns-1); err != nil {
		return Outcome{}, err
	}
	return succeeded(start, world.Location{StelliumID: o.destinationID}, o.cost), nil
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
	// Faction is another player's faction, or one of its ships or colonies,
	// when that is what is being named. A faction may only name one it has
	// encountered, and nothing records an encounter, so these two forms parse
	// and do not yet act.
	Faction *FactionRef `json:"faction,omitempty"`
	Name    string      `json:"name"`
}

// FactionRef is another faction as an order named it: the word the player wrote
// -- player or faction, which mean the same thing -- the id, and optionally one
// of its ships or colonies.
type FactionRef struct {
	As     string     `json:"as"`
	ID     int64      `json:"id"`
	Entity *entityRef `json:"entity,omitempty"`
}

// String renders the reference in the words the player used.
func (f FactionRef) String() string {
	if f.Entity != nil {
		return fmt.Sprintf("%s %d %s", f.As, f.ID, *f.Entity)
	}
	return fmt.Sprintf("%s %d", f.As, f.ID)
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
	if p.Faction != nil {
		return fmt.Sprintf("%s %q", *p.Faction, p.Name)
	}
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
	// Naming another faction, or one of its ships or colonies, is refused for
	// one it has never encountered -- and nothing records an encounter yet, so
	// the two forms parse and wait for the rule that would allow them.
	if p.Faction != nil {
		return unbuilt(b, "name", "", p)
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

// UNITS -----------------------------------------------------------------

// UnitQuantity is one item of an order that names units: how many, and the tag
// the player wrote them as. The tag is kept as written rather than split into
// a code and a level, because it is what the order file said and what the
// report prints back.
type UnitQuantity struct {
	Quantity int64  `json:"quantity"`
	Tag      string `json:"unit"`
}

// unitListInput renders the units an order named, in the words the player
// used: `4,500 GOLD, 18,000 FOOD`.
func unitListInput(items []UnitQuantity) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = fmt.Sprintf("%s %s", formatQuantity(item.Quantity), item.Tag)
	}
	return strings.Join(parts, ", ")
}

// unitWork is one lot of units an assemble or unassemble order named, resolved
// to the sections it moves between.
//
// from is a draw order rather than one section, because an assemble has two
// places to look. Which of them a lot actually comes out of depends on what is
// where when the order runs, so the split is Apply's; the list is Bind's.
type unitWork struct {
	tag       string
	unit      string
	techLevel int
	quantity  int64
	from      []string
	to        string
}

// assembleSources is where an assemble takes its units from, in order.
//
// Unassembled inventory goes first: it is what an unassemble leaves behind and
// what the market deals in, so it is the section units are kept in to be worked
// on. Cargo goes second because a transport sets its load down there and
// nowhere else, and stage 10 assembles what stage 9 delivered -- without this
// the unassemble-carry-assemble pipeline the turn sequence is arranged around
// would stop one step short at the far end.
var assembleSources = []string{units.SectionUnassembled, units.SectionCargo}

// route is where one lot of units comes from and where it goes, for the order
// asking. It also refuses a tag that order cannot move at all, which differs
// between them: an assemble puts things together and a stow only carries them,
// so a resource is a mistake in the one and freight in the other.
type route func(tag, unit string) (from []string, to string, err error)

// bindUnitWork resolves the tags an order named into the moves through
// inventory it is asking for.
//
// Which sections a unit moves between is a property of the unit code and the
// verb, so it cannot change during a turn and belongs here rather than in
// Apply. So does naming something the order can never move, and naming the
// same unit twice -- both are mistakes in the file rather than things the
// world might yet oblige.
func bindUnitWork(items []UnitQuantity, where route) ([]unitWork, error) {
	var found bindErrors
	seen := make(map[string]bool, len(items))
	work := make([]unitWork, 0, len(items))
	for _, item := range items {
		unit, techLevel, _, err := units.ParseTag(item.Tag)
		if err != nil {
			found = append(found, err)
			continue
		}
		if seen[item.Tag] {
			found = append(found, fmt.Errorf("%s is named twice", item.Tag))
			continue
		}
		seen[item.Tag] = true
		from, to, err := where(item.Tag, unit)
		if err != nil {
			found = append(found, err)
			continue
		}
		work = append(work, unitWork{tag: item.Tag, unit: unit, techLevel: techLevel,
			quantity: item.Quantity, from: from, to: to})
	}
	if len(found) != 0 {
		return nil, found
	}
	return work, nil
}

// toWork puts a unit to work. Nothing in the order says where it goes; the
// unit code does.
func toWork(tag, unit string) ([]string, string, error) {
	assembled, assemblable := units.AssembledSection(unit)
	if !assemblable {
		return nil, "", notAssemblable(tag, unit)
	}
	return assembleSources, assembled, nil
}

// outOfWork takes a unit apart again, leaving it in unassembled inventory or,
// when the order said `and stow`, straight in cargo.
func outOfWork(stow bool) route {
	return func(tag, unit string) ([]string, string, error) {
		assembled, assemblable := units.AssembledSection(unit)
		if !assemblable {
			return nil, "", notAssemblable(tag, unit)
		}
		if stow {
			return []string{assembled}, units.SectionCargo, nil
		}
		return []string{assembled}, units.SectionUnassembled, nil
	}
}

// asFreight moves a unit between unassembled inventory and cargo without
// taking it apart or putting it together.
//
// What may be named is wider than what may be assembled, because nothing is
// being made: the bulk resources are freight like anything else, and unstowing
// them is what readies them for the market. Population and cadres are still
// refused, neither being inventory at all.
func asFreight(from, to string) route {
	return func(tag, unit string) ([]string, string, error) {
		if units.IsPopulation(unit) || units.IsCadre(unit) {
			return nil, "", notFreight(tag, unit)
		}
		return []string{from}, to, nil
	}
}

// notAssemblable says why a thing an order named is never put together, in the
// terms of what it actually is.
func notAssemblable(tag, unit string) error {
	switch {
	case units.IsPopulation(unit):
		return fmt.Errorf("%s is population; people are carried and fed, not assembled", tag)
	case units.IsCadre(unit):
		return fmt.Errorf("%s is a cadre, an assignment of people rather than a unit, and is never assembled", tag)
	default:
		return fmt.Errorf("%s is a resource; it is measured rather than made, and is never assembled", tag)
	}
}

// notFreight says why a thing a stow or an unstow named is never stowed. Only
// two things are not: people ride a transport rather than being loaded onto
// one, and a cadre is not a thing at all.
func notFreight(tag, unit string) error {
	if units.IsPopulation(unit) {
		return fmt.Errorf("%s is population; people are carried rather than stowed", tag)
	}
	return fmt.Errorf("%s is a cadre, an assignment of people rather than a unit, and is never stowed", tag)
}

// ASSEMBLE and UNASSEMBLE ------------------------------------------------

// AssembleParams is an ASSEMBLE as written: an entity and the units it is to
// put to work.
type AssembleParams struct {
	Kind     string         `json:"-"`
	EntityID int64          `json:"-"`
	Units    []UnitQuantity `json:"units"`
}

// Actor is the entity whose construction workers do the assembling.
func (p AssembleParams) Actor() int64 { return p.EntityID }

// Input is the order as the player wrote it.
func (p AssembleParams) Input() string { return unitListInput(p.Units) }

// Bind settles which sections the units move between, which the unit codes
// decide and a turn cannot change.
func (p AssembleParams) Bind(b *Binder) ([]Bound, error) {
	entity, err := b.actor(p.EntityID, p.Kind)
	if err != nil {
		return nil, err
	}
	work, err := bindUnitWork(p.Units, toWork)
	if err != nil {
		return nil, err
	}
	return []Bound{&workBound{params: p, entity: entity, pool: cadre.Assembly,
		verb: "assembled", rate: cadreRate, work: work}}, nil
}

// UnassembleParams is an UNASSEMBLE as written. Stow says the units are put
// down in cargo rather than left in unassembled inventory, which is what makes
// them ready to be carried away.
type UnassembleParams struct {
	Kind     string         `json:"-"`
	EntityID int64          `json:"-"`
	Stow     bool           `json:"stow,omitempty"`
	Units    []UnitQuantity `json:"units"`
}

// Actor is the entity whose construction workers do the taking apart.
func (p UnassembleParams) Actor() int64 { return p.EntityID }

// Input is the order as the player wrote it.
func (p UnassembleParams) Input() string {
	if p.Stow {
		return "and stow " + unitListInput(p.Units)
	}
	return unitListInput(p.Units)
}

// Bind settles which sections the units move between. Unassembly is lossless:
// what comes apart is what went together, so nothing here weighs a yield.
func (p UnassembleParams) Bind(b *Binder) ([]Bound, error) {
	entity, err := b.actor(p.EntityID, p.Kind)
	if err != nil {
		return nil, err
	}
	work, err := bindUnitWork(p.Units, outOfWork(p.Stow))
	if err != nil {
		return nil, err
	}
	return []Bound{&workBound{params: p, entity: entity, pool: cadre.Unassembly,
		verb: "unassembled", rate: cadreRate, work: work}}, nil
}

// STOW and UNSTOW ------------------------------------------------------

// StowParams is a STOW as written: an entity and the units it is to put down
// in cargo.
type StowParams struct {
	Kind     string         `json:"-"`
	EntityID int64          `json:"-"`
	Units    []UnitQuantity `json:"units"`
}

// Actor is the entity whose production labour moves the freight.
func (p StowParams) Actor() int64 { return p.EntityID }

// Input is the order as the player wrote it.
func (p StowParams) Input() string { return unitListInput(p.Units) }

// Bind settles which sections the units move between, which the verb decides
// and a turn cannot change.
func (p StowParams) Bind(b *Binder) ([]Bound, error) {
	entity, err := b.actor(p.EntityID, p.Kind)
	if err != nil {
		return nil, err
	}
	work, err := bindUnitWork(p.Units, asFreight(units.SectionUnassembled, units.SectionCargo))
	if err != nil {
		return nil, err
	}
	return []Bound{&workBound{params: p, entity: entity, pool: labour.Stowing,
		verb: "stowed", rate: labourRate, work: work}}, nil
}

// UnstowParams is an UNSTOW as written: an entity and the units it is to take
// back out of cargo.
type UnstowParams struct {
	Kind     string         `json:"-"`
	EntityID int64          `json:"-"`
	Units    []UnitQuantity `json:"units"`
}

// Actor is the entity whose production labour moves the freight.
func (p UnstowParams) Actor() int64 { return p.EntityID }

// Input is the order as the player wrote it.
func (p UnstowParams) Input() string { return unitListInput(p.Units) }

// Bind settles which sections the units move between.
func (p UnstowParams) Bind(b *Binder) ([]Bound, error) {
	entity, err := b.actor(p.EntityID, p.Kind)
	if err != nil {
		return nil, err
	}
	work, err := bindUnitWork(p.Units, asFreight(units.SectionCargo, units.SectionUnassembled))
	if err != nil {
		return nil, err
	}
	return []Bound{&workBound{params: p, entity: entity, pool: labour.Unstowing,
		verb: "unstowed", rate: labourRate, work: work}}, nil
}

// workBound is any of the four orders that move units between the sections of
// one entity. They are one thing in four directions: each is rationed by a
// pool of workers at 500 MU a turn, each fills partway when the workers or the
// stock run out, and each fails only when the result would not fit. What
// separates them is which way the units move, which pool the order draws on,
// and who the workers are.
type workBound struct {
	params Params
	entity *world.Entity
	// pool is the pool of work this order draws on. Two pools of one kind
	// never pool with each other, so which one an order is in is the whole of
	// what separates the rounding of the one from the rounding of the other.
	pool string
	// verb is the past tense the note reads in: assembled, unassembled,
	// stowed, unstowed.
	verb string
	// rate is the note's account of who does this kind of work and how much of
	// it they had left when the order ran, for an order that outran them.
	rate func(entity *world.Entity, allowed int64) string
	work []unitWork
}

// cadreRate and labourRate say who did the work and how much of it they had
// left, for the note an order that outran them carries.
//
// What was left rather than what a full turn is worth: a worker does one task
// per turn and the pool is drawn down in phase order, so an order that came
// second is answered by whatever the first one left it. Quoting the whole
// turn's rate at a player whose second order stopped short of it is quoting a
// number that does not explain the one above it.
func cadreRate(entity *world.Entity, allowed int64) string {
	return fmt.Sprintf("its %s %s had %s MU of work left this turn",
		formatQuantity(entity.ConstructionWorkers()), cadre.Unit, formatQuantity(allowed))
}

func labourRate(entity *world.Entity, allowed int64) string {
	return fmt.Sprintf("its %s units of production labour had %s MU of freight left this turn",
		formatQuantity(entity.ProductionLabour()), formatQuantity(allowed))
}

// Params is the order as it will be stored.
func (o *workBound) Params() Params { return o.params }

// Fuel is nothing. Construction workers are people, and people are paid rather
// than burned.
func (o *workBound) Fuel() int64 { return 0 }

// Apply does as much of the work as the entity's workers and its stock allow.
//
// A shortage is a rate rather than a failure: an order that asks for more than
// the pool can do this turn, or for more than the entity holds, does what it
// can and says so. It fails for one reason only -- that the result would not
// fit -- because an entity cannot hold more than it encloses, and that is not
// something doing less of the order would fix.
func (o *workBound) Apply(t *Turn) (Outcome, error) {
	at := o.entity.Location
	allowed := t.World.WorkAllowed(o.pool, o.entity)
	done := int64(0)
	shifts := make([]world.Shift, 0, len(o.work))
	var short []string
	workersBound := false
	for _, want := range o.work {
		// The work is the mass being handled, so a heavier unit is more work.
		// A unit that masses nothing is no work at all and is not rationed.
		unitMass := units.MetricsForStored(want.unit, want.techLevel).Mass
		moved := int64(0)
		for _, section := range want.from {
			if moved == want.quantity {
				break
			}
			quantity := min(want.quantity-moved, o.entity.Held(section, want.unit, want.techLevel))
			if unitMass > 0 {
				if room := (allowed - done) / unitMass; room < quantity {
					quantity, workersBound = max(room, 0), true
				}
				done += quantity * unitMass
			}
			if quantity == 0 {
				continue
			}
			shifts = append(shifts, world.Shift{From: section, To: want.to,
				Unit: want.unit, TechLevel: want.techLevel, Quantity: quantity})
			moved += quantity
		}
		if moved != want.quantity {
			short = append(short, fmt.Sprintf("%s of %s %s",
				formatQuantity(moved), formatQuantity(want.quantity), want.tag))
		}
	}
	occupied, usable, err := o.entity.RoomAfter(shifts)
	if err != nil {
		return Outcome{}, err
	}
	if occupied > usable {
		return failed(at, fmt.Sprintf("%s %d would hold %s VU in %s VU of enclosed space",
			noun(o.entity), o.entity.ID, formatQuantity(occupied), formatQuantity(usable))), nil
	}
	if err := t.World.ShiftAll(o.entity, shifts); err != nil {
		return Outcome{}, err
	}
	t.World.RecordWork(o.pool, o.entity.ID, done)
	item := succeeded(at, at, 0)
	if len(short) != 0 {
		reason := "it holds no more"
		if workersBound {
			reason = o.rate(o.entity, allowed)
		}
		item.Note = fmt.Sprintf("%s %d %s %s; %s",
			noun(o.entity), o.entity.ID, o.verb, strings.Join(short, ", "), reason)
	}
	return item, nil
}

// TRANSFER --------------------------------------------------------------

// TransferParams is a TRANSFER as written: an entity, what it is handing over,
// and who it is handing it to.
//
// The recipient's id is stored, which is not the same thing as storing a
// resolved id: it is the number the player typed, and Bind looks it up again
// when the turn runs, so a recipient that has since gone is a rejected file
// rather than a corrupt row.
type TransferParams struct {
	Kind        string         `json:"-"`
	EntityID    int64          `json:"-"`
	Units       []UnitQuantity `json:"units"`
	Recipient   int64          `json:"to"`
	RecipientAs string         `json:"to_kind"`
}

// Actor is the entity handing the units over. Its transports do the carrying,
// and it pays the fuel.
func (p TransferParams) Actor() int64 { return p.EntityID }

// Input is the transfer as the player wrote it.
func (p TransferParams) Input() string {
	return fmt.Sprintf("%s to %s %d", unitListInput(p.Units), p.RecipientAs, p.Recipient)
}

// Bind finds the recipient and checks that the units named are things that can
// be handed over at all. Where the two entities are is not settled here: a
// ship moves during a turn, so co-location is measured when the order runs.
func (p TransferParams) Bind(b *Binder) ([]Bound, error) {
	entity, err := b.actor(p.EntityID, p.Kind)
	if err != nil {
		return nil, err
	}
	recipient, err := b.recipient(p.Recipient, p.RecipientAs)
	if err != nil {
		return nil, err
	}
	if recipient.ID == entity.ID {
		return nil, fmt.Errorf("%s %d cannot transfer to itself", noun(entity), entity.ID)
	}
	var found bindErrors
	seen := make(map[string]bool, len(p.Units))
	items := make([]transferItem, 0, len(p.Units))
	for _, item := range p.Units {
		unit, techLevel, _, err := units.ParseTag(item.Tag)
		if err != nil {
			found = append(found, err)
			continue
		}
		if seen[item.Tag] {
			found = append(found, fmt.Errorf("%s is named twice", item.Tag))
			continue
		}
		seen[item.Tag] = true
		// A cadre is an assignment rather than a thing: the people in it are
		// already counted as population, so a transfer names the population.
		if units.IsCadre(unit) {
			found = append(found, fmt.Errorf("%s is a cadre, an assignment of people rather than a thing to carry; transfer the population instead", item.Tag))
			continue
		}
		items = append(items, transferItem{
			tag: item.Tag, unit: unit, techLevel: techLevel, quantity: item.Quantity,
			population: units.IsPopulation(unit),
		})
	}
	if len(found) != 0 {
		return nil, found
	}
	return []Bound{&transferBound{params: p, entity: entity, recipient: recipient, items: items}}, nil
}

// transferItem is one lot a transfer carries, resolved. Population is not
// inventory -- it has a table of its own and is never assembled -- but it
// rides the same transports and is charged the same mass and volume.
type transferItem struct {
	tag        string
	unit       string
	techLevel  int
	quantity   int64
	population bool
}

// metrics is what one of these masses and what room it takes as freight.
func (i transferItem) metrics() units.Metrics {
	if i.population {
		return units.PopulationMetrics
	}
	return units.MetricsForStored(i.unit, i.techLevel)
}

type transferBound struct {
	params    TransferParams
	entity    *world.Entity
	recipient *world.Entity
	items     []transferItem
	// spent is the FUEL the transports cost, filled in when the order runs. A
	// transfer's bill depends on the load, so it is not known until the load
	// is, and what is stored with a pending order is what the dry run found.
	spent int64
}

// Params is the transfer as it will be stored.
func (o *transferBound) Params() Params { return o.params }

func (o *transferBound) Fuel() int64 { return o.spent }

// Apply carries as much as the transports take.
//
// Three things are weighed here rather than at Bind, because a turn can change
// all three: where the two entities are, what is in the sending entity's
// cargo, and how many of its transports are still free. The first is a failure
// -- there is no partial answer to being in the wrong place -- and the other
// two fill the order partway, which is what the transports rule says outright.
func (o *transferBound) Apply(t *Turn) (Outcome, error) {
	at := o.entity.Location
	if !sameBerth(o.entity.Location, o.recipient.Location) {
		return failed(at, fmt.Sprintf("%s %d and %s %d are not at the same place",
			noun(o.entity), o.entity.ID, noun(o.recipient), o.recipient.ID)), nil
	}
	free := t.World.TransportsFree(o.entity)
	capacity := transport.Capacity(free)
	var carried transport.Load
	var short []string
	loaded := make([]transferItem, 0, len(o.items))
	for _, item := range o.items {
		quantity := item.quantity
		if held := o.held(item); held < quantity {
			quantity = held
		}
		metrics := item.metrics()
		// Both limits hold, so what a lot can take is whichever runs out
		// first. A lot that masses nothing and takes no room is not rationed.
		if metrics.Mass > 0 {
			quantity = min(quantity, (capacity.Mass-carried.Mass)/metrics.Mass)
		}
		if metrics.CargoVolume > 0 {
			quantity = min(quantity, (capacity.Volume-carried.Volume)/metrics.CargoVolume)
		}
		quantity = max(quantity, 0)
		if quantity != item.quantity {
			short = append(short, fmt.Sprintf("%s of %s %s",
				formatQuantity(quantity), formatQuantity(item.quantity), item.tag))
		}
		carried.Mass += quantity * metrics.Mass
		carried.Volume += quantity * metrics.CargoVolume
		item.quantity = quantity
		loaded = append(loaded, item)
	}
	// The fuel is reckoned over the hulls the load actually needs, so a small
	// transfer is not charged for every transport the entity owns.
	used := transport.Pack(free, carried)
	cost := transport.Fuel(t.World.Squares(o.entity.ID)+transport.Squares(used)) -
		transport.Fuel(t.World.Squares(o.entity.ID))
	message, err := spendFuel(t, o.entity, "run its transports", cost)
	if err != nil {
		return Outcome{}, err
	}
	if message != "" {
		return failed(at, message), nil
	}
	o.spent = cost
	t.World.CommitTransports(o.entity, used)
	for _, item := range loaded {
		if item.quantity == 0 {
			continue
		}
		if item.population {
			err = t.World.HandPopulation(o.entity, o.recipient, item.unit, item.quantity)
		} else {
			err = t.World.Hand(o.entity, o.recipient, item.unit, item.techLevel, item.quantity)
		}
		if err != nil {
			return Outcome{}, err
		}
	}
	item := succeeded(at, at, cost)
	if len(short) != 0 {
		item.Note = fmt.Sprintf("%s %d transferred %s; %s",
			noun(o.entity), o.entity.ID, strings.Join(short, ", "), o.shortfall(free))
	}
	return item, nil
}

// held is how much of a lot the sending entity has to hand. Units must be in
// cargo to be transferred; population is wherever population is.
func (o *transferBound) held(item transferItem) int64 {
	if item.population {
		return o.entity.Population[item.unit]
	}
	return o.entity.Held(units.SectionCargo, item.unit, item.techLevel)
}

// shortfall says which of the two ran out, for the note on a partly filled
// transfer.
func (o *transferBound) shortfall(free []transport.Hulls) string {
	if len(free) == 0 {
		return fmt.Sprintf("it had no %s free this turn", transport.Unit)
	}
	capacity := transport.Capacity(free)
	return fmt.Sprintf("it had %s MU and %s VU of transport left this turn",
		formatQuantity(capacity.Mass), formatQuantity(capacity.Volume))
}

// sameBerth reports whether two entities are at the same place. The ring is
// not part of it: a ring is drawn afresh every time a ship settles at a
// planet, and a transport crossing between two rings of one planet has gone
// nowhere worth charging for.
func sameBerth(a, b world.Location) bool {
	return a.StelliumID != 0 && a.StelliumID == b.StelliumID &&
		a.SystemID == b.SystemID && a.PlanetID == b.PlanetID
}

// CREATE ----------------------------------------------------------------

// The kinds of thing a create builds, in the words the player writes. A trade
// station is not among them: it is an orbital colony with a flag on it.
const (
	buildsShip     = "ship"
	buildsOpenAir  = "open-air"
	buildsEnclosed = "enclosed"
	buildsOrbital  = "orbital"
)

// createKinds maps what the player wrote to the entity unit it becomes.
var createKinds = map[string]string{
	buildsShip:     "SHIP",
	buildsOpenAir:  "COPN",
	buildsEnclosed: "CSFC",
	buildsOrbital:  "CORB",
}

// CreateParams is a CREATE as written: who is building, what kind of thing, the
// two lists, and the ceiling on the workers a turn may use.
//
// A create is a commitment rather than a purchase. The order says build this as
// fast as you can and it succeeds the moment it is given; everything after that
// is rate rather than failure, which is the opposite of most orders in the game
// and the thing to hold on to when a detail below looks odd.
type CreateParams struct {
	Kind         string         `json:"-"`
	EntityID     int64          `json:"-"`
	Builds       string         `json:"builds"`
	TradeStation bool           `json:"trade_station,omitempty"`
	Using        []UnitQuantity `json:"using"`
	Transfering  []UnitQuantity `json:"transfering"`
	Workers      int64          `json:"with"`
}

// Actor is the entity that will feed the build: it claims from its stock,
// carries on its transports, and lends its construction workers a shift at a
// time.
func (p CreateParams) Actor() int64 { return p.EntityID }

// Input is the order as the player wrote it, on one line however many they
// spread it over.
func (p CreateParams) Input() string {
	kind := p.Builds
	if kind != buildsShip {
		kind += " colony"
		if p.TradeStation {
			kind += " as trade-station"
		}
	}
	return fmt.Sprintf("%s using %s transfering %s with %s %s", kind,
		unitListInput(p.Using), unitListInput(p.Transfering),
		formatQuantity(p.Workers), cadre.Unit)
}

// Bind settles what a turn cannot change: that the builder is the player's,
// that a colony has a planet to stand on and an open-air one air to breathe,
// and that the two lists name things a build can actually use.
//
// A shortfall of anything is never settled here. Materials, transports, and
// workers are all per-turn conditions and the next turn may cure them, so a
// build with none of them is a build that makes no progress rather than an
// order that failed.
func (p CreateParams) Bind(b *Binder) ([]Bound, error) {
	entity, err := b.actor(p.EntityID, p.Kind)
	if err != nil {
		return nil, err
	}
	kind, known := createKinds[p.Builds]
	if !known {
		return nil, fmt.Errorf("%q is not a thing a create builds", p.Builds)
	}
	var found bindErrors
	if kind != "SHIP" {
		// A colony is built at a planet, so the builder has to be at one. A
		// ship may be built from an entity in the stellium orbit and is built
		// there.
		if entity.Location.SystemID == 0 {
			found = append(found, fmt.Errorf("a colony is created at a planet, and %s %d is in the stellium orbit",
				noun(entity), entity.ID))
		} else if kind == "COPN" {
			planet, exists, err := b.World.PlanetByID(entity.Location.PlanetID)
			if err != nil {
				return nil, err
			}
			if exists && planet.Habitability == 0 {
				found = append(found, fmt.Errorf(
					"an open-air colony breathes the air outside and needs a planet whose habitability is above 0"))
			}
		}
	}
	items, err := buildItems(p)
	if err != nil {
		found = append(found, eachError(err)...)
	}
	if len(found) != 0 {
		return nil, found
	}
	return []Bound{&createBound{params: p, entity: entity, kind: kind, items: items}}, nil
}

// buildItems turns the two lists into the build's lines, numbered in the order
// the player wrote them, which is their priority.
//
// What may be named differs by clause, because the clauses do not mean the same
// thing. A using line names what the entity is made of, so it names something
// that can be assembled. A transfering line names what is handed over rather
// than built in, so it reaches cargo and people both. Neither reaches a cadre,
// which is an assignment of people rather than a thing to carry.
func buildItems(p CreateParams) ([]*world.BuildItem, error) {
	var found bindErrors
	items := make([]*world.BuildItem, 0, len(p.Using)+len(p.Transfering))
	for _, clause := range []struct {
		name  string
		lines []UnitQuantity
	}{{world.ClauseUsing, p.Using}, {world.ClauseTransfering, p.Transfering}} {
		seen := make(map[string]bool, len(clause.lines))
		for _, line := range clause.lines {
			unit, techLevel, _, err := units.ParseTag(line.Tag)
			if err != nil {
				found = append(found, err)
				continue
			}
			if seen[line.Tag] {
				found = append(found, fmt.Errorf("%s is named twice in the %s list", line.Tag, clause.name))
				continue
			}
			seen[line.Tag] = true
			if err := buildable(clause.name, line.Tag, unit); err != nil {
				found = append(found, err)
				continue
			}
			items = append(items, &world.BuildItem{
				Ordinal: len(items) + 1, Clause: clause.name,
				Unit: unit, TechLevel: techLevel, Required: line.Quantity,
			})
		}
	}
	if len(found) != 0 {
		return nil, found
	}
	return items, nil
}

// buildable says why a thing a create named cannot go on the line it was
// written on.
func buildable(clause, tag, unit string) error {
	if units.IsCadre(unit) {
		return fmt.Errorf("%s is a cadre, an assignment of people rather than a unit; name the population instead", tag)
	}
	if clause == world.ClauseTransfering {
		return nil
	}
	if _, assemblable := units.AssembledSection(unit); !assemblable {
		return fmt.Errorf("%s is not built into an entity; name it in the transfering list instead", tag)
	}
	return nil
}

// createBound is a CREATE whose builder and lists are settled.
type createBound struct {
	params CreateParams
	entity *world.Entity
	kind   string
	items  []*world.BuildItem
}

// Params is the order as it will be stored.
func (o *createBound) Params() Params { return o.params }

// Fuel is nothing. A create moves nothing on the turn it is given: it claims at
// stage 5, which needs no transport, and the fuel a delivery costs is charged
// when the delivery happens.
func (o *createBound) Fuel() int64 { return 0 }

// Apply puts the unfinished entity on the board. It cannot fail: everything a
// create could be refused for was settled at Bind, and everything else is rate.
//
// The entity exists from this moment. It belongs to the faction building it --
// the exception to "an entity with nobody aboard is uncontrolled", and the
// exception is the point, because an unfinished entity has no population yet by
// definition -- and it sits at a planet with a mass, so probes read it like
// anything else.
func (o *createBound) Apply(t *Turn) (Outcome, error) {
	at := o.entity.Location
	site := at
	switch o.kind {
	case "SHIP":
		// A ship built at a planet settles into a ring of its own, drawn the
		// way an arriving ship draws one. One built from an entity in the
		// stellium orbit is built there and has no ring at all.
		if at.SystemID != 0 {
			site.Ring = t.World.Game().Seed.RingFor(t.Number, t.FactionID, t.Sequence)
		}
	case "CORB":
		site.Ring = 1
	default:
		site.Ring = 0
	}
	build := &world.Build{
		BuilderID: o.entity.ID, WorkerCap: o.params.Workers,
		TradeStation: o.params.TradeStation, Items: o.items,
	}
	// The new entity takes the technology level of the entity that created it,
	// which settles the column with nothing to look up and nothing to write.
	created, err := t.World.CreateEntity(o.entity.FactionID, o.kind, o.entity.TechLevel, site, build)
	if err != nil {
		return Outcome{}, err
	}
	item := succeeded(at, at, 0)
	item.Note = buildProgress(created)
	return item, nil
}
