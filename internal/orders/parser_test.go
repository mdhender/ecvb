// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode"
)

// What the parser is for, beside reading a good file: telling a player exactly
// what is wrong with a bad one. These tests are about the message rather than
// about the grammar, so they read the whole of what the player is shown.

// A diagnostic shows the line and marks the part of it that failed, the way a
// compiler does, because a player editing an order file wants the same two
// things a programmer does.
func TestADiagnosticShowsTheLineAndPointsAtTheWord(t *testing.T) {
	want := "line 4, column 21: expected `system` or `orbit`, found \"orbti\"\n" +
		"  4 | ship 100002 move to orbti 6\n" +
		"    |                     ^^^^^\n" +
		"    = did you mean `orbit`?\n" +
		"    = MOVE is written:\n" +
		"    =   ship SHIP-ID move to orbit ORBIT\n" +
		"    =   ship SHIP-ID move to system SYSTEM orbit ORBIT"
	if got := parseProblems(t, "ship 100002 move to orbti 6\n"); got != want {
		t.Errorf("error =\n%s\n  want exactly\n%s", got, want)
	}
}

// The message is built from the furthest point the descent reached, not from
// the first alternative that failed.
//
// MOVE has two forms and the line matches neither, so the parse fails twice
// over. What the player is told is where their line stopped looking like a
// MOVE -- the system letter that is not there -- rather than that a `to` could
// have been wanted six words back.
func TestTheMessageComesFromWhereTheParseGotFurthest(t *testing.T) {
	got := parseProblems(t, "ship 100002 move to system\n")
	if want := "expected a system letter, found the end of the order"; !strings.Contains(got, want) {
		t.Errorf("error =\n%s\n  want it to say %q", got, want)
	}
	// The `to` was read and accepted, so it is no longer in question.
	if strings.Contains(got, "`to`") {
		t.Errorf("error =\n%s\n  want nothing about `to`, which was read", got)
	}
}

// An order that runs over several lines is still several lines. Every token
// remembers which one it came from, so a mistake three lines into a CREATE is
// reported there and not against the line the CREATE opened on.
func TestAMistakeInsideAMultiLineOrderIsPlacedOnItsOwnLine(t *testing.T) {
	got := parseProblems(t, "colony 100050 create ship\n"+
		"  using 60 STRC-8\n"+
		"  transfering 25 SNSR-99\n"+
		"  with 5 CWKR\n"+
		"end\n")
	want := "line 6, column 18: invalid unit tag \"SNSR-99\"\n" +
		"  6 |   transfering 25 SNSR-99\n" +
		"    |                  ^^^^^^^"
	if got != want {
		t.Errorf("error =\n%s\n  want exactly\n%s", got, want)
	}
}

// A word that was nearly one the order wanted is named. A word that was not is
// left alone: a suggestion nobody meant is worse than none, because it teaches
// a player to stop reading the next one.
func TestANearMissIsSuggestedAndAFarOneIsNot(t *testing.T) {
	for _, item := range []struct {
		line    string
		suggest string
	}{
		{"ship 100002 move to orbti 6", "did you mean `orbit`?"},
		{"colnoy 2 move to orbit 4", "did you mean `colony`?"},
		{"ship 100002 jumpp to (1,2,3)", "did you mean `jump`?"},
		{"ship 100002 move to planet 6", ""},
		{"fleet 2 probe orbit 1", ""},
	} {
		got := parseProblems(t, item.line+"\n")
		switch {
		case item.suggest == "" && strings.Contains(got, "did you mean"):
			t.Errorf("%s was offered a suggestion; want none:\n%s", item.line, got)
		case item.suggest != "" && !strings.Contains(got, item.suggest):
			t.Errorf("%s: error =\n%s\n  want it to say %q", item.line, got, item.suggest)
		}
	}
}

// Below three characters there is nothing to be nearly right about, so nothing
// is offered. A `5` where a `,` was wanted is not a mistyped comma.
func TestNothingIsSuggestedForATokenTooShortToBeNearlyAnything(t *testing.T) {
	if word, ok := suggest("5", []string{",", "orbit"}); ok {
		t.Errorf(`suggest("5") = %q; want nothing`, word)
	}
	if word, ok := suggest("orbti", []string{",", "orbit"}); !ok || word != "orbit" {
		t.Errorf(`suggest("orbti") = (%q, %v); want orbit`, word, ok)
	}
	// A word spelled right was not a typo, so nothing was meant by it.
	if word, ok := suggest("orbit", []string{"orbit", "system"}); ok {
		t.Errorf(`suggest("orbit") = %q; want nothing`, word)
	}
}

// The distance counts a transposition as one edit rather than two, because a
// transposition is much the commonest typo and `orbti` should cost what
// `orbot` costs.
func TestTheEditDistanceCountsATranspositionOnce(t *testing.T) {
	for _, item := range []struct {
		a, b string
		want int
	}{
		{"orbit", "orbit", 0},
		{"orbti", "orbit", 1},
		{"orbot", "orbit", 1},
		{"orbi", "orbit", 1},
		{"orbits", "orbit", 1},
		{"planet", "orbit", 5},
	} {
		if got := editDistance(item.a, item.b); got != item.want {
			t.Errorf("editDistance(%q, %q) = %d; want %d", item.a, item.b, got, item.want)
		}
	}
}

// Columns are counted in the characters a player sees, and a tab is one of
// them, so the caret lands under the word rather than a tab stop away from it.
func TestATabCountsAsOneColumnSoTheCaretLandsUnderTheWord(t *testing.T) {
	got := parseProblems(t, "\tcolony 100050 assemble 6 SNSR-99\n")
	want := "line 4, column 27: invalid unit tag \"SNSR-99\"\n" +
		"  4 |  colony 100050 assemble 6 SNSR-99\n" +
		"    |                           ^^^^^^^"
	if got != want {
		t.Errorf("error =\n%s\n  want exactly\n%s", got, want)
	}
}

// Giving up on a multi-line order skips to the next line that opens one.
// Reading on from where the gather stopped would report every remaining line
// of the order's body as an order of its own: four complaints about one
// mistake, three of them about lines the player wrote correctly.
func TestGivingUpOnAMultiLineOrderResumesAtTheNextOrder(t *testing.T) {
	_, err := Parse(strings.NewReader(header +
		"colony 100050 create ship\n" +
		"  using 60 STRC-8\n" +
		"  transfering 25 \"FOOD\n" +
		"  with 5 CWKR\n" +
		"end\n" +
		"colony 100050 assemble 20 SNSR-1\n"))
	if err == nil {
		t.Fatal("the file was accepted; want it refused")
	}
	// The broken quote, and nothing about the three lines after it.
	if got := countProblems(t, err); got != 1 {
		t.Errorf("problems = %d; want 1:\n%v", got, err)
	}
}

// Two defects the CLAUDE-01 fuzzing factions found, and the rule they share:
// a diagnostic is read by a person, and both halves of it have to be true.

// A field reader is called with the name of what it reads, and that name wants
// an article in `expected an orbit` and none in `invalid orbit`. The readers
// are called both ways, so they have to say both correctly: `invalid a price`
// is not a sentence.
func TestAFieldIsNamedWithoutAnArticleWhenItIsInvalid(t *testing.T) {
	for _, item := range []struct{ line, want string }{
		{"colony 100005 sell 100 FOOD 1. GOLD", `invalid price "1."`},
		{"colony 100005 sell 100 FOOD 1,00 GOLD", `invalid price "1,00"`},
		{"colony 100005 assemble , SNSR-1", `invalid quantity ","`},
		{"colony 100005 assemble six SNSR-1", `invalid quantity "six"`},
		{"colony 100005 attack ship 100001 -5%", `invalid commitment "-5%"`},
		{"colony 100005 attack ship 100001 150%", "invalid commitment 150%"},
		// The same rule read the other way: a missing field still takes one.
		{"ship 100002 broadcast system B orbit 8", "expected a quoted message"},
		{"colony 100005 assemble 6", "expected a unit code"},
		{"ship 100002 move to orbit", "expected an orbit"},
	} {
		got := parseProblems(t, item.line+"\n")
		if !strings.Contains(got, item.want) {
			t.Errorf("%s: error =\n%s\n  want it to say %q", item.line, got, item.want)
		}
		if strings.Contains(got, "invalid a ") || strings.Contains(got, "quoted a ") {
			t.Errorf("%s: error =\n%s\n  want no doubled article", item.line, got)
		}
	}
}

// The echo and the caret under it are two halves of one picture, and a
// character a terminal gives no width to pulls them apart: the line comes out
// shorter than the caret run and the caret points past the end of a line that
// looks perfectly ordinary. Every rune that does not print is shown as one
// that does.
func TestAControlCharacterKeepsItsColumnInTheEcho(t *testing.T) {
	got := parseProblems(t, "ship 100002 move to orbit\x7f6\n")
	want := "line 4, column 21: expected `system` or `orbit`, found \"orbit\\x7f6\"\n" +
		"  4 | ship 100002 move to orbit?6\n" +
		"    |                     ^^^^^^^"
	if !strings.HasPrefix(got, want) {
		t.Errorf("error =\n%s\n  want it to begin\n%s", got, want)
	}
	// Whatever the line held, the echo and the caret agree on how wide it is.
	for _, text := range []string{"ship 100002 move to orbit\x00 6", "ship\x016 move to orbit 6",
		"\tship 100002 move to orbti 6", "ship 100002 move to \x1b[31morbti 6"} {
		report := parseProblems(t, text+"\n")
		lines := strings.Split(report, "\n")
		echo, caret := lines[1], lines[2]
		if len([]rune(caret)) > len([]rune(echo)) {
			t.Errorf("%q:\n%s\n  the caret runs past the end of the echoed line", text, report)
		}
		for _, r := range echo {
			if r != '\t' && !unicode.IsGraphic(r) {
				t.Errorf("%q: the echoed line carries %q, which a terminal gives no width", text, r)
			}
		}
	}
}

// An order line may be 8 KB and a token may be most of it, so the echo is a
// window on the line rather than the line. Without one, a single mistyped unit
// code buries every other problem in the report under nine hundred carets.
func TestALongLineIsShownAsAWindowAroundTheCaret(t *testing.T) {
	for _, item := range []struct {
		name, line, under string
		lead, trail       bool
	}{
		{
			name:  "the problem near the start",
			line:  "ship 100002 move to " + strings.Repeat("y", 300),
			under: "y", trail: true,
		},
		{
			// Far enough in that the window cuts both ends.
			name:  "the problem in the middle",
			line:  "ship 100002 move to orbit 6 " + strings.Repeat("x", 300) + " 6",
			under: "x", lead: true, trail: true,
		},
		{
			name:  "the problem at the end",
			line:  "colony 100005 assemble 6 SNSR-1, " + strings.Repeat("6 STRC-2, ", 40) + "6 SNSR-99",
			under: "SNSR-99", lead: true,
		},
	} {
		t.Run(item.name, func(t *testing.T) {
			report := strings.Split(parseProblems(t, item.line+"\n"), "\n")
			echo, caret := report[1], report[2]
			text := echo[strings.Index(echo, "| ")+2:]
			if len([]rune(text)) > echoWidth+2*len(elision) {
				t.Errorf("the echo is %d columns; want it windowed:\n%s", len([]rune(text)), echo)
			}
			if got := strings.HasPrefix(text, elision); got != item.lead {
				t.Errorf("leading %q = %v; want %v:\n%s", elision, got, item.lead, echo)
			}
			if got := strings.HasSuffix(text, elision); got != item.trail {
				t.Errorf("trailing %q = %v; want %v:\n%s", elision, got, item.trail, echo)
			}
			// The caret still lands under the text the message is about.
			at := strings.Index(caret, "^")
			if at < 0 || at >= len(echo) {
				t.Fatalf("no caret under the echo:\n%s\n%s", echo, caret)
			}
			if !strings.HasPrefix(echo[at:], item.under) {
				t.Errorf("the caret is under %q; want it under %q:\n%s\n%s",
					echo[at:min(at+len(item.under)+4, len(echo))], item.under, echo, caret)
			}
			// And it never runs past what is shown, nor under the marks that
			// say text was cut.
			if len(strings.TrimRight(caret, "^"))+strings.Count(caret, "^") > len(echo)+1 {
				t.Errorf("the caret runs past the echo:\n%s\n%s", echo, caret)
			}
			if item.trail && strings.HasSuffix(strings.TrimRight(caret, " "), "^"+elision) {
				t.Errorf("the caret reaches under the elision:\n%s\n%s", echo, caret)
			}
		})
	}
}

// The message text quotes what it found, and an 8 KB token wants eliding there
// as much as in the echo.
func TestAMessageElidesALongToken(t *testing.T) {
	got := parseProblems(t, "ship 100002 move to "+strings.Repeat("x", 300)+" 6\n")
	head := strings.SplitN(got, "\n", 2)[0]
	if len(head) > 200 {
		t.Errorf("the message is %d characters:\n%s", len(head), head)
	}
	if !strings.Contains(head, "xxx"+elision) {
		t.Errorf("message = %q; want the token elided", head)
	}
}

// An order parser is meant to return errShape or a diagnostic that points at
// the token it read. Nothing stops a new one returning a plain error instead
// -- that compiles, and behaves correctly in every way except telling the
// player where to look -- so every error that is neither kind is given the
// line the order began on.
func TestAParseErrorAlwaysCarriesAtLeastItsLine(t *testing.T) {
	// The guard itself, against the error a future order parser might return.
	p := fieldParser("ship 100002 move to orbit 6")
	placed := p.place(errors.New("something a new order parser decided"))
	if !strings.HasPrefix(placed.Error(), "line 1: something a new order parser") {
		t.Errorf("place() = %q; want it to carry the order's line", placed)
	}
	if _, ok := errors.AsType[*diagnostic](placed); !ok {
		t.Errorf("place() = %T; want a diagnostic", placed)
	}
	if p.place(nil) != nil {
		t.Error("place(nil) invented an error")
	}
	if !isShapeError(p.place(errShape)) {
		t.Error("place() swallowed a shape error, which is answered with the verb's forms")
	}
	// And the four paths that used to arrive this way now point at a token.
	for _, item := range []struct{ line, want string }{
		{"colony 100005 raid ship 100001 seeking GOLD, FUEL, METL 22%",
			"line 4, column 52: a raid seeks one unit or two, and this one names 3"},
		{"ship 100051 add 5 FACT-1 to factory-group 3",
			"line 4, column 29: a factory-group belongs to a colony"},
		{"colony 100050 add 5 FARM-1, 3 FARM-2 to farm-group 1",
			"line 4, column 19: a farm-group order names one unit, and this one names 2"},
		{"colony 100050 draft 100 USK",
			"line 4, column 25: only SOL and the cadres may be drafted"},
		{"ship 100051 create factory-group with 5 FACT-1 making CNGD",
			"line 4, column 20: a factory-group belongs to a colony"},
	} {
		got := parseProblems(t, item.line+"\n")
		if !strings.HasPrefix(got, item.want) {
			t.Errorf("%s: error =\n%s\n  want it to begin %q", item.line, got, item.want)
		}
		if !strings.Contains(got, "^") {
			t.Errorf("%s: error =\n%s\n  want a caret under the token", item.line, got)
		}
	}
}

// A report reads down the file, whatever order its problems were found in.
//
// The parser finds its own in file order, but Bind's arrive in the order the
// turn resolves -- every order of one phase before any of the next -- so a
// file whose orders are all refused used to report its last line first. The
// three orders below are written in the reverse of the order they resolve in:
// naming is the second-to-last phase of a turn and probing is a dozen phases
// ahead of it.
func TestAReportReadsDownTheFile(t *testing.T) {
	_, err := Check(context.Background(), openInventoryOrderDatabase(t), strings.NewReader(header+
		"ship 100999 name \"the naming phase is late\"\n"+
		"ship 100999 probe orbit 4\n"+
		"ship 100999 move to orbit 4\n"))
	if err == nil {
		t.Fatal("the file was accepted; want it refused")
	}
	var lines []int
	for _, block := range strings.Split(err.Error(), "\n\n") {
		var line int
		if n, _ := fmt.Sscanf(block, "line %d", &line); n == 1 {
			lines = append(lines, line)
		}
	}
	if len(lines) < 3 {
		t.Fatalf("report named %d lines; want three:\n%v", len(lines), err)
	}
	if !slices.IsSorted(lines) {
		t.Errorf("the report names lines %v; want them in the order the file was written:\n%v", lines, err)
	}
}
