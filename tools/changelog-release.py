#!/usr/bin/env python3
"""Собрать changelog-фрагменты в общий CHANGELOG и бампнуть версии затронутых компонентов.

Запускается НА develop (после merge фича-веток), не в фича-ветке. Фича-ветки кладут
записи в changelog.d/<component>/<task#>[-slug].md и НЕ трогают CHANGELOG.md/VERSION —
так исчезают вечные конфликты в этих файлах.

Формат фрагмента (см. changelog.d/README.md):

    ---
    bump: patch            # patch|minor|major (необязательно; выводится из типа коммита)
    ---
    - **fix(web): краткое описание (#2612).**
      Опциональные детали с отступом — как в существующих записях CHANGELOG.

`component` = имя подпапки (backend|frontend|android|desktop|mcp). Имя файла завязано на
номер задачи → фрагменты разных задач физически не конфликтуют при merge.

Использование:
    tools/changelog-release.py                 # собрать все компоненты с фрагментами
    tools/changelog-release.py --only backend,frontend
    tools/changelog-release.py --dry-run       # показать, что будет сделано, ничего не меняя
    tools/changelog-release.py --date 2026-08-07
"""
import argparse
import datetime
import os
import re
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
FRAG_DIR = os.path.join(ROOT, "changelog.d")

# component -> (VERSION path, changelog file, heading style, anchor)
# heading style: "bracket" => "### [x.y.z] — DATE" ; "plain" => "## x.y.z — DATE"
# anchor: строка секции, ПОСЛЕ которой (сверху) вставляем новую версию.
COMPONENTS = {
    "backend":  {"version": "backend/VERSION",  "changelog": "CHANGELOG.md",         "style": "bracket", "anchor": "## backend"},
    "frontend": {"version": "frontend/VERSION", "changelog": "CHANGELOG.md",         "style": "bracket", "anchor": "## frontend"},
    "desktop":  {"version": "desktop/VERSION",  "changelog": "CHANGELOG.md",         "style": "bracket", "anchor": "## desktop"},
    "mcp":      {"version": "mcp/VERSION",      "changelog": "CHANGELOG.md",         "style": "bracket", "anchor": "## mcp"},
    "android":  {"version": "android/VERSION",  "changelog": "android/CHANGELOG.md", "style": "plain",   "anchor": "## Unreleased"},
}

BUMP_RANK = {"patch": 0, "minor": 1, "major": 2}
RANK_BUMP = {0: "patch", 1: "minor", 2: "major"}


def infer_bump_from_body(body: str) -> str:
    """Вывести уровень semver из conventional-commit префикса первой строки тела."""
    m = re.search(r"\*\*([a-z]+)(\([^)]*\))?(!)?:", body)
    if not m:
        return "patch"
    typ, _scope, bang = m.group(1), m.group(2), m.group(3)
    if bang:
        return "major"
    if typ == "feat":
        return "minor"
    return "patch"  # fix/perf/refactor/docs/chore/test/style/...


def parse_fragment(path: str):
    """Вернуть (bump, body). Frontmatter (--- ... ---) необязателен; bump выводится из тела."""
    raw = open(path, encoding="utf-8").read()
    bump = None
    body = raw
    if raw.lstrip().startswith("---"):
        # срезать первый frontmatter-блок
        s = raw.lstrip()
        end = s.find("\n---", 3)
        if end != -1:
            fm = s[3:end]
            body = s[end + 4:]
            for line in fm.splitlines():
                line = line.strip()
                if line.lower().startswith("bump:"):
                    val = line.split(":", 1)[1].strip().lower()
                    if val in BUMP_RANK:
                        bump = val
    body = body.strip("\n")
    if bump is None:
        bump = infer_bump_from_body(body)
    return bump, body


def task_num(filename: str) -> int:
    m = re.match(r"(\d+)", filename)
    return int(m.group(1)) if m else 10**9


def current_version(comp: str) -> str:
    p = os.path.join(ROOT, COMPONENTS[comp]["version"])
    return open(p, encoding="utf-8").read().strip()


def bumped_version(cur: str, level: str) -> str:
    major, minor, patch = (int(x) for x in cur.split("."))
    if level == "major":
        major, minor, patch = major + 1, 0, 0
    elif level == "minor":
        minor, patch = minor + 1, 0
    else:
        patch += 1
    return f"{major}.{minor}.{patch}"


def heading(comp: str, ver: str, date: str) -> str:
    if COMPONENTS[comp]["style"] == "bracket":
        return f"### [{ver}] — {date}"
    return f"## {ver} — {date}"


def render_entry(comp: str, ver: str, date: str, bodies: list) -> str:
    return heading(comp, ver, date) + "\n" + "\n".join(bodies) + "\n"


def insert_entry(comp: str, entry: str) -> str:
    """Вставить entry в нужную секцию changelog-файла; вернуть новый текст файла."""
    cfg = COMPONENTS[comp]
    path = os.path.join(ROOT, cfg["changelog"])
    lines = open(path, encoding="utf-8").read().splitlines(keepends=False)
    anchor = cfg["anchor"]

    # найти строку-якорь секции
    try:
        ai = next(i for i, ln in enumerate(lines) if ln.strip() == anchor)
    except StopIteration:
        # секции нет — создать в конце файла
        block = ["", anchor, ""] + entry.splitlines()
        return "\n".join(lines + block).rstrip("\n") + "\n"

    # для bracket-стиля вставляем перед первым '### [' внутри секции;
    # для plain (android) — сразу после '## Unreleased'.
    insert_at = ai + 1
    if cfg["style"] == "bracket":
        j = ai + 1
        while j < len(lines):
            s = lines[j].strip()
            if s.startswith("### ["):
                break
            if s.startswith("## ") and s != anchor:  # следующая секция — вставить до неё
                break
            j += 1
        insert_at = j
    else:
        # после '## Unreleased' (+ возможной пустой строки)
        insert_at = ai + 1

    new_block = entry.splitlines() + [""]
    # аккуратно с пустыми строками: гарантируем пустую строку-разделитель перед блоком
    out = lines[:insert_at]
    if out and out[-1].strip() != "":
        out.append("")
    out += new_block
    out += lines[insert_at:]
    text = "\n".join(out)
    if not text.endswith("\n"):
        text += "\n"
    return text


def main():
    ap = argparse.ArgumentParser(description="Собрать changelog-фрагменты и бампнуть версии.")
    ap.add_argument("--only", help="список компонентов через запятую (по умолчанию все с фрагментами)")
    ap.add_argument("--date", help="дата релиза YYYY-MM-DD (по умолчанию сегодня)")
    ap.add_argument("--dry-run", action="store_true", help="показать план, ничего не менять")
    args = ap.parse_args()

    date = args.date or datetime.date.today().isoformat()
    only = set(c.strip() for c in args.only.split(",")) if args.only else None

    if not os.path.isdir(FRAG_DIR):
        print("changelog.d/ не найден — нечего собирать.", file=sys.stderr)
        return 1

    any_work = False
    for comp, cfg in COMPONENTS.items():
        if only and comp not in only:
            continue
        cdir = os.path.join(FRAG_DIR, comp)
        if not os.path.isdir(cdir):
            continue
        frags = sorted(
            (f for f in os.listdir(cdir) if f.endswith(".md") and not f.startswith(".") and f != "README.md"),
            key=task_num,
        )
        if not frags:
            continue
        any_work = True

        bodies, levels, paths = [], [], []
        for f in frags:
            fp = os.path.join(cdir, f)
            bump, body = parse_fragment(fp)
            bodies.append(body)
            levels.append(bump)
            paths.append(fp)

        level = RANK_BUMP[max(BUMP_RANK[b] for b in levels)]
        cur = current_version(comp)
        new = bumped_version(cur, level)
        entry = render_entry(comp, new, date, bodies)

        print(f"\n=== {comp}: {cur} → {new}  ({level}, {len(frags)} фрагм.) ===")
        print(entry.rstrip())
        print(f"[файлы] {cfg['changelog']}  ← вставка под «{cfg['anchor']}»")
        print(f"[удалить] {', '.join(os.path.relpath(p, ROOT) for p in paths)}")

        if args.dry_run:
            continue

        # 1) вставить в changelog
        new_text = insert_entry(comp, entry)
        with open(os.path.join(ROOT, cfg["changelog"]), "w", encoding="utf-8") as fh:
            fh.write(new_text)
        # 2) бампнуть VERSION через канонический скрипт (у desktop он же правит Cargo.toml)
        subprocess.run([os.path.join(ROOT, "tools", "bump-version.sh"), comp, level], check=True, cwd=ROOT)
        # 3) удалить собранные фрагменты
        for p in paths:
            os.remove(p)

    if not any_work:
        print("Фрагментов нет — ничего не собрано.")
    elif args.dry_run:
        print("\n(dry-run: изменения не применены)")
    else:
        print("\nГотово. Проверь diff, затем закоммить как chore(release).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
