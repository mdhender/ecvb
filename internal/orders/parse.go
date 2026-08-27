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
	file := &lines{scanner: scanner}
	for {
		line, ok := file.next()
		if !ok {
			break
		}
		// A line that could not be tokenized names no order and never will, so
		// it is reported here rather than handed on to be misread. It is
		// checked before the header lines as well, because a game code is
		// quoted text too.
		if line.fault != nil {
			found = append(found, problem{line.Number, line.fault.Error()})
			continue
		}
		switch line.Number {
		case 1:
			if err := parseGameLine(line, &submission); err != nil {
				found = append(found, problem{line.Number, err.Error()})
			}
		case 2:
			if err := parseIdentityLine(line, &submission); err != nil {
				found = append(found, problem{line.Number, err.Error()})
			}
		default:
			if line.empty() {
				continue
			}
			// A create runs to `end`, so the physical line the scanner handed
			// over is not always the whole order. This is the one place in the
			// parser that knows an order may span lines, and it settles it
			// before the order is read, so every Parse still consumes from one
			// Line whatever the player's line breaks were.
			if spec, form, ok := opensOrder(line); ok && spec.Terminator != nil {
				if until := spec.Terminator(form); until != "" {
					if err := gather(file, line, spec, until); err != nil {
						found = append(found, problem{line.Number, err.Error()})
						continue
					}
				}
			}
			order, err := parseOrder(line)
			if err != nil {
				found = append(found, problem{line.Number, err.Error()})
				continue
			}
			submission.Orders = append(submission.Orders, order)
		}
	}
	if err := scanner.Err(); err != nil {
		return Submission{}, fmt.Errorf("read orders: %w", err)
	}
	if file.number < 1 {
		found = append(found, problem{1, `expected game "CODE" turn NUMBER`})
	}
	if file.number < 2 {
		found = append(found, problem{2, `expected id player "EMAIL" or id faction NUMBER`})
	}
	if len(found) != 0 {
		return Submission{}, found
	}
	return submission, nil
}

// lines reads an order file a physical line at a time and lets one line be put
// back.
//
// The pushback is what a multi-line order costs: gathering one has to read
// ahead to find out it has run past the end, and the line it stopped at is the
// next order rather than something to throw away.
type lines struct {
	scanner *bufio.Scanner
	number  int
	pending *Line
}

func (f *lines) next() (*Line, bool) {
	if f.pending != nil {
		line := f.pending
		f.pending = nil
		return line, true
	}
	if !f.scanner.Scan() {
		return nil, false
	}
	f.number++
	return newLine(f.number, f.scanner.Text()), true
}

func (f *lines) push(line *Line) { f.pending = line }

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

// opensOrder is the order a line names and the word that follows its verb, read
// without consuming anything.
//
// It exists for one question the file scanner has to answer before the order is
// parsed: does this order run to a terminator? The word after the verb goes
// with it because the answer can depend on the form, and a form is named there.
//
// The subject is read with parseSubject, which is what parseOrder reads it
// with, rather than by counting the tokens a subject takes. Counting
// worked -- `we` takes one word and every other subject two, so the verb was
// the second token or the third -- but it was a second copy of the subject
// grammar living where nothing would ever compare it against the first, and it
// would have gone on agreeing with it only for as long as no subject was ever
// written any other way.
//
// Nothing is consumed: the mark is taken before a word is read and restored
// however this returns, because Parse hands the same Line to parseOrder
// afterwards and that line has to still be at its beginning.
func opensOrder(line *Line) (spec *Spec, form string, ok bool) {
	restore := line.mark()
	defer restore()
	// The subject is however many words parseSubject reads, and the verb is
	// what comes after them. Whether the player wrote a good subject is not
	// asked here, and that is the point: a create is still a create when its id
	// is mistyped or its subject is missing altogether, and it still has to be
	// read to its `end`. Refusing it here would leave the player told about the
	// one thing they got wrong and about all four lines of the order's body as
	// well. parseSubject consumes nothing when it fails, so a line that named
	// no subject simply offers its first word as the verb.
	_, _ = parseSubject(line)
	word, found := line.next()
	if !found || word.quoted {
		return nil, "", false
	}
	spec, ok = Lookup(word.text)
	if !ok {
		return nil, "", false
	}
	if next, found := line.peek(); found && !next.quoted {
		form = next.text
	}
	return spec, form, true
}

// gather reads on until the order's terminator, appending each physical line to
// the one the order began on. The terminator itself is left in the line: it is
// part of the order's grammar, so the order's own Parse consumes it.
//
// A missing terminator stops at the next line that opens an order rather than
// running to the end of the file. Reading on would swallow every order after
// it, so a player would fix the one mistake the file reported and find three
// more waiting -- and a file is meant to be reported whole.
func gather(file *lines, line *Line, spec *Spec, until string) error {
	for !line.holds(until) {
		next, ok := file.next()
		if !ok {
			return fmt.Errorf("%s runs until `%s`, and the file ended first",
				strings.ToUpper(spec.Verb), until)
		}
		if next.begins() {
			file.push(next)
			return fmt.Errorf("%s runs until `%s`, and line %d begins another order",
				strings.ToUpper(spec.Verb), until, next.Number)
		}
		line.absorb(next)
		// A line that could not be tokenized cannot be searched for the
		// terminator, so gathering stops rather than reading to the end of the
		// file looking for one that the broken line may have been carrying.
		if line.fault != nil {
			return line.fault
		}
	}
	return nil
}

// parseOrder reads the subject, then dispatches on the verb, so a line is only
// ever measured against the forms of the order it names -- and only ever
// against the forms its subject may be given.
func parseOrder(line *Line) (Order, error) {
	subject, err := parseSubject(line)
	if err != nil {
		return Order{}, err
	}
	verb, ok := line.next()
	if !ok || verb.quoted {
		return Order{}, fmt.Errorf("expected an order after %s; %s", subjectNoun(subject.Kind), verbList())
	}
	spec, ok := Lookup(verb.text)
	if !ok {
		return Order{}, fmt.Errorf("unknown order %q; expected %s", verb.text, verbList())
	}
	if !spec.accepts(subject.Kind) {
		return Order{}, spec.subjectError(subject.Kind)
	}
	params, err := spec.Parse(subject, line)
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

// parseSubject reads who the order is being given to. Every order opens with
// it, so the parser knows the actor before it knows the verb, and an order
// never reads its own actor.
func parseSubject(line *Line) (Subject, error) {
	kind, ok := line.keyword(SubjectShip, SubjectColony, SubjectFaction)
	if !ok {
		return Subject{}, fmt.Errorf("expected an order to begin with %s, %s, or %s",
			SubjectShip, SubjectColony, SubjectFaction)
	}
	if kind == SubjectFaction {
		return Subject{Kind: kind}, nil
	}
	id, err := line.entityID(kind)
	if err != nil {
		return Subject{}, err
	}
	return Subject{Kind: kind, ID: id}, nil
}
