// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"fmt"

	"github.com/mdhender/ecvb/internal/cadre"
	"github.com/mdhender/ecvb/internal/transport"
	"github.com/mdhender/ecvb/internal/units"
	"github.com/mdhender/ecvb/internal/world"
)

// A build's turn is three sweeps on three stages, each hung where the resource
// it competes for is settled: it claims at 5, delivers at 9, and completes at
// 10. They are sweeps rather than orders because nobody writes them -- the
// create order that began the build departed and succeeded, perhaps turns ago.
//
// One rule runs through all three:
//
//	Explicitly ordered work outranks a standing commitment.
//
// A sweep runs after its phase's orders, so a transfer order is served before a
// build's claim and an assemble order before a build's own assembly. A build
// takes what is left, which is all it ever needs to do: a build never fails for
// want, it only slows.

// claimBuilds is stage 5. For each entity feeding builds, it walks its builds
// in seniority order and puts a claim on the stock it holds and has not already
// promised.
//
// Claiming moves nothing and needs no transport. It is the priority decision,
// and it is here because this is where creation's ordering has always been
// settled -- which also keeps the property that a build cannot claim units that
// only arrived this turn, transfers being four stages further on.
func claimBuilds(w *world.World, turn int) error {
	for _, builder := range w.Entities() {
		builds := w.Builds(builder.ID)
		if len(builds) == 0 {
			continue
		}
		// One tally across the builder's builds, so a senior build's claim is
		// not offered to a junior one as well.
		promised := make(map[world.Stack]int64)
		people := make(map[string]int64)
		for _, build := range builds {
			for _, item := range eligible(w, build) {
				want := item.Wanted()
				if want <= 0 {
					continue
				}
				var have int64
				if item.Population() {
					have = builder.Unassigned(item.Unit) - people[item.Unit]
				} else {
					// A claim is on cargo, because that is the only section a
					// transport loads from. Stowing is what readies a load for
					// a build, the same as for a transfer.
					stack := world.Stack{Section: units.SectionCargo, Unit: item.Unit, TechLevel: item.TechLevel}
					have = builder.Inventory[stack] - promised[stack]
				}
				claim := min(want, have)
				if claim <= 0 {
					continue
				}
				if item.Population() {
					people[item.Unit] += claim
				} else {
					promised[world.Stack{Section: units.SectionCargo, Unit: item.Unit, TechLevel: item.TechLevel}] += claim
				}
				if err := w.Claim(build, item, claim); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// deliverBuilds is stage 9. Everything claimed rides out on whatever transports
// the builder has left after its transfer orders, and the construction workers
// ride with it.
//
// Materials go first and the workers take what capacity is left. Delivered
// material is a real state that keeps across turns, while a worker who could
// not be carried costs only that turn's shift, so filling the hold with workers
// first would be the more expensive mistake.
//
// What does not fit is released. A claim lives for one turn and is never
// banked: next turn's claiming runs afresh, in seniority order, so a senior
// build's priority is renewed rather than carried over.
func deliverBuilds(w *world.World, turn int) error {
	for _, builder := range w.Entities() {
		for _, build := range w.Builds(builder.ID) {
			site := w.Entity(build.EntityID)
			if site == nil {
				continue
			}
			if err := carry(w, builder, build, site); err != nil {
				return err
			}
			if err := w.ReleaseClaims(build); err != nil {
				return err
			}
		}
	}
	return nil
}

// carry moves one build's claim, and then its workers, on what the builder can
// crew and pay for.
func carry(w *world.World, builder *world.Entity, build *world.Build, site *world.Entity) error {
	free := affordable(w, builder)
	capacity := transport.Capacity(free)
	// An entity cannot hold more than it encloses, so what may be delivered is
	// bounded by the space the build has made so far. This is the other half of
	// structure-first: structure delivered to an unfinished entity consumes no
	// enclosed space, and nothing else is exempt, so nothing else can arrive
	// until structure has made somewhere to put it.
	room, err := site.CargoRoom()
	if err != nil {
		return err
	}
	var carried transport.Load
	for _, item := range build.Items {
		if item.Claimed == 0 {
			continue
		}
		quantity := item.Claimed
		metrics := units.MetricsForStored(item.Unit, item.TechLevel)
		if item.Population() {
			metrics = units.PopulationMetrics
			// Unsupported people never leave the entity handing them over.
			// Only assembled LFSU supports anyone, so life support delivered
			// and not yet worked keeps nobody alive.
			if room, capped := site.SupportedPopulation(); capped {
				quantity = min(quantity, room)
			}
		}
		if metrics.Mass > 0 {
			quantity = min(quantity, (capacity.Mass-carried.Mass)/metrics.Mass)
		}
		if metrics.CargoVolume > 0 {
			quantity = min(quantity, (capacity.Volume-carried.Volume)/metrics.CargoVolume)
		}
		space := site.CargoVolumePerUnit(item.Unit, item.TechLevel)
		if space > 0 {
			quantity = min(quantity, room/space)
		}
		if quantity = max(quantity, 0); quantity == 0 {
			continue
		}
		room -= quantity * space
		// Everything a transport carries is set down in cargo. A using line
		// waits there until stage 10 assembles it; a transfering line is
		// already where it was going, so it is finished on arrival.
		if item.Population() {
			if err := w.HandPopulation(builder, site, item.Unit, quantity); err != nil {
				return err
			}
		} else if err := w.Hand(builder, site, item.Unit, item.TechLevel, quantity); err != nil {
			return err
		}
		if err := w.Deliver(build, item, quantity); err != nil {
			return err
		}
		if item.Clause == world.ClauseTransfering {
			if err := w.Complete(build, item, quantity); err != nil {
				return err
			}
		}
		carried.Mass += quantity * metrics.Mass
		carried.Volume += quantity * metrics.CargoVolume
	}
	// The workers commute: they ride out, work the shift, and come home, so no
	// population row moves and the builder masses the same at the end of the
	// turn as at the start. What the commute spends is transport and fuel.
	shift := min(build.WorkerCap, w.WorkersFree(builder))
	if room := (capacity.Mass - carried.Mass) / commuteMass; room < shift {
		shift = max(room, 0)
	}
	if shift > 0 {
		carried.Mass += shift * commuteMass
		carried.Volume += shift * commuteVolume
		w.AssignWorkers(build, shift)
	}
	used := transport.Pack(free, carried)
	cost := w.CommitTransports(builder, used)
	return w.BurnFuel(builder, cost)
}

// One CWKR is one SKW plus one USK, so a shift out and back is two population
// units of mass and volume per worker.
var (
	commuteMass   = 2 * units.PopulationMetrics.Mass
	commuteVolume = 2 * units.PopulationMetrics.CargoVolume
)

// affordable is the transports an entity has left that it can still pay the
// fuel for.
//
// The fuel is reckoned over every hull the entity used in the turn at once, so
// what one more hull costs depends on what is already out. A build never fails
// for want -- it only slows -- so a builder short of fuel carries a smaller
// load rather than nothing at all.
func affordable(w *world.World, builder *world.Entity) []transport.Hulls {
	free := w.TransportsFree(builder)
	spent := w.Squares(builder.ID)
	// ceil(x/10) is the fuel, so the squares the entity can still afford are
	// the ones that keep the rounded total within what it holds.
	room := 10*(transport.Fuel(spent)+builder.Fuel()) - spent
	var within []transport.Hulls
	for _, hull := range free {
		square := int64(hull.TechLevel) * int64(hull.TechLevel)
		if square == 0 || room <= 0 {
			continue
		}
		if count := min(hull.Count, room/square); count > 0 {
			within = append(within, transport.Hulls{TechLevel: hull.TechLevel, Count: count})
			room -= count * square
		}
	}
	return within
}

// completeBuilds is stage 10. The workers carried out at stage 9 work off what
// is on site, in the order the player wrote the lines, and then go home.
//
// The work is the build's own pool: its workers are at the new entity doing
// that build's work, so what they get through is reckoned for that build alone
// and rounds up on its own. It does not pool with the builder's assemble orders
// nor with a sibling build. The cadre they came out of is shared all the same,
// which is how a large assemble order and a build starve each other of workers
// without pooling their work.
func completeBuilds(w *world.World, turn int) error {
	for _, entity := range w.Entities() {
		build := entity.Build
		if build == nil {
			continue
		}
		if err := assembleOnSite(w, entity, build); err != nil {
			return err
		}
		if finished(build) {
			if err := w.FinishBuild(entity); err != nil {
				return err
			}
		}
	}
	return nil
}

// assembleOnSite puts one build's delivered units to work, as much of them as
// the workers on site manage.
func assembleOnSite(w *world.World, site *world.Entity, build *world.Build) error {
	allowed := w.WorkersOnSite(build) * cadre.WorkPerUnit
	done := int64(0)
	for _, item := range eligible(w, build) {
		if item.Clause != world.ClauseUsing || item.Delivered == 0 {
			continue
		}
		section, assemblable := units.AssembledSection(item.Unit)
		if !assemblable {
			continue
		}
		unitMass := units.MetricsForStored(item.Unit, item.TechLevel).Mass
		quantity := item.Delivered
		if unitMass > 0 {
			quantity = min(quantity, (allowed-done)/unitMass)
		}
		if quantity = max(quantity, 0); quantity == 0 {
			continue
		}
		shift := []world.Shift{{From: units.SectionCargo, To: section,
			Unit: item.Unit, TechLevel: item.TechLevel, Quantity: quantity}}
		// An entity cannot hold more than it encloses, so a lot that would not
		// fit is trimmed to what does rather than refused: the rest waits in
		// cargo for the structure that will make room for it.
		fits, err := howMuchFits(site, shift[0])
		if err != nil {
			return err
		}
		if quantity = min(quantity, fits); quantity == 0 {
			continue
		}
		shift[0].Quantity = quantity
		if err := w.ShiftAll(site, shift); err != nil {
			return err
		}
		if err := w.Complete(build, item, quantity); err != nil {
			return err
		}
		done += quantity * unitMass
	}
	return structureDone(w, build)
}

// howMuchFits is how many of a lot an entity has room to assemble. Assembling
// structure creates space rather than filling it, so a structural lot always
// fits; everything else is measured against the space the structure has made.
//
// A lot that would not fit is trimmed to what does rather than refused. That is
// not the rule an assemble order follows -- an order that would overpack fails
// whole -- and the difference is that nobody wrote this one: a build was told
// to go as fast as it can, so it assembles what there is room for and leaves
// the rest in cargo for the structure that will make room for it.
func howMuchFits(site *world.Entity, shift world.Shift) (int64, error) {
	occupied, usable, err := site.RoomAfter(nil)
	if err != nil {
		return 0, err
	}
	if units.IsStructural(shift.Unit) {
		return shift.Quantity, nil
	}
	one := shift
	one.Quantity = 1
	after, _, err := site.RoomAfter([]world.Shift{one})
	if err != nil {
		return 0, err
	}
	perUnit := after - occupied
	if perUnit <= 0 {
		return shift.Quantity, nil
	}
	return max(min(shift.Quantity, (usable-occupied)/perUnit), 0), nil
}

// structureDone records that every structural using line is finished, which is
// what makes the rest of the build eligible.
func structureDone(w *world.World, build *world.Build) error {
	if build.StructureComplete {
		return nil
	}
	for _, item := range build.Items {
		if item.Structural() && !item.Done() {
			return nil
		}
	}
	return w.SetStructureComplete(build)
}

// eligible is the lines of a build that can be worked now, in the order the
// player wrote them, which is their priority.
//
// A line that cannot make progress is skipped and the sweep goes on to the
// next: an unavailable line never freezes a build. As soon as a skipped line
// can be worked again it takes precedence over everything below it, so a badly
// ordered list costs a player turns rather than the build.
func eligible(w *world.World, build *world.Build) []*world.BuildItem {
	var ready []*world.BuildItem
	for _, item := range build.Items {
		if item.Done() {
			continue
		}
		// Until the structure is complete, only the structural using lines are
		// eligible. Nothing else can be delivered anyway: it needs enclosed
		// space, and structure is what makes some.
		if !build.StructureComplete && !item.Structural() {
			continue
		}
		ready = append(ready, item)
	}
	return ready
}

// finished reports whether every line of a build is completed.
func finished(build *world.Build) bool {
	for _, item := range build.Items {
		if !item.Done() {
			return false
		}
	}
	return true
}

// buildProgress is what a create order says about the build it began: the
// entity that now exists, so the player can give it orders once it is done.
func buildProgress(entity *world.Entity) string {
	return fmt.Sprintf("%s %d is under construction at %s",
		noun(entity), entity.Number, describeLocation(entity.Location))
}

// describeLocation names where an entity stands, for a message about it.
func describeLocation(at world.Location) string {
	if at.SystemID == 0 {
		return "the stellium orbit"
	}
	return fmt.Sprintf("ring %d of its planet", at.Ring)
}
