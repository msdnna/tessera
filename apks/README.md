# Android self-update store

The release build (`tools/build-android-release.sh`) drops the signed APK and a
`latest.json` manifest here; the frontend nginx serves them at `/apks/`, and the
Android app self-updates from `<server>/apks/latest.json`. APKs are gitignored;
this dir is kept so the build script and the nginx bind-mount have a target.
