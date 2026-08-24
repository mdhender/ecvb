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

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

const uncontrolledAgentCode = "uncontrolled"

type kitSeed struct {
	ID       string                         `json:"id"`
	Name     string                         `json:"kit-name"`
	Colonies map[string]map[string]kitAsset `json:"colonies"`
	Ships    map[string]kitAsset            `json:"SHIP"`
}

type kitAsset struct {
	TechLevel   *int             `json:"tech-level"`
	Population  map[string]int64 `json:"population"`
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
	inventory      []preparedInventory
}

type preparedInventory struct {
	section   string
	unit      string
	techLevel int
	quantity  int64
}

type unitMetrics struct {
	mass              int64
	cargoVolume       int64
	operationalVolume int64
	componentVolume   int64
}

var populationMetrics = unitMetrics{
	mass:              2,
	cargoVolume:       2,
	operationalVolume: 2,
	componentVolume:   2,
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
	for kind := range seed.Colonies {
		if kind != "COPN" && kind != "CSFC" && kind != "CORB" {
			return preparedKit{}, fmt.Errorf("unknown colony unit %q", kind)
		}
	}

	kit := preparedKit{name: seed.Name}
	seenIDs := make(map[string]bool)
	for _, group := range []struct {
		kind   string
		assets map[string]kitAsset
	}{
		{kind: "COPN", assets: seed.Colonies["COPN"]},
		{kind: "CSFC", assets: seed.Colonies["CSFC"]},
		{kind: "CORB", assets: seed.Colonies["CORB"]},
		{kind: "SHIP", assets: seed.Ships},
	} {
		ids := make([]string, 0, len(group.assets))
		for id := range group.assets {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			if id == "" || seenIDs[id] {
				return preparedKit{}, fmt.Errorf("%s has missing or duplicate entity id %q", group.kind, id)
			}
			seenIDs[id] = true
			entity, err := prepareEntity(group.kind, id, group.assets[id])
			if err != nil {
				return preparedKit{}, err
			}
			kit.entities = append(kit.entities, entity)
		}
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
	}
	occupied := int64(0)
	for class, quantity := range asset.Population {
		if class != "USK" && class != "SKW" && class != "SOL" && class != "NAS" {
			return preparedEntity{}, fmt.Errorf("%s entity %q has unknown population class %q", kind, id, class)
		}
		if quantity <= 0 {
			return preparedEntity{}, fmt.Errorf("%s entity %q population %s must be positive", kind, id, class)
		}
		if err := addQuantity(&entity.mass, quantity, populationMetrics.mass); err != nil {
			return preparedEntity{}, fmt.Errorf("%s entity %q population mass: %w", kind, id, err)
		}
		// Population in a starting kit is unassigned. Keep its cargo volume
		// independent from its operational and component volume definitions.
		if err := addQuantity(&occupied, quantity, populationMetrics.cargoVolume); err != nil {
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
			unit, techLevel, hasTechLevel, err := parseUnitTag(tag)
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
			metrics := metricsForTechLevel(techLevel, hasTechLevel)
			if err := addQuantity(&entity.mass, quantity, metrics.mass); err != nil {
				return preparedEntity{}, fmt.Errorf("%s entity %q %s unit %q mass: %w", kind, id, section.jsonName, tag, err)
			}
			if section.dbName == "component" && (unit == "STRC" || unit == "STRL") {
				perUnit := int64(techLevel * techLevel)
				if err := addQuantity(&entity.enclosedVolume, quantity, perUnit); err != nil {
					return preparedEntity{}, fmt.Errorf("%s entity %q enclosed volume: %w", kind, id, err)
				}
			} else if !(section.dbName == "cargo" && (kind == "COPN" || kind == "CORB") && isDepotResource(unit)) {
				volume := metrics.cargoVolume
				switch section.dbName {
				case "component":
					volume = metrics.componentVolume
				case "operational":
					volume = metrics.operationalVolume
				}
				if err := addQuantity(&occupied, quantity, volume); err != nil {
					return preparedEntity{}, fmt.Errorf("%s entity %q %s unit %q volume: %w", kind, id, section.jsonName, tag, err)
				}
			}
			entity.inventory = append(entity.inventory, preparedInventory{
				section: section.dbName, unit: unit, techLevel: techLevel, quantity: quantity,
			})
		}
	}

	enclosedSpace := usableEnclosedSpace(kind, entity.enclosedVolume)
	required, err := spaceWithTenPercentExcess(occupied)
	if err != nil {
		return preparedEntity{}, fmt.Errorf("%s entity %q occupied volume: %w", kind, id, err)
	}
	if enclosedSpace < required {
		return preparedEntity{}, fmt.Errorf("%s entity %q has %d VU enclosed space; need at least %d VU for %d VU occupied space", kind, id, enclosedSpace, required, occupied)
	}
	return entity, nil
}

func parseUnitTag(tag string) (unit string, techLevel int, hasTechLevel bool, err error) {
	if tag == "" {
		return "", 0, false, fmt.Errorf("unit code is required")
	}
	unit = tag
	if dash := strings.LastIndexByte(tag, '-'); dash >= 0 {
		unit = tag[:dash]
		if unit == "" || dash == len(tag)-1 {
			return "", 0, false, fmt.Errorf("invalid unit tag %q", tag)
		}
		techLevel, err = strconv.Atoi(tag[dash+1:])
		if err != nil || techLevel < 0 || techLevel > 10 {
			return "", 0, false, fmt.Errorf("invalid unit tag %q", tag)
		}
		hasTechLevel = true
	}
	for _, r := range unit {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return "", 0, false, fmt.Errorf("invalid unit code %q", unit)
		}
	}
	return unit, techLevel, hasTechLevel, nil
}

func metricsForTechLevel(techLevel int, hasTechLevel bool) unitMetrics {
	base := int64(6)
	if hasTechLevel {
		base = int64(2 * techLevel)
	}
	return unitMetrics{
		mass:              base,
		cargoVolume:       base,
		operationalVolume: 2 * base,
		componentVolume:   4 * base,
	}
}

func isDepotResource(unit string) bool {
	return unit == "GOLD" || unit == "FUEL" || unit == "METL" || unit == "MNRL"
}

func usableEnclosedSpace(kind string, enclosedVolume int64) int64 {
	switch kind {
	case "COPN":
		return enclosedVolume
	case "CSFC":
		return enclosedVolume / 5
	case "CORB", "SHIP":
		return enclosedVolume / 10
	default:
		panic("unknown entity kind " + kind)
	}
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
	if err := sqlitex.ExecuteTransient(conn, `
		INSERT OR IGNORE INTO faction (game_id, agent_id)
		VALUES (?, ?);`, &sqlitex.ExecOptions{Args: []any{gameID, agentID}}); err != nil {
		return 0, fmt.Errorf("create uncontrolled faction: %w", err)
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

func insertKit(conn *sqlite.Conn, kit preparedKit, home homeCandidate, playerFactionID, uncontrolledFactionID int64) error {
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
		if err := sqlitex.ExecuteTransient(conn, `
			INSERT INTO entity (
				unit, tech_level, stellium_id, system_id, planet_id, planet_ring,
				faction_id, enclosed_volume, mass
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);`, &sqlitex.ExecOptions{Args: []any{
			entity.kind, entity.techLevel, home.stelliumID, home.systemID, home.planetID,
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
