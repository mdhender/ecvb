// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"os"
	"strings"
	"testing"
)

// TestOrdersDocumentsEveryForm keeps orders.md and the order registry in step.
//
// orders.md is the reference a player reads and the spec this package is
// written against. With one order it was easy to keep both current by hand;
// with two dozen it is not, and a form the parser accepts but the reference
// never mentions is a form nobody will use. This fails the moment they part.
func TestOrdersDocumentsEveryForm(t *testing.T) {
	reference, err := os.ReadFile("../../orders.md")
	if err != nil {
		t.Fatalf("read orders.md: %v", err)
	}
	text := string(reference)
	for _, spec := range Specs() {
		for _, form := range spec.Syntax {
			if !strings.Contains(text, form) {
				t.Errorf("orders.md does not document %q; add it to the %s section", form, strings.ToUpper(spec.Verb))
			}
		}
		if !strings.Contains(text, "## "+strings.ToUpper(spec.Verb)) {
			t.Errorf("orders.md has no ## %s section", strings.ToUpper(spec.Verb))
		}
	}
}
