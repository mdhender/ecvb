// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func sample() *Report {
	r := New("TURN REPORT")
	r.Table("", "GAME", "TURN").Row("BETA-001", 3)
	entities := r.Table("ENTITIES", "ID", "UNIT", "MASS")
	entities.Row(40, "SHIP", 3000)
	entities.Row(41, "COPN", 1234)
	r.Table("PROBES", "SEQUENCE", "STATUS")
	return r
}

func TestWriteTextAlignsColumnsAndSeparatesTables(t *testing.T) {
	var out bytes.Buffer
	if err := WriteText(&out, sample()); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	got := out.String()
	want := strings.Join([]string{
		"TURN REPORT",
		"GAME      TURN",
		"BETA-001  3",
		"",
		"ENTITIES",
		"ID  UNIT  MASS",
		"40  SHIP  3000",
		"41  COPN  1234",
		"",
		"PROBES",
		"SEQUENCE  STATUS",
		"",
	}, "\n")
	if got != want {
		t.Errorf("WriteText =\n%q\nwant\n%q", got, want)
	}
}

// A report with an empty table must still round-trip as an empty list rather
// than as null, so a golden file reads the same whether or not rows were found.
func TestWriteJSONEncodesEmptyTablesAsLists(t *testing.T) {
	var out bytes.Buffer
	if err := WriteJSON(&out, New("EMPTY")); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if strings.Contains(out.String(), "null") {
		t.Errorf("WriteJSON = %s; want no nulls", out.String())
	}
	var decoded Report
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Title != "EMPTY" || len(decoded.Tables) != 0 {
		t.Errorf("decoded = %+v; want title EMPTY and no tables", decoded)
	}
}

func TestWriteJSONRoundTrips(t *testing.T) {
	var out bytes.Buffer
	if err := WriteJSON(&out, sample()); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var decoded Report
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Title != "TURN REPORT" || len(decoded.Tables) != 3 {
		t.Fatalf("decoded %d tables titled %q; want 3 and TURN REPORT", len(decoded.Tables), decoded.Title)
	}
	entities := decoded.Tables[1]
	if entities.Name != "ENTITIES" || len(entities.Rows) != 2 || entities.Rows[0][1] != "SHIP" {
		t.Errorf("entities table = %+v; want ENTITIES with SHIP in the first row", entities)
	}
	if got := decoded.Tables[2].Rows; len(got) != 0 {
		t.Errorf("probes rows = %v; want none", got)
	}
}

// Rendering must not depend on anything outside the report, so the same report
// written twice produces the same bytes.
func TestWriteIsReproducible(t *testing.T) {
	for _, format := range []Format{FormatText, FormatJSON} {
		var first, second bytes.Buffer
		if err := Write(&first, sample(), format); err != nil {
			t.Fatalf("Write %s: %v", format, err)
		}
		if err := Write(&second, sample(), format); err != nil {
			t.Fatalf("Write %s: %v", format, err)
		}
		if first.String() != second.String() {
			t.Errorf("format %s is not reproducible", format)
		}
	}
}

func TestParseFormat(t *testing.T) {
	for _, tc := range []struct {
		input     string
		want      Format
		wantError string
	}{
		{input: "", want: FormatText},
		{input: "text", want: FormatText},
		{input: "json", want: FormatJSON},
		{input: "JSON", want: FormatJSON},
		{input: " json ", want: FormatJSON},
		{input: "yaml", wantError: `invalid format "yaml"`},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseFormat(tc.input)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("ParseFormat(%q) error = %v; want containing %q", tc.input, err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFormat(%q): %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseFormat(%q) = %q; want %q", tc.input, got, tc.want)
			}
		})
	}
}
