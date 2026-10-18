#!/bin/sh
# Foreground gascity supervisor for the toy city (never `gc start`/`gc register`: those install a launchd agent).
. "$HOME/fleetpilot/env.sh"

exec "$HOME/fleetpilot/bin/gc" supervisor run
