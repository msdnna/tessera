#!/usr/bin/env python3
"""Aggregate the per-component coverage reports into one styled HTML page.

Reads whatever coverage artifacts exist (produced by the `make test-*-cover`
targets and by CI) and writes `reports/coverage/index.html` — an overview with a
coverage % + bar per component and a link to each component's own detailed HTML
report. Styling mirrors budget-go's aggregate-reports page (cards, badges, bars).

Sources (all optional; a missing one is shown as "not generated"):
  backend  backend/coverage.out            → `go tool cover -func` total
  mcp      mcp/coverage.out                → `go tool cover -func` total
  web      frontend/coverage/lcov.info     → LH/LF line totals
  android  android/app/build/reports/jacoco/jacocoTestReport/jacocoTestReport.xml

Usage: python3 tools/coverage-report.py [repo-root]
"""
import os
import subprocess
import sys
import time
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


# (component, kind, source path, detailed-report link, coverage metric, note)
COMPONENTS = [
    ("backend", "go", "backend/coverage.out", "backend/cover.html",
     "statements", "logic layer (generated sqlc / cmd / main excluded)"),
    ("frontend", "lcov", "frontend/coverage/lcov.info", "frontend/coverage/index.html",
     "lines", "logic layer: utils / stores / composables / api / router"),
    ("android", "jacoco",
     "android/app/build/reports/jacoco/jacocoTestReport/jacocoTestReport.xml",
     "android/app/build/reports/jacoco/jacocoTestReport/html/index.html",
     "instructions", "whole app (Compose UI not unit-tested)"),
    ("mcp", "go", "mcp/coverage.out", "mcp/cover.html",
     "statements", "REST client + tool handlers (stdio glue excluded)"),
]

PARSERS = {"go": pct_go, "lcov": pct_lcov, "jacoco": pct_jacoco}

CSS = """
:root { --accent: #7c5cff; }
body { font: 14px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  background: #fafafa; color: #222; margin: 0; padding: 28px; max-width: 900px;
  margin-left: auto; margin-right: auto; }
h1 { margin: 0 0 4px; font-size: 22px; }
.meta { color: #888; font-size: 12px; margin-bottom: 22px; }
.meta code { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
.overall { background: linear-gradient(135deg, #6d5fe0, #7c6cff 50%, #9183ff);
  color: #fff; border-radius: 12px; padding: 22px 26px; margin-bottom: 20px;
  display: flex; align-items: baseline; gap: 16px; }
.overall .big { font-size: 40px; font-weight: 700; font-variant-numeric: tabular-nums; }
.overall .sub { font-size: 13px; opacity: .85; }
.section { background: #fff; border: 1px solid #e6e6e6; border-radius: 10px;
  padding: 16px 20px; margin-bottom: 14px; }
.head { display: flex; align-items: center; gap: 12px; }
.head h2 { margin: 0; font-size: 17px; flex: 0 0 auto; width: 110px; }
.badge { display: inline-block; padding: 3px 12px; border-radius: 11px;
  font-size: 13px; font-weight: 700; font-variant-numeric: tabular-nums; color: #fff; }
.bar { background: #eee; border-radius: 4px; height: 8px; flex: 1 1 auto;
  overflow: hidden; }
.bar > i { display: block; height: 8px; }
.na { color: #999; font-style: italic; flex: 1 1 auto; }
.note { color: #777; font-size: 12px; margin: 6px 0 0 122px; }
.note a { color: var(--accent); text-decoration: none; }
.note a:hover { text-decoration: underline; }
"""


def color(p):
    return ("#2ea043" if p >= 80 else "#a4a61d" if p >= 60 else "#c98800"
            if p >= 40 else "#e0533d" if p >= 20 else "#c12626")


def main():
    rows = []
    for name, kind, src, report, metric, note in COMPONENTS:
        path = os.path.join(ROOT, src)
        pct = None
        if os.path.isfile(path):
            try:
                pct = PARSERS[kind](path)
            except Exception as e:  # noqa: BLE001 — report, don't crash
                print(f"[{name}] parse failed: {e}", file=sys.stderr)
        rows.append((name, pct, report, metric, note))

    measured = [p for _, p, _, _, _ in rows if p is not None]
    overall = sum(measured) / len(measured) if measured else 0.0

    out_dir = os.path.join(ROOT, "reports", "coverage")
    os.makedirs(out_dir, exist_ok=True)
    out = os.path.join(out_dir, "index.html")

    parts = [
        '<!DOCTYPE html><html lang="en"><head><meta charset="utf-8">',
        '<meta name="viewport" content="width=device-width, initial-scale=1">',
        '<title>Tessera — coverage</title>',
        f"<style>{CSS}</style></head><body>",
        "<h1>Tessera — test coverage</h1>",
        f'<div class="meta">generated {time.strftime("%Y-%m-%d %H:%M:%S")} · '
        f'<code>make coverage-report</code> · averaged across '
        f'{len(measured)} measured component(s)</div>',
        '<div class="overall">'
        f'<span class="big">{overall:.1f}%</span>'
        '<span class="sub">mean component coverage<br>'
        'per-component metrics differ (statements / lines / instructions)</span>'
        '</div>',
    ]

    for name, pct, report, metric, note in rows:
        rel = os.path.relpath(os.path.join(ROOT, report), out_dir)
        has_report = os.path.isfile(os.path.join(ROOT, report))
        parts.append('<div class="section"><div class="head">')
        parts.append(f"<h2>{name}</h2>")
        if pct is None:
            parts.append(
                '<span class="na">not generated — run '
                f'<code>make test-{name}-cover</code></span></div>'
                f'<div class="note">{note}</div></div>'
            )
            continue
        c = color(pct)
        parts.append(f'<span class="badge" style="background:{c}">{pct:.1f}%</span>')
        parts.append(f'<span class="bar"><i style="width:{min(100, pct):.1f}%;background:{c}"></i></span>')
        parts.append("</div>")
        link = f' · <a href="{rel}">detailed report ↗</a>' if has_report else ""
        parts.append(f'<div class="note">{metric} · {note}{link}</div>')
        parts.append("</div>")

    parts.append("</body></html>")
    with open(out, "w") as f:
        f.write("\n".join(parts))

    print(f"Coverage index → {out}")
    print(f"  overall (mean): {overall:.1f}%")
    for name, pct, _, _, _ in rows:
        print(f"  {name}: {'%.1f%%' % pct if pct is not None else 'n/a'}")


if __name__ == "__main__":
    main()
