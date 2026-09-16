#!/usr/bin/env python3
"""TEMPLATE — a real running application, built from an arbitrary tree, for RUNTIME rules.

PROJECT-SHAPED CONSTANTS, adjust before use: `SERVICE` (the compose service whose image carries the
interpreter and dependency set), the `/app` mount points, the virtualenv path, and the command that
starts the application. Everything else — the identity guard, the isolation, the in-container
client, resolving project resources from the PROJECT rather than the tree under test — is the part
that generalises and the part that was expensive to get right.

A real running application, built from an arbitrary tree, for RUNTIME rules.

WHY THIS EXISTS. A STATIC rule reads files; a RUNTIME rule has to observe the application actually
behaving — the header a server really sends, the shape of a log line it really writes. Neither was
reachable here, for two measured reasons:

  1. Checkers run on the HOST interpreter, which is 3.12 and carries none of the backend's
     dependencies — and since the py314 reformat it cannot even PARSE two backend modules, because
     PEP 758's unparenthesised `except A, B:` is a SyntaxError before 3.14. So a checker cannot
     import the app.
  2. The only runtime was the compose stack, which bind-mounts ./backend from the MAIN tree. A probe
     worktree is invisible to it: a marker appended inside a worktree read 1 in the worktree and 0
     in the running container. Every probe for a runtime rule would have reported UNCAUGHT while the
     checker inspected the main tree's application, so no runtime rule could be armed at all.

WHAT IT DOES. Starts the backend from the tree it is given, in a throwaway container built on the
image compose already built — so the dependencies come from the image and the CODE comes from the
tree. Nothing is networked to the stack: the app's lifespan connects to nothing at startup, so an
unrouted request produces a real response and a real log line without a database in sight. That is
deliberate rather than convenient — a conformance runtime that writes to the development database
would be a gate with side effects.

THE GUARD THAT MAKES IT WORTH ANYTHING. It hashes the `app` package INSIDE the running container and
compares it with the tree it was asked to serve, and refuses to yield if they differ. Without that,
every failure mode of this file looks like a passing rule: a mount that silently resolves to the
main tree, an image with the code baked in, a stale container. The whole point of the task that
produced this file was that the runtime was reading the wrong tree, so the check that it is reading
the right one cannot be optional.
"""

from __future__ import annotations

import hashlib
import json
import os
import subprocess
import time
import urllib.error
import urllib.request
from contextlib import contextmanager
from collections.abc import Iterator
from dataclasses import dataclass, field
from pathlib import Path

# The compose SERVICE whose image supplies the interpreter and the dependency set. It never
# supplies the code, which is mounted from the tree under test.
SERVICE = "app"
BOOT_TIMEOUT_S = 45.0


class RuntimeUnavailable(Exception):
    """The runtime could not be started. A HARNESS failure, never a verdict about the rule."""


def _sh(*args: str, check: bool = True) -> str:
    r = subprocess.run(args, capture_output=True, text=True)
    if check and r.returncode != 0:
        raise RuntimeUnavailable(f"{' '.join(args[:3])}… exited {r.returncode}: {r.stderr.strip()[:300]}")
    return r.stdout.strip()


def _digest(files: dict[str, str]) -> str:
    """One digest over `path -> sha256(content)`, so the comparison is order- and encoding-free."""
    h = hashlib.sha256()
    for name in sorted(files):
        h.update(f"{name}:{files[name]}\n".encode())
    return h.hexdigest()


FIND = "find app -name '*.py' -not -path '*__pycache__*'"


def _tree_files(backend: Path) -> dict[str, str]:
    out: dict[str, str] = {}
    for p in backend.joinpath("app").rglob("*.py"):
        if "__pycache__" in p.parts:
            continue
        out[str(p.relative_to(backend))] = hashlib.sha256(p.read_bytes()).hexdigest()
    return out


def _container_files(name: str) -> dict[str, str]:
    """Hashed INSIDE the container, one exec, and deliberately not by reading the files out.

    The first version cat-ed each file through a helper that `.strip()`ped the output — so every
    comparison failed on a trailing newline and the guard reported a mount that was in fact
    perfectly correct. A guard that cries wolf is disabled by the third person who meets it, which
    would have cost more than the bug. Hashing at the source removes the transport entirely.
    """
    r = subprocess.run(
        ["docker", "exec", name, "sh", "-c", f"cd /app && {FIND} -print0 | sort -z | xargs -0 sha256sum"],
        capture_output=True, text=True,
    )
    if r.returncode != 0:
        raise RuntimeUnavailable(f"could not hash the app package in the container: {r.stderr.strip()[:300]}")
    out: dict[str, str] = {}
    for line in r.stdout.splitlines():
        digest, _, path = line.partition("  ")
        if digest and path:
            out[path] = digest
    if not out:
        raise RuntimeUnavailable("the container shows no app/*.py at all — nothing is mounted")
    return out



# THE HTTP CLIENT LIVES INSIDE THE CONTAINER, because nothing is published and there is no network
# to publish onto. One `docker exec` per request, running the container's own interpreter — slower
# per call than a socket and irrelevant beside a boot, and it buys real isolation instead of an
# asserted one. It also removes the published port, so two runtimes can no longer collide on one.
# A CAP, AND IT SAYS WHEN IT BITES. The first version read 4096 bytes and said nothing — the served
# OpenAPI document is larger, so it came back as truncated JSON and the rule reported a harness
# failure about a delimiter. A silent truncation in a transport is the same class of defect as a
# silent skip in a checker: the caller receives something that looks like an answer.
_CLIENT = r"""
import json, sys, urllib.error, urllib.request
LIMIT = 4 * 1024 * 1024
method, path, headers, body = json.loads(sys.argv[1])
req = urllib.request.Request("http://127.0.0.1:8000" + path, method=method, headers=headers,
                             data=body.encode() if body else None)


def answer(status, head, stream):
    raw = stream.read(LIMIT + 1)
    if len(raw) > LIMIT:
        print(json.dumps([0, {}, "response exceeded %d bytes; the client refuses to truncate it "
                                 "silently" % LIMIT]))
        return
    print(json.dumps([status, dict(head), raw.decode("utf-8", "replace")]))


try:
    with urllib.request.urlopen(req, timeout=20) as r:
        answer(r.status, r.headers, r)
except urllib.error.HTTPError as e:
    answer(e.code, e.headers, e)
except Exception as e:
    print(json.dumps([0, {}, str(e)]))
"""


def _request(name: str, method: str, path: str,
             headers: dict[str, str], body: str = "") -> tuple[int, dict[str, str], str]:
    """(status, headers, body) for one request, made from inside the container."""
    payload = json.dumps([method, path, headers, body])
    r = subprocess.run(
        ["docker", "exec", name, "/app/.venv/bin/python", "-c", _CLIENT, payload],
        capture_output=True, text=True,
    )
    if r.returncode != 0 or not r.stdout.strip():
        return 0, {}, r.stderr.strip()[:200]
    status, head, text = json.loads(r.stdout.strip().splitlines()[-1])
    return status, {k.lower(): v for k, v in head.items()}, text


@dataclass
class AppRuntime:
    """One running application. `get` makes a request; `logs` returns everything it has written."""

    name: str
    reading: str
    _since: float = field(default_factory=time.time)

    def get(self, path: str, headers: dict[str, str] | None = None) -> tuple[int, dict[str, str]]:
        status, head, _ = self.request("GET", path, headers)
        return status, head

    def request(self, method: str, path: str, headers: dict[str, str] | None = None,
                body: str = "") -> tuple[int, dict[str, str], str]:
        """One request, WITH ITS BODY. A 404 or a 503 is a response and for these rules it is as
        informative as a 200 — more so, since it reaches the error paths."""
        status, head, text = _request(self.name, method, path, headers or {}, body)
        if status == 0:
            raise RuntimeUnavailable(f"the running app did not answer {method} {path}: {text}")
        return status, head, text

    def logs(self) -> list[str]:
        out = subprocess.run(["docker", "logs", self.name], capture_output=True, text=True)
        return [ln for ln in (out.stdout + out.stderr).splitlines() if ln.strip()]


def project_root() -> Path:
    """The MAIN checkout, not the tree under test.

    EVERYTHING THIS FILE NEEDS FROM THE PROJECT MUST COME FROM HERE, and that sentence is the whole
    lesson of building it: the virtualenv and the container image both belong to the project, and
    both were resolved relative to this file at first. A probe runs the checker from inside a
    WORKTREE, so `parents[1]` is the worktree — which has no virtualenv (the directory is untracked
    and always will be) and whose name makes docker compose derive a project nobody has ever built
    an image for. Each cost a CI run to find, separately, for the same reason.

    `--git-common-dir` is what a worktree knows about its main checkout. Outside a repository, or
    in an rsync'd copy with no link home, the answer is this file's own project — which is why both
    callers also take an explicit environment override.
    """
    here = Path(__file__).resolve().parents[1]
    r = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"],
                       cwd=here, capture_output=True, text=True)
    if r.returncode == 0 and r.stdout.strip():
        candidate = Path(r.stdout.strip()).parent
        if (candidate / "docker-compose.yml").is_file():
            return candidate
    return here


def _project_venv() -> Path:
    override = os.environ.get("CONFORMANCE_BACKEND_VENV")
    if override:
        return Path(override)
    return project_root() / "backend" / ".venv"


def _image() -> str:
    """The image compose builds for the app service, ASKED FOR rather than assembled.

    It was hardcoded as `<thisdirectory>-app` for an afternoon, which is a name that holds only on
    the machine the directory happens to be called that on: compose derives the project from the
    working directory, so the same image is `retrotv-app` on a CI runner that checked the repository
    out under its own name. A gate that works on one machine because of a coincidence in a path is
    the thing this harness exists to refuse.
    """
    override = os.environ.get("CONFORMANCE_APP_IMAGE")
    if override:
        return override
    r = subprocess.run(["docker", "compose", "config", "--format", "json"],
                       cwd=project_root(), capture_output=True, text=True)
    if r.returncode != 0:
        raise RuntimeUnavailable(f"docker compose could not resolve its config: {r.stderr.strip()[:200]}")
    config = json.loads(r.stdout)
    service = (config.get("services") or {}).get(SERVICE) or {}
    candidates = [service.get("image")] if service.get("image") else []
    project = config.get("name")
    if project:
        candidates += [f"{project}-{SERVICE}", f"{project}_{SERVICE}"]
    for candidate in candidates:
        if candidate and _sh("docker", "images", "-q", candidate, check=False):
            return candidate
    raise RuntimeUnavailable(
        f"no image for the {SERVICE!r} service exists (looked for {candidates}). Build it with "
        f"`./ctl.sh up` or `docker compose build {SERVICE}` — this runtime does not build images, "
        f"because a probe that triggers a multi-minute build is a probe nobody runs."
    )


@contextmanager
def app_runtime(tree: Path, *, env: dict[str, str] | None = None) -> Iterator[AppRuntime]:
    """Boot the backend from `tree`, prove it is serving THAT tree, and tear it down after."""
    backend = tree / "backend"
    if not (backend / "app" / "main.py").is_file():
        raise RuntimeUnavailable(f"{backend}/app/main.py does not exist; there is nothing to serve")
    venv = _project_venv()
    # `is_file()` FOLLOWS the symlink and answers False here. The virtualenv was created inside the
    # container, so bin/python points at /usr/local/bin/python3.14 — a path that exists only in the
    # image. From the host it is a dangling link that is nonetheless exactly the right thing to
    # mount. Test the link, not its target.
    interpreter = venv / "bin" / "python"
    if not (interpreter.is_symlink() or interpreter.is_file()):
        raise RuntimeUnavailable(
            f"{venv}/bin/python does not exist. The throwaway container takes its dependencies from "
            f"the project's virtualenv and its CODE from the tree under test; without the venv it "
            f"would have to resolve dependencies over the network on every probe."
        )
    image = _image()

    environment = {"SECRET_KEY": "conformance-runtime-not-a-real-secret", "DEBUG": "false"}
    environment.update(env or {})
    name = f"conformance-runtime-{int(time.time() * 1000)}"
    # --network none, AND THE CLAIM IS NOW TRUE. Four places said "networked to nothing"; the
    # container was on the default bridge with working outbound DNS, and the database isolation was
    # an ACCIDENT — `postgres` simply does not resolve there. The safety argument for ARCH-API-2
    # calling every operation without credentials rests on this, so it had to become a property of
    # the container rather than a property of a hostname.
    #
    # --user, because the container ran as root and wrote root-owned __pycache__ and mount points
    # into the tree under test. In a probe worktree that made `git worktree remove` fail on EACCES,
    # which the runner swallowed: 42 orphaned worktrees and 179 MB on this host in one afternoon,
    # not removable without sudo.
    args = ["docker", "run", "-d", "--name", name,
            "--network", "none",
            "--user", f"{os.getuid()}:{os.getgid()}",
            "-e", "PYTHONDONTWRITEBYTECODE=1",
            "-v", f"{backend.resolve()}:/app", "-v", f"{venv}:/app/.venv:ro"]
    for k, v in environment.items():
        args += ["-e", f"{k}={v}"]
    # Not `uv run`: uv would try to resolve the mounted project and could rebuild the environment.
    args += [image, "/app/.venv/bin/python", "-m", "uvicorn", "app.main:app",
             "--host", "127.0.0.1", "--port", "8000"]
    _sh(*args)

    try:
        deadline = time.time() + BOOT_TIMEOUT_S
        last = ""
        while time.time() < deadline:
            status, _, _ = _request(name, "GET", "/", {})
            if status:
                break
            last = "no answer yet"
            if _sh("docker", "inspect", "-f", "{{.State.Running}}", name, check=False) != "true":
                break
            time.sleep(0.4)
        else:
            raise RuntimeUnavailable(f"the app did not answer within {BOOT_TIMEOUT_S:.0f}s: {last}")

        running = _sh("docker", "inspect", "-f", "{{.State.Running}}", name, check=False)
        if running != "true":
            logs = subprocess.run(["docker", "logs", name], capture_output=True, text=True)
            raise RuntimeUnavailable(
                f"the app exited during startup:\n{(logs.stdout + logs.stderr).strip()[-800:]}")

        # THE GUARD. Every other failure of this file is indistinguishable from a passing rule.
        here, there = _tree_files(backend), _container_files(name)
        want, got = _digest(here), _digest(there)
        if want != got:
            differing = sorted(set(here) ^ set(there)) or \
                sorted(k for k in here if here[k] != there.get(k))
            raise RuntimeUnavailable(
                f"the running app is NOT serving {tree}. The app package inside the container "
                f"hashes to {got[:12]} and the tree hashes to {want[:12]}; first files that "
                f"differ: {differing[:5]}. A verdict from it would be about some other copy of the "
                f"code — which is the exact defect this runtime was built to fix."
            )
        # NO base_url FIELD, deliberately. It named an address only the container can reach, and
        # on the host that same address is the DEVELOPMENT STACK — so one `urllib.urlopen(
        # rt.base_url + path)` in a checker would silently inspect the running dev app instead of
        # the tree under test, past the identity guard and every other check here. A field that is
        # correct in one process and catastrophically wrong in another should not exist; every
        # request goes through `request()`, which runs inside the container or not at all.
        yield AppRuntime(name=name,
                         reading=f"container {name[-6:]} serving {tree} (app package {want[:12]})")
    finally:
        subprocess.run(["docker", "rm", "-f", name], capture_output=True)


if __name__ == "__main__":
    # `python3 conformance/runtime.py` boots the working tree and says what it saw. Useful on its
    # own: if this cannot run, every RUNTIME rule is blocked and the reason should be one command
    # away rather than buried in a probe.
    root = Path(__file__).resolve().parents[1]
    try:
        with app_runtime(root) as rt:
            status, headers = rt.get("/api/v1/does-not-exist")
            print(f"ok — {rt.reading}")
            print(f"     GET /api/v1/does-not-exist -> {status}, "
                  f"x-request-id={headers.get('x-request-id', '<none>')}")
            print(f"     {len(rt.logs())} log lines captured; first parses as JSON: "
                  f"{bool(json.loads(rt.logs()[0]))}")
    except RuntimeUnavailable as exc:
        raise SystemExit(f"runtime unavailable: {exc}")
