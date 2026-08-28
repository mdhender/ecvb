// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package replay holds the end-to-end regression test.
//
// TestReplay plays a whole scripted game -- submit, resolve, report, open the
// next turn -- against a real database built from the migrations, and compares
// everything it produced against committed golden files. It is the gate the
// order-pipeline rework has to pass: a refactor that preserves behavior leaves
// every golden untouched, and one that does not says exactly which report and
// which line changed.
//
// Both outputs are chosen so they hold still. Reports are compared as JSON
// rather than as aligned text, which shifts whenever a column widens, and the
// engine log is written without a wall clock, so the same turn logs the same
// bytes. Nothing else in a resolution is random: rings are drawn from the game
// seed and the identity of the order.
//
// Run `go test ./internal/replay -update` to rewrite the goldens after a
// deliberate rules change, and read the diff before committing it.
package replay

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mdhender/ecvb/internal/engine"
	"github.com/mdhender/ecvb/internal/logging"
	"github.com/mdhender/ecvb/internal/orders"
	"github.com/mdhender/ecvb/internal/report"
	"github.com/mdhender/ecvb/internal/testdb"
	"zombiezen.com/go/sqlite"
)

var update = flag.Bool("update", false, "rewrite the golden files instead of comparing against them")

const (
	gameCode = "GOLD-01"
	// lastTurn is one turn past the last turn that gives an order, so the
	// scripted game runs long enough for its crossings to land. Both of them
	// finish in a turn nobody writes an order for: a ship still crossing can be
	// given none, and the arrival step is a sweep. A turn with no order file is
	// skipped by submitOrders and resolved like any other.
	lastTurn  = 3
	goldenDir = "testdata/golden"
)

// factions covers both the player faction that gives orders and the agent
// faction that only ever gets looked at, because a report of a faction with no
// orders is its own thing to get wrong.
var factions = []int64{1, 2}

func TestReplay(t *testing.T) {
	conn := testdb.New(t)
	scenario, err := os.ReadFile("testdata/scenario.sql")
	if err != nil {
		t.Fatalf("read scenario: %v", err)
	}
	testdb.Exec(t, conn, string(scenario))

	ctx := context.Background()
	golden := newGoldenSet(t)
	for turn := 0; turn <= lastTurn; turn++ {
		submitOrders(t, ctx, conn, turn)
		golden.check(t, engineLogName(turn), resolveTurn(t, ctx, conn, turn))
		for _, faction := range factions {
			golden.check(t, reportName(turn, faction, "orders"), ordersJSON(t, conn, faction, turn))
			golden.check(t, reportName(turn, faction, "turn"), turnJSON(t, conn, faction))
		}
		if turn != lastTurn {
			if _, err := engine.OpenNextTurn(ctx, conn, gameCode, turn); err != nil {
				t.Fatalf("open the turn after %d: %v", turn, err)
			}
		}
	}
	golden.finish(t)
}

// submitOrders submits the turn's order file if the scenario has one. A turn
// with no file is a faction that ordered nothing, which must still resolve.
func submitOrders(t *testing.T, ctx context.Context, conn *sqlite.Conn, turn int) {
	t.Helper()
	path := filepath.Join("testdata/orders", orderFileName(turn))
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer file.Close()
	result, err := orders.Submit(ctx, conn, file)
	if err != nil {
		t.Fatalf("submit %s: %v", path, err)
	}
	// Warnings are part of the behavior under test: an order the ship cannot
	// pay for is accepted with a warning rather than rejected.
	for _, warning := range result.Warnings {
		t.Logf("turn %d warning: line %d: %s", turn, warning.Line, warning.Message)
	}
}

func resolveTurn(t *testing.T, ctx context.Context, conn *sqlite.Conn, turn int) []byte {
	t.Helper()
	var log bytes.Buffer
	if _, err := engine.Resolve(ctx, logging.NewWithoutTime(&log), conn, gameCode, turn); err != nil {
		t.Fatalf("resolve turn %d: %v", turn, err)
	}
	return log.Bytes()
}

func ordersJSON(t *testing.T, conn *sqlite.Conn, factionID int64, turn int) []byte {
	t.Helper()
	rpt, err := report.Orders(conn, gameCode, "", factionID, turn)
	if err != nil {
		t.Fatalf("orders report for faction %d turn %d: %v", factionID, turn, err)
	}
	return renderJSON(t, rpt)
}

func turnJSON(t *testing.T, conn *sqlite.Conn, factionID int64) []byte {
	t.Helper()
	// Every optional section is on, so the golden covers all of them.
	rpt, err := report.Turn(conn, gameCode, "", factionID, report.TurnOptions{
		ShowDeposits:       true,
		SummarizeResources: true,
		ShowWorkGroups:     true,
	})
	if err != nil {
		t.Fatalf("turn report for faction %d: %v", factionID, err)
	}
	return renderJSON(t, rpt)
}

func renderJSON(t *testing.T, rpt *report.Report) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := report.WriteJSON(&out, rpt); err != nil {
		t.Fatalf("render report: %v", err)
	}
	return out.Bytes()
}

func orderFileName(turn int) string { return fmt.Sprintf("t%d-f1-orders.txt", turn) }
func engineLogName(turn int) string { return fmt.Sprintf("t%d-engine.log", turn) }
func reportName(turn int, factionID int64, kind string) string {
	return fmt.Sprintf("t%d-f%d-%s.json", turn, factionID, kind)
}

// goldenSet compares produced files against testdata/golden, or rewrites them
// under -update. It also notices a golden nothing produced any more, which is
// how a report quietly disappearing gets caught.
type goldenSet struct {
	seen map[string]bool
}

func newGoldenSet(t *testing.T) *goldenSet {
	t.Helper()
	if *update {
		if err := os.MkdirAll(goldenDir, 0o755); err != nil {
			t.Fatalf("create golden directory: %v", err)
		}
	}
	return &goldenSet{seen: make(map[string]bool)}
}

func (g *goldenSet) check(t *testing.T, name string, got []byte) {
	t.Helper()
	g.seen[name] = true
	path := filepath.Join(goldenDir, name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden %s: %v", name, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("read golden %s: %v; rerun with -update to create it", name, err)
		return
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from its golden:\n%s", name, firstDifference(string(want), string(got)))
	}
}

// finish reports goldens that this run never produced, so deleting a report
// fails the test instead of leaving a stale file behind.
func (g *goldenSet) finish(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir(goldenDir)
	if err != nil {
		t.Fatalf("read golden directory: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || g.seen[entry.Name()] {
			continue
		}
		if *update {
			if err := os.Remove(filepath.Join(goldenDir, entry.Name())); err != nil {
				t.Errorf("remove stale golden %s: %v", entry.Name(), err)
			}
			continue
		}
		t.Errorf("golden %s was not produced by this run; rerun with -update if it should be gone", entry.Name())
	}
}

// firstDifference points at the first line that changed rather than printing
// two whole reports, which are hundreds of lines each.
func firstDifference(want, got string) string {
	wantLines, gotLines := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(wantLines) || i < len(gotLines); i++ {
		wantLine, gotLine := "", ""
		if i < len(wantLines) {
			wantLine = wantLines[i]
		}
		if i < len(gotLines) {
			gotLine = gotLines[i]
		}
		if wantLine != gotLine {
			return fmt.Sprintf("  line %d\n    want: %s\n     got: %s", i+1, wantLine, gotLine)
		}
	}
	return "  files differ only in trailing bytes"
}
