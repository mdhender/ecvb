// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package orders parses and validates player order files.
package orders

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

var (
	gamePattern       = regexp.MustCompile(`(?i)^game[ \t]+"([^"]+)"[ \t]+turn[ \t]+([0-9]+)[ \t]*$`)
	playerPattern     = regexp.MustCompile(`(?i)^id[ \t]+player[ \t]+"([^"]+)"[ \t]*$`)
	factionPattern    = regexp.MustCompile(`(?i)^id[ \t]+faction[ \t]+([0-9]+)[ \t]*$`)
	jumpPattern       = regexp.MustCompile(`(?i)^jump[ \t]+ship[ \t]+([0-9]+)[ \t]+to[ \t]+\([ \t]*(-?[0-9]+)[ \t]*,[ \t]*(-?[0-9]+)[ \t]*,[ \t]*(-?[0-9]+)[ \t]*\)[ \t]*$`)
	movePattern       = regexp.MustCompile(`(?i)^move[ \t]+ship[ \t]+([0-9]+)[ \t]+to[ \t]+orbit[ \t]+([0-9]+)[ \t]*$`)
	moveSystemPattern = regexp.MustCompile(`(?i)^move[ \t]+ship[ \t]+([0-9]+)[ \t]+to[ \t]+system[ \t]+([A-E])[ \t]+orbit[ \t]+([0-9]+)[ \t]*$`)
)

// Identity identifies the faction submitting an order file.
type Identity struct {
	PlayerEmail string
	FactionID   int64
}

// Order is one parsed ship order.
type Order struct {
	Line   int
	Verb   string
	ShipID int64
	X      int
	Y      int
	Z      int
	System string
	Orbit  int
}

// Submission is the parsed contents of an order file.
type Submission struct {
	GameCode string
	Turn     int
	Identity Identity
	Orders   []Order
}

type problem struct {
	line    int
	message string
}

type problems []problem

func (p problems) Error() string {
	lines := make([]string, len(p))
	for i, item := range p {
		lines[i] = fmt.Sprintf("line %d: %s", item.line, item.message)
	}
	return strings.Join(lines, "\n")
}

// Parse parses an order file without consulting the database.
func Parse(r io.Reader) (Submission, error) {
	var submission Submission
	var found problems
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSuffix(scanner.Text(), "\r")
		switch line {
		case 1:
			matches := gamePattern.FindStringSubmatch(text)
			if matches == nil {
				found = append(found, problem{line, `expected game "CODE" turn NUMBER`})
				continue
			}
			submission.GameCode = matches[1]
			turn, err := strconv.Atoi(matches[2])
			if err != nil {
				found = append(found, problem{line, "turn is too large"})
				continue
			}
			submission.Turn = turn
		case 2:
			if matches := playerPattern.FindStringSubmatch(text); matches != nil {
				submission.Identity.PlayerEmail = matches[1]
				continue
			}
			if matches := factionPattern.FindStringSubmatch(text); matches != nil {
				id, err := positiveInt64(matches[1])
				if err != nil {
					found = append(found, problem{line, "invalid faction id: " + err.Error()})
					continue
				}
				submission.Identity.FactionID = id
				continue
			}
			found = append(found, problem{line, `expected id player "EMAIL" or id faction NUMBER`})
		default:
			if strings.TrimSpace(text) == "" {
				continue
			}
			order, err := parseOrder(line, text)
			if err != nil {
				found = append(found, problem{line, err.Error()})
				continue
			}
			submission.Orders = append(submission.Orders, order)
		}
	}
	if err := scanner.Err(); err != nil {
		return Submission{}, fmt.Errorf("read orders: %w", err)
	}
	if line < 1 {
		found = append(found, problem{1, `expected game "CODE" turn NUMBER`})
	}
	if line < 2 {
		found = append(found, problem{2, `expected id player "EMAIL" or id faction NUMBER`})
	}
	if len(found) != 0 {
		return Submission{}, found
	}
	return submission, nil
}

func parseOrder(line int, text string) (Order, error) {
	if matches := jumpPattern.FindStringSubmatch(text); matches != nil {
		shipID, err := positiveInt64(matches[1])
		if err != nil {
			return Order{}, fmt.Errorf("invalid ship id: %w", err)
		}
		coordinates := [3]int{}
		for i := range coordinates {
			coordinates[i], err = strconv.Atoi(matches[i+2])
			if err != nil {
				return Order{}, fmt.Errorf("coordinate is too large")
			}
		}
		return Order{Line: line, Verb: "jump", ShipID: shipID, X: coordinates[0], Y: coordinates[1], Z: coordinates[2]}, nil
	}
	if matches := moveSystemPattern.FindStringSubmatch(text); matches != nil {
		shipID, err := positiveInt64(matches[1])
		if err != nil {
			return Order{}, fmt.Errorf("invalid ship id: %w", err)
		}
		orbit, err := strconv.Atoi(matches[3])
		if err != nil {
			return Order{}, fmt.Errorf("orbit is too large")
		}
		return Order{Line: line, Verb: "move", ShipID: shipID, System: strings.ToUpper(matches[2]), Orbit: orbit}, nil
	}
	if matches := movePattern.FindStringSubmatch(text); matches != nil {
		shipID, err := positiveInt64(matches[1])
		if err != nil {
			return Order{}, fmt.Errorf("invalid ship id: %w", err)
		}
		orbit, err := strconv.Atoi(matches[2])
		if err != nil {
			return Order{}, fmt.Errorf("orbit is too large")
		}
		return Order{Line: line, Verb: "move", ShipID: shipID, Orbit: orbit}, nil
	}
	return Order{}, fmt.Errorf("expected jump ship ID to (X,Y,Z), move ship ID to orbit N, or move ship ID to system S orbit N")
}

func positiveInt64(text string) (int64, error) {
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("number is too large")
	}
	if n < 1 {
		return 0, fmt.Errorf("must be positive")
	}
	return n, nil
}
