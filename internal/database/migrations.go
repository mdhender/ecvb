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
CREATE TABLE game (
    id INTEGER PRIMARY KEY,
    code TEXT NOT NULL,
    turn INTEGER NOT NULL DEFAULT 0 CHECK (turn >= 0),
    turn_state TEXT NOT NULL DEFAULT 'open'
        CHECK (turn_state IN ('open', 'resolved')),
    seed_high INTEGER NOT NULL DEFAULT 19 CHECK (seed_high >= 0),
    seed_low INTEGER NOT NULL DEFAULT 12 CHECK (seed_low >= 0)
);

-- A faction is played by a person or by an agent, never by both and never by
-- neither.
CREATE TABLE faction (
    id INTEGER PRIMARY KEY,
    game_id INTEGER NOT NULL REFERENCES game(id),
    user_id INTEGER REFERENCES users(id),
    agent_id INTEGER REFERENCES agent(id),
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

-- Where an entity may stand is a rule of the unit it is: a ship sits at
-- stellium level or in ring 1 through 99 of a planet, a surface colony or
-- factory in ring 0, an orbital colony in ring 1. See docs/entity-location.md.
CREATE TABLE entity (
    id INTEGER PRIMARY KEY,
    unit TEXT NOT NULL CHECK (unit IN ('SHIP', 'COPN', 'CSFC', 'CORB')),
    tech_level INTEGER NOT NULL CHECK (tech_level BETWEEN 0 AND 10),
    stellium_id INTEGER NOT NULL REFERENCES stellium(id),
    system_id INTEGER,
    planet_id INTEGER,
    planet_ring INTEGER,
    faction_id INTEGER NOT NULL REFERENCES faction(id),
    enclosed_volume INTEGER NOT NULL,
    mass INTEGER NOT NULL DEFAULT 0 CHECK (mass >= 0),
    FOREIGN KEY (system_id, stellium_id) REFERENCES system(id, stellium_id),
    FOREIGN KEY (planet_id, system_id) REFERENCES planet(id, system_id),
    CHECK (
        (unit = 'SHIP' AND system_id IS NULL AND planet_id IS NULL AND planet_ring IS NULL)
        OR
        (unit = 'SHIP' AND system_id IS NOT NULL AND planet_id IS NOT NULL AND planet_ring BETWEEN 1 AND 99)
        OR
        (unit IN ('COPN', 'CSFC') AND system_id IS NOT NULL AND planet_id IS NOT NULL AND planet_ring = 0)
        OR
        (unit = 'CORB' AND system_id IS NOT NULL AND planet_id IS NOT NULL AND planet_ring = 1)
    )
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
-- at all leaves actor_entity_id null.
CREATE TABLE game_order (
    game_id INTEGER NOT NULL,
    turn INTEGER NOT NULL CHECK (turn >= 0),
    faction_id INTEGER NOT NULL,
    sequence INTEGER NOT NULL CHECK (sequence > 0),
    source_line INTEGER NOT NULL CHECK (source_line > 0),
    verb TEXT NOT NULL CHECK (verb = lower(trim(verb)) AND verb <> ''),
    actor_entity_id INTEGER,
    input TEXT NOT NULL CHECK (input <> ''),
    params TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(params)),
    fuel_spent INTEGER NOT NULL DEFAULT 0 CHECK (fuel_spent >= 0),
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'succeeded', 'failed')),
    error_message TEXT,
    PRIMARY KEY (game_id, turn, faction_id, sequence),
    FOREIGN KEY (faction_id, game_id) REFERENCES faction(id, game_id),
    FOREIGN KEY (actor_entity_id, faction_id) REFERENCES entity(id, faction_id),
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
CREATE UNIQUE INDEX entity_id_faction_id_idx ON entity(id, faction_id);
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
CREATE INDEX game_order_actor_entity_id_idx ON game_order(actor_entity_id);
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
}

// SchemaVersion is the latest database schema version.
const SchemaVersion = len(migrations)

// Migrations returns the ordered database migrations.
func Migrations() []string {
	return append([]string(nil), migrations[:]...)
}
