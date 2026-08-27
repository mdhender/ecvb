// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"slices"
	"strings"
	"testing"
)

// The turn is the phase table and nothing else, so the table has to hold
// together on its own: every phase in it exactly once, each one knowing its
// own place, and every order pointing at one of them.
func TestTheTurnIsItsPhaseTable(t *testing.T) {
	turn := Phases()
	if len(turn) == 0 {
		t.Fatal("a turn has no phases")
	}
	seen := make(map[string]bool, len(turn))
	for i, phase := range turn {
		if phase.Name == "" {
			t.Errorf("phase %d has no name", i)
		}
		if seen[phase.Name] {
			t.Errorf("phase %q appears twice", phase.Name)
		}
		seen[phase.Name] = true
		if phase.order != i {
			t.Errorf("phase %q holds place %d but sits at %d", phase.Name, phase.order, i)
		}
	}
	// Probes and passive sensors both read where things stood when the turn
	// began, so both settle before anything moves.
	if slices.Index(turn, PhaseSensor) > slices.Index(turn, PhaseMove) {
		t.Error("the sensor sweep happens after ships have moved")
	}
	// Units have to be unassembled to be carried and are set down as freight
	// when they arrive, so one file may unassemble here, transfer there, and
	// assemble again, all in one turn. That only works in this order.
	if slices.Index(turn, PhaseUnassemble) > slices.Index(turn, PhaseTransfer) {
		t.Error("transfers settle before anything is unassembled to carry")
	}
	if slices.Index(turn, PhaseTransfer) > slices.Index(turn, PhaseAssemble) {
		t.Error("assembly settles before the transfer that brings the units")
	}
}

func TestEveryOrderResolvesInAPhaseTheTurnHas(t *testing.T) {
	for _, spec := range Specs() {
		if spec.Phase == nil {
			t.Errorf("order %q resolves in no phase", spec.Verb)
			continue
		}
		if !slices.Contains(Phases(), spec.Phase) {
			t.Errorf("order %q resolves in phase %q, which is not part of a turn", spec.Verb, spec.Phase.Name)
		}
	}
}

// The help is where the phase table reaches a player, so a phase nobody can
// read about may as well not be in the table.
func TestHelpNamesEveryPhaseAndPlacesEveryOrder(t *testing.T) {
	help := Help()
	for _, phase := range Phases() {
		if !strings.Contains(help, phase.Name) {
			t.Errorf("help does not name the %s phase", phase.Name)
		}
	}
	for _, spec := range Specs() {
		if want := spec.Phase.Name + " phase"; !strings.Contains(help, want) {
			t.Errorf("help does not say that %s resolves in the %s", spec.Verb, want)
		}
	}
}
