#!/usr/bin/env bash
set -euo pipefail

ANDROID_SDK_CACHE="${ANDROID_SDK_CACHE:-$HOME/.cache/my-tv/android-sdk}"
mkdir -p "$ANDROID_SDK_CACHE/ndk" "$ANDROID_SDK_CACHE/cmake"

docker run --rm \
  --volume "$PWD:/workspace" \
  --volume "${GRADLE_USER_HOME:-$HOME/.gradle}:/root/.gradle" \
  --volume "$ANDROID_SDK_CACHE/ndk:/opt/android-sdk-linux/ndk" \
  --volume "$ANDROID_SDK_CACHE/cmake:/opt/android-sdk-linux/cmake" \
  --env ANDROID_HOME=/opt/android-sdk-linux \
  --env ANDROID_SDK_ROOT=/opt/android-sdk-linux \
  --workdir /workspace \
  ghcr.io/cirruslabs/android-sdk:34-ndk \
  ./gradlew -PIS_SO_BUILD=false "${1:-assembleDebug}"
