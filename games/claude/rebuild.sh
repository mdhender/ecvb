#!/usr/bin/env bash

set -euo pipefail

[ -d cmd/db ] || {
    echo "error: must run from repository root"
    exit 2
}
[ -d games/claude ] || {
    echo "error: missing games/claude folder"
    exit 2
}

rm -f games/claude/ecvb.db games/claude/ecvb.db-*

echo " info: create the database..."
go run ./cmd/db create games/claude

echo " info: seed the database..."
go run ./cmd/db seed games/claude

echo " info: create the game..."
go run ./cmd/ec --db-path games/claude game create --game-seed games/claude/game-seed.json

echo " info: load the game data..."
go run ./cmd/ec --db-path games/claude load game CLAUDE-01

for player in 01 02 03 04 05 06 07 08 09 10; do
    echo " info: add player '${player}'..."
    go run ./cmd/ec --db-path games/claude add player --game CLAUDE-01 --email "user${player}@example.com" --kit games/claude/home-planet-seed.json
done
