#!/usr/bin/env bash
# The vulnerability gate: does this tree call anything newly known to be vulnerable?
#
# WHY IT IS NOT SIMPLY "govulncheck reports nothing". This project has no external dependencies
# (docs/decisions/0001-scaffold.md), so the standard library is the whole supply-chain surface --
# and the toolchain is pinned at Go 1.22 because the host cannot fetch a newer one. Go 1.22 is out
# of support, and 26 of the 32 vulnerabilities our code currently reaches are fixed only in 1.23 or
# later. A plain `govulncheck` step would therefore be red on every run for ever, with nothing any
# committer could do about it, and a gate that is permanently red is one people stop reading. That
# is the same decoration failure as a gate that is permanently green, arrived at from the other
# side.
#
# So the question this asks is "anything NEW?". The accepted set lives in
# tools/govulncheck-baseline.txt with its reasoning, and anything outside it fails.
#
# A vulnerability in the baseline that is NO LONGER reported is reported here but does not fail the
# gate. The exact set varies with the 1.22.x patch level in use, so failing on it would turn a
# newer toolchain into a red build nobody could act on -- the same trap as vetting packages the
# committer did not touch.
set -uo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

BASELINE="tools/govulncheck-baseline.txt"
WRITE=0
REPORT=""

while (( $# )); do
    case "$1" in
        --write-baseline) WRITE=1 ;;
        # Takes a govulncheck JSON report from a file instead of running a scan. This exists so
        # the gate itself can be tested -- including watched going red -- without a 30-second
        # scan each time. It is an input override, not a bypass: the comparison below is
        # identical either way, and nothing skips it.
        --report) shift; REPORT="${1:-}" ;;
        *) printf 'unknown argument: %s\n' "$1" >&2; exit 2 ;;
    esac
    shift
done

say()  { printf '%s\n' "$*"; }
warn() { printf '%s\n' "$*" >&2; }
die()  { printf '%s\n' "$*" >&2; exit 1; }

command -v jq >/dev/null 2>&1 || die "vulncheck: jq is required to read govulncheck's JSON report"

resolve_govulncheck() {
    if command -v govulncheck >/dev/null 2>&1; then
        command -v govulncheck
        return 0
    fi
    # `go install` puts it in GOPATH/bin, which is not on PATH in a plain non-login shell. The
    # same trap that had ./ctl.sh lint reporting a pass while running twelve fewer linters.
    local gopath_bin
    gopath_bin="$(go env GOPATH)/bin"
    [[ -x "$gopath_bin/govulncheck" ]] && { printf '%s\n' "$gopath_bin/govulncheck"; return 0; }
    return 1
}

report_file=""
cleanup() { [[ -n "$report_file" && -z "$REPORT" ]] && rm -f "$report_file"; }
trap cleanup EXIT

if [[ -n "$REPORT" ]]; then
    [[ -f "$REPORT" ]] || die "vulncheck: no such report file: $REPORT"
    report_file="$REPORT"
else
    GOVULNCHECK="$(resolve_govulncheck)" \
        || die "vulncheck: govulncheck not found on PATH or in $(go env GOPATH)/bin
  install it with: go install golang.org/x/vuln/cmd/govulncheck@v1.1.4
A gate that cannot run its check must go red, never green."

    report_file="$(mktemp)"
    # NOTE: with -format json, govulncheck exits 0 EVEN WHEN IT FINDS VULNERABILITIES -- the
    # exit-3 convention applies to the text format only. So the verdict below comes from the
    # comparison and never from this status. Anything non-zero here means the scan itself failed,
    # which is a harness failure rather than a clean run.
    if ! GOTOOLCHAIN=local "$GOVULNCHECK" -format json ./... > "$report_file" 2>/dev/null; then
        die "vulncheck: govulncheck failed to run; this is a broken check, not a clean tree"
    fi
fi

# Only symbol-level findings: ones where our code has a real call path to the vulnerable function.
# A finding whose first trace frame has no function is imported-but-not-called and is not reachable.
mapfile -t found < <(jq -r 'select(.finding) | .finding | select(.trace[0].function != null) | .osv' \
                        < "$report_file" 2>/dev/null | sort -u)

# Assert the instrument examined something before trusting what it says. An empty report and a
# clean tree are the same output here, and only one of them is good news.
if ! jq -e 'select(.osv or .finding or .progress)' < "$report_file" >/dev/null 2>&1; then
    die "vulncheck: the report contains no govulncheck records at all -- the scan did not run, and
an empty report is not a clean tree."
fi

if (( WRITE )); then
    tmp="$(mktemp)"
    awk '/^GO-/{exit} {print}' "$BASELINE" > "$tmp"
    printf '%s\n' "${found[@]}" >> "$tmp"
    mv "$tmp" "$BASELINE"
    say "vulncheck: baseline rewritten with ${#found[@]} called vulnerabilities"
    exit 0
fi

[[ -f "$BASELINE" ]] || die "vulncheck: missing $BASELINE"
mapfile -t accepted < <(sed 's/#.*//' "$BASELINE" | tr -d '[:blank:]' | grep -E '^GO-[0-9]{4}-[0-9]+$' | sort -u)
(( ${#accepted[@]} )) || die "vulncheck: $BASELINE lists no vulnerability ids; a baseline that
accepts nothing and is never read is not a baseline."

new=()
for id in "${found[@]:-}"; do
    [[ -n "$id" ]] || continue
    printf '%s\n' "${accepted[@]}" | grep -qxF "$id" || new+=("$id")
done

gone=()
for id in "${accepted[@]}"; do
    printf '%s\n' "${found[@]:-}" | grep -qxF "$id" || gone+=("$id")
done

say "vulncheck: ${#found[@]} called, ${#accepted[@]} accepted, ${#new[@]} new"

if (( ${#gone[@]} )); then
    warn ""
    warn "No longer reported (prune them from $BASELINE when convenient):"
    printf '  %s\n' "${gone[@]}" >&2
    warn "Not a failure: the set varies with the 1.22.x patch level in use."
fi

if (( ${#new[@]} )); then
    warn ""
    warn "NEW vulnerabilities this tree calls, not in $BASELINE:"
    for id in "${new[@]}"; do
        warn "  $id  https://pkg.go.dev/vuln/$id"
    done
    warn ""
    die "vulncheck: FAILED. Judge each one: fix the call, or add it to the baseline with a reason."
fi

say "vulncheck: ok — nothing new"
