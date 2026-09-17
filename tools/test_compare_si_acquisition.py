"""Negative controls for SI evidence provenance."""

import hashlib
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
PAGE = ROOT / "reference/digibox-boot.html"
COMPARATOR = ROOT / "tools/compare-si-acquisition.py"


class OracleProvenanceTest(unittest.TestCase):
    def test_mutated_page_fails_before_go_trace_is_read(self):
        original = PAGE.read_bytes()
        with tempfile.TemporaryDirectory() as directory:
            work = Path(directory)
            evidence = work / "oracle.json"
            evidence.write_text(json.dumps({"oracleSha256": hashlib.sha256(original).hexdigest()}))
            changed = work / "digibox-boot.html"
            changed.write_bytes(original + b"\n")
            missing_go_log = work / "missing-go.log"
            result = subprocess.run(
                [sys.executable, str(COMPARATOR), "--oracle", str(evidence),
                 "--oracle-page", str(changed), "--go-log", str(missing_go_log)],
                capture_output=True, text=True, check=False,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("oracle source changed", result.stderr)
            self.assertNotIn("missing-go.log", result.stderr)

    def test_missing_digest_cannot_pass_with_matching_page(self):
        with tempfile.TemporaryDirectory() as directory:
            work = Path(directory)
            evidence = work / "oracle.json"
            evidence.write_text("{}")
            result = subprocess.run(
                [sys.executable, str(COMPARATOR), "--oracle", str(evidence),
                 "--go-log", str(work / "missing-go.log")],
                capture_output=True, text=True, check=False,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("no valid oracleSha256", result.stderr)
            self.assertNotIn("missing-go.log", result.stderr)


if __name__ == "__main__":
    unittest.main()
