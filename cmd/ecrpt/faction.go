// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"fmt"
	"net/mail"
	"strings"
)

func normalizeFactionSelector(gameCode, email string, factionNumber int64) (string, error) {
	if gameCode == "" {
		return "", fmt.Errorf("game is required")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email != "" {
		address, err := mail.ParseAddress(email)
		if err != nil || address.Address != email {
			return "", fmt.Errorf("invalid email address %q", email)
		}
	}
	if (email == "") == (factionNumber == 0) {
		return "", fmt.Errorf("exactly one of email or faction is required")
	}
	if factionNumber < 0 {
		return "", fmt.Errorf("invalid faction id %d", factionNumber)
	}
	return email, nil
}
