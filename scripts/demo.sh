#!/bin/bash
# A short scripted Peapod demo. Run it, or screen-record it to make a GIF.
# Needs OrbStack (or Docker) running.
set -e
PEAPOD="${PEAPOD:-./bin/peapod}"
g() { printf '\033[32m$\033[0m %s\n' "$*"; }

echo "# Peapod — an isolated, audited Python sandbox"
echo
g "peapod sandbox create python:3.12-slim --net none"
ID=$("$PEAPOD" sandbox create python:3.12-slim --net none)
echo "$ID"; echo

g "peapod sandbox exec $ID python3 -c 'print(6*7)'"
"$PEAPOD" sandbox exec "$ID" python3 -c 'print(6*7)'; echo

g "peapod sandbox exec $ID  # prove there's no network"
"$PEAPOD" sandbox exec "$ID" sh -lc 'wget -T2 -q -O- http://example.com >/dev/null 2>&1 && echo "has network" || echo "no network (isolated)"'; echo

g "peapod sandbox history $ID  # audit trail"
"$PEAPOD" sandbox history "$ID"; echo

g "peapod sandbox rm $ID"
"$PEAPOD" sandbox rm "$ID"
