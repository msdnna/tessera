#!/usr/bin/env python3
"""Product release of Tessera — «Tessera 2.0» over the components' own semver.

Two layers of versions (docs/versioning.md):

  * components — backend, frontend, android, desktop, mcp — keep their own semver
    in <component>/VERSION, bumped on develop by `make changelog-release`. They
    are the technical contract (API ↔ clients, self-update, release workflows
    tagged `api/v…`, `web/v…`).
  * the product — what a person installs and talks about: «Tessera 2.0». Its
    version lives in release.json at the root with a snapshot of the component
    versions it ships. MAJOR = a product generation (a new interface/brand: v2 is
    the redesign), MINOR = a release train (a wave of features), PATCH = a fix
    release of the same train.

This tool cuts a product release on develop, after `make changelog-release`:

  1. bumps the product version (or sets it), snapshots every component VERSION;
  2. marks the «Что нового» entries of the train (web versions after the previous
     release's frontend, up to the current one) with `product: '<version>'`;
  3. writes the release notes docs/releases/v<version>.md — the user-facing
     «Что нового» of the train plus the component table — which the GitHub
     Release of tag v<version> publishes (.github/workflows/release-product.yml).

Usage:
  tools/product-release.py [--bump major|minor|patch | --set X.Y.Z]
                           [--since-web X.Y.Z] [--dry-run]

`--since-web` overrides where the train starts (exclusive) — for the first
release, which has no previous snapshot to start from.
"""
from __future__ import annotations

import argparse
import datetime as dt
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
RELEASE = ROOT / "release.json"
COMPONENTS = ["backend", "frontend", "android", "desktop", "mcp"]
WHATS_NEW = ROOT / "frontend/src/data/whatsNew.js"
WHATS_NEW_RU = ROOT / "frontend/src/locales/ru/whatsNew.json"
NOTES_DIR = ROOT / "docs/releases"
SEMVER = re.compile(r"^(\d+)\.(\d+)\.(\d+)$")


def parse(v: str) -> tuple[int, int, int]:
    m = SEMVER.match(v.strip())
    if not m:
        sys.exit(f"not a semver version: {v!r}")
    return tuple(int(x) for x in m.groups())  # type: ignore[return-value]


def fmt(t: tuple[int, int, int]) -> str:
    return ".".join(str(x) for x in t)


def bump(v: str, level: str) -> str:
    major, minor, patch = parse(v)
    if level == "major":
        return fmt((major + 1, 0, 0))
    if level == "minor":
        return fmt((major, minor + 1, 0))
    return fmt((major, minor, patch + 1))


def short(v: str) -> str:
    """2.0.0 → 2.0, 2.0.1 → 2.0.1 — how people say it."""
    major, minor, patch = parse(v)
    return f"{major}.{minor}" if patch == 0 else f"{major}.{minor}.{patch}"


def component_versions() -> dict[str, str]:
    return {c: (ROOT / c / "VERSION").read_text().strip() for c in COMPONENTS}


def whats_new_entries() -> list[dict]:
    """(version, titleKey, itemKeys) of every entry in whatsNew.js, newest first."""
    src = WHATS_NEW.read_text()
    out = []
    for m in re.finditer(r"\{\s*version:\s*'([^']+)'(.*?)\n  \}", src, re.S):
        body = m.group(2)
        title = re.search(r"titleKey:\s*'([^']+)'", body)
        items = re.findall(r"'(whatsNew\.[^']+\.item\d+)'", body)
        out.append({"version": m.group(1), "titleKey": title.group(1) if title else "", "items": items})
    return out


def mark_whats_new(versions: set[str], product: str, dry: bool) -> int:
    """Add `product: '<v>'` after `version: '<web>'` of the train's entries."""
    src = WHATS_NEW.read_text()
    # A re-run replaces its own marks instead of stacking them.
    src = re.sub(r"(\n\s*version: '[^']+',)\n\s*product: '[^']*',", r"\1", src)
    n = 0

    def repl(m: re.Match) -> str:
        nonlocal n
        if m.group(3) not in versions:
            return m.group(0)
        n += 1
        return f"{m.group(1)}\n{m.group(2)}product: '{product}',"

    src = re.sub(r"(\n(\s*)version: '([^']+)',)", repl, src)
    if not dry:
        WHATS_NEW.write_text(src)
    return n


def locale_text(key: str, ru: dict) -> str:
    node = ru
    for part in key.split(".")[1:]:  # whatsNew.<entry>.<item>
        node = node.get(part, {}) if isinstance(node, dict) else {}
    text = node if isinstance(node, str) else key
    return text.replace("{'@'}", "@")


def notes(product: str, name: str, date: str, comps: dict, prev: dict | None, train: list[dict]) -> str:
    ru = json.loads(WHATS_NEW_RU.read_text())
    lines = [f"# Tessera {short(product)} — {date}", "", f"{name} · версия продукта {product}.", ""]
    if train:
        lines += ["## Что нового", ""]
        for e in sorted(train, key=lambda e: parse(e["version"])):
            title = locale_text(e["titleKey"], ru) if e["titleKey"] else e["version"]
            lines.append(f"### {title}")
            lines.append("")
            for it in e["items"]:
                lines.append(f"- {locale_text(it, ru)}")
            lines.append("")
    lines += ["## Компоненты", "", "| Компонент | Версия | Было |", "|---|---|---|"]
    for c in COMPONENTS:
        was = (prev or {}).get(c, "—")
        lines.append(f"| {c} | {comps[c]} | {was} |")
    lines += [
        "",
        "Подробности по компонентам — `CHANGELOG.md` (backend, frontend, desktop, mcp) и "
        "`android/CHANGELOG.md`. Версии компонентов — semver, версия продукта — "
        "`release.json` (см. `docs/versioning.md`).",
        "",
    ]
    return "\n".join(lines)


def main() -> None:
    ap = argparse.ArgumentParser()
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--bump", choices=["major", "minor", "patch"], default="minor")
    g.add_argument("--set", dest="set_version")
    ap.add_argument("--since-web", help="start of the train (exclusive web version)")
    ap.add_argument("--name", help="product name (default: «Tessera <major>»)")
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--show", action="store_true", help="print the current product release and exit")
    a = ap.parse_args()
    if a.show:
        r = json.loads(RELEASE.read_text())
        print(f"product:  Tessera {short(r['product'])} ({r['product']}, {r['date']})")
        return

    prev = json.loads(RELEASE.read_text()) if RELEASE.exists() else None
    if a.set_version:
        new = fmt(parse(a.set_version))
    elif prev:
        new = bump(prev["product"], a.bump)
    else:
        sys.exit("no release.json yet — the first release needs --set X.Y.Z")
    if prev and parse(new) <= parse(prev["product"]):
        sys.exit(f"{new} is not after the current product release {prev['product']}")

    comps = component_versions()
    since = a.since_web or (prev or {}).get("components", {}).get("frontend")
    if not since:
        sys.exit("where does the train start? pass --since-web for the first release")
    web_now = parse(comps["frontend"])
    train = [e for e in whats_new_entries() if parse(since) < parse(e["version"]) <= web_now]

    date = dt.date.today().isoformat()
    name = a.name or f"Tessera {parse(new)[0]}"
    release = {"product": new, "name": name, "date": date, "components": comps}
    body = notes(new, name, date, comps, (prev or {}).get("components"), train)
    note_path = NOTES_DIR / f"v{new}.md"

    print(f"Tessera {short(new)} ({new}) — {name}, {date}")
    for c in COMPONENTS:
        was = (prev or {}).get("components", {}).get(c, "—")
        print(f"  {c:9} {was:>9} → {comps[c]}")
    print(f"  «Что нового» поезда: {', '.join(e['version'] for e in train) or '—'}")
    print(f"  заметки: {note_path.relative_to(ROOT)}")
    if a.dry_run:
        print("\n(dry-run: ничего не записано)")
        return

    RELEASE.write_text(json.dumps(release, ensure_ascii=False, indent=2) + "\n")
    NOTES_DIR.mkdir(parents=True, exist_ok=True)
    note_path.write_text(body)
    marked = mark_whats_new({e["version"] for e in train}, new, False)
    print(f"\nГотово: release.json, {note_path.relative_to(ROOT)}, «Что нового» помечено: {marked}.")
    print(f"Коммит: chore(release): Tessera {short(new)}; тег (сам): git tag v{new}")


if __name__ == "__main__":
    main()
