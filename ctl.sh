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

# Build the published runtime image without starting a service. This also gives the firmware-image
# conformance probe's Docker build an operator-visible diagnostic path when its captured output is
# too short to show the compiler failure.
cmd_image() {
    export_build_args
    docker build --progress=plain --target runtime \
        --build-arg VERSION --build-arg COMMIT --build-arg BUILD_DATE \
        -t "$PROJECT:local" .
}

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

public_compose() {
    docker compose -f docker-compose.yml -f docker-compose.traefik.yml "$@"
}

public_env() {
    # The private snapshot is mode 0600 in a 0700 directory. Run as its non-root owner inside
    # the container rather than making either path world-readable for the distroless UID.
    GORETROTV_RUNTIME_UID="$(id -u)"
    GORETROTV_RUNTIME_GID="$(id -g)"
    [[ "$GORETROTV_RUNTIME_UID" != 0 ]] || die "public container must run as a non-root snapshot owner"
    GORETROTV_ENV=production
    export GORETROTV_RUNTIME_UID GORETROTV_RUNTIME_GID GORETROTV_ENV
}

cmd_public_config() {
    public_env
    public_compose config "$@"
}

cmd_up_public() {
    require_firmware
    [[ -f snapshots/post-acquisition.snapshot ]] \
        || die "missing private post-acquisition snapshot; create it with ./ctl.sh snapshot seed"
    [[ -r snapshots/post-acquisition.snapshot ]] \
        || die "post-acquisition snapshot is not readable by the current user"
    public_env
    export_build_args
    public_compose up -d --build
    cmd_public_health
}

cmd_down_public() {
    public_env
    public_compose down
}

cmd_restart_public() {
    public_env
    public_compose restart goretrotv
    cmd_public_health
}

cmd_public_health() {
    local domain="${GORETROTV_DOMAIN:-goretrotv.demosrv.uk}"
    local url="https://${domain}/health"
    for ((i = 1; i <= 90; i++)); do
        if curl -fsS --max-time 3 "$url" >/dev/null 2>&1; then
            ok "healthy through Traefik: $url"
            curl -fsS "$url"
            say ""
            return 0
        fi
        sleep 1
    done
    die "public route unhealthy: $url did not answer within 90s"
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

cmd_cpu_gate() { ./tools/cpu-gate.sh "$@"; }

cmd_handoff_gate() { ./tools/handoff-gate.sh "$@"; }

cmd_oracle_gate() { ./tools/oracle-cold-boot-gate.sh "$@"; }

cmd_links_gate() { ./tools/links-gate.sh "$@"; }

cmd_snapshot_gate() { ./tools/snapshot-runon-gate.sh "$@"; }
cmd_replay_gate() { ./tools/input-replay-gate.sh "$@"; }

cmd_snapshot() { ./tools/snapshot-library.sh "$@"; }

cmd_test() { make test-race; }

# THE PER-TURN GATE. `cmd_test` is a ~25 minute race-detector run because the firmware tests
# restore real boxes and retire millions of guest instructions each; that is a pre-push and CI
# concern, and running it after every edit timed the quality hook out every single time -- four
# times in one session, each one green, each one paid for by hand.
#
# -short is what makes this fast, and the gate for it lives in the two `restoredBox` fixtures
# (internal/broadcast and internal/multiplex/firmwaretests) rather than here, so a firmware test
# added later is covered without anyone remembering to add it to a list. Everything else still
# runs, under the race detector: ~45s against the full suite's ~25 minutes.
#
# It is NOT a substitute for `test`. A change to anything the firmware touches is unproven until
# the full suite has run.
cmd_test_fast() { go test -short -race ./...; }

cmd_conformance() { python3 conformance/run.py "$@"; }

cmd_conformance_stop_gate() { python3 conformance/stop-gate.py "$@"; }

cmd_conformance_ablate() { python3 conformance/ablate.py "$@"; }

cmd_authorise_rules() { scripts/conformance/authorise-rules.sh "$@"; }

cmd_rule_guard_selftest() { scripts/conformance/rule-guard-selftest.sh "$@"; }

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

# AGENTS.md must stay a symlink to CLAUDE.md, so Codex CLI and Claude Code read the same
# instructions. This is asserted rather than trusted because the failure is silent: `bd setup codex`
# and similar generators WRITE to AGENTS.md, and replacing the symlink with a real file resumes the
# drift with no error. That is not hypothetical -- AGENTS.md previously held three copies of the
# beads block and none of this project's actual instructions, so an agent reading it learned nothing
# about the oracle, the firmware, or why changing what a read returns is dangerous.
cmd_agentsdoc() {
    if [[ ! -L AGENTS.md ]]; then
        die "AGENTS.md is not a symlink. Codex CLI reads it and Claude Code reads CLAUDE.md, so they
have drifted apart -- and a generator that rewrote it would have said nothing.
  fix: rm -f AGENTS.md && ln -s CLAUDE.md AGENTS.md
  then re-apply anything the generator meant to add to CLAUDE.md instead."
    fi
    local target
    target="$(readlink AGENTS.md)"
    [[ "$target" == "CLAUDE.md" ]] \
        || die "AGENTS.md points at '$target', not CLAUDE.md. fix: rm -f AGENTS.md && ln -s CLAUDE.md AGENTS.md"
    [[ -f CLAUDE.md ]] || die "AGENTS.md is a symlink to CLAUDE.md, which does not exist"
    ok "AGENTS.md -> CLAUDE.md: Codex and Claude read the same instructions"
}

cmd_docs_serve() {
    # A viewer for the docs tree, so they can be read in a browser with working search and
    # navigation rather than as raw markdown. It rebuilds the page when the docs have moved, so
    # editing a file and refreshing is the whole loop -- no watcher, no restart.
    #
    # LOOPBACK IS THE DEFAULT AND --lan IS A DELIBERATE EXCEPTION TO ARCH-DEV-1. The recorded
    # invariant is "developer surfaces bind only to loopback" (plan/module-decisions.md ->
    # Deployment and access), and --lan publishes this one to the local network so it can be read
    # from another machine. That is a decision, not a convenience: it is opt-in, it is never
    # implied by anything else, and what it exposes is this repository's documentation -- which is
    # not secret, but does describe the box in detail.
    #
    # It binds the machine's own LAN address, not 0.0.0.0. A wildcard bind would also publish the
    # viewer on every container bridge and VPN interface this host happens to have, which on a
    # docker host is dozens of networks nobody meant to serve it on.
    local port=8098 lan=""
    while (( $# )); do
        case "$1" in
            --lan) lan="--lan" ;;
            *)  [[ "$1" =~ ^[0-9]+$ ]] || die "unexpected argument '$1'  (usage: ./ctl.sh docs-serve [port] [--lan])"
                port="$1" ;;
        esac
        shift
    done
    command -v python3 >/dev/null 2>&1 \
        || die "python3 not found, and the docs viewer needs it (scripts/docs-site.py)"
    # shellcheck disable=SC2086
    exec python3 scripts/docs-site.py serve --docs docs --title "GoRetroTV" --port "$port" $lan
}

cmd_docs() {
    # Every documentation section names the code it describes, and carries a fingerprint taken the
    # day somebody verified it against that code. This is the check that turns those notes into
    # something that can FAIL. Without it the failure is silent and one-sided: the code moves, the
    # section goes on describing what used to be there, and nothing anywhere disagrees. That has
    # become a correctness problem rather than a tidy-desk one -- most of what reads these files now
    # is an agent, which reproduces a stale claim as code with full confidence and flags nothing.
    #
    # --strict also fails on a section that makes a claim about code and names none. That is the
    # vacuity guard, and it is the half that matters over time: without it a doc tree drifts back to
    # unanchored one new section at a time while the gate keeps reporting green.
    command -v python3 >/dev/null 2>&1 \
        || die "python3 not found, and the docs gate needs it (scripts/docs-anchors.py)"

    python3 scripts/docs-anchors.py --repo . --docs docs check --strict || die "the docs and the code they describe have diverged.
  Re-read each section listed above against its anchor, correct whatever is no longer true, then:
    python3 scripts/docs-anchors.py --repo . --docs docs stamp --section '<heading>'
  A section that makes no claim about code says so instead, with a reason:
    <!-- anchor: none - measured firmware evidence, not a claim about this repo's code -->
  Stamping without re-reading is the one move that breaks this: a fingerprint nobody earned reads
  as verified for ever, and takes the section out of the queue permanently."

    # site.html is generated and gitignored, so a clone legitimately has none. Check it only when it
    # is there -- and then insist it matches, because a docs site built from older docs is exactly
    # the confident-looking wrong answer this whole mechanism exists to prevent.
    if [[ -f docs/site.html ]]; then
        python3 scripts/docs-site.py check --docs docs >/dev/null \
            || die "docs/site.html was generated from different docs than the tree now holds.
  fix: python3 scripts/docs-site.py build --docs docs --title 'GoRetroTV'"
    fi

    ok "docs anchored to the code they describe, and every section accounted for"
}

cmd_lint() {
    # Folded in so the hook assertion runs wherever checks run, rather than depending on somebody
    # remembering a verb that only matters when it fails.
    cmd_hooks
    cmd_agentsdoc
    cmd_docs
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
    # Run the checked vulnerability gate against the current Go toolchain.
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
  up-public      Build and start the real HTTPS Traefik route with private data mounted read-only
  config-public  Print the effective public compose configuration
  down           Stop the compose stack
  down-public    Stop the public compose stack
  restart-public Restart the published box from its private snapshot, then wait for HTTPS health
  restart        down, then up
  status         Compose state plus a health probe
  logs [n]       Follow the container log (default: last 100 lines)
  health         Probe the health endpoint and print what it says
  health-public  Probe the real HTTPS URL through Traefik
  gate           Boot gate: build, verify firmware, listen, /health, graceful stop
  cpu-gate       Real firmware CPU/oracle gate through the first unmodelled video RAM read
  handoff-gate   Prove declared handoff, guest loader and application entry
  oracle-gate    Full 42-task cold boot and 470,000 matching browser checkpoints
  links-gate     Guest handset, card, NVRAM and acknowledgement-policy checks
  snapshot-gate  Compare a restored real-firmware run with 10 million uninterrupted instructions
  replay-gate    Record a Sky key and verify two real-firmware framebuffer replays
  snapshot       Save, run, inspect, list or seed named local machine snapshots

Building and checking
  build          Build every binary into bin/
  image          Build the firmware-free runtime container image
  test           Run the tests under the race detector (~25 min: restores real boxes)
  test:fast      The same suite with -short, so the firmware boxes skip (~45s) -- the per-turn gate
  conformance    Run architecture rules and their probes
  stop-gate      Neutralise each conformance detector and check probe independence
  ablate         Remove each rule subject in turn and check its coverage guard
  authorise-rules <reason>  Record why staged architecture controls changed
  rule-guard-selftest    Prove the commit guard catches governed edits
  lint           Hook, AGENTS.md, docs-anchor and go vet checks, then golangci-lint
  hooks          Assert the git hooks are armed and delegating to beads
  agents         Assert AGENTS.md still symlinks to CLAUDE.md (Codex and Claude read one file)
  docs           Assert every doc section is anchored to the code it describes, and current
  docs-serve [port] [--lan]  Read the docs in a browser (default 8098, loopback; --lan also
                 serves this machine's LAN address so other machines can reach it)
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
        image)   cmd_image "$@" ;;
        run)     cmd_run "$@" ;;
        up)      cmd_up "$@" ;;
        up-public) cmd_up_public "$@" ;;
        config-public) cmd_public_config "$@" ;;
        down)    cmd_down "$@" ;;
        down-public) cmd_down_public "$@" ;;
        restart-public) cmd_restart_public "$@" ;;
        restart) cmd_restart "$@" ;;
        status)  cmd_status "$@" ;;
        logs)    cmd_logs "$@" ;;
        health)  cmd_health "$@" ;;
        health-public) cmd_public_health "$@" ;;
        gate)    cmd_gate "$@" ;;
        cpu-gate) cmd_cpu_gate "$@" ;;
        handoff-gate) cmd_handoff_gate "$@" ;;
        oracle-gate) cmd_oracle_gate "$@" ;;
        links-gate) cmd_links_gate "$@" ;;
        snapshot-gate) cmd_snapshot_gate "$@" ;;
        replay-gate) cmd_replay_gate "$@" ;;
        snapshot) cmd_snapshot "$@" ;;
        test)    cmd_test "$@" ;;
        test:fast) cmd_test_fast "$@" ;;
        conformance) cmd_conformance "$@" ;;
        stop-gate) cmd_conformance_stop_gate "$@" ;;
        ablate) cmd_conformance_ablate "$@" ;;
        authorise-rules) cmd_authorise_rules "$@" ;;
        rule-guard-selftest) cmd_rule_guard_selftest "$@" ;;
        lint)    cmd_lint "$@" ;;
        hooks)   cmd_hooks "$@" ;;
        agents)  cmd_agentsdoc "$@" ;;
        docs)    cmd_docs "$@" ;;
        docs-serve) cmd_docs_serve "$@" ;;
        fmt)     cmd_fmt "$@" ;;
        vuln)    cmd_vuln "$@" ;;
        clean)   cmd_clean "$@" ;;
        help|-h|--help) cmd_help ;;
        *)       warn "unknown command: $cmd"; cmd_help; exit 1 ;;
    esac
}

main "$@"
