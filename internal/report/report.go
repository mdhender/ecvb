// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package report models the reports ecrpt prints.
//
// Every report in the game is the same shape: a title, then a run of named
// tables, each with a header row and zero or more data rows. Building that
// shape before rendering it lets one report be printed as the aligned text a
// player reads or as the JSON a golden test diffs, without writing the queries
// twice.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// Table is one section of a report.
type Table struct {
	// Name is the heading printed above the table. The first table of a
	// report usually has none, because the title already names it.
	Name    string     `json:"name"`
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

// Report is a title and the tables under it.
type Report struct {
	Title  string  `json:"title"`
	Tables []Table `json:"tables"`
}

// New returns a report with the given title.
func New(title string) *Report {
	return &Report{Title: title}
}

// Table appends a table and returns it. Pass an empty name for the leading
// table of a report, which prints its header directly under the title.
func (r *Report) Table(name string, columns ...string) *Table {
	r.Tables = append(r.Tables, Table{Name: name, Columns: columns})
	return &r.Tables[len(r.Tables)-1]
}

// Row appends a row, formatting each value with %v. The number of values
// should match the table's columns; rendering does not enforce it, because a
// report that is one cell short should still print.
func (t *Table) Row(values ...any) {
	row := make([]string, len(values))
	for i, value := range values {
		row[i] = fmt.Sprintf("%v", value)
	}
	t.Rows = append(t.Rows, row)
}

// WriteText prints the report as column-aligned text.
func WriteText(w io.Writer, r *Report) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, r.Title)
	for _, table := range r.Tables {
		if table.Name != "" {
			fmt.Fprintf(tw, "\n%s\n", table.Name)
		}
		fmt.Fprintln(tw, strings.Join(table.Columns, "\t"))
		for _, row := range table.Rows {
			fmt.Fprintln(tw, strings.Join(row, "\t"))
		}
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

// WriteJSON prints the report as indented JSON. Golden tests compare this
// rather than the text, because it carries the report's structure and holds
// still when a column widens.
func WriteJSON(w io.Writer, r *Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	// A report with no tables should still encode as an empty list rather
	// than as null, so a golden file reads the same either way.
	out := *r
	if out.Tables == nil {
		out.Tables = []Table{}
	}
	for i := range out.Tables {
		if out.Tables[i].Rows == nil {
			out.Tables[i].Rows = [][]string{}
		}
		if out.Tables[i].Columns == nil {
			out.Tables[i].Columns = []string{}
		}
	}
	if err := encoder.Encode(out); err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	return nil
}

// Format names an output format.
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

// ParseFormat converts a --format flag value into a Format.
func ParseFormat(value string) (Format, error) {
	switch Format(strings.ToLower(strings.TrimSpace(value))) {
	case "", FormatText:
		return FormatText, nil
	case FormatJSON:
		return FormatJSON, nil
	default:
		return "", fmt.Errorf("invalid format %q; want text or json", value)
	}
}

// Write prints the report in the given format.
func Write(w io.Writer, r *Report, format Format) error {
	if format == FormatJSON {
		return WriteJSON(w, r)
	}
	return WriteText(w, r)
}
