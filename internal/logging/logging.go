// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package logging builds the loggers the commands hand to the engine.
//
// The engine writes one structured record per resolved order, which is both
// the operator's audit trail and, for a golden test, the record of what the
// turn did. Those two uses disagree about the wall clock: an operator wants
// the time, and a golden file cannot hold one, because the same turn resolved
// twice would produce two different files. NewWithoutTime drops it.
package logging

import (
	"io"
	"log/slog"
)

// New returns a text logger that records the wall-clock time of each entry.
// This is what an operator wants in an engine log.
func New(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, nil))
}

// NewWithoutTime returns a text logger that omits the wall-clock timestamp.
// Its output depends only on what was logged, so resolving the same turn twice
// writes the same bytes and the log can be compared against a golden file.
func NewWithoutTime(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			// Only the record's own time key is dropped. An attribute a
			// caller named "time" inside a group is its own business.
			if len(groups) == 0 && attr.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return attr
		},
	}))
}

// NewLogger returns a logger with or without timestamps.
func NewLogger(w io.Writer, withTime bool) *slog.Logger {
	if withTime {
		return New(w)
	}
	return NewWithoutTime(w)
}
