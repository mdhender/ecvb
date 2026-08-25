// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package logging

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestNewRecordsTheWallClock(t *testing.T) {
	var out bytes.Buffer
	New(&out).InfoContext(context.Background(), "order resolved", "turn", 3)
	if !strings.HasPrefix(out.String(), "time=") {
		t.Errorf("log = %q; want it to start with time=", out.String())
	}
}

func TestNewWithoutTimeOmitsTheWallClock(t *testing.T) {
	var out bytes.Buffer
	NewWithoutTime(&out).InfoContext(context.Background(), "order resolved", "turn", 3)
	got := out.String()
	if strings.Contains(got, "time=") {
		t.Errorf("log = %q; want no time=", got)
	}
	if !strings.HasPrefix(got, "level=INFO msg=\"order resolved\"") {
		t.Errorf("log = %q; want it to start with the level and message", got)
	}
	if !strings.Contains(got, "turn=3") {
		t.Errorf("log = %q; want it to keep the attributes", got)
	}
}

// Golden files depend on this: the same call must write the same bytes every
// time, which is exactly what dropping the wall clock buys.
func TestNewWithoutTimeIsReproducible(t *testing.T) {
	var first, second bytes.Buffer
	NewWithoutTime(&first).InfoContext(context.Background(), "order resolved", "turn", 3, "status", "succeeded")
	NewWithoutTime(&second).InfoContext(context.Background(), "order resolved", "turn", 3, "status", "succeeded")
	if first.String() != second.String() {
		t.Errorf("logs differ:\n%q\n%q", first.String(), second.String())
	}
}

// An attribute a caller happens to name "time" is data, not the record's
// timestamp, and must survive.
func TestNewWithoutTimeKeepsAGroupedTimeAttribute(t *testing.T) {
	var out bytes.Buffer
	NewWithoutTime(&out).WithGroup("start").InfoContext(context.Background(), "order resolved", "time", 7)
	if !strings.Contains(out.String(), "start.time=7") {
		t.Errorf("log = %q; want it to keep start.time=7", out.String())
	}
}

func TestNewLoggerSelectsByFlag(t *testing.T) {
	for _, tc := range []struct {
		name     string
		withTime bool
		wantTime bool
	}{
		{name: "with timestamps", withTime: true, wantTime: true},
		{name: "without timestamps", withTime: false, wantTime: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			NewLogger(&out, tc.withTime).InfoContext(context.Background(), "resolved")
			if got := strings.Contains(out.String(), "time="); got != tc.wantTime {
				t.Errorf("log %q contains time = %v; want %v", out.String(), got, tc.wantTime)
			}
		})
	}
}
