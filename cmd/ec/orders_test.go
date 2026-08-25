// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"bytes"
	"strings"
	"testing"

	orderpkg "github.com/mdhender/ecvb/internal/orders"
)

func TestWriteOrderResultReportsWarningsAfterTheSummary(t *testing.T) {
	var out bytes.Buffer
	result := orderpkg.Result{GameCode: "TEST", Turn: 3, FactionID: 1, Orders: 2, Warnings: []orderpkg.Warning{
		{Line: 4, Message: "ship 40 needs 160 FUEL to jump and will hold 36; the order fails unless fuel reaches the ship first"},
		{Line: 5, Message: "ship 40 needs 4 FUEL to move and will hold 0; the order fails unless fuel reaches the ship first"},
	}}
	if err := writeOrderResult(&out, "submitted", result); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"submitted 2 orders for game TEST turn 3 faction 1",
		"warning: line 4: ship 40 needs 160 FUEL to jump and will hold 36; the order fails unless fuel reaches the ship first",
		"warning: line 5: ship 40 needs 4 FUEL to move and will hold 0; the order fails unless fuel reaches the ship first",
		"",
	}, "\n")
	if out.String() != want {
		t.Errorf("output = %q; want %q", out.String(), want)
	}
}

func TestWriteOrderResultOmitsWarningsWhenThereAreNone(t *testing.T) {
	var out bytes.Buffer
	if err := writeOrderResult(&out, "checked", orderpkg.Result{GameCode: "TEST", Turn: 3, FactionID: 1, Orders: 2}); err != nil {
		t.Fatal(err)
	}
	if want := "checked 2 orders for game TEST turn 3 faction 1\n"; out.String() != want {
		t.Errorf("output = %q; want %q", out.String(), want)
	}
}
