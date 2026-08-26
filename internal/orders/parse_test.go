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
		"MOVE ship 2 to orbit 6\n" +
		"jump ship 2 to (6, -9, 8)\n" +
		"move ship 2 to system b orbit 4\n"))
	if err != nil {
		t.Fatal(err)
	}
	if submission.GameCode != "BETA-001" || submission.Turn != 0 || submission.Identity.FactionID != 1 {
		t.Fatalf("header = %+v", submission)
	}
	if len(submission.Orders) != 3 {
		t.Fatalf("orders = %d; want 3", len(submission.Orders))
	}
	if got := submission.Orders[0]; got.Line != 4 || got.Verb != "move" ||
		got.Params != (MoveParams{ShipID: 2, Orbit: 6}) {
		t.Fatalf("first order = %+v", got)
	}
	if got := submission.Orders[1]; got.Line != 5 || got.Verb != "jump" ||
		got.Params != (JumpParams{ShipID: 2, X: 6, Y: -9, Z: 8}) {
		t.Fatalf("second order = %+v", got)
	}
	if got := submission.Orders[2]; got.Line != 6 ||
		got.Params != (MoveParams{ShipID: 2, System: "B", Orbit: 4}) {
		t.Fatalf("third order = %+v", got)
	}
}

func TestParseReportsAllSyntaxErrors(t *testing.T) {
	_, err := Parse(strings.NewReader("" +
		"game BETA-001 turn zero\n" +
		"id somebody 1\n" +
		"ship 2 jump to (1,2,3)\n" +
		"move ship 0 to orbit 4\n"))
	if err == nil {
		t.Fatal("Parse succeeded; want errors")
	}
	message := err.Error()
	for _, want := range []string{"line 1:", "line 2:", "line 3:", "line 4: invalid ship id"} {
		if !strings.Contains(message, want) {
			t.Errorf("error %q does not contain %q", message, want)
		}
	}
}

func TestParseProbeOrders(t *testing.T) {
	input := `game "TEST" turn 3
id faction 1

probe ship 2 orbit 6
PROBE SHIP 2 ORBIT 1 2 3 4 5 8 9 10
`
	submission, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(submission.Orders) != 2 {
		t.Fatalf("orders = %d; want 2", len(submission.Orders))
	}
	one := probeParams(t, submission.Orders[0])
	if one.Kind != "ship" || one.EntityID != 2 || !slices.Equal(one.Orbits, []int{6}) {
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

probe ship 2
`
	if _, err := Parse(strings.NewReader(input)); err == nil {
		t.Fatal("Parse succeeded; want a problem")
	}
}

func TestParseProbeWithASystem(t *testing.T) {
	input := `game "TEST" turn 3
id faction 1

probe ship 4 system a orbit 1 2 3
`
	submission, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	order := probeParams(t, submission.Orders[0])
	if order.EntityID != 4 || order.System != "A" {
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
move ship 2 to orbit 6   # and a trailing one
   # indented, still a comment

probe ship 2 orbit 1
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
			name:  "unknown verb names the orders that exist",
			input: "attak ship 2",
			want:  `unknown order "attak"; expected jump, move, or probe`,
		},
		{
			name:  "a move that does not parse reports only move's forms",
			input: "move ship 2 to planet 6",
			want:  "expected move ship SHIP-ID to orbit ORBIT, or move ship SHIP-ID to system SYSTEM orbit ORBIT",
		},
		{
			name:  "a jump that does not parse reports only jump's form",
			input: "jump ship 2 towards (1,2,3)",
			want:  "expected jump ship SHIP-ID to (X,Y,Z)",
		},
		{
			name:  "a field that was read and found wrong says so itself",
			input: "move ship 0 to orbit 6",
			want:  "invalid ship id: must be positive",
		},
		{
			name:  "a system outside A through E",
			input: "move ship 2 to system Z orbit 4",
			want:  `invalid system "Z"; systems are A through E`,
		},
		{
			name:  "trailing words are a mistake, not ignored",
			input: "move ship 2 to orbit 6 please",
			want:  "expected move ship SHIP-ID to orbit ORBIT",
		},
		{
			name:  "a probe must name a ship or a colony",
			input: "probe fleet 2 orbit 1",
			want:  "expected probe ship SHIP-ID orbit ORBIT",
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
		submission, err := Parse(strings.NewReader("game \"TEST\" turn 3\nid faction 1\n\njump ship 2 to " + form + "\n"))
		if err != nil {
			t.Fatalf("Parse(%q): %v", form, err)
		}
		if got := submission.Orders[0].Params; got != (JumpParams{ShipID: 2, X: 6, Y: -9, Z: 8}) {
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
		for _, form := range spec.Syntax {
			if !strings.HasPrefix(form, spec.Verb+" ") {
				t.Errorf("order %q lists syntax %q, which does not start with the verb", spec.Verb, form)
			}
		}
		if spec.Parse == nil {
			t.Errorf("order %q has no parser", spec.Verb)
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
	if got, err := HelpFor("MOVE"); err != nil || !strings.Contains(got, "move ship SHIP-ID to orbit ORBIT") {
		t.Errorf("HelpFor(MOVE) = %q, %v; want move's syntax", got, err)
	}
}
