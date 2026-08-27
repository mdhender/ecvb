# FUZZ-01 Game Rules

A game played against the parser rather than against the map. All ten player
factions attack `internal/orders`; none of them ever moves a ship.

It began as a side rule in [CLAUDE-01](../claude/game-rules.md), where three
factions were allowed to win by breaking the parser while seven searched for a
home. Both halves worked, so they were split: CLAUDE-01 went back to being a
search-and-capture game, and the attack became this one.

## Rules

1. A faction wins if an order file it submits makes the parser **panic**.
2. A faction wins if an order file it submits makes the parser return a
   diagnostic that is **misleading or unintelligible** -- one that points at
   the wrong place, contradicts itself, describes the input inaccurately, or is
   not a sentence a player can read.
3. A file may hold up to **24 lines** and **8 KB**. Nothing on those lines has
   to be an order, or text a player would ever write. Fuzzing is the point.
4. A finding already written down in
   [`docs/plan/parser-burndown.md`](../../docs/plan/parser-burndown.md) is not
   a finding. A faction that turns one up again has told us nothing, and the
   lines that produce it are retired.

The gamemaster judges rule 2, against what a player would do with the message.
A message that is merely terse, or that lists more of an order's forms than the
player needed, is not a win. A message whose caret does not land under the text
it is about, or that says `invalid a price`, is.

## The ten factions

Each attacks one seam, so a round is ten different questions rather than ten
copies of one. The seams are the parser's own files read as a target list.

| Faction | Seam | Where it lives |
| --- | --- | --- |
| 1 | the lexer and quoting | `lex.go` |
| 3 | numbers and quantities | `fields.go` |
| 4 | prices and currencies | `fields.go` |
| 5 | percentages and technology levels | `fields.go` |
| 6 | coordinates, and orders to a ship that is nowhere | `fields.go`, `world` |
| 7 | subjects, verbs, and other factions' entities | `file.go`, `verbs.go` |
| 8 | the multi-line create and the gather | `file.go` |
| 9 | unit codes and lists | `fields.go`, `verbs.go` |
| 10 | the header and the shape of a file | `file.go` |
| 11 | the caret and the columns | `diagnostic.go` |

A faction's pool is walked rather than sampled, so a long game covers a seam
instead of rolling the same handful of lines. The rest of each file is single
edits to lines that would otherwise have parsed, which is where a reader is
likeliest to index past something.

Two of the ten do more than write bad lines.

**Faction 6 really moves**, and it moves so that it can stop being anywhere. A
jump takes ceil(d / 10) turns and a ship is nowhere for every one of them --
`entity.stellium_id` is null, and no other state in the game is like that -- so
faction 6 launches a crossing it cannot finish quickly and then spends the
turns it is in transit giving the ship every order in the book. Those lines
parse, so they reach `Bind`, which is the half that has to answer for a ship
with no location. Nothing else in either game reaches it. The distances are
chosen against the tank: 28 light years first, then the 12 that are left, and
then one it cannot afford, because a jump that fails for want of fuel is worth
an answer too.

**Faction 7 gives orders to entities it does not own.** Ids are handed out four
at a time in faction order -- faction 1 holds 1, 2, and 4, faction 3 holds 5,
6, and 8, and the orbital colony of each group goes to the agent faction -- so
guessing another faction's ships and colonies takes no information at all. The
point is not that it might be allowed. It is that "this is not yours" is a
thing the pipeline has to say clearly about an id that is perfectly well
formed, and about one that is not an id at all.

## Playing a round

```sh
games/fuzzers/replay.sh [FIRST-TURN] [LAST-TURN]   # default 0 2
games/fuzzers/replay.sh 0 11                       # every round played so far
```

A round is three turns, so the committed corpus is four of them: turns 0-2,
3-5, 6-8, and 9-11. Ten factions times twenty-four lines times twelve turns is
about 2,800 attack lines.

Every file is expected to be refused, so a refusal is not an error. What the
run produces is `diagnostics/`, one file per order file, holding what the
parser said; that directory is not committed, because the findings worth
keeping belong in the burndown. A panic ends the game: the run says so and
exits non-zero.

The map is `games/claude`'s, copied in rather than committed twice. The
fuzzers never move, so which map it is does not matter -- the game exists only
to have something to submit an order file to.

## The corpus is not tidy, and must not be tidied

`git diff --check` reports trailing whitespace across `orders/`, and every
instance of it is deliberate. A line that is `ship ` and stops, a line that is
one tab, a mutation cut mid-token -- those are the attack. An editor set to
strip trailing whitespace on save, or a well-meant cleanup commit, would defang
several dozen lines of it without changing a single visible character.

Run the check against the source instead:

```sh
git diff --check -- ':!games/fuzzers/orders'
```

## Beside the game

`FuzzParse` in `internal/orders/fuzz_test.go` is the same question asked by
machine, and it is much better at rule 1 than a faction with 24 lines a turn:

```sh
go test ./internal/orders -run XXX -fuzz FuzzParse -fuzztime 150s
```

The game is better at rule 2, because a diagnostic has to be read to be judged
and only a person can say whether `invalid a price` is a sentence.

## Results

| Round | Turns | Panics | New findings |
| --- | --- | --- | --- |
| 1 | 0-2 | none | confirmed the two already in the burndown: parse errors that carry no line, and a long token whose echo no one can read. Faction 6's first crossing found a third: a report that came back in the order the turn resolves rather than the order the file was written |
| 2 | 3-5 | none | one, faction 6, from inside a crossing that landed the turn it was refused: `arrives on turn 5; it can be given no orders until then`, read while writing turn 5's orders. Arrivals resolve after every phase that carries an order, so the first turn it can be ordered is 6. The guard's own comment said so; its message did not |
| 3 | 6-8 | none | none |
| 4 | 9-11 | none | none |

Every finding is fixed and written down in the burndown, and the lines that
produced it are retired under rule 4. Rounds 3 and 4 came back empty against a
parser that had just been substantially rewritten, which is the result worth
reporting: the seams these ten factions know how to reach are answered
correctly now.

Beside the game, `FuzzParse` has been run three times -- once before the
burndown work and twice after, about 4.5 million inputs each -- and has never
found a panic. Its corpus stands at 747 interesting inputs.
