# EAGLE-01 Game Rules

A game played against the engine's rules rather than against the map, and
against the documentation that is supposed to describe them. All ten player
factions are rule lawyers; none of them is trying to win the search.

It is the third of the three games and the last one to arrive. [CLAUDE-01](../claude/game-rules.md)
plays the game. [FUZZ-01](../fuzzers/game-rules.md) attacks the parser, which
is everything that happens before an order is accepted. EAGLE-01 takes it from
there: every order it files is **legal**, and the argument is about what the
engine did with it.

## Rules

1. A faction scores when the engine produces a result the documentation does
   not support: the engine and a document disagree, or two documents disagree.
2. A faction scores when the documentation does not say what should happen at
   all. **Missing documentation is a scoring result** -- it is most of them.
3. Every order a faction files must parse, bind, and resolve. A refused docket
   has argued nothing; `replay.sh` reports it as a mistake in the docket, not
   as a finding.
4. A finding already written down in
   [`docs/plan/engine-burndown.md`](../../docs/plan/engine-burndown.md) is not
   a finding.

The gamemaster judges, and judges from `docs/`. "The engine does something
surprising" is not a score on its own -- the question is always whether a
player reading the documentation would have predicted it.

Rule 3 is the whole difference between this game and the fuzzers'. A fuzzer
wins by writing something the parser cannot read; a rule lawyer has to write
something it reads perfectly and then argue about the consequence. That means
the dockets need real entities in real states, so EAGLE-01 is a proper game on
`games/claude`'s map with the shipped starting kit.

## The ten dockets

| Faction | Docket |
| --- | --- |
| 1 | MOVE: what a move costs and where it leaves the ship |
| 3 | JUMP: the fuel bill, the crossing, and what a jump leaves behind |
| 4 | PROBE: the budget, and what a repeated orbit costs |
| 5 | NAME: what a name may be, and what a second one does |
| 6 | ASSEMBLE and UNASSEMBLE: work with no one to do it |
| 7 | STOW and UNSTOW: the other pool of workers |
| 8 | TRANSFER: what carries it, and what it costs |
| 9 | CREATE: a commitment made with workers that do not exist |
| 10 | The group forms of CREATE, which parse and are not built |
| 11 | The turn: what order things happen in, and what the report says |

Each files four turns of it. The orders are chosen to make the engine state a
rule out loud -- a fuel figure, a ring, a shortfall note, a status -- so that
there is something specific to hold the documentation against.

## Playing a round

```sh
games/eagles/replay.sh [FIRST-TURN] [LAST-TURN]   # default 0 3
```

The run writes `games/eagles/reports/`, which is not committed: the arguing is
done by reading those against `docs/`, and what survives goes in the burndown.
A refused docket is printed with `!!` and the check output kept, because that
is a bug in the docket to be fixed before the round counts.

The map and the kit are `games/claude`'s, copied in rather than committed
twice.

## Results

| Round | Turns | Findings |
| --- | --- | --- |
| 1 | 0-3 | five, all in the burndown: a report column that calls a ceiling `WORKERS`; `docs/orders.md` contradicting itself about a second MOVE; no document describing what a starting kit can do; `games/claude/game-rules.md` claiming a ring the engine never draws; and NAME saying nothing about renaming or duplicate names |

Four of the five are documentation rather than engine defects, which is the
result worth reporting: the engine did almost exactly what it says it does. Of
the twelve behaviours round 1 probed, seven matched their documentation
exactly -- move costs and rings, jump fuel and crossings, probe budgets, the
not-built group forms, and the phase renumbering. Those are listed in the
burndown too, because a conformance game is only worth reading if it says what
it failed to find.
