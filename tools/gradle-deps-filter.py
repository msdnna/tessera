#!/usr/bin/env python3
"""Narrow a Gradle verification-metadata file to what the app actually ships (#2938).

`gradlew --write-verification-metadata` records every artifact the build resolved,
and a build resolves far more than the app's runtime classpath: the Gradle plugin
classpath (AGP, detekt, ktlint, google-services) drags in netty, jdom2, jose4j and
friends. Scanning that file as-is produced 94 findings, most of them CVEs in build
tooling that never enters the APK — a baseline nobody can triage, because "is this
shipped?" has to be answered by hand for every line.

So the runtime classpath is read back out of the `:app:dependencies` report that
the same build printed, and the metadata is filtered down to it. What drops out is
not ignored, it is *a different surface*: build-host supply chain, worth its own
decision, not something to smuggle into the app's dependency report.

Usage: gradle-deps-filter.py <verification-metadata.xml> <dependencies.log> <out.xml>
Prints "<kept> <total>" on stdout.
"""

import re
import sys
import xml.etree.ElementTree as ET

NS = "https://schema.gradle.org/dependency-verification"

# `+--- group:name:1.2.3`, possibly `-> 4.5.6` when a constraint or resolution
# strategy bumped it, possibly trailing `(c)` (a constraint, not a real edge) or
# `(*)` / `(n)` markers. The resolved version is what ends up on the classpath, so
# the arrow target wins when present.
DEP = re.compile(
    r"^[|\\+\- ]+--- ([A-Za-z0-9_.\-]+):([A-Za-z0-9_.\-]+):([^ \n]+)"
    r"(?: -> ([^ \n]+))?(.*)$"
)


def runtime_coordinates(dep_log: str) -> set[tuple[str, str, str]]:
    """group/name/version triples on the release runtime classpath."""
    coords: set[tuple[str, str, str]] = set()
    in_section = False
    for line in open(dep_log, encoding="utf-8", errors="replace"):
        if line.startswith("releaseRuntimeClasspath"):
            in_section = True
            continue
        if in_section:
            # The report separates configurations with a blank line.
            if not line.strip():
                in_section = False
                continue
            m = DEP.match(line)
            if not m:
                continue
            group, name, declared, resolved, tail = m.groups()
            # `(c)` marks a constraint line — a version rule, not a dependency that
            # is actually on the classpath. Its real edge is listed separately.
            if "(c)" in tail:
                continue
            coords.add((group, name, resolved or declared))
    return coords


def main() -> int:
    metadata, dep_log, out = sys.argv[1:4]
    coords = runtime_coordinates(dep_log)
    if not coords:
        print("0 0")
        return 1

    ET.register_namespace("", NS)
    tree = ET.parse(metadata)
    root = tree.getroot()
    components = root.find(f"{{{NS}}}components")
    if components is None:
        print("0 0")
        return 1

    total = kept = 0
    for component in list(components):
        total += 1
        triple = (
            component.get("group"),
            component.get("name"),
            component.get("version"),
        )
        if triple in coords:
            kept += 1
        else:
            components.remove(component)

    tree.write(out, encoding="utf-8", xml_declaration=True)
    print(f"{kept} {total}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
