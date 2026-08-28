// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package database

// migrations is the ordered database schema, applied in this order and never
// out of it. SchemaVersion is the length of this list, and a database whose
// user_version does not match it is refused rather than migrated in place.
//
// **Add a migration by appending a new element; never edit an existing one.**
// The one exception is the baseline below, and it stops being an exception the
// day a game is live: no database that anyone cares about has yet been built
// from it, so it is still cheap to rewrite. From the first live game onward
// this list is append-only without exception.
var migrations = [...]string{`
-- ACCOUNTS AND GAMES ------------------------------------------------------

CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    email TEXT NOT NULL UNIQUE
        CHECK (email = lower(trim(email)) AND email <> ''),
    role TEXT NOT NULL
        CHECK (role IN ('administrator', 'non-administrator'))
);

CREATE TABLE agent (
    id INTEGER PRIMARY KEY,
    code TEXT,
    description TEXT NOT NULL
);

-- A game's seed is every random decision it will ever make. Re-resolving a
-- turn draws from it again and reaches the same answers.
--
-- The two counters are how a game hands out the numbers it shows the player.
-- They are per game rather than per database, which is the point: a row id is
-- drawn from one sequence shared by every game in the file, so a second game's
-- ids would depend on how many rows the first one wrote. These do not.
CREATE TABLE game (
    id INTEGER PRIMARY KEY,
    code TEXT NOT NULL,
    turn INTEGER NOT NULL DEFAULT 0 CHECK (turn >= 0),
    turn_state TEXT NOT NULL DEFAULT 'open'
        CHECK (turn_state IN ('open', 'resolved')),
    seed_high INTEGER NOT NULL DEFAULT 19 CHECK (seed_high >= 0),
    seed_low INTEGER NOT NULL DEFAULT 12 CHECK (seed_low >= 0),
    -- How many entities this game has created. It is not entity.number: the
    -- number is a keyed permutation of this, so that a number tells a player
    -- nothing about how many entities exist. See internal/entityid.
    next_entity_ordinal INTEGER NOT NULL DEFAULT 0 CHECK (next_entity_ordinal >= 0),
    -- Faction numbers need no such cover -- a player already knows how many
    -- factions there are -- so this counter is the number itself, from 1.
    next_faction_number INTEGER NOT NULL DEFAULT 0 CHECK (next_faction_number >= 0)
);

-- A faction is played by a person or by an agent, never by both and never by
-- neither.
CREATE TABLE faction (
    id INTEGER PRIMARY KEY,
    game_id INTEGER NOT NULL REFERENCES game(id),
    -- The faction as the player knows it, counted from 1 within this game. The
    -- id is a row id and belongs to the database; this belongs to the game, and
    -- it is what a report prints, what "ecrpt --faction" takes, and what
    -- addresses a prng draw about this faction.
    number INTEGER NOT NULL CHECK (number > 0),
    user_id INTEGER REFERENCES users(id),
    agent_id INTEGER REFERENCES agent(id),
    UNIQUE (game_id, number),
    CHECK ((user_id IS NOT NULL) <> (agent_id IS NOT NULL))
);

-- THE MAP -----------------------------------------------------------------

CREATE TABLE stellium (
    id INTEGER PRIMARY KEY,
    game_id INTEGER NOT NULL REFERENCES game(id),
    x INTEGER NOT NULL CHECK (x BETWEEN -15 AND 15),
    y INTEGER NOT NULL CHECK (y BETWEEN -15 AND 15),
    z INTEGER NOT NULL CHECK (z BETWEEN -15 AND 15),
    UNIQUE (game_id, x, y, z)
);

CREATE TABLE system (
    id INTEGER PRIMARY KEY,
    stellium_id INTEGER NOT NULL REFERENCES stellium(id),
    sequence TEXT NOT NULL CHECK (sequence IN ('A', 'B', 'C', 'D', 'E')),
    UNIQUE (stellium_id, sequence),
    UNIQUE (id, stellium_id)
);

CREATE TABLE planet (
    id INTEGER PRIMARY KEY,
    system_id INTEGER NOT NULL REFERENCES system(id),
    orbit INTEGER NOT NULL CHECK (orbit BETWEEN 1 AND 10),
    kind TEXT NOT NULL CHECK (kind IN ('rocky', 'asteroid', 'gas-giant', 'ice-giant')),
    habitability INTEGER NOT NULL CHECK (habitability BETWEEN 0 AND 25),
    faction_id INTEGER REFERENCES faction(id),
    UNIQUE (system_id, orbit),
    UNIQUE (id, system_id)
);

CREATE TABLE deposit (
    id INTEGER PRIMARY KEY,
    planet_id INTEGER NOT NULL REFERENCES planet(id),
    sequence INTEGER NOT NULL CHECK (sequence BETWEEN 1 AND 45),
    resource TEXT NOT NULL CHECK (resource IN ('fuel', 'gold', 'metals', 'minerals')),
    quality INTEGER NOT NULL,
    initial_qty INTEGER NOT NULL,
    current_qty INTEGER NOT NULL,
    UNIQUE (planet_id, sequence)
);

-- THINGS IN THE WORLD -----------------------------------------------------

-- The game_id column is not redundant with faction_id. A ship crossing between
-- stellia has a null stellium_id, so without it the only path from an entity to
-- its game is the faction join, and the per-game uniqueness of number has
-- nothing to be written against.
--
-- Where an entity may stand is a rule of the unit it is: a ship sits at
-- stellium level or in ring 1 through 99 of a planet, a surface colony or
-- factory in ring 0, an orbital colony in ring 1. See docs/entity-location.md.
CREATE TABLE entity (
    -- The id is the database's handle and never leaves it: every child table
    -- points at it, and the order entities were created in -- which is what
    -- settles build seniority -- is the order it rises in.
    --
    -- The number is the game's handle and is the only one a player ever sees or
    -- types. It is unique within the game and never reused, and it is a keyed
    -- permutation of the game's entity ordinal rather than the ordinal itself,
    -- so it says nothing about how many entities the game has made. A player
    -- who could read a count off an opponent's ship id could count the
    -- opponent's fleet. See internal/entityid.
    id INTEGER PRIMARY KEY,
    game_id INTEGER NOT NULL REFERENCES game(id),
    number INTEGER NOT NULL CHECK (number BETWEEN 100000 AND 999999),
    unit TEXT NOT NULL CHECK (unit IN ('SHIP', 'COPN', 'CSFC', 'CORB')),
    tech_level INTEGER NOT NULL CHECK (tech_level BETWEEN 0 AND 10),
    -- Null for a ship crossing between stellia, which is nowhere until it
    -- arrives. Nothing else may be nowhere: a colony does not travel.
    stellium_id INTEGER REFERENCES stellium(id),
    system_id INTEGER,
    planet_id INTEGER,
    planet_ring INTEGER,
    faction_id INTEGER NOT NULL REFERENCES faction(id),
    enclosed_volume INTEGER NOT NULL,
    mass INTEGER NOT NULL DEFAULT 0 CHECK (mass >= 0),
    UNIQUE (game_id, number),
    FOREIGN KEY (faction_id, game_id) REFERENCES faction(id, game_id),
    FOREIGN KEY (system_id, stellium_id) REFERENCES system(id, stellium_id),
    FOREIGN KEY (planet_id, system_id) REFERENCES planet(id, system_id),
    CHECK (
        (unit = 'SHIP' AND stellium_id IS NULL AND system_id IS NULL AND planet_id IS NULL AND planet_ring IS NULL)
        OR
        (unit = 'SHIP' AND stellium_id IS NOT NULL AND system_id IS NULL AND planet_id IS NULL AND planet_ring IS NULL)
        OR
        (unit = 'SHIP' AND stellium_id IS NOT NULL AND system_id IS NOT NULL AND planet_id IS NOT NULL AND planet_ring BETWEEN 1 AND 99)
        OR
        (unit IN ('COPN', 'CSFC') AND stellium_id IS NOT NULL AND system_id IS NOT NULL AND planet_id IS NOT NULL AND planet_ring = 0)
        OR
        (unit = 'CORB' AND stellium_id IS NOT NULL AND system_id IS NOT NULL AND planet_id IS NOT NULL AND planet_ring = 1)
    )
);

-- A ship crossing between stellia.
--
-- The crossing is not the order that began it. A jump order departs -- it draws
-- the whole fuel bill, takes the ship off the board, and succeeds -- and this
-- row is what continues after it. While the row stands the ship is nowhere:
-- entity.stellium_id is null, so it cannot be probed, does not appear on a
-- sensor sweep, and can be given no order. The arrival step of ship movement
-- lands every ship due and deletes its row.
--
-- One row per ship, because a ship makes one crossing at a time. Nothing purges
-- this table: a crossing is live state, not turn history.
CREATE TABLE in_transit (
    game_id INTEGER NOT NULL,
    entity_id INTEGER NOT NULL PRIMARY KEY REFERENCES entity(id),
    destination_stellium_id INTEGER NOT NULL,
    arrival_turn INTEGER NOT NULL CHECK (arrival_turn >= 0),
    FOREIGN KEY (destination_stellium_id, game_id) REFERENCES stellium(id, game_id)
);

CREATE TABLE inventory (
    entity_id INTEGER NOT NULL REFERENCES entity(id),
    section TEXT NOT NULL CHECK (section IN ('component', 'operational', 'unassembled', 'cargo')),
    unit TEXT NOT NULL,
    tech_level INTEGER NOT NULL CHECK (tech_level BETWEEN 0 AND 10),
    quantity INTEGER NOT NULL,
    PRIMARY KEY (entity_id, section, unit, tech_level)
);

CREATE TABLE entity_population (
    entity_id INTEGER NOT NULL REFERENCES entity(id),
    class TEXT NOT NULL CHECK (class IN ('USK', 'SKW', 'SOL', 'NAS')),
    quantity INTEGER NOT NULL CHECK (quantity >= 0),
    PRIMARY KEY (entity_id, class)
);

-- The population an entity has assigned to a cadre. A cadre is not a unit and
-- has no mass or volume of its own: the people in it are already counted in
-- entity_population, and this says what they have been assigned to do. One
-- CWKR is one SKW plus one USK, so an entity's CWKR count is bounded by both.
--
-- Nothing forms or dissolves a cadre yet; that is the draft and disband orders.
-- Until they exist a kit is the only thing that puts a row here.
CREATE TABLE entity_cadre (
    entity_id INTEGER NOT NULL REFERENCES entity(id),
    cadre TEXT NOT NULL CHECK (cadre IN ('CWKR', 'PLCF', 'SPCF', 'TRNE')),
    quantity INTEGER NOT NULL CHECK (quantity >= 0),
    PRIMARY KEY (entity_id, cadre)
);

CREATE TABLE work_group (
    id INTEGER PRIMARY KEY,
    entity_id INTEGER NOT NULL REFERENCES entity(id),
    unit TEXT NOT NULL CHECK (unit IN ('FACT', 'FARM', 'MINE')),
    sequence INTEGER NOT NULL CHECK (sequence BETWEEN 1 AND 99),
    deposit_id INTEGER REFERENCES deposit(id),
    UNIQUE (entity_id, unit, sequence)
);

CREATE TABLE work_group_units (
    work_group_id INTEGER NOT NULL REFERENCES work_group(id),
    tech_level INTEGER NOT NULL CHECK (tech_level BETWEEN 0 AND 10),
    quantity INTEGER NOT NULL,
    PRIMARY KEY (work_group_id, tech_level)
);

-- ORDERS ------------------------------------------------------------------

-- Every order a player writes is a row here, whatever the verb. What the order
-- said beyond its actor lives in params as the words the player used -- never
-- as ids, because the engine resolves the names again when the turn runs, and
-- a name that no longer resolves is a failed order rather than a corrupt row.
-- input is that same order rendered back for a report to print.
--
-- The two ids worth enforcing are still columns: the faction belongs to the
-- game, and the actor belongs to the faction. An order that acts on no entity
-- at all leaves actor_entity_number null.
--
-- The actor is stored as the entity's number rather than its row id, which is
-- what makes the paragraph above true rather than nearly true: every value in
-- this row is now a word or a number the player wrote, and a stored order can
-- render itself back without resolving anything.
CREATE TABLE game_order (
    game_id INTEGER NOT NULL,
    turn INTEGER NOT NULL CHECK (turn >= 0),
    faction_id INTEGER NOT NULL,
    sequence INTEGER NOT NULL CHECK (sequence > 0),
    source_line INTEGER NOT NULL CHECK (source_line > 0),
    verb TEXT NOT NULL CHECK (verb = lower(trim(verb)) AND verb <> ''),
    actor_entity_number INTEGER,
    input TEXT NOT NULL CHECK (input <> ''),
    params TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(params)),
    fuel_spent INTEGER NOT NULL DEFAULT 0 CHECK (fuel_spent >= 0),
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'succeeded', 'failed')),
    error_message TEXT,
    PRIMARY KEY (game_id, turn, faction_id, sequence),
    FOREIGN KEY (faction_id, game_id) REFERENCES faction(id, game_id),
    FOREIGN KEY (actor_entity_number, faction_id) REFERENCES entity(number, faction_id),
    -- What a status means, written once for every order there will ever be
    -- rather than once per order table. A pending order carries the fuel it is
    -- projected to burn; a failed one burned none and says why it failed.
    CHECK (
        (status = 'pending' AND error_message IS NULL)
        OR
        (status = 'succeeded' AND error_message IS NULL)
        OR
        (status = 'failed' AND error_message IS NOT NULL AND error_message <> '' AND fuel_spent = 0)
    )
);

-- Where an order took its actor. Most orders move nothing and have no row
-- here; the row is written once, when the turn resolves.
CREATE TABLE order_movement (
    game_id INTEGER NOT NULL,
    turn INTEGER NOT NULL CHECK (turn >= 0),
    faction_id INTEGER NOT NULL,
    sequence INTEGER NOT NULL CHECK (sequence > 0),
    start_stellium_id INTEGER NOT NULL,
    start_system_id INTEGER,
    start_planet_id INTEGER,
    start_planet_ring INTEGER,
    final_stellium_id INTEGER NOT NULL,
    final_system_id INTEGER,
    final_planet_id INTEGER,
    final_planet_ring INTEGER,
    PRIMARY KEY (game_id, turn, faction_id, sequence),
    FOREIGN KEY (game_id, turn, faction_id, sequence)
        REFERENCES game_order(game_id, turn, faction_id, sequence),
    FOREIGN KEY (start_stellium_id, game_id) REFERENCES stellium(id, game_id),
    FOREIGN KEY (start_system_id, start_stellium_id) REFERENCES system(id, stellium_id),
    FOREIGN KEY (start_planet_id, start_system_id) REFERENCES planet(id, system_id),
    FOREIGN KEY (final_stellium_id, game_id) REFERENCES stellium(id, game_id),
    FOREIGN KEY (final_system_id, final_stellium_id) REFERENCES system(id, stellium_id),
    FOREIGN KEY (final_planet_id, final_system_id) REFERENCES planet(id, system_id),
    -- A location is a stellium, or a stellium and a planet in a ring of one of
    -- its systems. There is nothing in between.
    CHECK ((start_system_id IS NULL AND start_planet_id IS NULL AND start_planet_ring IS NULL)
        OR (start_system_id IS NOT NULL AND start_planet_id IS NOT NULL AND start_planet_ring IS NOT NULL)),
    CHECK ((final_system_id IS NULL AND final_planet_id IS NULL AND final_planet_ring IS NULL)
        OR (final_system_id IS NOT NULL AND final_planet_id IS NOT NULL AND final_planet_ring IS NOT NULL))
);

-- A failed order goes nowhere. The status is on the spine and the locations
-- are here, so no single CHECK can see both and the rule is a trigger.
CREATE TRIGGER order_movement_failed_goes_nowhere
BEFORE INSERT ON order_movement
WHEN (SELECT status FROM game_order
      WHERE game_id = NEW.game_id AND turn = NEW.turn
        AND faction_id = NEW.faction_id AND sequence = NEW.sequence) = 'failed'
    AND NOT (NEW.final_stellium_id IS NEW.start_stellium_id
        AND NEW.final_system_id IS NEW.start_system_id
        AND NEW.final_planet_id IS NEW.start_planet_id
        AND NEW.final_planet_ring IS NEW.start_planet_ring)
BEGIN
    SELECT RAISE(ABORT, 'a failed order goes nowhere: record it ending where it started');
END;

-- What an order read. A probe that failed read nothing and has no row.
CREATE TABLE order_survey (
    game_id INTEGER NOT NULL,
    turn INTEGER NOT NULL CHECK (turn >= 0),
    faction_id INTEGER NOT NULL,
    sequence INTEGER NOT NULL CHECK (sequence > 0),
    stellium_id INTEGER NOT NULL,
    system_id INTEGER NOT NULL,
    planet_id INTEGER NOT NULL,
    habitability INTEGER NOT NULL CHECK (habitability BETWEEN 0 AND 25),
    PRIMARY KEY (game_id, turn, faction_id, sequence),
    FOREIGN KEY (game_id, turn, faction_id, sequence)
        REFERENCES game_order(game_id, turn, faction_id, sequence),
    FOREIGN KEY (stellium_id, game_id) REFERENCES stellium(id, game_id),
    FOREIGN KEY (system_id, stellium_id) REFERENCES system(id, stellium_id),
    FOREIGN KEY (planet_id, system_id) REFERENCES planet(id, system_id)
);

-- FINDINGS ----------------------------------------------------------------

-- What a faction has seen, recorded when it saw it. A finding is not an order
-- and outlives the order that produced it within the turn: a ship may probe a
-- planet and then jump away, and the reading still stands.

CREATE TABLE probe_contact (
    game_id INTEGER NOT NULL,
    turn INTEGER NOT NULL CHECK (turn >= 0),
    faction_id INTEGER NOT NULL,
    planet_id INTEGER NOT NULL REFERENCES planet(id),
    entity_id INTEGER NOT NULL REFERENCES entity(id),
    unit TEXT NOT NULL CHECK (unit IN ('SHIP', 'COPN', 'CSFC', 'CORB')),
    planet_ring INTEGER NOT NULL CHECK (planet_ring BETWEEN 0 AND 99),
    mass INTEGER NOT NULL CHECK (mass >= 0),
    PRIMARY KEY (game_id, turn, faction_id, planet_id, entity_id),
    FOREIGN KEY (faction_id, game_id) REFERENCES faction(id, game_id)
);

CREATE TABLE probe_deposit (
    game_id INTEGER NOT NULL,
    turn INTEGER NOT NULL CHECK (turn >= 0),
    faction_id INTEGER NOT NULL,
    planet_id INTEGER NOT NULL REFERENCES planet(id),
    deposit_id INTEGER NOT NULL REFERENCES deposit(id),
    resource TEXT NOT NULL CHECK (resource IN ('fuel', 'gold', 'metals', 'minerals')),
    quantity INTEGER NOT NULL CHECK (quantity >= 0),
    PRIMARY KEY (game_id, turn, faction_id, planet_id, deposit_id),
    FOREIGN KEY (faction_id, game_id) REFERENCES faction(id, game_id)
);

CREATE TABLE sensor_survey (
    game_id INTEGER NOT NULL,
    turn INTEGER NOT NULL CHECK (turn >= 0),
    faction_id INTEGER NOT NULL,
    entity_id INTEGER NOT NULL REFERENCES entity(id),
    stellium_id INTEGER NOT NULL,
    system_id INTEGER,
    systems INTEGER NOT NULL CHECK (systems >= 0),
    PRIMARY KEY (game_id, turn, faction_id, entity_id),
    FOREIGN KEY (faction_id, game_id) REFERENCES faction(id, game_id),
    FOREIGN KEY (stellium_id, game_id) REFERENCES stellium(id, game_id),
    FOREIGN KEY (system_id, stellium_id) REFERENCES system(id, stellium_id)
);

CREATE TABLE sensor_contact (
    game_id INTEGER NOT NULL,
    turn INTEGER NOT NULL CHECK (turn >= 0),
    faction_id INTEGER NOT NULL,
    entity_id INTEGER NOT NULL REFERENCES entity(id),
    planet_id INTEGER NOT NULL REFERENCES planet(id),
    contact_id INTEGER NOT NULL REFERENCES entity(id),
    unit TEXT NOT NULL CHECK (unit IN ('SHIP', 'CORB')),
    planet_ring INTEGER NOT NULL CHECK (planet_ring BETWEEN 0 AND 99),
    mass INTEGER NOT NULL CHECK (mass >= 0),
    PRIMARY KEY (game_id, turn, faction_id, entity_id, contact_id),
    FOREIGN KEY (faction_id, game_id) REFERENCES faction(id, game_id)
);

-- INDEXES -----------------------------------------------------------------

-- The compound uniques the compound foreign keys above are written against.
-- SQLite requires a foreign key's parent columns to be a unique index.
CREATE UNIQUE INDEX game_code_idx ON game(code);
CREATE UNIQUE INDEX faction_id_game_id_idx ON faction(id, game_id);
CREATE UNIQUE INDEX entity_number_faction_id_idx ON entity(number, faction_id);
CREATE UNIQUE INDEX stellium_id_game_id_idx ON stellium(id, game_id);

-- One person plays one faction in a game, and one agent plays the rest.
CREATE UNIQUE INDEX agent_code_idx ON agent(code) WHERE code IS NOT NULL;
CREATE UNIQUE INDEX faction_game_user_idx ON faction(game_id, user_id) WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX faction_game_agent_idx ON faction(game_id, agent_id) WHERE agent_id IS NOT NULL;

CREATE INDEX faction_game_id_idx ON faction(game_id);
CREATE INDEX faction_user_id_idx ON faction(user_id);
CREATE INDEX faction_agent_id_idx ON faction(agent_id);
CREATE INDEX stellium_game_id_idx ON stellium(game_id);
CREATE INDEX planet_faction_id_idx ON planet(faction_id);
CREATE INDEX entity_faction_id_idx ON entity(faction_id);
CREATE INDEX work_group_deposit_id_idx ON work_group(deposit_id);
CREATE INDEX game_order_actor_entity_number_idx ON game_order(actor_entity_number);

-- The arrival step asks one question: which ships are due this turn.
CREATE INDEX in_transit_arrival_idx ON in_transit(game_id, arrival_turn);
`,
	`
-- What a faction calls things.
--
-- A name is a label its owner reads, not a property of the thing named: a
-- player may name a stellium they have never visited, and naming their own
-- ship does not change what anybody else's report calls it. So a name belongs
-- to a faction and a subject, and exactly one kind of subject at a time.
CREATE TABLE faction_name (
    game_id INTEGER NOT NULL,
    faction_id INTEGER NOT NULL,
    stellium_id INTEGER REFERENCES stellium(id),
    system_id INTEGER REFERENCES system(id),
    planet_id INTEGER REFERENCES planet(id),
    entity_id INTEGER REFERENCES entity(id),
    name TEXT NOT NULL
        CHECK (name = trim(name) AND name <> '' AND length(name) <= 24 AND instr(name, '  ') = 0),
    FOREIGN KEY (faction_id, game_id) REFERENCES faction(id, game_id),
    FOREIGN KEY (stellium_id, game_id) REFERENCES stellium(id, game_id),
    CHECK ((stellium_id IS NOT NULL) + (system_id IS NOT NULL)
        + (planet_id IS NOT NULL) + (entity_id IS NOT NULL) = 1)
);

-- One name per faction per thing: naming something again renames it.
CREATE UNIQUE INDEX faction_name_stellium_idx ON faction_name(faction_id, stellium_id) WHERE stellium_id IS NOT NULL;
CREATE UNIQUE INDEX faction_name_system_idx ON faction_name(faction_id, system_id) WHERE system_id IS NOT NULL;
CREATE UNIQUE INDEX faction_name_planet_idx ON faction_name(faction_id, planet_id) WHERE planet_id IS NOT NULL;
CREATE UNIQUE INDEX faction_name_entity_idx ON faction_name(faction_id, entity_id) WHERE entity_id IS NOT NULL;
`,
	`
-- A ship or colony being built.
--
-- The create order that began it departed and succeeded, the way a jump does,
-- and these two tables carry what continues. Nothing purges them, for the
-- reason nothing purges in_transit: a build is live state rather than turn
-- history, and it outlives the order row that "ec turn open" takes away.
--
-- The presence of an under_construction row IS the entity's status, so the
-- entity table needs no status column: when the last item completes, both rows go and what
-- is left is an ordinary entity.
--
-- Seniority is entity_id, the row id -- not entity.number, which is a
-- permutation and carries no order at all. A row id is unique, rises
-- monotonically, and is never reused, so one builder's unfinished entities are
-- already in the order their builds started -- within a turn, because create
-- orders execute in the order they were written, and across turns for the
-- obvious reason. Nothing is stored to settle it and nothing references
-- game_order, which is purged anyway.
CREATE TABLE under_construction (
    entity_id INTEGER PRIMARY KEY REFERENCES entity(id),
    game_id INTEGER NOT NULL REFERENCES game(id),
    -- The entity feeding this build. It claims from its stock, carries on its
    -- transports, and lends its construction workers a turn at a time.
    builder_entity_id INTEGER NOT NULL REFERENCES entity(id),
    -- The "with" clause: a ceiling on the workers a build may use in a turn,
    -- never a reservation. It holds nothing back.
    cwkr_cap INTEGER NOT NULL CHECK (cwkr_cap > 0),
    -- Set when every structural "using" line is completed. Until then only the
    -- STRC and STRL lines are eligible, because everything else needs enclosed
    -- space to be delivered into and structure is what makes some.
    structure_complete INTEGER NOT NULL DEFAULT 0 CHECK (structure_complete IN (0, 1))
);

-- One line of a build's two lists, in the order the player wrote it.
--
-- ordinal is the line's place in its clause, which is its priority: list order
-- decides what gets scarce materials, transport, and workers first. What is
-- still wanted on a line is required - claimed - delivered - completed; it is
-- derived rather than stored.
--
-- The two clauses do not mean the same thing. A "using" line names what the new
-- entity is made of and is completed when its units are assembled into it. A
-- "transfering" line names what is handed over rather than built in, and is
-- completed when its units are stowed in cargo or, for a population class, when
-- the people are aboard.
CREATE TABLE construction_item (
    entity_id INTEGER NOT NULL REFERENCES under_construction(entity_id) ON DELETE CASCADE,
    ordinal INTEGER NOT NULL CHECK (ordinal > 0),
    clause TEXT NOT NULL CHECK (clause IN ('using', 'transfering')),
    unit TEXT NOT NULL CHECK (unit = upper(trim(unit)) AND unit <> ''),
    tech_level INTEGER NOT NULL DEFAULT 0 CHECK (tech_level BETWEEN 0 AND 10),
    required INTEGER NOT NULL CHECK (required > 0),
    -- This turn's call on the builder's stock, written at stage 5 and consumed
    -- at stage 9. A claim lives for one turn and is never banked: what does not
    -- get carried is released, and next turn's claiming runs afresh.
    claimed INTEGER NOT NULL DEFAULT 0 CHECK (claimed >= 0),
    -- At the new entity and not yet worked.
    delivered INTEGER NOT NULL DEFAULT 0 CHECK (delivered >= 0),
    -- Assembled, stowed, or aboard.
    completed INTEGER NOT NULL DEFAULT 0 CHECK (completed >= 0),
    PRIMARY KEY (entity_id, ordinal),
    CHECK (claimed + delivered + completed <= required)
);

-- A trade station is an orbital colony with a flag on it. What the flag confers
-- is stage 11's business and is not written; the grammar accepts it now so that
-- a build begun today is the thing the player asked for when the rules land.
ALTER TABLE entity ADD COLUMN trade_station INTEGER NOT NULL DEFAULT 0
    CHECK (trade_station IN (0, 1));
ALTER TABLE under_construction ADD COLUMN trade_station INTEGER NOT NULL DEFAULT 0
    CHECK (trade_station IN (0, 1));

-- What an order still wants to say when it succeeded and did less than it was
-- asked for. It is not an error_message: the order succeeded, and a shortage is
-- a rate rather than a failure.
ALTER TABLE game_order ADD COLUMN note TEXT;

-- The two questions a build's sweeps ask: which builds does this entity feed,
-- and what is left on this build.
CREATE INDEX under_construction_builder_idx ON under_construction(builder_entity_id);
`,
}

// SchemaVersion is the latest database schema version.
const SchemaVersion = len(migrations)

// Migrations returns the ordered database migrations.
func Migrations() []string {
	return append([]string(nil), migrations[:]...)
}
