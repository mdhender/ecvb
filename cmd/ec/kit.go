// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/mdhender/ecvb/internal/units"
	"github.com/mdhender/ecvb/internal/world"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

const uncontrolledAgentCode = "uncontrolled"

type kitSeed struct {
	ID       string              `json:"id"`
	Name     string              `json:"kit-name"`
	Entities map[string]kitAsset `json:"entities"`
}

type kitAsset struct {
	Kind        string           `json:"kind"`
	TechLevel   *int             `json:"tech-level"`
	Population  map[string]int64 `json:"population"`
	Cadres      map[string]int64 `json:"cadres"`
	Components  map[string]int64 `json:"components"`
	Operational map[string]int64 `json:"operational"`
	Cargo       map[string]int64 `json:"cargo"`
}

type preparedKit struct {
	name     string
	entities []preparedEntity
}

type preparedEntity struct {
	kind           string
	seedID         string
	techLevel      int
	controlled     bool
	mass           int64
	enclosedVolume int64
	population     map[string]int64
	cadres         map[string]int64
	inventory      []preparedInventory
}

type preparedInventory struct {
	section   string
	unit      string
	techLevel int
	quantity  int64
}

func readKit(path string) (preparedKit, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return preparedKit{}, fmt.Errorf("kit file %s does not exist", path)
		}
		return preparedKit{}, fmt.Errorf("stat kit file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return preparedKit{}, fmt.Errorf("kit file %s is not a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return preparedKit{}, fmt.Errorf("open kit file %s: %w", path, err)
	}
	defer file.Close()

	var seed kitSeed
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&seed); err != nil {
		return preparedKit{}, fmt.Errorf("parse kit file %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("unexpected additional JSON value")
		}
		return preparedKit{}, fmt.Errorf("parse kit file %s: %w", path, err)
	}
	kit, err := prepareKit(seed)
	if err != nil {
		return preparedKit{}, fmt.Errorf("validate kit file %s: %w", path, err)
	}
	return kit, nil
}

func prepareKit(seed kitSeed) (preparedKit, error) {
	if strings.TrimSpace(seed.Name) == "" {
		return preparedKit{}, fmt.Errorf("kit-name is required")
	}
	kit := preparedKit{name: seed.Name}
	ids := make([]string, 0, len(seed.Entities))
	for id := range seed.Entities {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if id == "" {
			return preparedKit{}, fmt.Errorf("entity id is required")
		}
		asset := seed.Entities[id]
		if asset.Kind != "COPN" && asset.Kind != "CSFC" && asset.Kind != "CORB" && asset.Kind != "SHIP" {
			return preparedKit{}, fmt.Errorf("entity %q has invalid kind %q", id, asset.Kind)
		}
		entity, err := prepareEntity(asset.Kind, id, asset)
		if err != nil {
			return preparedKit{}, err
		}
		kit.entities = append(kit.entities, entity)
	}
	if len(kit.entities) == 0 {
		return preparedKit{}, fmt.Errorf("kit has no entities")
	}
	return kit, nil
}

func prepareEntity(kind, id string, asset kitAsset) (preparedEntity, error) {
	if asset.TechLevel == nil {
		return preparedEntity{}, fmt.Errorf("%s entity %q requires a tech-level", kind, id)
	}
	if *asset.TechLevel < 0 || *asset.TechLevel > 10 {
		return preparedEntity{}, fmt.Errorf("%s entity %q has invalid tech-level %d", kind, id, *asset.TechLevel)
	}
	entity := preparedEntity{
		kind:       kind,
		seedID:     id,
		techLevel:  *asset.TechLevel,
		controlled: len(asset.Population) != 0,
		population: asset.Population,
		cadres:     asset.Cadres,
	}
	occupied := int64(0)
	for class, quantity := range asset.Population {
		if class != "USK" && class != "SKW" && class != "SOL" && class != "NAS" {
			return preparedEntity{}, fmt.Errorf("%s entity %q has unknown population class %q", kind, id, class)
		}
		if quantity <= 0 {
			return preparedEntity{}, fmt.Errorf("%s entity %q population %s must be positive", kind, id, class)
		}
		if err := addQuantity(&entity.mass, quantity, units.PopulationMetrics.Mass); err != nil {
			return preparedEntity{}, fmt.Errorf("%s entity %q population mass: %w", kind, id, err)
		}
		// Population in a starting kit is unassigned. Keep its cargo volume
		// independent from its operational and component volume definitions.
		if err := addQuantity(&occupied, quantity, units.PopulationMetrics.CargoVolume); err != nil {
			return preparedEntity{}, fmt.Errorf("%s entity %q population volume: %w", kind, id, err)
		}
	}

	seenInventory := make(map[string]bool)
	for _, section := range []struct {
		jsonName string
		dbName   string
		values   map[string]int64
	}{
		{jsonName: "components", dbName: "component", values: asset.Components},
		{jsonName: "operational", dbName: "operational", values: asset.Operational},
		{jsonName: "cargo", dbName: "cargo", values: asset.Cargo},
	} {
		tags := make([]string, 0, len(section.values))
		for tag := range section.values {
			tags = append(tags, tag)
		}
		sort.Strings(tags)
		for _, tag := range tags {
			quantity := section.values[tag]
			if quantity <= 0 {
				return preparedEntity{}, fmt.Errorf("%s entity %q %s unit %q must have a positive quantity", kind, id, section.jsonName, tag)
			}
			unit, techLevel, hasTechLevel, err := units.ParseTag(tag)
			if err != nil {
				return preparedEntity{}, fmt.Errorf("%s entity %q %s: %w", kind, id, section.jsonName, err)
			}
			if (unit == "STRC" || unit == "STRL") && !hasTechLevel {
				return preparedEntity{}, fmt.Errorf("%s entity %q structural unit %q requires a tech level", kind, id, tag)
			}
			key := section.dbName + "\x00" + unit + "\x00" + strconv.Itoa(techLevel)
			if seenInventory[key] {
				return preparedEntity{}, fmt.Errorf("%s entity %q has duplicate normalized inventory unit %q", kind, id, tag)
			}
			seenInventory[key] = true
			metrics := units.MetricsFor(unit, techLevel, hasTechLevel)
			if err := addQuantity(&entity.mass, quantity, metrics.Mass); err != nil {
				return preparedEntity{}, fmt.Errorf("%s entity %q %s unit %q mass: %w", kind, id, section.jsonName, tag, err)
			}
			if err := addQuantity(&entity.enclosedVolume, quantity,
				units.EnclosedVolumePerUnit(section.dbName, unit, techLevel)); err != nil {
				return preparedEntity{}, fmt.Errorf("%s entity %q enclosed volume: %w", kind, id, err)
			}
			if err := addQuantity(&occupied, quantity,
				units.OccupiedVolumePerUnit(kind, section.dbName, unit, techLevel, hasTechLevel)); err != nil {
				return preparedEntity{}, fmt.Errorf("%s entity %q %s unit %q volume: %w", kind, id, section.jsonName, tag, err)
			}
			entity.inventory = append(entity.inventory, preparedInventory{
				section: section.dbName, unit: unit, techLevel: techLevel, quantity: quantity,
			})
		}
	}

	if err := prepareCadres(kind, id, &entity); err != nil {
		return preparedEntity{}, err
	}
	enclosedSpace, err := units.UsableEnclosedSpace(kind, entity.enclosedVolume)
	if err != nil {
		return preparedEntity{}, fmt.Errorf("%s entity %q: %w", kind, id, err)
	}
	required, err := spaceWithTenPercentExcess(occupied)
	if err != nil {
		return preparedEntity{}, fmt.Errorf("%s entity %q occupied volume: %w", kind, id, err)
	}
	if enclosedSpace < required {
		return preparedEntity{}, fmt.Errorf("%s entity %q has %d VU enclosed space; need at least %d VU for %d VU occupied space", kind, id, enclosedSpace, required, occupied)
	}
	return entity, nil
}

// prepareCadres checks the population a kit has assigned to a cadre. A cadre is
// an assignment rather than a unit, so it adds no mass and no volume: the
// people it names are already in the entity's population, and this only says
// that the entity can put them to work.
//
// Only CWKR is accepted. The other four cadres have names and nothing else --
// docs/units.md says outright that what they permit is not settled -- so a kit
// that assigns one is asking for something the game cannot yet honour.
func prepareCadres(kind, id string, entity *preparedEntity) error {
	names := make([]string, 0, len(entity.cadres))
	for name := range entity.cadres {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		quantity := entity.cadres[name]
		if name != "CWKR" {
			return fmt.Errorf("%s entity %q cadre %q is not specified yet; only CWKR may be assigned", kind, id, name)
		}
		if quantity <= 0 {
			return fmt.Errorf("%s entity %q cadre %s must be positive", kind, id, name)
		}
		// One CWKR is one SKW plus one USK, so the entity has to carry both.
		for _, class := range []string{"SKW", "USK"} {
			if held := entity.population[class]; held < quantity {
				return fmt.Errorf("%s entity %q assigns %d %s and carries %d %s; one %s is one SKW plus one USK",
					kind, id, quantity, name, held, class, name)
			}
		}
	}
	return nil
}

func spaceWithTenPercentExcess(occupied int64) (int64, error) {
	extra := occupied / 10
	if occupied%10 != 0 {
		extra++
	}
	if occupied > math.MaxInt64-extra {
		return 0, fmt.Errorf("quantity overflow")
	}
	return occupied + extra, nil
}

func addQuantity(total *int64, quantity, perUnit int64) error {
	if quantity < 0 || perUnit < 0 || (perUnit != 0 && quantity > (math.MaxInt64-*total)/perUnit) {
		return fmt.Errorf("quantity overflow")
	}
	*total += quantity * perUnit
	return nil
}

func ensureUncontrolledFaction(conn *sqlite.Conn, gameID int64) (int64, error) {
	if err := sqlitex.ExecuteTransient(conn, `
		INSERT OR IGNORE INTO agent (code, description)
		VALUES (?, ?);`, &sqlitex.ExecOptions{Args: []any{uncontrolledAgentCode, "Generates basic orders for uncontrolled entities"}}); err != nil {
		return 0, fmt.Errorf("create uncontrolled agent: %w", err)
	}
	var agentID int64
	if err := sqlitex.ExecuteTransient(conn, "SELECT id FROM agent WHERE code = ?;", &sqlitex.ExecOptions{
		Args: []any{uncontrolledAgentCode},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			agentID = stmt.ColumnInt64(0)
			return nil
		},
	}); err != nil {
		return 0, fmt.Errorf("find uncontrolled agent: %w", err)
	}
	if agentID == 0 {
		return 0, fmt.Errorf("find uncontrolled agent: agent does not exist")
	}
	// The number is taken before the insert, and only when the insert will
	// happen: OR IGNORE means this runs once per game, and a counter bumped for
	// a row that was ignored would leave a gap in the game's faction numbers.
	var existing int64
	if err := sqlitex.ExecuteTransient(conn, "SELECT count(*) FROM faction WHERE game_id = ? AND agent_id = ?;", &sqlitex.ExecOptions{
		Args: []any{gameID, agentID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			existing = stmt.ColumnInt64(0)
			return nil
		},
	}); err != nil {
		return 0, fmt.Errorf("find uncontrolled faction: %w", err)
	}
	if existing == 0 {
		number, err := world.NextFactionNumber(conn, gameID)
		if err != nil {
			return 0, err
		}
		if err := sqlitex.ExecuteTransient(conn, `
			INSERT INTO faction (game_id, number, agent_id)
			VALUES (?, ?, ?);`, &sqlitex.ExecOptions{Args: []any{gameID, number, agentID}}); err != nil {
			return 0, fmt.Errorf("create uncontrolled faction: %w", err)
		}
	}
	var factionID int64
	if err := sqlitex.ExecuteTransient(conn, "SELECT id FROM faction WHERE game_id = ? AND agent_id = ?;", &sqlitex.ExecOptions{
		Args: []any{gameID, agentID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			factionID = stmt.ColumnInt64(0)
			return nil
		},
	}); err != nil {
		return 0, fmt.Errorf("find uncontrolled faction: %w", err)
	}
	if factionID == 0 {
		return 0, fmt.Errorf("find uncontrolled faction: faction does not exist")
	}
	return factionID, nil
}

func insertKit(conn *sqlite.Conn, kit preparedKit, home homeCandidate, gameID, playerFactionID, uncontrolledFactionID int64) error {
	for _, entity := range kit.entities {
		factionID := playerFactionID
		if !entity.controlled {
			factionID = uncontrolledFactionID
		}
		planetRing := 0
		switch entity.kind {
		case "CORB":
			planetRing = 1
		case "SHIP":
			planetRing = 64
		}
		// The kit's own string ids name the entities to each other while the
		// kit is being read; what the player will call them is the game's
		// number, taken here, and the two never meet.
		number, err := world.NextEntityNumber(conn, gameID)
		if err != nil {
			return err
		}
		if err := sqlitex.ExecuteTransient(conn, `
			INSERT INTO entity (
				game_id, number, unit, tech_level, stellium_id, system_id, planet_id, planet_ring,
				faction_id, enclosed_volume, mass
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`, &sqlitex.ExecOptions{Args: []any{
			gameID, number, entity.kind, entity.techLevel, home.stelliumID, home.systemID, home.planetID,
			planetRing, factionID, entity.enclosedVolume, entity.mass,
		}}); err != nil {
			return fmt.Errorf("insert %s entity %q: %w", entity.kind, entity.seedID, err)
		}
		entityID := conn.LastInsertRowID()
		for _, item := range entity.inventory {
			if err := sqlitex.ExecuteTransient(conn, `
				INSERT INTO inventory (entity_id, section, unit, tech_level, quantity)
				VALUES (?, ?, ?, ?, ?);`, &sqlitex.ExecOptions{Args: []any{
				entityID, item.section, item.unit, item.techLevel, item.quantity,
			}}); err != nil {
				return fmt.Errorf("insert %s entity %q inventory: %w", entity.kind, entity.seedID, err)
			}
		}
		cadres := make([]string, 0, len(entity.cadres))
		for name := range entity.cadres {
			cadres = append(cadres, name)
		}
		sort.Strings(cadres)
		for _, name := range cadres {
			if err := sqlitex.ExecuteTransient(conn, `
				INSERT INTO entity_cadre (entity_id, cadre, quantity)
				VALUES (?, ?, ?);`, &sqlitex.ExecOptions{Args: []any{
				entityID, name, entity.cadres[name],
			}}); err != nil {
				return fmt.Errorf("insert %s entity %q cadre: %w", entity.kind, entity.seedID, err)
			}
		}
		classes := make([]string, 0, len(entity.population))
		for class := range entity.population {
			classes = append(classes, class)
		}
		sort.Strings(classes)
		for _, class := range classes {
			if err := sqlitex.ExecuteTransient(conn, `
				INSERT INTO entity_population (entity_id, class, quantity)
				VALUES (?, ?, ?);`, &sqlitex.ExecOptions{Args: []any{
				entityID, class, entity.population[class],
			}}); err != nil {
				return fmt.Errorf("insert %s entity %q population: %w", entity.kind, entity.seedID, err)
			}
		}
	}
	return nil
}
