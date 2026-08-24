#!/usr/bin/env bash

set -euo pipefail

[ -d cmd/db ] || {
    echo "error: must run from repository root"
    exit 2
}
[ -d games/beta ] || {
    echo "error: missing games/beta folder"
    exit 2
}

rm -f games/beta/ecvb.db games/beta/ecvb.db-*

echo " info: create the database..."
go run ./cmd/db create games/beta

echo " info: seed the database..."
go run ./cmd/db seed games/beta

echo " info: create the game..."
go run ./cmd/ec --db-path games/beta game create --game-seed games/beta/game-seed.json

echo " info: load the game data..."
go run ./cmd/ec --db-path games/beta load game BETA-001

for player in 01 02 03 04 05 06 07 08 09 10; do
    echo " info: add player '${player}'..."
    go run ./cmd/ec --db-path games/beta add player --game BETA-001 --email "user${player}@example.com" --kit games/beta/home-planet-seed.json
done

go run ./cmd/ecrpt --db-path games/beta show --output games/beta/reports/t0-stellium-79.txt stellium 79
go run ./cmd/ecrpt --db-path games/beta show --output games/beta/reports/t0-system-89.txt   system   89
go run ./cmd/ecrpt --db-path games/beta show --output games/beta/reports/t0-system-89-d.txt system --show-deposits 89

for faction in 1 2 3; do
    go run ./cmd/ecrpt --db-path games/beta show --output games/beta/reports/t0-f${faction}-turn-report.txt turn --game BETA-001 --faction ${faction}
done

go run ./cmd/ec --db-path games/beta orders check  games/beta/orders/t0-f1-orders-v1.txt

go run ./cmd/ec --db-path games/beta orders submit games/beta/orders/t0-f1-orders-v1.txt

go run ./cmd/ecrpt --db-path games/beta show orders --game BETA-001 --email user01@example.com
go run ./cmd/ecrpt --db-path games/beta show orders --game BETA-001 --faction 2

go run ./cmd/ec --db-path games/beta turn resolve --game BETA-001 --turn 0 \
    2>games/beta/reports/t0-engine.log

for faction in 1 2 3; do
    go run ./cmd/ecrpt --db-path games/beta show \
        --output games/beta/reports/t0-f${faction}-resolved-turn-report.txt \
        turn --game BETA-001 --faction ${faction}
done

go run ./cmd/ec --db-path games/beta turn open --game BETA-001 --turn 0
