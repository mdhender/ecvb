#!/usr/bin/env bash
#
# Play the EAGLE-01 rule-lawyer mini-game from the committed order files.
#
#   games/eagles/replay.sh [FIRST-TURN] [LAST-TURN]
#
# Unlike games/fuzzers, every order file here is meant to be accepted: a rule
# lawyer argues about what the engine DID, so its orders have to parse, bind,
# and resolve. A refused file is a bug in the docket, not a finding, and the
# run says so.
#
# What the run produces is games/eagles/reports/, and the arguing is done by
# reading those against docs/. Findings go in docs/plan/engine-burndown.md.
set -euo pipefail

[ -d cmd/db ] || { echo "error: must run from repository root"; exit 2; }

GAME=EAGLE-01
DB=games/eagles
ORDERS=games/eagles/orders
OUT=games/eagles/reports
MAP=games/claude
FIRST="${1:-0}"
LAST="${2:-3}"
FACTIONS="1 3 4 5 6 7 8 9 10 11"

BIN="$(mktemp -d)"
trap 'rm -rf "${BIN}"' EXIT
echo " info: building binaries..."
go build -o "${BIN}/db" ./cmd/db
go build -o "${BIN}/ec" ./cmd/ec
go build -o "${BIN}/ecrpt" ./cmd/ecrpt

# The map is games/claude's. The lawyers argue about rules, not about terrain.
for seed in stellia systems planets deposits; do
    cp "${MAP}/${seed}-seed.json" "${DB}/${seed}-seed.json"
done
cp "${MAP}/home-planet-seed.json" "${DB}/home-planet-seed.json"

rm -f "${DB}/ecvb.db" "${DB}/ecvb.db-"*
rm -rf "${OUT}"
mkdir -p "${OUT}"

echo " info: create the database..."
"${BIN}/db" create "${DB}"
"${BIN}/db" seed "${DB}"
"${BIN}/ec" --db-path "${DB}" game create --game-seed "${DB}/game-seed.json"
"${BIN}/ec" --db-path "${DB}" load game "${GAME}"
for player in 01 02 03 04 05 06 07 08 09 10; do
    "${BIN}/ec" --db-path "${DB}" add player --game "${GAME}" \
        --email "user${player}@example.com" --kit "${DB}/home-planet-seed.json" >/dev/null
done

refused=0
for turn in $(seq "${FIRST}" "${LAST}"); do
    echo " info: turn ${turn}: the lawyers file..."
    for faction in ${FACTIONS}; do
        file="${ORDERS}/t${turn}-f${faction}-orders-v1.txt"
        [ -f "${file}" ] || continue
        # A docket that does not submit has argued nothing, so a refusal is
        # reported loudly: it is a mistake in the docket, not a finding.
        if ! "${BIN}/ec" --db-path "${DB}" orders check "${file}" \
                >"${OUT}/t${turn}-f${faction}-check.txt" 2>&1; then
            echo " !! turn ${turn} faction ${faction}: REFUSED, see ${OUT}/t${turn}-f${faction}-check.txt"
            refused=$((refused + 1))
            continue
        fi
        "${BIN}/ec" --db-path "${DB}" orders submit "${file}" >/dev/null
    done
    "${BIN}/ec" --db-path "${DB}" turn resolve --game "${GAME}" --turn "${turn}" \
        --no-log-timestamps >/dev/null 2>"${OUT}/t${turn}-engine.log"
    for faction in ${FACTIONS}; do
        "${BIN}/ecrpt" --db-path "${DB}" show --output "${OUT}/t${turn}-f${faction}-orders-report.txt" \
            orders --game "${GAME}" --turn "${turn}" --faction "${faction}"
        "${BIN}/ecrpt" --db-path "${DB}" show --output "${OUT}/t${turn}-f${faction}-turn-report.txt" \
            turn --game "${GAME}" --faction "${faction}"
    done
    if [ "${turn}" -lt "${LAST}" ]; then
        "${BIN}/ec" --db-path "${DB}" turn open --game "${GAME}" --turn "${turn}" >/dev/null
    fi
done

echo " info: ${refused} file(s) refused; reports in ${OUT}"
