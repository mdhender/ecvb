// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// testSeed is the seed block every hand-built fixture carries. A stage refuses
// an input without one, because the block is how it rebuilds the prng root.
func testSeed() pcgSeed { return newPCGSeed(19, 12) }

func TestRunGeneratesDeterministicStellia(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	if err := run(context.Background(), []string{"stellia", "--stellia-seed", "1912", first}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := run(context.Background(), []string{"stellia", "--stellia-seed", "1912", second}); err != nil {
		t.Fatalf("second run: %v", err)
	}

	firstData, err := os.ReadFile(filepath.Join(first, stelliaSeedFilename))
	if err != nil {
		t.Fatal(err)
	}
	secondData, err := os.ReadFile(filepath.Join(second, stelliaSeedFilename))
	if err != nil {
		t.Fatal(err)
	}
	if string(firstData) != string(secondData) {
		t.Fatal("same seed generated different files")
	}

	var data stelliaSeed
	if err := json.Unmarshal(firstData, &data); err != nil {
		t.Fatalf("parse generated file: %v", err)
	}
	if len(data.Stellia) != 100 {
		t.Fatalf("stellia count = %d; want 100", len(data.Stellia))
	}
	for _, half := range []string{data.Seed.High, data.Seed.Lo} {
		if _, err := strconv.ParseUint(half, 10, 64); err != nil {
			t.Fatalf("seed half %q is not an unsigned decimal: %v", half, err)
		}
	}
	if data.Generator.Stage != "stellia" || data.Generator.ID != genStellia || data.Generator.Version != versionStellia {
		t.Errorf("generator = %#v; want stage stellia, id %d, version %d", data.Generator, genStellia, versionStellia)
	}
	seen := make(map[[3]int]bool, len(data.Stellia))
	systemCounts := make(map[int]int)
	for i, s := range data.Stellia {
		if s.X < -15 || s.X > 15 || s.Y < -15 || s.Y > 15 || s.Z < -15 || s.Z > 15 {
			t.Fatalf("stellia[%d] has invalid coordinates: %#v", i, s)
		}
		if s.X == 0 && s.Y == 0 && s.Z == 0 {
			t.Fatalf("stellia[%d] is the excluded origin", i)
		}
		location := [3]int{s.X, s.Y, s.Z}
		if seen[location] {
			t.Fatalf("duplicate location at stellia[%d]: %#v", i, s)
		}
		seen[location] = true
		systemCounts[s.SystemCount]++
		if i > 0 && compareStellia(data.Stellia[i-1], s) > 0 {
			t.Fatalf("stellia are not sorted at indexes %d and %d", i-1, i)
		}
		for j := 0; j < i; j++ {
			other := data.Stellia[j]
			dx, dy, dz := s.X-other.X, s.Y-other.Y, s.Z-other.Z
			if distanceSquared := dx*dx + dy*dy + dz*dz; distanceSquared < 4 {
				t.Fatalf("stellia[%d] and stellia[%d] have squared distance %d; want at least 4", j, i, distanceSquared)
			}
		}
	}
	wantSystemCounts := map[int]int{1: 90, 2: 7, 3: 2, 4: 1}
	for count, want := range wantSystemCounts {
		if got := systemCounts[count]; got != want {
			t.Errorf("stellia with system-count %d = %d; want %d", count, got, want)
		}
	}
}

func TestRunUsesDefaultAndEnvironmentSeed(t *testing.T) {
	defaultDirectory, explicitDirectory := t.TempDir(), t.TempDir()
	if err := run(context.Background(), []string{"stellia", defaultDirectory}); err != nil {
		t.Fatalf("run with default seed: %v", err)
	}
	if err := run(context.Background(), []string{"stellia", "--stellia-seed", "1912", explicitDirectory}); err != nil {
		t.Fatalf("run with explicit default seed: %v", err)
	}
	assertFilesEqual(t, defaultDirectory, explicitDirectory)

	t.Setenv("EC_STELLIA_SEED", "8675309")
	environmentDirectory, matchingDirectory := t.TempDir(), t.TempDir()
	if err := run(context.Background(), []string{"stellia", environmentDirectory}); err != nil {
		t.Fatalf("run with EC_STELLIA_SEED: %v", err)
	}
	if err := run(context.Background(), []string{"stellia", "--stellia-seed", "8675309", matchingDirectory}); err != nil {
		t.Fatalf("run with matching explicit seed: %v", err)
	}
	assertFilesEqual(t, environmentDirectory, matchingDirectory)
}

// A different seed must produce a different map. Without this, a bug that
// dropped the seed on the floor would leave every other test here green.
func TestADifferentSeedIsADifferentMap(t *testing.T) {
	one, two := t.TempDir(), t.TempDir()
	if err := run(context.Background(), []string{"stellia", "--stellia-seed", "1912", one}); err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), []string{"stellia", "--stellia-seed", "2026", two}); err != nil {
		t.Fatal(err)
	}
	a, b := readStellia(t, one), readStellia(t, two)
	if a.Seed == b.Seed {
		t.Fatal("two seeds produced one seed block")
	}
	same := 0
	for i := range a.Stellia {
		if a.Stellia[i] == b.Stellia[i] {
			same++
		}
	}
	if same == len(a.Stellia) {
		t.Fatal("two seeds produced the same 100 stellia")
	}
}

func TestRunGeneratesSystemsFromStellia(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	input := stelliaSeed{Seed: testSeed(), Stellia: []stellium{
		{X: 1, Y: 2, Z: 3, SystemCount: 1},
		{X: -4, Y: 0, Z: 5, SystemCount: 3},
	}}
	for _, directory := range []string{first, second} {
		writeJSON(t, filepath.Join(directory, stelliaSeedFilename), input)
		if err := run(context.Background(), []string{"systems", directory}); err != nil {
			t.Fatalf("run systems: %v", err)
		}
	}

	firstData, err := os.ReadFile(filepath.Join(first, systemsSeedFilename))
	if err != nil {
		t.Fatal(err)
	}
	secondData, err := os.ReadFile(filepath.Join(second, systemsSeedFilename))
	if err != nil {
		t.Fatal(err)
	}
	if string(firstData) != string(secondData) {
		t.Fatal("same stellia generated different systems files")
	}

	var data systemsSeed
	if err := json.Unmarshal(firstData, &data); err != nil {
		t.Fatalf("parse generated file: %v", err)
	}
	want := []system{
		{X: 1, Y: 2, Z: 3, Sequence: "A"},
		{X: -4, Y: 0, Z: 5, Sequence: "A"},
		{X: -4, Y: 0, Z: 5, Sequence: "B"},
		{X: -4, Y: 0, Z: 5, Sequence: "C"},
	}
	if got := len(data.Systems); got != len(want) {
		t.Fatalf("system count = %d; want %d", got, len(want))
	}
	for i, w := range want {
		if data.Systems[i] != w {
			t.Errorf("systems[%d] = %#v; want %#v", i, data.Systems[i], w)
		}
	}
	if data.Seed != input.Seed {
		t.Errorf("seed block = %#v; want it carried forward as %#v", data.Seed, input.Seed)
	}
	if data.Generator.Stage != "systems" {
		t.Errorf("generator stage = %q; want systems", data.Generator.Stage)
	}
}

func TestGenerateSystemsRejectsInvalidInputAndExistingOutput(t *testing.T) {
	missingDirectory := t.TempDir()
	if err := generateSystems(missingDirectory); err == nil || !strings.Contains(err.Error(), "open input file") {
		t.Fatalf("missing input error = %v", err)
	}

	for _, test := range []struct {
		name  string
		input stelliaSeed
		want  string
	}{
		{"system count", stelliaSeed{Seed: testSeed(), Stellia: []stellium{{X: 1, SystemCount: 6}}}, "outside 1 through 5"},
		{"duplicate", stelliaSeed{Seed: testSeed(), Stellia: []stellium{
			{X: 1, Y: 2, Z: 3, SystemCount: 1}, {X: 1, Y: 2, Z: 3, SystemCount: 1},
		}}, "repeats the stellium at (1,2,3)"},
		{"out of range", stelliaSeed{Seed: testSeed(), Stellia: []stellium{{X: 16, SystemCount: 1}}}, "outside -15 through 15"},
		{"no seed block", stelliaSeed{Stellia: []stellium{{X: 1, SystemCount: 1}}}, "has no seed block"},
	} {
		directory := t.TempDir()
		writeJSON(t, filepath.Join(directory, stelliaSeedFilename), test.input)
		if err := generateSystems(directory); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("%s error = %v; want it to mention %q", test.name, err, test.want)
		}
		if _, err := os.Stat(filepath.Join(directory, systemsSeedFilename)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s created output: %v", test.name, err)
		}
	}

	existingDirectory := t.TempDir()
	writeJSON(t, filepath.Join(existingDirectory, stelliaSeedFilename), stelliaSeed{
		Seed: testSeed(), Stellia: []stellium{{X: 1, Y: 2, Z: 3, SystemCount: 1}},
	})
	assertOutputIsKept(t, existingDirectory, systemsSeedFilename, generateSystems)
}

func TestRunGeneratesPlanetsFromSystems(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	input := systemsSeed{Seed: testSeed(), Systems: []system{
		{X: 1, Y: 2, Z: 3, Sequence: "A"},
		{X: -4, Y: 0, Z: 5, Sequence: "B"},
	}}
	for _, directory := range []string{first, second} {
		writeJSON(t, filepath.Join(directory, systemsSeedFilename), input)
		if err := run(context.Background(), []string{"planets", directory}); err != nil {
			t.Fatalf("run planets: %v", err)
		}
	}

	firstData, err := os.ReadFile(filepath.Join(first, planetsSeedFilename))
	if err != nil {
		t.Fatal(err)
	}
	secondData, err := os.ReadFile(filepath.Join(second, planetsSeedFilename))
	if err != nil {
		t.Fatal(err)
	}
	if string(firstData) != string(secondData) {
		t.Fatal("same systems generated different planets files")
	}

	var data planetsSeed
	if err := json.Unmarshal(firstData, &data); err != nil {
		t.Fatalf("parse generated file: %v", err)
	}
	if got, want := len(data.Planets), 20; got != want {
		t.Fatalf("planet count = %d; want %d", got, want)
	}
	wantTypes := []string{"rocky", "rocky", "rocky", "rocky", "asteroid", "gas-giant", "ice-giant", "ice-giant", "ice-giant", "asteroid"}
	wantHabitability := []int{0, 1, 8, 25, 0, 15, 4, 2, 0, 0}
	for i, planet := range data.Planets {
		systemIndex, orbitIndex := i/10, i%10
		parent := input.Systems[systemIndex]
		if planet.X != parent.X || planet.Y != parent.Y || planet.Z != parent.Z || planet.Sequence != parent.Sequence {
			t.Errorf("planets[%d] = %#v; want it under %#v", i, planet, parent)
		}
		if planet.Orbit != orbitIndex+1 {
			t.Errorf("planets[%d] orbit = %d; want %d", i, planet.Orbit, orbitIndex+1)
		}
		if planet.Type != wantTypes[orbitIndex] || planet.Habitability != wantHabitability[orbitIndex] {
			t.Errorf("planets[%d] = type %q, habitability %d; want %q, %d", i, planet.Type, planet.Habitability, wantTypes[orbitIndex], wantHabitability[orbitIndex])
		}
	}
	if data.Seed != input.Seed {
		t.Errorf("seed block = %#v; want it carried forward", data.Seed)
	}
}

func TestGeneratePlanetsRejectsInvalidInputAndExistingOutput(t *testing.T) {
	missingDirectory := t.TempDir()
	if err := generatePlanets(missingDirectory); err == nil || !strings.Contains(err.Error(), "open input file") {
		t.Fatalf("missing input error = %v", err)
	}

	for _, test := range []struct {
		name  string
		input systemsSeed
		want  string
	}{
		{"duplicate", systemsSeed{Seed: testSeed(), Systems: []system{
			{X: 1, Y: 2, Z: 3, Sequence: "A"}, {X: 1, Y: 2, Z: 3, Sequence: "A"},
		}}, "repeats system A at (1,2,3)"},
		{"bad sequence", systemsSeed{Seed: testSeed(), Systems: []system{{X: 1, Sequence: "F"}}}, "not a letter A through E"},
		{"no seed block", systemsSeed{Systems: []system{{X: 1, Sequence: "A"}}}, "has no seed block"},
	} {
		directory := t.TempDir()
		writeJSON(t, filepath.Join(directory, systemsSeedFilename), test.input)
		if err := generatePlanets(directory); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("%s error = %v; want it to mention %q", test.name, err, test.want)
		}
		if _, err := os.Stat(filepath.Join(directory, planetsSeedFilename)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s created output: %v", test.name, err)
		}
	}

	existingDirectory := t.TempDir()
	writeJSON(t, filepath.Join(existingDirectory, systemsSeedFilename), systemsSeed{
		Seed: testSeed(), Systems: []system{{X: 1, Y: 2, Z: 3, Sequence: "A"}},
	})
	assertOutputIsKept(t, existingDirectory, planetsSeedFilename, generatePlanets)
}

func TestRunGeneratesDepositsFromPlanets(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	input := planetsSeed{Seed: testSeed(), Planets: []planet{
		{X: 1, Y: 2, Z: 3, Sequence: "A", Orbit: 1, Type: "rocky"},
		{X: 1, Y: 2, Z: 3, Sequence: "A", Orbit: 2, Type: "asteroid"},
		{X: -4, Y: 0, Z: 5, Sequence: "B", Orbit: 6, Type: "gas-giant"},
		{X: -4, Y: 0, Z: 5, Sequence: "B", Orbit: 7, Type: "ice-giant"},
	}}
	for _, directory := range []string{first, second} {
		writeJSON(t, filepath.Join(directory, planetsSeedFilename), input)
		if err := run(context.Background(), []string{"deposits", directory}); err != nil {
			t.Fatalf("run deposits: %v", err)
		}
	}

	firstData, err := os.ReadFile(filepath.Join(first, depositsSeedFilename))
	if err != nil {
		t.Fatal(err)
	}
	secondData, err := os.ReadFile(filepath.Join(second, depositsSeedFilename))
	if err != nil {
		t.Fatal(err)
	}
	if string(firstData) != string(secondData) {
		t.Fatal("same planets generated different deposits files")
	}

	var data depositsSeed
	if err := json.Unmarshal(firstData, &data); err != nil {
		t.Fatalf("parse generated file: %v", err)
	}
	wantCounts := []int{20, 35, 15, 15}
	if got, want := len(data.Deposits), 85; got != want {
		t.Fatalf("deposit count = %d; want %d", got, want)
	}
	index := 0
	for planetIndex, count := range wantCounts {
		parent := input.Planets[planetIndex]
		for number := 1; number <= count; number++ {
			deposit := data.Deposits[index]
			if deposit.X != parent.X || deposit.Y != parent.Y || deposit.Z != parent.Z ||
				deposit.Sequence != parent.Sequence || deposit.Orbit != parent.Orbit || deposit.DepositNo != number {
				t.Errorf("deposits[%d] = %#v; want deposit %d of %#v", index, deposit, number, parent)
			}
			if deposit.Quantity != 10_000_000 || deposit.Quality != 5 {
				t.Errorf("deposits[%d] = quantity %d, quality %d; want 10000000, 5", index, deposit.Quantity, deposit.Quality)
			}
			switch deposit.Resource {
			case "fuel", "gold", "metals", "minerals":
			default:
				t.Errorf("deposits[%d] has invalid resource %q", index, deposit.Resource)
			}
			if parent.Type == "asteroid" && deposit.Resource == "fuel" {
				t.Errorf("deposits[%d] gives asteroid fuel", index)
			}
			if parent.Type != "asteroid" && deposit.Resource == "gold" {
				t.Errorf("deposits[%d] gives %s gold", index, parent.Type)
			}
			index++
		}
	}
}

// The point of drawing from internal/prng: a deposit's resource is a function
// of its address and nothing else, so the order the planets happen to be listed
// in cannot change what is under any of them.
func TestDepositsAreAddressedNotSequenced(t *testing.T) {
	planets := []planet{
		{X: 1, Y: 2, Z: 3, Sequence: "A", Orbit: 1, Type: "rocky"},
		{X: 1, Y: 2, Z: 3, Sequence: "A", Orbit: 2, Type: "asteroid"},
		{X: 0, Y: 0, Z: 5, Sequence: "C", Orbit: 6, Type: "gas-giant"},
		{X: -4, Y: 0, Z: 5, Sequence: "B", Orbit: 7, Type: "ice-giant"},
	}
	reversed := make([]planet, len(planets))
	for i, p := range planets {
		reversed[len(planets)-1-i] = p
	}

	byAddress := func(order []planet) map[deposit]bool {
		t.Helper()
		directory := t.TempDir()
		writeJSON(t, filepath.Join(directory, planetsSeedFilename), planetsSeed{Seed: testSeed(), Planets: order})
		if err := generateDeposits(directory); err != nil {
			t.Fatalf("generate deposits: %v", err)
		}
		var data depositsSeed
		raw, err := os.ReadFile(filepath.Join(directory, depositsSeedFilename))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &data); err != nil {
			t.Fatal(err)
		}
		out := make(map[deposit]bool, len(data.Deposits))
		for _, d := range data.Deposits {
			out[d] = true
		}
		return out
	}

	forward, backward := byAddress(planets), byAddress(reversed)
	if len(forward) != len(backward) {
		t.Fatalf("deposit counts differ: %d and %d", len(forward), len(backward))
	}
	for d := range forward {
		if !backward[d] {
			t.Errorf("reversing the planets changed %#v", d)
		}
	}
}

// The old roll was sha256 % sides, which leans on the low faces. Nothing
// noticed, because nothing looked.
func TestDepositRollIsUnbiased(t *testing.T) {
	directory := t.TempDir()
	planets := make([]planet, 0, 500)
	for orbit := 1; orbit <= 10; orbit++ {
		for x := 1; x <= 50; x++ {
			planets = append(planets, planet{X: x % 16, Y: x / 16, Z: orbit, Sequence: "A", Orbit: orbit, Type: "rocky"})
		}
	}
	writeJSON(t, filepath.Join(directory, planetsSeedFilename), planetsSeed{Seed: testSeed(), Planets: planets})
	if err := generateDeposits(directory); err != nil {
		t.Fatalf("generate deposits: %v", err)
	}
	var data depositsSeed
	raw, err := os.ReadFile(filepath.Join(directory, depositsSeedFilename))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}

	// A rocky planet rolls d3: 1 is fuel, 2 is metals, 3 is minerals. Each face
	// should be near a third. The band is wide on purpose -- this catches a
	// systematic lean, not a fair run of luck.
	counts := map[string]int{}
	for _, d := range data.Deposits {
		counts[d.Resource]++
	}
	total := len(data.Deposits)
	if total == 0 {
		t.Fatal("no deposits")
	}
	for _, resource := range []string{"fuel", "metals", "minerals"} {
		share := float64(counts[resource]) / float64(total)
		if share < 0.28 || share > 0.39 {
			t.Errorf("%s is %.3f of %d deposits; want near one third (counts %v)", resource, share, total, counts)
		}
	}
}

func TestDepositResourceRolls(t *testing.T) {
	tests := []struct {
		planetType string
		roll       int
		want       string
	}{
		{"asteroid", 1, "gold"}, {"asteroid", 4, "gold"},
		{"asteroid", 5, "minerals"}, {"asteroid", 6, "metals"},
		{"asteroid", 52, "metals"}, {"asteroid", 53, "minerals"},
		{"rocky", 1, "fuel"}, {"rocky", 2, "metals"}, {"rocky", 3, "minerals"},
		{"gas-giant", 1, "fuel"}, {"gas-giant", 3, "fuel"},
		{"gas-giant", 4, "metals"}, {"gas-giant", 5, "minerals"},
		{"ice-giant", 1, "fuel"}, {"ice-giant", 2, "fuel"},
		{"ice-giant", 3, "metals"}, {"ice-giant", 4, "minerals"},
	}
	for _, test := range tests {
		if got := depositResource(test.planetType, test.roll); got != test.want {
			t.Errorf("depositResource(%q, %d) = %q; want %q", test.planetType, test.roll, got, test.want)
		}
	}
}

func TestGenerateDepositsRejectsInvalidInputAndExistingOutput(t *testing.T) {
	missingDirectory := t.TempDir()
	if err := generateDeposits(missingDirectory); err == nil || !strings.Contains(err.Error(), "open input file") {
		t.Fatalf("missing input error = %v", err)
	}

	for _, test := range []struct {
		name  string
		input planetsSeed
		want  string
	}{
		{"planet type", planetsSeed{Seed: testSeed(), Planets: []planet{
			{X: 1, Sequence: "A", Orbit: 1, Type: "ocean"},
		}}, "invalid type"},
		{"duplicate", planetsSeed{Seed: testSeed(), Planets: []planet{
			{X: 1, Y: 2, Z: 3, Sequence: "A", Orbit: 4, Type: "rocky"},
			{X: 1, Y: 2, Z: 3, Sequence: "A", Orbit: 4, Type: "rocky"},
		}}, "repeats orbit 4 of system A at (1,2,3)"},
		{"orbit", planetsSeed{Seed: testSeed(), Planets: []planet{
			{X: 1, Sequence: "A", Orbit: 11, Type: "rocky"},
		}}, "orbit 11 is outside 1 through 10"},
		{"no seed block", planetsSeed{Planets: []planet{
			{X: 1, Sequence: "A", Orbit: 1, Type: "rocky"},
		}}, "has no seed block"},
	} {
		directory := t.TempDir()
		writeJSON(t, filepath.Join(directory, planetsSeedFilename), test.input)
		if err := generateDeposits(directory); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("%s error = %v; want it to mention %q", test.name, err, test.want)
		}
		if _, err := os.Stat(filepath.Join(directory, depositsSeedFilename)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s created output: %v", test.name, err)
		}
	}

	existingDirectory := t.TempDir()
	writeJSON(t, filepath.Join(existingDirectory, planetsSeedFilename), planetsSeed{
		Seed: testSeed(), Planets: []planet{{X: 1, Y: 2, Z: 3, Sequence: "A", Orbit: 1, Type: "rocky"}},
	})
	assertOutputIsKept(t, existingDirectory, depositsSeedFilename, generateDeposits)
}

// The whole chain, end to end: the seed block written by `stellia` is the block
// `deposits` finally draws from, unchanged by the two stages between.
func TestSeedBlockCarriesForward(t *testing.T) {
	directory := t.TempDir()
	// Only stellia takes the seed; the rest must find it in the file.
	if err := run(context.Background(), []string{"stellia", "--stellia-seed", "2026", directory}); err != nil {
		t.Fatalf("run stellia: %v", err)
	}
	for _, stage := range []string{"systems", "planets", "deposits"} {
		if err := run(context.Background(), []string{stage, directory}); err != nil {
			t.Fatalf("run %s: %v", stage, err)
		}
	}
	want := readStellia(t, directory).Seed
	if want.High == "" || want.Lo == "" {
		t.Fatal("stellia wrote no seed block")
	}
	for _, stage := range []struct {
		filename string
		seed     func([]byte) pcgSeed
	}{
		{systemsSeedFilename, func(b []byte) pcgSeed { var v systemsSeed; mustJSON(t, b, &v); return v.Seed }},
		{planetsSeedFilename, func(b []byte) pcgSeed { var v planetsSeed; mustJSON(t, b, &v); return v.Seed }},
		{depositsSeedFilename, func(b []byte) pcgSeed { var v depositsSeed; mustJSON(t, b, &v); return v.Seed }},
	} {
		raw, err := os.ReadFile(filepath.Join(directory, stage.filename))
		if err != nil {
			t.Fatal(err)
		}
		if got := stage.seed(raw); got != want {
			t.Errorf("%s seed = %#v; want %#v", stage.filename, got, want)
		}
	}
}

func TestGenerateStelliaRejectsInvalidOutput(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if err := generateStellia(missing, 1912); err == nil || !strings.Contains(err.Error(), "stat output directory") {
		t.Fatalf("missing directory error = %v", err)
	}

	notDirectory := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notDirectory, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := generateStellia(notDirectory, 1912); err == nil || !strings.Contains(err.Error(), "is not a directory") {
		t.Fatalf("file path error = %v", err)
	}

	directory := t.TempDir()
	path := filepath.Join(directory, stelliaSeedFilename)
	if err := os.WriteFile(path, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := generateStellia(directory, 1912); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing output error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "keep me" {
		t.Fatalf("existing output was changed to %q", data)
	}
}

func TestSplitMix64(t *testing.T) {
	var state uint64
	if got, want := splitMix64(&state), uint64(0xe220a8397b1dcdaf); got != want {
		t.Fatalf("first value = %#x; want %#x", got, want)
	}
	if got, want := splitMix64(&state), uint64(0x6e789e6aa1b965f4); got != want {
		t.Fatalf("second value = %#x; want %#x", got, want)
	}
}

// assertOutputIsKept covers the shared refusal: a stage never overwrites, and a
// refused run leaves what was there untouched.
func assertOutputIsKept(t *testing.T, directory, filename string, generate func(string) error) {
	t.Helper()
	outputPath := filepath.Join(directory, filename)
	if err := os.WriteFile(outputPath, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := generate(directory); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing output error = %v", err)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "keep me" {
		t.Fatalf("existing output was changed to %q", data)
	}
}

func assertFilesEqual(t *testing.T, first, second string) {
	t.Helper()
	a, err := os.ReadFile(filepath.Join(first, stelliaSeedFilename))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(second, stelliaSeedFilename))
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("generated files differ")
	}
}

func readStellia(t *testing.T, directory string) stelliaSeed {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(directory, stelliaSeedFilename))
	if err != nil {
		t.Fatal(err)
	}
	var data stelliaSeed
	mustJSON(t, raw, &data)
	return data
}

func mustJSON(t *testing.T, raw []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("parse generated file: %v", err)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
