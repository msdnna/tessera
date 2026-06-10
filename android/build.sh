#!/usr/bin/env bash
# Build the Tessera Android debug APK.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

if [ -f "$SCRIPT_DIR/local.env" ]; then
  # shellcheck disable=SC1091
  source "$SCRIPT_DIR/local.env"
fi

export ANDROID_HOME="${ANDROID_HOME:-$HOME/Android/Sdk}"
export JAVA_HOME="${JAVA_HOME:-/usr/lib/jvm/java-21-openjdk-amd64}"

PROXY_OPTS=""
if [ -n "${SOCKS_PROXY_HOST:-}" ]; then
  PROXY_OPTS="-DsocksProxyHost=$SOCKS_PROXY_HOST -DsocksProxyPort=${SOCKS_PROXY_PORT:-1080} -DsocksProxyVersion=5"
fi
export GRADLE_OPTS="${PROXY_OPTS:+$PROXY_OPTS }-Dorg.gradle.internal.http.socketTimeout=300000"

cd "$SCRIPT_DIR"
./gradlew assembleDebug

VERSION="$(tr -d '[:space:]' < "$SCRIPT_DIR/VERSION")"
APK_NAME="msdnna-tessera-v${VERSION}.apk"

find "$SCRIPT_DIR" -maxdepth 1 -name 'msdnna-tessera-v*.apk' -delete
cp app/build/outputs/apk/debug/app-debug.apk "./${APK_NAME}"
echo "APK: $SCRIPT_DIR/${APK_NAME}"

# Drop into the repo-level /apks store (bind-mounted into the frontend nginx
# container for in-app self-update), purging stale versions first.
APKS_DIR="$(cd "$SCRIPT_DIR/.." && pwd)/apks"
if [ -d "$APKS_DIR" ]; then
  find "$APKS_DIR" -maxdepth 1 -name 'msdnna-tessera-v*.apk' -delete
  cp "./${APK_NAME}" "$APKS_DIR/${APK_NAME}"
  # latest.json manifest for in-app self-update. versionCode mirrors
  # app/build.gradle: major*10000 + minor*100 + patch.
  IFS=. read -r VMAJ VMIN VPATCH <<EOF
$VERSION
EOF
  VCODE=$((10#$VMAJ * 10000 + 10#$VMIN * 100 + 10#$VPATCH))
  cat > "$APKS_DIR/latest.json" <<EOF
{"version":"$VERSION","versionCode":$VCODE,"apk":"${APK_NAME}","notes":""}
EOF
  echo "Served at: /apks/${APK_NAME} (latest.json versionCode=$VCODE)"
fi
