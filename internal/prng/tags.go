// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package prng

// The root domain-tag registry: the leading element of every root key path
// names the purpose of a draw, providing domain separation so two purposes do
// not accidentally share a stream. Derived Seeds may define a subsystem-local
// registry. This is the single, authoritative place root tags are defined.
//
// Instance keys for map objects are their canonical coordinates, never SQLite
// autoincrement row ids: row ids depend on insertion order, so addressing draws
// by them would weld a game's randomness to the order rows happened to be
// written. Coordinates are intrinsic to the map — a stellium's (x, y, z) with
// each axis in -15..15, a system adding its sequence letter (A=1, B=2, ...),
// and a planet adding its orbit (1..10). A deposit adds its sequential deposit
// number on the planet (1..40), assigned at generation and never reused or
// renumbered. TagPlayer and TagFaction draws use their game-assigned numbers,
// which must likewise be stable — never database row ids.
//
// An entity has no coordinates of its own — it has a location, and the location
// moves — so what addresses draws about it is entity.number: a six-digit number
// the game assigns from a per-game ordinal, unique within the game, never
// reused, printed in every report and written by the player as `ship 482137`.
// A faction is addressed by faction.number the same way. Neither is a row id,
// so the rule above holds without exception: nothing in a key path depends on
// the order rows happened to be written.
//
// FROZEN SURFACE — APPEND ONLY. The block starts at 1 (0 is invalid, so a
// forgotten tag is an obvious bug rather than a silent alias). Never insert or
// reorder a constant: iota would renumber every tag after it and silently
// rewrite every live game. To add a tag, append it to the END of this block and
// pin a golden vector for its stream.
const (
	_           Key = iota // 0 is invalid — never use as a domain tag
	TagCluster             // 1: cluster generation
	TagStellium            // 2: per-stellium contents, addressed by (x, y, z)
	TagSystem              // 3: per-system contents, addressed by (x, y, z, seq)
	TagPlanet              // 4: per-orbit contents, addressed by (x, y, z, seq, orbit)
	TagDeposit             // 5: per-deposit draws, addressed by (x, y, z, seq, orbit, deposit_no)
	TagPlayer              // 6: per-player draws, addressed by player number
	TagFaction             // 7: per-faction draws, addressed by faction number
	// 8: the ring a ship settles into at a planet, addressed by
	// (x, y, z, seq, orbit, turn, faction number, entity id). The turn is in
	// the address because a ship ordered to the planet it is already at draws a
	// fresh ring; the faction and entity are there because two ships may settle
	// at one planet in one turn.
	TagRing
	// 9: the public number of an entity, addressed by (round, half). It is the
	// round function of the keyed permutation in internal/entityid that turns a
	// game's entity ordinal into the number the player sees.
	TagEntityNumber
)
