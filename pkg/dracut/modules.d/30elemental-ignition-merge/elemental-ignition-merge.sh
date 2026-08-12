#!/usr/bin/env bash
set -eu

ROOT="${ELEMENTAL_IGNITION_MERGE_ROOT:-}"
MARKER="${ROOT}/ignition/elemental-merge"
BASE="${ROOT}/ignition/config.ign"
IGNITION_DIR="${ROOT}/usr/lib/ignition"
BASE_DIR="${IGNITION_DIR}/base.d"
TARGET="${BASE_DIR}/10-elemental-base.ign"
LOG_TO_STDERR="${ELEMENTAL_IGNITION_MERGE_LOG_TO_STDERR:-}"
MOUNTED_IGNITION=""

log() {
    local msg="elemental-ignition-merge: $*"
    if [ -n "$LOG_TO_STDERR" ]; then
        printf '%s\n' "$msg" >&2
    else
        printf '%s\n' "$msg" >/dev/kmsg 2>/dev/null || true
    fi
}

remount_usr_rw() {
    if [ -w "${ROOT}/usr" ]; then
        return 0
    fi

    mount -o rw,remount "${ROOT}/usr" 2>/dev/null || true
}

mount_ignition_media() {
    if [ -e "$MARKER" ] || [ -n "$ROOT" ]; then
        return 0
    fi

    if [ ! -e /dev/disk/by-label/ignition ]; then
        log "ignition media device not found"
        return 0
    fi

    mkdir -p /ignition
    if mountpoint -q /ignition; then
        log "ignition media already mounted at /ignition"
        return 0
    fi

    if mount -o ro /dev/disk/by-label/ignition /ignition; then
        MOUNTED_IGNITION=1
        log "mounted ignition media at /ignition"
    else
        log "failed mounting ignition media at /ignition"
    fi
}

cleanup() {
if [ -n "$MOUNTED_IGNITION" ]; then
umount /ignition 2>/dev/null || true
fi
}

select_merge_source() {
local candidate

for candidate in "${ROOT}/ignition" "${ROOT}/ignition/ignition"; do
if [ -e "${candidate}/elemental-merge" ] || [ -s "${candidate}/config.ign" ]; then
MARKER="${candidate}/elemental-merge"
BASE="${candidate}/config.ign"
return 0
fi
done
}

main() {
trap cleanup EXIT
mount_ignition_media

select_merge_source

if [ ! -e "$MARKER" ]; then
log "merge marker not found; no merge staging needed"
return 0
    fi

    if [ ! -s "$BASE" ]; then
        log "missing embedded base ignition: $BASE"
        return 1
    fi

    remount_usr_rw
    mkdir -p "$BASE_DIR"
    cp "$BASE" "$TARGET"
    chmod 0644 "$TARGET"
    rm -f "${IGNITION_DIR}/user.ign"
    mkdir -p "${ROOT}/run/ignition" 2>/dev/null || true
    cp "$BASE" "${ROOT}/run/ignition/10-elemental-base.ign" 2>/dev/null || true
    log "staged Elemental base Ignition at $TARGET"
}

main "$@"
