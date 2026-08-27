// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"fmt"
	"strings"
)

// The lexer, and the only thing in the parser that touches raw text.
//
// One pass over a physical line turns it into tokens, and every token
// remembers where it came from. That is what lets a message point at the word
// that was wrong rather than at the order that held it -- which matters most
// for an order running over several lines, where the order begins on one line
// and the mistake is three lines further down.

// Position is where a token begins, in the coordinates a player edits in: the
// physical line of the file and the column within it, both counted from one.
//
// Columns count runes rather than bytes, so a caret drawn under a name with an
// accent in it lands where the eye expects.
type Position struct {
	Line   int
	Column int
}

// String places a problem for a player. A column of zero is a problem with a
// whole order rather than with a word of it -- what Bind reports -- and says
// only the line.
func (p Position) String() string {
	if p.Column == 0 {
		return fmt.Sprintf("line %d", p.Line)
	}
	return fmt.Sprintf("line %d, column %d", p.Line, p.Column)
}

// token is one word of an order. Nothing here is packed or interned: a token
// keeps its own position and width because every one of them is wanted the
// moment something goes wrong, which is the case this parser is written for.
type token struct {
	// text is the token's value: for quoted text, what stood between the
	// quotes.
	text string
	// quoted marks text written inside quotes. It is tracked because a game
	// code or an email address may hold characters that would otherwise split
	// a token, because a `#` inside quotes is not a comment, and because a
	// keyword is never quoted -- `ship 18 name "move"` names a ship.
	quoted bool
	pos    Position
	// width is the columns the token covers in the source, quotes included, so
	// a caret can be drawn the length of what went wrong.
	width int
}

// describe names a token the way a message reports what it found. Quoted text
// is called that, because `expected a unit code, found "GOLD"` would otherwise
// read as though GOLD had been rejected on its merits.
func (t token) describe() string {
	text := elide(t.text, longestFound)
	if t.quoted {
		return fmt.Sprintf("the quoted text %q", text)
	}
	return fmt.Sprintf("%q", text)
}

// end is the position just past the token, where a message about something
// missing points.
func (t token) end() Position {
	return Position{Line: t.pos.Line, Column: t.pos.Column + t.width}
}

// punctuation characters stand alone as tokens, so that a jump may be written
// `to (6,-9,8)` or `to ( 6 , -9 , 8 )` and read the same either way.
const punctuation = "(),"

// breaks are the characters that end an unquoted token.
const breaks = " \t\r\"#" + punctuation

// lex splits one physical line into tokens, dropping any trailing comment. A
// `#` outside quotes begins a comment that runs to the end of the line.
//
// A quote that is never closed is the one thing a line can get wrong before
// any order is read from it, and it is refused here rather than passed on.
// Reading to the end of the line and calling it a token is worse than it
// sounds: a name is quoted text, so `ship 18 name "Jalopy` would name the ship
// and say nothing, and the player would find out from a report a turn later.
func lex(number int, text string) ([]token, error) {
	var tokens []token
	runes := []rune(text)
	for i := 0; i < len(runes); {
		at := Position{Line: number, Column: i + 1}
		switch c := runes[i]; {
		case c == '#':
			return tokens, nil
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '"':
			i++ // the opening quote
			start := i
			for i < len(runes) && runes[i] != '"' {
				i++
			}
			if i == len(runes) {
				return nil, &diagnostic{
					pos:   at,
					width: len(runes) - start + 1,
					text: fmt.Sprintf("unterminated quoted text %q; a quote is closed on the line it opens",
						elide(string(runes[start:]), longestFound)),
					source: text,
				}
			}
			tokens = append(tokens, token{
				text: string(runes[start:i]), quoted: true,
				pos: at, width: i - start + 2, // both quotes
			})
			i++ // the closing quote
		case strings.ContainsRune(punctuation, c):
			tokens = append(tokens, token{text: string(c), pos: at, width: 1})
			i++
		default:
			start := i
			for i < len(runes) && !strings.ContainsRune(breaks, runes[i]) {
				i++
			}
			tokens = append(tokens, token{text: string(runes[start:i]), pos: at, width: i - start})
		}
	}
	return tokens, nil
}

// source is the physical text of the file, kept so that a message can show the
// line it is about.
//
// Holding every line in memory is the plainest thing that works, and it is a
// deliberate trade: a parser that valued space over clarity would re-read the
// file to render an error, and would then have to be given a file it could
// re-read rather than the io.Reader it has. An order file is a page long.
type source struct{ lines []string }

func (s *source) add(text string) { s.lines = append(s.lines, text) }

// line is the text of one physical line, or empty for a line number that is
// not in the file. A diagnostic with no source line still renders; it simply
// shows no caret.
func (s *source) line(number int) string {
	if number < 1 || number > len(s.lines) {
		return ""
	}
	return s.lines[number-1]
}
