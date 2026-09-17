#!/usr/bin/env bash
# Fail on any called vulnerability in the project or Go standard library.
set -uo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

baseline="tools/govulncheck-baseline.txt"
write=0
report=""
while (( $# )); do
    case "$1" in
        --write-baseline) write=1 ;;
        --report) shift; report="${1:-}" ;;
        *) printf 'unknown argument: %s\n' "$1" >&2; exit 2 ;;
    esac
    shift
done

die() { printf '%s\n' "$*" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || die "vulncheck: jq is required"
[[ -f "$baseline" ]] || die "vulncheck: missing $baseline"

report_file=""
findings_file=""
cleanup() {
    [[ -n "$report_file" && -z "$report" ]] && rm -f "$report_file"
    [[ -n "$findings_file" ]] && rm -f "$findings_file"
}
trap cleanup EXIT
if [[ -n "$report" ]]; then
    [[ -f "$report" ]] || die "vulncheck: no such report file: $report"
    report_file="$report"
else
    scanner="$(command -v govulncheck 2>/dev/null || true)"
    if [[ -z "$scanner" ]]; then
        scanner="$(go env GOPATH)/bin/govulncheck"
    fi
    [[ -x "$scanner" ]] || die "vulncheck: govulncheck missing; install with GOTOOLCHAIN=go1.27.1 go install golang.org/x/vuln/cmd/govulncheck@v1.8.0"
    report_file="$(mktemp)"
    # JSON mode exits 0 even when findings exist; the parsed findings below decide the result.
    "$scanner" -format json ./... > "$report_file" || die "vulncheck: scanner failed"
fi

# Slurp and parse the whole report before counting findings. A config record followed by malformed
# JSON must fail as a broken scan. The SBOM must name examined roots under this exact toolchain;
# a config or progress message alone does not prove package analysis completed.
findings_file="$(mktemp)"
expected_go="$(go version | awk '{print $3}')"
if ! jq -s -e --arg go "$expected_go" 'if any(.[]; .SBOM.go_version == $go and (.SBOM.roots | length > 0))
    then [.[] | select(.finding) | .finding | select(.trace[0].function != null) | .osv] | unique
    else error("report has no analysis records") end' < "$report_file" > "$findings_file"; then
    die "vulncheck: malformed or incomplete govulncheck report"
fi
mapfile -t found < <(jq -r '.[]' < "$findings_file")

if (( write )); then
    (( ${#found[@]} == 0 )) || die "vulncheck: cannot write a nonempty baseline; fix the findings first"
    printf '# Called-vulnerability baseline: empty under Go 1.27.1.\n# Any called finding fails tools/vulncheck.sh.\n' > "$baseline"
    printf 'vulncheck: baseline confirmed empty\n'
    exit 0
fi

# Keeping the file as an explicit zero-entry baseline makes a future accidental acceptance edit
# visible in review. It is not a suppression mechanism after the toolchain upgrade.
if grep -Eq '^GO-[0-9]{4}-[0-9]+' "$baseline"; then
    die "vulncheck: $baseline must remain empty after the toolchain upgrade"
fi
if (( ${#found[@]} )); then
    printf 'vulncheck: %d called vulnerabilities found:\n' "${#found[@]}" >&2
    for id in "${found[@]}"; do
        printf '  %s  https://pkg.go.dev/vuln/%s\n' "$id" "$id" >&2
    done
    exit 1
fi
printf 'vulncheck: 0 called vulnerabilities\n'
