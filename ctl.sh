#!/usr/bin/env bash
# The one entry point for this project. Never call docker/compose/go directly -- this script owns
# the dev/prod distinction, the port, the health check and the safety confirmations, and a command
# that bypasses it is a command that skips one of them.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

ENV_FILE=".env"
ENV_EXAMPLE=".env.example"

# Settings live in the environment, and ENV_FILE is the operator's copy of ENV_EXAMPLE. It is
# gitignored, and it is deliberately NOT created here: a file conjured with defaults is exactly the
# silent default that internal/config refuses to boot on. When it is missing, the binary's own
# refusal names the variables, which is the message worth reading.
if [[ -f "$ENV_FILE" ]]; then
    set -a
    # shellcheck source=/dev/null
    source "./$ENV_FILE"
    set +a
fi

PROJECT="goretrotv"
PORT="${GORETROTV_PORT:-8099}"
HEALTH_URL="http://127.0.0.1:${PORT}/health"

# Colour only when a terminal is watching. A gate's output gets grepped, and an escape sequence
# between the indent and the word defeats the grep that was supposed to read it.
if [[ -t 1 ]]; then
    RED=$'\033[31m'; GREEN=$'\033[32m'; YELLOW=$'\033[33m'; BOLD=$'\033[1m'; OFF=$'\033[0m'
else
    RED=''; GREEN=''; YELLOW=''; BOLD=''; OFF=''
fi

say()  { printf '%s\n' "$*"; }
ok()   { printf '%s%s%s\n' "$GREEN" "$*" "$OFF"; }
warn() { printf '%s%s%s\n' "$YELLOW" "$*" "$OFF" >&2; }
die()  { printf '%s%s%s\n' "$RED" "$*" "$OFF" >&2; exit 1; }

require_firmware() {
    # Nothing in this repository runs without the flash images, and they are gitignored because
    # they are Pace's. Saying so plainly beats a confusing failure deep inside the loader.
    local missing=()
    for f in firmware/FLASH_U202.bin firmware/FLASH_U203.bin; do
        [[ -f "$f" ]] || missing+=("$f")
    done
    if (( ${#missing[@]} )); then
        die "missing firmware: ${missing[*]}
The flash images are not redistributable and are not in git. See firmware/MANIFEST.md."
    fi
}

cmd_build() { make build; }

require_env_file() {
    [[ -f "$ENV_FILE" ]] && return 0
    warn "no $ENV_FILE; copy it from $ENV_EXAMPLE and edit it (cp $ENV_EXAMPLE $ENV_FILE)"
    warn "continuing so the loader can name what is actually missing"
}

cmd_run() {
    require_firmware
    require_env_file
    make run
}

# Build identity for the image. Without these, compose falls back to VERSION=dev/COMMIT=unknown and
# /health answers with a build nobody can name -- which defeats the reason it reports one at all.
export_build_args() {
    VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
    COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
    BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    export VERSION COMMIT BUILD_DATE
}

cmd_up() {
    require_firmware
    require_env_file
    export_build_args
    docker compose up -d --build
    cmd_health
}

cmd_down() { docker compose down; }

cmd_restart() { cmd_down; cmd_up; }

cmd_status() {
    docker compose ps
    say ""
    cmd_health || true
}

cmd_logs() { docker compose logs -f --tail="${1:-100}" "$PROJECT"; }

cmd_health() {
    local tries=30
    for ((i = 1; i <= tries; i++)); do
        if curl -fsS --max-time 2 "$HEALTH_URL" >/dev/null 2>&1; then
            ok "healthy: $HEALTH_URL"
            curl -fsS "$HEALTH_URL"
            say ""
            return 0
        fi
        sleep 1
    done
    die "unhealthy: $HEALTH_URL did not answer within ${tries}s"
}

cmd_gate() {
    # The boot gate: start the real build on the real firmware and assert it comes up. Unit tests
    # cannot see a model that runs for ever doing something plausible; only starting it can.
    # It refuses rather than skips when the firmware is absent, so it can never wear the colour of
    # a pass without having checked anything.
    ./tools/boot-gate.sh "$@"
}

cmd_test() { make test-race; }

# The five hooks git must be running, and the beads hooks each one delegates to.
HOOK_NAMES=(pre-commit pre-push post-merge post-checkout prepare-commit-msg)

cmd_hooks() {
    # This check lives out here rather than inside a hook deliberately. Every failure below is a
    # failure in which the hooks are NOT INVOKED, so a hook cannot report it -- and git says
    # nothing: point core.hooksPath at a directory that does not exist and `git commit` exits 0,
    # prints nothing, and makes the commit. Unguarded is indistinguishable from guarded, which is
    # how four proof cases came back green against no hook at all.
    local problems=0
    local configured
    configured="$(git config core.hooksPath || true)"

    if [[ "$configured" != ".githooks" ]]; then
        warn "core.hooksPath is '${configured:-unset}', not .githooks -- the credential guard is NOT running"
        warn "  fix: git config core.hooksPath .githooks"
        problems=1
    fi

    if [[ ! -d .githooks ]]; then
        warn "the .githooks directory is missing -- git runs no hooks at all and says nothing about it"
        problems=1
    else
        for h in "${HOOK_NAMES[@]}"; do
            if [[ ! -f ".githooks/$h" ]]; then
                warn "missing hook: .githooks/$h"
                problems=1
            elif [[ ! -x ".githooks/$h" ]]; then
                warn "not executable, so git will skip it silently: .githooks/$h"
                problems=1
            fi
        done
    fi

    # The delegation targets. Losing these does not break a commit -- it quietly stops beads doing
    # its half, which is the kind of breakage nobody notices for a week.
    for h in "${HOOK_NAMES[@]}"; do
        if [[ ! -x ".beads/hooks/$h" ]]; then
            warn "beads hook missing or not executable: .beads/hooks/$h (bd hooks install)"
            problems=1
        fi
    done

    if (( problems )); then
        die "hooks are not armed. A commit made now would look exactly like a guarded one."
    fi
    ok "hooks armed: core.hooksPath=.githooks, ${#HOOK_NAMES[@]} shims present, beads delegation intact"
}

cmd_lint() {
    # Folded in so the hook assertion runs wherever checks run, rather than depending on somebody
    # remembering a verb that only matters when it fails.
    cmd_hooks
    make vet
    # `go install` puts it in GOPATH/bin, which is not on PATH in a plain non-login shell. Looking
    # there before giving up is the difference between running the linters and reporting a pass
    # having run twelve fewer of them than the name implies.
    local gopath_bin
    gopath_bin="$(go env GOPATH)/bin"
    if ! command -v golangci-lint >/dev/null 2>&1 && [[ -x "$gopath_bin/golangci-lint" ]]; then
        PATH="$gopath_bin:$PATH"
        export PATH
    fi
    command -v golangci-lint >/dev/null 2>&1 \
        || die "golangci-lint not found on PATH or in $gopath_bin: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"
    make lint
}

cmd_fmt() { make fmt; }

cmd_vuln() {
    # The gate, not a bare scan. With no external dependencies the standard library is the whole
    # supply-chain surface, and the Go 1.22 line is out of support -- so "govulncheck reports
    # nothing" can never be true here and would be a permanently red gate. tools/vulncheck.sh
    # asks the answerable question instead: is there anything NEW?
    ./tools/vulncheck.sh "$@"
}

cmd_clean() {
    printf '%sThis removes bin/ and the compose stack (volumes kept). Continue? [y/N] %s' "$BOLD" "$OFF"
    read -r reply
    [[ "$reply" == [yY] ]] || die "aborted"
    make clean
    docker compose down
}

cmd_help() {
    cat <<'USAGE'
GoRetroTV control script

  ./ctl.sh <command>

Running
  run            Build and run natively (the fast loop -- the emulator gains nothing from a container)
  up             Build and start the compose stack, then wait for health
  down           Stop the compose stack
  restart        down, then up
  status         Compose state plus a health probe
  logs [n]       Follow the container log (default: last 100 lines)
  health         Probe the health endpoint and print what it says
  gate           Boot gate: build, verify firmware, listen, /health, graceful stop

Building and checking
  build          Build every binary into bin/
  test           Run the tests under the race detector
  lint           Hook check, go vet and golangci-lint (fails if the linter is absent)
  hooks          Assert the git hooks are armed and delegating to beads
  fmt            Format the tree
  vuln           Check against the Go vulnerability database
  clean          Remove bin/ and stop the stack (asks first)

  help           This text
USAGE
}

main() {
    local cmd="${1:-help}"
    shift || true
    case "$cmd" in
        build)   cmd_build "$@" ;;
        run)     cmd_run "$@" ;;
        up)      cmd_up "$@" ;;
        down)    cmd_down "$@" ;;
        restart) cmd_restart "$@" ;;
        status)  cmd_status "$@" ;;
        logs)    cmd_logs "$@" ;;
        health)  cmd_health "$@" ;;
        gate)    cmd_gate "$@" ;;
        test)    cmd_test "$@" ;;
        lint)    cmd_lint "$@" ;;
        hooks)   cmd_hooks "$@" ;;
        fmt)     cmd_fmt "$@" ;;
        vuln)    cmd_vuln "$@" ;;
        clean)   cmd_clean "$@" ;;
        help|-h|--help) cmd_help ;;
        *)       warn "unknown command: $cmd"; cmd_help; exit 1 ;;
    esac
}

main "$@"
