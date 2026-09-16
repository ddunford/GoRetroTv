"""The Python side of `conformance/corpus.sh` — the one definition of what a rule may look at.

corpus.sh has always said "Every rule that walks files walks this list", and no checker did: they
enumerated the filesystem with glob and rglob. The written contract was untrue, and the failure it
argues against was live — a gitignored scratch file in the working tree is inside every checker's
reach, so a rule can go red on a file that is deliberately not in version control, differs on every
machine, and no rule can legitimately govern. The fix everyone then reaches for is a standing
exception, which is how an exception list quietly becomes the rule.

TRACKED CONTENT ONLY, and the probe runner builds its worktree from the same snapshot, so the two
cannot disagree about what the repository asserts.
"""

from __future__ import annotations

import subprocess
from functools import lru_cache
from pathlib import Path


@lru_cache(maxsize=1)
def tracked(root: str) -> frozenset[str]:
    """Every path `git ls-files` reports, relative to the repository root."""
    r = subprocess.run(["git", "ls-files", "-z"], cwd=root, capture_output=True, text=True)
    if r.returncode != 0:
        # A corpus that cannot be read is a harness failure, and the caller must be able to tell
        # that from an empty corpus — which is why this raises rather than returning nothing.
        raise RuntimeError(f"git ls-files failed in {root}: {r.stderr.strip()[:200]}")
    return frozenset(p for p in r.stdout.split("\0") if p)


def in_corpus(root: Path, path: Path) -> bool:
    try:
        return str(path.resolve().relative_to(root.resolve())) in tracked(str(root))
    except ValueError:
        return False


def walk(root: Path, base: Path, pattern: str = "*.py") -> list[Path]:
    """`base.rglob(pattern)`, filtered to tracked content and sorted for a stable report."""
    if not base.is_dir():
        return []
    return sorted(p for p in base.rglob(pattern)
                  if p.is_file() and "__pycache__" not in p.parts and in_corpus(root, p))
