// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"strings"
	"testing"
)

// The price reader carries four rules at once -- the thousands separators a
// quantity uses, a decimal part that only the last group may carry, a currency,
// and a whole-number form for a technology level -- so it gets a table of its
// own rather than being exercised only through the orders that read it.
func TestPriceReadsTheShapesTheMarketAllows(t *testing.T) {
	for _, item := range []struct{ text, want string }{
		// The four the accepted doc gives.
		{"1.0 GOLD", "1.0 GOLD"},
		{"3 CNGD", "3 CNGD"},
		{"0.1 GOLD", "0.1 GOLD"},
		{"25,600 CNGD", "25,600 CNGD"},
		{"1,000,000 GOLD", "1,000,000 GOLD"},
		// The decimal rides on the last group, and ends the number.
		{"1,000.50 GOLD", "1,000.50 GOLD"},
		// A currency is a keyword, so it is read whatever case it is written in.
		{"1 gold", "1 GOLD"},
	} {
		line := fieldParser(item.text)
		price, err := line.price("a price", false)
		if err != nil {
			t.Errorf("%s: %v", item.text, err)
			continue
		}
		if got := price.String(); got != item.want {
			t.Errorf("%s read back as %q; want %q", item.text, got, item.want)
		}
	}
}

func TestPriceRefusesWhatTheGrammarDoesNotAllow(t *testing.T) {
	for _, item := range []struct{ text, want string }{
		{"5000 GOLD", "a number over 999 separates every three digits with a comma"},
		// A comma inside a number groups three digits and can be doing nothing
		// else, so a group of the wrong length says so. It used to fall through
		// to the currency check and blame the currency.
		{"1,00 GOLD", `invalid price "1,00"; a comma in a number groups exactly three digits`},
		{"1,0000 GOLD", `invalid price "1,0000"; a comma in a number groups exactly three digits`},
		{"1,00.5 GOLD", "a comma in a number groups exactly three digits"},
		{"1,", "a comma in a number groups exactly three digits"},
		// A leading zero is refused, but one zero before a decimal point is
		// required rather than refused.
		{"00.5 GOLD", `invalid price "00.5"; a number carries no leading zero`},
		{"0500 GOLD", "a number carries no leading zero"},
		{"0 GOLD", "a price is greater than zero"},
		{"0.0 GOLD", "a price is greater than zero"},
		{".5 GOLD", "a price is a number and a currency"},
		{"1. GOLD", "a price is a number and a currency"},
		{"-1 GOLD", "a price is a number and a currency"},
		{"1 SILVER", "a price is paid in GOLD or CNGD"},
	} {
		line := fieldParser(item.text)
		price, err := line.price("a price", false)
		if err == nil {
			t.Errorf("%s was read as %q; want it refused", item.text, price)
			continue
		}
		if !strings.Contains(err.Error(), item.want) {
			t.Errorf("%s: error = %q; want it to mention %q", item.text, err, item.want)
		}
	}
}

// A technology level is bought once, for a whole number of GOLD.
func TestAWholePriceRefusesADecimalAndRefusesGoods(t *testing.T) {
	for _, item := range []struct{ text, want string }{
		{"800,000.5 GOLD", "it is paid in whole units"},
		{"800,000 CNGD", "a technology level is paid for in GOLD"},
	} {
		line := fieldParser(item.text)
		if _, err := line.price("a price", true); err == nil || !strings.Contains(err.Error(), item.want) {
			t.Errorf("%s: err = %v; want it to mention %q", item.text, err, item.want)
		}
	}
	line := fieldParser("800,000 GOLD")
	if price, err := line.price("a price", true); err != nil || price.String() != "800,000 GOLD" {
		t.Errorf("800,000 GOLD = (%q, %v); want it read whole", price, err)
	}
}

// fieldParser is one line of text on a Parser of its own, for the tests that
// exercise a field reader directly rather than through an order that reads it.
func fieldParser(text string) *Parser {
	src := &source{}
	src.add(text)
	tokens, _ := lex(1, text)
	return newParser(src, 1, tokens, "line")
}
