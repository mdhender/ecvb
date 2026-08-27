// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/mdhender/ecvb/internal/world"
)

// Phase is a stage of a turn. Every order of one phase resolves before any
// order of the next, whichever way round the player wrote them; file order
// decides only between orders of the same phase.
//
// Most phases are their orders and nothing else. A phase may also carry a
// Sweep, which is what the phase does apart from anyone's orders: the sensor
// phase has no orders at all and is only its sweep, and combat, when it
// arrives, will be mostly sweep, because a battle is settled between the
// fleets that met rather than one order at a time.
type Phase struct {
	// Name identifies the phase to a player.
	Name string
	// Sweep runs after the phase's orders, or is the whole phase when it has
	// none. nil for a phase that is only its orders.
	Sweep func(*world.World, int) error
	// order is the phase's place in the turn, filled in from the table below.
	order int
}

// The phases of a turn. Production and combat will be new entries here and a
// Phase on their orders' Specs; nothing else has to learn about them.
var (
	PhaseProbe  = &Phase{Name: "probe"}
	PhaseSensor = &Phase{Name: "sensor", Sweep: (*world.World).RecordSensors}
	PhaseMove   = &Phase{Name: "move"}
	PhaseJump   = &Phase{Name: "jump"}
	// PhaseArrival has no orders. A crossing between stellia is not an order --
	// the jump that began it departed and succeeded turns ago -- so landing the
	// ships that are due is the whole of the phase.
	PhaseArrival = &Phase{Name: "arrival", Sweep: (*world.World).LandArrivals}
	PhaseNaming  = &Phase{Name: "naming"}
)

// phases is the turn, in the order it happens. Probes and passive sensors both
// read where things stood when the turn began, so both come before anything
// moves; a ship that jumps this turn reports its new stellium next turn.
//
// Departures come before arrivals, so this turn's jumps are settled before this
// turn's landings and a ship cannot be caught by a jump order written the turn
// it arrives.
var phases = []*Phase{PhaseProbe, PhaseSensor, PhaseMove, PhaseJump, PhaseArrival, PhaseNaming}

func init() {
	for i, phase := range phases {
		phase.order = i
	}
}

// Phases lists the phases of a turn in the order they resolve.
func Phases() []*Phase { return phases }

// PhaseOf is when an order resolves. An unregistered verb never reaches this:
// the parser refuses a line before it becomes an order.
func PhaseOf(verb string) *Phase {
	if spec, ok := Lookup(verb); ok {
		return spec.Phase
	}
	return PhaseProbe
}

// The subjects an order may be given to. Every order names its subject first,
// so the parser knows who is acting before it knows what they are doing.
const (
	// SubjectShip is a ship of the submitting faction.
	SubjectShip = "ship"
	// SubjectColony is a colony of the submitting faction.
	SubjectColony = "colony"
	// SubjectFaction is the faction itself, written `we`. It is the subject of
	// the orders no ship or colony carries out, such as naming a place, and it
	// takes no id because the file header already names the faction.
	SubjectFaction = "we"
)

// Subject is what an order was given to, read off the front of the line.
type Subject struct {
	// Kind is the word the player wrote: ship, colony, or we.
	Kind string
	// ID is the entity's id, and 0 for we, which names no entity.
	ID int64
}

// Spec is everything the order pipeline knows about one verb. Registering a
// Spec is how an order joins the game: the parser reads the subject, then
// dispatches on Verb, errors quote Syntax, `ec orders help` prints both, and
// Phase is when the order takes effect.
type Spec struct {
	// Verb is the keyword that names the order, lowercase. It follows the
	// subject rather than opening the line.
	Verb string
	// Subjects are the subjects that may be given this order. A line whose
	// subject is not one of them is refused before Parse ever sees it, so no
	// order has to check who it was given to.
	Subjects []string
	// Summary is one line describing what the order does.
	Summary string
	// Syntax lists every legal form of the order.
	Syntax []string
	// Phase is the stage of a turn the order resolves in.
	Phase *Phase
	// Movement says whether the order can move the entity it acts on, and so
	// whether where that entity began and ended is worth recording. It is a
	// property of the kind of order, not of how one went: an order that can
	// move records that it stayed put when it failed.
	Movement bool
	// Parse reads the rest of the line, after the subject and the verb. The
	// subject is handed in already read, so an order never parses its own
	// actor.
	Parse func(subject Subject, line *Line) (Params, error)
	// Decode is Parse's counterpart for an order read back out of the
	// database: it rebuilds the parameters from the stored JSON and the actor,
	// which is a column of its own rather than part of the JSON.
	Decode func(actor int64, params string) (Params, error)
}

var registry = map[string]*Spec{}

// register adds a Spec. It panics on a duplicate verb, because two orders
// answering to one keyword is a programming mistake, not a player's, and on a
// Spec that names no subject, because a line with nobody to give it to can
// never be written.
func register(spec *Spec) {
	if _, exists := registry[spec.Verb]; exists {
		panic("orders: duplicate verb " + spec.Verb)
	}
	if len(spec.Subjects) == 0 {
		panic("orders: " + spec.Verb + " names no subject")
	}
	registry[spec.Verb] = spec
}

// accepts reports whether the order may be given to this kind of subject.
func (s *Spec) accepts(kind string) bool {
	return slices.Contains(s.Subjects, kind)
}

// subjectError reports an order given to something that cannot be given it.
func (s *Spec) subjectError(kind string) error {
	return fmt.Errorf("%s is given to %s, not to %s",
		strings.ToUpper(s.Verb), wordList(subjectNouns(s.Subjects)), subjectNoun(kind))
}

// subjectNoun names a subject the way an error message reads it.
func subjectNoun(kind string) string {
	if kind == SubjectFaction {
		return "we"
	}
	return "a " + kind
}

func subjectNouns(kinds []string) []string {
	nouns := make([]string, len(kinds))
	for i, kind := range kinds {
		nouns[i] = subjectNoun(kind)
	}
	return nouns
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
	if len(names) == 0 {
		return "no orders"
	}
	return wordList(names)
}

// wordList joins words the way a sentence does, so an error reads as one.
func wordList(words []string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	case 2:
		return words[0] + " or " + words[1]
	default:
		return strings.Join(words[:len(words)-1], ", ") + ", or " + words[len(words)-1]
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

// Help returns the reference `ec orders help` prints. It opens with the turn,
// because when an order takes effect is as much a part of writing a file as
// what the order says.
func Help() string {
	var out strings.Builder
	out.WriteString("ORDERS\n\n")
	out.WriteString("Every order names its subject first -- a ship, a colony, or `we` for the\n")
	out.WriteString("faction itself -- and then what it is being told to do.\n\n")
	out.WriteString("A turn resolves in phases. Every order of one phase resolves before any\n")
	out.WriteString("order of the next; the order of the lines in a file decides only between\n")
	out.WriteString("orders of the same phase. The phases, in the order they happen:\n\n")
	for i, phase := range phases {
		fmt.Fprintf(&out, "  %d. %s\n", i+1, phase.Name)
	}
	for _, spec := range Specs() {
		out.WriteString("\n")
		out.WriteString(describe(spec))
	}
	return out.String()
}

// HelpFor returns the reference for one order.
func HelpFor(verb string) (string, error) {
	spec, ok := Lookup(verb)
	if !ok {
		return "", fmt.Errorf("unknown order %q; expected %s", verb, verbList())
	}
	return describe(spec), nil
}

func describe(spec *Spec) string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s (%s phase, given to %s)\n  %s\n",
		strings.ToUpper(spec.Verb), spec.Phase.Name, wordList(subjectNouns(spec.Subjects)), spec.Summary)
	for _, form := range spec.Syntax {
		fmt.Fprintf(&out, "    %s\n", form)
	}
	return out.String()
}
