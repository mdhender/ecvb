// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"strings"
	"testing"
)

// FuzzParse holds the one promise the parser makes about input it has never
// seen: whatever a player writes, it is answered rather than crashed on.
//
// Parse takes bytes off a player's disk and every field reader indexes into
// them, so a panic here is a denial of service on the whole order pipeline --
// one malformed file and no faction's turn resolves.
func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		"game \"TEST\" turn 0\nid faction 1\n\nship 2 move to orbit 6\n",
		"game \"TEST\" turn 0\nid faction 1\n\nship 2 jump to (1,2,3)\n",
		"game \"TEST\" turn 0\nid faction 1\n\ncolony 5 create ship using 60 STRC-8 with 5 CWKR end\n",
		"game \"TEST\" turn 0\nid faction 1\n\ncolony 5 sell 100 FOOD 1,000.50 GOLD 75%\n",
		"game \"TEST\" turn 0\nid faction 1\n\nwe name (1,2,3) system A orbit 8 \"x\"\n",
		"game \"TEST\" turn 0\nid faction 1\n\nship 2 probe system A orbit 1 2 3\n",
		"game \"TEST\" turn 0\nid faction 1\n\ncolony 5 buy tech-level TL-4 800,000 GOLD\n",
		"(((((,,,,)))))",
		"\t\"\n\"\n#\n,\n",
		"game \"\" turn -0\nid player \"\"\n\nwe name (0,0,0) \"\"\n",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		submission, err := Parse(strings.NewReader(text))
		if err != nil {
			// Rendering the diagnostic is half the parser and is where the
			// caret arithmetic lives, so the fuzzer has to walk it.
			_ = err.Error()
			return
		}
		for _, order := range submission.Orders {
			_ = order.Params.Input()
		}
	})
}
