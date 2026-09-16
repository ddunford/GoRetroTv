#!/usr/bin/env bash
# THE BOOT GATE: does this build actually come up, on the real firmware, after whatever just changed?
#
# A hardware model does not fail with a stack trace. It fails by running for ever doing something
# plausible (CLAUDE.md -> Non-Obvious Domain Patterns), and the predecessor lost a day twice to
# exactly that. Unit tests cannot see it because each one is right about its own piece. Only
# starting the thing can.
#
# WHAT IT ASSERTS TODAY. There is no CPU until phase 2, so this is deliberately the skeleton the
# phase asks for rather than a machine check pretending to be one. Each stage is a layer that can
# independently break:
#   1. the binary builds
#   2. it verifies the firmware against the manifest, and says so
#   3. it reaches the listener
#   4. /health answers, and NAMES THIS BUILD -- the commit it reports must match this working
#      tree's HEAD. A gate that passes against a binary nobody can identify has proved nothing
#      about the tree it was supposed to be checking, and "nobody can identify" includes the
#      linker defaults, which are non-empty and mean exactly nothing
#   5. it shuts down gracefully on a signal rather than being killed
#
# The stages are named and ordered so phase 2 adds "and the machine reached N tasks" as another
# stage rather than rewriting this. docs/reference/oracle-boot-gate.md records what the full gate
# must eventually assert, and the measured numbers to assert it against.
#
# IF IT CANNOT RUN, IT GOES RED. The firmware is not redistributable and is not in git, so this
# gate cannot run in CI and does not run there. Absent firmware is reported as a refusal rather
# than skipped: a gate that cannot perform its check must never wear the colour of a pass, because
# an unknown result that looks like a pass is worse than a red one -- nobody re-runs it.
set -uo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

BINARY=""
FIRMWARE_DIR="${GORETROTV_FIRMWARE_DIR:-./firmware}"
TIMEOUT_S=20
KEEP=0

while (( $# )); do
    case "$1" in
        # Run an already-built binary instead of building one. This is how the gate is pointed at
        # a deliberately broken build to watch it go red, and how CI or a release job would check
        # an artefact it did not build itself. It is an input override, not a bypass: every stage
        # below runs identically either way.
        --binary)   shift; BINARY="${1:-}" ;;
        --firmware) shift; FIRMWARE_DIR="${1:-}" ;;
        --timeout)  shift; TIMEOUT_S="${1:-20}" ;;
        --keep-logs) KEEP=1 ;;
        *) printf 'unknown argument: %s\n' "$1" >&2; exit 2 ;;
    esac
    shift
done

if [[ -t 1 ]]; then
    RED=$'\033[31m'; GREEN=$'\033[32m'; OFF=$'\033[0m'
else
    RED=''; GREEN=''; OFF=''
fi

failures=0
stage_ok()   { printf '%sok  %s%s  %s\n'   "$GREEN" "$1" "$OFF" "${2:-}"; }
stage_fail() { printf '%sFAIL %s%s  %s\n'  "$RED"   "$1" "$OFF" "${2:-}" >&2; failures=$((failures + 1)); }
die()        { printf '%s%s%s\n' "$RED" "$*" "$OFF" >&2; exit 1; }

work="$(mktemp -d)"
log="$work/goretrotv.log"
pid=""
cleanup() {
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
        kill -KILL "$pid" 2>/dev/null
        wait "$pid" 2>/dev/null
    fi
    if (( KEEP )); then
        printf 'log kept: %s\n' "$log" >&2
    else
        rm -rf "$work"
    fi
}
trap cleanup EXIT

# ---- preconditions: a gate that cannot run must say so, not pass -------------------------------
[[ -d "$FIRMWARE_DIR" ]] || die "boot gate cannot run: no firmware directory at $FIRMWARE_DIR
The flash images are Pace's, are not redistributable and are not in git (firmware/MANIFEST.md).
A gate that cannot perform its check is a refusal, never a pass."
for f in MANIFEST.md FLASH_U202.bin FLASH_U203.bin application-ram-image.bin; do
    [[ -f "$FIRMWARE_DIR/$f" ]] || die "boot gate cannot run: $FIRMWARE_DIR/$f is missing"
done

# ---- a port nobody else is using ---------------------------------------------------------------
# Derived from this process rather than fixed: the tree is worked by several agents at once, and a
# fixed port is a shared name. A container vanishing or a port being stolen mid-run costs somebody
# else a false diagnosis, and a wrong answer is worse than a crash because it looks like an answer.
port=""
for try in 0 1 2 3 4 5 6 7 8 9; do
    candidate=$(( 20000 + ((($$ + try * 997) % 20000) ) ))
    if ! (exec 3<>"/dev/tcp/127.0.0.1/$candidate") 2>/dev/null; then
        port="$candidate"
        break
    fi
done
[[ -n "$port" ]] || die "boot gate cannot run: found no free loopback port after ten tries"

# ---- 1. the binary builds ----------------------------------------------------------------------
if [[ -n "$BINARY" ]]; then
    [[ -x "$BINARY" ]] || die "boot gate cannot run: $BINARY is not an executable"
    stage_ok "binary" "supplied: $BINARY"
else
    # Through the Makefile's own target, not a re-implementation of it. A bare `go build` here
    # omitted -ldflags, so the binary under test could only ever report the linker defaults -
    # which made the identity stage below pass against exactly the binary it exists to reject.
    # One definition of the build identity, in one place.
    if make -s build BIN_DIR="$work" > "$work/build.txt" 2>&1; then
        BINARY="$work/goretrotv"
        stage_ok "binary builds" "$(du -h "$BINARY" | cut -f1), stamped by the Makefile's ldflags"
    else
        stage_fail "binary builds" "go build failed:"
        sed 's/^/    /' "$work/build.txt" >&2
        die "boot gate: FAILED at the first stage; nothing below could be attempted."
    fi
fi

# ---- start it ----------------------------------------------------------------------------------
GORETROTV_ENV=development \
GORETROTV_FIRMWARE_DIR="$FIRMWARE_DIR" \
GORETROTV_HTTP_ADDR="127.0.0.1:$port" \
GORETROTV_LOG_FORMAT=json \
    "$BINARY" > "$log" 2>&1 &
pid=$!

# Wait for the listener, or for the process to die. Both are answers; hanging is not.
listening=0
for (( i = 0; i < TIMEOUT_S * 10; i++ )); do
    if grep -q '"msg":"listening"' "$log" 2>/dev/null; then listening=1; break; fi
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.1
done

# ---- 2. the firmware is verified ---------------------------------------------------------------
if grep -q '"msg":"firmware verified"' "$log" 2>/dev/null; then
    images=$(sed -n 's/.*"images":\([0-9]*\).*/\1/p' "$log" | head -1)
    stage_ok "firmware verified" "${images:-?} images against $FIRMWARE_DIR/MANIFEST.md"
else
    stage_fail "firmware verified" "the build never reported verifying the firmware"
fi

# ---- 3. it reaches the listener ----------------------------------------------------------------
if (( listening )); then
    stage_ok "reaches the listener" "127.0.0.1:$port"
else
    if kill -0 "$pid" 2>/dev/null; then
        stage_fail "reaches the listener" "still running after ${TIMEOUT_S}s and never listened"
    else
        wait "$pid" 2>/dev/null; rc=$?
        stage_fail "reaches the listener" "the process exited ($rc) before listening"
    fi
    # The reason lives in the log, and a gate that withholds it makes the reader go and find it.
    if [[ -s "$log" ]]; then
        printf '  the build said:\n' >&2
        sed 's/^/    /' "$log" | tail -6 >&2
    fi
fi

# ---- 4. /health answers, and names the build ---------------------------------------------------
if (( listening )); then
    body="$work/health.json"
    if curl -fsS --max-time 5 "http://127.0.0.1:$port/health" > "$body" 2>"$work/curl.txt"; then
        status=$(sed -n 's/.*"status":"\([^"]*\)".*/\1/p' "$body")
        version=$(sed -n 's/.*"version":"\([^"]*\)".*/\1/p' "$body")
        commit=$(sed -n 's/.*"commit":"\([^"]*\)".*/\1/p' "$body")
        if [[ "$status" == "ok" ]]; then
            stage_ok "health answers ok" "$(cat "$body")"
        else
            stage_fail "health answers ok" "status is '${status:-absent}': $(cat "$body")"
        fi
        # A gate that passes against a binary nobody can identify has proved nothing about the
        # tree it was checking -- so the claim has to be IDENTITY, not merely non-emptiness.
        #
        # The previous version asserted both fields were non-empty. `dev` and `unknown` are the
        # linker defaults in internal/version: they are precisely what a binary reports when
        # nobody told it what it is, and they are non-empty, so the check passed against the one
        # binary it existed to reject, on every run. Its negative control fired only on an EMPTY
        # version, which the real build path cannot produce -- a control proving the assertion
        # against a fault the system cannot exhibit.
        #
        # Comparing the reported commit to this working tree's HEAD is a claim that can actually
        # be false, and it fails on the real path the moment the stamping stops happening. It also
        # answers the question that matters when a divergence turns up later: was that binary
        # built from this tree?
        expected_commit="$(git rev-parse --short HEAD 2>/dev/null || true)"
        if [[ -z "$expected_commit" ]]; then
            stage_fail "health names this build" "cannot determine the expected commit (not a git tree?) — the identity claim cannot be checked, so it is not granted"
        elif [[ "$commit" != "$expected_commit" ]]; then
            stage_fail "health names this build" "reports commit='${commit}', this tree is ${expected_commit}$(
                [[ "$commit" == "unknown" || "$version" == "dev" ]] && printf '%s' " — those are the linker defaults, so the build was not stamped")"
        elif [[ "$version" == "dev" || -z "$version" ]]; then
            stage_fail "health names this build" "commit matches but version='${version}' is the linker default — half-stamped"
        else
            stage_ok "health names this build" "version=$version commit=$commit (matches HEAD)"
        fi
    else
        stage_fail "health answers ok" "no answer from http://127.0.0.1:$port/health: $(tr -d '\n' < "$work/curl.txt")"
        stage_fail "health names this build" "not reached"
    fi
else
    stage_fail "health answers ok" "not reached — it never listened"
    stage_fail "health names this build" "not reached"
fi

# ---- 5. it shuts down gracefully ---------------------------------------------------------------
if kill -0 "$pid" 2>/dev/null; then
    kill -TERM "$pid" 2>/dev/null
    stopped=0
    for (( i = 0; i < 100; i++ )); do
        kill -0 "$pid" 2>/dev/null || { stopped=1; break; }
        sleep 0.1
    done
    if (( stopped )); then
        wait "$pid" 2>/dev/null; rc=$?
        if grep -q '"msg":"stopped"' "$log" 2>/dev/null && (( rc == 0 )); then
            stage_ok "shuts down gracefully" "exit $rc on SIGTERM"
        else
            stage_fail "shuts down gracefully" "exit $rc, and the build did not report stopping"
        fi
    else
        stage_fail "shuts down gracefully" "still alive 10s after SIGTERM"
    fi
    pid=""
elif (( listening )); then
    stage_fail "shuts down gracefully" "the process had already gone before it was signalled"
else
    stage_fail "shuts down gracefully" "not reached"
fi

printf '\n'
if (( failures )); then
    die "boot gate: FAILED — $failures of 5 stages. Each FAIL line above says which claim did not hold."
fi
printf '%sboot gate: ok%s\n' "$GREEN" "$OFF"
