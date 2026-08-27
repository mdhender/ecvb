-- The golden scenario.
--
-- A small game built to make every rule fire: each kind of move and its fuel
-- cost, each reason a move or a jump fails, probes of the current and of a
-- named system, a probe from the stellium orbit, a colony probe, a crossing
-- that finishes in the turn it began and one that takes three, and passive
-- sensor readings of another faction.
--
-- A ship moves once a turn and jumps once a turn, so a turn that wants to show
-- two moves has to spend two ships on it, and a ship that wants to show two
-- moves has to take two turns over them.
--
-- Numbers are chosen so the arithmetic is checkable by hand:
--   HDRV-3 masses 135 MU, jumps 3 ly, propels 3,135 MU, burns 40 FUEL/ly.
--   A hop costs 4 FUEL per unit, crossing systems 8, going nowhere 0.
--   SNSR-2 launches 2 probes, SNSR-1 launches 1. FUEL masses 1 MU.

INSERT INTO users (id, email, role) VALUES
    (1, 'player@example.com', 'non-administrator');
INSERT INTO agent (id, code, description) VALUES (1, 'uncontrolled', 'Uncontrolled');
INSERT INTO game (id, code, turn, turn_state, seed_high, seed_low) VALUES
    (1, 'GOLD-01', 0, 'open', 19, 12);
INSERT INTO faction (id, game_id, user_id) VALUES (1, 1, 1);
INSERT INTO faction (id, game_id, agent_id) VALUES (2, 1, 1);

-- Home is the origin. NEAR is 3 ly away and FAR is 5. Technology level does not
-- cap the distance -- every drive reaches everywhere -- it divides it: a HDRV-3
-- crosses to NEAR in one turn and a HDRV-1 takes three.
INSERT INTO stellium (id, game_id, x, y, z) VALUES
    (10, 1, 0, 0, 0),   -- HOME
    (11, 1, 1, 2, 2),   -- NEAR, distance 3
    (12, 1, 3, 4, 0);   -- FAR, distance 5

INSERT INTO system (id, stellium_id, sequence) VALUES
    (20, 10, 'A'), (21, 10, 'B'),
    (22, 11, 'A');

INSERT INTO planet (id, system_id, orbit, kind, habitability, faction_id) VALUES
    (30, 20, 4, 'rocky', 12, 1),      -- the home planet
    (31, 20, 6, 'rocky', 5, NULL),    -- another planet of system A
    (32, 21, 4, 'asteroid', 0, NULL), -- a planet of system B, across the stellium
    (33, 22, 1, 'rocky', 9, NULL);    -- a planet of NEAR, reachable only by jump

INSERT INTO deposit (id, planet_id, sequence, resource, quality, initial_qty, current_qty) VALUES
    (40, 30, 1, 'fuel', 20, 5000, 4200),
    (41, 30, 2, 'gold', 35, 900, 900),
    (42, 31, 1, 'metals', 10, 60000, 60000);

-- 100 is the working ship: a drive it can afford to run and sensors to spend.
-- 101 is its home colony, which may probe but never move.
-- 102 has no drive at all. 103 is too massive for the drive it has.
-- 104 has a drive and no fuel to run it.
-- 105 has the slowest drive there is, so its crossing spans turns.
-- 106 is parked in the stellium orbit, which is the one place a move can go
--     nowhere and the one place a probe has to name its system.
-- 200 belongs to the other faction and exists to be seen.
INSERT INTO entity (id, unit, tech_level, stellium_id, system_id, planet_id, planet_ring, faction_id, enclosed_volume, mass) VALUES
    (100, 'SHIP', 1, 10, 20, 30, 64, 1, 5000, 2270),
    (101, 'COPN', 1, 10, 20, 30,  0, 1, 5000, 1000),
    (102, 'SHIP', 1, 10, 20, 30, 64, 1, 5000,  500),
    (103, 'SHIP', 1, 10, 20, 30, 64, 1, 5000, 9000),
    (104, 'SHIP', 1, 10, 20, 30, 64, 1, 5000,  271),
    (105, 'SHIP', 1, 10, 20, 30, 64, 1, 5000,  400),
    (106, 'SHIP', 1, 10, NULL, NULL, NULL, 1, 5000, 500),
    (200, 'SHIP', 1, 10, 20, 31, 55, 2, 5000, 7400);

INSERT INTO inventory (entity_id, section, unit, tech_level, quantity) VALUES
    -- 2 HDRV-3: range 3, capacity 6,270 MU, 8 FUEL a hop, 240 FUEL for a 3 ly jump.
    (100, 'component', 'HDRV', 3, 2),
    (100, 'component', 'SNSR', 2, 2),   -- 4 probes a turn
    (100, 'cargo', 'FUEL', 0, 2000),
    (101, 'component', 'SNSR', 1, 1),   -- 1 probe a turn
    (102, 'cargo', 'FUEL', 0, 500),     -- fuel but no drive
    (103, 'component', 'HDRV', 1, 1),   -- capacity 1,045 MU against a 9,000 MU ship
    (103, 'cargo', 'FUEL', 0, 500),
    (104, 'component', 'HDRV', 3, 1),   -- a working drive
    (104, 'cargo', 'FUEL', 0, 1),       -- and one unit of fuel
    -- 1 HDRV-1: capacity 1,045 MU, 4 FUEL a hop, 120 FUEL for a 3 ly jump, and
    -- three turns to cross it, because a technology level divides the distance.
    (105, 'component', 'HDRV', 1, 1),
    (105, 'cargo', 'FUEL', 0, 200),
    (106, 'component', 'HDRV', 3, 1),
    (106, 'component', 'SNSR', 1, 1),  -- 1 probe a turn, from the stellium orbit
    (106, 'cargo', 'FUEL', 0, 200),
    (200, 'component', 'HDRV', 3, 2),
    (200, 'cargo', 'FUEL', 0, 5000);

INSERT INTO entity_population (entity_id, class, quantity) VALUES
    (100, 'SKW', 10), (101, 'USK', 40), (101, 'NAS', 5);
