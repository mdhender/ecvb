// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"context"
	"encoding/json"
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
