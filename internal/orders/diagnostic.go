// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// What the parser hands back when a file is wrong.
//
// A diagnostic is one problem, placed. It renders the way a compiler does,
// because a player editing an order file wants the same two things a
// programmer does: the line, and a mark under the part of it that failed.
//
//	line 6, column 16: expected `system` or `orbit`, found "planet"
//	  6 | ship 2 move to planet 6
//	    |                ^^^^^^
//	    = MOVE is written:
//	    =   ship SHIP-ID move to orbit ORBIT
//	    =   ship SHIP-ID move to system SYSTEM orbit ORBIT
type diagnostic struct {
	// pos is where the offending text begins.
	pos Position
	// width is how many columns to draw the caret under. Zero means the
	// problem is a thing that is not there, and one mark is drawn.
	width int
	// text is the one-line statement of what is wrong.
	text string
	// notes are the supporting lines: a suggestion, the order's forms.
	notes []string
	// source is the physical line, kept so the caret has something to point
	// at. A diagnostic built without one renders as a single line.
	source string
}

// How much of a line a diagnostic will show. An order line may be 8 KB and a
// token may be most of it, so the echo is a window on the line rather than the
// line: without one, a single mistyped unit code buries every other problem in
// the report under nine hundred carets.
const (
	// echoWidth is the widest run of source a diagnostic echoes.
	echoWidth = 96
	// echoLead is how much of the line before the caret the window keeps, so
	// the reader sees what led up to the problem rather than starting at it.
	echoLead = 24
	// elision marks an end the window cut.
	elision = "..."
	// longestFound is how much of a token the message text quotes.
	longestFound = 48
)

// elide shortens text to at most limit characters, marking that it did.
func elide(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + elision
}

func (d *diagnostic) Error() string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s: %s", d.pos, d.text)
	if d.source == "" {
		return out.String()
	}
	echo, column, width := d.window()
	// The gutter is as wide as the line number, so the bar under a message
	// about line 9 and one about line 148 each sit against their own number.
	gutter := len(strconv.Itoa(d.pos.Line))
	fmt.Fprintf(&out, "\n  %*d | %s", gutter, d.pos.Line, echo)
	fmt.Fprintf(&out, "\n  %*s | %s%s", gutter, "",
		strings.Repeat(" ", column), strings.Repeat("^", width))
	for _, note := range d.notes {
		fmt.Fprintf(&out, "\n  %*s = %s", gutter, "", note)
	}
	return out.String()
}

// window is the source to echo, and where the caret goes under it.
//
// A line that fits is shown whole and the caret is where it always was, so
// every diagnostic short enough to read is unchanged. A longer one is cut to a
// run around the caret, each cut end marked with `...`, and the caret moved to
// keep its place under the same text it was under before.
func (d *diagnostic) window() (echo string, column, width int) {
	runes := []rune(showable(d.source))
	column, width = max(d.pos.Column-1, 0), max(d.width, 1)
	lead, trail, start, end := "", "", 0, len(runes)
	if len(runes) > echoWidth {
		if column > echoLead {
			start, lead = column-echoLead, elision
		}
		if start+echoWidth < end {
			end, trail = start+echoWidth, elision
		}
	}
	column += len([]rune(lead)) - start
	echo = lead + string(runes[start:end]) + trail
	// The caret marks source, never the marks that say source was cut. It may
	// reach one column past the last of it -- that is the caret for something
	// that is not there -- and no further.
	limit := len([]rune(lead)) + (end - start)
	if column > limit {
		column = limit
	}
	if column+width > limit {
		width = max(limit-column, 1)
	}
	return echo, column, width
}

// onLine is a problem with a whole order rather than with a word of it: a
// destination that is not in the game, a ship that is not the faction's. These
// come from Bind rather than from the parser, so there is no token to point
// at, and the line alone is what the player needs.
func onLine(number int, format string, args ...any) *diagnostic {
	return &diagnostic{pos: Position{Line: number}, text: fmt.Sprintf(format, args...)}
}

// note adds a supporting line.
func (d *diagnostic) note(format string, args ...any) *diagnostic {
	d.notes = append(d.notes, fmt.Sprintf(format, args...))
	return d
}

// forms adds an order's syntax to a diagnostic, which is what a line that
// never matched the shape of its order is answered with.
func (d *diagnostic) forms(spec *Spec) *diagnostic {
	d.note("%s is written:", strings.ToUpper(spec.Verb))
	for _, form := range spec.Syntax {
		d.note("  %s", form)
	}
	return d
}

// showable is the source line as the caret line was measured against: one
// column per rune, every rune visible.
//
// Both halves matter. A tab is one column to the lexer and eight to a
// terminal, so it is shown as the one space that keeps the caret under the
// word. Anything else that does not print -- a NUL, a DEL, an escape -- is
// shown as `?`, because a terminal gives those no width at all: the line would
// come out shorter than the caret run beneath it and the caret would point
// past the end of a line that looks perfectly ordinary. The message names the
// byte exactly, being written with %q; the echo only has to keep its place.
func showable(text string) string {
	var out strings.Builder
	for _, r := range text {
		switch {
		case r == '\t' || r == '\r':
			out.WriteByte(' ')
		case unicode.IsGraphic(r):
			out.WriteRune(r)
		default:
			out.WriteByte('?')
		}
	}
	return out.String()
}

// problems is every diagnostic a file produced. The parser reports them all
// rather than stopping at the first, so a player fixing a file sees the whole
// list; each one is a block of its own, so they are separated by a blank line
// rather than run together.
type problems []error

func (p problems) Error() string {
	blocks := make([]string, len(p))
	for i, item := range p {
		blocks[i] = item.Error()
	}
	return strings.Join(blocks, "\n\n")
}

// inFileOrder sorts the report the way the player wrote the file.
//
// The parser finds its problems in file order and needs no help, but Bind's
// arrive in the order the turn resolves -- every order of one phase before any
// of the next -- so a file whose orders are all refused reports line 23 first
// and line 3 fifteenth. That is not wrong and it is not readable: a player
// fixing a file reads down it. Sorting is stable, so two problems on one line
// stay in the order they were found.
func (p problems) inFileOrder() problems {
	sort.SliceStable(p, func(i, j int) bool {
		return lineOf(p[i]) < lineOf(p[j])
	})
	return p
}

// lineOf is the physical line a problem is about, and zero for one that names
// no line at all, which sorts it to the top.
func lineOf(err error) int {
	if placed, ok := errors.AsType[*diagnostic](err); ok {
		return placed.pos.Line
	}
	return 0
}

// shortestSuggestion is the length below which nothing is suggested.
const shortestSuggestion = 3

// suggest is the candidate closest to what the player wrote, when one is close
// enough to be worth naming. It is what turns `expected `orbit`, found
// "orbti"` into an answer rather than a report.
//
// The distance is optimal string alignment rather than plain Levenshtein, so a
// transposition -- much the commonest typo, and exactly what `orbti` is --
// counts as one edit and not two.
func suggest(written string, candidates []string) (string, bool) {
	written = strings.ToLower(written)
	// Below three characters there is nothing to be nearly right about: every
	// short token is one edit from every other, so `5` would be offered `,`
	// and a player would be told they had mistyped a punctuation mark.
	if len(written) < shortestSuggestion {
		return "", false
	}
	best, distance := "", 0
	for _, candidate := range candidates {
		if len(candidate) < shortestSuggestion {
			continue
		}
		lowered := strings.ToLower(candidate)
		if lowered == written {
			return "", false // it was not a typo, so nothing was meant by it
		}
		d := editDistance(written, lowered)
		if best == "" || d < distance {
			best, distance = candidate, d
		}
	}
	// A short word tolerates one mistake and a longer one two. Any looser and
	// the suggestion is a guess, which is worse than none: a player who is
	// told they may have meant `orbit` when they wrote `raid` stops trusting
	// the next suggestion.
	allowed := 2
	if len(written) < 5 {
		allowed = 1
	}
	if best == "" || distance > allowed {
		return "", false
	}
	return best, true
}

// editDistance is the optimal string alignment distance: insertions,
// deletions, substitutions, and the transposition of two adjacent characters,
// each costing one.
func editDistance(a, b string) int {
	x, y := []rune(a), []rune(b)
	// rows holds the last three rows of the matrix, which is all a
	// transposition ever reaches back to.
	prev2 := make([]int, len(y)+1)
	prev := make([]int, len(y)+1)
	current := make([]int, len(y)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(x); i++ {
		current[0] = i
		for j := 1; j <= len(y); j++ {
			cost := 1
			if x[i-1] == y[j-1] {
				cost = 0
			}
			current[j] = min(prev[j]+1, current[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && x[i-1] == y[j-2] && x[i-2] == y[j-1] {
				current[j] = min(current[j], prev2[j-2]+1)
			}
		}
		prev2, prev, current = prev, current, prev2
	}
	return prev[len(y)]
}
