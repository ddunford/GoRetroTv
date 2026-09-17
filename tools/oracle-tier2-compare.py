#!/usr/bin/env python3
"""Find the exact first instruction-state difference in a real firmware trace window."""

import json
import re
import sys


def fail(message):
    raise SystemExit(f"tier 2 harness failure: {message}")


if len(sys.argv) != 5:
    fail("usage: oracle-tier2-compare.py oracle.jsonl go-cp1.stream go.trace expected-icount")

oracle_path, go_hash_path, go_trace_path, expected_text = sys.argv[1:]
expected = int(expected_text)
go_hash = {}
with open(go_hash_path, encoding="ascii") as stream:
    for line in stream:
        fields = line.split()
        if len(fields) == 2 and fields[0].isdigit():
            go_hash[int(fields[0])] = int(fields[1], 16)

go_state = {}
pattern = re.compile(r"^(\d+) PC=([0-9A-F]+) ISA=(\w+) Count=([0-9A-F]+) "
                     r"Status=([0-9A-F]+) GPR=")
with open(go_trace_path, encoding="ascii") as stream:
    for line in stream:
        match = pattern.match(line)
        if match:
            at, pc, isa, count, status = match.groups()
            go_state[int(at)] = (int(pc, 16), isa == "true", int(count, 16), int(status, 16))

compared = 0
first = None
with open(oracle_path, encoding="utf-8") as stream:
    for line in stream:
        row = json.loads(line)
        at = row["at"]
        if at not in go_hash or at not in go_state:
            continue  # MIPS32 branch/delay-slot pairs omit a browser loop boundary.
        compared += 1
        if row["hash"] != go_hash[at]:
            first = (row, go_hash[at], go_state[at])
            break

if compared < 2 or first is None:
    fail(f"only {compared} comparable states or no divergent state")
oracle, go_digest, (go_pc, go_isa, go_count, go_status) = first
at = oracle["at"]
if at != expected:
    fail(f"first difference at {at}, expected injected instruction {expected}")
if oracle["pc"] != go_pc or bool(oracle["isa"]) != go_isa or \
   oracle["cp0"][9] != go_count or oracle["cp0"][12] ^ go_status != 0x10000000:
    fail("states differ beyond the deliberately injected Status bit")

print(f"TIER2 DIVERGE at retired instruction {at}; {compared-1} prior browser states agree")
print(f"  oracle PC={oracle['pc']:08X} ISA={oracle['isa']} Count={oracle['cp0'][9]:08X} "
      f"Status={oracle['cp0'][12]:08X} HASH={oracle['hash']:08X}")
print(f"  Go     PC={go_pc:08X} ISA={int(go_isa)} Count={go_count:08X} "
      f"Status={go_status:08X} HASH={go_digest:08X}")
