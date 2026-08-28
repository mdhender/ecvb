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

# The map in games/beta/*-seed.json is committed and is an INPUT here: this
# script loads it and never regenerates it. It was made with
#
#     for stage in stellia systems planets deposits; do
#         go run ./cmd/ecgen "$stage" --stellia-seed 1912 games/beta
#     done
#
# (only the first stage reads the flag; the rest take the seed from the file
# they read). ecgen refuses to overwrite, so regenerating means deleting the
# four files first -- and doing that invalidates every coordinate the order
# files name.

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

go run ./cmd/ecrpt --db-path games/beta show --output games/beta/reports/t0-stellium-74.txt stellium 74
go run ./cmd/ecrpt --db-path games/beta show --output games/beta/reports/t0-system-82.txt   system   82
go run ./cmd/ecrpt --db-path games/beta show --output games/beta/reports/t0-system-82-d.txt system --show-deposits 82

for faction in 1 2 3; do
    go run ./cmd/ecrpt --db-path games/beta show --output games/beta/reports/t0-f${faction}-turn-report.txt turn --game BETA-001 --faction ${faction}
done

go run ./cmd/ec --db-path games/beta orders check  games/beta/orders/t0-f2-orders-v1.txt

go run ./cmd/ec --db-path games/beta orders submit games/beta/orders/t0-f2-orders-v1.txt

go run ./cmd/ecrpt --db-path games/beta show orders --game BETA-001 --email user01@example.com
go run ./cmd/ecrpt --db-path games/beta show orders --game BETA-001 --faction 1

go run ./cmd/ec --db-path games/beta turn resolve --game BETA-001 --turn 0 \
    2>games/beta/reports/t0-engine.log

for faction in 1 2 3; do
    go run ./cmd/ecrpt --db-path games/beta show \
        --output games/beta/reports/t0-f${faction}-resolved-turn-report.txt \
        turn --game BETA-001 --faction ${faction}
done

go run ./cmd/ec --db-path games/beta turn open --game BETA-001 --turn 0
