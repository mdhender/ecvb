// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package database

var migrations = [...]string{`
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    email TEXT NOT NULL UNIQUE
        CHECK (email = lower(trim(email)) AND email <> ''),
    role TEXT NOT NULL
        CHECK (role IN ('administrator', 'non-administrator'))
);

CREATE TABLE game (
    id INTEGER PRIMARY KEY,
    code TEXT NOT NULL,
    turn INTEGER NOT NULL DEFAULT 0 CHECK (turn >= 0)
);

CREATE TABLE agent (
    id INTEGER PRIMARY KEY,
    description TEXT NOT NULL
);

CREATE TABLE faction (
    id INTEGER PRIMARY KEY,
    game_id INTEGER NOT NULL REFERENCES game(id),
    user_id INTEGER REFERENCES users(id),
    agent_id INTEGER REFERENCES agent(id),
    CHECK ((user_id IS NOT NULL) <> (agent_id IS NOT NULL))
);

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

CREATE TABLE order_entry (
    game_id INTEGER NOT NULL REFERENCES game(id),
    faction_id INTEGER NOT NULL REFERENCES faction(id),
    sequence INTEGER NOT NULL,
    entity_id INTEGER NOT NULL REFERENCES entity(id),
    verb TEXT NOT NULL,
    target_entity_id INTEGER REFERENCES entity(id),
    support_entity_id INTEGER REFERENCES entity(id),
    parameters TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (faction_id, sequence)
);

CREATE INDEX faction_game_id_idx ON faction(game_id);
CREATE INDEX faction_user_id_idx ON faction(user_id);
CREATE INDEX faction_agent_id_idx ON faction(agent_id);
CREATE INDEX stellium_game_id_idx ON stellium(game_id);
CREATE INDEX planet_faction_id_idx ON planet(faction_id);
CREATE INDEX entity_faction_id_idx ON entity(faction_id);
CREATE INDEX work_group_deposit_id_idx ON work_group(deposit_id);
CREATE INDEX order_entry_game_id_idx ON order_entry(game_id);
CREATE INDEX order_entry_entity_id_idx ON order_entry(entity_id);
CREATE INDEX order_entry_target_entity_id_idx ON order_entry(target_entity_id);
CREATE INDEX order_entry_support_entity_id_idx ON order_entry(support_entity_id);
`,
	`
CREATE UNIQUE INDEX game_code_idx ON game(code);
`,
	`
ALTER TABLE game ADD COLUMN seed_high INTEGER NOT NULL DEFAULT 19 CHECK (seed_high >= 0);
ALTER TABLE game ADD COLUMN seed_low INTEGER NOT NULL DEFAULT 12 CHECK (seed_low >= 0);
CREATE UNIQUE INDEX faction_game_user_idx ON faction(game_id, user_id) WHERE user_id IS NOT NULL;
`,
	`
ALTER TABLE agent ADD COLUMN code TEXT;
CREATE UNIQUE INDEX agent_code_idx ON agent(code) WHERE code IS NOT NULL;
CREATE UNIQUE INDEX faction_game_agent_idx ON faction(game_id, agent_id) WHERE agent_id IS NOT NULL;
ALTER TABLE entity ADD COLUMN mass INTEGER NOT NULL DEFAULT 0 CHECK (mass >= 0);

CREATE TABLE entity_population (
    entity_id INTEGER NOT NULL REFERENCES entity(id),
    class TEXT NOT NULL CHECK (class IN ('USK', 'SKW', 'SOL', 'NAS')),
    quantity INTEGER NOT NULL CHECK (quantity >= 0),
    PRIMARY KEY (entity_id, class)
);
`,
	`
ALTER TABLE game ADD COLUMN turn_state TEXT NOT NULL DEFAULT 'open'
    CHECK (turn_state IN ('open', 'resolved'));

CREATE TABLE order_migration_guard (id INTEGER);
CREATE TRIGGER order_migration_requires_empty_order_entry
BEFORE INSERT ON order_migration_guard
BEGIN
    SELECT RAISE(ABORT, 'cannot migrate submitted order_entry rows; preserve the order files, clear the submissions, and submit them after migration');
END;
INSERT INTO order_migration_guard SELECT 1 FROM order_entry LIMIT 1;
DROP TRIGGER order_migration_requires_empty_order_entry;
DROP TABLE order_migration_guard;
DROP TABLE order_entry;

CREATE UNIQUE INDEX faction_id_game_id_idx ON faction(id, game_id);
CREATE UNIQUE INDEX entity_id_faction_id_idx ON entity(id, faction_id);
CREATE UNIQUE INDEX stellium_id_game_id_idx ON stellium(id, game_id);

CREATE TABLE jump_order (
    game_id INTEGER NOT NULL,
    turn INTEGER NOT NULL CHECK (turn >= 0),
    faction_id INTEGER NOT NULL,
    sequence INTEGER NOT NULL CHECK (sequence > 0),
    source_line INTEGER NOT NULL CHECK (source_line > 0),
    ship_id INTEGER NOT NULL,
    destination_x INTEGER NOT NULL,
    destination_y INTEGER NOT NULL,
    destination_z INTEGER NOT NULL,
    destination_stellium_id INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'succeeded', 'failed')),
    error_message TEXT,
    start_stellium_id INTEGER,
    start_system_id INTEGER,
    start_planet_id INTEGER,
    start_planet_ring INTEGER,
    final_stellium_id INTEGER,
    final_system_id INTEGER,
    final_planet_id INTEGER,
    final_planet_ring INTEGER,
    PRIMARY KEY (game_id, turn, faction_id, sequence),
    FOREIGN KEY (faction_id, game_id) REFERENCES faction(id, game_id),
    FOREIGN KEY (ship_id, faction_id) REFERENCES entity(id, faction_id),
    FOREIGN KEY (destination_stellium_id, game_id) REFERENCES stellium(id, game_id),
    FOREIGN KEY (start_stellium_id, game_id) REFERENCES stellium(id, game_id),
    FOREIGN KEY (start_system_id, start_stellium_id) REFERENCES system(id, stellium_id),
    FOREIGN KEY (start_planet_id, start_system_id) REFERENCES planet(id, system_id),
    FOREIGN KEY (final_stellium_id, game_id) REFERENCES stellium(id, game_id),
    FOREIGN KEY (final_system_id, final_stellium_id) REFERENCES system(id, stellium_id),
    FOREIGN KEY (final_planet_id, final_system_id) REFERENCES planet(id, system_id),
    CHECK (
        (status = 'pending' AND error_message IS NULL
            AND start_stellium_id IS NULL AND start_system_id IS NULL
            AND start_planet_id IS NULL AND start_planet_ring IS NULL
            AND final_stellium_id IS NULL AND final_system_id IS NULL
            AND final_planet_id IS NULL AND final_planet_ring IS NULL)
        OR
        (status = 'succeeded' AND error_message IS NULL
            AND start_stellium_id IS NOT NULL AND final_stellium_id IS NOT NULL)
        OR
        (status = 'failed' AND error_message IS NOT NULL AND error_message <> ''
            AND start_stellium_id IS NOT NULL AND final_stellium_id IS start_stellium_id
            AND final_system_id IS start_system_id AND final_planet_id IS start_planet_id
            AND final_planet_ring IS start_planet_ring)
    ),
    CHECK ((start_system_id IS NULL AND start_planet_id IS NULL AND start_planet_ring IS NULL)
        OR (start_system_id IS NOT NULL AND start_planet_id IS NOT NULL AND start_planet_ring IS NOT NULL)),
    CHECK ((final_system_id IS NULL AND final_planet_id IS NULL AND final_planet_ring IS NULL)
        OR (final_system_id IS NOT NULL AND final_planet_id IS NOT NULL AND final_planet_ring IS NOT NULL))
);

CREATE TABLE move_order (
    game_id INTEGER NOT NULL,
    turn INTEGER NOT NULL CHECK (turn >= 0),
    faction_id INTEGER NOT NULL,
    sequence INTEGER NOT NULL CHECK (sequence > 0),
    source_line INTEGER NOT NULL CHECK (source_line > 0),
    ship_id INTEGER NOT NULL,
    requested_system TEXT CHECK (requested_system IS NULL OR requested_system IN ('A', 'B', 'C', 'D', 'E')),
    requested_orbit INTEGER NOT NULL CHECK (requested_orbit BETWEEN 1 AND 10),
    destination_stellium_id INTEGER NOT NULL,
    destination_system_id INTEGER NOT NULL,
    destination_planet_id INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'succeeded', 'failed')),
    error_message TEXT,
    start_stellium_id INTEGER,
    start_system_id INTEGER,
    start_planet_id INTEGER,
    start_planet_ring INTEGER,
    final_stellium_id INTEGER,
    final_system_id INTEGER,
    final_planet_id INTEGER,
    final_planet_ring INTEGER,
    PRIMARY KEY (game_id, turn, faction_id, sequence),
    FOREIGN KEY (faction_id, game_id) REFERENCES faction(id, game_id),
    FOREIGN KEY (ship_id, faction_id) REFERENCES entity(id, faction_id),
    FOREIGN KEY (destination_stellium_id, game_id) REFERENCES stellium(id, game_id),
    FOREIGN KEY (destination_system_id, destination_stellium_id) REFERENCES system(id, stellium_id),
    FOREIGN KEY (destination_planet_id, destination_system_id) REFERENCES planet(id, system_id),
    FOREIGN KEY (start_stellium_id, game_id) REFERENCES stellium(id, game_id),
    FOREIGN KEY (start_system_id, start_stellium_id) REFERENCES system(id, stellium_id),
    FOREIGN KEY (start_planet_id, start_system_id) REFERENCES planet(id, system_id),
    FOREIGN KEY (final_stellium_id, game_id) REFERENCES stellium(id, game_id),
    FOREIGN KEY (final_system_id, final_stellium_id) REFERENCES system(id, stellium_id),
    FOREIGN KEY (final_planet_id, final_system_id) REFERENCES planet(id, system_id),
    CHECK (
        (status = 'pending' AND error_message IS NULL
            AND start_stellium_id IS NULL AND start_system_id IS NULL
            AND start_planet_id IS NULL AND start_planet_ring IS NULL
            AND final_stellium_id IS NULL AND final_system_id IS NULL
            AND final_planet_id IS NULL AND final_planet_ring IS NULL)
        OR
        (status = 'succeeded' AND error_message IS NULL
            AND start_stellium_id IS NOT NULL AND final_stellium_id IS NOT NULL)
        OR
        (status = 'failed' AND error_message IS NOT NULL AND error_message <> ''
            AND start_stellium_id IS NOT NULL AND final_stellium_id IS start_stellium_id
            AND final_system_id IS start_system_id AND final_planet_id IS start_planet_id
            AND final_planet_ring IS start_planet_ring)
    ),
    CHECK ((start_system_id IS NULL AND start_planet_id IS NULL AND start_planet_ring IS NULL)
        OR (start_system_id IS NOT NULL AND start_planet_id IS NOT NULL AND start_planet_ring IS NOT NULL)),
    CHECK ((final_system_id IS NULL AND final_planet_id IS NULL AND final_planet_ring IS NULL)
        OR (final_system_id IS NOT NULL AND final_planet_id IS NOT NULL AND final_planet_ring IS NOT NULL))
);

CREATE INDEX jump_order_game_turn_idx ON jump_order(game_id, turn);
CREATE INDEX jump_order_ship_id_idx ON jump_order(ship_id);
CREATE INDEX move_order_game_turn_idx ON move_order(game_id, turn);
CREATE INDEX move_order_ship_id_idx ON move_order(ship_id);
`,
}

// SchemaVersion is the latest database schema version.
const SchemaVersion = len(migrations)

// Migrations returns the ordered database migrations.
func Migrations() []string {
	return append([]string(nil), migrations[:]...)
}
