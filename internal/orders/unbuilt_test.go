// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"context"
	"strings"
	"testing"
)

// Every verb of docs/accepted-orders.md is registered. The accepted doc is the
// order set the game is heading for, and a verb the parser does not know is one
// a player cannot even be told is unfinished.
func TestEveryAcceptedVerbIsRegistered(t *testing.T) {
	accepted := []string{
		"activate", "add", "assemble", "assess", "attack", "broadcast", "buy",
		"control", "convert", "create", "detect", "disband", "draft", "grant",
		"idle", "incite", "invade", "jump", "move", "name", "neutralize",
		"obtain", "pay", "probe", "raid", "rations", "refuse", "release",
		"remove", "retool", "sell", "stow", "support", "survey", "transfer",
		"unassemble", "unstow",
	}
	for _, verb := range accepted {
		if _, ok := Lookup(verb); !ok {
			t.Errorf("%s is in docs/accepted-orders.md and the parser does not know it", verb)
		}
	}
	if len(Specs()) != len(accepted) {
		t.Errorf("registry holds %d orders; the accepted doc carries %d", len(Specs()), len(accepted))
	}
}

// The whole point of the choice: a file holding an order that is not built is
// still a file a player can submit. It is a warning at submission, not a
// refusal, and the orders around it are unaffected.
func TestAnOrderThatIsNotBuiltIsKeptAndFailsWhenTheTurnResolves(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	result := check(t, conn, "colony 50 rations 75%\ncolony 50 assemble 20 SNSR-1\n")
	if result.Orders != 2 {
		t.Fatalf("orders = %d; want both kept", result.Orders)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %+v; want one, on the rations", result.Warnings)
	}
	want := "RATIONS is accepted but not built yet; the rules it needs are not written"
	if !strings.Contains(result.Warnings[0].Message, want) {
		t.Errorf("warning = %q; want it to say %q", result.Warnings[0].Message, want)
	}
	// The order beside it worked.
	apply(t, conn, "colony 50 rations 75%\ncolony 50 assemble 20 SNSR-1\n")
	if got := storedQuantity(t, conn, 50, "component", "SNSR", 1); got != 20 {
		t.Errorf("assembled SNSR-1 = %d; want 20: the unbuilt order beside it changed nothing", got)
	}
}

// An unbuilt order still settles who may give it. What is missing is only what
// the order does.
func TestAnOrderThatIsNotBuiltStillChecksWhoGaveIt(t *testing.T) {
	for _, item := range []struct{ line, want string }{
		{"ship 54 rations 75%", "does not belong to faction 1"},
		{"ship 50 rations 75%", "is a COPN, not a ship"},
		{"colony 99 rations 75%", "colony 99 does not exist"},
	} {
		_, err := Check(context.Background(), openInventoryOrderDatabase(t), strings.NewReader(header+item.line+"\n"))
		if err == nil || !strings.Contains(err.Error(), item.want) {
			t.Errorf("%s: err = %v; want it to mention %q", item.line, err, item.want)
		}
	}
}

// A malformed order is refused whether or not its verb is finished. This is
// what the parser is for.
func TestTheParserRefusesAMalformedUnbuiltOrder(t *testing.T) {
	for _, item := range []struct{ line, want string }{
		{"colony 50 attack ship 18 150%", "it runs from 1% to 100%"},
		{"colony 50 attack ship 18 75", "a percentage is digits and a % sign"},
		// A line that never matched the shape of its order is answered with that
		// order's forms rather than with the token that failed.
		{"colony 50 attack planet 18 75%", "expected ship SHIP-ID attack (ship | colony) ID PERCENT%"},
		{"colony 50 raid ship 18 seeking GOLD, FUEL, METL 22%", "a raid seeks one unit or two"},
		{"colony 50 rations 75", "a percentage is digits and a % sign"},
		{"colony 50 draft 100 USK", "only SOL and the cadres may be drafted"},
		{"colony 50 pay GOLD 15%", "expected ship SHIP-ID pay CLASS PERCENT%"},
		{"colony 50 sell tech-level TL-99 800,000 GOLD", "a level runs from 1 to 10"},
		{"colony 50 sell tech-level TL-4 800,000 CNGD", "a technology level is paid for in GOLD"},
		{"colony 50 sell tech-level TL-4 800,000.5 GOLD", "it is paid in whole units"},
		{"colony 50 sell 100 FOOD 5000 CNGD", "separates every three digits with a comma"},
		{"colony 50 sell 100 FOOD 3 SILVER", "a price is paid in GOLD or CNGD"},
		{"ship 51 add 5 FACT-1 to factory-group 3", "a factory-group belongs to a colony"},
		{"colony 50 add 5 FARM-1, 3 FARM-2 to farm-group 1", "a farm-group order names one unit"},
		{"ship 51 create factory-group with 5 FACT-1 making CNGD", "a factory-group belongs to a colony"},
	} {
		_, err := Check(context.Background(), openInventoryOrderDatabase(t), strings.NewReader(header+item.line+"\n"))
		if err == nil {
			t.Errorf("%s was accepted; want it refused", item.line)
			continue
		}
		if !strings.Contains(err.Error(), item.want) {
			t.Errorf("%s: error = %q; want it to mention %q", item.line, err, item.want)
		}
	}
}

// A well-formed order of every verb parses and reads back in the words the
// player used. Reading it back is what proves the parse kept everything: an
// Input that lost a field would show it here.
func TestEveryAcceptedFormParsesAndReadsBack(t *testing.T) {
	for _, item := range []struct{ line, want string }{
		{"colony 24 attack ship 18 75%", "ship 18 75%"},
		{"colony 24 invade ship 18 55%", "ship 18 55%"},
		{"ship 18 raid colony 24 seeking GOLD, FUEL 22%", "colony 24 seeking GOLD, FUEL 22%"},
		{"ship 18 support ship 97 attacking 35%", "ship 97 attacking 35%"},
		{"ship 18 support ship 97 attacking colony 24 35%", "ship 97 attacking colony 24 35%"},
		{"ship 18 support colony 14 defending 40%", "colony 14 defending 40%"},
		{"ship 18 support colony 14 defending against ship 33 45%", "colony 14 defending against ship 33 45%"},
		{"colony 24 create factory-group with 54,000 FACT-6 making CNGD", "factory-group with 54,000 FACT-6 making CNGD"},
		{"ship 18 create farm-group with 1,234,000 FARM-6", "farm-group with 1,234,000 FARM-6"},
		{"colony 83 create mine-group with 25,680 MINE-2 working deposit 18", "mine-group with 25,680 MINE-2 working deposit 18"},
		{"colony 24 add 63 FACT-9 to factory-group 3", "63 FACT-9 to factory-group 3"},
		{"colony 24 remove 12,000 FACT-6, 63 FACT-9 from factory-group 3 and stow",
			"12,000 FACT-6, 63 FACT-9 from factory-group 3 and stow"},
		{"colony 24 idle 5,000 FACT-6 in factory-group 3", "5,000 FACT-6 in factory-group 3"},
		{"colony 24 activate 5,000 FACT-6 in factory-group 3", "5,000 FACT-6 in factory-group 3"},
		{"colony 24 retool immediately factory-group 3 making CNGD", "immediately factory-group 3 making CNGD"},
		{"colony 24 retool factory-group 3 making CNGD", "factory-group 3 making CNGD"},
		{"ship 18 sell 4,500 GOLD 1.0 CNGD", "4,500 GOLD 1.0 CNGD"},
		{"ship 18 buy 100 FOOD 3 CNGD, 25 CNGD 25,600 GOLD 5%", "100 FOOD 3 CNGD, 25 CNGD 25,600 GOLD 5%"},
		{"colony 24 sell tech-level TL-4 800,000 GOLD 5%", "tech-level TL-4 800,000 GOLD 5%"},
		{"ship 18 buy tech-level TL-6 1,000,000 GOLD", "tech-level TL-6 1,000,000 GOLD"},
		{"ship 18 survey", "ship 18"},
		{"colony 24 assess rebels using 1 spies", "rebels using 1 spies"},
		{"colony 24 detect spies using 4 spies", "spies using 4 spies"},
		{"colony 24 obtain information from ship 18 using 200 spies", "information from ship 18 using 200 spies"},
		{"colony 24 convert rebels using 3 spies", "rebels using 3 spies"},
		{"colony 24 incite rebels using 21 spies", "rebels using 21 spies"},
		{"colony 24 neutralize faction 1 spies using 11 spies", "faction 1 spies using 11 spies"},
		{`ship 18 broadcast system B orbit 8 "message" "optional signature"`,
			`system B orbit 8 "message" "optional signature"`},
		{`ship 18 broadcast system B orbit 8 "message"`, `system B orbit 8 "message"`},
		{"colony 24 draft 200 SOL, 1,000 CWKR", "200 SOL, 1,000 CWKR"},
		{"ship 18 disband 1,000 CWKR", "1,000 CWKR"},
		{"colony 24 pay SKW 15%, USK 18%", "SKW 15%, USK 18%"},
		{"colony 24 rations 130%", "130%"},
		{"we release ship 18", "ship 18"},
		{"we release (-1,2,3) system A orbit 5", "(-1,2,3) system A orbit 5"},
		{"we grant trade (-1,2,3) system A orbit 5 station 4 to faction 1",
			"trade (-1,2,3) system A orbit 5 station 4 to faction 1"},
		{"we refuse colonize (-1,2,3) system A orbit 5 to faction 1",
			"colonize (-1,2,3) system A orbit 5 to faction 1"},
		{"ship 18 control colony 24", "colony 24"},
		{"colony 8 control system A orbit 5", "system A orbit 5"},
		{`we name faction 5 "The Hegemony"`, `faction 5 "The Hegemony"`},
		{`we name player 5 ship 19 "Easy Target"`, `player 5 ship 19 "Easy Target"`},
	} {
		order, err := parseOne(item.line)
		if err != nil {
			t.Errorf("%s: %v", item.line, err)
			continue
		}
		if got := order.Params.Input(); got != item.want {
			t.Errorf("%s read back as %q; want %q", item.line, got, item.want)
		}
	}
}

// A group create is one line and takes no `end`, and a ship or colony create
// runs to one. Which it is depends on the form, which is why the terminator is
// asked of the Spec with the form in hand.
func TestOnlyTheShipAndColonyCreateRunsToEnd(t *testing.T) {
	body := `colony 50 create farm-group with 100 FARM-1
colony 50 assemble 20 SNSR-1
`
	submission, err := Parse(strings.NewReader(header + body))
	if err != nil {
		t.Fatalf("a group create was read as running to `end`: %v", err)
	}
	if len(submission.Orders) != 2 {
		t.Fatalf("orders = %d; want 2: a group create is one line", len(submission.Orders))
	}
}

// The Unbuilt flag and the Bound have to agree, or the reference tells a player
// one thing and the engine does another. registerUnbuilt is what keeps them
// together, and this is what holds it honest: exactly the verbs of unbuilt.go
// carry the flag, and none of the built ones do.
func TestExactlyTheUnbuiltVerbsAreMarkedUnbuilt(t *testing.T) {
	built := map[string]bool{
		"assemble": true, "create": true, "jump": true, "move": true, "name": true,
		"probe": true, "stow": true, "transfer": true, "unassemble": true, "unstow": true,
	}
	for _, spec := range Specs() {
		if spec.Unbuilt == built[spec.Verb] {
			t.Errorf("%s is marked unbuilt=%t; want %t", spec.Verb, spec.Unbuilt, !built[spec.Verb])
		}
	}
}
