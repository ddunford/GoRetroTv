#!/usr/bin/env python3
"""Render a docs tree to ONE self-contained HTML file. No dependencies, no build step.

    docs-site.py build [--docs DIR] [--out DIR/site.html] [--title NAME]
    docs-site.py check [--docs DIR] [--out DIR/site.html]      exit 1 if the page is stale
    docs-site.py serve [--docs DIR] [--port N] [--lan]         viewer, rebuilds on refresh
                                                               loopback by default; --lan also
                                                               listens on this machine's LAN address

The page is GENERATED: regenerate it, never hand-edit it. It carries a hash of its
inputs so `check` can say it has fallen behind, which is the same bargain the
anchors make for the docs themselves -- derivable content is allowed to be
persisted only when something can tell you it went stale.

Each section shows what it is anchored to and when it was last verified, so the
freshness of a page is visible to whoever is reading it rather than only to CI.
"""

from __future__ import annotations

import argparse
import fnmatch
import hashlib
import html
import json
import re
import sys
from pathlib import Path

ANCHOR_RE = re.compile(r"<!--\s*anchor:\s*(.+?)\s*-->", re.I)
FP_RE = re.compile(r"<!--\s*fingerprint:\s*sha256:[0-9a-f]+\s*(?:@\s*([0-9-]+))?\s*-->", re.I)
COMMENT_RE = re.compile(r"<!--.*?-->", re.S)
HEAD_RE = re.compile(r"^(#{1,6})\s+(.*\S)\s*$")


# ------------------------------------------------------------------ inline markdown

def inline(text: str) -> str:
    codes: list[str] = []

    def stash(m: re.Match) -> str:
        codes.append(html.escape(m.group(1)))
        return f"\x00{len(codes) - 1}\x00"

    text = re.sub(r"`([^`]+)`", stash, text)
    text = html.escape(text)
    text = re.sub(r"\[([^\]]+)\]\(([^)]+)\)", r'<a href="\2">\1</a>', text)
    text = re.sub(r"(?<!\*)\*\*([^*]+)\*\*(?!\*)", r"<strong>\1</strong>", text)
    text = re.sub(r"(?<![*\w])\*([^*\n]+)\*(?![*\w])", r"<em>\1</em>", text)
    text = re.sub(r"\x00(\d+)\x00", lambda m: f"<code>{codes[int(m.group(1))]}</code>", text)
    return text


def slug(text: str, seen: dict[str, int]) -> str:
    base = re.sub(r"[^a-z0-9]+", "-", re.sub(r"`", "", text.lower())).strip("-") or "section"
    seen[base] = seen.get(base, 0) + 1
    return base if seen[base] == 1 else f"{base}-{seen[base]}"


# ------------------------------------------------------------------- block markdown

def render(md: str, seen: dict[str, int]) -> tuple[str, list[dict]]:
    lines = md.replace("\r\n", "\n").split("\n")
    out: list[str] = []
    toc: list[dict] = []
    i, n = 0, len(lines)
    pending: list[str] = []      # anchors seen since the last heading
    verified: str | None = None
    current: dict | None = None

    def flush_para(buf: list[str]) -> None:
        if buf:
            out.append(f"<p>{inline(' '.join(buf))}</p>")
            buf.clear()

    para: list[str] = []
    while i < n:
        line = lines[i]

        if line.lstrip().startswith("```"):
            flush_para(para)
            lang = line.strip().strip("`").strip()
            body, i = [], i + 1
            while i < n and not lines[i].lstrip().startswith("```"):
                body.append(lines[i]); i += 1
            i += 1
            cls = f' class="lang-{html.escape(lang)}"' if lang else ""
            out.append(f"<pre{cls}><code>{html.escape(chr(10).join(body))}</code></pre>")
            continue

        if m := ANCHOR_RE.search(line):
            pending.append(m.group(1))
            i += 1
            continue
        if m := FP_RE.search(line):
            verified = m.group(1)
            i += 1
            continue
        if COMMENT_RE.fullmatch(line.strip()):
            i += 1
            continue

        if m := HEAD_RE.match(line):
            flush_para(para)
            level, title = len(m.group(1)), m.group(2)
            sid = slug(title, seen)
            if current is not None:
                current["anchors"] = pending[:]
                current["verified"] = verified
            out.append(f'<h{level} id="{sid}">{inline(title)}</h{level}>')
            marker = f"\x01{sid}\x01"
            out.append(marker)
            current = {"id": sid, "level": level, "title": re.sub(r"`", "", title),
                       "marker": marker, "anchors": [], "verified": None}
            toc.append(current)
            pending, verified = [], None
            i += 1
            continue

        if re.match(r"^\s*\|.*\|\s*$", line) and i + 1 < n and re.match(r"^\s*\|[\s:|-]+\|\s*$", lines[i + 1]):
            flush_para(para)
            def cells(row: str) -> list[str]:
                # Split on UNESCAPED pipes only: GFM lets a cell carry a literal pipe as
                # `\|`, and a naive split turns one cell into two, silently shifting every
                # column after it. Real documents do this inside code spans.
                parts = re.split(r"(?<!\\)\|", row.strip().strip("|"))
                return [c.strip().replace("\\|", "|") for c in parts]
            head = cells(line)
            i += 2
            body_rows = []
            while i < n and re.match(r"^\s*\|.*\|\s*$", lines[i]):
                body_rows.append(cells(lines[i])); i += 1
            th = "".join(f"<th>{inline(c)}</th>" for c in head)
            tb = "".join("<tr>" + "".join(f"<td>{inline(c)}</td>" for c in r) + "</tr>" for r in body_rows)
            out.append(f"<div class=tw><table><thead><tr>{th}</tr></thead><tbody>{tb}</tbody></table></div>")
            continue

        if re.match(r"^\s*([-*+]|\d+[.)])\s+", line):
            flush_para(para)
            ordered = bool(re.match(r"^\s*\d+[.)]\s+", line))
            items: list[str] = []
            while i < n and re.match(r"^\s*([-*+]|\d+[.)])\s+", lines[i]):
                item = re.sub(r"^\s*([-*+]|\d+[.)])\s+", "", lines[i])
                i += 1
                while i < n and lines[i].startswith("  ") and lines[i].strip() and \
                        not re.match(r"^\s*([-*+]|\d+[.)])\s+", lines[i]):
                    item += " " + lines[i].strip(); i += 1
                items.append(f"<li>{inline(item)}</li>")
            tag = "ol" if ordered else "ul"
            out.append(f"<{tag}>{''.join(items)}</{tag}>")
            continue

        if line.startswith(">"):
            flush_para(para)
            quote = []
            while i < n and lines[i].startswith(">"):
                quote.append(lines[i].lstrip("> ").rstrip()); i += 1
            out.append(f"<blockquote>{inline(' '.join(quote))}</blockquote>")
            continue

        if re.match(r"^\s*(-{3,}|\*{3,}|_{3,})\s*$", line):
            flush_para(para); out.append("<hr>"); i += 1; continue

        if not line.strip():
            flush_para(para); i += 1; continue

        para.append(line.strip()); i += 1

    flush_para(para)
    if current is not None:
        current["anchors"] = pending[:]
        current["verified"] = verified

    body = "\n".join(out)
    for sec in toc:
        bits = []
        for a in sec["anchors"]:
            cls = "ax ax-none" if a.lower().startswith("none") else "ax"
            label = a if not a.lower().startswith("none") else a[4:].lstrip(" -—")
            bits.append(f'<span class="{cls}">{html.escape(label)}</span>')
        if sec["verified"]:
            bits.append(f'<span class="ax ax-v">verified {html.escape(sec["verified"])}</span>')
        body = body.replace(sec["marker"], f'<div class=axs>{"".join(bits)}</div>' if bits else "")
        del sec["marker"]
    return body, toc


# --------------------------------------------------------------------------- page

CSS = """
:root{--bg:#fbfaf8;--fg:#1b1a17;--mut:#6a6760;--line:#e2ded6;--card:#fff;--acc:#8a5209;--code:#f3f0ea}
@media (prefers-color-scheme:dark){:root:not([data-theme=light]){
 --bg:#131311;--fg:#eceae5;--mut:#9a958c;--line:#2c2b27;--card:#1a1a18;--acc:#e0a960;--code:#1f1e1b}}
:root[data-theme=dark]{--bg:#131311;--fg:#eceae5;--mut:#9a958c;--line:#2c2b27;--card:#1a1a18;--acc:#e0a960;--code:#1f1e1b}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--fg);font:16px/1.65 ui-sans-serif,system-ui,-apple-system,"Segoe UI",sans-serif}
.wrap{display:grid;grid-template-columns:290px minmax(0,1fr);gap:0;min-height:100vh}
nav{border-right:1px solid var(--line);padding:20px 16px;position:sticky;top:0;height:100vh;overflow:auto}
nav h1{font-size:15px;margin:0 0 4px;letter-spacing:.01em}
nav .sub{color:var(--mut);font-size:12px;margin-bottom:14px}
#q{width:100%;padding:8px 10px;border:1px solid var(--line);border-radius:7px;background:var(--card);color:var(--fg);font-size:13px}
nav ul{list-style:none;margin:14px 0 0;padding:0}
nav li{margin:1px 0}
nav a{display:block;padding:3px 8px;border-radius:6px;color:var(--fg);text-decoration:none;font-size:13px;line-height:1.4}
nav a:hover{background:var(--code)}
nav a.f{color:var(--mut);font-size:12px;padding-left:18px}
nav .grp{margin-top:14px;font-size:11px;text-transform:uppercase;letter-spacing:.07em;color:var(--mut);padding:0 8px}
main{padding:34px 40px 120px;max-width:860px;overflow:hidden}
h1,h2,h3,h4,h5,h6{line-height:1.25;margin:1.8em 0 .5em;scroll-margin-top:16px}
main>h1:first-child{margin-top:0}
h1{font-size:30px}h2{font-size:22px;padding-top:12px;border-top:1px solid var(--line)}h3{font-size:17px}h4{font-size:15px;color:var(--mut)}
p,li{overflow-wrap:anywhere}
a{color:var(--acc)}
code{background:var(--code);padding:.12em .36em;border-radius:4px;font:13px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace}
pre{background:var(--code);padding:14px 16px;border-radius:9px;overflow:auto;border:1px solid var(--line)}
pre code{background:none;padding:0;font-size:12.5px}
blockquote{margin:1em 0;padding:.5em 1em;border-left:3px solid var(--acc);background:var(--card);color:var(--mut)}
.tw{overflow-x:auto;margin:1em 0}
table{border-collapse:collapse;width:100%;font-size:14px}
th,td{border:1px solid var(--line);padding:7px 10px;text-align:left;vertical-align:top}
th{background:var(--card);font-weight:600}
hr{border:0;border-top:1px solid var(--line);margin:2em 0}
.axs{display:flex;flex-wrap:wrap;gap:5px;margin:-.2em 0 1em}
.ax{font:11px/1.5 ui-monospace,monospace;color:var(--mut);background:var(--card);border:1px solid var(--line);border-radius:5px;padding:1px 7px}
.ax-none{opacity:.72;font-style:italic}
.ax-v{border-color:var(--acc);color:var(--acc)}
.doc{display:none}.doc.on{display:block}
.bar{display:flex;gap:10px;align-items:center;margin-bottom:22px;flex-wrap:wrap}
.bar .stat{font:11px/1.5 ui-monospace,monospace;color:var(--mut);border:1px solid var(--line);border-radius:5px;padding:2px 8px}
#tg{margin-left:auto;background:var(--card);border:1px solid var(--line);color:var(--fg);border-radius:7px;padding:5px 11px;cursor:pointer;font-size:12px}
.hit{padding:7px 9px;border:1px solid var(--line);border-radius:7px;margin:5px 0;cursor:pointer;background:var(--card)}
.hit b{display:block;font-size:13px}.hit span{font-size:11.5px;color:var(--mut)}
@media(max-width:860px){.wrap{grid-template-columns:1fr}nav{position:static;height:auto;border-right:0;border-bottom:1px solid var(--line)}main{padding:22px 16px 90px}}
"""

JS = """
const D=DOCS;let cur=null;
const nav=document.getElementById('nav'),main=document.getElementById('main'),q=document.getElementById('q');
function show(f,h){cur=f;document.querySelectorAll('.doc').forEach(e=>e.classList.toggle('on',e.dataset.f===f));
 document.querySelectorAll('nav a').forEach(a=>a.style.fontWeight=a.dataset.f===f&&!a.dataset.h?'600':'400');
 if(h){const el=document.getElementById(h);if(el)el.scrollIntoView();}else main.scrollTo(0,0),window.scrollTo(0,0);
 history.replaceState(null,'','#'+f+(h?'::'+h:''));}
function buildNav(){let h='';for(const g of Object.keys(D.groups)){h+=`<div class=grp>${g==='.'?'Docs':g}</div><ul>`;
 for(const f of D.groups[g]){const d=D.docs[f];h+=`<li><a href="#" data-f="${f}">${d.title}</a></li>`;
  for(const s of d.toc.filter(s=>s.level===2).slice(0,40))h+=`<li><a class=f href="#" data-f="${f}" data-h="${s.id}">${s.title}</a></li>`;}
 h+='</ul>';}nav.innerHTML=h;
 nav.querySelectorAll('a').forEach(a=>a.onclick=e=>{e.preventDefault();show(a.dataset.f,a.dataset.h)});}
function search(t){t=t.trim().toLowerCase();if(!t.length){document.getElementById('res').innerHTML='';buildNav();return;}
 const hits=[];for(const f of Object.keys(D.docs)){const d=D.docs[f];
  for(const s of d.toc)if(s.title.toLowerCase().includes(t))hits.push({f,d,s});
  if(d.text.toLowerCase().includes(t)&&!hits.some(x=>x.f===f))hits.push({f,d,s:d.toc[0]||{id:'',title:d.title}});}
 document.getElementById('res').innerHTML=hits.slice(0,60).map(x=>
  `<div class=hit data-f="${x.f}" data-h="${x.s.id}"><b>${x.s.title}</b><span>${x.s.title===x.d.title?x.f:x.d.title}</span></div>`).join('')||'<p style="font-size:13px;color:var(--mut)">No match.</p>';
 document.querySelectorAll('.hit').forEach(e=>e.onclick=()=>show(e.dataset.f,e.dataset.h));}
q.oninput=()=>search(q.value);
document.getElementById('tg').onclick=()=>{const r=document.documentElement;
 const d=r.dataset.theme?r.dataset.theme==='dark':matchMedia('(prefers-color-scheme:dark)').matches;
 r.dataset.theme=d?'light':'dark';try{localStorage.setItem('t',r.dataset.theme)}catch(e){}};
try{const t=localStorage.getItem('t');if(t)document.documentElement.dataset.theme=t}catch(e){}
buildNav();
function fromHash(){const h=decodeURIComponent(location.hash.slice(1)).split('::');
 show(D.docs[h[0]]?h[0]:D.start,h[1]);}
// A hash change on an already-loaded page fires no navigation, so without this a shared
// deep link only works when the page is opened cold -- which is never how a link is used.
addEventListener('hashchange',fromHash);
fromHash();
"""


def build(docs: Path, out: Path, title: str, exclude: list[str] | None = None) -> int:
    patterns = exclude or []
    files = [p for p in sorted(docs.rglob("*.md"))
             if not any(fnmatch.fnmatch(p.relative_to(docs).as_posix(), pat) for pat in patterns)]
    if not files:
        print(f"no markdown under {docs}", file=sys.stderr)
        return 1
    seen: dict[str, int] = {}
    payload: dict[str, dict] = {}
    groups: dict[str, list[str]] = {}
    body_html: list[str] = []
    digest = hashlib.sha256()
    anchored = exempt = 0

    for path in files:
        rel = path.relative_to(docs).as_posix()
        raw = path.read_text(encoding="utf-8")
        digest.update(rel.encode() + b"\x00" + raw.encode())
        html_body, toc = render(raw, seen)
        for sec in toc:
            for a in sec["anchors"]:
                if a.lower().startswith("none"):
                    exempt += 1
                else:
                    anchored += 1
        doc_title = next((s["title"] for s in toc if s["level"] == 1), rel)
        group = rel.rsplit("/", 1)[0] if "/" in rel else "."
        groups.setdefault(group, []).append(rel)
        text = COMMENT_RE.sub("", raw)
        payload[rel] = {"title": doc_title, "toc": toc, "text": text[:200000]}
        body_html.append(f'<article class=doc data-f="{html.escape(rel)}">{html_body}</article>')

    # Reading order, not filename order: the way in, then the map, then the parts, then the rest.
    # A nav sorted by path puts "architecture" above "orientation" and buries both under whatever
    # happens to start with a letter earlier in the alphabet.
    GROUP_RANK = {".": 0, "components": 1, "explanation": 2, "how-to": 3, "tutorial": 4, "reference": 5}
    FILE_RANK = {"README.md": 0, "orientation.md": 1, "architecture.md": 2}
    order = sorted(groups, key=lambda g: (GROUP_RANK.get(g, 9), g))
    for g in groups:
        groups[g] = sorted(groups[g], key=lambda f: (FILE_RANK.get(f, 9), f))
    start = "README.md" if "README.md" in payload else \
            ("orientation.md" if "orientation.md" in payload else files[0].relative_to(docs).as_posix())
    digest.update(b"\x00exclude\x00" + "\x00".join(sorted(patterns)).encode())
    src_hash = digest.hexdigest()
    data = json.dumps({"docs": payload, "groups": {g: groups[g] for g in order}, "start": start})

    page = f"""<!DOCTYPE html>
<html lang=en><head><meta charset=utf-8>
<meta name=viewport content="width=device-width,initial-scale=1">
<title>{html.escape(title)}</title>
<meta name=generator content="docs-site.py src:{src_hash}">
<link rel=icon href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 16 16'%3E%3Crect width='16' height='16' rx='3' fill='%238a5209'/%3E%3Cpath d='M4 5h8M4 8h8M4 11h5' stroke='%23fff' stroke-width='1.5' stroke-linecap='round'/%3E%3C/svg%3E">
<style>{CSS}</style></head><body>
<div class=wrap>
<nav><h1>{html.escape(title)}</h1><div class=sub>{len(files)} documents</div>
<input id=q placeholder="Search" autocomplete=off><div id=res></div><div id=nav></div></nav>
<main id=main><div class=bar>
<span class=stat>{anchored} anchored</span><span class=stat>{exempt} exempt</span>
<span class=stat>generated &mdash; do not edit</span>
<button id=tg>Theme</button></div>
{''.join(body_html)}
</main></div>
<script>const DOCS={data};{JS}</script>
</body></html>"""
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(page, encoding="utf-8")
    print(f"{out}  ({len(page)/1024:.0f} KB, {len(files)} documents, {anchored} anchored, {exempt} exempt)")
    return 0


def source_hash(docs: Path, exclude: list[str] | None = None) -> str:
    patterns = exclude or []
    digest = hashlib.sha256()
    for path in sorted(docs.rglob("*.md")):
        rel = path.relative_to(docs).as_posix()
        if any(fnmatch.fnmatch(rel, pat) for pat in patterns):
            continue
        digest.update(rel.encode() + b"\x00" + path.read_text(encoding="utf-8").encode())
    digest.update(b"\x00exclude\x00" + "\x00".join(sorted(patterns)).encode())
    return digest.hexdigest()


def page_hash(out: Path) -> str | None:
    if not out.exists():
        return None
    m = re.search(r'content="docs-site\.py src:([0-9a-f]+)"', out.read_text(encoding="utf-8"))
    return m.group(1) if m else None


def check(docs: Path, out: Path, exclude: list[str] | None = None) -> int:
    if not out.exists():
        print(f"{out} does not exist — run: docs-site.py build", file=sys.stderr)
        return 1
    stamped = page_hash(out)
    if stamped is None:
        print(f"{out} carries no generator stamp — not generated by this tool", file=sys.stderr)
        return 1
    if stamped != source_hash(docs, exclude):
        print(f"STALE: {out} was generated from different docs — rebuild it", file=sys.stderr)
        return 1
    print(f"{out} is current")
    return 0


def primary_lan_address() -> str | None:
    """This machine's address on its default route -- not every address it has.

    A UDP connect to a documentation-only address (TEST-NET-1, RFC 5737) sends no
    packets; it just asks the routing table which source address would be used. That
    picks the real LAN interface and skips the container bridges and VPN interfaces a
    host may have dozens of, which is the whole point of not binding 0.0.0.0.
    """
    import socket
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        s.connect(("192.0.2.1", 9))
        addr = s.getsockname()[0]
        return None if addr.startswith("127.") else addr
    except OSError:
        return None
    finally:
        s.close()


def serve(docs: Path, out: Path, title: str, port: int, hosts: list[str],
          exclude: list[str] | None = None) -> int:
    """Serve the viewer on each named address, rebuilding whenever the docs move.

    Loopback is the default and everything else is opt-in, because a docs server is a
    developer surface and most projects record an invariant about where those may
    listen. Reaching it from another machine is a deliberate exception, asked for by
    name -- not something that happens because a default was convenient.

    Each address gets its own listener rather than binding 0.0.0.0. On a host with
    container bridges or a VPN, a wildcard bind publishes the viewer on every one of
    them; naming the addresses publishes it on exactly the ones asked for.
    """
    import functools
    import threading
    from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer

    def ensure() -> None:
        if page_hash(out) != source_hash(docs, exclude):
            build(docs, out, title, exclude)

    class Handler(SimpleHTTPRequestHandler):
        def do_GET(self):  # noqa: N802
            path = self.path.split("?")[0]
            if path in ("/", "/index.html"):
                ensure()
                self.send_response(302)
                self.send_header("Location", "/" + out.name)
                self.end_headers()
                return
            if path == "/" + out.name:
                ensure()          # edit a doc, refresh, see it: no restart, no watcher
            super().do_GET()

        def end_headers(self):
            # A viewer that serves a cached copy of the page it just rebuilt is worse
            # than one that never rebuilds, because it looks current and is not.
            self.send_header("Cache-Control", "no-store")
            super().end_headers()

        def log_message(self, fmt, *a):
            pass

    ensure()
    handler = functools.partial(Handler, directory=str(out.parent))
    servers: list[ThreadingHTTPServer] = []
    for host in dict.fromkeys(hosts):          # de-duplicate, keep the order given
        try:
            servers.append(ThreadingHTTPServer((host, port), handler))
        except OSError as exc:
            for made in servers:
                made.server_close()
            print(f"cannot listen on {host}:{port} — {exc}", file=sys.stderr)
            return 1

    # flush: stdout is block-buffered when this is not a tty (ctl.sh redirects, CI logs),
    # and a server whose only sign of life appears after you kill it reads as a hang.
    for httpd in servers:
        print(f"docs viewer  http://{httpd.server_address[0]}:{port}/", flush=True)
    if any(not h.startswith("127.") for h in hosts):
        print("             reachable from other machines on this network", flush=True)
    print("             rebuilds on refresh; Ctrl-C to stop", flush=True)

    threads = [threading.Thread(target=h.serve_forever, daemon=True) for h in servers]
    for t in threads:
        t.start()
    try:
        while True:
            threads[0].join(timeout=1.0)
    except KeyboardInterrupt:
        print("\nstopped")
    finally:
        for httpd in servers:
            httpd.shutdown()
            httpd.server_close()
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("cmd", choices=["build", "check", "serve"])
    ap.add_argument("--docs", default="docs")
    ap.add_argument("--out", default=None)
    ap.add_argument("--title", default="Documentation")
    ap.add_argument("--exclude", action="append", default=None, metavar="GLOB",
                    help="a doc to leave OUT of the viewer, repeatable. For build-history or "
                         "generated pages whose place is the tracker, not the documentation.")
    ap.add_argument("--port", type=int, default=8098, help="serve: port (default 8098)")
    ap.add_argument("--host", action="append", default=None,
                    help="serve: an address to listen on, repeatable (default 127.0.0.1)")
    ap.add_argument("--lan", action="store_true",
                    help="serve: ALSO listen on this machine's LAN address, reachable from other machines")
    a = ap.parse_args()
    docs = Path(a.docs)
    out = Path(a.out) if a.out else docs / "site.html"
    if a.cmd == "build":
        return build(docs, out, a.title, a.exclude)
    if a.cmd == "serve":
        hosts = list(a.host) if a.host else ["127.0.0.1"]
        if a.lan:
            lan = primary_lan_address()
            if lan is None:
                print("--lan: no non-loopback address on the default route", file=sys.stderr)
                return 1
            hosts.append(lan)
        return serve(docs, out, a.title, a.port, hosts, a.exclude)
    return check(docs, out, a.exclude)


if __name__ == "__main__":
    sys.exit(main())
