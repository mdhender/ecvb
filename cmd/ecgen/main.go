// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/mdhender/ecvb/internal/dotenv"
	"github.com/peterbourgon/ff/v4"
)

const stelliaSeedFilename = "stellia-seed.json"
const systemsSeedFilename = "systems-seed.json"
const planetsSeedFilename = "planets-seed.json"
const depositsSeedFilename = "deposits-seed.json"

func main() {
	env, ok := os.LookupEnv("EC_ENV")
	if !ok {
		env = "development"
	}
	if err := dotenv.Load(env); err != nil {
		fmt.Fprintf(os.Stderr, "ecgen: %v\n", err)
		os.Exit(1)
	}

	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "ecgen: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	root := &ff.Command{
		Name:      "ecgen",
		Usage:     "ecgen SUBCOMMAND",
		ShortHelp: "generate ECVB seed files",
		Exec: func(context.Context, []string) error {
			return fmt.Errorf("a subcommand is required (stellia, systems, planets, deposits)")
		},
	}

	stelliaFlags := ff.NewFlagSet("stellia")
	seed := stelliaFlags.Int64Long("stellia-seed", 1912, "seed for deterministic stellia generation")
	root.Subcommands = []*ff.Command{
		{
			Name:      "stellia",
			Usage:     "ecgen stellia [--stellia-seed SEED] <path>",
			ShortHelp: "generate 100 stellia",
			Flags:     stelliaFlags,
			Exec: func(_ context.Context, args []string) error {
				if len(args) != 1 {
					return fmt.Errorf("expected exactly one directory path")
				}
				return generateStellia(args[0], *seed)
			},
		},
		{
			Name:      "systems",
			Usage:     "ecgen systems <path>",
			ShortHelp: "generate systems from stellia-seed.json",
			Exec: func(_ context.Context, args []string) error {
				if len(args) != 1 {
					return fmt.Errorf("expected exactly one directory path")
				}
				return generateSystems(args[0])
			},
		},
		{
			Name:      "planets",
			Usage:     "ecgen planets <path>",
			ShortHelp: "generate planets from systems-seed.json",
			Exec: func(_ context.Context, args []string) error {
				if len(args) != 1 {
					return fmt.Errorf("expected exactly one directory path")
				}
				return generatePlanets(args[0])
			},
		},
		{
			Name:      "deposits",
			Usage:     "ecgen deposits <path>",
			ShortHelp: "generate deposits from planets-seed.json",
			Exec: func(_ context.Context, args []string) error {
				if len(args) != 1 {
					return fmt.Errorf("expected exactly one directory path")
				}
				return generateDeposits(args[0])
			},
		},
	}

	return root.ParseAndRun(ctx, args, ff.WithEnvVarPrefix("EC"))
}

type stelliaSeed struct {
	Seed    pcgSeed    `json:"seed"`
	Stellia []stellium `json:"stellia"`
}

type pcgSeed struct {
	High string `json:"high"`
	Lo   string `json:"lo"`
}

type stellium struct {
	UUID        string `json:"uuid"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Z           int    `json:"z"`
	SystemCount int    `json:"system-count"`
}

type systemsSeed struct {
	Systems []system `json:"systems"`
}

type system struct {
	UUID         string `json:"uuid"`
	StelliumUUID string `json:"stellium-uuid"`
	Sequence     string `json:"sequence"`
}

type planetsSeed struct {
	Planets []planet `json:"planets"`
}

type planet struct {
	UUID         string `json:"uuid"`
	SystemUUID   string `json:"system-uuid"`
	Orbit        int    `json:"orbit"`
	Type         string `json:"type"`
	Habitability int    `json:"habitability"`
}

type depositsSeed struct {
	Deposits []deposit `json:"deposits"`
}

type deposit struct {
	PlanetUUID string `json:"planet-uuid"`
	Sequence   int    `json:"sequence"`
	Resource   string `json:"resource"`
	Quantity   int    `json:"quantity"`
	Quality    int    `json:"quality"`
}

func generateStellia(directory string, seed int64) (err error) {
	info, err := os.Stat(directory)
	if err != nil {
		return fmt.Errorf("stat output directory %s: %w", directory, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("output path %s is not a directory", directory)
	}

	path := filepath.Join(directory, stelliaSeedFilename)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("output file %s already exists", path)
		}
		return fmt.Errorf("create output file %s: %w", path, err)
	}
	complete := false
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close output file %s: %w", path, closeErr)
		}
		if !complete {
			_ = os.Remove(path)
		}
	}()

	data := newStelliaSeed(seed)
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("write output file %s: %w", path, err)
	}
	complete = true
	return nil
}

func newStelliaSeed(seed int64) stelliaSeed {
	state := uint64(seed)
	high, lo := splitMix64(&state), splitMix64(&state)
	rng := rand.New(rand.NewPCG(high, lo))

	locations := make([]stellium, 0, 31*31*31-1)
	for x := -15; x <= 15; x++ {
		for y := -15; y <= 15; y++ {
			for z := -15; z <= 15; z++ {
				if x != 0 || y != 0 || z != 0 {
					locations = append(locations, stellium{X: x, Y: y, Z: z})
				}
			}
		}
	}
	rng.Shuffle(len(locations), func(i, j int) {
		locations[i], locations[j] = locations[j], locations[i]
	})
	selected := make([]stellium, 0, 100)
	for _, candidate := range locations {
		tooClose := false
		for _, existing := range selected {
			dx, dy, dz := candidate.X-existing.X, candidate.Y-existing.Y, candidate.Z-existing.Z
			if dx*dx+dy*dy+dz*dz < 4 {
				tooClose = true
				break
			}
		}
		if tooClose {
			continue
		}
		switch len(selected) {
		case 0:
			candidate.SystemCount = 4
		case 1, 2:
			candidate.SystemCount = 3
		case 3, 4, 5, 6, 7, 8, 9:
			candidate.SystemCount = 2
		default:
			candidate.SystemCount = 1
		}
		selected = append(selected, candidate)
		if len(selected) == 100 {
			break
		}
	}
	slices.SortFunc(selected, func(a, b stellium) int {
		if n := a.X - b.X; n != 0 {
			return n
		}
		if n := a.Y - b.Y; n != 0 {
			return n
		}
		return a.Z - b.Z
	})
	for i := range selected {
		selected[i].UUID = randomUUID(rng)
	}

	return stelliaSeed{
		Seed: pcgSeed{
			High: strconv.FormatInt(int64(high), 10),
			Lo:   strconv.FormatInt(int64(lo), 10),
		},
		Stellia: selected,
	}
}

func generateSystems(directory string) (err error) {
	inputPath := filepath.Join(directory, stelliaSeedFilename)
	input, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open input file %s: %w", inputPath, err)
	}
	defer input.Close()

	var stellia stelliaSeed
	decoder := json.NewDecoder(input)
	if err := decoder.Decode(&stellia); err != nil {
		return fmt.Errorf("parse input file %s: %w", inputPath, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("unexpected additional JSON value")
		}
		return fmt.Errorf("parse input file %s: %w", inputPath, err)
	}

	data := systemsSeed{Systems: make([]system, 0, len(stellia.Stellia))}
	seenStellia := make(map[string]bool, len(stellia.Stellia))
	for i, s := range stellia.Stellia {
		if s.UUID == "" {
			return fmt.Errorf("parse input file %s: stellia[%d] has no uuid", inputPath, i)
		}
		if seenStellia[s.UUID] {
			return fmt.Errorf("parse input file %s: stellia[%d] has duplicate uuid %q", inputPath, i, s.UUID)
		}
		seenStellia[s.UUID] = true
		if s.SystemCount < 1 || s.SystemCount > 5 {
			return fmt.Errorf("parse input file %s: stellia[%d] system-count %d is outside 1 through 5", inputPath, i, s.SystemCount)
		}
		for sequence := 0; sequence < s.SystemCount; sequence++ {
			letter := string(rune('A' + sequence))
			data.Systems = append(data.Systems, system{
				UUID:         systemUUID(s.UUID, letter),
				StelliumUUID: s.UUID,
				Sequence:     letter,
			})
		}
	}

	outputPath := filepath.Join(directory, systemsSeedFilename)
	output, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("output file %s already exists", outputPath)
		}
		return fmt.Errorf("create output file %s: %w", outputPath, err)
	}
	complete := false
	defer func() {
		if closeErr := output.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close output file %s: %w", outputPath, closeErr)
		}
		if !complete {
			_ = os.Remove(outputPath)
		}
	}()

	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("write output file %s: %w", outputPath, err)
	}
	complete = true
	return nil
}

func systemUUID(stelliumUUID, sequence string) string {
	id := sha256.Sum256([]byte("ecvb/system/" + stelliumUUID + "/" + sequence))
	id[6] = id[6]&0x0f | 0x40
	id[8] = id[8]&0x3f | 0x80
	return formatUUID([16]byte(id[:16]))
}

func generatePlanets(directory string) (err error) {
	inputPath := filepath.Join(directory, systemsSeedFilename)
	input, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open input file %s: %w", inputPath, err)
	}
	defer input.Close()

	var systems systemsSeed
	decoder := json.NewDecoder(input)
	if err := decoder.Decode(&systems); err != nil {
		return fmt.Errorf("parse input file %s: %w", inputPath, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("unexpected additional JSON value")
		}
		return fmt.Errorf("parse input file %s: %w", inputPath, err)
	}

	types := [...]string{
		"rocky", "rocky", "rocky", "rocky", "asteroid",
		"gas-giant", "ice-giant", "ice-giant", "ice-giant", "asteroid",
	}
	habitability := [...]int{0, 1, 8, 25, 0, 15, 4, 2, 0, 0}
	data := planetsSeed{Planets: make([]planet, 0, len(systems.Systems)*len(types))}
	seenSystems := make(map[string]bool, len(systems.Systems))
	for i, system := range systems.Systems {
		if system.UUID == "" {
			return fmt.Errorf("parse input file %s: systems[%d] has no uuid", inputPath, i)
		}
		if seenSystems[system.UUID] {
			return fmt.Errorf("parse input file %s: systems[%d] has duplicate uuid %q", inputPath, i, system.UUID)
		}
		seenSystems[system.UUID] = true
		for index := range types {
			orbit := index + 1
			data.Planets = append(data.Planets, planet{
				UUID:         planetUUID(system.UUID, orbit),
				SystemUUID:   system.UUID,
				Orbit:        orbit,
				Type:         types[index],
				Habitability: habitability[index],
			})
		}
	}

	outputPath := filepath.Join(directory, planetsSeedFilename)
	output, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("output file %s already exists", outputPath)
		}
		return fmt.Errorf("create output file %s: %w", outputPath, err)
	}
	complete := false
	defer func() {
		if closeErr := output.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close output file %s: %w", outputPath, closeErr)
		}
		if !complete {
			_ = os.Remove(outputPath)
		}
	}()

	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("write output file %s: %w", outputPath, err)
	}
	complete = true
	return nil
}

func planetUUID(systemUUID string, orbit int) string {
	id := sha256.Sum256([]byte("ecvb/planet/" + systemUUID + "/" + strconv.Itoa(orbit)))
	id[6] = id[6]&0x0f | 0x40
	id[8] = id[8]&0x3f | 0x80
	return formatUUID([16]byte(id[:16]))
}

func generateDeposits(directory string) (err error) {
	inputPath := filepath.Join(directory, planetsSeedFilename)
	input, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open input file %s: %w", inputPath, err)
	}
	defer input.Close()

	var planets planetsSeed
	decoder := json.NewDecoder(input)
	if err := decoder.Decode(&planets); err != nil {
		return fmt.Errorf("parse input file %s: %w", inputPath, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("unexpected additional JSON value")
		}
		return fmt.Errorf("parse input file %s: %w", inputPath, err)
	}

	data := depositsSeed{Deposits: make([]deposit, 0, len(planets.Planets)*20)}
	seenPlanets := make(map[string]bool, len(planets.Planets))
	for i, planet := range planets.Planets {
		if planet.UUID == "" {
			return fmt.Errorf("parse input file %s: planets[%d] has no uuid", inputPath, i)
		}
		if seenPlanets[planet.UUID] {
			return fmt.Errorf("parse input file %s: planets[%d] has duplicate uuid %q", inputPath, i, planet.UUID)
		}
		seenPlanets[planet.UUID] = true

		count, sides := 15, 6
		switch planet.Type {
		case "rocky":
			count, sides = 20, 3
		case "asteroid":
			count, sides = 35, 100
		case "gas-giant", "ice-giant":
		default:
			return fmt.Errorf("parse input file %s: planets[%d] has invalid type %q", inputPath, i, planet.Type)
		}
		for sequence := 1; sequence <= count; sequence++ {
			roll := depositRoll(planet.UUID, sequence, sides)
			data.Deposits = append(data.Deposits, deposit{
				PlanetUUID: planet.UUID,
				Sequence:   sequence,
				Resource:   depositResource(planet.Type, roll),
				Quantity:   10_000_000,
				Quality:    5,
			})
		}
	}

	outputPath := filepath.Join(directory, depositsSeedFilename)
	output, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("output file %s already exists", outputPath)
		}
		return fmt.Errorf("create output file %s: %w", outputPath, err)
	}
	complete := false
	defer func() {
		if closeErr := output.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close output file %s: %w", outputPath, closeErr)
		}
		if !complete {
			_ = os.Remove(outputPath)
		}
	}()

	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("write output file %s: %w", outputPath, err)
	}
	complete = true
	return nil
}

func depositRoll(planetUUID string, sequence, sides int) int {
	digest := sha256.Sum256([]byte("ecvb/deposit/" + planetUUID + "/" + strconv.Itoa(sequence)))
	return int(binary.BigEndian.Uint64(digest[:8])%uint64(sides)) + 1
}

func depositResource(planetType string, roll int) string {
	switch planetType {
	case "asteroid":
		if roll <= 4 {
			return "gold"
		}
		if roll >= 6 && roll <= 52 {
			return "metals"
		}
	case "rocky":
		if roll == 1 {
			return "fuel"
		}
		if roll == 2 {
			return "metals"
		}
	case "gas-giant":
		if roll <= 3 {
			return "fuel"
		}
		if roll == 4 {
			return "metals"
		}
	case "ice-giant":
		if roll <= 2 {
			return "fuel"
		}
		if roll == 3 {
			return "metals"
		}
	}
	return "minerals"
}

func splitMix64(state *uint64) uint64 {
	*state += 0x9e3779b97f4a7c15
	z := *state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func randomUUID(rng *rand.Rand) string {
	var id [16]byte
	binary.BigEndian.PutUint64(id[:8], rng.Uint64())
	binary.BigEndian.PutUint64(id[8:], rng.Uint64())
	id[6] = id[6]&0x0f | 0x40
	id[8] = id[8]&0x3f | 0x80
	return formatUUID(id)
}

func formatUUID(id [16]byte) string {
	var text [36]byte
	hex.Encode(text[0:8], id[0:4])
	text[8] = '-'
	hex.Encode(text[9:13], id[4:6])
	text[13] = '-'
	hex.Encode(text[14:18], id[6:8])
	text[18] = '-'
	hex.Encode(text[19:23], id[8:10])
	text[23] = '-'
	hex.Encode(text[24:36], id[10:16])
	return string(text[:])
}
