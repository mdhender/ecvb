// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"slices"
	"strings"

	"github.com/mdhender/ecvb/internal/world"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// Result summarizes a checked or submitted order file.
type Result struct {
	GameCode  string
	Turn      int
	FactionID int64
	Orders    int
	Warnings  []Warning
}

// Warning is a condition that does not stop a submission but that the player
// should see. It is what an order's Apply reported when the order failed the
// dry run: the order is kept, because what Apply weighs -- fuel on hand today
// -- may be different when the turn resolves.
type Warning struct {
	Line    int
	Message string
}

// placed is a bound order with its place in the file and in the turn.
type placed struct {
	verb     string
	sequence int
	line     int
	bound    Bound
}

type validatedSubmission struct {
	result Result
	gameID int64
	orders []placed
}

// Check parses and validates an order file without changing the database.
func Check(ctx context.Context, conn *sqlite.Conn, r io.Reader) (Result, error) {
	submission, err := Parse(r)
	if err != nil {
		return Result{}, err
	}
	var validated validatedSubmission
	if err := discard(conn, func() (err error) {
		validated, err = simulate(ctx, conn, submission)
		return err
	}); err != nil {
		return Result{}, err
	}
	return validated.result, nil
}

// Submit parses and validates an order file, then atomically replaces the
// faction's submitted orders. A file with anything wrong in it stores nothing.
func Submit(ctx context.Context, conn *sqlite.Conn, r io.Reader) (result Result, err error) {
	submission, err := Parse(r)
	if err != nil {
		return Result{}, err
	}
	end, err := sqlitex.ImmediateTransaction(conn)
	if err != nil {
		return Result{}, fmt.Errorf("begin submit orders transaction: %w", err)
	}
	defer end(&err)

	var validated validatedSubmission
	if err = discard(conn, func() (err error) {
		validated, err = simulate(ctx, conn, submission)
		return err
	}); err != nil {
		return Result{}, err
	}
	// The dry run has been rolled back and the orders are known to be legal.
	// Replacing them is the only thing this transaction keeps.
	//
	// What an order recorded goes before the order itself, because the record
	// points at it; the findings go too, because a resubmitted file reads the
	// planets again.
	for _, table := range []string{"probe_contact", "probe_deposit",
		"order_movement", "order_survey", "game_order"} {
		if err = sqlitex.ExecuteTransient(conn,
			"DELETE FROM "+table+" WHERE game_id = ? AND turn = ? AND faction_id = ?;", &sqlitex.ExecOptions{
				Args: []any{validated.gameID, validated.result.Turn, validated.result.FactionID},
			}); err != nil {
			return Result{}, fmt.Errorf("delete previous %s rows: %w", table, err)
		}
	}
	for _, order := range validated.orders {
		if err = store(conn, validated.gameID, submission.Turn, validated.result.FactionID, order); err != nil {
			return Result{}, fmt.Errorf("store the order on line %d: %w", order.line, err)
		}
	}
	return validated.result, nil
}

// store writes one bound order. Every order is a row of game_order whatever
// its verb, so this is written once rather than once per kind of order.
func store(conn *sqlite.Conn, gameID int64, turn int, factionID int64, order placed) error {
	params := order.bound.Params()
	encoded, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encode %s parameters: %w", order.verb, err)
	}
	if err := sqlitex.ExecuteTransient(conn, `
		INSERT INTO game_order (
			game_id, turn, faction_id, sequence, source_line,
			verb, actor_entity_id, input, params, fuel_spent
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`, &sqlitex.ExecOptions{
		Args: []any{gameID, turn, factionID, order.sequence, order.line,
			order.verb, nullableID(params.Actor()), params.Input(), string(encoded), order.bound.Fuel()},
	}); err != nil {
		return fmt.Errorf("insert %s order: %w", order.verb, err)
	}
	return nil
}

// errDiscarded forces a savepoint to roll back. It never reaches a caller.
var errDiscarded = errors.New("orders: discarding a dry run")

// discard runs fn inside a savepoint that is always rolled back.
//
// Checking a file and submitting one both have to know what the turn will do,
// and the honest way to know is to do it: burn the fuel, move the ships,
// record what the probes read, and then put the database back the way it was
// found. It is what lets one implementation of a rule serve the player's check
// and the engine's resolution both.
func discard(conn *sqlite.Conn, fn func() error) error {
	release := sqlitex.Save(conn)
	err := fn()
	rollback := err
	if rollback == nil {
		rollback = errDiscarded
	}
	release(&rollback)
	return err
}

// simulate plays a submitted file against the world and reports what happened.
// It is called inside discard, so everything it does is undone.
func simulate(ctx context.Context, conn *sqlite.Conn, submission Submission) (validatedSubmission, error) {
	if err := ctx.Err(); err != nil {
		return validatedSubmission{}, err
	}
	loaded, exists, err := world.Load(conn, submission.GameCode)
	if err != nil {
		return validatedSubmission{}, err
	}
	if !exists {
		return validatedSubmission{}, problems{{1, fmt.Sprintf("game %q does not exist", submission.GameCode)}}
	}
	game := loaded.Game()
	var found problems
	if game.Turn != submission.Turn {
		found = append(found, problem{1, fmt.Sprintf("game %q is on turn %d, not turn %d", game.Code, game.Turn, submission.Turn)})
	}
	if game.State != "open" {
		found = append(found, problem{1, fmt.Sprintf("game %q turn %d is resolved and not accepting orders", game.Code, game.Turn)})
	}
	factionID, identityProblems, err := resolveFaction(conn, game.ID, submission.Identity)
	if err != nil {
		return validatedSubmission{}, err
	}
	found = append(found, identityProblems...)
	if factionID == 0 {
		return validatedSubmission{}, found
	}

	binder := &Binder{World: loaded, FactionID: factionID}
	turn := &Turn{World: loaded, Number: submission.Turn, FactionID: factionID}
	written := byPhase(submission.Orders)
	var orders []placed
	var warnings []Warning
	sequence := 0
	// The same turn the engine will resolve, phase by phase, so a file is
	// priced against the world each phase leaves behind rather than against
	// the world as it stands now.
	for _, phase := range Phases() {
		for _, order := range written[phase] {
			if err := ctx.Err(); err != nil {
				return validatedSubmission{}, err
			}
			bounds, err := order.Params.Bind(binder)
			if err != nil {
				// A Bind failure will still be a failure when the turn
				// resolves, so the file is refused rather than stored. Every
				// reason is reported: a player fixing a file sees the list.
				for _, each := range eachError(err) {
					found = append(found, problem{order.Line, each.Error()})
				}
				continue
			}
			for _, bound := range bounds {
				sequence++
				turn.Sequence = sequence
				outcome, err := bound.Apply(turn)
				if err != nil {
					return validatedSubmission{}, err
				}
				if outcome.Status == StatusFailed {
					warnings = append(warnings, Warning{Line: order.Line,
						Message: outcome.Message + "; the order is kept in case that changes before the turn resolves"})
				}
				orders = append(orders, placed{verb: order.Verb, sequence: sequence, line: order.Line, bound: bound})
			}
		}
		if phase.Sweep == nil {
			continue
		}
		if err := phase.Sweep(loaded, submission.Turn); err != nil {
			return validatedSubmission{}, err
		}
	}
	if len(found) != 0 {
		return validatedSubmission{}, found
	}
	slices.SortStableFunc(warnings, func(a, b Warning) int { return cmp.Compare(a.Line, b.Line) })
	return validatedSubmission{
		result: Result{GameCode: submission.GameCode, Turn: submission.Turn, FactionID: factionID,
			Orders: len(orders), Warnings: warnings},
		gameID: game.ID,
		orders: orders,
	}, nil
}

// byPhase sorts a file's orders into the phases that will resolve them,
// leaving the orders of one phase in the order the player wrote them.
func byPhase(written []Order) map[*Phase][]Order {
	grouped := make(map[*Phase][]Order, len(phases))
	for _, order := range written {
		phase := PhaseOf(order.Verb)
		grouped[phase] = append(grouped[phase], order)
	}
	return grouped
}

func resolveFaction(conn *sqlite.Conn, gameID int64, identity Identity) (int64, problems, error) {
	if identity.PlayerEmail != "" {
		email := strings.ToLower(strings.TrimSpace(identity.PlayerEmail))
		address, parseErr := mail.ParseAddress(email)
		if parseErr != nil || address.Address != email {
			return 0, problems{{2, fmt.Sprintf("invalid player email %q", email)}}, nil
		}
		var factionID int64
		err := sqlitex.ExecuteTransient(conn, `
			SELECT f.id FROM faction AS f JOIN users AS u ON u.id = f.user_id
			WHERE f.game_id = ? AND u.email = ?;`, &sqlitex.ExecOptions{
			Args: []any{gameID, email},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				factionID = stmt.ColumnInt64(0)
				return nil
			},
		})
		if err != nil {
			return 0, nil, fmt.Errorf("find player %q: %w", email, err)
		}
		if factionID == 0 {
			return 0, problems{{2, fmt.Sprintf("player %q does not belong to this game", email)}}, nil
		}
		return factionID, nil, nil
	}
	var belongs bool
	err := sqlitex.ExecuteTransient(conn, "SELECT EXISTS (SELECT 1 FROM faction WHERE id = ? AND game_id = ?);", &sqlitex.ExecOptions{
		Args: []any{identity.FactionID, gameID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			belongs = stmt.ColumnInt(0) != 0
			return nil
		},
	})
	if err != nil {
		return 0, nil, fmt.Errorf("find faction %d: %w", identity.FactionID, err)
	}
	if !belongs {
		return 0, problems{{2, fmt.Sprintf("faction %d does not belong to this game", identity.FactionID)}}, nil
	}
	return identity.FactionID, nil, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}
