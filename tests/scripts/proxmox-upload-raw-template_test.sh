#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

fake_bin="${tmpdir}/bin"
remote_bin="${tmpdir}/remote-bin"
raw_image="${tmpdir}/merge-mode.raw"
remote_dir="${tmpdir}/remote"
mkdir -p "${fake_bin}" "${remote_bin}"
printf 'raw' >"${raw_image}"

cat >"${fake_bin}/qemu-img" <<'SH'
#!/usr/bin/env bash
printf 'local qemu-img %s\n' "$*" >>"${TEST_LOG}"
if [ "$1" = "resize" ]; then
  printf 'unexpected local qemu-img resize\n' >&2
  exit 66
fi
printf 'image: %s\nfile format: raw\nvirtual size: 8 GiB\n' "$2"
SH

cat >"${fake_bin}/parted" <<'SH'
#!/usr/bin/env bash
cat <<'OUT'
Number  Start   End     Size    File system  Name      Flags
 1      1MiB    64MiB   63MiB   fat32        EFI       boot, esp
 2      64MiB   1GiB    960MiB  ext4         RECOVERY
 3      1GiB    2GiB    1GiB    btrfs        ignition
 4      2GiB    8GiB    6GiB    btrfs        SYSTEM
OUT
SH

cat >"${fake_bin}/scp" <<'SH'
#!/usr/bin/env bash
printf 'scp %s\n' "$*" >>"${TEST_LOG}"
SH

cat >"${fake_bin}/ssh" <<'SH'
#!/usr/bin/env bash
host="$1"
shift
printf 'ssh %s %s\n' "${host}" "$*" >>"${TEST_LOG}"

if [ "${1:-}" != "bash" ]; then
  exit 0
fi

shift 3 # bash -s --
template_id="$1"
template_name="$2"
raw_image="$3"
storage="$4"
bridge="$5"
memory="$6"
cores="$7"
force="$8"
disk_size="${9:-}"

mkdir -p "$(dirname "${raw_image}")"
printf 'remote raw' >"${raw_image}"

PATH="${REMOTE_BIN}:${PATH}" bash -s -- \
  "${template_id}" "${template_name}" "${raw_image}" "${storage}" \
  "${bridge}" "${memory}" "${cores}" "${force}" "${disk_size}"
SH

cat >"${remote_bin}/qm" <<'SH'
#!/usr/bin/env bash
printf 'qm %s\n' "$*" >>"${TEST_LOG}"
case "$1" in
  status)
    exit 1
    ;;
  config)
    printf 'unused0: local-lvm:vm-9012-disk-1\n'
    ;;
esac
SH

chmod +x "${fake_bin}/qemu-img" "${fake_bin}/parted" "${fake_bin}/scp" "${fake_bin}/ssh" "${remote_bin}/qm"

export TEST_LOG="${tmpdir}/commands.log"
export REMOTE_BIN="${remote_bin}"

PATH="${fake_bin}:${PATH}" \
  "${repo_root}/scripts/proxmox-upload-raw-template.sh" \
    --host fake-pve \
    --template-id 9012 \
    --raw "${raw_image}" \
    --storage local-lvm \
    --remote-dir "${remote_dir}" \
    --force

if ! grep -qx 'local qemu-img info .*merge-mode.raw' "${TEST_LOG}"; then
  printf 'expected local qemu-img info only, got:\n' >&2
  cat "${TEST_LOG}" >&2
  exit 1
fi

if grep -q 'local qemu-img resize' "${TEST_LOG}"; then
  printf 'raw image was resized before upload:\n' >&2
  cat "${TEST_LOG}" >&2
  exit 1
fi

if ! grep -qx 'qm set 9012 --scsi0 local-lvm:vm-9012-disk-1' "${TEST_LOG}"; then
  printf 'expected imported disk to be attached as scsi0, got:\n' >&2
  cat "${TEST_LOG}" >&2
  exit 1
fi

if ! grep -qx 'qm resize 9012 scsi0 18G' "${TEST_LOG}"; then
  printf 'expected Proxmox-side scsi0 resize to default 18G, got:\n' >&2
  cat "${TEST_LOG}" >&2
  exit 1
fi
