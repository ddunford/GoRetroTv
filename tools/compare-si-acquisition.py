#!/usr/bin/env python3
"""Compare browser-oracle SI samples with a Go firmwaretrace run from the same guest state.

The Go trace may restore a snapshot at the first sample. Browser counters are therefore
normalised to the first sample before comparison; device registers and ring pointers are not.
"""

import argparse
import json
import re
from pathlib import Path


def require(condition, message):
    if not condition:
        raise SystemExit(f"SI comparison failed: {message}")


def parse_go(path):
    samples, sample_hits, final_hits, filters, matches, demod = [], {}, {}, {}, [], {}
    state = None
    for line in Path(path).read_text().splitlines():
        if line.startswith("section-sample at="):
            pattern = (r"section-sample at=(\d+) enable=([0-9A-F]+) status=([0-9A-F]+) "
                       r"armed-pids=\[([\d ]*)\] last22=([0-9A-F]+) last23=([0-9A-F]+) "
                       r"last24=([0-9A-F]+) demod-reads=(\d+)")
            match = re.fullmatch(pattern, line)
            require(match, f"malformed Go sample: {line}")
            at, enable, status, pids, last22, last23, last24, reads = match.groups()
            samples.append(dict(at=int(at), enable=int(enable, 16), status=int(status, 16),
                                pids=[int(pid) for pid in pids.split()],
                                last={22: int(last22, 16), 23: int(last23, 16),
                                      24: int(last24, 16)}, reads=int(reads)))
        elif line.startswith("section-sample-hit "):
            match = re.fullmatch(r"section-sample-hit at=(\d+) pc=([0-9A-F]+) total=(\d+)", line)
            require(match, f"malformed Go sample hit: {line}")
            at, pc, total = match.groups()
            sample_hits.setdefault(int(at), {})[f"0x{pc}"] = int(total)
        elif line.startswith("section-state "):
            match = re.fullmatch(r"section-state enable=([0-9A-F]+) status=([0-9A-F]+) armed-pids=\[([\d ]*)\]", line)
            require(match, f"malformed Go final state: {line}")
            state = dict(enable=int(match[1], 16), status=int(match[2], 16),
                         pids=[int(pid) for pid in match[3].split()])
        elif line.startswith("section-filter "):
            match = re.fullmatch(r"section-filter channel=(\d+) ring=([0-9A-F]+) "
                                 r"record-start=([0-9A-F]+) record-end=([0-9A-F]+) "
                                 r"record-current=([0-9A-F]+) record-last=([0-9A-F]+) "
                                 r"context=([0-9A-F]+)", line)
            require(match, f"malformed Go filter: {line}")
            filters[int(match[1])] = dict(start=int(match[3], 16), end=int(match[4], 16),
                                          current=int(match[5], 16), last=int(match[6], 16),
                                          context=int(match[7], 16))
        elif line.startswith("section-match "):
            match = re.fullmatch(r"section-match unit=(\d+) table=([0-9A-F]+)/([0-9A-F]+) "
                                 r"extension=([0-9A-F]+)/([0-9A-F]+)", line)
            require(match, f"malformed Go match: {line}")
            matches.append(dict(unit=int(match[1]), table=int(match[2], 16),
                                mask=int(match[3], 16), ext=int(match[4], 16),
                                ext_mask=int(match[5], 16)))
        elif line.startswith("demod-read register="):
            match = re.fullmatch(r"demod-read register=(\d+) count=(\d+) value=([0-9A-F]+)", line)
            require(match, f"malformed Go demod count: {line}")
            demod[match[1]] = int(match[2])
        elif line.startswith("pc-hit "):
            match = re.fullmatch(r"pc-hit ([0-9A-F]+) total=(\d+)(?: before-key=\d+ after-key=\d+)?", line)
            require(match, f"malformed Go PC hit: {line}")
            final_hits[f"0x{match[1]}"] = int(match[2])
    require(samples and state is not None and filters and final_hits,
            "Go trace did not assert samples, final device state, filter records and guest PCs")
    return dict(samples=samples, sample_hits=sample_hits, state=state, filters=filters,
                matches=matches, demod=demod, hits=final_hits)


def oracle_pids(snapshot):
    return [int(entry["pid"], 16) for entry in snapshot["demux"]["pids"]]


def read_demod_baseline(path):
    counts = {}
    for line in path.read_text().splitlines():
        match = re.fullmatch(r"demod-read register=(\d+) count=(\d+) value=[0-9A-F]+", line)
        if match:
            counts[match[1]] = int(match[2])
    require(counts, f"no demod histogram in baseline {path}")
    return counts


def compare(oracle, go, acquired, go_demod_baseline=None):
    samples = oracle.get("samples")
    require(samples and len(samples) == len(go["samples"]), "sample count differs or is zero")
    baseline = samples[0]
    base_reads = sum(baseline["demodReads"].values())
    go_base_hits = go["sample_hits"].get(go["samples"][0]["at"], {})
    go_demod_baseline = go_demod_baseline or {}
    go_base_reads = sum(go_demod_baseline.values())
    require(go["samples"][0]["reads"] == go_base_reads,
            "Go first sample does not match supplied demod baseline")
    for expected, actual in zip(samples, go["samples"]):
        at = expected["at"]
        require(at == actual["at"], f"sample instruction differs: oracle {at}, Go {actual['at']}")
        require(int(expected["demux"]["enable"][2], 16) == actual["enable"], f"enable at {at}")
        require(int(expected["demux"]["status"][2], 16) == actual["status"], f"status at {at}")
        require(oracle_pids(expected) == actual["pids"], f"armed PIDs at {at}")
        for filter_id in (22, 23, 24):
            require(expected["records"][str(filter_id)]["last"] == actual["last"][filter_id],
                    f"filter {filter_id} last-read pointer at {at}")
        require(sum(expected["demodReads"].values()) - base_reads == actual["reads"] - go_base_reads,
                f"demod read total at {at}")
        for pc, total in go["sample_hits"].get(at, {}).items():
            require(pc in expected["hits"] and pc in baseline["hits"], f"oracle lacks PC {pc}")
            require(expected["hits"][pc] - baseline["hits"][pc] ==
                    total - go_base_hits.get(pc, 0),
                    f"guest PC {pc} at {at}")
    final = oracle["after"]
    require(int(final["demux"]["enable"][2], 16) == go["state"]["enable"], "final enable")
    require(int(final["demux"]["status"][2], 16) == go["state"]["status"], "final status")
    require(oracle_pids(final) == go["state"]["pids"], "final armed PIDs")
    for filter_id, record in go["filters"].items():
        if str(filter_id) not in final["records"]:
            require(not acquired and filter_id == 21,
                    f"oracle lacks filter {filter_id} record")
            continue
        require(final["records"][str(filter_id)] == record, f"filter {filter_id} record")
    oracle_matches = {match["filter"]: match for match in final["matches"]}
    require(len(oracle_matches) == len(go["matches"]), "match-unit count")
    for match in go["matches"]:
        subject = oracle_matches.get(match["unit"])
        require(subject is not None, f"oracle lacks match unit {match['unit']}")
        require((subject["tableId"], subject["tableIdMask"], subject["extension"] or 0,
                 subject["extensionMask"] or 0) ==
                (match["table"], match["mask"], match["ext"], match["ext_mask"]),
                f"match unit {match['unit']}")
    for pc, total in go["hits"].items():
        require(pc in final["hits"] and pc in baseline["hits"], f"oracle lacks PC {pc}")
        require(final["hits"][pc] - baseline["hits"][pc] ==
                total - go_base_hits.get(pc, 0), f"final guest PC {pc}")
    all_registers = set(baseline["demodReads"]) | set(final["demodReads"]) | set(go["demod"])
    for register in all_registers:
        difference = final["demodReads"].get(register, 0) - baseline["demodReads"].get(register, 0)
        require(difference == go["demod"].get(register, 0) - go_demod_baseline.get(register, 0),
                f"demod register {register}")
    if acquired:
        require(82 in go["state"]["pids"], "guest never armed PID 0x52")
        require({0x40, 0x42, 0x4A, 0x73} <= {match["table"] for match in go["matches"]},
                "guest did not program NIT, SDT, BAT and TOT match units")
        require(go["hits"].get("0x800A6414", 0) -
                go_base_hits.get("0x800A6414", 0) > 0,
                "service-list guest dispatcher never ran")
    return len(samples), len(go["hits"]), len(go["matches"])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--oracle", type=Path, required=True)
    parser.add_argument("--go-log", type=Path, required=True)
    parser.add_argument("--go-baseline-log", type=Path,
                        help="demod histogram at the first sample for an uninterrupted Go run")
    parser.add_argument("--require-acquired", action="store_true")
    args = parser.parse_args()
    go_baseline = read_demod_baseline(args.go_baseline_log) if args.go_baseline_log else None
    counts = compare(json.loads(args.oracle.read_text()), parse_go(args.go_log),
                     args.require_acquired, go_baseline)
    print(f"SI comparison passed: {counts[0]} samples, {counts[1]} guest PCs, "
          f"{counts[2]} match units; oracle and Go agree")


if __name__ == "__main__":
    main()
