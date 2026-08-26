// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package fuel implements the FUEL rules that decide whether an entity can pay
// for what it is about to do, and that draw the fuel when it does.
package fuel

import (
	"fmt"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// Unit is the inventory unit code of fuel. Unlike a drive or a sensor, fuel
// works from any section: it is burned, not assembled.
const Unit = "FUEL"

// UnitMass is the mass in MU of one FUEL unit. FUEL is a bulk resource, so it
// masses 1 MU like the other three. See docs/units.md.
const UnitMass = 1

// drawOrder is the order the sections are emptied in. Fuel that is already
// plumbed into the entity goes first and freight goes last, so a hold of spare
// fuel survives until the working supply is gone.
var drawOrder = [...]string{"operational", "unassembled", "cargo"}

// Available returns the FUEL one entity holds across every section.
func Available(conn *sqlite.Conn, entityID int64) (int64, error) {
	var quantity int64
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT coalesce(sum(quantity), 0) FROM inventory
		WHERE entity_id = ? AND unit = ? AND quantity > 0;`, &sqlitex.ExecOptions{
		Args: []any{entityID, Unit},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			quantity = stmt.ColumnInt64(0)
			return nil
		},
	}); err != nil {
		return 0, fmt.Errorf("load fuel for entity %d: %w", entityID, err)
	}
	return quantity, nil
}

// AvailableAll returns the FUEL every entity in a game holds. An entity with
// no fuel is absent from the map, which reads as zero.
func AvailableAll(conn *sqlite.Conn, gameID int64) (map[int64]int64, error) {
	quantities := make(map[int64]int64)
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT i.entity_id, sum(i.quantity)
		FROM inventory AS i
		JOIN entity AS e ON e.id = i.entity_id
		JOIN faction AS f ON f.id = e.faction_id
		WHERE f.game_id = ? AND i.unit = ? AND i.quantity > 0
		GROUP BY i.entity_id;`, &sqlitex.ExecOptions{
		Args: []any{gameID, Unit},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			quantities[stmt.ColumnInt64(0)] = stmt.ColumnInt64(1)
			return nil
		},
	}); err != nil {
		return nil, fmt.Errorf("load fuel: %w", err)
	}
	return quantities, nil
}

// Spend burns quantity FUEL from an entity, emptying its operational fuel
// first, then its unassembled fuel, and its cargo last. Burned fuel leaves the
// entity, so the entity's mass falls with it.
//
// Spend reports an error when the entity does not hold the fuel. Callers check
// availability first and record a failed order rather than reaching this.
func Spend(conn *sqlite.Conn, entityID, quantity int64) error {
	if quantity < 0 {
		return fmt.Errorf("spend fuel on entity %d: quantity must be nonnegative", entityID)
	}
	if quantity == 0 {
		return nil
	}
	// Count the fuel before drawing any, so a shortfall leaves the entity
	// untouched rather than half-drained for a caller that does not roll back.
	held, err := Available(conn, entityID)
	if err != nil {
		return err
	}
	if held < quantity {
		return fmt.Errorf("spend fuel on entity %d: needs %d %s and holds %d", entityID, quantity, Unit, held)
	}
	remaining := quantity
	for _, section := range drawOrder {
		if remaining == 0 {
			break
		}
		held, err := heldInSection(conn, entityID, section)
		if err != nil {
			return err
		}
		for _, stack := range held {
			if remaining == 0 {
				break
			}
			drawn := min(stack.quantity, remaining)
			if err := drawFromStack(conn, entityID, section, stack.techLevel, stack.quantity-drawn); err != nil {
				return err
			}
			remaining -= drawn
		}
	}
	if remaining != 0 {
		return fmt.Errorf("spend fuel on entity %d: drew %d of %d %s", entityID, quantity-remaining, quantity, Unit)
	}
	if err := sqlitex.ExecuteTransient(conn, "UPDATE entity SET mass = mass - ? WHERE id = ?;", &sqlitex.ExecOptions{
		Args: []any{quantity * UnitMass, entityID},
	}); err != nil {
		return fmt.Errorf("reduce entity %d mass by spent fuel: %w", entityID, err)
	}
	if conn.Changes() != 1 {
		return fmt.Errorf("reduce entity %d mass by spent fuel: entity does not exist", entityID)
	}
	return nil
}

// stack is one inventory row of fuel: FUEL carries no technology level, so an
// entity normally holds one stack per section, but nothing in the schema says
// so and the draw walks whatever it finds.
type stack struct {
	techLevel int
	quantity  int64
}

func heldInSection(conn *sqlite.Conn, entityID int64, section string) ([]stack, error) {
	var stacks []stack
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT tech_level, quantity FROM inventory
		WHERE entity_id = ? AND section = ? AND unit = ? AND quantity > 0
		ORDER BY tech_level;`, &sqlitex.ExecOptions{
		Args: []any{entityID, section, Unit},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			stacks = append(stacks, stack{techLevel: stmt.ColumnInt(0), quantity: stmt.ColumnInt64(1)})
			return nil
		},
	}); err != nil {
		return nil, fmt.Errorf("load %s fuel for entity %d: %w", section, entityID, err)
	}
	return stacks, nil
}

// drawFromStack leaves one inventory stack holding exactly the given quantity.
// An emptied stack loses its row rather than keeping a zero.
func drawFromStack(conn *sqlite.Conn, entityID int64, section string, techLevel int, left int64) error {
	statement := "UPDATE inventory SET quantity = ? WHERE entity_id = ? AND section = ? AND unit = ? AND tech_level = ?;"
	args := []any{left, entityID, section, Unit, techLevel}
	if left == 0 {
		statement = "DELETE FROM inventory WHERE entity_id = ? AND section = ? AND unit = ? AND tech_level = ?;"
		args = args[1:]
	}
	if err := sqlitex.ExecuteTransient(conn, statement, &sqlitex.ExecOptions{Args: args}); err != nil {
		return fmt.Errorf("draw %s fuel from entity %d: %w", section, entityID, err)
	}
	if conn.Changes() != 1 {
		return fmt.Errorf("draw %s fuel from entity %d: stack changed while it was spending", section, entityID)
	}
	return nil
}
