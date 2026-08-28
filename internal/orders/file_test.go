// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"slices"
	"strings"
	"testing"
)

func TestParseShipOrders(t *testing.T) {
	submission, err := Parse(strings.NewReader("" +
		"game \"BETA-001\" turn 0\n" +
		"id faction 1\n" +
		"\n" +
		"SHIP 100002 MOVE to orbit 6\n" +
		"ship 100002 jump to (6, -9, 8)\n" +
		"ship 100002 move to system b orbit 4\n"))
	if err != nil {
		t.Fatal(err)
	}
	if submission.GameCode != "BETA-001" || submission.Turn != 0 || submission.Identity.FactionNumber != 1 {
		t.Fatalf("header = %+v", submission)
	}
	if len(submission.Orders) != 3 {
		t.Fatalf("orders = %d; want 3", len(submission.Orders))
	}
	if got := submission.Orders[0]; got.Line != 4 || got.Verb != "move" ||
		got.Params != (MoveParams{ShipID: 100002, Orbit: 6}) {
		t.Fatalf("first order = %+v", got)
	}
	if got := submission.Orders[1]; got.Line != 5 || got.Verb != "jump" ||
		got.Params != (JumpParams{ShipID: 100002, X: 6, Y: -9, Z: 8}) {
		t.Fatalf("second order = %+v", got)
	}
	if got := submission.Orders[2]; got.Line != 6 ||
		got.Params != (MoveParams{ShipID: 100002, System: "B", Orbit: 4}) {
		t.Fatalf("third order = %+v", got)
	}
}

func TestParseReportsAllSyntaxErrors(t *testing.T) {
	_, err := Parse(strings.NewReader("" +
		"game BETA-001 turn zero\n" +
		"id somebody 1\n" +
		"ship 100002 jump to (1,2)\n" +
		"ship 0 move to orbit 4\n"))
	if err == nil {
		t.Fatal("Parse succeeded; want errors")
	}
	message := err.Error()
	for _, want := range []string{
		`line 1, column 6: expected a quoted game code, found "BETA-001"`,
		"line 2, column 4: expected `player` or `faction`, found \"somebody\"",
		"line 3, column 25: expected `,`, found \")\"",
		"line 4, column 6: invalid ship id: \"0\" is not a six-digit entity id",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("error %q does not contain %q", message, want)
		}
	}
}

func TestParseProbeOrders(t *testing.T) {
	input := `game "TEST" turn 3
id faction 1

ship 100002 probe orbit 6
SHIP 100002 PROBE ORBIT 1 2 3 4 5 8 9 10
`
	submission, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(submission.Orders) != 2 {
		t.Fatalf("orders = %d; want 2", len(submission.Orders))
	}
	one := probeParams(t, submission.Orders[0])
	if one.Kind != "ship" || one.EntityID != 100002 || !slices.Equal(one.Orbits, []int{6}) {
		t.Errorf("order = %+v; want a probe of orbit 6", one)
	}
	many := probeParams(t, submission.Orders[1])
	if want := []int{1, 2, 3, 4, 5, 8, 9, 10}; !slices.Equal(many.Orbits, want) {
		t.Errorf("orbits = %v; want %v", many.Orbits, want)
	}
}

// probeParams is the parsed order's own type. Every order carries its own
// parameters, so reading one means naming which order it is.
func probeParams(t *testing.T, order Order) ProbeParams {
	t.Helper()
	params, ok := order.Params.(ProbeParams)
	if !ok {
		t.Fatalf("order = %+v; want a probe", order)
	}
	return params
}

func TestParseRejectsProbeWithoutOrbits(t *testing.T) {
	input := `game "TEST" turn 3
id faction 1

ship 100002 probe
`
	if _, err := Parse(strings.NewReader(input)); err == nil {
		t.Fatal("Parse succeeded; want a problem")
	}
}

func TestParseProbeWithASystem(t *testing.T) {
	input := `game "TEST" turn 3
id faction 1

ship 100004 probe system a orbit 1 2 3
`
	submission, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	order := probeParams(t, submission.Orders[0])
	if order.EntityID != 100004 || order.System != "A" {
		t.Errorf("order = %+v; want a probe of system A", order)
	}
	if want := []int{1, 2, 3}; !slices.Equal(order.Orbits, want) {
		t.Errorf("orbits = %v; want %v", order.Orbits, want)
	}
}

func TestParseIgnoresCommentsAndBlankLines(t *testing.T) {
	input := `game "TEST" turn 3   # the header may carry a comment
id faction 1

# a whole line of commentary
ship 100002 move to orbit 6   # and a trailing one
   # indented, still a comment

ship 100002 probe orbit 1
`
	submission, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if submission.GameCode != "TEST" || submission.Turn != 3 {
		t.Fatalf("header = %+v", submission)
	}
	if len(submission.Orders) != 2 {
		t.Fatalf("orders = %d; want 2", len(submission.Orders))
	}
	// The line numbers are the physical ones, so an error still points at the
	// line the player has to edit.
	if got := submission.Orders[0]; got.Verb != "move" || got.Line != 5 {
		t.Errorf("first order = %+v; want a move on line 5", got)
	}
	if got := submission.Orders[1]; got.Verb != "probe" || got.Line != 8 {
		t.Errorf("second order = %+v; want a probe on line 8", got)
	}
}

// A mistyped verb is told which orders exist, so the list has to hold every
// registered one rather than a remembered few.
func TestParseNamesEveryOrderWhenTheVerbIsUnknown(t *testing.T) {
	_, err := Parse(strings.NewReader("game \"TEST\" turn 3\nid faction 1\n\nship 100002 attak\n"))
	if err == nil {
		t.Fatal("Parse succeeded; want an error")
	}
	if want := `unknown order "attak"`; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %v; want it to contain %q", err, want)
	}
	for _, spec := range Specs() {
		if !strings.Contains(err.Error(), spec.Verb) {
			t.Errorf("error = %v; want it to name the %s order", err, spec.Verb)
		}
	}
}

func TestParseNameOrders(t *testing.T) {
	input := `game "TEST" turn 3
id faction 1

ship 100018 name "Jalopy"
we name (-1,2,3) "Stellium Joe"
we name (-1,2,3) system A "Alpha Sur"
we name (-1,2,3) system A orbit 8 "Headly's Gate"
`
	submission, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`ship 100018 "Jalopy"`,
		`(-1,2,3) "Stellium Joe"`,
		`(-1,2,3) system A "Alpha Sur"`,
		`(-1,2,3) system A orbit 8 "Headly's Gate"`,
	}
	if len(submission.Orders) != len(want) {
		t.Fatalf("orders = %d; want %d", len(submission.Orders), len(want))
	}
	for i, order := range submission.Orders {
		if order.Verb != "name" {
			t.Errorf("order %d verb = %q; want name", i, order.Verb)
		}
		// A name order reads back as the line the player wrote, which is what
		// gets stored and what the reports print.
		if got := order.Params.Input(); got != want[i] {
			t.Errorf("order %d input = %q; want %q", i, got, want[i])
		}
	}
	// Naming a place is an order with no actor at all.
	if got := submission.Orders[1].Params.Actor(); got != 0 {
		t.Errorf("naming a stellium acts on entity %d; want none", got)
	}
	if got := submission.Orders[0].Params.Actor(); got != 100018 {
		t.Errorf("naming a ship acts on entity %d; want 100018", got)
	}
}

// An orbit belongs to a system, so a name may not reach for one without
// naming the system it is in.
func TestParseRejectsAnOrbitWithoutASystem(t *testing.T) {
	_, err := Parse(strings.NewReader("game \"TEST\" turn 3\nid faction 1\n\nwe name (1,2,3) orbit 8 \"Nope\"\n"))
	if err == nil {
		t.Fatal("Parse succeeded; want an error")
	}
	if want := `we name (X,Y,Z) "NAME"`; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %v; want it to show %q", err, want)
	}
}

// A `#` inside quotes is part of the value, not the start of a comment.
func TestParseKeepsAHashInsideQuotes(t *testing.T) {
	submission, err := Parse(strings.NewReader("game \"BETA#1\" turn 0\nid faction 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if submission.GameCode != "BETA#1" {
		t.Errorf("game code = %q; want BETA#1", submission.GameCode)
	}
}

func TestParseReportsTheOrderThatFailed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "a move that does not parse says which word stopped it",
			input: "ship 100002 move to planet 6",
			want:  "expected `system` or `orbit`, found \"planet\"",
		},
		{
			// The message names the word, and only move's forms follow it.
			name:  "a move that does not parse then shows only move's forms",
			input: "ship 100002 move to planet 6",
			want:  "MOVE is written:\n    =   ship SHIP-ID move to orbit ORBIT\n    =   ship SHIP-ID move to system SYSTEM orbit ORBIT",
		},
		{
			name:  "a jump that does not parse reports only jump's form",
			input: "ship 100002 jump towards (1,2,3)",
			want:  "expected `to`, found \"towards\"",
		},
		{
			// A word that was nearly right is named, because a player who
			// mistyped one wants the word and not the grammar.
			name:  "a near miss is suggested",
			input: "ship 100002 move to orbti 6",
			want:  "did you mean `orbit`?",
		},
		{
			name:  "a field that was read and found wrong says so itself",
			input: "ship 42 move to orbit 6",
			want:  `invalid ship id: "42" is not a six-digit entity id`,
		},
		{
			name:  "a system outside A through E",
			input: "ship 100002 move to system Z orbit 4",
			want:  `invalid system "Z"; systems are A through E`,
		},
		{
			name:  "trailing words are a mistake, not ignored",
			input: "ship 100002 move to orbit 6 please",
			want:  "expected the end of the order, found \"please\"",
		},
		{
			name:  "an order must name a subject the game knows",
			input: "fleet 2 probe orbit 1",
			want:  "expected an order to begin with ship, colony, or we",
		},
		{
			name:  "an order given to the wrong kind of subject says so",
			input: "colony 100002 move to orbit 4",
			want:  "MOVE is given to a ship, not to a colony",
		},
		{
			// The subject is one NAME takes, so the line is measured against
			// NAME's forms and shown them; only the place form is a faction's.
			name:  "a place named by a ship reports name's forms",
			input: `ship 100002 name (1,2,3) "Nope"`,
			want:  `expected a quoted name, found "("`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader("game \"TEST\" turn 3\nid faction 1\n\n" + tc.input + "\n"))
			if err == nil {
				t.Fatal("Parse succeeded; want an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v; want it to contain %q", err, tc.want)
			}
		})
	}
}

// Coordinates read the same however they are spaced.
func TestParseAcceptsAnySpacingInCoordinates(t *testing.T) {
	for _, form := range []string{"(6,-9,8)", "( 6 , -9 , 8 )", "(6, -9,8)"} {
		submission, err := Parse(strings.NewReader("game \"TEST\" turn 3\nid faction 1\n\nship 100002 jump to " + form + "\n"))
		if err != nil {
			t.Fatalf("Parse(%q): %v", form, err)
		}
		if got := submission.Orders[0].Params; got != (JumpParams{ShipID: 100002, X: 6, Y: -9, Z: 8}) {
			t.Errorf("Parse(%q) = %+v; want a jump to (6,-9,8)", form, got)
		}
	}
}

// Every registered order must describe itself, because the syntax it lists is
// what a player sees when their line does not parse.
func TestEveryOrderDescribesItself(t *testing.T) {
	specs := Specs()
	if len(specs) == 0 {
		t.Fatal("no orders are registered")
	}
	for _, spec := range specs {
		if spec.Summary == "" {
			t.Errorf("order %q has no summary", spec.Verb)
		}
		if len(spec.Syntax) == 0 {
			t.Errorf("order %q lists no syntax", spec.Verb)
		}
		// Every form opens with a subject the order takes and names the verb
		// after it, because that is the shape a player writes.
		shown := map[string]bool{}
		for _, form := range spec.Syntax {
			subject, rest, ok := strings.Cut(form, " ")
			if !ok || !spec.accepts(subject) {
				t.Errorf("order %q lists syntax %q, which does not open with a subject it takes", spec.Verb, form)
				continue
			}
			shown[subject] = true
			if !strings.Contains(" "+rest+" ", " "+spec.Verb+" ") {
				t.Errorf("order %q lists syntax %q, which never names the verb", spec.Verb, form)
			}
		}
		for _, subject := range spec.Subjects {
			if !shown[subject] {
				t.Errorf("order %q takes %q but no form shows one", spec.Verb, subject)
			}
		}
		if spec.Parse == nil {
			t.Errorf("order %q has no parser", spec.Verb)
		}
		if spec.Phase == nil {
			t.Errorf("order %q says nothing about when it resolves", spec.Verb)
		}
	}
}

func TestHelpListsEveryOrder(t *testing.T) {
	help := Help()
	for _, spec := range Specs() {
		if !strings.Contains(help, strings.ToUpper(spec.Verb)) {
			t.Errorf("help does not mention %q", spec.Verb)
		}
		for _, form := range spec.Syntax {
			if !strings.Contains(help, form) {
				t.Errorf("help does not show %q", form)
			}
		}
	}
	if _, err := HelpFor("nosuchorder"); err == nil {
		t.Error("HelpFor an unknown order succeeded; want an error")
	}
	if got, err := HelpFor("MOVE"); err != nil || !strings.Contains(got, "ship SHIP-ID move to orbit ORBIT") {
		t.Errorf("HelpFor(MOVE) = %q, %v; want move's syntax", got, err)
	}
	// The reference says who an order may be given to, because that is half of
	// knowing how to write the line.
	if got, err := HelpFor("PROBE"); err != nil || !strings.Contains(got, "given to a ship or a colony") {
		t.Errorf("HelpFor(PROBE) = %q, %v; want probe's subjects", got, err)
	}
}

// opensOrder is a lookahead: it finds the verb of a line the same way the
// parser proper does, by reading the subject, and it has to leave the line
// exactly as it found it, because Parse hands that same Line straight on to
// parseOrder. This is the test of both halves at once -- the verb it read and
// A quote that is never closed is refused.
//
// The tokenizer used to read to the end of the line and call it a token, so
// `ship 100018 name "Jalopy` named the ship and said nothing: quoted text is how a
// name is written, and a player who dropped the closing quote found out from a
// report a turn later. Nothing else a line can get wrong is settled this early,
// and nothing else needs to be.
func TestAQuoteThatIsNeverClosedIsRefused(t *testing.T) {
	for _, item := range []struct{ line, want string }{
		{`ship 100018 name "Jalopy`, `unterminated quoted text "Jalopy"`},
		{`we name (1,2,3) "Near`, `unterminated quoted text "Near"`},
		{`ship 100018 broadcast system B orbit 8 "hello" "sig`, `unterminated quoted text "sig"`},
		// An empty run is still a run: the quote is still open at the newline.
		{`ship 100018 name "`, `unterminated quoted text ""`},
	} {
		_, err := Parse(strings.NewReader(header + item.line + "\n"))
		if err == nil {
			t.Errorf("%s was accepted; want it refused", item.line)
			continue
		}
		if !strings.Contains(err.Error(), item.want) {
			t.Errorf("%s: error = %q; want it to mention %q", item.line, err, item.want)
		}
	}
}

// The header is quoted text too, and is read before any order.
func TestAnUnterminatedQuoteInTheHeaderIsRefused(t *testing.T) {
	_, err := Parse(strings.NewReader("game \"BETA-001 turn 0\nid faction 1\n"))
	if err == nil || !strings.Contains(err.Error(), "line 1, column 6: unterminated quoted text") {
		t.Errorf("err = %v; want line 1 refused for an unterminated quote", err)
	}
}

// A broken line inside a multi-line order stops the gather rather than letting
// it read on to the end of the file looking for a terminator the broken line
// might have swallowed.
func TestAnUnterminatedQuoteInsideACreateStopsTheGather(t *testing.T) {
	body := `colony 100050 create ship
  using 60 STRC-8
  transfering 25 "FOOD
  with 5 CWKR
end
colony 100050 assemble 6 SNSR-99
`
	err := parseProblems(t, body)
	// The quote is reported where it was written rather than against the line
	// the CREATE opened on. An order that runs over several lines is still
	// several lines, and every token in it remembers which one it came from.
	for _, want := range []string{
		`line 6, column 18: unterminated quoted text "FOOD"`,
		`line 9, column 26: invalid unit tag "SNSR-99"`,
	} {
		if !strings.Contains(err, want) {
			t.Errorf("error = %q;\n  want it to hold %q", err, want)
		}
	}
}

func parseProblems(t *testing.T, body string) string {
	t.Helper()
	_, err := Parse(strings.NewReader(header + body))
	if err == nil {
		t.Fatal("the file was accepted; want it refused")
	}
	return err.Error()
}

// the cursor it did not move.
func TestOpensOrderReadsTheVerbAndConsumesNothing(t *testing.T) {
	for _, item := range []struct {
		text string
		verb string
		form string
	}{
		// Every subject the grammar has, because the verb sits at a different
		// word in each of them.
		{"ship 100002 move to orbit 6", "move", "to"},
		{"colony 100050 create ship using 60 STRC-8 end", "create", "ship"},
		{`we name (-1,2,3) "Stellium Joe"`, "name", "("},
		// A line that is not an order at all, which is every continuation line
		// of a create, and a line whose verb is not one the game has.
		{"  using 60 STRC-8", "", ""},
		{"ship 100002 fly to orbit 6", "", ""},
	} {
		line := fieldParser(item.text)
		spec, form, ok := line.opens()
		switch {
		case item.verb == "" && ok:
			t.Errorf("%q opens %q; want no order", item.text, spec.Verb)
		case item.verb != "" && !ok:
			t.Errorf("%q opens no order; want %q", item.text, item.verb)
		case item.verb != "" && (spec.Verb != item.verb || form != item.form):
			t.Errorf("%q opens %q with form %q; want %q and %q",
				item.text, spec.Verb, form, item.verb, item.form)
		}
		if line.pos != 0 {
			t.Errorf("%q left the line at word %d; want it back at the beginning",
				item.text, line.pos)
		}
	}
}
