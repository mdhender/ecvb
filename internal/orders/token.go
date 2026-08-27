// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mdhender/ecvb/internal/units"
)

// syntaxErr marks a line that never matched the shape of its order, as
// opposed to a field that was read and found wrong. The first is answered with
// the verb's syntax, which is more use than naming the token that failed; the
// second already says what is wrong and survives unchanged.
type syntaxErr struct{ message string }

func (e syntaxErr) Error() string { return e.message }

func badSyntax(format string, args ...any) error {
	return syntaxErr{message: fmt.Sprintf(format, args...)}
}

func isSyntaxError(err error) bool {
	_, ok := errors.AsType[syntaxErr](err)
	return ok
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

// token is one word of an order line. Quoted tokens are tracked separately
// because a game code or an email address may hold characters that would
// otherwise split a token, and because a `#` inside quotes is not a comment.
type token struct {
	text   string
	quoted bool
}

// punctuation characters that stand alone as tokens, so that a jump may be
// written `to (6,-9,8)` or `to ( 6 , -9 , 8 )` and read the same either way.
const punctuation = "(),"

// tokenize splits one line into tokens, dropping any trailing comment. A `#`
// outside quotes begins a comment that runs to the end of the line.
//
// A quote that is never closed is the one thing a line can get wrong before any
// order is read from it, and it is refused here rather than passed on. Reading
// to the end of the line and calling it a token is worse than it sounds: a name
// is quoted text, so `ship 18 name "Jalopy` would name the ship and say
// nothing, and the player would find out from a report a turn later.
func tokenize(text string) ([]token, error) {
	var tokens []token
	runes := []rune(text)
	for i := 0; i < len(runes); {
		switch c := runes[i]; {
		case c == '#':
			return tokens, nil
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '"':
			i++
			start := i
			for i < len(runes) && runes[i] != '"' {
				i++
			}
			if i == len(runes) {
				return nil, fmt.Errorf("unterminated quoted text %q; a quote is closed on the line it opens",
					string(runes[start:i]))
			}
			tokens = append(tokens, token{text: string(runes[start:i]), quoted: true})
			i++ // the closing quote
		case strings.ContainsRune(punctuation, c):
			tokens = append(tokens, token{text: string(c)})
			i++
		default:
			start := i
			for i < len(runes) && !strings.ContainsRune(" \t\r\"#"+punctuation, runes[i]) {
				i++
			}
			tokens = append(tokens, token{text: string(runes[start:i])})
		}
	}
	return tokens, nil
}

// Line is one tokenized order line, read left to right. Every order's parser
// consumes from it, so the field parsers below are written once rather than
// once per order.
type Line struct {
	Number int
	tokens []token
	pos    int
	// fault is what was wrong with the line before any order was read from it.
	// Only an unterminated quote can be: everything else a line might get wrong
	// is a question about the order it names, and that is the order's to answer.
	fault error
}

func newLine(number int, text string) *Line {
	tokens, fault := tokenize(text)
	return &Line{Number: number, tokens: tokens, fault: fault}
}

// absorb adds another physical line's tokens to this one, for the orders that
// run to a terminator rather than to the end of a line. The Number stays the
// line the order began on, which is where a player looks for it.
func (l *Line) absorb(other *Line) {
	l.tokens = append(l.tokens, other.tokens...)
	// A fault on a line gathered into this one is this one's now: the order
	// began here, and here is where the player is told about it.
	if l.fault == nil {
		l.fault = other.fault
	}
}

// begins reports whether the line opens an order. Every order names its subject
// first, so the first word is the whole of the test.
//
// It is what tells a gather that it has run past the end of the order it was
// reading: a continuation of a create is a clause or a lot of units, and never
// a subject.
func (l *Line) begins() bool {
	word, ok := l.peek()
	if !ok || word.quoted {
		return false
	}
	for _, subject := range []string{SubjectShip, SubjectColony, SubjectFaction} {
		if strings.EqualFold(word.text, subject) {
			return true
		}
	}
	return false
}

// holds reports whether an unquoted keyword appears anywhere in the line. It is
// how the file scanner knows a multi-line order has reached its terminator
// without parsing the order to find out.
func (l *Line) holds(word string) bool {
	for _, item := range l.tokens {
		if !item.quoted && strings.EqualFold(item.text, word) {
			return true
		}
	}
	return false
}

// empty reports whether the line holds nothing but whitespace and comments.
func (l *Line) empty() bool { return len(l.tokens) == 0 }

func (l *Line) more() bool { return l.pos < len(l.tokens) }

func (l *Line) next() (token, bool) {
	if !l.more() {
		return token{}, false
	}
	l.pos++
	return l.tokens[l.pos-1], true
}

func (l *Line) peek() (token, bool) { return l.peekAt(0) }

// peekAt looks ahead without consuming. A quantity needs two tokens of
// lookahead, because whether a comma continues the number or separates the
// next item is decided by what follows it.
func (l *Line) peekAt(offset int) (token, bool) {
	if l.pos+offset >= len(l.tokens) {
		return token{}, false
	}
	return l.tokens[l.pos+offset], true
}

// mark saves where the line is being read from and hands back what puts it
// there again.
//
// It is what a lookahead is written with. Finding out what a line says means
// reading it, and reading it moves the cursor; a reader that has to leave the
// line untouched takes a mark first and restores it on the way out, so the
// parser that comes after cannot tell the lookahead happened.
func (l *Line) mark() (restore func()) {
	pos := l.pos
	return func() { l.pos = pos }
}

// keyword consumes the next token when it matches one of the given words,
// ignoring case, and reports which one. Keywords are always unquoted.
func (l *Line) keyword(words ...string) (string, bool) {
	current, ok := l.peek()
	if !ok || current.quoted {
		return "", false
	}
	for _, word := range words {
		if strings.EqualFold(current.text, word) {
			l.pos++
			return word, true
		}
	}
	return "", false
}

// expect consumes a required keyword.
func (l *Line) expect(word string) error {
	if _, ok := l.keyword(word); !ok {
		return badSyntax("expected %s", word)
	}
	return nil
}

// end reports an error when anything is left over, so a trailing word is a
// mistake rather than something quietly ignored.
func (l *Line) end() error {
	if l.more() {
		return badSyntax("unexpected %q", l.rest())
	}
	return nil
}

func (l *Line) rest() string {
	words := make([]string, 0, len(l.tokens)-l.pos)
	for _, item := range l.tokens[l.pos:] {
		words = append(words, item.text)
	}
	return strings.Join(words, " ")
}

// quoted consumes a quoted string, such as a game code or an email address.
func (l *Line) quoted(what string) (string, error) {
	current, ok := l.next()
	if !ok || !current.quoted {
		return "", badSyntax("expected a quoted %s", what)
	}
	return current.text, nil
}

// entityID consumes a positive entity id.
func (l *Line) entityID(kind string) (int64, error) {
	current, ok := l.next()
	if !ok || current.quoted {
		return 0, badSyntax("expected a %s id", kind)
	}
	id, err := strconv.ParseInt(current.text, 10, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return 0, fmt.Errorf("invalid %s id: number is too large", kind)
		}
		return 0, fmt.Errorf("invalid %s id: %q is not a number", kind, current.text)
	}
	if id < 1 {
		return 0, fmt.Errorf("invalid %s id: must be positive", kind)
	}
	return id, nil
}

// number consumes a nonnegative whole number, such as a turn or an orbit.
func (l *Line) number(what string) (int, error) {
	current, ok := l.next()
	if !ok || current.quoted {
		return 0, badSyntax("expected %s", what)
	}
	return wholeNumber(current.text, what)
}

// systemLetter consumes a system's letter, A through E.
func (l *Line) systemLetter() (string, error) {
	current, ok := l.next()
	if !ok || current.quoted || len(current.text) != 1 {
		return "", badSyntax("expected a system letter")
	}
	letter := strings.ToUpper(current.text)
	if letter < "A" || letter > "E" {
		return "", fmt.Errorf("invalid system %q; systems are A through E", current.text)
	}
	return letter, nil
}

// coordinates consumes a bracketed point, `(X,Y,Z)`, with any spacing.
func (l *Line) coordinates() (x, y, z int, err error) {
	if err := l.expect("("); err != nil {
		return 0, 0, 0, badSyntax("expected (X,Y,Z)")
	}
	values := [3]int{}
	for i := range values {
		if i != 0 {
			if err := l.expect(","); err != nil {
				return 0, 0, 0, badSyntax("expected (X,Y,Z)")
			}
		}
		current, ok := l.next()
		if !ok || current.quoted {
			return 0, 0, 0, badSyntax("expected (X,Y,Z)")
		}
		if values[i], err = wholeNumber(current.text, "coordinate"); err != nil {
			return 0, 0, 0, err
		}
	}
	if err := l.expect(")"); err != nil {
		return 0, 0, 0, badSyntax("expected (X,Y,Z)")
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

// quantity consumes a quantity: a whole number greater than zero, written with
// a comma between every three digits once it passes 999.
//
// The separator is the same character the lists of units use, so reading one
// takes two tokens of lookahead: a comma continues the number when exactly
// three digits follow it, and separates the next item otherwise. Nothing is
// ambiguous, because a quantity is always followed by a unit code and never by
// another quantity.
func (l *Line) quantity(what string) (int64, error) {
	current, ok := l.next()
	if !ok || current.quoted {
		return 0, badSyntax("expected %s", what)
	}
	if !isDigits(current.text) {
		return 0, fmt.Errorf("invalid %s %q", what, current.text)
	}
	if current.text[0] == '0' {
		return 0, fmt.Errorf("invalid %s %q; a quantity is greater than zero and carries no leading zero",
			what, current.text)
	}
	digits := current.text
	for {
		separator, hasSeparator := l.peekAt(0)
		group, hasGroup := l.peekAt(1)
		if !hasSeparator || !hasGroup || separator.quoted || separator.text != "," ||
			group.quoted || len(group.text) != digitGroup || !isDigits(group.text) {
			break
		}
		l.pos += 2
		digits += group.text
	}
	if len(digits) > digitGroup && len(current.text) > digitGroup {
		return 0, fmt.Errorf("invalid %s %q; a quantity over 999 separates every three digits with a comma, as in %s",
			what, digits, formatQuantity(mustParse(digits)))
	}
	value, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s is too large", what)
	}
	if value < 1 {
		return 0, fmt.Errorf("invalid %s %q; a quantity is greater than zero", what, digits)
	}
	return value, nil
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

// unitTag consumes a unit code with an optional technology level, such as GOLD
// or LFSU-7. It is upper-cased, the way a system letter is, so a player who
// types in lower case reads their own order back in the game's spelling.
func (l *Line) unitTag() (string, error) {
	current, ok := l.next()
	if !ok || current.quoted {
		return "", badSyntax("expected a unit code")
	}
	tag := strings.ToUpper(current.text)
	if _, _, _, err := units.ParseTag(tag); err != nil {
		return "", err
	}
	return tag, nil
}

// unitList consumes one or more quantities of units, separated by commas:
// `4,500 GOLD, 18,000 FOOD`. Every order that names units names them this way.
func (l *Line) unitList() ([]UnitQuantity, error) {
	var items []UnitQuantity
	for {
		quantity, err := l.quantity("a quantity")
		if err != nil {
			return nil, err
		}
		tag, err := l.unitTag()
		if err != nil {
			return nil, err
		}
		items = append(items, UnitQuantity{Quantity: quantity, Tag: tag})
		if _, ok := l.keyword(","); !ok {
			return items, nil
		}
	}
}

// orbitList consumes one or more orbits. A probe spends one probe on each.
func (l *Line) orbitList() ([]int, error) {
	var orbits []int
	for l.more() {
		orbit, err := l.number("orbit")
		if err != nil {
			return nil, err
		}
		orbits = append(orbits, orbit)
	}
	if len(orbits) == 0 {
		return nil, badSyntax("expected at least one orbit")
	}
	return orbits, nil
}

// percentage consumes an integer percentage: digits with a % on the end, as in
// 75%.
//
// The % carries no space before it, which is a decision rather than an accident
// of the tokenizer: % is not punctuation the tokenizer splits on, so 75% is one
// token and 75 % is two, and the second is refused. docs/accepted-orders.md
// says so where it defines a commission.
//
// Four fields of the accepted grammar are this shape and differ only in their
// range: a commitment and a commission run 1 to 100, and a pay rate and a
// ration rate are any percentage at all, a colony being free to overpay or
// overfeed. The range is the caller's, which is why it is a parameter.
func (l *Line) percentage(what string, low, high int) (int, error) {
	current, ok := l.next()
	if !ok || current.quoted {
		return 0, badSyntax("expected %s", what)
	}
	digits, found := strings.CutSuffix(current.text, "%")
	if !found || !isDigits(digits) {
		return 0, fmt.Errorf("invalid %s %q; a percentage is digits and a %% sign, as in 75%%", what, current.text)
	}
	value, err := wholeNumber(digits, what)
	if err != nil {
		return 0, err
	}
	if value < low || value > high {
		return 0, fmt.Errorf("invalid %s %d%%; it runs from %d%% to %d%%", what, value, low, high)
	}
	return value, nil
}

// TechLevel is the technology level a market order names, written TL-4.
const techLevelPrefix = "TL-"

// techLevel consumes a technology level written TL and the level, as in TL-4.
// It is the market's way of naming one, and it is not a unit tag: a technology
// level is bought once rather than by quantity.
func (l *Line) techLevel() (int, error) {
	current, ok := l.next()
	if !ok || current.quoted {
		return 0, badSyntax("expected a technology level")
	}
	digits, found := strings.CutPrefix(strings.ToUpper(current.text), techLevelPrefix)
	if !found || !isDigits(digits) {
		return 0, fmt.Errorf("invalid technology level %q; it is written TL and the level, as in TL-4", current.text)
	}
	value, err := wholeNumber(digits, "a technology level")
	if err != nil {
		return 0, err
	}
	if value < 1 || value > 10 {
		return 0, fmt.Errorf("invalid technology level %d; a level runs from 1 to 10", value)
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
func (l *Line) price(what string, whole bool) (Price, error) {
	current, ok := l.next()
	if !ok || current.quoted {
		return Price{}, badSyntax("expected %s", what)
	}
	digits, decimal, err := priceGroup(current.text, what)
	if err != nil {
		return Price{}, err
	}
	leading := len(current.text)
	for !decimal {
		separator, hasSeparator := l.peekAt(0)
		group, hasGroup := l.peekAt(1)
		if !hasSeparator || !hasGroup || separator.quoted || separator.text != "," || group.quoted {
			break
		}
		next, hasDecimal, err := priceGroup(group.text, what)
		// A group is exactly three digits, and only the last of them may carry
		// the decimal part. Anything else is the comma that separates the items
		// of a list rather than one that groups a number.
		if integer, _, _ := strings.Cut(next, "."); err != nil || len(integer) != digitGroup {
			break
		}
		l.pos += 2
		digits, decimal = digits+next, hasDecimal
	}
	before, after, _ := strings.Cut(digits, ".")
	if len(before) > digitGroup && leading > digitGroup {
		return Price{}, fmt.Errorf("invalid %s %q; a number over 999 separates every three digits with a comma", what, digits)
	}
	if whole && after != "" {
		return Price{}, fmt.Errorf("invalid %s %q; it is paid in whole units", what, digits)
	}
	if strings.Trim(before+after, "0") == "" {
		return Price{}, fmt.Errorf("invalid %s %q; a price is greater than zero", what, digits)
	}
	currency, ok := l.keyword(CurrencyGold, CurrencyGoods)
	if !ok {
		return Price{}, fmt.Errorf("invalid %s; a price is paid in %s or %s", what, CurrencyGold, CurrencyGoods)
	}
	if whole && currency != CurrencyGold {
		return Price{}, fmt.Errorf("invalid %s; a technology level is paid for in %s", what, CurrencyGold)
	}
	return Price{Amount: formatPrice(before, after), Currency: currency}, nil
}

// priceGroup reads one run of digits of a price, which may carry the decimal
// part on its end.
func priceGroup(text, what string) (digits string, decimal bool, err error) {
	before, after, found := strings.Cut(text, ".")
	if !isDigits(before) || (found && !isDigits(after)) {
		return "", false, fmt.Errorf("invalid %s %q; a price is a number and a currency, as in 1.0 %s",
			what, text, CurrencyGold)
	}
	if found {
		return before + "." + after, true, nil
	}
	return before, false, nil
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
