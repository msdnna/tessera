#!/usr/bin/env bash
# Build a signed Tessera release APK.
#
# Required env vars (set in environment or android/local.env):
#   ANDROID_KEYSTORE_FILE     — absolute path to .jks / .keystore file
#   ANDROID_KEYSTORE_PASSWORD — keystore password
#   ANDROID_KEY_ALIAS         — key alias inside the keystore
#   ANDROID_KEY_PASSWORD      — key password (often same as keystore password)
#
# Generate a keystore (first time only):
#   keytool -genkey -v -keystore ~/tessera.jks -alias tessera \
#     -keyalg RSA -keysize 2048 -validity 10000
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ANDROID_DIR="$SCRIPT_DIR/../android"

if [ -f "$ANDROID_DIR/local.env" ]; then
  # shellcheck disable=SC1091
  source "$ANDROID_DIR/local.env"
fi

for var in ANDROID_KEYSTORE_FILE ANDROID_KEYSTORE_PASSWORD ANDROID_KEY_ALIAS ANDROID_KEY_PASSWORD; do
  if [ -z "${!var:-}" ]; then
    echo "Error: $var is not set"
    echo "Set it in the environment or in android/local.env"
    exit 1
  fi
done

if [ ! -f "$ANDROID_KEYSTORE_FILE" ]; then
  echo "Error: keystore not found: $ANDROID_KEYSTORE_FILE"
  exit 1
fi

export ANDROID_HOME="${ANDROID_HOME:-$HOME/Android/Sdk}"
export JAVA_HOME="${JAVA_HOME:-/usr/lib/jvm/java-21-openjdk-amd64}"
export ANDROID_KEYSTORE_FILE ANDROID_KEYSTORE_PASSWORD ANDROID_KEY_ALIAS ANDROID_KEY_PASSWORD

PROXY_OPTS=""
if [ -n "${SOCKS_PROXY_HOST:-}" ]; then
  PROXY_OPTS="-DsocksProxyHost=$SOCKS_PROXY_HOST -DsocksProxyPort=${SOCKS_PROXY_PORT:-1080} -DsocksProxyVersion=5"
fi
export GRADLE_OPTS="${PROXY_OPTS:+$PROXY_OPTS }-Dorg.gradle.internal.http.socketTimeout=300000"

cd "$ANDROID_DIR"
./gradlew assembleRelease

VERSION="$(tr -d '[:space:]' < "$ANDROID_DIR/VERSION")"
OUTPUT="$ANDROID_DIR/app/build/outputs/apk/release/app-release.apk"
DEST="$ANDROID_DIR/msdnna-tessera-v${VERSION}.apk"

find "$ANDROID_DIR" -maxdepth 1 -name 'msdnna-tessera-v*.apk' -delete
cp "$OUTPUT" "$DEST"
echo "APK: $DEST"

# Drop into the repo-level /apks store (bind-mounted into the frontend nginx
# container for in-app self-update). Stale APKs purged so only the current
# version is reachable over HTTP.
APKS_DIR="$(cd "$ANDROID_DIR/.." && pwd)/apks"
if [ -d "$APKS_DIR" ]; then
  find "$APKS_DIR" -maxdepth 1 -name 'msdnna-tessera-v*.apk' -delete
  cp "$DEST" "$APKS_DIR/msdnna-tessera-v${VERSION}.apk"
  # latest.json manifest the app polls for self-update. versionCode mirrors
  # app/build.gradle: major*10000 + minor*100 + patch.
  IFS=. read -r VMAJ VMIN VPATCH <<EOF
$VERSION
EOF
  VCODE=$((10#$VMAJ * 10000 + 10#$VMIN * 100 + 10#$VPATCH))
  cat > "$APKS_DIR/latest.json" <<EOF
{"version":"$VERSION","versionCode":$VCODE,"apk":"msdnna-tessera-v${VERSION}.apk","notes":""}
EOF
  echo "Served at: /apks/msdnna-tessera-v${VERSION}.apk (latest.json versionCode=$VCODE)"
fi
