// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mdhender/ecvb/internal/fuel"
	"github.com/mdhender/ecvb/internal/world"
)

// An order is written twice and no more: once as Params, which is what the
// player typed, and once as Bound, which is what the game will do. Between
// them sits the line this whole package is organised around.
//
// Bind settles what a turn cannot change -- who owns the ship, what kind of
// entity it is, whether the destination exists, whether the drive reaches it.
// None of that can be true when a file is submitted and false when the turn
// resolves, so a Bind failure rejects the whole file rather than storing an
// order that is already dead.
//
// Apply executes against live turn state, for the things that can change in
// between: fuel that has not arrived yet, a probe budget another order spent,
// a place another faction reached first. An Apply failure is a warning at
// submission -- the order is kept, because the world may yet oblige -- and a
// failed order row when the turn resolves.
//
// Both run in all three of check, submit, and resolve. Check and submit run
// them inside a savepoint that is rolled back, so a player's file is measured
// by doing the turn rather than by simulating it.

// Params is one parsed order: what the player wrote, with nothing looked up.
//
// A Params is also how an order is stored. It marshals to the params column of
// game_order, which holds the words the player used and never an id: the actor
// is a column of its own with a foreign key on it, and everything else is
// resolved again when the turn runs, so a name that stops resolving is a
// failed order rather than a corrupt row. Every Params therefore tags its
// actor field `json:"-"`, and its Spec's Decode puts the actor back.
type Params interface {
	// Actor is the entity the order acts on, or 0 for an order that acts on
	// none.
	Actor() int64
	// Input is the order rendered back in the words the player used. It is
	// stored with the order, and it is what the reports print and what the
	// engine log echoes.
	Input() string
	// Bind resolves the order's names into database ids and settles the
	// legality the turn cannot change.
	//
	// One line may bind to more than one order: a probe spends one probe per
	// orbit, and each orbit is its own order. An error reports everything
	// wrong with the line, not only the first thing.
	Bind(*Binder) ([]Bound, error)
}

// Bound is an order whose names are ids and whose legality is settled.
type Bound interface {
	// Params is this one order as it will be stored: what the player wrote,
	// narrowed to the single order it became. A probe line becomes one Bound
	// per orbit, and each stores the one orbit it will read.
	Params() Params
	// Fuel is what the order burns if it succeeds. It is stored with the order
	// so a player can see what a pending turn will cost.
	Fuel() int64
	// Apply executes the order. A failed Outcome is a game-rule failure and
	// the turn goes on; an error is a database or state failure and rolls the
	// whole turn back.
	Apply(*Turn) (Outcome, error)
}

// Binder is what Bind consults: the world as it stands and the faction whose
// order it is.
type Binder struct {
	World     *world.World
	FactionID int64
}

// Turn is the live state an order executes against.
type Turn struct {
	World     *world.World
	Number    int
	FactionID int64
	// Sequence is the order's place in the turn's resolution order. Draws are
	// seeded from it, so the same turn resolved twice reaches the same ring.
	Sequence int
}

// The status an order carries once it has been applied.
const (
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
)

// Outcome is what happened when an order was applied.
type Outcome struct {
	Status  string
	Message string
	// Start and Final are where the order found its actor and where it left
	// it. Every order reports both, because the log records where an order
	// happened; for an order that moves nothing they are the same place. Only
	// an order whose Spec has Movement set records them in the database.
	Start     world.Location
	Final     world.Location
	FuelSpent int64
	// Survey is the planet the order read, for the orders that read one. An
	// order that read nothing, or a probe that failed, leaves it nil.
	Survey *Survey
	// Note is what an order that succeeded still wants to say: that it did
	// less than it was asked for, because the workers or the transports or the
	// stock ran out. A shortage is a rate rather than a failure, so it is not
	// an error message and does not go where one goes -- the engine log
	// carries it, and a check or a submit reports it as a warning, so a player
	// sees before the turn runs that their file asks for more than it will get.
	Note string
}

// Survey is a planet an order read.
type Survey struct {
	StelliumID   int64
	SystemID     int64
	PlanetID     int64
	Habitability int
}

func succeeded(start, final world.Location, burned int64) Outcome {
	return Outcome{Status: StatusSucceeded, Start: start, Final: final, FuelSpent: burned}
}

// failed records a game-rule failure. A failed order leaves its actor where it
// found it, so the start and the final location are the same.
func failed(at world.Location, message string) Outcome {
	return Outcome{Status: StatusFailed, Message: message, Start: at, Final: at}
}

// actor finds the entity an order names and checks that the player may name it
// that way and that their faction owns it.
//
// kind is the word the player wrote, "ship" or "colony", and only a probe may
// write "colony". It is empty for an order read back out of the database,
// where which word was written was settled when it was written. An entity that
// exists is named in the message by what it actually is, so one wording serves
// a file being checked and a turn being resolved, where nobody wrote anything.
func (b *Binder) actor(id int64, kind string) (*world.Entity, error) {
	entity := b.World.Entity(id)
	if entity == nil {
		if kind == "" {
			return nil, fmt.Errorf("entity %d does not exist", id)
		}
		return nil, fmt.Errorf("%s %d does not exist", kind, id)
	}
	switch kind {
	case "colony":
		if entity.Unit == "SHIP" {
			return nil, fmt.Errorf("entity %d is a ship, not a colony", id)
		}
	case "ship":
		if entity.Unit != "SHIP" {
			return nil, fmt.Errorf("entity %d is a %s, not a ship", id, entity.Unit)
		}
	}
	if entity.FactionID != b.FactionID {
		return nil, fmt.Errorf("%s %d does not belong to faction %d", noun(entity), id, b.FactionID)
	}
	// A ship crossing between stellia is nowhere, and nowhere is out of reach.
	// This is checked once, here, rather than by each order, because it holds
	// for every order there will ever be: a crossing cannot be recalled or
	// redirected, and the ship is not somewhere an order can act on until it
	// lands. Arrivals resolve after every order that could name a ship, so a
	// ship due this turn is still out of reach for the whole of it.
	if entity.InTransit() {
		return nil, fmt.Errorf("%s %d is in transit and arrives on turn %d; it can be given no orders until then",
			noun(entity), id, entity.Transit.ArrivalTurn)
	}
	// An entity still being built exists and is visible, but it is not yet a
	// thing that acts: it has no people, nothing assembled, and a standing
	// commitment already working on it. Refusing it here is the same guard as
	// the one above, written once for every order there will ever be.
	if entity.UnderConstruction() {
		return nil, fmt.Errorf("%s %d is under construction and can be given no orders until it is finished",
			noun(entity), id)
	}
	return entity, nil
}

// once refuses an entity a second order of the same kind in one turn, and
// counts this one against it.
//
// A ship travels twice a turn at most, and never the same way twice: it may
// MOVE inside its stellium and then JUMP out of it, which is how a ship at a
// planet leaves, but it may not MOVE twice or JUMP twice.
//
// The order is counted whatever it goes on to do, because what is spent is the
// order and not the journey: a MOVE that fails for want of fuel has still been
// given, and the ship does not get another. This settles at Bind, so a file
// with two of them is refused rather than half-executed -- how many orders a
// player wrote for one ship is not something a turn can change.
func (b *Binder) once(verb string, entity *world.Entity) error {
	if b.World.OrdersGiven(verb, entity.ID) > 0 {
		return fmt.Errorf("%s %d already has a %s order this turn and may be given one a turn",
			noun(entity), entity.ID, strings.ToUpper(verb))
	}
	b.World.RecordOrder(verb, entity.ID)
	return nil
}

// recipient finds the entity an order is handing something to. It is not
// actor: a faction may hand things to its own entities and to the derelicts
// nobody holds, but not to another faction's -- so ownership is checked here
// against two answers rather than one.
func (b *Binder) recipient(id int64, kind string) (*world.Entity, error) {
	entity := b.World.Entity(id)
	if entity == nil {
		return nil, fmt.Errorf("%s %d does not exist", kind, id)
	}
	switch kind {
	case "colony":
		if entity.Unit == "SHIP" {
			return nil, fmt.Errorf("entity %d is a ship, not a colony", id)
		}
	case "ship":
		if entity.Unit != "SHIP" {
			return nil, fmt.Errorf("entity %d is a %s, not a ship", id, entity.Unit)
		}
	}
	if entity.FactionID != b.FactionID && entity.FactionID != b.World.Game().Uncontrolled {
		return nil, fmt.Errorf("%s %d belongs to another faction", noun(entity), id)
	}
	if entity.InTransit() {
		return nil, fmt.Errorf("%s %d is in transit and arrives on turn %d; nothing can reach it until then",
			noun(entity), id, entity.Transit.ArrivalTurn)
	}
	// A build is fed by the entity that ordered it and by nothing else, so an
	// unfinished entity is not somewhere a transfer may put things down.
	if entity.UnderConstruction() {
		return nil, fmt.Errorf("%s %d is under construction; only the build that began it may deliver to it",
			noun(entity), id)
	}
	return entity, nil
}

// system is the system an order names: the one the player wrote, which has to
// belong to the entity's own stellium, or the one the entity is in when the
// order named none. Zero is the stellium orbit, which is no system at all;
// what an order does about that is the order's business.
func (b *Binder) system(entity *world.Entity, letter string) (int64, error) {
	if letter == "" {
		return entity.Location.SystemID, nil
	}
	id, err := b.World.System(entity.Location.StelliumID, letter)
	if err != nil {
		return 0, err
	}
	if id == 0 {
		return 0, fmt.Errorf("current stellium has no system %s", letter)
	}
	return id, nil
}

// spendFuel charges an order's fuel, reporting the reason the entity cannot
// pay when it cannot. It is the one rule that is deliberately not settled at
// Bind: fuel may reach a ship between the file and the turn.
func spendFuel(t *Turn, entity *world.Entity, verb string, cost int64) (string, error) {
	if held := entity.Fuel(); held < cost {
		return fmt.Sprintf("%s %d needs %d %s to %s and holds %d",
			noun(entity), entity.ID, cost, fuel.Unit, verb, held), nil
	}
	return "", t.World.BurnFuel(entity, cost)
}

// noun is what an entity is called in a message. Every unit that is not a ship
// is a colony to a player, which is the only distinction the orders draw.
func noun(entity *world.Entity) string {
	if entity.Unit == "SHIP" {
		return "ship"
	}
	return "colony"
}

// displaySystem names the system an order asked for, in a message about what
// went wrong with it. An order that named none asked for the one its actor was
// already in.
func displaySystem(letter string) string {
	if letter == "" {
		return "current"
	}
	return letter
}

// orbitInput renders an order that named orbits, in the words the player used.
// An order that named no system asked for the one its actor was already in and
// says so by leaving the system out.
func orbitInput(system string, orbits ...int) string {
	var out strings.Builder
	if system != "" {
		fmt.Fprintf(&out, "system %s ", system)
	}
	out.WriteString("orbit")
	for _, orbit := range orbits {
		fmt.Fprintf(&out, " %d", orbit)
	}
	return out.String()
}

// bindErrors is more than one thing wrong with a single line, such as a probe
// naming several empty orbits. Every one of them is reported against that line.
type bindErrors []error

func (e bindErrors) Error() string {
	messages := make([]string, len(e))
	for i, item := range e {
		messages[i] = item.Error()
	}
	return strings.Join(messages, "; ")
}

// eachError flattens what Bind returned into one message per thing wrong.
func eachError(err error) []error {
	if many, ok := errors.AsType[bindErrors](err); ok {
		return many
	}
	return []error{err}
}

// Problem is the one message a stored order carries when it fails to bind
// while the turn is resolving.
func Problem(err error) string {
	return eachError(err)[0].Error()
}
