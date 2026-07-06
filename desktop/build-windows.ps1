<#
.SYNOPSIS
  Build the Tessera Windows installer (NSIS .exe) natively on Windows.

.DESCRIPTION
  The native Windows toolchain (cargo/MSVC/NSIS) CANNOT build over a UNC / \\wsl$
  path, and the repo's sh scripts + Makefile aren't available outside WSL. So:

    1. Get a NATIVE Windows checkout (on C:\ etc., not \\wsl.localhost\...):
         git clone \\wsl.localhost\<distro>\home\msdnna\GolandProjects\tessera C:\src\tessera
       Re-sync later with:  git -C C:\src\tessera pull

    2. Build the frontend in WSL (fast, that's where the toolchain lives) and
       copy the result into the Windows checkout (frontend\dist is gitignored,
       so `git pull` won't bring it):
         # in WSL:      corepack yarn --cwd frontend build
         # then (PowerShell), mirror it over:
         robocopy \\wsl.localhost\<distro>\home\msdnna\GolandProjects\tessera\frontend\dist C:\src\tessera\frontend\dist /MIR

    3. Run this script from the native checkout:
         pwsh C:\src\tessera\desktop\build-windows.ps1

  Pass -BuildFrontend to build the frontend here instead (needs Node 22 + a prior
  `corepack yarn install` in frontend\).

.PARAMETER BuildFrontend
  Build the frontend on Windows via the normal beforeBuildCommand (needs Node +
  corepack). Default: reuse the existing frontend\dist and skip the frontend build.

.NOTES
  Self-update signing: copy the minisign key from WSL
  (~/.tessera/tessera-desktop-updater.key) and set, before running:
    $env:TAURI_SIGNING_PRIVATE_KEY = 'C:\path\to\tessera-desktop-updater.key'
    $env:TAURI_SIGNING_PRIVATE_KEY_PASSWORD = ''
  The matching public key is already baked into tauri.conf.json. Without these a
  signed installer can't be produced (updater artifacts require the key).

  After a successful build, copy target\release\bundle\nsis\*-setup.exe AND its
  *.sig into the WSL repo's desktop-dist\ folder, then run
  `make desktop-release` (or tools/build-desktop-release.sh) in WSL to fold the
  Windows entry into latest.json.
#>
[CmdletBinding()]
param(
  [switch]$BuildFrontend
)

$ErrorActionPreference = 'Stop'

$desktopDir = $PSScriptRoot
$repoRoot   = Split-Path -Parent $desktopDir
$srcTauri   = Join-Path $desktopDir 'src-tauri'
$distIndex  = Join-Path $repoRoot 'frontend\dist\index.html'

# cargo/MSVC/Tauri choke on UNC paths — refuse to build from \\wsl$ / \\wsl.localhost.
if ($desktopDir -like '\\*') {
  throw "Refusing to build from a UNC/WSL path:`n  $desktopDir`nClone to a native drive (e.g. C:\src\tessera) and run from there."
}

if (-not (Get-Command cargo -ErrorAction SilentlyContinue)) {
  throw "cargo not found on PATH. Install Rust (MSVC toolchain) via rustup."
}

Push-Location $srcTauri
try {
  if ($BuildFrontend) {
    Write-Host "==> Building frontend on Windows (beforeBuildCommand)..." -ForegroundColor Cyan
    cargo tauri build
  }
  else {
    if (-not (Test-Path $distIndex)) {
      throw "No prebuilt frontend at frontend\dist\index.html.`nBuild it in WSL (corepack yarn --cwd frontend build) and copy frontend\dist here, or re-run with -BuildFrontend (needs Node)."
    }
    Write-Host "==> Reusing existing frontend\dist (skipping frontend rebuild)" -ForegroundColor Cyan
    # Override beforeBuildCommand to a no-op so Tauri doesn't try to run yarn.
    $noFront = Join-Path $env:TEMP 'tessera-nofront.conf.json'
    '{"build":{"beforeBuildCommand":""}}' | Set-Content -Path $noFront -Encoding utf8
    cargo tauri build --config $noFront
  }
}
finally {
  Pop-Location
}

$nsisDir = Join-Path $srcTauri 'target\release\bundle\nsis'
Write-Host "`n==> Installer(s):" -ForegroundColor Green
Get-ChildItem $nsisDir -Filter *.exe -ErrorAction SilentlyContinue | ForEach-Object { Write-Host "   $($_.FullName)" }
Write-Host ''
Write-Host 'Copy the *-setup.exe and its *.sig into the WSL repo''s desktop-dist\, then run'
Write-Host '"make desktop-release" in WSL to publish + update latest.json.'
