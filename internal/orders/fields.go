// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mdhender/ecvb/internal/entityid"
	"github.com/mdhender/ecvb/internal/units"
)

// The fields of the grammar: the productions every order is built out of.
//
// Each is written once here rather than once per order, and each reports in
// one of the two ways the parser has. A field that was never there at all
// records what it wanted and returns errShape, so the order it belongs to can
// say what was expected where and show its forms. A field that was read and
// found wrong -- a system outside A through E, a quantity with a leading zero
// -- returns a diagnostic of its own, pointing at the token it read, because
// nothing the order could add would improve on it.

// word consumes the next token when it is an unquoted word, and records what
// was wanted when it is not.
//
// Nothing is consumed on failure, which is what lets the message point at the
// token that was actually there rather than at the one after it.
func (p *Parser) word(what string) (token, bool) {
	p.want(what)
	current, ok := p.peek()
	if !ok || current.quoted {
		return token{}, false
	}
	p.pos++
	return current, true
}

// punct consumes a punctuation mark without recording an expectation for it.
// It is for the productions that describe themselves whole -- coordinates are
// wanted as `(X,Y,Z)`, not as a `(` and then three numbers and then a `)`.
func (p *Parser) punct(mark string) bool {
	current, ok := p.peek()
	if !ok || current.quoted || current.text != mark {
		return false
	}
	p.pos++
	return true
}

// A field reader is called with the name of what it reads, and that name is
// wanted in two spellings: `expected an orbit` takes an article and `invalid
// orbit` does not. article and noun put one on and take one off, so a reader
// says both correctly whichever spelling its caller passed.
//
// Both spellings are in the call sites -- `number("orbit")` and
// `quantity("a quantity")` -- and normalising here rather than at the calls is
// what keeps a reader from reading `invalid a price` the day someone adds one
// more field in the other style.
var articles = []string{"a ", "an ", "the "}

// article puts an article in front of a noun, for `expected ...`.
func article(what string) string {
	if what == "" || what != bare(what) || strings.HasPrefix(what, "at least ") {
		return what
	}
	if strings.ContainsRune("aeiouAEIOU", rune(what[0])) {
		return "an " + what
	}
	return "a " + what
}

// bare takes an article off, for `invalid ...`.
func bare(what string) string {
	for _, prefix := range articles {
		if rest, found := strings.CutPrefix(what, prefix); found {
			return rest
		}
	}
	return what
}

// wholeNumber converts a token, telling a value that is out of range from one
// that was never a number at all.
func wholeNumber(text, what string) (int, error) {
	value, err := strconv.Atoi(text)
	if err == nil {
		return value, nil
	}
	if errors.Is(err, strconv.ErrRange) {
		return 0, fmt.Errorf("%s is too large", what)
	}
	return 0, fmt.Errorf("invalid %s %q", what, text)
}

// quoted consumes a quoted string, such as a game code, an email address, or a
// name.
func (p *Parser) quoted(what string) (string, error) {
	p.want("a quoted " + bare(what))
	current, ok := p.peek()
	if !ok || !current.quoted {
		return "", errShape
	}
	p.pos++
	return current.text, nil
}

// entityID consumes the number a player writes for a ship or a colony. Every
// entity number is six digits, so a mistyped one is caught here and named as a
// mistyped number rather than reaching Bind and being reported as an entity
// that does not exist.
func (p *Parser) entityID(kind string) (int64, error) {
	id, current, err := p.wholeNumber(kind)
	if err != nil {
		return 0, err
	}
	if id < entityid.MinNumber || id > entityid.MaxNumber {
		return 0, p.fail(current, "invalid %s id: %q is not a six-digit entity id", kind, current.text)
	}
	return id, nil
}

// factionID consumes the number a player writes for a faction. A faction is
// counted from 1 within its game, so unlike an entity there is no width to
// check -- only that it is a number and positive.
func (p *Parser) factionID() (int64, error) {
	id, current, err := p.wholeNumber("faction")
	if err != nil {
		return 0, err
	}
	if id < 1 {
		return 0, p.fail(current, "invalid faction id: must be positive")
	}
	return id, nil
}

// wholeNumber reads the id token itself, which is the half the two share.
func (p *Parser) wholeNumber(kind string) (int64, token, error) {
	current, ok := p.word("a " + kind + " id")
	if !ok {
		return 0, token{}, errShape
	}
	id, err := strconv.ParseInt(current.text, 10, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return 0, current, p.fail(current, "invalid %s id: number is too large", kind)
		}
		return 0, current, p.fail(current, "invalid %s id: %q is not a number", kind, current.text)
	}
	return id, current, nil
}

// number consumes a nonnegative whole number, such as a turn or an orbit.
func (p *Parser) number(what string) (int, error) {
	current, ok := p.word(article(what))
	if !ok {
		return 0, errShape
	}
	value, err := wholeNumber(current.text, what)
	if err != nil {
		return 0, p.fail(current, "%s", err)
	}
	return value, nil
}

// systemLetter consumes a system's letter, A through E.
func (p *Parser) systemLetter() (string, error) {
	current, ok := p.word("a system letter")
	if !ok {
		return "", errShape
	}
	letter := strings.ToUpper(current.text)
	if len(current.text) != 1 || letter < "A" || letter > "E" {
		return "", p.fail(current, "invalid system %q; systems are A through E", current.text)
	}
	return letter, nil
}

// coordinates consumes a bracketed point, `(X,Y,Z)`, with any spacing.
//
// The whole point is one expectation rather than five, because a player who
// wrote something else there meant to write coordinates and wants to be shown
// the shape of them, not told that a `(` was missing.
func (p *Parser) coordinates() (x, y, z int, err error) {
	p.want("coordinates (X,Y,Z)")
	if !p.punct("(") {
		return 0, 0, 0, errShape
	}
	var values [3]int
	for i := range values {
		if i != 0 {
			p.want("`,`")
			if !p.punct(",") {
				return 0, 0, 0, errShape
			}
		}
		current, ok := p.word("a coordinate")
		if !ok {
			return 0, 0, 0, errShape
		}
		if values[i], err = wholeNumber(current.text, "coordinate"); err != nil {
			return 0, 0, 0, p.fail(current, "%s", err)
		}
	}
	p.want("`)`")
	if !p.punct(")") {
		return 0, 0, 0, errShape
	}
	return values[0], values[1], values[2], nil
}

// digitGroup is the number of digits a quantity separates on.
const digitGroup = 3

func isDigits(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// formatQuantity writes a whole number the way a player writes a quantity,
// with every three digits separated by a comma.
func formatQuantity(value int64) string {
	digits := strconv.FormatInt(value, 10)
	var out strings.Builder
	for i, r := range digits {
		if i != 0 && (len(digits)-i)%digitGroup == 0 {
			out.WriteByte(',')
		}
		out.WriteRune(r)
	}
	return out.String()
}

// mustParse is only ever handed digits that came out of a token, and is only
// used to spell a number back at the player who mistyped it. A number too
// large to parse is spelled back as it was written.
func mustParse(digits string) int64 {
	value, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0
	}
	return value
}

// quantity consumes a quantity: a whole number greater than zero, written with
// a comma between every three digits once it passes 999.
//
// The separator is the same character the lists of units use, so reading one
// takes two tokens of lookahead: a comma continues the number when exactly
// three digits follow it, and separates the next item otherwise. Nothing is
// ambiguous, because a quantity is always followed by a unit code and never by
// another quantity.
func (p *Parser) quantity(what string) (int64, error) {
	current, ok := p.word(article(what))
	if !ok {
		return 0, errShape
	}
	if !isDigits(current.text) {
		return 0, p.fail(current, "invalid %s %q", bare(what), current.text)
	}
	if current.text[0] == '0' {
		return 0, p.fail(current, "invalid %s %q; a quantity is greater than zero and carries no leading zero",
			bare(what), current.text)
	}
	digits := current.text
	for {
		separator, hasSeparator := p.peekAt(0)
		group, hasGroup := p.peekAt(1)
		if !hasSeparator || !hasGroup || separator.quoted || separator.text != "," ||
			group.quoted || len(group.text) != digitGroup || !isDigits(group.text) {
			break
		}
		p.pos += 2
		digits += group.text
	}
	if len(digits) > digitGroup && len(current.text) > digitGroup {
		return 0, p.fail(current,
			"invalid %s %q; a quantity over 999 separates every three digits with a comma, as in %s",
			bare(what), digits, formatQuantity(mustParse(digits)))
	}
	value, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, p.fail(current, "%s is too large", article(what))
	}
	if value < 1 {
		return 0, p.fail(current, "invalid %s %q; a quantity is greater than zero", bare(what), digits)
	}
	return value, nil
}

// longestTag is longer than any unit tag the game has: four letters, a
// hyphen, and a level.
const longestTag = 12

// unitTag consumes a unit code with an optional technology level, such as GOLD
// or LFSU-7. It is upper-cased, the way a system letter is, so a player who
// types in lower case reads their own order back in the game's spelling.
func (p *Parser) unitTag() (string, error) {
	current, ok := p.word("a unit code")
	if !ok {
		return "", errShape
	}
	tag := strings.ToUpper(current.text)
	// A unit code is four letters and at most a two-digit level, so anything
	// longer is not one and is refused here rather than by units.ParseTag.
	// That is not a second copy of the rule: it is a length the message cares
	// about and the rule does not, because ParseTag quotes the whole tag back
	// and the tag may be most of an 8 KB line.
	if len([]rune(tag)) > longestTag {
		return "", p.fail(current, "invalid unit tag %q", elide(tag, longestFound))
	}
	if _, _, _, err := units.ParseTag(tag); err != nil {
		return "", p.fail(current, "%s", err)
	}
	return tag, nil
}

// unitList consumes one or more quantities of units, separated by commas:
// `4,500 GOLD, 18,000 FOOD`. Every order that names units names them this way.
func (p *Parser) unitList() ([]UnitQuantity, error) {
	var items []UnitQuantity
	for {
		quantity, err := p.quantity("a quantity")
		if err != nil {
			return nil, err
		}
		tag, err := p.unitTag()
		if err != nil {
			return nil, err
		}
		items = append(items, UnitQuantity{Quantity: quantity, Tag: tag})
		if _, ok := p.keyword(","); !ok {
			return items, nil
		}
	}
}

// orbitList consumes one or more orbits. A probe spends one probe on each.
func (p *Parser) orbitList() ([]int, error) {
	var orbits []int
	for p.more() {
		orbit, err := p.number("orbit")
		if err != nil {
			return nil, err
		}
		orbits = append(orbits, orbit)
	}
	if len(orbits) == 0 {
		p.want("at least one orbit")
		return nil, errShape
	}
	return orbits, nil
}

// percentage consumes an integer percentage: digits with a % on the end, as in
// 75%.
//
// The % carries no space before it, which is a decision rather than an accident
// of the tokenizer: % is not punctuation the lexer splits on, so 75% is one
// token and 75 % is two, and the second is refused. docs/accepted-orders.md
// says so where it defines a commission.
//
// Four fields of the accepted grammar are this shape and differ only in their
// range: a commitment and a commission run 1 to 100, and a pay rate and a
// ration rate are any percentage at all, a colony being free to overpay or
// overfeed. The range is the caller's, which is why it is a parameter.
func (p *Parser) percentage(what string, low, high int) (int, error) {
	current, ok := p.word(article(what))
	if !ok {
		return 0, errShape
	}
	digits, found := strings.CutSuffix(current.text, "%")
	if !found || !isDigits(digits) {
		return 0, p.fail(current, "invalid %s %q; a percentage is digits and a %% sign, as in 75%%",
			bare(what), current.text)
	}
	value, err := wholeNumber(digits, what)
	if err != nil {
		return 0, p.fail(current, "%s", err)
	}
	if value < low || value > high {
		return 0, p.fail(current, "invalid %s %d%%; it runs from %d%% to %d%%", bare(what), value, low, high)
	}
	return value, nil
}

// techLevelPrefix is how a market order names a technology level, TL-4.
const techLevelPrefix = "TL-"

// techLevel consumes a technology level written TL and the level, as in TL-4.
// It is the market's way of naming one, and it is not a unit tag: a technology
// level is bought once rather than by quantity.
func (p *Parser) techLevel() (int, error) {
	current, ok := p.word("a technology level")
	if !ok {
		return 0, errShape
	}
	digits, found := strings.CutPrefix(strings.ToUpper(current.text), techLevelPrefix)
	if !found || !isDigits(digits) {
		return 0, p.fail(current, "invalid technology level %q; it is written TL and the level, as in TL-4",
			current.text)
	}
	value, err := wholeNumber(digits, "a technology level")
	if err != nil {
		return 0, p.fail(current, "%s", err)
	}
	if value < 1 || value > 10 {
		return 0, p.fail(current, "invalid technology level %d; a level runs from 1 to 10", value)
	}
	return value, nil
}

// The two currencies a market order may be priced in. A technology level is
// paid for in GOLD and never in CNGD.
const (
	CurrencyGold  = "GOLD"
	CurrencyGoods = "CNGD"
)

// Price is an amount of one of the two currencies.
//
// The amount is kept as the player wrote it rather than converted to a number.
// The shape is checked -- digits, the thousands separators a quantity uses, at
// most one decimal point, and at least one digit before it -- but nothing
// prices anything yet, and choosing a scale to store it in would be inventing a
// rule the market has not been given.
type Price struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

// String renders a price the way the player wrote it.
func (p Price) String() string { return p.Amount + " " + p.Currency }

// price consumes an amount and the currency it is in: a positive number with
// the same thousands separators a quantity uses, then GOLD or CNGD.
//
// It reads the separators the way quantity does, with two tokens of lookahead,
// because the comma that groups a number is the same character that separates
// the items of a list. The one difference is that the last group may carry a
// decimal part -- 25,600.50 -- so a group that does ends the number.
//
// whole says the price must be a round number, which is what a technology level
// is bought for.
func (p *Parser) price(what string, whole bool) (Price, error) {
	current, ok := p.word(article(what))
	if !ok {
		return Price{}, errShape
	}
	digits, decimal, err := priceGroup(current.text, what)
	if err != nil {
		return Price{}, p.fail(current, "%s", err)
	}
	leading := len(current.text)
	for !decimal {
		separator, hasSeparator := p.peekAt(0)
		if !hasSeparator || separator.quoted || separator.text != "," {
			break
		}
		// A comma here is grouping a number rather than separating a list. It
		// cannot be doing anything else: a price is always followed by its
		// currency, so nothing but digits can stand between it and the amount.
		// That is what lets a malformed group be reported as one -- a quantity
		// has to guess, because the token after its comma may belong to the
		// next item of a list.
		group, hasGroup := p.peekAt(1)
		malformed := func() error {
			at := separator
			if hasGroup {
				at = group
			}
			return p.fail(at, "invalid %s %q; a comma in a number groups exactly three digits",
				bare(what), digits+","+groupText(group, hasGroup))
		}
		if !hasGroup || group.quoted {
			return Price{}, malformed()
		}
		next, hasDecimal, err := priceGroup(group.text, what)
		// Only the last group may carry the decimal part, which is what ends
		// the number.
		if integer, _, _ := strings.Cut(next, "."); err != nil || len(integer) != digitGroup {
			return Price{}, malformed()
		}
		p.pos += 2
		digits, decimal = digits+next, hasDecimal
	}
	before, after, _ := strings.Cut(digits, ".")
	// A leading zero is refused, as it is in a quantity, but the rule cannot be
	// "no zero first": a price needs one before the decimal point, so 0.1 is
	// written exactly that way. One zero is the whole of the integer part or it
	// is a mistake.
	if len(before) > 1 && before[0] == '0' {
		return Price{}, p.fail(current, "invalid %s %q; a number carries no leading zero", bare(what), digits)
	}
	if len(before) > digitGroup && leading > digitGroup {
		return Price{}, p.fail(current,
			"invalid %s %q; a number over 999 separates every three digits with a comma", bare(what), digits)
	}
	if whole && after != "" {
		return Price{}, p.fail(current, "invalid %s %q; it is paid in whole units", bare(what), digits)
	}
	if strings.Trim(before+after, "0") == "" {
		return Price{}, p.fail(current, "invalid %s %q; a price is greater than zero", bare(what), digits)
	}
	p.want("`" + CurrencyGold + "`")
	p.want("`" + CurrencyGoods + "`")
	currency, ok := p.keyword(CurrencyGold, CurrencyGoods)
	if !ok {
		return Price{}, p.here("invalid %s; a price is paid in %s or %s", bare(what), CurrencyGold, CurrencyGoods)
	}
	if whole && currency != CurrencyGold {
		return Price{}, p.at(p.pos-1, "invalid %s; a technology level is paid for in %s", bare(what), CurrencyGold)
	}
	return Price{Amount: formatPrice(before, after), Currency: currency}, nil
}

// priceGroup reads one run of digits of a price, which may carry the decimal
// part on its end.
func priceGroup(text, what string) (digits string, decimal bool, err error) {
	before, after, found := strings.Cut(text, ".")
	if !isDigits(before) || (found && !isDigits(after)) {
		return "", false, fmt.Errorf("invalid %s %q; a price is a number and a currency, as in 1.0 %s",
			bare(what), text, CurrencyGold)
	}
	if found {
		return before + "." + after, true, nil
	}
	return before, false, nil
}

// groupText is the group a malformed number stopped on, for the message about
// it. A comma with nothing after it stopped on nothing.
func groupText(group token, found bool) string {
	if !found {
		return ""
	}
	return group.text
}

// formatPrice writes a price back the way a player writes one, with the
// thousands separators put back in.
func formatPrice(before, after string) string {
	value := formatQuantity(mustParse(before))
	if after == "" {
		return value
	}
	return value + "." + after
}
