#!/usr/bin/env bash
#
# Play the FUZZ-01 fuzzing mini-game from the committed order files.
#
#   games/fuzzers/replay.sh [FIRST-TURN] [LAST-TURN]
#
# Every faction attacks the parser instead of the map, so every order file is
# expected to be refused and a refusal is not an error here. What the run
# produces is games/fuzzers/diagnostics/, one file per order file, holding what
# the parser said. Read those; the findings worth keeping go in
# docs/plan/parser-burndown.md.
#
# The one thing that ends the game is a panic, and the run says so loudly and
# exits non-zero.
set -euo pipefail

[ -d cmd/db ] || { echo "error: must run from repository root"; exit 2; }

GAME=FUZZ-01
DB=games/fuzzers
ORDERS=games/fuzzers/orders
DIAG=games/fuzzers/diagnostics
MAP=games/claude
FIRST="${1:-0}"
LAST="${2:-2}"
# The ten player factions. Faction 1 is the uncontrolled agent that holds the
# derelicts a kit hands over; it is made before the first player and gives no
# orders, so the players are 2 through 11.
FACTIONS="2 3 4 5 6 7 8 9 10 11"

BIN="$(mktemp -d)"
trap 'rm -rf "${BIN}"' EXIT
echo " info: building binaries..."
go build -o "${BIN}/db" ./cmd/db
go build -o "${BIN}/ec" ./cmd/ec

# The map is games/claude's. Copying it rather than committing a second copy
# keeps four megabytes of deposits out of the repository twice over; the
# fuzzers never move, so which map it is does not matter.
for seed in stellia systems planets deposits; do
    cp "${MAP}/${seed}-seed.json" "${DB}/${seed}-seed.json"
done
cp "${MAP}/home-planet-seed.json" "${DB}/home-planet-seed.json"

rm -f "${DB}/ecvb.db" "${DB}/ecvb.db-"*
rm -rf "${DIAG}"
mkdir -p "${DIAG}"

echo " info: create the database..."
"${BIN}/db" create "${DB}"
"${BIN}/db" seed "${DB}"
"${BIN}/ec" --db-path "${DB}" game create --game-seed "${DB}/game-seed.json"
"${BIN}/ec" --db-path "${DB}" load game "${GAME}"
for player in 01 02 03 04 05 06 07 08 09 10; do
    "${BIN}/ec" --db-path "${DB}" add player --game "${GAME}" \
        --email "user${player}@example.com" --kit "${DB}/home-planet-seed.json" >/dev/null
done

panicked=0
accepted=0
refused=0
for turn in $(seq "${FIRST}" "${LAST}"); do
    echo " info: turn ${turn}: the factions attack..."
    for faction in ${FACTIONS}; do
        file="${ORDERS}/t${turn}-f${faction}-orders-v1.txt"
        [ -f "${file}" ] || continue
        out="${DIAG}/t${turn}-f${faction}.txt"
        if "${BIN}/ec" --db-path "${DB}" orders check "${file}" >"${out}" 2>&1; then
            accepted=$((accepted + 1))
            "${BIN}/ec" --db-path "${DB}" orders submit "${file}" >/dev/null 2>&1 || true
        else
            refused=$((refused + 1))
        fi
        if grep -qE '^panic:|^runtime error:|^goroutine [0-9]+ \[' "${out}"; then
            echo " !!!! FACTION ${faction} PANICKED THE PARSER ON TURN ${turn}: ${out}"
            panicked=$((panicked + 1))
        fi
    done
    "${BIN}/ec" --db-path "${DB}" turn resolve --game "${GAME}" --turn "${turn}" \
        --no-log-timestamps >/dev/null 2>&1
    if [ "${turn}" -lt "${LAST}" ]; then
        "${BIN}/ec" --db-path "${DB}" turn open --game "${GAME}" --turn "${turn}" >/dev/null
    fi
done

echo " info: ${accepted} files accepted, ${refused} refused, ${panicked} panics"
echo " info: what the parser said is in ${DIAG}"
[ "${panicked}" -eq 0 ] || exit 1
