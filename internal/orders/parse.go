// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package orders parses and validates player order files.
package orders

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// StelliumOrbit is the orbit a MOVE order names to send a ship back to the
// stellium orbit. No planet occupies it: it is a fiction that gives MOVE a way
// to say "leave the planets", and it is never a probe target.
const StelliumOrbit = 11

// Identity identifies the faction submitting an order file.
type Identity struct {
	PlayerEmail string
	FactionID   int64
}

// Order is one parsed order line: where it came from, what it is, and what it
// says. Params is the order's own type -- MoveParams, JumpParams, ProbeParams
// -- so a field belongs to the one order that has it.
type Order struct {
	Line   int
	Verb   string
	Params Params
}

// Submission is the parsed contents of an order file.
type Submission struct {
	GameCode string
	Turn     int
	Identity Identity
	Orders   []Order
}

type problem struct {
	line    int
	message string
}

type problems []problem

func (p problems) Error() string {
	lines := make([]string, len(p))
	for i, item := range p {
		lines[i] = fmt.Sprintf("line %d: %s", item.line, item.message)
	}
	return strings.Join(lines, "\n")
}

// Parse parses an order file without consulting the database.
//
// The first two physical lines are the header: the game and turn, then the
// submitting player or faction. Everything after them is an order, one per
// line. Blank lines are allowed, and a `#` outside quotes begins a comment.
//
// Parse reports every problem it finds rather than stopping at the first, so a
// player fixing a file sees the whole list.
func Parse(r io.Reader) (Submission, error) {
	var submission Submission
	var found problems
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	number := 0
	for scanner.Scan() {
		number++
		line := newLine(number, scanner.Text())
		switch number {
		case 1:
			if err := parseGameLine(line, &submission); err != nil {
				found = append(found, problem{number, err.Error()})
			}
		case 2:
			if err := parseIdentityLine(line, &submission); err != nil {
				found = append(found, problem{number, err.Error()})
			}
		default:
			if line.empty() {
				continue
			}
			order, err := parseOrder(line)
			if err != nil {
				found = append(found, problem{number, err.Error()})
				continue
			}
			submission.Orders = append(submission.Orders, order)
		}
	}
	if err := scanner.Err(); err != nil {
		return Submission{}, fmt.Errorf("read orders: %w", err)
	}
	if number < 1 {
		found = append(found, problem{1, `expected game "CODE" turn NUMBER`})
	}
	if number < 2 {
		found = append(found, problem{2, `expected id player "EMAIL" or id faction NUMBER`})
	}
	if len(found) != 0 {
		return Submission{}, found
	}
	return submission, nil
}

func parseGameLine(line *Line, submission *Submission) error {
	badHeader := fmt.Errorf(`expected game "CODE" turn NUMBER`)
	if err := line.expect("game"); err != nil {
		return badHeader
	}
	code, err := line.quoted("game code")
	if err != nil {
		return badHeader
	}
	if err := line.expect("turn"); err != nil {
		return badHeader
	}
	turn, err := line.number("turn")
	if err != nil {
		return err
	}
	if turn < 0 {
		return fmt.Errorf("turn must be nonnegative")
	}
	if err := line.end(); err != nil {
		return err
	}
	submission.GameCode, submission.Turn = code, turn
	return nil
}

func parseIdentityLine(line *Line, submission *Submission) error {
	badHeader := fmt.Errorf(`expected id player "EMAIL" or id faction NUMBER`)
	if err := line.expect("id"); err != nil {
		return badHeader
	}
	kind, ok := line.keyword("player", "faction")
	if !ok {
		return badHeader
	}
	if kind == "player" {
		email, err := line.quoted("email address")
		if err != nil {
			return badHeader
		}
		if err := line.end(); err != nil {
			return err
		}
		submission.Identity.PlayerEmail = email
		return nil
	}
	id, err := line.entityID("faction")
	if err != nil {
		return err
	}
	if err := line.end(); err != nil {
		return err
	}
	submission.Identity.FactionID = id
	return nil
}

// parseOrder dispatches on the verb, so a line is only ever measured against
// the forms of the order it names.
func parseOrder(line *Line) (Order, error) {
	verb, ok := line.next()
	if !ok || verb.quoted {
		return Order{}, fmt.Errorf("expected an order; %s", verbList())
	}
	spec, ok := Lookup(verb.text)
	if !ok {
		return Order{}, fmt.Errorf("unknown order %q; expected %s", verb.text, verbList())
	}
	params, err := spec.Parse(line)
	if err != nil {
		// A field that was read but wrong says so itself. A form that never
		// matched reports this verb's syntax instead.
		if isSyntaxError(err) {
			return Order{}, spec.syntaxError()
		}
		return Order{}, err
	}
	if err := line.end(); err != nil {
		return Order{}, spec.syntaxError()
	}
	return Order{Line: line.Number, Verb: spec.Verb, Params: params}, nil
}
