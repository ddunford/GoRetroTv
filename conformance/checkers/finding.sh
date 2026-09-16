# How a shell checker says WHAT it found. The shell half of conformance/checkers/finding.py --
# read that file for the argument; this is the same vocabulary for the two checkers that are bash.
#
# Sourced, not executed:   . "$(dirname "$0")/finding.sh"
#
#   violation RULE DETECTOR MESSAGE...   one rule violation, attributed to the detector that fired
#   harness   RULE MESSAGE...            the checker could not do its job; carries NO detector
#
# Exit codes, and the runner reads them: 0 the rule holds, 1 the rule is VIOLATED (the ONLY code
# that means that), EXIT_HARNESS for "could not run".

EXIT_HARNESS=2

violation() {
  local rule="$1" detector="$2"
  shift 2
  case "$detector" in
    *[!a-z0-9-]*|-*|*-|*--*|"")
      # Loud rather than quiet. A malformed id emits a line the harness cannot attribute, which is
      # indistinguishable from the checker having said nothing -- so it dies here, at the call
      # site, instead of turning every probe for this rule red for a reason nobody can see.
      echo "$rule: BUG — '$detector' is not a lowercase-kebab detector id." >&2
      exit "$EXIT_HARNESS"
      ;;
  esac
  printf '%s: FAIL [%s] — %s\n' "$rule" "$detector" "$*"
}

harness() {
  local rule="$1"
  shift
  printf '%s: FAIL — HARNESS: %s\n' "$rule" "$*"
}
