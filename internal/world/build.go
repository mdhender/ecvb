// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package world

import (
	"fmt"
	"slices"

	"github.com/mdhender/ecvb/internal/cadre"
	"github.com/mdhender/ecvb/internal/units"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// A build is live state beside the order rather than a state of it. The create
// order that began it departed and succeeded, the way a jump does, and these
// rows carry what continues. Nothing purges them, for the reason nothing purges
// in_transit.
//
// This file is the only thing that reads or writes under_construction and
// construction_item, on the same principle as inventory.go: what a build has
// claimed, delivered, and completed is one story, and letting two places tell
// it is what would let them drift.

// The two clauses of a create order. They do not mean the same thing: a using
// line names what the new entity is made of and completes when its units are
// assembled into it; a transfering line names what is handed over rather than
// built in, and completes when its units are stowed in cargo or, for a
// population class, when the people are aboard.
const (
	ClauseUsing       = "using"
	ClauseTransfering = "transfering"
)

// Build is a ship or colony under construction: the entity being built, who is
// feeding it, and the two lists the order named.
type Build struct {
	// EntityID is the unfinished entity's row id, and is also the build's
	// seniority. A row id rises monotonically and is never reused, so one
	// builder's builds are already in the order they started. It is the row id
	// and not entity.number, which is a permutation and carries no order.
	EntityID int64
	// BuilderID is the entity feeding this build: it claims from that entity's
	// stock, carries on its transports, and borrows its construction workers a
	// turn at a time.
	BuilderID int64
	// WorkerCap is the `with` clause -- a ceiling on the workers the build may
	// use in a turn, never a reservation. It holds nothing back.
	WorkerCap int64
	// StructureComplete is set when every structural using line is completed.
	// Until then only the STRC and STRL lines are eligible.
	StructureComplete bool
	// TradeStation is the `as trade-station` flag, carried through to the
	// finished entity. Nothing reads it yet.
	TradeStation bool
	// Items are the build's lines, in the order the player wrote them, which
	// is their priority.
	Items []*BuildItem
}

// BuildItem is one line of a build's lists.
type BuildItem struct {
	Ordinal   int
	Clause    string
	Unit      string
	TechLevel int
	Required  int64
	// Claimed is this turn's call on the builder's stock, put on at stage 5 and
	// consumed at stage 9. A claim lives for one turn and is never banked.
	Claimed int64
	// Delivered is at the new entity and not yet worked.
	Delivered int64
	// Completed is assembled, stowed, or aboard.
	Completed int64
}

// Tag renders the item's unit the way a player writes it.
func (i *BuildItem) Tag() string {
	if !units.StoredHasTechLevel(i.TechLevel) {
		return i.Unit
	}
	return fmt.Sprintf("%s-%d", i.Unit, i.TechLevel)
}

// Wanted is what is still to be done on a line: what was asked for, less what
// is claimed, on site, or finished. It is derived rather than stored.
func (i *BuildItem) Wanted() int64 {
	return i.Required - i.Claimed - i.Delivered - i.Completed
}

// Population reports whether the line names people rather than units. Only a
// transfering line can, because people are carried and fed rather than built
// into anything.
func (i *BuildItem) Population() bool { return units.IsPopulation(i.Unit) }

// Structural reports whether the line is one of the two that enclose space.
// Until every one of them is completed, nothing else on the build is eligible.
func (i *BuildItem) Structural() bool {
	return i.Clause == ClauseUsing && units.IsStructural(i.Unit)
}

// Done reports whether the line needs nothing more.
func (i *BuildItem) Done() bool { return i.Completed >= i.Required }

// UnderConstruction reports whether the entity is still being built. Such an
// entity can be given no order, the way a ship in transit cannot: it exists,
// it is visible, and it is not yet a thing that acts.
func (e *Entity) UnderConstruction() bool { return e.Build != nil }

// Builds lists the unfinished entities this entity is feeding, in seniority
// order, which is the order their builds started.
func (w *World) Builds(builderID int64) []*Build {
	var found []*Build
	for _, entity := range w.Entities() {
		if entity.Build != nil && entity.Build.BuilderID == builderID {
			found = append(found, entity.Build)
		}
	}
	slices.SortFunc(found, func(a, b *Build) int { return int(a.EntityID - b.EntityID) })
	return found
}

// CreateEntity puts a new, unfinished entity on the board and records the build
// that will finish it.
//
// The entity exists from this moment: it belongs to the faction building it,
// it has a location and a mass, and probes and sensors read it like anything
// else. What it does not have is anything in it -- a build delivers that over
// the turns that follow.
//
// A ship built at a planet settles into a ring of its own, drawn the way an
// arriving ship draws one. The draw is addressed by the entity's number, which
// is taken before the row is written, so the ring is known before the insert
// and the entity goes down where it belongs. One built from an entity in the
// stellium orbit is built there and has no ring at all.
func (w *World) CreateEntity(factionID int64, kind string, techLevel, turn int, at Location, build *Build) (*Entity, error) {
	number, err := NextEntityNumber(w.conn, w.game.ID)
	if err != nil {
		return nil, err
	}
	entity := &Entity{
		Number: number, Unit: kind, FactionID: factionID,
		FactionNumber: w.factions[factionID], Location: at,
		TradeStation: build.TradeStation,
		Inventory:    make(Inventory),
		Population:   make(map[string]int64),
		Cadre:        make(map[string]int64),
	}
	if kind == "SHIP" && at.PlanetID != 0 {
		ring, err := w.DrawRing(at, turn, entity)
		if err != nil {
			return nil, err
		}
		at.Ring = ring
		entity.Location = at
	}
	if err := sqlitex.ExecuteTransient(w.conn, `
		INSERT INTO entity (game_id, number, unit, tech_level, stellium_id, system_id, planet_id, planet_ring,
			faction_id, enclosed_volume, mass, trade_station)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, ?);`, &sqlitex.ExecOptions{
		Args: []any{w.game.ID, number, kind, techLevel, nullableID(at.StelliumID), nullableID(at.SystemID),
			nullableID(at.PlanetID), nullableRing(at), factionID, boolean(build.TradeStation)},
	}); err != nil {
		return nil, fmt.Errorf("create a %s for faction %d: %w", kind, factionID, err)
	}
	id := w.conn.LastInsertRowID()
	entity.ID = id
	build.EntityID = id
	entity.Build = build
	if err := sqlitex.ExecuteTransient(w.conn, `
		INSERT INTO under_construction (entity_id, game_id, builder_entity_id, cwkr_cap, trade_station)
		VALUES (?, ?, ?, ?, ?);`, &sqlitex.ExecOptions{
		Args: []any{id, w.game.ID, build.BuilderID, build.WorkerCap, boolean(build.TradeStation)},
	}); err != nil {
		return nil, fmt.Errorf("record the build of entity %d: %w", id, err)
	}
	for _, item := range build.Items {
		if err := sqlitex.ExecuteTransient(w.conn, `
			INSERT INTO construction_item (entity_id, ordinal, clause, unit, tech_level, required)
			VALUES (?, ?, ?, ?, ?, ?);`, &sqlitex.ExecOptions{
			Args: []any{id, item.Ordinal, item.Clause, item.Unit, item.TechLevel, item.Required},
		}); err != nil {
			return nil, fmt.Errorf("record line %d of the build of entity %d: %w", item.Ordinal, id, err)
		}
	}
	w.entities[id] = entity
	w.byNumber[number] = entity
	return entity, nil
}

// Claim puts this turn's call on the builder's stock. It moves nothing and
// needs no transport: it is the priority decision, and it is settled at stage 5
// because that is where creation's ordering has always been settled.
func (w *World) Claim(build *Build, item *BuildItem, quantity int64) error {
	if quantity <= 0 {
		return nil
	}
	return w.setBuildItem(build, item, item.Claimed+quantity, item.Delivered, item.Completed)
}

// ReleaseClaims gives back everything a build claimed and did not get carried.
// A claim lives for one turn; next turn's claiming runs afresh in seniority
// order, so a senior build's priority is renewed rather than banked.
func (w *World) ReleaseClaims(build *Build) error {
	for _, item := range build.Items {
		if item.Claimed == 0 {
			continue
		}
		if err := w.setBuildItem(build, item, 0, item.Delivered, item.Completed); err != nil {
			return err
		}
	}
	return nil
}

// Deliver carries part of a claim to the new entity. Everything a transport
// carries is set down in cargo, so a using line waits there until stage 10
// assembles it and a transfering line is already where it was going.
func (w *World) Deliver(build *Build, item *BuildItem, quantity int64) error {
	if quantity <= 0 {
		return nil
	}
	if quantity > item.Claimed {
		return fmt.Errorf("deliver line %d of entity %d: claimed %d and delivering %d",
			item.Ordinal, build.EntityID, item.Claimed, quantity)
	}
	return w.setBuildItem(build, item, item.Claimed-quantity, item.Delivered+quantity, item.Completed)
}

// Complete finishes part of a line: a using line's units are assembled, a
// transfering line's are already stowed, and a population line's people are
// aboard.
func (w *World) Complete(build *Build, item *BuildItem, quantity int64) error {
	if quantity <= 0 {
		return nil
	}
	if quantity > item.Delivered {
		return fmt.Errorf("complete line %d of entity %d: delivered %d and completing %d",
			item.Ordinal, build.EntityID, item.Delivered, quantity)
	}
	return w.setBuildItem(build, item, item.Claimed, item.Delivered-quantity, item.Completed+quantity)
}

// SetStructureComplete records that every structural using line is done, which
// is what makes the rest of the build eligible.
func (w *World) SetStructureComplete(build *Build) error {
	if build.StructureComplete {
		return nil
	}
	if err := sqlitex.ExecuteTransient(w.conn,
		"UPDATE under_construction SET structure_complete = 1 WHERE entity_id = ?;", &sqlitex.ExecOptions{
			Args: []any{build.EntityID},
		}); err != nil {
		return fmt.Errorf("mark the structure of entity %d complete: %w", build.EntityID, err)
	}
	build.StructureComplete = true
	return nil
}

// FinishBuild takes the build away, and what is left is an ordinary entity.
//
// Nothing has to be released: the workers were never held, only assigned a turn
// at a time, so the turn a build finishes is simply the first turn they are all
// idle again.
func (w *World) FinishBuild(entity *Entity) error {
	if entity.Build == nil {
		return nil
	}
	for _, statement := range []string{
		"DELETE FROM construction_item WHERE entity_id = ?;",
		"DELETE FROM under_construction WHERE entity_id = ?;",
	} {
		if err := sqlitex.ExecuteTransient(w.conn, statement, &sqlitex.ExecOptions{
			Args: []any{entity.ID},
		}); err != nil {
			return fmt.Errorf("finish the build of entity %d: %w", entity.Number, err)
		}
	}
	entity.Build = nil
	return nil
}

// setBuildItem writes one line's three counters, in the database and in the
// loaded copy, so the second sweep of a turn reads the line as the first left
// it.
func (w *World) setBuildItem(build *Build, item *BuildItem, claimed, delivered, completed int64) error {
	if claimed < 0 || delivered < 0 || completed < 0 {
		return fmt.Errorf("set line %d of entity %d: a count must be nonnegative", item.Ordinal, build.EntityID)
	}
	if claimed+delivered+completed > item.Required {
		return fmt.Errorf("set line %d of entity %d: %d of %d wanted",
			item.Ordinal, build.EntityID, claimed+delivered+completed, item.Required)
	}
	if err := sqlitex.ExecuteTransient(w.conn, `
		UPDATE construction_item SET claimed = ?, delivered = ?, completed = ?
		WHERE entity_id = ? AND ordinal = ?;`, &sqlitex.ExecOptions{
		Args: []any{claimed, delivered, completed, build.EntityID, item.Ordinal},
	}); err != nil {
		return fmt.Errorf("set line %d of entity %d: %w", item.Ordinal, build.EntityID, err)
	}
	item.Claimed, item.Delivered, item.Completed = claimed, delivered, completed
	return nil
}

func (w *World) loadBuilds() error {
	builds := make(map[int64]*Build)
	if err := sqlitex.ExecuteTransient(w.conn, `
		SELECT entity_id, builder_entity_id, cwkr_cap, structure_complete, trade_station
		FROM under_construction WHERE game_id = ?;`, &sqlitex.ExecOptions{
		Args: []any{w.game.ID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			build := &Build{
				EntityID: stmt.ColumnInt64(0), BuilderID: stmt.ColumnInt64(1),
				WorkerCap: stmt.ColumnInt64(2), StructureComplete: stmt.ColumnInt(3) != 0,
				TradeStation: stmt.ColumnInt(4) != 0,
			}
			builds[build.EntityID] = build
			return nil
		},
	}); err != nil {
		return fmt.Errorf("load builds: %w", err)
	}
	if err := sqlitex.ExecuteTransient(w.conn, `
		SELECT ci.entity_id, ci.ordinal, ci.clause, ci.unit, ci.tech_level,
		       ci.required, ci.claimed, ci.delivered, ci.completed
		FROM construction_item AS ci
		JOIN under_construction AS uc ON uc.entity_id = ci.entity_id
		WHERE uc.game_id = ?
		ORDER BY ci.entity_id, ci.ordinal;`, &sqlitex.ExecOptions{
		Args: []any{w.game.ID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			build, ok := builds[stmt.ColumnInt64(0)]
			if !ok {
				return nil
			}
			build.Items = append(build.Items, &BuildItem{
				Ordinal: stmt.ColumnInt(1), Clause: stmt.ColumnText(2),
				Unit: stmt.ColumnText(3), TechLevel: stmt.ColumnInt(4),
				Required: stmt.ColumnInt64(5), Claimed: stmt.ColumnInt64(6),
				Delivered: stmt.ColumnInt64(7), Completed: stmt.ColumnInt64(8),
			})
			return nil
		},
	}); err != nil {
		return fmt.Errorf("load build lines: %w", err)
	}
	for id, build := range builds {
		if entity, ok := w.entities[id]; ok {
			entity.Build = build
		}
	}
	return nil
}

// boolean renders a Go bool the way SQLite stores one.
func boolean(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

// WorkersFree is the construction workers an entity has not spent this turn:
// its cadre, less the workers its own assembly and unassembly used, less the
// ones its builds have already taken.
//
// A worker does one task per turn, so a build and an assemble order at one
// entity draw down one pool. What they do not share is the work: a build's
// workers are at the new entity doing that build's work, so what they get
// through is reckoned for that build alone.
func (w *World) WorkersFree(entity *Entity) int64 {
	spent := cadre.WorkersFor(w.WorkDone(cadre.Assembly, entity.ID)) +
		cadre.WorkersFor(w.WorkDone(cadre.Unassembly, entity.ID))
	for id, count := range w.workers {
		if build := w.entities[id]; build != nil && build.Build != nil && build.Build.BuilderID == entity.ID {
			spent += count
		}
	}
	return max(entity.ConstructionWorkers()-spent, 0)
}

// AssignWorkers lends a build construction workers for the turn, and reports
// how many it has.
func (w *World) AssignWorkers(build *Build, count int64) { w.workers[build.EntityID] += count }

// WorkersOnSite is how many construction workers reached a build this turn. A
// worker who could not be carried cannot work.
func (w *World) WorkersOnSite(build *Build) int64 { return w.workers[build.EntityID] }
