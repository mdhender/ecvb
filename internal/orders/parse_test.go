// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
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
	if got := submission.Orders[0]; got.Line != 4 || got.Verb != "move" || got.ShipID != 2 || got.Orbit != 6 || got.System != "" {
		t.Fatalf("first order = %+v", got)
	}
	if got := submission.Orders[1]; got.Line != 5 || got.Verb != "jump" || got.X != 6 || got.Y != -9 || got.Z != 8 {
		t.Fatalf("second order = %+v", got)
	}
	if got := submission.Orders[2]; got.Line != 6 || got.System != "B" || got.Orbit != 4 {
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
