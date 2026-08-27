// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"errors"
	"fmt"
	"strings"
)

// The cursor a recursive descent runs on. Every production of the grammar --
// the file, an order, a field -- consumes from a Parser; `Spec.Parse` is the
// production for one verb, so the thirty-seven orders are the thirty-seven
// branches of the descent, each written where its order lives.

// A production reports in one of two ways, and which one decides what the
// player is shown.
//
// errShape says the line never matched the shape of its order. It carries no
// message, because a shape failure is only worth reporting once the descent
// has given up: an alternative that failed early is not what the player meant.
// The message is built at the top from the furthest point reached (see
// Parser.furthest), and the marker is all that travels up.
//
// A diagnostic says a field was read and found wrong. It already names the
// token and what is wrong with it, so it survives to the player as written.
var errShape = errors.New("the line matched no form of its order")

func isShapeError(err error) bool { return errors.Is(err, errShape) }

// expectation is one thing that could have stood at a given point.
type expectation struct {
	// what is how it reads in a message: "`orbit`", "a system letter".
	what string
	// keyword is the literal word, when the expectation is one, so a near miss
	// can be suggested. Empty for a description rather than a word.
	keyword string
}

// Parser reads one order's tokens left to right.
//
// It is the order rather than the line, which is what the type is named for: a
// CREATE runs to `end`, and the tokens of every line it spans are here, each
// still carrying the line it came from.
type Parser struct {
	src    *source
	tokens []token
	pos    int
	// begins is the physical line the order opened on, where it is reported
	// and stored.
	begins int
	// noun is what this Parser is reading -- an order, or a header line -- for
	// the message about running off the end of it.
	noun string
	// furthest is the highest pos any production reached before failing, and
	// wanted is everything that could have stood there. Together they are the
	// message for a line that matched no form of its order.
	//
	// The furthest point is the right one because a grammar with alternatives
	// fails several times over: `move to planet 6` fails at `system` and again
	// at `orbit`, both at the same word, and both are worth saying. What is
	// not worth saying is that the line also failed to be a JUMP.
	furthest int
	wanted   []expectation
}

func newParser(src *source, begins int, tokens []token, noun string) *Parser {
	return &Parser{src: src, tokens: tokens, begins: begins, noun: noun}
}

// absorb adds another physical line's tokens to the order, for the orders that
// run to a terminator. Every token keeps the line it came from, so the order
// gains lines without losing where anything in it is.
func (p *Parser) absorb(tokens []token) { p.tokens = append(p.tokens, tokens...) }

// clone is a second cursor over the same order, for a lookahead that has to
// leave no trace.
//
// It is used rather than saving and restoring a position because a lookahead
// records expectations as it goes, and those belong to the reading that was
// thrown away. Restoring the cursor would leave them behind to be reported
// against the reading that was kept.
func (p *Parser) clone() *Parser {
	return &Parser{src: p.src, tokens: p.tokens, pos: p.pos, begins: p.begins, noun: p.noun}
}

// want records that something could have stood at the cursor. Only the
// furthest point keeps its expectations: getting further into a line means the
// earlier reading was right.
func (p *Parser) want(what string) { p.record(expectation{what: what}) }

// wantKeyword records a literal word, keeping the word itself for a suggestion.
func (p *Parser) wantKeyword(word string) {
	p.record(expectation{what: "`" + word + "`", keyword: word})
}

func (p *Parser) record(item expectation) {
	if p.pos > p.furthest {
		p.furthest, p.wanted = p.pos, nil
	}
	if p.pos < p.furthest {
		return
	}
	for _, existing := range p.wanted {
		if existing.what == item.what {
			return
		}
	}
	p.wanted = append(p.wanted, item)
}

func (p *Parser) more() bool { return p.pos < len(p.tokens) }

func (p *Parser) next() (token, bool) {
	if !p.more() {
		return token{}, false
	}
	p.pos++
	return p.tokens[p.pos-1], true
}

func (p *Parser) peek() (token, bool) { return p.peekAt(0) }

// peekAt looks ahead without consuming. A quantity needs two tokens of
// lookahead, because whether a comma continues the number or separates the
// next item is decided by what follows it.
func (p *Parser) peekAt(offset int) (token, bool) {
	if p.pos+offset >= len(p.tokens) {
		return token{}, false
	}
	return p.tokens[p.pos+offset], true
}

// holds reports whether an unquoted keyword appears anywhere in the order. It
// is how the file scanner knows a multi-line order has reached its terminator
// without parsing the order to find out.
func (p *Parser) holds(word string) bool {
	for _, item := range p.tokens {
		if !item.quoted && strings.EqualFold(item.text, word) {
			return true
		}
	}
	return false
}

// empty reports whether the line held nothing but whitespace and comments.
func (p *Parser) empty() bool { return len(p.tokens) == 0 }

// keyword consumes the next token when it matches one of the given words,
// ignoring case, and reports which one. Keywords are always unquoted.
//
// Every word offered is recorded as an expectation whether or not one matches,
// which is what makes a run of alternatives report as a set: three calls that
// each fail at the same word produce one message naming all three.
func (p *Parser) keyword(words ...string) (string, bool) {
	for _, word := range words {
		p.wantKeyword(word)
	}
	current, ok := p.peek()
	if !ok || current.quoted {
		return "", false
	}
	for _, word := range words {
		if strings.EqualFold(current.text, word) {
			p.pos++
			return word, true
		}
	}
	return "", false
}

// expect consumes a required keyword.
func (p *Parser) expect(word string) error {
	if _, ok := p.keyword(word); !ok {
		return errShape
	}
	return nil
}

// end requires that nothing is left over, so a trailing word is a mistake
// rather than something quietly ignored. It records an expectation of its own,
// which is what puts the caret under the trailing word rather than back where
// the last field was read.
func (p *Parser) end() error {
	if p.more() {
		p.want("the end of the " + p.noun)
		return errShape
	}
	return nil
}

// where is the position and width of the token at an index, and what to call
// it in a message. An index past the last token is the point just after it,
// which is where a message about something missing belongs.
func (p *Parser) where(index int) (pos Position, width int, found string) {
	if index < len(p.tokens) {
		current := p.tokens[index]
		return current.pos, current.width, current.describe()
	}
	if len(p.tokens) != 0 {
		return p.tokens[len(p.tokens)-1].end(), 1, "the end of the " + p.noun
	}
	return Position{Line: p.begins, Column: 1}, 1, "the end of the " + p.noun
}

// at builds a diagnostic against the token at an index.
func (p *Parser) at(index int, format string, args ...any) *diagnostic {
	pos, width, _ := p.where(index)
	return &diagnostic{pos: pos, width: width,
		text: fmt.Sprintf(format, args...), source: p.src.line(pos.Line)}
}

// fail reports a field that was read and found wrong, against the token it was
// read from. This is the error kind that survives to the player as written: it
// already says what is wrong, and the order's forms would not add to it.
func (p *Parser) fail(at token, format string, args ...any) *diagnostic {
	return &diagnostic{pos: at.pos, width: at.width,
		text: fmt.Sprintf(format, args...), source: p.src.line(at.pos.Line)}
}

// here builds a diagnostic against the cursor, for something that is not
// there to be pointed at.
func (p *Parser) here(format string, args ...any) *diagnostic {
	return p.at(p.pos, format, args...)
}

// expectedError is the message for a descent that stopped: what could have
// stood at the furthest point it reached, and what stood there instead.
//
// The furthest point is the right one because a grammar with alternatives
// fails several times over, and the failure that got closest to the end of the
// line is the reading the player meant.
func (p *Parser) expectedError() *diagnostic {
	pos, width, found := p.where(p.furthest)
	report := &diagnostic{pos: pos, width: width, source: p.src.line(pos.Line)}
	if len(p.wanted) == 0 {
		// Nothing was ever wanted, which happens only when a production takes
		// nothing after it and was given something.
		report.text = fmt.Sprintf("expected the end of the %s, found %s", p.noun, found)
		return report
	}
	words := make([]string, len(p.wanted))
	for i, item := range p.wanted {
		words[i] = item.what
	}
	report.text = fmt.Sprintf("expected %s, found %s", wordList(words), found)
	if word, near := p.suggestion(p.furthest); near {
		report.note("did you mean `%s`?", word)
	}
	return report
}

// place is the guard on the two error kinds an order parser may return.
//
// A parser is meant to return either errShape or a diagnostic that points at
// the token it read. Nothing stops it returning a plain error instead -- that
// compiles, and it behaves correctly in every way except the one that matters,
// which is that the player is told where to look. Rather than trusting
// thirty-seven parsers not to, every error that is neither of the two kinds is
// given the line the order began on here.
//
// The order's line is the coarsest placement the parser has and the right one
// for what tends to arrive this way: `a raid seeks one unit or two, and this
// one names 3` is about a span rather than a token. A parser with a token in
// hand should still use fail, and the four that had one now do.
func (p *Parser) place(err error) error {
	if err == nil || isShapeError(err) {
		return err
	}
	if _, placed := errors.AsType[*diagnostic](err); placed {
		return err
	}
	return onLine(p.begins, "%s", err)
}

// shapeError is expectedError with the verb's forms after it, for a line that
// matched no form of the order it named.
//
// The forms follow the message rather than standing in for it. The parser
// before this one printed them alone -- `expected ship SHIP-ID move to orbit
// ORBIT, or ship SHIP-ID move to system SYSTEM orbit ORBIT` -- which told a
// player everything except which word of theirs was the problem.
func (p *Parser) shapeError(spec *Spec) error {
	return p.expectedError().forms(spec)
}

// suggestion is the expected keyword that the word at an index was nearly.
func (p *Parser) suggestion(index int) (string, bool) {
	if index >= len(p.tokens) || p.tokens[index].quoted {
		return "", false
	}
	var keywords []string
	for _, item := range p.wanted {
		if item.keyword != "" {
			keywords = append(keywords, item.keyword)
		}
	}
	if len(keywords) == 0 {
		return "", false
	}
	return suggest(p.tokens[index].text, keywords)
}
