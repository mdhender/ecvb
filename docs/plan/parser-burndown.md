# Parser quality burndown

This plan records the findings from comparing `internal/orders` on the
`parser-recursive-descent` branch with the local `main` branch, and the
findings since from the fuzzing mini-game in `games/fuzzers`. Work is ordered
by severity. The parser already provides substantially better syntax
diagnostics than `main`; the remaining work is to make that quality consistent
and remove smaller readability defects.

Items marked **done** are closed. The fuzzers are pointed at this file so that
a round of the game does not re-report what is already written down here; a
finding that is listed, fixed or not, is not a new finding.

## Medium: preserve source locations for semantic parse errors

**Done.** The four named paths now report against the token they are about --
the unit that took a raid past two, the group keyword whose subject cannot own
it, the first unit of a list that should have held one, the class word that is
not draftable. `Parser.place`, called from `parseOrder`, is the guard: an error
from an order parser that is neither `errShape` nor a diagnostic is given the
line the order began on, so a parser written later cannot bypass the format by
returning a plain error. `TestAParseErrorAlwaysCarriesAtLeastItsLine` covers the
guard, the four paths, and that `place` leaves `errShape` alone.

### Issue

The new error collector stores parser errors directly. Errors returned as a
`diagnostic` include a line, column, source excerpt, and caret, but plain errors
returned by order parsers do not. On `main`, every order error was at least
wrapped with the order's line number.

Known parse-time paths that currently return an unplaced `fmt.Errorf` include:

- a RAID that seeks more than two units;
- a group order given to a subject that cannot own that group;
- a farm or mine group order that names more than one unit; and
- a DRAFT or DISBAND order that names an ineligible unit class.

The existing tests assert only message substrings for these cases, so they do
not detect the missing location. This also exposes a maintenance hazard: a new
order parser can return a plain error that compiles and behaves correctly while
silently bypassing the diagnostic format.

### Acceptance criteria

- Every error originating while parsing a header or order identifies at least
  its physical line.
- Errors associated with a specific token identify its line and column and
  render the source excerpt and caret used by other parser diagnostics.
- The four known paths above return placed diagnostics.
- Focused tests assert location information, not only message substrings, for
  each known path.
- A focused test proves that a future non-diagnostic error returned by an order
  parser cannot reach the caller without at least an order-line location.
- Existing accepted-order behavior and diagnostic aggregation remain
  unchanged.

## Medium: a long token makes an unreadable diagnostic

**Done.** Found by the fuzzing game, faction 5, and accepted.

`diagnostic.window` echoes a run of `echoWidth` columns around the caret rather
than the whole line, marking each end it cut with `...`, and moves the caret to
keep its place under the same text. The caret run is cut with it and never
reaches under the elision marks. The message text elides what it quotes at
`longestFound`, and so do the two places that quote a token through another
package's error -- `wholeNumber`, and `unitTag`, which refuses a tag longer
than any the game has rather than let `units.ParseTag` quote 8 KB back.
`TestALongLineIsShownAsAWindowAroundTheCaret` covers the problem at the start,
the middle, and the end of a long line; `TestAMessageElidesALongToken` covers
the headline.

### Issue

The source line is echoed whole and the caret run is drawn the full width of
the offending token. A 900-character token therefore produces a 983-character
message line with 900 carets beneath it, and an order line may be 8 KB. Nothing
in the message is untrue, which is why it is not a correctness defect, but no
one can read it and it buries the other diagnostics in the same report.

### Acceptance criteria

- A diagnostic whose source line exceeds a fixed display width echoes a window
  around the caret rather than the whole line, marked at each end it was cut
  so a reader knows text was dropped.
- The caret stays under the same text it was under before the window was
  applied.
- The caret run is capped at the display width, and a token wider than the
  window is marked as continuing rather than drawn to its full length.
- `found ...` in the message text is elided at the same limit, so the headline
  does not carry the 8 KB the echo just avoided.
- A short line is untouched: existing diagnostics render exactly as they do
  now, and the tests that assert whole rendered blocks do not change.
- Tests cover a line longer than the window with the problem at its start, in
  its middle, and at its end.

## Medium: a list separator is not distinguished from a digit group

### Issue

A comma does two jobs and nothing tells them apart. It groups the digits of a
quantity above 999, and it separates the items of a list. The parser accepts a
list separator with no whitespace after it, so a line can be read two ways and
is read one of them silently:

```text
ship 18 stow 5,000 FOOD,5,000 METL
```

That is accepted today. So is `ship 18 stow 800 HDRV-1,500 FOOD`. Nothing in a
diagnostic tells the player which reading was taken, because there is no
diagnostic: the file parses.

`docs/orders.md` claims the ambiguity cannot arise -- "a quantity is always
followed by a unit code and never by another quantity" -- which is true of the
grammar and not of the reading. It settles what the parser does; it does not
give the player a rule they can apply by eye.

### Acceptance criteria

- A comma that separates the items of a list must be followed by whitespace: a
  space, or the end of a physical line inside a gathered order.
- A comma with no whitespace after it is a digit group separator and nothing
  else, so `5,000` is one quantity and `HDRV-1,500 FOOD` is a diagnostic rather
  than two items.
- The diagnostic points at the comma and says a list separator needs a space
  after it.
- `docs/orders.md` and `docs/reference-manual.md` state the rule where they
  describe quantities.
- Tests cover the ambiguous pair above in both spellings, and a quantity above
  999 in a list, which must keep parsing as it does now.

## Low: correct quoted-field wording

**Done.** `article` gained a counterpart `bare` in `fields.go`, and each field
reader now uses whichever spelling its sentence wants, so a reader is right
whether its caller passes `price` or `a price`. This closed the BROADCAST case
below and, at the same time, the `invalid a price` family that the fuzzing
game's faction 7 found -- `invalid a price "1."`, `invalid a quantity ","`,
`invalid a commitment "-5%"` -- which was the same defect read the other way.
`TestAFieldIsNamedWithoutAnArticleWhenItIsInvalid` holds both directions.

### Issue

The shared quoted-field reader prefixes its description with `a quoted`.
Most callers pass a bare noun, but BROADCAST passes `a message` and
`a signature`. Missing values therefore produce phrases such as
`expected a quoted a message` and `expected a quoted a signature`.

### Acceptance criteria

- A BROADCAST missing its message reports `expected a quoted message`.
- A BROADCAST with an invalid optional signature reports
  `expected a quoted signature`.
- Tests assert the complete primary diagnostic text for both cases.
- Existing wording for game codes, email addresses, and names remains correct.

## Low: a control character breaks the caret alignment

**Done.** Found by the fuzzing game, faction 5.

### Issue

`showable` replaced tabs and carriage returns and passed every other rune
through, so a control character reached the terminal raw. A terminal gives it
no width, so the echoed line came out shorter than the caret run beneath it and
the caret pointed past the end of a line that looked perfectly ordinary:

```text
line 4, column 16: expected `system` or `orbit`, found "orbit\x7f6"
  4 | ship 2 move to orbit6      <- the DEL is here, and invisible
    |                ^^^^^^^     <- seven carets under six columns
```

### Resolution

`showable` now renders every non-graphic rune as `?`, one column for one
column, so the echo keeps its place. Tab and carriage return stay a space,
being whitespace. The message text still names the byte exactly, being written
with `%q`. `TestAControlCharacterKeepsItsColumnInTheEcho` asserts the rendered
block and, for a set of lines, that the caret never runs past the echo and that
the echo carries nothing a terminal gives no width to.

## Low: repair inaccurate parser comments

**Done.** Four comments in `spec.go` and `verbs.go` said "the p" where the
mechanical rename had eaten "line". `Subject`, `Spec.Verb`, and
`Spec.Terminator` now say order, physical line, or input as each means, and a
search finds no remaining prose from that replacement.

### Issue

Mechanical `Line`-to-`Parser` renaming left comments in `spec.go` that refer to
the front of “the p,” a verb opening “the p,” and an order being “always one
p.” These comments obscure otherwise clear API documentation.

### Acceptance criteria

- Comments on `Subject`, `Spec.Verb`, and `Spec.Terminator` use domain terms
  such as order, input, or physical line rather than the parameter name `p`.
- Comments accurately distinguish a physical line from a potentially
  multi-line order.
- A search of parser documentation finds no remaining prose produced by the
  same mechanical replacement.
- Comment-only cleanup does not change executable code or generated output.

## Low: trim duplicated parser rationale

**Done.** The furthest-failure rule is explained once, on the `Parser.furthest`
field that holds it, and the file header and `want` now point at it instead of
restating it. The same for token positioning (`lex.go`'s header, with the
`token` fields saying only what they are) and for the shape-versus-value split,
which is now one comment over `errShape` rather than three. Dead code went with
it: `Parser.rest` had had no caller since `end` started recording an
expectation instead of quoting what was left over. 1,701 lines to 1,664, with
the four invariants the item names still documented.

### Issue

Splitting lexing, diagnostics, cursor state, field productions, and file
traversal into focused files improves structural readability. However, the core
parsing infrastructure grew from 974 lines on `main` to 1,547 lines, with some
design rationale repeated across file headers, type comments, and function
comments. The repetition makes the control flow harder to scan and increases
the cost of keeping documentation accurate.

This is not a request to remove explanations of non-obvious behavior. In
particular, the furthest-failure rule, multiline recovery, token positioning,
and shape-versus-value error distinction need documentation.

### Acceptance criteria

- Each non-obvious parser invariant has one primary explanation close to the
  code that enforces it.
- Other comments refer briefly to that invariant instead of restating its full
  rationale.
- Comments describing straightforward mechanics are removed when names and
  control flow already communicate the same fact.
- Documentation for furthest-failure selection, multiline recovery, token
  positioning, and shape-versus-value errors remains present and accurate.
- Parser behavior and test results remain unchanged.

## Low: a report did not read down the file

**Done.** Found by the fuzzing game, faction 6, from inside a jump.

### Issue

The parser finds its problems in file order, but `Bind`'s arrive in the order
the turn resolves -- every order of one phase before any of the next. A file
whose orders were all refused therefore reported its lines scrambled: for a
faction that gave twenty-two orders to a ship in transit, the report opened at
line 23 and reached line 3 fifteenth. Nothing in it was untrue and none of it
could be read, because a player fixing a file reads down it.

### Resolution

`problems.inFileOrder` sorts by line wherever a report is built -- in `Parse`
and at both of `simulate`'s exits. The sort is stable, so two problems on one
line keep the order they were found in, and a problem that names no line at all
sorts to the top. `TestAReportReadsDownTheFile` gives three orders whose file
order is the reverse of their phase order and asserts the report is sorted.

## Low: an in-transit order named the wrong turn

**Done.** Found by the fuzzing game, faction 6, round 2, from inside a
crossing that landed the turn it was refused.

### Issue

A ship crossing between stellia was refused with

```text
ship 20 is in transit and arrives on turn 5; it can be given no orders until then
```

and that file was turn 5's. Arrivals resolve at the last phase that touches a
ship, after every phase that carries an order, so a ship due this turn is out
of reach for the whole of it and can first be ordered on the turn after. The
guard's own comment said exactly that; its message named the wrong turn, and a
player reading `until then` on turn 5 would take it for permission.

### Resolution

Both messages now name the turn orders resume rather than the turn the ship
lands: `is in transit; it arrives on turn 5 and can be given orders from turn
6`, and for a transfer aimed at one, `nothing can reach it before turn 6`.
`TestSubmitRefusesASecondJumpWhileTheShipIsCrossing` and the engine's
equivalent assert the new wording.

## Low: a comma may float away from the item it separates

### Issue

A list separator may be preceded by whitespace or by a line break, so a comma
can sit apart from the item it follows and still be accepted:

```text
ship 18 stow 800 HDRV-1 , 500 FOOD
```

Inside a gathered order the same latitude lets a continuation line open with a
comma, which reads to a player as an extra one. The CREATE example in
`docs/orders.md` is written that way and parses:

```text
ship 18 create ship
  using 60 STRC-8,
        61 HDRV-1, 5 SDRV-1
        , 5 LFSU-3, 1 SNSR-1
  transfering 25 FOOD, 5 SKW, 16,800 FUEL, 93 GOLD
  with 500 CWKR
end
```

Genuinely doubled commas are already refused -- `,,`, `, ,`, a trailing comma,
a comma opening the list, and a line ending in a comma followed by a line
opening with one all produce a diagnostic. What is missing is the rule that a
separator belongs to the item before it.

### Acceptance criteria

- A list separator attaches to the item it follows: no whitespace and no line
  break between the item and its comma.
- A comma that opens a physical line inside a gathered order is a diagnostic,
  pointing at the comma.
- The diagnostic distinguishes this from the doubled-comma cases, which keep
  their current wording.
- The CREATE examples in `docs/orders.md` and `docs/reference-manual.md` are
  rewritten to the accepted form. **Both are invalidated by this change**, so
  neither can be left as it stands.
- Tests cover a space before a comma, a comma opening a continuation line, and
  the corrected CREATE example.

## Low: a CREATE split before its kind reports six unrelated errors

### Issue

A CREATE is gathered by reading the word after the verb, which is what chooses
the terminator. The subject, the verb, and the kind must therefore be on one
physical line. That rule holds -- but nothing states it, and breaking it costs
one diagnostic per line of the order:

```text
ship 18 create
  ship
  using 60 STRC-8
  transfering 25 FOOD
  with 5 CWKR
end
```

produces six. The first is about the missing kind and is correct. The other
five are the clause lines and `end` being read as fresh orders -- `expected an
order to begin with ship, colony, or we; found "using"` -- which tells the
player nothing they can act on and buries the one message that would.

`resumeAt` already exists for this shape of problem: giving up on a multi-line
order skips to the next line that opens one. It does not fire here, because the
order never became a multi-line order -- the gather is what failed.

### Acceptance criteria

- A CREATE whose kind is not on the same physical line as its verb produces one
  diagnostic, pointing at the end of the opening line.
- The lines that would have been its body are not reported as orders of their
  own, and `end` is not reported at all.
- `docs/orders.md` and `docs/reference-manual.md` state that the subject, the
  verb, and the kind are one line, where they say line breaks inside a CREATE
  mean nothing.
- Tests cover the split above and the same order written correctly.

## Completion checks

- `gofmt` reports no changes to modified Go files.
- `git diff --check -- ':!games/fuzzers/orders'` passes. The fuzzing corpus is
  excluded on purpose: trailing whitespace is the attack in several dozen of
  its lines, and stripping it would defang them.
- `go test ./internal/orders` passes.
- `go test ./...` passes.
