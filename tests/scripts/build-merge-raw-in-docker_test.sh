#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

config_dir="${tmpdir}/config"
fake_bin="${tmpdir}/bin"
docker_log="${tmpdir}/docker.log"
mkdir -p "${config_dir}" "${fake_bin}"

cat >"${config_dir}/install.yaml" <<'YAML'
schema: v0
bootloader: grub
raw:
  diskSize: 1G
YAML

cat >"${config_dir}/release.yaml" <<'YAML'
name: suse-product
manifestURI: file://./suse-product-manifest.yaml
components:
  kubernetes: {}
YAML

cat >"${fake_bin}/docker" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"${DOCKER_LOG}"
case "$*" in
  "volume create "*)
    exit 0
    ;;
  image\ rm\ -f\ *)
    exit 0
    ;;
  buildx\ build\ --platform\ linux/amd64\ --target\ runner-elemental3\ -t\ test.local/elemental:stale\ --load\ *)
    exit 0
    ;;
  run\ --rm\ -v*)
    case "$*" in
      *"${EXPECTED_CONFIG_DIR}:/host-config:ro"*)
        exit 0
        ;;
    esac
    ;;
esac
echo "docker reached: $*" >&2
exit 64
SH
chmod +x "${fake_bin}/docker"

set +e
output="$(
  PATH="${fake_bin}:${PATH}" \
CONFIG_DIR="${config_dir}" \
EXPECTED_CONFIG_DIR="${config_dir}" \
RESET_VOLUME=0 \
COPY_TO_HOST=0 \
BUILDER_IMAGE="test.local/elemental:stale" \
DOCKER_LOG="${docker_log}" \
"${repo_root}/scripts/build-merge-raw-in-docker.sh" 2>&1
)"
status=$?
set -e

if [ "${status}" -ne 64 ]; then
  printf 'expected script to reach fake docker and exit 64, got %s\n%s\n' "${status}" "${output}" >&2
  exit 1
fi

if ! grep -q 'docker reached:' <<<"${output}"; then
  printf 'expected script to reach docker, got:\n%s\n' "${output}" >&2
  exit 1
fi

if grep -q 'core-manifest.yaml' <<<"${output}"; then
  printf 'script still requires core-manifest.yaml:\n%s\n' "${output}" >&2
  exit 1
fi

if ! grep -Fxq 'image rm -f test.local/elemental:stale' "${docker_log}"; then
  printf 'expected script to remove stale builder image before raw build, got docker calls:\n' >&2
  cat "${docker_log}" >&2
  exit 1
fi

image_rm_line="$(grep -nFx 'image rm -f test.local/elemental:stale' "${docker_log}" | cut -d: -f1 || true)"
builder_run_line="$(grep -nF 'test.local/elemental:stale customize --type raw' "${docker_log}" | cut -d: -f1 || true)"
if [ -z "${builder_run_line}" ] || [ "${image_rm_line}" -ge "${builder_run_line}" ]; then
  printf 'expected stale builder image removal before builder run, got docker calls:\n' >&2
  cat "${docker_log}" >&2
  exit 1
fi

build_line="$(grep -nF 'buildx build --platform linux/amd64 --target runner-elemental3 -t test.local/elemental:stale --load ' "${docker_log}" | cut -d: -f1 || true)"
if [ -z "${build_line}" ] || [ "${build_line}" -le "${image_rm_line}" ] || [ "${build_line}" -ge "${builder_run_line}" ]; then
  printf 'expected builder image rebuild after cleanup and before builder run, got docker calls:\n' >&2
  cat "${docker_log}" >&2
  exit 1
fi

if ! grep -Fq 'run --rm --pull never --platform linux/amd64 --network host --privileged' "${docker_log}"; then
  printf 'expected builder run to disable remote image pulls, got docker calls:\n' >&2
  cat "${docker_log}" >&2
  exit 1
fi

default_config_dir="${repo_root}/examples/elemental/customize/dynamic-rke"
: >"${docker_log}"

set +e
output="$(
  PATH="${fake_bin}:${PATH}" \
  EXPECTED_CONFIG_DIR="${default_config_dir}" \
  RESET_VOLUME=0 \
  COPY_TO_HOST=0 \
  BUILDER_IMAGE="test.local/elemental:stale" \
  DOCKER_LOG="${docker_log}" \
  "${repo_root}/scripts/build-merge-raw-in-docker.sh" 2>&1
)"
status=$?
set -e

if [ "${status}" -ne 64 ]; then
  printf 'expected default-config run to reach fake docker and exit 64, got %s\n%s\n' "${status}" "${output}" >&2
  exit 1
fi

if ! grep -Fq "${default_config_dir}:/host-config:ro" "${docker_log}"; then
  printf 'expected default CONFIG_DIR to use dynamic-rke example, got docker calls:\n' >&2
  cat "${docker_log}" >&2
  exit 1
fi
