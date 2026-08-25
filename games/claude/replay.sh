#!/usr/bin/env bash
#
# Replay the whole CLAUDE-01 game from the committed order files and write every
# report to a directory. This is the golden-capture and golden-verify harness:
# the reports it produces are the contract that the order-pipeline rework must
# preserve byte for byte.
#
#   games/claude/replay.sh [--json] [OUTPUT-DIR]
#
# The default output directory is games/claude/reports. Pass a different one to
# capture a candidate set for diffing against the goldens.
#
# --json writes the reports as JSON and drops the wall-clock timestamps from
# the engine log, so replaying the same game twice writes the same bytes. That
# is what a golden file needs; the default text output is what a player reads.

set -euo pipefail

[ -d cmd/db ] || {
    echo "error: must run from repository root"
    exit 2
}
[ -d games/claude ] || {
    echo "error: missing games/claude folder"
    exit 2
}

FORMAT=text
EXT=txt
# A single optional flag, held as a scalar rather than an array: bash 3.2 ships
# with macOS, and there an empty array expansion trips `set -u`.
LOGFLAG=
if [ "${1:-}" = "--json" ]; then
    FORMAT=json
    EXT=json
    LOGFLAG=--no-log-timestamps
    shift
fi

GAME=CLAUDE-01
DB=games/claude
ORDERS=games/claude/orders
OUT="${1:-games/claude/reports}"
LAST_TURN=6
# Faction 2 is the uncontrolled agent the game load creates; the ten player
# factions are not contiguous with the player numbers.
FACTIONS="1 3 4 5 6 7 8 9 10 11"

mkdir -p "${OUT}"

# Build once. Replaying seven turns takes a few hundred command invocations and
# `go run` would recompile for every one of them.
BIN="$(mktemp -d)"
trap 'rm -rf "${BIN}"' EXIT
echo " info: building binaries..."
go build -o "${BIN}/db" ./cmd/db
go build -o "${BIN}/ec" ./cmd/ec
go build -o "${BIN}/ecrpt" ./cmd/ecrpt

rm -f "${DB}/ecvb.db" "${DB}/ecvb.db-"*

echo " info: create the database..."
"${BIN}/db" create "${DB}"
echo " info: seed the database..."
"${BIN}/db" seed "${DB}"
echo " info: create the game..."
"${BIN}/ec" --db-path "${DB}" game create --game-seed "${DB}/game-seed.json"
echo " info: load the game data..."
"${BIN}/ec" --db-path "${DB}" load game "${GAME}"

for player in 01 02 03 04 05 06 07 08 09 10; do
    echo " info: add player '${player}'..."
    "${BIN}/ec" --db-path "${DB}" add player --game "${GAME}" \
        --email "user${player}@example.com" --kit "${DB}/home-planet-seed.json"
done

for turn in $(seq 0 "${LAST_TURN}"); do
    echo " info: turn ${turn}: submit orders..."
    for faction in ${FACTIONS}; do
        file="${ORDERS}/t${turn}-f${faction}-orders-v1.txt"
        [ -f "${file}" ] || continue
        "${BIN}/ec" --db-path "${DB}" orders check "${file}" >/dev/null
        "${BIN}/ec" --db-path "${DB}" orders submit "${file}" >/dev/null
    done

    echo " info: turn ${turn}: resolve..."
    "${BIN}/ec" --db-path "${DB}" turn resolve --game "${GAME}" --turn "${turn}" \
        ${LOGFLAG} >/dev/null 2>"${OUT}/t${turn}-engine.log"

    echo " info: turn ${turn}: report..."
    for faction in ${FACTIONS}; do
        "${BIN}/ecrpt" --db-path "${DB}" show --format "${FORMAT}" \
            --output "${OUT}/t${turn}-f${faction}-orders-report.${EXT}" \
            orders --game "${GAME}" --turn "${turn}" --faction "${faction}"
        "${BIN}/ecrpt" --db-path "${DB}" show --format "${FORMAT}" \
            --output "${OUT}/t${turn}-f${faction}-resolved-turn-report.${EXT}" \
            turn --game "${GAME}" --faction "${faction}"
    done

    if [ "${turn}" -lt "${LAST_TURN}" ]; then
        "${BIN}/ec" --db-path "${DB}" turn open --game "${GAME}" --turn "${turn}"
    fi
done

echo " info: replay complete; reports in ${OUT}"
