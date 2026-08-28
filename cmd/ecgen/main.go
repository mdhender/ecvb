// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/mdhender/ecvb/internal/dotenv"
	"github.com/mdhender/ecvb/internal/mapkey"
	"github.com/mdhender/ecvb/internal/prng"
	"github.com/peterbourgon/ff/v4"
)

const stelliaSeedFilename = "stellia-seed.json"
const systemsSeedFilename = "systems-seed.json"
const planetsSeedFilename = "planets-seed.json"
const depositsSeedFilename = "deposits-seed.json"

// Each stage roots its draws at Derive(stageTag, generatorID, version) and owns
// its addressing below that root. The version is the knob that lets a stage's
// rules change without touching prng's frozen tag registry: bump it and every
// draw of that stage moves, while a map generated under the old version stays
// reproducible by the code that still writes it.
//
// Bump version when what a stage DECIDES changes. Leave it alone for a
// refactor -- the whole point of addressing a draw is that moving the lines
// around cannot move the map.
const (
	genStellia  prng.Key = 1
	genSystems  prng.Key = 1
	genPlanets  prng.Key = 1
	genDeposits prng.Key = 1

	versionStellia  prng.Key = 1
	versionSystems  prng.Key = 1
	versionPlanets  prng.Key = 1
	versionDeposits prng.Key = 1
)

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

// pcgSeed is the map's two master seeds, rendered as unsigned decimal strings
// because JSON numbers cannot carry a uint64 without losing the top bits.
//
// Every stage copies this block forward from its input, and every stage
// rebuilds the same prng.Seeds from it. That is what makes the file, rather
// than the --stellia-seed flag, the authority on how a map was drawn: the flag
// is read once, by `ecgen stellia`.
type pcgSeed struct {
	High string `json:"high"`
	Lo   string `json:"lo"`
}

func newPCGSeed(high, lo uint64) pcgSeed {
	return pcgSeed{High: strconv.FormatUint(high, 10), Lo: strconv.FormatUint(lo, 10)}
}

// seeds rebuilds the prng root this pair addresses.
func (p pcgSeed) seeds() (prng.Seeds, error) {
	if p.High == "" || p.Lo == "" {
		return prng.Seeds{}, fmt.Errorf("has no seed block; it was not written by ecgen")
	}
	high, err := strconv.ParseUint(p.High, 10, 64)
	if err != nil {
		return prng.Seeds{}, fmt.Errorf("has seed high %q, which is not a uint64", p.High)
	}
	lo, err := strconv.ParseUint(p.Lo, 10, 64)
	if err != nil {
		return prng.Seeds{}, fmt.Errorf("has seed lo %q, which is not a uint64", p.Lo)
	}
	return prng.New(high, lo), nil
}

// generatorInfo records which stage wrote a file and under which rules, so that
// a map carries the version that produced it rather than leaving a reader to
// guess from the repository's history.
type generatorInfo struct {
	Stage   string   `json:"stage"`
	ID      prng.Key `json:"id"`
	Version prng.Key `json:"version"`
}

// Every record below is keyed by its own prng address -- a stellium by its
// coordinates, a system by its stellium and sequence, and so on down. There are
// no UUIDs: the address is the join key AND the thing a draw is addressed by, so
// the two cannot drift apart.

type stelliaSeed struct {
	Seed      pcgSeed       `json:"seed"`
	Generator generatorInfo `json:"generator"`
	Stellia   []stellium    `json:"stellia"`
}

type stellium struct {
	X           int `json:"x"`
	Y           int `json:"y"`
	Z           int `json:"z"`
	SystemCount int `json:"system-count"`
}

type systemsSeed struct {
	Seed      pcgSeed       `json:"seed"`
	Generator generatorInfo `json:"generator"`
	Systems   []system      `json:"systems"`
}

type system struct {
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Z        int    `json:"z"`
	Sequence string `json:"sequence"`
}

type planetsSeed struct {
	Seed      pcgSeed       `json:"seed"`
	Generator generatorInfo `json:"generator"`
	Planets   []planet      `json:"planets"`
}

type planet struct {
	X            int    `json:"x"`
	Y            int    `json:"y"`
	Z            int    `json:"z"`
	Sequence     string `json:"sequence"`
	Orbit        int    `json:"orbit"`
	Type         string `json:"type"`
	Habitability int    `json:"habitability"`
}

type depositsSeed struct {
	Seed      pcgSeed       `json:"seed"`
	Generator generatorInfo `json:"generator"`
	Deposits  []deposit     `json:"deposits"`
}

// A deposit's own ordinal on its planet is "deposit-no" and not "sequence":
// "sequence" is the system's A-through-E letter, and both now appear in one
// record. The database column is still deposit.sequence; cmd/ec/load.go is the
// single place the two names meet.
type deposit struct {
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Z         int    `json:"z"`
	Sequence  string `json:"sequence"`
	Orbit     int    `json:"orbit"`
	DepositNo int    `json:"deposit-no"`
	Resource  string `json:"resource"`
	Quantity  int    `json:"quantity"`
	Quality   int    `json:"quality"`
}

func generateStellia(directory string, seed int64) error {
	info, err := os.Stat(directory)
	if err != nil {
		return fmt.Errorf("stat output directory %s: %w", directory, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("output path %s is not a directory", directory)
	}
	return writeSeedFile(directory, stelliaSeedFilename, newStelliaSeed(seed))
}

// newStelliaSeed picks the 100 stellia of a map.
//
// Every point of the lattice is given a placement key drawn at its own address,
// and the points are taken in that key's order. The key is a pure function of
// the seeds and the coordinates, so the order the map comes out in does not
// depend on the order anything is computed -- which is the whole reason this
// draws from internal/prng at all. The generator before this one shuffled one
// sequential stream and then drew a hundred UUIDs from the same stream, so
// inserting a single draw anywhere in this function silently rewrote the map.
//
// A shuffle would have been the smaller change and is what Roller.Shuffle is
// there for, but it is still one stream: any later draw taken from it would
// move every stellium. A key per point cannot be broken that way, and thirty
// thousand hashes cost nothing against a stage that writes a megabyte of JSON.
func newStelliaSeed(seed int64) stelliaSeed {
	state := uint64(seed)
	high, lo := splitMix64(&state), splitMix64(&state)
	root := prng.New(high, lo).Derive(prng.TagCluster, genStellia, versionStellia)

	type candidate struct {
		stellium
		key uint64
	}
	locations := make([]candidate, 0, 31*31*31-1)
	for x := mapkey.MinCoordinate; x <= mapkey.MaxCoordinate; x++ {
		for y := mapkey.MinCoordinate; y <= mapkey.MaxCoordinate; y++ {
			for z := mapkey.MinCoordinate; z <= mapkey.MaxCoordinate; z++ {
				if x == 0 && y == 0 && z == 0 {
					continue
				}
				locations = append(locations, candidate{
					stellium: stellium{X: x, Y: y, Z: z},
					key:      root.Stream(mapkey.Stellium(x, y, z)...).Uint64(),
				})
			}
		}
	}
	// Ties break on coordinates so the order is total: two points sharing a key
	// is vanishingly unlikely, but "vanishingly unlikely" is not "reproducible".
	slices.SortFunc(locations, func(a, b candidate) int {
		if a.key < b.key {
			return -1
		} else if a.key > b.key {
			return 1
		}
		return compareStellia(a.stellium, b.stellium)
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
		next := candidate.stellium
		switch len(selected) {
		case 0:
			next.SystemCount = 4
		case 1, 2:
			next.SystemCount = 3
		case 3, 4, 5, 6, 7, 8, 9:
			next.SystemCount = 2
		default:
			next.SystemCount = 1
		}
		selected = append(selected, next)
		if len(selected) == 100 {
			break
		}
	}
	slices.SortFunc(selected, compareStellia)

	return stelliaSeed{
		Seed:      newPCGSeed(high, lo),
		Generator: generatorInfo{Stage: "stellia", ID: genStellia, Version: versionStellia},
		Stellia:   selected,
	}
}

func compareStellia(a, b stellium) int {
	if n := a.X - b.X; n != 0 {
		return n
	}
	if n := a.Y - b.Y; n != 0 {
		return n
	}
	return a.Z - b.Z
}

// stelliumKey, systemKey, and planetKey are the address tuples the stages join
// and deduplicate on. They are comparable, so they are map keys directly.
type stelliumKey struct{ x, y, z int }
type systemKey struct {
	stelliumKey
	sequence string
}
type planetKey struct {
	systemKey
	orbit int
}

func (k stelliumKey) String() string { return fmt.Sprintf("(%d,%d,%d)", k.x, k.y, k.z) }
func (k systemKey) String() string   { return fmt.Sprintf("system %s at %s", k.sequence, k.stelliumKey) }
func (k planetKey) String() string   { return fmt.Sprintf("orbit %d of %s", k.orbit, k.systemKey) }

func generateSystems(directory string) error {
	inputPath := filepath.Join(directory, stelliaSeedFilename)
	var stellia stelliaSeed
	if err := readSeedFile(inputPath, &stellia); err != nil {
		return err
	}
	if _, err := stellia.Seed.seeds(); err != nil {
		return fmt.Errorf("parse input file %s: %w", inputPath, err)
	}

	data := systemsSeed{
		Seed:      stellia.Seed,
		Generator: generatorInfo{Stage: "systems", ID: genSystems, Version: versionSystems},
		Systems:   make([]system, 0, len(stellia.Stellia)),
	}
	seen := make(map[stelliumKey]bool, len(stellia.Stellia))
	for i, s := range stellia.Stellia {
		key := stelliumKey{s.X, s.Y, s.Z}
		if err := checkCoordinates(s.X, s.Y, s.Z); err != nil {
			return fmt.Errorf("parse input file %s: stellia[%d] %w", inputPath, i, err)
		}
		if seen[key] {
			return fmt.Errorf("parse input file %s: stellia[%d] repeats the stellium at %s", inputPath, i, key)
		}
		seen[key] = true
		if s.SystemCount < 1 || s.SystemCount > 5 {
			return fmt.Errorf("parse input file %s: stellia[%d] system-count %d is outside 1 through 5", inputPath, i, s.SystemCount)
		}
		for sequence := 0; sequence < s.SystemCount; sequence++ {
			data.Systems = append(data.Systems, system{
				X: s.X, Y: s.Y, Z: s.Z,
				Sequence: string(rune('A' + sequence)),
			})
		}
	}
	return writeSeedFile(directory, systemsSeedFilename, data)
}

func generatePlanets(directory string) error {
	inputPath := filepath.Join(directory, systemsSeedFilename)
	var systems systemsSeed
	if err := readSeedFile(inputPath, &systems); err != nil {
		return err
	}
	if _, err := systems.Seed.seeds(); err != nil {
		return fmt.Errorf("parse input file %s: %w", inputPath, err)
	}

	// A system's planets are a fixed template rather than a draw: orbit decides
	// type and habitability, and always has.
	types := [...]string{
		"rocky", "rocky", "rocky", "rocky", "asteroid",
		"gas-giant", "ice-giant", "ice-giant", "ice-giant", "asteroid",
	}
	habitability := [...]int{0, 1, 8, 25, 0, 15, 4, 2, 0, 0}

	data := planetsSeed{
		Seed:      systems.Seed,
		Generator: generatorInfo{Stage: "planets", ID: genPlanets, Version: versionPlanets},
		Planets:   make([]planet, 0, len(systems.Systems)*len(types)),
	}
	seen := make(map[systemKey]bool, len(systems.Systems))
	for i, s := range systems.Systems {
		key := systemKey{stelliumKey{s.X, s.Y, s.Z}, s.Sequence}
		if err := checkCoordinates(s.X, s.Y, s.Z); err != nil {
			return fmt.Errorf("parse input file %s: systems[%d] %w", inputPath, i, err)
		}
		if _, err := mapkey.Sequence(s.Sequence); err != nil {
			return fmt.Errorf("parse input file %s: systems[%d] %w", inputPath, i, err)
		}
		if seen[key] {
			return fmt.Errorf("parse input file %s: systems[%d] repeats %s", inputPath, i, key)
		}
		seen[key] = true
		for index := range types {
			data.Planets = append(data.Planets, planet{
				X: s.X, Y: s.Y, Z: s.Z,
				Sequence:     s.Sequence,
				Orbit:        index + 1,
				Type:         types[index],
				Habitability: habitability[index],
			})
		}
	}
	return writeSeedFile(directory, planetsSeedFilename, data)
}

func generateDeposits(directory string) error {
	inputPath := filepath.Join(directory, planetsSeedFilename)
	var planets planetsSeed
	if err := readSeedFile(inputPath, &planets); err != nil {
		return err
	}
	seeds, err := planets.Seed.seeds()
	if err != nil {
		return fmt.Errorf("parse input file %s: %w", inputPath, err)
	}
	root := seeds.Derive(prng.TagDeposit, genDeposits, versionDeposits)

	data := depositsSeed{
		Seed:      planets.Seed,
		Generator: generatorInfo{Stage: "deposits", ID: genDeposits, Version: versionDeposits},
		Deposits:  make([]deposit, 0, len(planets.Planets)*20),
	}
	seen := make(map[planetKey]bool, len(planets.Planets))
	for i, p := range planets.Planets {
		key := planetKey{systemKey{stelliumKey{p.X, p.Y, p.Z}, p.Sequence}, p.Orbit}
		if err := checkCoordinates(p.X, p.Y, p.Z); err != nil {
			return fmt.Errorf("parse input file %s: planets[%d] %w", inputPath, i, err)
		}
		sequence, err := mapkey.Sequence(p.Sequence)
		if err != nil {
			return fmt.Errorf("parse input file %s: planets[%d] %w", inputPath, i, err)
		}
		if p.Orbit < mapkey.MinOrbit || p.Orbit > mapkey.MaxOrbit {
			return fmt.Errorf("parse input file %s: planets[%d] orbit %d is outside %d through %d", inputPath, i, p.Orbit, mapkey.MinOrbit, mapkey.MaxOrbit)
		}
		if seen[key] {
			return fmt.Errorf("parse input file %s: planets[%d] repeats %s", inputPath, i, key)
		}
		seen[key] = true

		count, sides := 15, 6
		switch p.Type {
		case "rocky":
			count, sides = 20, 3
		case "asteroid":
			count, sides = 35, 100
		case "gas-giant", "ice-giant":
		default:
			return fmt.Errorf("parse input file %s: planets[%d] has invalid type %q", inputPath, i, p.Type)
		}
		for number := 1; number <= count; number++ {
			// One roller per deposit, at the deposit's own address. The old
			// hand-rolled sha256 % sides was biased toward the low faces;
			// RollN rejects rather than folds.
			roll := root.Roller(mapkey.Deposit(p.X, p.Y, p.Z, sequence, p.Orbit, number)...).RollN(1, sides)
			data.Deposits = append(data.Deposits, deposit{
				X: p.X, Y: p.Y, Z: p.Z,
				Sequence:  p.Sequence,
				Orbit:     p.Orbit,
				DepositNo: number,
				Resource:  depositResource(p.Type, roll),
				Quantity:  10_000_000,
				Quality:   5,
			})
		}
	}
	return writeSeedFile(directory, depositsSeedFilename, data)
}

func checkCoordinates(x, y, z int) error {
	for _, n := range [3]int{x, y, z} {
		if n < mapkey.MinCoordinate || n > mapkey.MaxCoordinate {
			return fmt.Errorf("has coordinates (%d,%d,%d) outside %d through %d", x, y, z, mapkey.MinCoordinate, mapkey.MaxCoordinate)
		}
	}
	return nil
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

// readSeedFile decodes one stage's input and insists the file holds exactly one
// JSON value, so a concatenated or truncated file is a complaint rather than a
// half-read map.
func readSeedFile(path string, dst any) error {
	input, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open input file %s: %w", path, err)
	}
	defer input.Close()

	decoder := json.NewDecoder(input)
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("parse input file %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("unexpected additional JSON value")
		}
		return fmt.Errorf("parse input file %s: %w", path, err)
	}
	return nil
}

// writeSeedFile writes one stage's output, refusing to overwrite and removing a
// partial file if the encode fails. Every stage writes the same way, so it is
// written once.
func writeSeedFile(directory, filename string, data any) (err error) {
	path := filepath.Join(directory, filename)
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

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("write output file %s: %w", path, err)
	}
	complete = true
	return nil
}

// splitMix64 expands one memorable --stellia-seed into the pair prng roots at.
// Mixing quality is no longer its job -- SHA-256 does that inside prng -- but
// one integer on a command line is friendlier than two.
func splitMix64(state *uint64) uint64 {
	*state += 0x9e3779b97f4a7c15
	z := *state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}
