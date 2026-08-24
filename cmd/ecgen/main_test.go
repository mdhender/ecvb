// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

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
	if data.Seed.High == "" || data.Seed.Lo == "" {
		t.Fatalf("PCG seed = %#v; want two strings", data.Seed)
	}
	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
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
		if !uuidPattern.MatchString(s.UUID) {
			t.Fatalf("stellia[%d] UUID = %q; want UUID v4", i, s.UUID)
		}
		systemCounts[s.SystemCount]++
		if i > 0 {
			previous := data.Stellia[i-1]
			if previous.X > s.X || previous.X == s.X && previous.Y > s.Y || previous.X == s.X && previous.Y == s.Y && previous.Z > s.Z {
				t.Fatalf("stellia are not sorted at indexes %d and %d", i-1, i)
			}
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

func TestRunGeneratesSystemsFromStellia(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	input := stelliaSeed{Stellia: []stellium{
		{UUID: "11111111-1111-4111-8111-111111111111", SystemCount: 1},
		{UUID: "22222222-2222-4222-8222-222222222222", SystemCount: 3},
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
	if got, want := len(data.Systems), 4; got != want {
		t.Fatalf("system count = %d; want %d", got, want)
	}
	wantParents := []string{input.Stellia[0].UUID, input.Stellia[1].UUID, input.Stellia[1].UUID, input.Stellia[1].UUID}
	wantSequences := []string{"A", "A", "B", "C"}
	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	seen := make(map[string]bool)
	for i, system := range data.Systems {
		if system.StelliumUUID != wantParents[i] || system.Sequence != wantSequences[i] {
			t.Errorf("systems[%d] = %#v; want parent %q and sequence %q", i, system, wantParents[i], wantSequences[i])
		}
		if !uuidPattern.MatchString(system.UUID) {
			t.Errorf("systems[%d] UUID = %q; want UUID v4", i, system.UUID)
		}
		if seen[system.UUID] {
			t.Errorf("systems[%d] has duplicate UUID %q", i, system.UUID)
		}
		seen[system.UUID] = true
	}
}

func TestGenerateSystemsRejectsInvalidInputAndExistingOutput(t *testing.T) {
	missingDirectory := t.TempDir()
	if err := generateSystems(missingDirectory); err == nil || !strings.Contains(err.Error(), "open input file") {
		t.Fatalf("missing input error = %v", err)
	}

	invalidDirectory := t.TempDir()
	writeJSON(t, filepath.Join(invalidDirectory, stelliaSeedFilename), stelliaSeed{
		Stellia: []stellium{{UUID: "11111111-1111-4111-8111-111111111111", SystemCount: 6}},
	})
	if err := generateSystems(invalidDirectory); err == nil || !strings.Contains(err.Error(), "outside 1 through 5") {
		t.Fatalf("invalid system-count error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(invalidDirectory, systemsSeedFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid input created output: %v", err)
	}

	existingDirectory := t.TempDir()
	writeJSON(t, filepath.Join(existingDirectory, stelliaSeedFilename), stelliaSeed{
		Stellia: []stellium{{UUID: "11111111-1111-4111-8111-111111111111", SystemCount: 1}},
	})
	outputPath := filepath.Join(existingDirectory, systemsSeedFilename)
	if err := os.WriteFile(outputPath, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := generateSystems(existingDirectory); err == nil || !strings.Contains(err.Error(), "already exists") {
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

func TestRunGeneratesPlanetsFromSystems(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	input := systemsSeed{Systems: []system{
		{UUID: "11111111-1111-4111-8111-111111111111"},
		{UUID: "22222222-2222-4222-8222-222222222222"},
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
	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	seen := make(map[string]bool)
	for i, planet := range data.Planets {
		systemIndex, orbitIndex := i/10, i%10
		if planet.SystemUUID != input.Systems[systemIndex].UUID || planet.Orbit != orbitIndex+1 {
			t.Errorf("planets[%d] = %#v; want system %q and orbit %d", i, planet, input.Systems[systemIndex].UUID, orbitIndex+1)
		}
		if planet.Type != wantTypes[orbitIndex] || planet.Habitability != wantHabitability[orbitIndex] {
			t.Errorf("planets[%d] = type %q, habitability %d; want %q, %d", i, planet.Type, planet.Habitability, wantTypes[orbitIndex], wantHabitability[orbitIndex])
		}
		if !uuidPattern.MatchString(planet.UUID) {
			t.Errorf("planets[%d] UUID = %q; want UUID v4", i, planet.UUID)
		}
		if seen[planet.UUID] {
			t.Errorf("planets[%d] has duplicate UUID %q", i, planet.UUID)
		}
		seen[planet.UUID] = true
	}
}

func TestGeneratePlanetsRejectsInvalidInputAndExistingOutput(t *testing.T) {
	missingDirectory := t.TempDir()
	if err := generatePlanets(missingDirectory); err == nil || !strings.Contains(err.Error(), "open input file") {
		t.Fatalf("missing input error = %v", err)
	}

	invalidDirectory := t.TempDir()
	writeJSON(t, filepath.Join(invalidDirectory, systemsSeedFilename), systemsSeed{
		Systems: []system{{UUID: "11111111-1111-4111-8111-111111111111"}, {UUID: "11111111-1111-4111-8111-111111111111"}},
	})
	if err := generatePlanets(invalidDirectory); err == nil || !strings.Contains(err.Error(), "duplicate uuid") {
		t.Fatalf("duplicate system UUID error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(invalidDirectory, planetsSeedFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid input created output: %v", err)
	}

	existingDirectory := t.TempDir()
	writeJSON(t, filepath.Join(existingDirectory, systemsSeedFilename), systemsSeed{
		Systems: []system{{UUID: "11111111-1111-4111-8111-111111111111"}},
	})
	outputPath := filepath.Join(existingDirectory, planetsSeedFilename)
	if err := os.WriteFile(outputPath, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := generatePlanets(existingDirectory); err == nil || !strings.Contains(err.Error(), "already exists") {
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

func TestRunGeneratesDepositsFromPlanets(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	input := planetsSeed{Planets: []planet{
		{UUID: "11111111-1111-4111-8111-111111111111", Type: "rocky"},
		{UUID: "22222222-2222-4222-8222-222222222222", Type: "asteroid"},
		{UUID: "33333333-3333-4333-8333-333333333333", Type: "gas-giant"},
		{UUID: "44444444-4444-4444-8444-444444444444", Type: "ice-giant"},
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
		for sequence := 1; sequence <= count; sequence++ {
			deposit := data.Deposits[index]
			if deposit.PlanetUUID != input.Planets[planetIndex].UUID || deposit.Sequence != sequence {
				t.Errorf("deposits[%d] = %#v; want planet %q and sequence %d", index, deposit, input.Planets[planetIndex].UUID, sequence)
			}
			if deposit.Quantity != 10_000_000 || deposit.Quality != 5 {
				t.Errorf("deposits[%d] = quantity %d, quality %d; want 10000000, 5", index, deposit.Quantity, deposit.Quality)
			}
			if deposit.Resource != "fuel" && deposit.Resource != "gold" && deposit.Resource != "metals" && deposit.Resource != "minerals" {
				t.Errorf("deposits[%d] has invalid resource %q", index, deposit.Resource)
			}
			if input.Planets[planetIndex].Type == "asteroid" && deposit.Resource == "fuel" {
				t.Errorf("deposits[%d] gives asteroid fuel", index)
			}
			if input.Planets[planetIndex].Type != "asteroid" && deposit.Resource == "gold" {
				t.Errorf("deposits[%d] gives %s gold", index, input.Planets[planetIndex].Type)
			}
			index++
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

	invalidDirectory := t.TempDir()
	writeJSON(t, filepath.Join(invalidDirectory, planetsSeedFilename), planetsSeed{
		Planets: []planet{{UUID: "11111111-1111-4111-8111-111111111111", Type: "ocean"}},
	})
	if err := generateDeposits(invalidDirectory); err == nil || !strings.Contains(err.Error(), "invalid type") {
		t.Fatalf("invalid planet type error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(invalidDirectory, depositsSeedFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid input created output: %v", err)
	}

	existingDirectory := t.TempDir()
	writeJSON(t, filepath.Join(existingDirectory, planetsSeedFilename), planetsSeed{
		Planets: []planet{{UUID: "11111111-1111-4111-8111-111111111111", Type: "rocky"}},
	})
	outputPath := filepath.Join(existingDirectory, depositsSeedFilename)
	if err := os.WriteFile(outputPath, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := generateDeposits(existingDirectory); err == nil || !strings.Contains(err.Error(), "already exists") {
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
