#!/usr/bin/env bash
set -euo pipefail

# Build an Elemental raw image using Rig's Docker-volume pattern:
# host config -> Docker volume -> Linux builder container -> Docker volume -> host output.
#
# This avoids relying on macOS filesystem or disk tooling while still exposing the
# final raw image through a normal host directory.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

CONFIG_DIR="${CONFIG_DIR:-$REPO_ROOT/examples/elemental/customize/dynamic-rke}"
OUTPUT_DIR="${OUTPUT_DIR:-/tmp/elemental-merge-raw/out}"
VOLUME="${VOLUME:-elemental-merge-build-vol}"
RAW_NAME="${RAW_NAME:-merge-mode.raw}"
BUILDER_IMAGE="${BUILDER_IMAGE:-docker.io/ravan/elemental:3.0-merge-mode}"
BUILDER_CONTEXT="${BUILDER_CONTEXT:-$REPO_ROOT}"
BUILDER_TARGET="${BUILDER_TARGET:-runner-elemental3}"
BUILD_BUILDER_IMAGE="${BUILD_BUILDER_IMAGE:-1}"
CLEAN_BUILDER_IMAGE="${CLEAN_BUILDER_IMAGE:-1}"
BUILDER_PULL_POLICY="${BUILDER_PULL_POLICY:-never}"
HELPER_IMAGE="${HELPER_IMAGE:-registry.opensuse.org/opensuse/tumbleweed:latest}"
PLATFORM="${PLATFORM:-linux/amd64}"
NETWORK="${NETWORK:-host}"
MODE="${MODE:-merge}"
RESET_VOLUME="${RESET_VOLUME:-1}"
COPY_TO_HOST="${COPY_TO_HOST:-1}"
REQUIRE_LABELS="${REQUIRE_LABELS:-EFI RECOVERY ignition SYSTEM}"

log() {
  printf '[merge-raw] %s\n' "$*"
}

require_dir() {
  if [ ! -d "$1" ]; then
    printf 'missing directory: %s\n' "$1" >&2
    exit 1
  fi
}

require_file() {
  if [ ! -f "$1" ]; then
    printf 'missing file: %s\n' "$1" >&2
    exit 1
  fi
}

require_dir "$CONFIG_DIR"
require_file "${CONFIG_DIR}/install.yaml"
require_file "${CONFIG_DIR}/release.yaml"

if [ "$RESET_VOLUME" = "1" ]; then
  log "resetting Docker volume ${VOLUME}"
  if docker volume inspect "$VOLUME" >/dev/null 2>&1; then
    if ! docker volume rm "$VOLUME" >/dev/null 2>&1; then
      printf 'failed to remove docker volume %s (still in use?)\n' "$VOLUME" >&2
      exit 1
    fi
  fi
fi

docker volume create "$VOLUME" >/dev/null

log "copying config into Docker volume"
docker run --rm \
  -v "${VOLUME}:/config" \
  -v "${CONFIG_DIR}:/host-config:ro" \
  "$HELPER_IMAGE" \
  bash -lc '
    set -euo pipefail
    shopt -s dotglob nullglob
    rm -rf /config/*
  tar -C /host-config -cf - . | tar -C /config -xf -
'

if [ "$CLEAN_BUILDER_IMAGE" = "1" ]; then
log "removing stale builder image ${BUILDER_IMAGE}"
docker image rm -f "$BUILDER_IMAGE" >/dev/null 2>&1 || true
fi

if [ "$BUILD_BUILDER_IMAGE" = "1" ]; then
log "building builder image ${BUILDER_IMAGE}"
docker buildx build \
--platform "$PLATFORM" \
--target "$BUILDER_TARGET" \
-t "$BUILDER_IMAGE" \
--load \
"$BUILDER_CONTEXT"
fi

log "running elemental3 customize in ${BUILDER_IMAGE}"
docker run --rm \
--pull "$BUILDER_PULL_POLICY" \
--platform "$PLATFORM" \
--network "$NETWORK" \
  --privileged \
  -v "${VOLUME}:/config" \
  -v /var/run/docker.sock:/var/run/docker.sock \
  "$BUILDER_IMAGE" \
  customize \
    --type raw \
    --mode "$MODE" \
    --platform "$PLATFORM" \
    --config-dir /config \
    --output "/config/${RAW_NAME}"

log "inspecting raw partition layout inside Linux"
docker run --rm \
  --platform "$PLATFORM" \
  --privileged \
  --device-cgroup-rule='b 7:* rmw' \
  --device-cgroup-rule='b 259:* rmw' \
  -v "${VOLUME}:/config" \
  "$HELPER_IMAGE" \
  bash -lc '
    set -euo pipefail
    zypper --non-interactive install --no-recommends util-linux util-linux-systemd libblkid1 parted >/dev/null
    test -f "/config/'"${RAW_NAME}"'"
    parted -s "/config/'"${RAW_NAME}"'" print
    fdisk -l "/config/'"${RAW_NAME}"'"

    # Ensure loop nodes exist (the build container may not pre-populate /dev/loop*).
    for i in 0 1 2 3 4 5 6 7; do
      [ -b /dev/loop$i ] || mknod -m 660 /dev/loop$i b 7 $i 2>/dev/null || true
    done

    # Detach any stale loop bindings from previous runs in the same kernel.
    losetup -D 2>/dev/null || true

    lo=$(losetup -fP --show "/config/'"${RAW_NAME}"'")
    trap '"'"'losetup -d "$lo" 2>/dev/null || true'"'"' EXIT
    sleep 2
    partprobe "$lo" 2>/dev/null || true
    # Make sure partition device nodes exist; some hosts do not auto-create them.
    for n in 1 2 3 4; do
      [ -b "${lo}p${n}" ] || mknod -m 660 "${lo}p${n}" b 259 "$((n - 1))" 2>/dev/null || true
    done
    ls -la "${lo}"* 2>&1 || true

    # Probe each partition directly with blkid -p (bypasses the udev cache,
    # which is not populated inside an ephemeral privileged container).
    echo "=== filesystem labels (blkid -p) ==="
    fs_labels=""
    for part in ${lo}p1 ${lo}p2 ${lo}p3 ${lo}p4; do
      echo "$part:"
      label=""
      while IFS= read -r line; do
        printf "  %s\n" "$line"
        case "$line" in
          LABEL=*) label="${line#LABEL=}";;
        esac
      done < <(blkid -p -o export "$part" 2>/dev/null || true)
      [ -n "$label" ] && fs_labels="$fs_labels $label"
    done

    missing=0
    for label in '"${REQUIRE_LABELS}"'; do
      if ! printf "%s\n" $fs_labels | grep -Fxq "$label"; then
        echo "missing expected filesystem LABEL: $label" >&2
        missing=1
      fi
    done
    if [ "$missing" != "0" ]; then
      exit 1
    fi
    echo "all required filesystem labels present: '"${REQUIRE_LABELS}"'"
  '

if [ "$COPY_TO_HOST" = "1" ]; then
  mkdir -p "$OUTPUT_DIR"
  log "copying raw image to ${OUTPUT_DIR}/${RAW_NAME}"
  docker run --rm \
    -v "${VOLUME}:/config:ro" \
    -v "${OUTPUT_DIR}:/host-out" \
    "$HELPER_IMAGE" \
    bash -lc 'cp "/config/'"${RAW_NAME}"'" "/host-out/'"${RAW_NAME}"'" && ls -lh "/host-out/'"${RAW_NAME}"'"'
fi

log "done"
