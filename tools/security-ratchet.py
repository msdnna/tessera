#!/usr/bin/env python3
"""Ratchet for the SAST/SCA scans (#2939).

`tools/security-scan.sh` reports findings; this decides whether the report is
acceptable. The tree has ~100 findings today and triaging them is its own body
of work, so a hard "zero findings" gate would paint develop red on day one and
get switched off within a week. Instead the current state is frozen in
`tools/security-baseline.json` and the rule is the same one the Russian-literal
ratchet uses in the frontend: the list may shrink, never grow.

    check   compare the SARIF reports of this run against the baseline
    update  rewrite the baseline from this run (only the tools that ran)

A finding is identified by (tool, rule, where) and counted, not by line number:
line numbers move with every edit above them, and a baseline that churns on
unrelated edits is a baseline nobody keeps current. "Where" is the file for
code findings and the *package name* for dependency findings — deliberately
without the version, so bumping a still-vulnerable dependency does not read as
a new problem.

Two classes of tool, because they fail differently:

  code  gosec, detekt, gitleaks — deterministic given the tree. A new finding
        here is something this commit introduced → hard failure. A finding that
        disappeared is work that must be ticked off, exactly like the i18n
        ratchet, otherwise the baseline slowly stops meaning anything.
  deps  govulncheck, osv-scanner — read an advisory feed that changes daily. A
        new finding here usually means "somebody published a CVE", not "this
        branch regressed", so by default it is reported loudly and does not
        fail the build; SCAN_RATCHET_DEPS=fail makes it fail too. For the same
        reason a disappeared advisory is not an error: advisories get withdrawn
        and rescoped without anyone touching the repo.
"""

import argparse
import json
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

# Tools whose findings depend on an advisory feed rather than only on the tree.
DEPS_TOOLS = ("govulncheck", "osv")

# Package name out of an osv-scanner message: "Package 'glib@0.18.5' is
# vulnerable to 'RUSTSEC-2024-0429'". The version is dropped on purpose (see
# module docstring).
OSV_PKG = re.compile(r"Package '([^']+?)(?:@[^'@]*)?' is vulnerable")


def tool_name(path):
    """Report file → tool key. osv-scanner runs three times over three
    ecosystems and writes osv-frontend/osv-android/osv-desktop; keeping them
    apart means a frontend scan never looks like it wiped the desktop entries."""
    return os.path.basename(path)[: -len(".sarif")] if path.endswith(".sarif") else os.path.basename(path)


def is_deps(tool):
    return tool.split("-")[0] in DEPS_TOOLS


def rel_path(uri):
    """SARIF artifact URI → something stable across machines.

    osv-scanner writes absolute file:// URIs, so an unnormalised baseline would
    match only on the machine that produced it — and never in CI.
    """
    if not uri:
        return "?"
    if uri.startswith("file://"):
        uri = uri[len("file://"):]
    if os.path.isabs(uri):
        try:
            rel = os.path.relpath(uri, ROOT)
        except ValueError:
            return uri
        # Outside the repo (a GOROOT path, say) — keep it absolute rather than
        # emit a ../../.. that encodes where the checkout happens to live.
        uri = rel if not rel.startswith("..") else uri
    return uri


def fingerprint(tool, result):
    """(rule, where) for one SARIF result — no line numbers, see docstring."""
    rule = result.get("ruleId") or "?"
    text = (result.get("message") or {}).get("text") or ""

    if tool.startswith("osv"):
        # Every finding of a lockfile scan points at the same lockfile, so the
        # file is no discriminator; the package is.
        match = OSV_PKG.search(text)
        if match:
            return "%s %s" % (rule, match.group(1))

    locations = result.get("locations") or []
    uri = ""
    if locations:
        uri = ((locations[0].get("physicalLocation") or {}).get("artifactLocation") or {}).get("uri", "")
    return "%s %s" % (rule, rel_path(uri))


def read_report(path):
    """Report file → {fingerprint: count}. Raises on anything unreadable: a
    scan whose report we cannot parse must not read as "everything is fixed"."""
    with open(path) as fh:
        sarif = json.load(fh)
    tool = tool_name(path)
    counts = {}
    for run in sarif.get("runs", []):
        for result in run.get("results", []):
            key = fingerprint(tool, result)
            counts[key] = counts.get(key, 0) + 1
    return counts


def load_baseline(path):
    if not os.path.exists(path):
        return {}
    with open(path) as fh:
        data = json.load(fh)
    return data.get("tools", {})


def write_baseline(path, tools):
    payload = {
        "note": (
            "Frozen state of the SAST/SCA scans — see docs/security-scanning.md. "
            "Counts per (rule, file-or-package); regenerate with "
            "`make scan-baseline-update` after triaging, never by hand."
        ),
        "tools": dict(sorted((t, dict(sorted(e.items()))) for t, e in tools.items())),
    }
    with open(path, "w") as fh:
        json.dump(payload, fh, indent=2, ensure_ascii=False, sort_keys=False)
        fh.write("\n")


def diff(baseline, current):
    """(added, resolved) as lists of (key, count) — counts, so a fourteenth
    G115 in a file that already had thirteen still registers."""
    added, resolved = [], []
    for key, count in sorted(current.items()):
        was = baseline.get(key, 0)
        if count > was:
            added.append((key, count - was))
    for key, count in sorted(baseline.items()):
        now = current.get(key, 0)
        if now < count:
            resolved.append((key, count - now))
    return added, resolved


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("mode", choices=("check", "update"))
    parser.add_argument("reports", nargs="*", help="SARIF files produced by this run")
    parser.add_argument("--baseline", default=os.path.join(ROOT, "tools", "security-baseline.json"))
    parser.add_argument(
        "--deps",
        choices=("warn", "fail"),
        default=os.environ.get("SCAN_RATCHET_DEPS", "warn"),
        help="what new dependency advisories do (default: warn)",
    )
    # intermixed, not parse_args: with a `nargs="*"` positional argparse cannot
    # split `check --baseline X a.sarif` and rejects the file list outright.
    args = parser.parse_intermixed_args()

    if not args.reports:
        print("security-ratchet: no reports given — nothing to compare", file=sys.stderr)
        return 2

    baseline = load_baseline(args.baseline)
    current = {}
    for path in args.reports:
        try:
            current[tool_name(path)] = read_report(path)
        except (OSError, json.JSONDecodeError) as exc:
            print("security-ratchet: cannot read %s (%s)" % (path, exc), file=sys.stderr)
            return 2

    if args.mode == "update":
        # Merge, do not replace: `make scan-backend scan-baseline-update` must
        # not wipe the android entries just because android did not run.
        merged = dict(baseline)
        merged.update(current)
        write_baseline(args.baseline, merged)
        total = sum(sum(e.values()) for e in current.values())
        print("  baseline updated: %d finding(s) across %s → %s"
              % (total, ", ".join(sorted(current)), os.path.relpath(args.baseline, ROOT)))
        return 0

    failed = False
    warned = 0
    tightenable = []
    for tool in sorted(current):
        added, resolved = diff(baseline.get(tool, {}), current[tool])
        soft = is_deps(tool) and args.deps == "warn"
        if added:
            label = "new advisories (feed moves on its own — not failing, SCAN_RATCHET_DEPS=fail to)" if soft else "NEW"
            print("  %s %s: %s" % ("!" if soft else "✗", tool, label), file=sys.stderr)
            for key, delta in added:
                print("      +%d  %s" % (delta, key), file=sys.stderr)
            if soft:
                warned += len(added)
            else:
                failed = True
        if resolved:
            tightenable.append((tool, resolved, is_deps(tool)))

    for tool, resolved, deps in tightenable:
        # A shrunken code baseline is a hard failure on purpose: leaving stale
        # entries in means the file stops describing the tree, and then nobody
        # trusts it enough to act on the next real regression.
        print("  %s %s: %d finding(s) gone — %s" % (
            "·" if deps else "✗", tool, sum(d for _, d in resolved),
            "will drop out on the next update" if deps else "run `make scan-baseline-update` and commit",
        ), file=sys.stderr)
        for key, delta in resolved:
            print("      -%d  %s" % (delta, key), file=sys.stderr)
        if not deps:
            failed = True

    if failed:
        print("  ratchet: FAILED (see above)", file=sys.stderr)
        return 1
    rel = os.path.relpath(args.baseline, ROOT)
    if warned:
        print("  ratchet: ok, but %d new advisory/ies since %s — triage them" % (warned, rel))
    else:
        print("  ratchet: ok (no new findings over %s)" % rel)
    return 0


if __name__ == "__main__":
    sys.exit(main())
