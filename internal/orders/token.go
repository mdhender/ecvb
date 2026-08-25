// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// syntaxErr marks a line that never matched the shape of its order, as
// opposed to a field that was read and found wrong. The first is answered with
// the verb's syntax, which is more use than naming the token that failed; the
// second already says what is wrong and survives unchanged.
type syntaxErr struct{ message string }

func (e syntaxErr) Error() string { return e.message }

func badSyntax(format string, args ...any) error {
	return syntaxErr{message: fmt.Sprintf(format, args...)}
}

func isSyntaxError(err error) bool {
	var target syntaxErr
	return errors.As(err, &target)
}

// wholeNumber converts a token, telling a value that is out of range from one
// that was never a number at all.
func wholeNumber(text, what string) (int, error) {
	value, err := strconv.Atoi(text)
	if err == nil {
		return value, nil
	}
	if errors.Is(err, strconv.ErrRange) {
		return 0, fmt.Errorf("%s is too large", what)
	}
	return 0, fmt.Errorf("invalid %s %q", what, text)
}

// token is one word of an order line. Quoted tokens are tracked separately
// because a game code or an email address may hold characters that would
// otherwise split a token, and because a `#` inside quotes is not a comment.
type token struct {
	text   string
	quoted bool
}

// punctuation characters that stand alone as tokens, so that a jump may be
// written `to (6,-9,8)` or `to ( 6 , -9 , 8 )` and read the same either way.
const punctuation = "(),"

// tokenize splits one line into tokens, dropping any trailing comment. A `#`
// outside quotes begins a comment that runs to the end of the line.
func tokenize(text string) []token {
	var tokens []token
	runes := []rune(text)
	for i := 0; i < len(runes); {
		switch c := runes[i]; {
		case c == '#':
			return tokens
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '"':
			i++
			start := i
			for i < len(runes) && runes[i] != '"' {
				i++
			}
			tokens = append(tokens, token{text: string(runes[start:i]), quoted: true})
			if i < len(runes) {
				i++ // the closing quote
			}
		case strings.ContainsRune(punctuation, c):
			tokens = append(tokens, token{text: string(c)})
			i++
		default:
			start := i
			for i < len(runes) && !strings.ContainsRune(" \t\r\"#"+punctuation, runes[i]) {
				i++
			}
			tokens = append(tokens, token{text: string(runes[start:i])})
		}
	}
	return tokens
}

// Line is one tokenized order line, read left to right. Every order's parser
// consumes from it, so the field parsers below are written once rather than
// once per order.
type Line struct {
	Number int
	tokens []token
	pos    int
}

func newLine(number int, text string) *Line {
	return &Line{Number: number, tokens: tokenize(text)}
}

// empty reports whether the line holds nothing but whitespace and comments.
func (l *Line) empty() bool { return len(l.tokens) == 0 }

func (l *Line) more() bool { return l.pos < len(l.tokens) }

func (l *Line) next() (token, bool) {
	if !l.more() {
		return token{}, false
	}
	l.pos++
	return l.tokens[l.pos-1], true
}

func (l *Line) peek() (token, bool) {
	if !l.more() {
		return token{}, false
	}
	return l.tokens[l.pos], true
}

// keyword consumes the next token when it matches one of the given words,
// ignoring case, and reports which one. Keywords are always unquoted.
func (l *Line) keyword(words ...string) (string, bool) {
	current, ok := l.peek()
	if !ok || current.quoted {
		return "", false
	}
	for _, word := range words {
		if strings.EqualFold(current.text, word) {
			l.pos++
			return word, true
		}
	}
	return "", false
}

// expect consumes a required keyword.
func (l *Line) expect(word string) error {
	if _, ok := l.keyword(word); !ok {
		return badSyntax("expected %s", word)
	}
	return nil
}

// end reports an error when anything is left over, so a trailing word is a
// mistake rather than something quietly ignored.
func (l *Line) end() error {
	if l.more() {
		return badSyntax("unexpected %q", l.rest())
	}
	return nil
}

func (l *Line) rest() string {
	words := make([]string, 0, len(l.tokens)-l.pos)
	for _, item := range l.tokens[l.pos:] {
		words = append(words, item.text)
	}
	return strings.Join(words, " ")
}

// quoted consumes a quoted string, such as a game code or an email address.
func (l *Line) quoted(what string) (string, error) {
	current, ok := l.next()
	if !ok || !current.quoted {
		return "", badSyntax("expected a quoted %s", what)
	}
	return current.text, nil
}

// entityID consumes a positive entity id.
func (l *Line) entityID(kind string) (int64, error) {
	current, ok := l.next()
	if !ok || current.quoted {
		return 0, badSyntax("expected a %s id", kind)
	}
	id, err := strconv.ParseInt(current.text, 10, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return 0, fmt.Errorf("invalid %s id: number is too large", kind)
		}
		return 0, fmt.Errorf("invalid %s id: %q is not a number", kind, current.text)
	}
	if id < 1 {
		return 0, fmt.Errorf("invalid %s id: must be positive", kind)
	}
	return id, nil
}

// number consumes a nonnegative whole number, such as a turn or an orbit.
func (l *Line) number(what string) (int, error) {
	current, ok := l.next()
	if !ok || current.quoted {
		return 0, badSyntax("expected %s", what)
	}
	return wholeNumber(current.text, what)
}

// systemLetter consumes a system's letter, A through E.
func (l *Line) systemLetter() (string, error) {
	current, ok := l.next()
	if !ok || current.quoted || len(current.text) != 1 {
		return "", badSyntax("expected a system letter")
	}
	letter := strings.ToUpper(current.text)
	if letter < "A" || letter > "E" {
		return "", fmt.Errorf("invalid system %q; systems are A through E", current.text)
	}
	return letter, nil
}

// coordinates consumes a bracketed point, `(X,Y,Z)`, with any spacing.
func (l *Line) coordinates() (x, y, z int, err error) {
	if err := l.expect("("); err != nil {
		return 0, 0, 0, badSyntax("expected (X,Y,Z)")
	}
	values := [3]int{}
	for i := range values {
		if i != 0 {
			if err := l.expect(","); err != nil {
				return 0, 0, 0, badSyntax("expected (X,Y,Z)")
			}
		}
		current, ok := l.next()
		if !ok || current.quoted {
			return 0, 0, 0, badSyntax("expected (X,Y,Z)")
		}
		if values[i], err = wholeNumber(current.text, "coordinate"); err != nil {
			return 0, 0, 0, err
		}
	}
	if err := l.expect(")"); err != nil {
		return 0, 0, 0, badSyntax("expected (X,Y,Z)")
	}
	return values[0], values[1], values[2], nil
}

// orbitList consumes one or more orbits. A probe spends one probe on each.
func (l *Line) orbitList() ([]int, error) {
	var orbits []int
	for l.more() {
		orbit, err := l.number("orbit")
		if err != nil {
			return nil, err
		}
		orbits = append(orbits, orbit)
	}
	if len(orbits) == 0 {
		return nil, badSyntax("expected at least one orbit")
	}
	return orbits, nil
}
