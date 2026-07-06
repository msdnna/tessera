# Tessera Desktop (Tauri v2)

Native desktop app (Windows + Linux) wrapping the shared Vue frontend
(`../frontend`). The frontend detects Tauri at runtime and points the API/WS at
the login-configured server (default: production `tessera.msdnna.website`) — see
`frontend/src/utils/serverBase.js`. There is **no separate JS package**: the
desktop app consumes the frontend's dev server and build output directly.

Independently versioned via `desktop/VERSION` (semver), like the other
components. Bump with `make bump-desktop BUMP=minor` and add a `CHANGELOG.md`
entry (see the root `CLAUDE.md` conventions).

## Toolchain

- **Rust** (stable) via [rustup](https://rustup.rs). Cargo bin on `PATH`.
- **Tauri CLI v2**: `cargo install tauri-cli --version '^2' --locked`
  (invoked as `cargo tauri …`).

### Linux (built on the dev box)

System `-dev` packages (Debian/Ubuntu):

```
libwebkit2gtk-4.1-dev  libgtk-3-dev  libayatana-appindicator3-dev
librsvg2-dev  patchelf  build-essential  curl  wget  file
```

Build: `make desktop` (or `cargo tauri build`) → AppImage + `.deb` under
`src-tauri/target/release/bundle/`. The `.deb` declares the appindicator/webkit
runtime deps; AppImage users need `libayatana-appindicator3` installed for the
system tray (Phase 2).

**AppImage under WSL:** the AppImage step runs `linuxdeploy`, itself an AppImage
that needs FUSE — this fails on many WSL setups (`failed to run linuxdeploy`),
even with `APPIMAGE_EXTRACT_AND_RUN=1`. The `.deb` builds fine there. Build the
AppImage (the Linux self-update target) on a native Linux host or CI runner.
`build-desktop-release.sh` tolerates a failed AppImage step and still publishes
the `.deb`.

### Windows (built natively on Windows — not from WSL)

Cross-compiling Windows installers from Linux is impractical; build on Windows:

1. Install **Rust (MSVC)**: rustup with the `x86_64-pc-windows-msvc` target.
2. Install **Visual Studio C++ Build Tools** (the MSVC linker).
3. **WebView2 Runtime** — preinstalled on current Windows 10/11; otherwise install
   the Evergreen runtime.
4. `cargo install tauri-cli --version '^2' --locked`.
5. From `desktop/src-tauri`: `cargo tauri build` → NSIS `.exe` installer under
   `target/release/bundle/nsis/`.

## Develop

```
make dev-desktop        # cargo tauri dev — starts Vite :5174 + the app,
                        # talking to the local backend via the proxy
```

## Release + self-update

`tools/build-desktop-release.sh` builds the Linux bundles, signs the updater
artifacts, and writes the updater manifest into `desktop-dist/` for serving at
`https://tessera.msdnna.website/desktop/`. Windows artifacts are built on Windows
and dropped into the same directory.

- The minisign **private** key lives outside the repo at
  `~/.tessera/tessera-desktop-updater.key` (password-less); the **public** key is
  baked into `src-tauri/tauri.conf.json` (`plugins.updater.pubkey`). Signing reads
  `TAURI_SIGNING_PRIVATE_KEY` / `TAURI_SIGNING_PRIVATE_KEY_PATH`. Keep the private
  key safe — losing it breaks updates (same discipline as the Android keystore).
