#!/usr/bin/env bash
set -euo pipefail

gradle_user_home="${GRADLE_USER_HOME:-$HOME/.gradle}"
version_state_file="$gradle_user_home/my-tv-version-code"
mkdir -p "$gradle_user_home"

description="$(git describe --tags --long --always)"
if [[ "$description" =~ ^v?([0-9]+)\.([0-9]+)\.([0-9]+)-([0-9]+)-g[[:xdigit:]]+$ ]]; then
  major="${BASH_REMATCH[1]}"
  minor="${BASH_REMATCH[2]}"
  patch="${BASH_REMATCH[3]}"
  commit_distance="${BASH_REMATCH[4]}"
elif [[ "$description" =~ ^v?([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
  major="${BASH_REMATCH[1]}"
  minor="${BASH_REMATCH[2]}"
  patch="${BASH_REMATCH[3]}"
  commit_distance=0
else
  echo "Unable to derive version from git description: $description" >&2
  exit 1
fi

version_name_base="$major.$minor.$patch"
version_code_base=$((major * 16777216 + minor * 65536 + patch * 256))
version_code=$((version_code_base + commit_distance))
if [[ -f "$version_state_file" ]]; then
  previous_version_code="$(cat "$version_state_file")"
  if [[ "$previous_version_code" =~ ^[0-9]+$ ]] && (( previous_version_code > version_code )); then
    version_code="$previous_version_code"
  fi
fi
version_code=$((version_code + 1))
if (( version_code > 2147483647 )); then
  echo "Android versionCode limit exceeded: $version_code" >&2
  exit 1
fi
build_number=$((version_code - version_code_base))
version_name="$version_name_base+$build_number"
printf '%s\n' "$version_code" > "$version_state_file"
echo "Building $version_name (versionCode $version_code)"

docker run --rm \
  --volume "$PWD:/workspace" \
  --volume "$gradle_user_home:/root/.gradle" \
  --volume "$HOME/.android:/root/.android" \
  --env ANDROID_HOME=/opt/android-sdk-linux \
  --env ANDROID_SDK_ROOT=/opt/android-sdk-linux \
  --workdir /workspace \
  ghcr.io/cirruslabs/android-sdk:34-ndk \
  bash -lc '
    mkdir -p /root/.android
    if [ -f /workspace/signing/debug.keystore ]; then
      if [ -f /root/.android/debug.keystore ]; then
        if ! cmp -s /workspace/signing/debug.keystore /root/.android/debug.keystore; then
          echo "Local and H255 signing keys do not match" >&2
          exit 1
        fi
      else
        install -m 600 /workspace/signing/debug.keystore /root/.android/debug.keystore
      fi
    fi
    if [ ! -f /root/.android/debug.keystore ]; then
      keytool -genkeypair -v \
        -keystore /root/.android/debug.keystore \
        -storepass android -alias androiddebugkey -keypass android \
        -dname "CN=Android Debug,O=Android,C=US" \
        -keyalg RSA -keysize 2048 -validity 10000
    fi
    ./gradlew "$@"
  ' _ "${1:-assembleDebug}" -PtvVersionCode="$version_code" -PtvVersionName="$version_name"
