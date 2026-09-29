#!/usr/bin/env bash
set -euo pipefail

docker run --rm \
  --volume "$PWD:/workspace" \
  --volume "${GRADLE_USER_HOME:-$HOME/.gradle}:/root/.gradle" \
  --env ANDROID_HOME=/opt/android-sdk-linux \
  --env ANDROID_SDK_ROOT=/opt/android-sdk-linux \
  --workdir /workspace \
  ghcr.io/cirruslabs/android-sdk:34-ndk \
  ./gradlew "${1:-assembleDebug}"
