#!/usr/bin/env python3
"""Aggregate the per-component coverage reports into one HTML index.

Reads whatever coverage artifacts exist (they are produced by the
`make test-*-cover` targets and the CI run) and writes
`reports/coverage/index.html` — a single page with a coverage % per component
and a link to each component's own detailed HTML report.

Sources (all optional; a missing one is reported as "not generated"):
  backend  backend/coverage.out            → % via `go tool cover -func`
  mcp      mcp/coverage.out                → % via `go tool cover -func`
  web      frontend/coverage/lcov.info     → % via LH/LF totals
  android  android/app/build/reports/jacoco/jacocoTestReport/jacocoTestReport.xml

Usage: python3 tools/coverage-report.py [repo-root]
"""
import os
import subprocess
import sys
import xml.etree.ElementTree as ET

ROOT = os.path.abspath(sys.argv[1] if len(sys.argv) > 1 else ".")
GO = os.environ.get("GO", "go")


def pct_go(profile):
    mod_dir = os.path.dirname(profile)
    out = subprocess.check_output(
        [GO, "tool", "cover", "-func=" + os.path.basename(profile)],
        cwd=mod_dir, text=True,
    )
    for line in out.splitlines():
        if line.startswith("total:"):
            return float(line.split()[-1].rstrip("%"))
    return None


def pct_lcov(info):
    hit = found = 0
    with open(info) as f:
        for line in f:
            if line.startswith("LH:"):
                hit += int(line[3:])
            elif line.startswith("LF:"):
                found += int(line[3:])
    return 100.0 * hit / found if found else 0.0


def pct_jacoco(xml):
    root = ET.parse(xml).getroot()
    for c in root.findall("counter"):
        if c.get("type") == "INSTRUCTION":
            missed, covered = int(c.get("missed")), int(c.get("covered"))
            total = missed + covered
            return 100.0 * covered / total if total else 0.0
    return None


# (component, kind, source path, detailed-report link relative to repo root)
COMPONENTS = [
    ("backend", "go", "backend/coverage.out", "backend/cover.html"),
    ("frontend", "lcov", "frontend/coverage/lcov.info", "frontend/coverage/index.html"),
    ("android", "jacoco",
     "android/app/build/reports/jacoco/jacocoTestReport/jacocoTestReport.xml",
     "android/app/build/reports/jacoco/jacocoTestReport/html/index.html"),
    ("mcp", "go", "mcp/coverage.out", "mcp/cover.html"),
]


def color(p):
    return ("#4c1" if p >= 80 else "#a4a61d" if p >= 60 else "#dfb317"
            if p >= 40 else "#fe7d37" if p >= 20 else "#e05d44")


def main():
    rows = []
    for name, kind, src, report in COMPONENTS:
        path = os.path.join(ROOT, src)
        pct = None
        if os.path.isfile(path):
            try:
                pct = {"go": pct_go, "lcov": pct_lcov, "jacoco": pct_jacoco}[kind](path)
            except Exception as e:  # noqa: BLE001 — report, don't crash
                print(f"[{name}] parse failed: {e}", file=sys.stderr)
        rows.append((name, pct, report))

    out_dir = os.path.join(ROOT, "reports", "coverage")
    os.makedirs(out_dir, exist_ok=True)
    out = os.path.join(out_dir, "index.html")

    cells = []
    for name, pct, report in rows:
        rel = os.path.relpath(os.path.join(ROOT, report), out_dir)
        if pct is None:
            cells.append(
                f'<tr><td>{name}</td><td class="na">not generated</td>'
                f'<td>run <code>make test-{name}-cover</code></td></tr>'
            )
        else:
            link = (f'<a href="{rel}">detailed report</a>'
                    if os.path.isfile(os.path.join(ROOT, report)) else "—")
            cells.append(
                f'<tr><td>{name}</td>'
                f'<td><span class="pct" style="background:{color(pct)}">{pct:.1f}%</span></td>'
                f'<td>{link}</td></tr>'
            )

    html = f"""<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<title>Tessera — coverage</title>
<style>
  body {{ font-family: system-ui, sans-serif; margin: 2rem auto; max-width: 640px; color: #222; }}
  h1 {{ font-size: 1.4rem; }}
  table {{ border-collapse: collapse; width: 100%; margin-top: 1rem; }}
  th, td {{ text-align: left; padding: .5rem .75rem; border-bottom: 1px solid #eee; }}
  .pct {{ color: #fff; padding: .1rem .5rem; border-radius: 4px; font-weight: 600; }}
  .na {{ color: #999; }}
  code {{ background: #f4f4f4; padding: .1rem .3rem; border-radius: 3px; }}
</style></head><body>
<h1>Tessera — test coverage</h1>
<p>Aggregated from each component's own coverage run. Backend/frontend count the
logic layer (see per-component notes); Android counts instructions of the whole
app (UI excluded from unit tests).</p>
<table>
<thead><tr><th>Component</th><th>Coverage</th><th>Report</th></tr></thead>
<tbody>
{chr(10).join(cells)}
</tbody></table>
</body></html>
"""
    with open(out, "w") as f:
        f.write(html)
    print(f"Coverage index → {out}")
    for name, pct, _ in rows:
        print(f"  {name}: {'%.1f%%' % pct if pct is not None else 'n/a'}")


if __name__ == "__main__":
    main()
