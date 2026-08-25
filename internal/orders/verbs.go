// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

var errExpectedActor = syntaxErr{message: "expected ship or colony"}

// The orders the game understands. Each one registers a Spec here and nowhere
// else: the parser finds it by verb, `ec orders help` prints its syntax, and a
// line that does not match reports this verb's forms rather than every form in
// the game.

func init() {
	register(&Spec{
		Verb:    "jump",
		Summary: "send a ship to another stellium, within its drive's range",
		Syntax:  []string{"jump ship SHIP-ID to (X,Y,Z)"},
		Parse: func(line *Line) (Order, error) {
			order := Order{Verb: "jump", Actor: "ship"}
			if err := line.expect("ship"); err != nil {
				return Order{}, err
			}
			var err error
			if order.ShipID, err = line.entityID("ship"); err != nil {
				return Order{}, err
			}
			if err := line.expect("to"); err != nil {
				return Order{}, err
			}
			if order.X, order.Y, order.Z, err = line.coordinates(); err != nil {
				return Order{}, err
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
		Parse: func(line *Line) (Order, error) {
			order := Order{Verb: "move", Actor: "ship"}
			if err := line.expect("ship"); err != nil {
				return Order{}, err
			}
			var err error
			if order.ShipID, err = line.entityID("ship"); err != nil {
				return Order{}, err
			}
			if err := line.expect("to"); err != nil {
				return Order{}, err
			}
			// A move may name a system of the ship's stellium, or leave the
			// system out and mean the one the ship is already in.
			if _, ok := line.keyword("system"); ok {
				if order.System, err = line.systemLetter(); err != nil {
					return Order{}, err
				}
			}
			if err := line.expect("orbit"); err != nil {
				return Order{}, err
			}
			if order.Orbit, err = line.number("orbit"); err != nil {
				return Order{}, err
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
		Parse: func(line *Line) (Order, error) {
			order := Order{Verb: "probe"}
			actor, ok := line.keyword("ship", "colony")
			if !ok {
				return Order{}, errExpectedActor
			}
			order.Actor = actor
			var err error
			if order.ShipID, err = line.entityID(actor); err != nil {
				return Order{}, err
			}
			// A probe that names a system reads any system of the entity's
			// stellium; one that does not reads the system it is in.
			if _, ok := line.keyword("system"); ok {
				if order.System, err = line.systemLetter(); err != nil {
					return Order{}, err
				}
			}
			if err := line.expect("orbit"); err != nil {
				return Order{}, err
			}
			if order.Orbits, err = line.orbitList(); err != nil {
				return Order{}, err
			}
			return order, nil
		},
	})
}
