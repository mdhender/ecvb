// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/mdhender/ecvb/internal/cadre"
	"github.com/mdhender/ecvb/internal/units"
	"github.com/mdhender/ecvb/internal/world"
)

// The orders whose surface form is accepted and whose rules are not written.
//
// `docs/accepted-orders.md` gives every verb's grammar and defines every field
// it reads, so all of them can be parsed today. What none of them can be told
// is what to do: what a spy costs, what a group produces per turn, who the
// market's counterparty is, what control confers. Those are the 1978 rules,
// and inventing them here would be authoring the game rather than implementing
// it.
//
// So each order below is a real Spec -- the parser refuses a malformed one, the
// reference prints its forms, and `Bind` still settles who may give it -- and
// each binds to notBuilt, which fails when the turn resolves.
//
// **This file is where an order lives until its rules land.** The day a verb is
// specified, its Spec and Params move to verbs.go with a Bound of their own and
// nothing here has to be untangled from anything else.

// notBuilt is the Bound of an order that parses and cannot yet act.
//
// Bind succeeds and Apply fails, which is the deliberate choice among three.
// Refusing at Bind would reject the whole file, so a player could not submit a
// turn that mentioned one of these at all. Succeeding at both would report an
// order that did nothing as having worked. Failing at Apply puts it where every
// other game-rule failure goes: a warning at submission, a failed row with a
// reason when the turn resolves, and the rest of the file unaffected.
type notBuilt struct {
	params Params
	verb   string
	at     world.Location
}

// Params is the order as it will be stored. It is stored, and read back, and
// reported on, like any other: only the doing of it is missing.
func (o *notBuilt) Params() Params { return o.params }

// Fuel is nothing. What an unwritten rule would charge is not knowable.
func (o *notBuilt) Fuel() int64 { return 0 }

// Apply fails, and says why in the terms of the thing that is missing. The
// failure is marked unsupported, because a rule is not something the world
// might yet oblige between submission and resolution.
func (o *notBuilt) Apply(t *Turn) (Outcome, error) {
	outcome := failed(o.at, fmt.Sprintf(
		"%s is accepted but not built yet; the rules it needs are not written", strings.ToUpper(o.verb)))
	outcome.Unsupported = true
	return outcome, nil
}

// registerUnbuilt adds an order that parses and does not yet act. It is how the
// flag and this file stay the same fact: an order registered here is marked
// unbuilt, and nothing outside this file can mark one.
func registerUnbuilt(spec *Spec) {
	spec.Unbuilt = true
	register(spec)
}

// unbuilt binds an order whose rules are not written.
//
// It still finds the actor, so ownership, the kind of entity, a ship in
// transit, and an entity under construction are all settled the way every other
// order settles them -- what is missing is only what the order does. An order
// whose subject is `we` names no entity and skips that.
func unbuilt(b *Binder, verb, kind string, params Params) ([]Bound, error) {
	bound := &notBuilt{params: params, verb: verb}
	if params.Actor() != 0 {
		entity, err := b.actor(params.Actor(), kind)
		if err != nil {
			return nil, err
		}
		bound.at = entity.Location
	}
	return []Bound{bound}, nil
}

// entityRef is a ship or colony an order names other than its subject: the
// word the player wrote and the id they wrote after it.
//
// It is stored rather than resolved, the way a transfer's recipient is. What
// may be attacked, controlled, or spied on is part of the rules that are not
// written, so nothing here looks the target up.
type entityRef struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id"`
}

// String renders the reference in the words the player used.
func (e entityRef) String() string { return fmt.Sprintf("%s %d", e.Kind, e.ID) }

// entityRef reads a ship or colony and its id.
func (p *Parser) entityRef() (entityRef, error) {
	kind, ok := p.keyword(SubjectShip, SubjectColony)
	if !ok {
		return entityRef{}, errShape
	}
	id, err := p.entityID(kind)
	if err != nil {
		return entityRef{}, err
	}
	return entityRef{Kind: kind, ID: id}, nil
}

// place is a planet named the long way, by coordinates and system and orbit.
// The administrative orders name one that way because no ship or colony carries
// them out, so there is no entity whose location could be meant instead.
type place struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Z      int    `json:"z"`
	System string `json:"system"`
	Orbit  int    `json:"orbit"`
}

func (p place) String() string {
	return fmt.Sprintf("(%d,%d,%d) system %s orbit %d", p.X, p.Y, p.Z, p.System, p.Orbit)
}

// planet reads coordinates, a system letter, and an orbit.
func (p *Parser) planet() (place, error) {
	var at place
	var err error
	if at.X, at.Y, at.Z, err = p.coordinates(); err != nil {
		return place{}, err
	}
	if err = p.expect("system"); err != nil {
		return place{}, err
	}
	if at.System, err = p.systemLetter(); err != nil {
		return place{}, err
	}
	if err = p.expect("orbit"); err != nil {
		return place{}, err
	}
	if at.Orbit, err = p.number("orbit"); err != nil {
		return place{}, err
	}
	return at, nil
}

// share reads a percentage that is a share of something -- a combat commitment,
// a market commission -- which cannot be none of it or more than all of it.
func (p *Parser) share(what string) (int, error) { return p.percentage(what, 1, 100) }

// rate reads a percentage that is a multiple of a standard -- a pay rate, a
// ration rate -- which has no ceiling: a faction may overpay or overfeed, and
// the accepted grammar puts no bound on either.
func (p *Parser) rate(what string) (int, error) { return p.percentage(what, 0, math.MaxInt32) }

// actorOf is the subject an entity order was given to. Every order given to a
// ship or a colony carries one, so Actor is written once rather than thirty
// times, and the fields are tagged out of the JSON because the actor is a
// column of its own.
type actorOf struct {
	// Kind is the word the player wrote, "ship" or "colony". It is stored,
	// because it is a word rather than an id and because an order read back out
	// of the database has to render itself in the words it was written in.
	Kind string `json:"kind,omitempty"`
	// EntityID is not stored: the actor is a column of its own, with a foreign
	// key on it.
	EntityID int64 `json:"-"`
}

// Actor is the entity the order acts on.
func (a actorOf) Actor() int64 { return a.EntityID }

// factionOrder is the subject of an order no ship or colony carries out. It
// names no entity, because the file header already names the faction.
type factionOrder struct{}

// Actor is nobody: a faction order acts on no entity.
func (factionOrder) Actor() int64 { return 0 }

// decode rebuilds a stored order. The actor is a column rather than part of the
// JSON, so the caller puts it back by handing in a value that already has it.
func decode[T Params](encoded string, into T) (Params, error) {
	if err := json.Unmarshal([]byte(encoded), &into); err != nil {
		return nil, err
	}
	return into, nil
}

// COMBAT ----------------------------------------------------------------
//
// Stage 4. The orders name a target and commit a share of the entity; what
// settles them is a sweep between the entities that met, and that is combat's
// rule and is not written.

func init() {
	for _, item := range []struct {
		verb    string
		summary string
		phase   *Phase
	}{
		{"attack", "commit a share of an entity to a battle against another", PhaseAttack},
		{"invade", "commit a share of an entity to taking another by landing on it", PhaseInvade},
	} {
		phase, verb := item.phase, item.verb
		registerUnbuilt(&Spec{
			Verb:     verb,
			Subjects: []string{SubjectShip, SubjectColony},
			Summary:  item.summary,
			Syntax: []string{
				"ship SHIP-ID " + verb + " (ship | colony) ID PERCENT%",
				"colony COLONY-ID " + verb + " (ship | colony) ID PERCENT%",
			},
			Phase: phase,
			Decode: func(actor int64, encoded string) (Params, error) {
				return decode(encoded, BattleParams{actorOf: actorOf{EntityID: actor}, Verb: verb})
			},
			Parse: func(subject Subject, p *Parser) (Params, error) {
				order := BattleParams{actorOf: actorOf{Kind: subject.Kind, EntityID: subject.ID}, Verb: verb}
				var err error
				if order.Target, err = p.entityRef(); err != nil {
					return nil, err
				}
				order.Commitment, err = p.share("a commitment")
				return order, err
			},
		})
	}

	registerUnbuilt(&Spec{
		Verb:     "raid",
		Subjects: []string{SubjectShip, SubjectColony},
		Summary:  "commit a share of an entity to taking named units off another",
		Syntax: []string{
			"ship SHIP-ID raid (ship | colony) ID seeking UNIT, UNIT PERCENT%",
			"colony COLONY-ID raid (ship | colony) ID seeking UNIT, UNIT PERCENT%",
		},
		Phase: PhaseRaid,
		Decode: func(actor int64, encoded string) (Params, error) {
			return decode(encoded, RaidParams{actorOf: actorOf{EntityID: actor}})
		},
		Parse: func(subject Subject, p *Parser) (Params, error) {
			order := RaidParams{actorOf: actorOf{Kind: subject.Kind, EntityID: subject.ID}}
			var err error
			if order.Target, err = p.entityRef(); err != nil {
				return nil, err
			}
			if err = p.expect("seeking"); err != nil {
				return nil, err
			}
			// A raid names one unit or two, and no more: it is a snatch rather
			// than a shopping list.
			for {
				at := p.pos
				tag, err := p.unitTag()
				if err != nil {
					return nil, err
				}
				order.Seeking = append(order.Seeking, tag)
				// The unit that took the raid past two is the one to point at.
				if len(order.Seeking) > 2 {
					return nil, p.at(at, "a raid seeks one unit or two, and this one names %d",
						len(order.Seeking))
				}
				if _, ok := p.keyword(","); !ok {
					break
				}
			}
			order.Commitment, err = p.share("a commitment")
			return order, err
		},
	})

	registerUnbuilt(&Spec{
		Verb:     "support",
		Subjects: []string{SubjectShip, SubjectColony},
		Summary:  "commit a share of an entity to another's battle, attacking or defending",
		Syntax: []string{
			"ship SHIP-ID support (ship | colony) ID attacking [(ship | colony) ID] PERCENT%",
			"ship SHIP-ID support (ship | colony) ID defending [against (ship | colony) ID] PERCENT%",
			"colony COLONY-ID support (ship | colony) ID attacking [(ship | colony) ID] PERCENT%",
			"colony COLONY-ID support (ship | colony) ID defending [against (ship | colony) ID] PERCENT%",
		},
		Phase: PhaseSupport,
		Decode: func(actor int64, encoded string) (Params, error) {
			return decode(encoded, SupportParams{actorOf: actorOf{EntityID: actor}})
		},
		Parse: func(subject Subject, p *Parser) (Params, error) {
			order := SupportParams{actorOf: actorOf{Kind: subject.Kind, EntityID: subject.ID}}
			var err error
			if order.Ally, err = p.entityRef(); err != nil {
				return nil, err
			}
			stance, ok := p.keyword(stanceAttacking, stanceDefending)
			if !ok {
				return nil, errShape
			}
			order.Stance = stance
			// Naming the other side is optional either way: support given
			// without one is given against whoever turns up. Defending says
			// `against` first, because "defending ship 33" would otherwise read
			// as defending that ship rather than from it.
			named := false
			if stance == stanceDefending {
				_, named = p.keyword("against")
			} else if word, ok := p.peek(); ok && !word.quoted {
				named = strings.EqualFold(word.text, SubjectShip) || strings.EqualFold(word.text, SubjectColony)
			}
			if named {
				other, err := p.entityRef()
				if err != nil {
					return nil, err
				}
				order.Against = &other
			}
			order.Commitment, err = p.share("a commitment")
			return order, err
		},
	})
}

// The two sides a support order may take.
const (
	stanceAttacking = "attacking"
	stanceDefending = "defending"
)

// BattleParams is an ATTACK or an INVADE: one entity, another, and the share of
// the first committed to it. The two are one type because they are one shape;
// which verb it was is stored, because the engine will need to know.
type BattleParams struct {
	actorOf
	Verb       string    `json:"-"`
	Target     entityRef `json:"target"`
	Commitment int       `json:"commitment"`
}

// Input is the order as the player wrote it.
func (p BattleParams) Input() string { return fmt.Sprintf("%s %d%%", p.Target, p.Commitment) }

// Bind settles who may give the order and nothing else: what may be attacked,
// and what happens when it is, are combat's rules and are not written.
func (p BattleParams) Bind(b *Binder) ([]Bound, error) { return unbuilt(b, p.Verb, p.Kind, p) }

// RaidParams is a RAID: a target, the one or two units the raid is after, and
// the share of the entity committed to it.
type RaidParams struct {
	actorOf
	Target     entityRef `json:"target"`
	Seeking    []string  `json:"seeking"`
	Commitment int       `json:"commitment"`
}

// Input is the order as the player wrote it.
func (p RaidParams) Input() string {
	return fmt.Sprintf("%s seeking %s %d%%", p.Target, strings.Join(p.Seeking, ", "), p.Commitment)
}

// Bind settles who may give the order and nothing else.
func (p RaidParams) Bind(b *Binder) ([]Bound, error) { return unbuilt(b, "raid", p.Kind, p) }

// SupportParams is a SUPPORT: whose battle, which side of it, optionally who
// the other side is, and the share committed.
type SupportParams struct {
	actorOf
	Ally       entityRef  `json:"ally"`
	Stance     string     `json:"stance"`
	Against    *entityRef `json:"against,omitempty"`
	Commitment int        `json:"commitment"`
}

// Input is the order as the player wrote it.
func (p SupportParams) Input() string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s %s", p.Ally, p.Stance)
	if p.Against != nil {
		if p.Stance == stanceDefending {
			out.WriteString(" against")
		}
		fmt.Fprintf(&out, " %s", *p.Against)
	}
	fmt.Fprintf(&out, " %d%%", p.Commitment)
	return out.String()
}

// Bind settles who may give the order and nothing else.
func (p SupportParams) Bind(b *Binder) ([]Bound, error) { return unbuilt(b, "support", p.Kind, p) }

// GROUPS ----------------------------------------------------------------
//
// Stages 7 and 8, plus the three group forms of CREATE. A group is a factory, a
// farm, or a mine; what one produces per turn is the production system's rule
// and is not written, and neither is the work in progress that a remove
// salvages or a retool drains.

// The three kinds of work group, in the words the player writes.
const (
	groupFactory = "factory-group"
	groupFarm    = "farm-group"
	groupMine    = "mine-group"
)

var groupKinds = []string{groupFactory, groupFarm, groupMine}

// groupAllows refuses a group order the subject cannot be given. A factory and
// a mine belong to a colony; only a farm group may be worked from a ship.
func groupAllows(p *Parser, at int, subject, group string) error {
	if subject == SubjectShip && group != groupFarm {
		return p.at(at, "a %s belongs to a colony, and this order was given to a ship", group)
	}
	return nil
}

// groupLots refuses a list where the grammar reads one lot. Only a factory
// group names several units at once; a farm and a mine take one.
func groupLots(p *Parser, at int, group string, units []UnitQuantity) error {
	if group != groupFactory && len(units) != 1 {
		return p.at(at, "a %s order names one unit, and this one names %d", group, len(units))
	}
	return nil
}

func init() {
	// ADD, IDLE, and ACTIVATE are one shape with one preposition each, so the
	// three are registered from a table rather than written out three times.
	for _, item := range []struct {
		verb, preposition, summary string
		phase                      *Phase
	}{
		{"add", "to", "put units into a work group, assembling them on the way", PhaseAdd},
		{"idle", "in", "stop units in a work group without taking them out of it", PhaseIdle},
		{"activate", "in", "set idle units in a work group working again", PhaseActivate},
	} {
		verb, preposition, phase := item.verb, item.preposition, item.phase
		registerUnbuilt(&Spec{
			Verb:     verb,
			Subjects: []string{SubjectShip, SubjectColony},
			Summary:  item.summary,
			Syntax: []string{
				"ship SHIP-ID " + verb + " QUANTITY UNIT, ... " + preposition + " (factory-group | farm-group | mine-group) GROUP-NO",
				"colony COLONY-ID " + verb + " QUANTITY UNIT, ... " + preposition + " (factory-group | farm-group | mine-group) GROUP-NO",
			},
			Phase: phase,
			Decode: func(actor int64, encoded string) (Params, error) {
				return decode(encoded, GroupUnitsParams{actorOf: actorOf{EntityID: actor}, Verb: verb, Preposition: preposition})
			},
			Parse: func(subject Subject, p *Parser) (Params, error) {
				order := GroupUnitsParams{actorOf: actorOf{Kind: subject.Kind, EntityID: subject.ID},
					Verb: verb, Preposition: preposition}
				return order, order.read(subject, p)
			},
		})
	}

	registerUnbuilt(&Spec{
		Verb:     "remove",
		Subjects: []string{SubjectShip, SubjectColony},
		Summary:  "take units out of a work group, unassembling them and optionally stowing them",
		Syntax: []string{
			"ship SHIP-ID remove QUANTITY UNIT, ... from (factory-group | farm-group | mine-group) GROUP-NO [and stow]",
			"colony COLONY-ID remove QUANTITY UNIT, ... from (factory-group | farm-group | mine-group) GROUP-NO [and stow]",
		},
		Phase: PhaseRemove,
		Decode: func(actor int64, encoded string) (Params, error) {
			return decode(encoded, GroupUnitsParams{actorOf: actorOf{EntityID: actor}, Verb: "remove", Preposition: "from"})
		},
		Parse: func(subject Subject, p *Parser) (Params, error) {
			order := GroupUnitsParams{actorOf: actorOf{Kind: subject.Kind, EntityID: subject.ID},
				Verb: "remove", Preposition: "from"}
			if err := order.read(subject, p); err != nil {
				return nil, err
			}
			// Removal unassembles what it takes out and may stow it, the same
			// way an unassemble order may.
			if _, ok := p.keyword("and"); ok {
				if err := p.expect("stow"); err != nil {
					return nil, err
				}
				order.Stow = true
			}
			return order, nil
		},
	})

	registerUnbuilt(&Spec{
		Verb:     "retool",
		Subjects: []string{SubjectColony},
		Summary:  "change what a factory group makes, draining the line first or discarding it",
		Syntax: []string{
			"colony COLONY-ID retool factory-group GROUP-NO making UNIT",
			"colony COLONY-ID retool immediately factory-group GROUP-NO making UNIT",
		},
		Phase: PhaseRetool,
		Decode: func(actor int64, encoded string) (Params, error) {
			return decode(encoded, RetoolParams{actorOf: actorOf{EntityID: actor}})
		},
		Parse: func(subject Subject, p *Parser) (Params, error) {
			order := RetoolParams{actorOf: actorOf{Kind: subject.Kind, EntityID: subject.ID}}
			// The plain form drains the line before it retools, which may take
			// three turns; `immediately` discards the work in progress and
			// spends the retooling turn now.
			_, order.Immediately = p.keyword("immediately")
			if err := p.expect(groupFactory); err != nil {
				return nil, err
			}
			var err error
			if order.Group, err = p.number("a group number"); err != nil {
				return nil, err
			}
			if err = p.expect("making"); err != nil {
				return nil, err
			}
			order.Making, err = p.unitTag()
			return order, err
		},
	})
}

// GroupUnitsParams is an ADD, a REMOVE, an IDLE, or an ACTIVATE: the four
// verbs that name units and a work group. They are one type because they are
// one shape -- only the preposition and what the engine will do with them
// differ -- and remove is the one that carries a stow.
type GroupUnitsParams struct {
	actorOf
	Verb        string         `json:"-"`
	Preposition string         `json:"-"`
	Units       []UnitQuantity `json:"units"`
	Group       string         `json:"group"`
	GroupNo     int            `json:"group_no"`
	Stow        bool           `json:"stow,omitempty"`
}

// read consumes the units, the preposition, and the group, which is the same
// for all four verbs.
func (g *GroupUnitsParams) read(subject Subject, p *Parser) error {
	units := p.pos
	var err error
	if g.Units, err = p.unitList(); err != nil {
		return err
	}
	if err = p.expect(g.Preposition); err != nil {
		return err
	}
	named := p.pos
	kind, ok := p.keyword(groupKinds...)
	if !ok {
		return errShape
	}
	g.Group = kind
	if g.GroupNo, err = p.number("a group number"); err != nil {
		return err
	}
	if err := groupAllows(p, named, subject.Kind, kind); err != nil {
		return err
	}
	return groupLots(p, units, kind, g.Units)
}

// Input is the order as the player wrote it.
func (p GroupUnitsParams) Input() string {
	input := fmt.Sprintf("%s %s %s %d", unitListInput(p.Units), p.Preposition, p.Group, p.GroupNo)
	if p.Stow {
		input += " and stow"
	}
	return input
}

// Bind settles who may give the order and nothing else: what a group produces,
// and what a removal salvages of the work in progress, are the production
// system's rules and are not written.
func (p GroupUnitsParams) Bind(b *Binder) ([]Bound, error) { return unbuilt(b, p.Verb, p.Kind, p) }

// RetoolParams is a RETOOL: which factory group, what it is to make, and
// whether the line is drained first or discarded.
type RetoolParams struct {
	actorOf
	Immediately bool   `json:"immediately,omitempty"`
	Group       int    `json:"group_no"`
	Making      string `json:"making"`
}

// Input is the order as the player wrote it.
func (p RetoolParams) Input() string {
	immediately := ""
	if p.Immediately {
		immediately = "immediately "
	}
	return fmt.Sprintf("%s%s %d making %s", immediately, groupFactory, p.Group, p.Making)
}

// Bind settles who may give the order and nothing else.
func (p RetoolParams) Bind(b *Binder) ([]Bound, error) { return unbuilt(b, "retool", p.Kind, p) }

// MARKET ----------------------------------------------------------------
//
// Stage 11. Each order is an offer with a price and a commission; the sweep
// that matches them, preferring what returns the market the most, is the
// market's rule and is not written. Neither is who the counterparty is.

func init() {
	for _, item := range []struct {
		verb, summary string
		phase         *Phase
	}{
		{"sell", "offer units, or a technology level, to the market at a price", PhaseSell},
		{"buy", "bid for units, or a technology level, from the market at a price", PhaseBuy},
	} {
		verb, phase := item.verb, item.phase
		registerUnbuilt(&Spec{
			Verb:     verb,
			Subjects: []string{SubjectShip, SubjectColony},
			Summary:  item.summary,
			Syntax: []string{
				"ship SHIP-ID " + verb + " QUANTITY UNIT PRICE (GOLD | CNGD), ... [PERCENT%]",
				"ship SHIP-ID " + verb + " tech-level TL-N PRICE GOLD [PERCENT%]",
				"colony COLONY-ID " + verb + " QUANTITY UNIT PRICE (GOLD | CNGD), ... [PERCENT%]",
				"colony COLONY-ID " + verb + " tech-level TL-N PRICE GOLD [PERCENT%]",
			},
			Phase: phase,
			Decode: func(actor int64, encoded string) (Params, error) {
				return decode(encoded, MarketParams{actorOf: actorOf{EntityID: actor}, Verb: verb})
			},
			Parse: func(subject Subject, p *Parser) (Params, error) {
				order := MarketParams{actorOf: actorOf{Kind: subject.Kind, EntityID: subject.ID}, Verb: verb}
				var err error
				// A technology level trades in the same market and is not
				// cargo: it is paid for in whole GOLD, needs no transports, and
				// is bought once rather than by quantity.
				if _, ok := p.keyword("tech-level"); ok {
					if order.TechLevel, err = p.techLevel(); err != nil {
						return nil, err
					}
					if order.Price, err = p.price("a price", true); err != nil {
						return nil, err
					}
				} else {
					for {
						var lot MarketLot
						if lot.Quantity, err = p.quantity("a quantity"); err != nil {
							return nil, err
						}
						if lot.Tag, err = p.unitTag(); err != nil {
							return nil, err
						}
						if lot.Price, err = p.price("a price", false); err != nil {
							return nil, err
						}
						order.Lots = append(order.Lots, lot)
						if _, ok := p.keyword(","); !ok {
							break
						}
					}
				}
				// A commission is optional: an order that names none pays the
				// default the market sets.
				if word, ok := p.peek(); ok && !word.quoted && strings.HasSuffix(word.text, "%") {
					if order.Commission, err = p.share("a commission"); err != nil {
						return nil, err
					}
				}
				return order, nil
			},
		})
	}
}

// MarketLot is one lot of a market order: how many of a unit, and what each is
// offered or bid at.
type MarketLot struct {
	Quantity int64  `json:"quantity"`
	Tag      string `json:"unit"`
	Price    Price  `json:"price"`
}

// String renders a lot the way the player wrote it.
func (l MarketLot) String() string {
	return fmt.Sprintf("%s %s %s", formatQuantity(l.Quantity), l.Tag, l.Price)
}

// MarketParams is a BUY or a SELL. The two are one type because they are one
// shape, and each has two forms: lots of units, or a single technology level.
// TechLevel is zero for the first, which is what tells them apart.
type MarketParams struct {
	actorOf
	Verb       string      `json:"-"`
	Lots       []MarketLot `json:"lots,omitempty"`
	TechLevel  int         `json:"tech_level,omitempty"`
	Price      Price       `json:"price,omitzero"`
	Commission int         `json:"commission,omitempty"`
}

// Input is the order as the player wrote it.
func (p MarketParams) Input() string {
	var out strings.Builder
	if p.TechLevel != 0 {
		fmt.Fprintf(&out, "tech-level %s%d %s", techLevelPrefix, p.TechLevel, p.Price)
	} else {
		parts := make([]string, len(p.Lots))
		for i, lot := range p.Lots {
			parts[i] = lot.String()
		}
		out.WriteString(strings.Join(parts, ", "))
	}
	if p.Commission != 0 {
		fmt.Fprintf(&out, " %d%%", p.Commission)
	}
	return out.String()
}

// Bind settles who may give the order and nothing else: what the market does
// with an offer is the market's rule and is not written.
func (p MarketParams) Bind(b *Binder) ([]Bound, error) { return unbuilt(b, p.Verb, p.Kind, p) }

// INFORMATION -----------------------------------------------------------
//
// Stages 12, 14, and 22. A survey reads the planet an entity is at; the six spy
// orders spend spies on contested outcomes; a broadcast releases a message at a
// place. What a spy costs, what REBL is, and what the news service compiles are
// all unwritten.

func init() {
	registerUnbuilt(&Spec{
		Verb:     "survey",
		Subjects: []string{SubjectShip, SubjectColony},
		Summary:  "read the planet the entity is at",
		Syntax: []string{
			"ship SHIP-ID survey",
			"colony COLONY-ID survey",
		},
		Phase: PhaseSurvey,
		Decode: func(actor int64, encoded string) (Params, error) {
			return decode(encoded, SurveyParams{actorOf: actorOf{EntityID: actor}})
		},
		Parse: func(subject Subject, p *Parser) (Params, error) {
			return SurveyParams{actorOf: actorOf{Kind: subject.Kind, EntityID: subject.ID}}, nil
		},
	})

	// The six spy orders differ in what they are aimed at and in nothing else:
	// each names its object and then says how many spies it spends.
	for _, item := range []struct {
		verb, object, summary string
		phase                 *Phase
		target                bool
		faction               bool
	}{
		{verb: "assess", object: "rebels", summary: "spend spies reading how rebellious a place is", phase: PhaseAssess},
		{verb: "detect", object: "spies", summary: "spend spies finding another faction's", phase: PhaseDetect},
		{verb: "obtain", object: "information from", summary: "spend spies reading another entity", phase: PhaseObtain, target: true},
		{verb: "convert", object: "rebels", summary: "spend spies turning rebels back", phase: PhaseConvert},
		{verb: "incite", object: "rebels", summary: "spend spies raising rebels somewhere", phase: PhaseIncite},
		{verb: "neutralize", object: "faction", summary: "spend spies against another faction's spies", phase: PhaseNeutralize, faction: true},
	} {
		item := item
		syntax := "ship SHIP-ID " + item.verb + " " + item.object
		switch {
		case item.target:
			syntax += " (ship | colony) ID"
		case item.faction:
			syntax += " FACTION-ID spies"
		}
		syntax += " using QUANTITY spies"
		registerUnbuilt(&Spec{
			Verb:     item.verb,
			Subjects: []string{SubjectShip, SubjectColony},
			Summary:  item.summary,
			Syntax:   []string{syntax, strings.Replace(syntax, "ship SHIP-ID", "colony COLONY-ID", 1)},
			Phase:    item.phase,
			Decode: func(actor int64, encoded string) (Params, error) {
				return decode(encoded, SpyParams{actorOf: actorOf{EntityID: actor}, Verb: item.verb, Object: item.object})
			},
			Parse: func(subject Subject, p *Parser) (Params, error) {
				order := SpyParams{actorOf: actorOf{Kind: subject.Kind, EntityID: subject.ID},
					Verb: item.verb, Object: item.object}
				for _, word := range strings.Fields(item.object) {
					if err := p.expect(word); err != nil {
						return nil, err
					}
				}
				var err error
				if item.target {
					target, err := p.entityRef()
					if err != nil {
						return nil, err
					}
					order.Target = &target
				}
				if item.faction {
					if order.Faction, err = p.entityID("faction"); err != nil {
						return nil, err
					}
					if err = p.expect("spies"); err != nil {
						return nil, err
					}
				}
				if err = p.expect("using"); err != nil {
					return nil, err
				}
				if order.Spies, err = p.quantity("a quantity of spies"); err != nil {
					return nil, err
				}
				return order, p.expect("spies")
			},
		})
	}

	registerUnbuilt(&Spec{
		Verb:     "broadcast",
		Subjects: []string{SubjectShip, SubjectColony},
		Summary:  "release a message at a place, for the news service to carry",
		Syntax: []string{
			`ship SHIP-ID broadcast system SYSTEM orbit ORBIT "MESSAGE" ["SIGNATURE"]`,
			`colony COLONY-ID broadcast system SYSTEM orbit ORBIT "MESSAGE" ["SIGNATURE"]`,
		},
		Phase: PhaseBroadcast,
		Decode: func(actor int64, encoded string) (Params, error) {
			return decode(encoded, BroadcastParams{actorOf: actorOf{EntityID: actor}})
		},
		Parse: func(subject Subject, p *Parser) (Params, error) {
			order := BroadcastParams{actorOf: actorOf{Kind: subject.Kind, EntityID: subject.ID}}
			if err := p.expect("system"); err != nil {
				return nil, err
			}
			var err error
			if order.System, err = p.systemLetter(); err != nil {
				return nil, err
			}
			if err = p.expect("orbit"); err != nil {
				return nil, err
			}
			if order.Orbit, err = p.number("orbit"); err != nil {
				return nil, err
			}
			if order.Message, err = p.quoted("a message"); err != nil {
				return nil, err
			}
			// The signature is optional: a broadcast may be anonymous.
			if word, ok := p.peek(); ok && word.quoted {
				if order.Signature, err = p.quoted("a signature"); err != nil {
					return nil, err
				}
			}
			return order, nil
		},
	})
}

// SurveyParams is a SURVEY, which names nothing: an entity reads the planet it
// is already at.
type SurveyParams struct{ actorOf }

// Input is the order as the player wrote it, which is the verb and nothing
// else. It is the entity's id, because an order's input may not be empty and
// what this order says is only who is doing it.
func (p SurveyParams) Input() string { return fmt.Sprintf("%s %d", p.Kind, p.EntityID) }

// Bind settles who may give the order and nothing else.
func (p SurveyParams) Bind(b *Binder) ([]Bound, error) { return unbuilt(b, "survey", p.Kind, p) }

// SpyParams is one of the six espionage orders: what it is aimed at, and how
// many spies it spends.
type SpyParams struct {
	actorOf
	Verb    string     `json:"-"`
	Object  string     `json:"-"`
	Target  *entityRef `json:"target,omitempty"`
	Faction int64      `json:"faction,omitempty"`
	Spies   int64      `json:"spies"`
}

// Input is the order as the player wrote it.
func (p SpyParams) Input() string {
	var out strings.Builder
	out.WriteString(p.Object)
	if p.Target != nil {
		fmt.Fprintf(&out, " %s", *p.Target)
	}
	if p.Faction != 0 {
		fmt.Fprintf(&out, " %d spies", p.Faction)
	}
	fmt.Fprintf(&out, " using %s spies", formatQuantity(p.Spies))
	return out.String()
}

// Bind settles who may give the order and nothing else: what a spy costs and
// what one achieves are unwritten, and so is REBL.
func (p SpyParams) Bind(b *Binder) ([]Bound, error) { return unbuilt(b, p.Verb, p.Kind, p) }

// BroadcastParams is a BROADCAST: where the message is released, what it says,
// and who it says it is from.
type BroadcastParams struct {
	actorOf
	System    string `json:"system"`
	Orbit     int    `json:"orbit"`
	Message   string `json:"message"`
	Signature string `json:"signature,omitempty"`
}

// Input is the order as the player wrote it.
func (p BroadcastParams) Input() string {
	input := fmt.Sprintf("%s %q", orbitInput(p.System, p.Orbit), p.Message)
	if p.Signature != "" {
		input += fmt.Sprintf(" %q", p.Signature)
	}
	return input
}

// Bind settles who may give the order and nothing else.
func (p BroadcastParams) Bind(b *Binder) ([]Bound, error) { return unbuilt(b, "broadcast", p.Kind, p) }

// POPULATION ------------------------------------------------------------
//
// Stages 16 and 17. What a draft costs, what a pay rate buys, and what a ration
// rate feeds are the population system's rules and are not written; the rates
// set here are the input to rebellion and to population growth, and neither of
// those exists either.

// The population a draft may make. Only SOL and the cadres may be drafted: no
// other class can be, and drafting SOL is the one draft that changes anyone's
// type rather than assigning them.
var draftable = []string{units.ClassSoldier, cadre.Unit, "PLCF", "SPCF", "TRNE"}

func init() {
	for _, item := range []struct {
		verb, summary string
		phase         *Phase
	}{
		{"draft", "make soldiers or a cadre out of the population an entity carries", PhaseDraft},
		{"disband", "return soldiers or a cadre to the population they came from", PhaseDisband},
	} {
		verb, phase := item.verb, item.phase
		registerUnbuilt(&Spec{
			Verb:     verb,
			Subjects: []string{SubjectShip, SubjectColony},
			Summary:  item.summary,
			Syntax: []string{
				"ship SHIP-ID " + verb + " QUANTITY (SOL | CADRE), ...",
				"colony COLONY-ID " + verb + " QUANTITY (SOL | CADRE), ...",
			},
			Phase: phase,
			Decode: func(actor int64, encoded string) (Params, error) {
				return decode(encoded, DraftParams{actorOf: actorOf{EntityID: actor}, Verb: verb})
			},
			Parse: func(subject Subject, p *Parser) (Params, error) {
				order := DraftParams{actorOf: actorOf{Kind: subject.Kind, EntityID: subject.ID}, Verb: verb}
				for {
					quantity, err := p.quantity("a quantity")
					if err != nil {
						return nil, err
					}
					code, ok := p.keyword(draftable...)
					if !ok {
						return nil, p.here("only %s and the cadres may be %sed", units.ClassSoldier, verb)
					}
					order.Lots = append(order.Lots, UnitQuantity{Quantity: quantity, Tag: code})
					if _, ok := p.keyword(","); !ok {
						return order, nil
					}
				}
			},
		})
	}

	registerUnbuilt(&Spec{
		Verb:     "pay",
		Subjects: []string{SubjectShip, SubjectColony},
		Summary:  "set what a class of population is paid, as a share of the standard rate",
		Syntax: []string{
			"ship SHIP-ID pay CLASS PERCENT%, ...",
			"colony COLONY-ID pay CLASS PERCENT%, ...",
		},
		Phase: PhasePay,
		Decode: func(actor int64, encoded string) (Params, error) {
			return decode(encoded, PayParams{actorOf: actorOf{EntityID: actor}})
		},
		Parse: func(subject Subject, p *Parser) (Params, error) {
			order := PayParams{actorOf: actorOf{Kind: subject.Kind, EntityID: subject.ID}}
			for {
				class, ok := p.keyword(units.ClassUnskilled, units.ClassSkilled,
					units.ClassSoldier, units.ClassNonAssignable)
				if !ok {
					return nil, errShape
				}
				rate, err := p.rate("a pay rate")
				if err != nil {
					return nil, err
				}
				order.Rates = append(order.Rates, PayRate{Class: class, Rate: rate})
				if _, ok := p.keyword(","); !ok {
					return order, nil
				}
			}
		},
	})

	registerUnbuilt(&Spec{
		Verb:     "rations",
		Subjects: []string{SubjectShip, SubjectColony},
		Summary:  "set what everyone aboard is fed, as a share of the standard ration",
		Syntax: []string{
			"ship SHIP-ID rations PERCENT%",
			"colony COLONY-ID rations PERCENT%",
		},
		Phase: PhaseRations,
		Decode: func(actor int64, encoded string) (Params, error) {
			return decode(encoded, RationsParams{actorOf: actorOf{EntityID: actor}})
		},
		Parse: func(subject Subject, p *Parser) (Params, error) {
			order := RationsParams{actorOf: actorOf{Kind: subject.Kind, EntityID: subject.ID}}
			var err error
			order.Rate, err = p.rate("a ration rate")
			return order, err
		},
	})
}

// DraftParams is a DRAFT or a DISBAND: the two are one shape, and only SOL and
// the cadres may be named in either.
type DraftParams struct {
	actorOf
	Verb string         `json:"-"`
	Lots []UnitQuantity `json:"lots"`
}

// Input is the order as the player wrote it.
func (p DraftParams) Input() string { return unitListInput(p.Lots) }

// Bind settles who may give the order and nothing else.
func (p DraftParams) Bind(b *Binder) ([]Bound, error) { return unbuilt(b, p.Verb, p.Kind, p) }

// PayRate is one class of population and what it is paid.
type PayRate struct {
	Class string `json:"class"`
	Rate  int    `json:"rate"`
}

// PayParams is a PAY: one rate per class named.
type PayParams struct {
	actorOf
	Rates []PayRate `json:"rates"`
}

// Input is the order as the player wrote it.
func (p PayParams) Input() string {
	parts := make([]string, len(p.Rates))
	for i, rate := range p.Rates {
		parts[i] = fmt.Sprintf("%s %d%%", rate.Class, rate.Rate)
	}
	return strings.Join(parts, ", ")
}

// Bind settles who may give the order and nothing else.
func (p PayParams) Bind(b *Binder) ([]Bound, error) { return unbuilt(b, "pay", p.Kind, p) }

// RationsParams is a RATIONS: one rate for everyone aboard.
type RationsParams struct {
	actorOf
	Rate int `json:"rate"`
}

// Input is the order as the player wrote it.
func (p RationsParams) Input() string { return fmt.Sprintf("%d%%", p.Rate) }

// Bind settles who may give the order and nothing else.
func (p RationsParams) Bind(b *Binder) ([]Bound, error) { return unbuilt(b, "rations", p.Kind, p) }

// CONTROL AND DIPLOMACY -------------------------------------------------
//
// Stage 20, everything administrative. What control confers, and what a
// permission permits, are unwritten -- and so is the encounter record that
// decides whether a faction may name another at all.

func init() {
	registerUnbuilt(&Spec{
		Verb:     "control",
		Subjects: []string{SubjectShip, SubjectColony},
		Summary:  "take control of an uncontrolled entity or planet where the entity stands",
		Syntax: []string{
			"ship SHIP-ID control (ship | colony) ID",
			"ship SHIP-ID control system SYSTEM orbit ORBIT",
			"colony COLONY-ID control (ship | colony) ID",
			"colony COLONY-ID control system SYSTEM orbit ORBIT",
		},
		Phase: PhaseControl,
		Decode: func(actor int64, encoded string) (Params, error) {
			return decode(encoded, ControlParams{actorOf: actorOf{EntityID: actor}})
		},
		Parse: func(subject Subject, p *Parser) (Params, error) {
			order := ControlParams{actorOf: actorOf{Kind: subject.Kind, EntityID: subject.ID}}
			// Taking control is a physical act, so the place is named the short
			// way: the entity is already there, and only its own system's
			// orbits are within reach.
			if _, ok := p.keyword("system"); ok {
				var err error
				if order.System, err = p.systemLetter(); err != nil {
					return nil, err
				}
				if err = p.expect("orbit"); err != nil {
					return nil, err
				}
				order.Orbit, err = p.number("orbit")
				return order, err
			}
			target, err := p.entityRef()
			if err != nil {
				return nil, err
			}
			order.Target = &target
			return order, nil
		},
	})

	registerUnbuilt(&Spec{
		Verb:     "release",
		Subjects: []string{SubjectFaction},
		Summary:  "give up control of an entity or a planet",
		Syntax: []string{
			"we release (ship | colony) ID",
			"we release (X,Y,Z) system SYSTEM orbit ORBIT",
		},
		Phase: PhaseRelease,
		Decode: func(actor int64, encoded string) (Params, error) {
			return decode(encoded, ReleaseParams{})
		},
		Parse: func(subject Subject, p *Parser) (Params, error) {
			var order ReleaseParams
			// Releasing is administrative and needs no entity at the place, so
			// a planet is named the long way, by coordinates.
			if word, ok := p.peek(); ok && !word.quoted && word.text == "(" {
				at, err := p.planet()
				if err != nil {
					return nil, err
				}
				order.Place = &at
				return order, nil
			}
			target, err := p.entityRef()
			if err != nil {
				return nil, err
			}
			order.Target = &target
			return order, nil
		},
	})

	for _, item := range []struct {
		verb, summary string
		phase         *Phase
	}{
		{"grant", "give a faction leave to trade at a station or to colonize a planet", PhaseGrant},
		{"refuse", "take back a faction's leave to trade at a station or to colonize a planet", PhaseRefuse},
	} {
		verb, phase := item.verb, item.phase
		registerUnbuilt(&Spec{
			Verb:     verb,
			Subjects: []string{SubjectFaction},
			Summary:  item.summary,
			Syntax: []string{
				"we " + verb + " trade (X,Y,Z) system SYSTEM orbit ORBIT station STATION-NO to faction FACTION-ID",
				"we " + verb + " colonize (X,Y,Z) system SYSTEM orbit ORBIT to faction FACTION-ID",
			},
			Phase: phase,
			Decode: func(actor int64, encoded string) (Params, error) {
				return decode(encoded, PermissionParams{Verb: verb})
			},
			Parse: func(subject Subject, p *Parser) (Params, error) {
				order := PermissionParams{Verb: verb}
				kind, ok := p.keyword(permissionTrade, permissionColonize)
				if !ok {
					return nil, errShape
				}
				order.Permission = kind
				var err error
				if order.Place, err = p.planet(); err != nil {
					return nil, err
				}
				// A trade permission is about one station at the planet; a
				// colonize permission is about the planet itself.
				if kind == permissionTrade {
					if err = p.expect("station"); err != nil {
						return nil, err
					}
					if order.Station, err = p.number("a station number"); err != nil {
						return nil, err
					}
				}
				if err = p.expect("to"); err != nil {
					return nil, err
				}
				if err = p.expect("faction"); err != nil {
					return nil, err
				}
				order.Faction, err = p.entityID("faction")
				return order, err
			},
		})
	}
}

// The two permissions a grant or a refuse carries.
const (
	permissionTrade    = "trade"
	permissionColonize = "colonize"
)

// ControlParams is a CONTROL: an entity to take, or a planet in the system the
// acting entity is already in.
type ControlParams struct {
	actorOf
	Target *entityRef `json:"target,omitempty"`
	System string     `json:"system,omitempty"`
	Orbit  int        `json:"orbit,omitempty"`
}

// Input is the order as the player wrote it.
func (p ControlParams) Input() string {
	if p.Target != nil {
		return p.Target.String()
	}
	return orbitInput(p.System, p.Orbit)
}

// Bind settles who may give the order and nothing else: what control confers is
// unwritten.
func (p ControlParams) Bind(b *Binder) ([]Bound, error) { return unbuilt(b, "control", p.Kind, p) }

// ReleaseParams is a RELEASE: an entity to give up, or a planet named the long
// way, because no ship or colony carries the order out.
type ReleaseParams struct {
	factionOrder
	Target *entityRef `json:"target,omitempty"`
	Place  *place     `json:"place,omitempty"`
}

// Input is the order as the player wrote it.
func (p ReleaseParams) Input() string {
	if p.Target != nil {
		return p.Target.String()
	}
	return p.Place.String()
}

// Bind has no actor to find: a release is a faction order and needs no entity
// at the place at all.
func (p ReleaseParams) Bind(b *Binder) ([]Bound, error) { return unbuilt(b, "release", "", p) }

// PermissionParams is a GRANT or a REFUSE: which permission, where, and to
// whom.
type PermissionParams struct {
	factionOrder
	Verb       string `json:"-"`
	Permission string `json:"permission"`
	Place      place  `json:"place"`
	Station    int    `json:"station,omitempty"`
	Faction    int64  `json:"faction"`
}

// Input is the order as the player wrote it.
func (p PermissionParams) Input() string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s %s", p.Permission, p.Place)
	if p.Permission == permissionTrade {
		fmt.Fprintf(&out, " station %d", p.Station)
	}
	fmt.Fprintf(&out, " to faction %d", p.Faction)
	return out.String()
}

// Bind has no actor to find.
func (p PermissionParams) Bind(b *Binder) ([]Bound, error) { return unbuilt(b, p.Verb, "", p) }

// CREATE, THE GROUP FORMS -----------------------------------------------
//
// Stage 5, and the other half of the create verb. A group create is
// kill-and-fill: it runs once, in the turn it was given, builds as much as the
// labour and materials pay for, and closes out. Nothing about the ship and
// colony build applies to it. What a factory, a farm, or a mine produces per
// turn is unwritten, and so is the work_group column that would hold what a
// factory is making.

// parseCreateGroup reads the three group forms of a create. The group kind is
// already consumed, because it is what told the parser which half of the verb
// this is.
func parseCreateGroup(subject Subject, p *Parser, group string) (Params, error) {
	order := CreateGroupParams{actorOf: actorOf{Kind: subject.Kind, EntityID: subject.ID}, Group: group}
	// The group kind is the word before the cursor: it is what told the parser
	// which half of the verb this is, so it is already consumed.
	if err := groupAllows(p, p.pos-1, subject.Kind, group); err != nil {
		return nil, err
	}
	if err := p.expect("with"); err != nil {
		return nil, err
	}
	units := p.pos
	var err error
	if order.Units, err = p.unitList(); err != nil {
		return nil, err
	}
	if err := groupLots(p, units, group, order.Units); err != nil {
		return nil, err
	}
	switch group {
	case groupFactory:
		// A factory group must say what it will make.
		if err = p.expect("making"); err != nil {
			return nil, err
		}
		order.Making, err = p.unitTag()
	case groupMine:
		// A mine group's deposit is fixed for its life, which is why moving a
		// mine is a remove and a fresh create rather than an order of its own.
		if err = p.expect("working"); err != nil {
			return nil, err
		}
		if err = p.expect("deposit"); err != nil {
			return nil, err
		}
		order.Deposit, err = p.number("a deposit number")
	}
	return order, err
}

// CreateGroupParams is a CREATE in one of its three group forms.
type CreateGroupParams struct {
	actorOf
	Group   string         `json:"group"`
	Units   []UnitQuantity `json:"units"`
	Making  string         `json:"making,omitempty"`
	Deposit int            `json:"deposit,omitempty"`
}

// Input is the order as the player wrote it.
func (p CreateGroupParams) Input() string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s with %s", p.Group, unitListInput(p.Units))
	if p.Making != "" {
		fmt.Fprintf(&out, " making %s", p.Making)
	}
	if p.Deposit != 0 {
		fmt.Fprintf(&out, " working deposit %d", p.Deposit)
	}
	return out.String()
}

// Bind settles who may give the order and nothing else.
func (p CreateGroupParams) Bind(b *Binder) ([]Bound, error) { return unbuilt(b, "create", p.Kind, p) }
