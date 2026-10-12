# Build the Tessera desktop Linux bundles (.deb + AppImage) against an OLD glibc so
# they run on a wide range of distros. glibc is backward- but NOT forward-compatible:
# a binary built on the host (WSL Ubuntu 24.04 / glibc 2.39) references GLIBC_2.3{7,8,9}
# symbols that older systems lack, so it dies with a glibc error on e.g. Astra Linux
# SE 1.8 (Debian 12 / glibc 2.36). Building here instead pins the floor at glibc 2.35.
#
# Base: Ubuntu 22.04 = glibc 2.35 AND webkit2gtk-4.1 (Tauri v2 / wry link against 4.1,
# not 4.0 — Debian 11 / Ubuntu 20.04 only have 4.0, so 22.04 is the oldest base that
# still builds Tauri v2). Result runs on any glibc >= 2.35: Astra SE 1.8, Debian 12,
# Ubuntu 22.04+, RHEL/Alma 9, Fedora, etc. The target still needs libwebkit2gtk-4.1-0
# installed (the AppImage does not bundle it — it is a system lib, same as the .deb's
# declared dependency).
FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

# Tauri v2 Linux build deps + AppImage/.deb bundler tooling. webkit2gtk-4.1-dev must
# be >= 2.40 to satisfy the crate's `v2_40` feature (Cargo.toml) — 22.04's security
# updates (jammy-updates/-security, in the default sources) carry a new-enough webkit.
# libfuse2 + patchelf are for linuxdeploy (AppImage); file/desktop-file-utils are its
# runtime probes.
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential curl wget file ca-certificates pkg-config \
      libwebkit2gtk-4.1-dev libgtk-3-dev librsvg2-dev \
      libayatana-appindicator3-dev libssl-dev \
      libfuse2 patchelf desktop-file-utils xdg-utils \
    && rm -rf /var/lib/apt/lists/*

# Rust toolchain + the Tauri CLI. Kept in a world-readable location so the build can
# run as root (the runner chowns artifacts back to the host user afterwards).
ENV RUSTUP_HOME=/opt/rust/rustup \
    CARGO_HOME=/opt/rust/cargo \
    PATH=/opt/rust/cargo/bin:$PATH
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \
      | sh -s -- -y --profile minimal --default-toolchain stable \
    && cargo install tauri-cli --version "^2" --locked

WORKDIR /work/desktop/src-tauri
