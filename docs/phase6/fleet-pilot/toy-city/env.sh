# Source for fleetpilot commands only (never from a shell rc).
export PATH="$HOME/fleetpilot/bin:$PATH"
export BD_DISABLE_METRICS=1 DO_NOT_TRACK=1
export BD_NON_INTERACTIVE=1
unalias gc 2>/dev/null
export GC_HOME="$HOME/fleetpilot/gchome"
export DOLT_ROOT_PATH="$HOME/fleetpilot/doltroot"
export GIT_CONFIG_GLOBAL="$HOME/fleetpilot/gitconfig"   # includes ~/.gitconfig; adds beads.role without touching it
