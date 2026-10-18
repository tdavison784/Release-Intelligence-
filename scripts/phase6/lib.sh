# Shared settings of the Phase 6 natural-agent harness (sourced, not run).
# HARNESS.md explains the isolation; PROTOCOL.md §5 is what it implements.

P6_HOME=${P6_HOME:-$HOME/ri-p6}                      # workspaces, live run copies, evidence: outside the repo
P6_EVIDENCE=${P6_EVIDENCE:-$P6_HOME/evidence}        # materialise.sh output, one dir per case id
P6_LIVE=${P6_LIVE:-$P6_HOME/live}                    # per-run workspace copies (kept for arm D)
P6_SHARED_RI=${P6_SHARED_RI:-/Users/tommydavison/repos/Release-Intelligence-/.ri}
P6_INVESTIGATOR_MODEL=claude-sonnet-5-5              # PROTOCOL §9
P6_EXTRACTOR_MODEL=claude-opus-5-5                   # PROTOCOL §8
P6_BUDGET_TOKENS=${P6_BUDGET_TOKENS:-400000}         # PROTOCOL §9; counted as RI's driver does (claudecli.go)

# The user's own claude install (the native one, not an older Homebrew copy
# that a reduced PATH would find first).
p6_claude_bin() {
  if [ -n "${CLAUDE_BIN:-}" ]; then echo "$CLAUDE_BIN"; return; fi
  if [ -x "$HOME/.local/bin/claude" ]; then echo "$HOME/.local/bin/claude"; return; fi
  command -v claude
}

# A scrubbed environment: nothing inherited from the calling Claude Code
# session (CLAUDE_*, effort, messaging socket), no kube context, no tokens.
p6_env() { # <tmpdir> -> sets the array P6_ENV (prefix a command with "${P6_ENV[@]}")
  P6_ENV=(env -i "HOME=$HOME" "USER=$USER" "LOGNAME=$USER"
    "PATH=/usr/bin:/bin:/usr/sbin:/sbin:/usr/local/bin:/opt/homebrew/bin"
    "LANG=en_US.UTF-8" "TERM=${TERM:-xterm-256color}" "TMPDIR=$1" "KUBECONFIG=/dev/null")
}

# Claude Code's per-project directory for a working directory, and the rule
# that lets a session read back its own oversized tool outputs, which the CLI
# saves under <project>/<session>/tool-results/ (outside the workspace).
p6_project_dir() { echo "$HOME/.claude/projects/$(echo "$1" | sed 's/[^A-Za-z0-9]/-/g')"; }
p6_tool_results_rule() { echo "Read(/$(p6_project_dir "$1")/$2/tool-results/**)"; } # <cwd> <session-id>

# Settings passed with --settings (user/project/local settings are not loaded).
P6_SETTINGS='{"enabledPlugins":{"agents-md@builtin":false,"telemetry@builtin":false,"plugin-authoring@builtin":false,"diff@builtin":false},"autoMemoryEnabled":false,"includeCoAuthoredBy":false,"attribution":{"commit":"","pr":""}}'

# Isolation and tool limits (PROTOCOL §5). In dontAsk mode anything not
# allowed here is denied; Read rules also govern Grep and Glob, and Claude
# Code's Bash path checks deny allowed commands that touch paths outside the
# working directory. The canary (canary.sh) verifies the result.
P6_ISOLATION_ARGS=(
  --safe-mode --setting-sources "" --strict-mcp-config --disable-slash-commands
  --settings "$P6_SETTINGS"
  --tools "Read,Grep,Glob,Bash,Write,Edit"
  --disallowedTools "WebFetch" "WebSearch" "Task" "Agent"
  --permission-mode dontAsk
)
# Passed last, as --allowedTools "${P6_ALLOWED[@]}" <tool-results rule> (the option takes a list).
P6_ALLOWED=(
  "Read(./**)"
  "Bash(ls:*)" "Bash(cat:*)" "Bash(head:*)" "Bash(tail:*)" "Bash(wc:*)" "Bash(find:*)"
  "Bash(grep:*)" "Bash(diff:*)" "Bash(jq:*)" "Bash(yq:*)" "Bash(kubectl kustomize:*)"
  "Write(./scratch/**)" "Edit(./scratch/**)"
)

p6_sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }

# Read one field of a case's input.yaml (dotted path), empty when absent.
p6_input() { # <input.yaml> <dotted.path>
  python3 -c 'import sys,yaml
v=yaml.safe_load(open(sys.argv[1]))
for k in sys.argv[2].split("."):
    v=v.get(k) if isinstance(v,dict) else None
print("" if v is None else v)' "$1" "$2"
}
