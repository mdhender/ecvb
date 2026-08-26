// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"fmt"
	"sort"
	"strings"
)

// Phase is when in a turn an order resolves. Every order of one phase resolves
// before any order of the next, whichever way round the player wrote them.
type Phase int

// The phases of a turn, in the order they resolve. Production and combat
// append to this list.
const (
	PhaseProbe Phase = iota
	PhaseMove
	PhaseJump
)

// Phases lists the phases of a turn in resolution order.
func Phases() []Phase { return []Phase{PhaseProbe, PhaseMove, PhaseJump} }

// PhaseOf is when an order resolves. An unregistered verb never reaches this:
// the parser refuses a line before it becomes an order.
func PhaseOf(verb string) Phase {
	if spec, ok := Lookup(verb); ok {
		return spec.Phase
	}
	return PhaseProbe
}

// Spec is everything the order pipeline knows about one verb. Registering a
// Spec is how an order joins the game: the parser dispatches on Verb, errors
// quote Syntax, `ec orders help` prints both, and Phase is when the order
// takes effect.
type Spec struct {
	// Verb is the keyword that opens the order, lowercase.
	Verb string
	// Summary is one line describing what the order does.
	Summary string
	// Syntax lists every legal form of the order.
	Syntax []string
	// Phase is when in a turn the order resolves.
	Phase Phase
	// Parse reads the rest of the line, after the verb.
	Parse func(line *Line) (Params, error)
}

var registry = map[string]*Spec{}

// register adds a Spec. It panics on a duplicate verb, because two orders
// answering to one keyword is a programming mistake, not a player's.
func register(spec *Spec) {
	if _, exists := registry[spec.Verb]; exists {
		panic("orders: duplicate verb " + spec.Verb)
	}
	registry[spec.Verb] = spec
}

// Specs returns every registered order, ordered by verb.
func Specs() []*Spec {
	specs := make([]*Spec, 0, len(registry))
	for _, spec := range registry {
		specs = append(specs, spec)
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Verb < specs[j].Verb })
	return specs
}

// Lookup returns the Spec for a verb, ignoring case.
func Lookup(verb string) (*Spec, bool) {
	spec, ok := registry[strings.ToLower(verb)]
	return spec, ok
}

// verbList names every order, for the error a mistyped verb gets.
func verbList() string {
	specs := Specs()
	names := make([]string, len(specs))
	for i, spec := range specs {
		names[i] = spec.Verb
	}
	switch len(names) {
	case 0:
		return "no orders"
	case 1:
		return names[0]
	case 2:
		return names[0] + " or " + names[1]
	default:
		return strings.Join(names[:len(names)-1], ", ") + ", or " + names[len(names)-1]
	}
}

// syntaxError reports that a line did not match any form of its verb. It lists
// only that verb's forms, which is why the verb is worth dispatching on before
// the line is understood.
func (s *Spec) syntaxError() error {
	if len(s.Syntax) == 1 {
		return fmt.Errorf("expected %s", s.Syntax[0])
	}
	return fmt.Errorf("expected %s", strings.Join(s.Syntax, ", or "))
}

// Help returns the reference `ec orders help` prints.
func Help() string {
	var out strings.Builder
	out.WriteString("ORDERS\n")
	for _, spec := range Specs() {
		fmt.Fprintf(&out, "\n%s\n  %s\n", strings.ToUpper(spec.Verb), spec.Summary)
		for _, form := range spec.Syntax {
			fmt.Fprintf(&out, "    %s\n", form)
		}
	}
	return out.String()
}

// HelpFor returns the reference for one order.
func HelpFor(verb string) (string, error) {
	spec, ok := Lookup(verb)
	if !ok {
		return "", fmt.Errorf("unknown order %q; expected %s", verb, verbList())
	}
	var out strings.Builder
	fmt.Fprintf(&out, "%s\n  %s\n", strings.ToUpper(spec.Verb), spec.Summary)
	for _, form := range spec.Syntax {
		fmt.Fprintf(&out, "    %s\n", form)
	}
	return out.String(), nil
}
