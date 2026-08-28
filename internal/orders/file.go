// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package orders parses and validates player order files.
package orders

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// The top of the descent: a file is two header lines and then an order on each
// line that has one.
//
// Parse reports every problem it finds rather than stopping at the first, so a
// player fixing a file sees the whole list -- and when it gives up on an order
// it skips to the next line that opens one, so a single mistake inside a
// multi-line CREATE is one complaint rather than one for every line of the
// order's body.

// StelliumOrbit is the orbit a MOVE order names to send a ship back to the
// stellium orbit. No planet occupies it: it is a fiction that gives MOVE a way
// to say "leave the planets", and it is never a probe target.
const StelliumOrbit = 11

// The shapes of the two header lines, shown to a player whose file does not
// open with them.
const (
	gameHeaderForm = `game "CODE" turn NUMBER`
	idHeaderForm   = `id player "EMAIL", or id faction NUMBER`
)

// Identity identifies the faction submitting an order file. A file that names
// a faction names it by the number the game knows it as -- the number every
// report prints -- and never by a row id.
type Identity struct {
	PlayerEmail   string
	FactionNumber int64
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

// physical is one line of the file as the lexer left it.
//
// The whole file is lexed before anything is parsed, which is what lets a
// multi-line order be gathered by looking forward in a slice rather than by
// reading ahead and pushing back what it did not want. It also means the text
// of every line is on hand when a message needs to show one.
type physical struct {
	number int
	tokens []token
	fault  error // the unterminated quote, when there was one
}

// Parse parses an order file without consulting the database.
//
// The first two physical lines are the header: the game and turn, then the
// submitting player or faction. Everything after them is an order. Blank lines
// are allowed, and a `#` outside quotes begins a comment.
func Parse(r io.Reader) (Submission, error) {
	src := &source{}
	var file []physical
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		text := scanner.Text()
		src.add(text)
		number := len(src.lines)
		tokens, fault := lex(number, text)
		file = append(file, physical{number: number, tokens: tokens, fault: fault})
	}
	if err := scanner.Err(); err != nil {
		return Submission{}, fmt.Errorf("read orders: %w", err)
	}

	var submission Submission
	var found problems
	for i := 0; i < len(file); {
		current := file[i]
		// A line that could not be lexed names no order and never will, so it
		// is reported here rather than handed on to be misread. It is checked
		// before the header lines as well, because a game code is quoted text
		// too.
		if current.fault != nil {
			found = append(found, current.fault)
			i++
			continue
		}
		switch current.number {
		case 1:
			if err := readHeader(src, current, gameLine, gameHeaderForm, &submission); err != nil {
				found = append(found, err)
			}
			i++
		case 2:
			if err := readHeader(src, current, identityLine, idHeaderForm, &submission); err != nil {
				found = append(found, err)
			}
			i++
		default:
			if len(current.tokens) == 0 {
				i++
				continue
			}
			order, resume, err := readOrder(src, file, i)
			i = resume
			if err != nil {
				found = append(found, err)
				continue
			}
			submission.Orders = append(submission.Orders, order)
		}
	}
	if len(file) < 1 {
		found = append(found, &diagnostic{pos: Position{Line: 1, Column: 1},
			text: "expected " + gameHeaderForm})
	}
	if len(file) < 2 {
		found = append(found, &diagnostic{pos: Position{Line: 2, Column: 1},
			text: "expected " + idHeaderForm})
	}
	if len(found) != 0 {
		return Submission{}, found.inFileOrder()
	}
	return submission, nil
}

// readHeader runs one of the two header productions and turns a shape failure
// into a message that shows the line's form. A header has one shape, so
// showing it is always the right answer.
func readHeader(src *source, line physical, read func(*Parser, *Submission) error,
	form string, submission *Submission) error {
	p := newParser(src, line.number, line.tokens, "line")
	if err := read(p, submission); err != nil {
		if isShapeError(err) {
			return p.expectedError().note("the header line is written: %s", form)
		}
		return err
	}
	return nil
}

// gameLine reads `game "CODE" turn NUMBER`.
func gameLine(p *Parser, submission *Submission) error {
	if err := p.expect("game"); err != nil {
		return err
	}
	code, err := p.quoted("game code")
	if err != nil {
		return err
	}
	if err := p.expect("turn"); err != nil {
		return err
	}
	turn, err := p.number("turn")
	if err != nil {
		return err
	}
	if turn < 0 {
		return p.at(p.pos-1, "invalid turn %d; a turn is nonnegative", turn)
	}
	if err := p.end(); err != nil {
		return err
	}
	submission.GameCode, submission.Turn = code, turn
	return nil
}

// identityLine reads `id player "EMAIL"` or `id faction NUMBER`.
func identityLine(p *Parser, submission *Submission) error {
	if err := p.expect("id"); err != nil {
		return err
	}
	kind, ok := p.keyword("player", "faction")
	if !ok {
		return errShape
	}
	if kind == "player" {
		email, err := p.quoted("email address")
		if err != nil {
			return err
		}
		if err := p.end(); err != nil {
			return err
		}
		submission.Identity.PlayerEmail = email
		return nil
	}
	id, err := p.factionID()
	if err != nil {
		return err
	}
	if err := p.end(); err != nil {
		return err
	}
	submission.Identity.FactionNumber = id
	return nil
}

// readOrder gathers one order's physical lines and parses it, reporting where
// the file should be picked up again.
func readOrder(src *source, file []physical, start int) (Order, int, error) {
	p, resume, err := gather(src, file, start)
	if err != nil {
		return Order{}, resume, err
	}
	order, err := parseOrder(p)
	return order, resume, err
}

// gather collects the physical lines of one order into a single Parser, and
// says which line the file continues on.
//
// Most orders are one line and gather does nothing. A CREATE runs to `end`, so
// the lines after it are read until the terminator turns up. The terminator is
// left in the order: it is part of the order's grammar, so the order's own
// Parse consumes it.
//
// A missing terminator stops at the next line that opens an order rather than
// running to the end of the file. Reading on would swallow every order after
// it, so a player would fix the one mistake the file reported and find three
// more waiting -- and a file is meant to be reported whole.
func gather(src *source, file []physical, start int) (*Parser, int, error) {
	p := newParser(src, file[start].number, file[start].tokens, "order")
	spec, form, ok := p.opens()
	if !ok || spec.Terminator == nil {
		return p, start + 1, nil
	}
	until := spec.Terminator(form)
	if until == "" {
		return p, start + 1, nil
	}
	at := start
	for !p.holds(until) {
		at++
		if at >= len(file) {
			return nil, len(file), p.at(0, "%s runs until `%s`, and the file ended first",
				strings.ToUpper(spec.Verb), until)
		}
		if begins(file[at].tokens) {
			return nil, at, p.at(0, "%s runs until `%s`, and line %d begins another order",
				strings.ToUpper(spec.Verb), until, file[at].number)
		}
		// A line that could not be lexed cannot be searched for the
		// terminator, so gathering stops rather than reading to the end of the
		// file looking for one the broken line may have been carrying.
		if file[at].fault != nil {
			return nil, resumeAt(file, at+1), file[at].fault
		}
		p.absorb(file[at].tokens)
	}
	return p, at + 1, nil
}

// resumeAt is the next line that opens an order, which is where the file is
// picked up after a multi-line order was given up on.
//
// Reading on from where the gather stopped would report every remaining line
// of the order's body as an order of its own: four complaints about one
// mistake, three of them about lines the player wrote correctly.
func resumeAt(file []physical, from int) int {
	for i := from; i < len(file); i++ {
		if begins(file[i].tokens) {
			return i
		}
	}
	return len(file)
}

// begins reports whether a line opens an order. Every order names its subject
// first, so the first word is the whole of the test.
//
// It is what tells a gather that it has run past the end of the order it was
// reading: a continuation of a CREATE is a clause or a lot of units, and never
// a subject.
func begins(tokens []token) bool {
	if len(tokens) == 0 || tokens[0].quoted {
		return false
	}
	for _, subject := range []string{SubjectShip, SubjectColony, SubjectFaction} {
		if strings.EqualFold(tokens[0].text, subject) {
			return true
		}
	}
	return false
}

// opens is the order a line names and the word that follows its verb, read
// without consuming anything.
//
// It exists for one question gather has to answer before the order is parsed:
// does this order run to a terminator? The word after the verb goes with it
// because the answer can depend on the form, and a form is named there.
//
// The subject is read with the same production the parse uses, rather than by
// counting the words a subject takes. Counting worked -- `we` takes one word
// and every other subject two -- but it was a second copy of the subject
// grammar living where nothing would ever compare it against the first.
//
// Whether the player wrote a good subject is not asked, and that is the point:
// a CREATE is still a CREATE when its id is mistyped or its subject is missing
// altogether, and it still has to be read to its `end`. Refusing it here would
// leave the player told about the one thing they got wrong and about all four
// lines of the order's body as well.
func (p *Parser) opens() (spec *Spec, form string, ok bool) {
	ahead := p.clone()
	_, _ = ahead.subject()
	word, found := ahead.next()
	if !found || word.quoted {
		return nil, "", false
	}
	if spec, ok = Lookup(word.text); !ok {
		return nil, "", false
	}
	if next, found := ahead.peek(); found && !next.quoted {
		form = next.text
	}
	return spec, form, true
}

// parseOrder reads the subject, then dispatches on the verb, so a line is only
// ever measured against the forms of the order it names -- and only ever
// against the forms its subject may be given.
func parseOrder(p *Parser) (Order, error) {
	opening, _ := p.peek()
	subject, err := p.subject()
	if err != nil {
		return Order{}, err
	}
	verb, ok := p.peek()
	if !ok || verb.quoted {
		_, _, found := p.where(p.pos)
		return Order{}, p.here("expected an order after %s, found %s",
			subjectNoun(subject.Kind), found).note("the orders are: %s", verbList())
	}
	p.pos++
	spec, ok := Lookup(verb.text)
	if !ok {
		report := p.fail(verb, "unknown order %q", elide(verb.text, longestFound))
		if word, near := suggest(verb.text, verbNames()); near {
			report.note("did you mean `%s`?", word)
		}
		return Order{}, report.note("the orders are: %s", verbList())
	}
	if !spec.accepts(subject.Kind) {
		return Order{}, p.fail(opening, "%s", spec.subjectError(subject.Kind)).forms(spec)
	}
	params, err := spec.Parse(subject, p)
	if err != nil {
		// A field that was read and found wrong says so itself. A line that
		// matched no form of the order is told what was expected where, and
		// shown the forms.
		if isShapeError(err) {
			return Order{}, p.shapeError(spec)
		}
		return Order{}, p.place(err)
	}
	if err := p.end(); err != nil {
		return Order{}, p.shapeError(spec)
	}
	return Order{Line: p.begins, Verb: spec.Verb, Params: params}, nil
}

// subject reads who the order is being given to. Every order opens with it, so
// the parser knows the actor before it knows the verb, and an order never
// reads its own actor.
func (p *Parser) subject() (Subject, error) {
	kind, ok := p.keyword(SubjectShip, SubjectColony, SubjectFaction)
	if !ok {
		_, _, found := p.where(p.pos)
		report := p.here("expected an order to begin with %s, %s, or %s; found %s",
			SubjectShip, SubjectColony, SubjectFaction, found)
		if word, near := p.suggestion(p.pos); near {
			report.note("did you mean `%s`?", word)
		}
		return Subject{}, report
	}
	if kind == SubjectFaction {
		return Subject{Kind: kind}, nil
	}
	id, err := p.entityID(kind)
	if err != nil {
		if isShapeError(err) {
			return Subject{}, p.expectedError()
		}
		return Subject{}, err
	}
	return Subject{Kind: kind, ID: id}, nil
}
