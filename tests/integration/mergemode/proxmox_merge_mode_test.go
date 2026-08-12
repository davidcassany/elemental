//go:build integration

package mergemode

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const providerIgnitionPayload = `{
  "ignition": { "version": "3.5.0" },
  "storage": {
    "files": [
      {
        "path": "/var/lib/elemental/provider-ignition-marker",
        "mode": 420,
        "overwrite": true,
        "contents": { "source": "data:,provider-ok" }
      },
      {
        "path": "/var/lib/elemental/k8s-dynamic/userdata.yaml",
        "mode": 420,
        "overwrite": true,
        "contents": {
          "source": "data:,hostname%3A%20merge-node.example.com%0Arke2%3A%0A%20%20type%3A%20server%0A%20%20init%3A%20true%0A%20%20token%3A%20merge-token%0Aelemental%3A%0A%20%20kubernetes%3A%0A%20%20%20%20deployResources%3A%20true%0A"
        }
      }
    ]
  }
}`

var expectedMergeModeGuestFiles = []string{
	"/var/lib/elemental/base-ignition-marker",
	"/var/lib/elemental/provider-ignition-marker",
	"/var/lib/elemental/k8s-dynamic/userdata.yaml",
	"/var/lib/elemental/k8s-dynamic/status.yaml",
	"/var/lib/elemental/kubernetes/init.yaml",
}

func ssh(t *testing.T, host string, script string) string {
	t.Helper()

	cmd := exec.Command("ssh", host, "bash", "-se")
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ssh %s failed: %v\n%s", host, err, out)
	}

	return string(out)
}

func scpToHost(t *testing.T, source string, host string, target string) {
	t.Helper()

	cmd := exec.Command("scp", "-r", source, host+":"+target)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("scp %s to %s:%s failed: %v\n%s", source, host, target, err, out)
	}
}

func TestProxmoxMergeModeContract(t *testing.T) {
	env := requireProxmoxEnv(t)

	var payload map[string]any
	if err := json.Unmarshal([]byte(providerIgnitionPayload), &payload); err != nil {
		t.Fatalf("provider ignition payload must be valid JSON: %v", err)
	}
	if len(expectedMergeModeGuestFiles) == 0 {
		t.Fatal("merge-mode integration assertions must name expected guest files")
	}

	vmName := "elemental-merge-mode-" + env.VMID
	remoteDir := "/var/lib/vz/template/iso/elemental-merge-mode-" + env.VMID
	platformModuleDir := remoteDir + "/31elemental-platform-resolver"
	mergeModuleDir := remoteDir + "/30elemental-ignition-merge"
	remoteElemental3ctl := remoteDir + "/elemental3ctl"
	workImage := remoteDir + "/source-with-merge.qcow2"
	ignitionISO := "/var/lib/vz/template/iso/elemental-merge-mode-" + env.VMID + "-ignition.iso"
	cidataISO := "/var/lib/vz/template/iso/vm-" + env.VMID + "-cidata.iso"
	platformISO := "/var/lib/vz/template/iso/elemental-merge-mode-" + env.VMID + "-platform.iso"
	localPlatformModuleDir := filepath.Join("..", "..", "..", "pkg", "dracut", "modules.d", "29elemental-platform-resolver")
	localMergeModuleDir := filepath.Join("..", "..", "..", "pkg", "dracut", "modules.d", "30elemental-ignition-merge")
	localElemental3ctl := filepath.Join("..", "..", "..", "build", "elemental3ctl")

	cleanup := fmt.Sprintf(`
set -euo pipefail
qm stop %[1]s >/dev/null 2>&1 || true
qm destroy %[1]s --purge >/dev/null 2>&1 || true
rm -rf %[2]q
rm -f %[3]q %[4]q %[5]q
`, env.VMID, remoteDir, platformISO, cidataISO, ignitionISO)
	if os.Getenv("ELEMENTAL_PROXMOX_KEEP_VM") == "" {
		defer ssh(t, env.Host, cleanup)
	} else {
		t.Logf("preserving Proxmox VM %s and remote dir %s for debugging", env.VMID, remoteDir)
	}

	ssh(t, env.Host, fmt.Sprintf("set -euo pipefail\nrm -rf %[1]q\nmkdir -p %[1]q\n", remoteDir))
	scpToHost(t, localPlatformModuleDir, env.Host, remoteDir+"/")
	scpToHost(t, localMergeModuleDir, env.Host, remoteDir+"/")
	scpToHost(t, localElemental3ctl, env.Host, remoteElemental3ctl)

	setup := fmt.Sprintf(`
set -euo pipefail
remote_dir=%[1]q
ignition_iso=%[2]q
cidata_iso=%[3]q
platform_iso=%[4]q
vmid=%[5]q
vm_name=%[6]q
source_image=%[7]q
storage=%[8]q
work_image=%[9]q
platform_module_dir=%[10]q
merge_module_dir=%[11]q
elemental3ctl=%[12]q

cd "$remote_dir"
rm -rf cidata embedded platform initrd-work
mkdir -p cidata embedded/ignition platform initrd-work/out initrd-work/extract

make_iso() {
  local label="$1"
  local output="$2"
  local source="$3"
  if command -v xorriso >/dev/null 2>&1; then
    xorriso -as mkisofs -volid "$label" -joliet -rock -output "$output" "$source" >/dev/null
  elif command -v genisoimage >/dev/null 2>&1; then
    genisoimage -volid "$label" -joliet -rock -output "$output" "$source" >/dev/null
  elif command -v mkisofs >/dev/null 2>&1; then
    mkisofs -volid "$label" -joliet -rock -output "$output" "$source" >/dev/null
  else
    echo "xorriso, genisoimage, or mkisofs is required" >&2
    exit 1
  fi
}

if command -v grub2-editenv >/dev/null 2>&1; then
  grub2-editenv platform/grubenv create
  grub2-editenv platform/grubenv set platform_id=proxmoxve
else
  printf 'platform_id=proxmoxve\n' > platform/grubenv
fi

cat > cidata/user-data <<'IGN'
%[13]s
IGN
cat > cidata/meta-data <<META
instance-id: vm-$vmid
local-hostname: $vm_name
META
cat > embedded/ignition/elemental-merge <<'MARKER'
enabled
MARKER
cat > embedded/ignition/config.ign <<'BASE'
{"ignition":{"version":"3.5.0"},"storage":{"files":[{"path":"/var/lib/elemental/base-ignition-marker","mode":420,"overwrite":true,"contents":{"source":"data:,base-ok"}}]},"systemd":{"units":[{"name":"elemental-k8s-dynamic.service","enabled":true,"contents":"[Unit]\nDescription=Elemental K8s Dynamic Configuration\nBefore=k8s-config-installer.service\nConditionPathExists=!/run/elemental/k8s-dynamic-applied\n\n[Service]\nType=oneshot\nRemainAfterExit=yes\nTimeoutSec=120\nExecStartPre=/bin/mkdir -p /run/elemental\nExecStart=/usr/bin/elemental3ctl k8s-dynamic apply --config /var/lib/elemental/k8s-dynamic/userdata.yaml\nExecStartPost=/bin/touch /run/elemental/k8s-dynamic-applied\n\n[Install]\nWantedBy=multi-user.target\n"}]}}
BASE
make_iso ignition "$ignition_iso" embedded
make_iso cidata "$cidata_iso" cidata
make_iso PLATFORM_HINT "$platform_iso" platform

cp "$source_image" "$work_image"
guestfish --rw -a "$work_image" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot debug sh 'btrfs property set /sysroot ro false'
virt-copy-in -a "$work_image" "$platform_module_dir" /usr/lib/dracut/modules.d
virt-copy-in -a "$work_image" "$merge_module_dir" /usr/lib/dracut/modules.d
virt-copy-in -a "$work_image" "$elemental3ctl" /usr/bin
guestfish --rw -a "$work_image" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot chmod 0755 /usr/bin/elemental3ctl
guestfish --rw -a "$work_image" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot rm-f /etc/systemd/system/default.target.wants/jeos-firstboot.service
guestfish --rw -a "$work_image" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot rm-f /etc/systemd/system/sysinit.target.wants/systemd-firstboot.service
guestfish --rw -a "$work_image" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot ln-sf /dev/null /etc/systemd/system/jeos-firstboot.service
guestfish --rw -a "$work_image" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot ln-sf /dev/null /etc/systemd/system/systemd-firstboot.service

virt-copy-out -a "$work_image" /boot/initrd-7.0.12-1-default initrd-work/out
(cd initrd-work/extract && zstd -dc ../out/initrd-7.0.12-1-default | cpio -id --quiet)
install -D -m 0644 "$platform_module_dir/elemental-platform-resolver.service" initrd-work/extract/etc/systemd/system/elemental-platform-resolver.service
mkdir -p initrd-work/extract/etc/systemd/system/initrd.target.wants
ln -sf ../elemental-platform-resolver.service initrd-work/extract/etc/systemd/system/initrd.target.wants/elemental-platform-resolver.service
install -D -m 0755 "$platform_module_dir/elemental-platform-resolver.sh" initrd-work/extract/usr/libexec/elemental-platform-resolver.sh
install -D -m 0644 "$merge_module_dir/elemental-ignition-merge.service" initrd-work/extract/etc/systemd/system/elemental-ignition-merge.service
ln -sf ../elemental-ignition-merge.service initrd-work/extract/etc/systemd/system/initrd.target.wants/elemental-ignition-merge.service
install -D -m 0755 "$merge_module_dir/elemental-ignition-merge.sh" initrd-work/extract/usr/lib/elemental/elemental-ignition-merge.sh
(cd initrd-work/extract && find . -print0 | cpio --null -o --format=newc --quiet | zstd -19 -T0 > ../out/initrd-7.0.12-1-default)
virt-copy-in -a "$work_image" initrd-work/out/initrd-7.0.12-1-default /boot
guestfish --rw -a "$work_image" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot debug sh 'btrfs property set /sysroot ro true'

qm stop "$vmid" >/dev/null 2>&1 || true
qm destroy "$vmid" --purge >/dev/null 2>&1 || true
qm create "$vmid" --name "$vm_name" --memory 2048 --cores 2 --net0 virtio,bridge=vmbr0 --serial0 socket --vga serial0 --bios ovmf --scsihw virtio-scsi-pci
qm importdisk "$vmid" "$work_image" "$storage" >/dev/null
qm set "$vmid" --scsi0 "$storage:vm-$vmid-disk-0"
qm set "$vmid" --efidisk0 "$storage:1,efitype=4m,pre-enrolled-keys=0"
qm set "$vmid" --scsi1 "local:iso/$(basename "$cidata_iso"),media=cdrom"
qm set "$vmid" --scsi2 "local:iso/$(basename "$platform_iso"),media=cdrom"
qm set "$vmid" --scsi3 "local:iso/$(basename "$ignition_iso"),media=cdrom"
qm set "$vmid" --boot order=scsi0
qm config "$vmid"
qm start "$vmid" >/dev/null
`, remoteDir, ignitionISO, cidataISO, platformISO, env.VMID, vmName, env.SourceImage, env.Storage, workImage, platformModuleDir, mergeModuleDir, remoteElemental3ctl, providerIgnitionPayload)
	ssh(t, env.Host, setup)

	check := fmt.Sprintf(`
set -euo pipefail
sleep 180
qm shutdown %[2]s --timeout 120 >/dev/null 2>&1 || qm stop %[2]s >/dev/null
qm status %[2]s
config="$(qm config %[2]s)"
printf '%%s\n' "$config"
printf '%%s\n' "$config" | grep -F "scsi1: local:iso/vm-%[2]s-cidata.iso,media=cdrom"
printf '%%s\n' "$config" | grep -F "scsi2: local:iso/elemental-merge-mode-%[2]s-platform.iso,media=cdrom"
printf '%%s\n' "$config" | grep -F "scsi3: local:iso/elemental-merge-mode-%[2]s-ignition.iso,media=cdrom"
vol="$(printf '%%s\n' "$config" | awk -F'[ ,]+' '/^scsi0:/{print $2; exit}')"
disk="$(pvesm path "$vol" 2>/dev/null || true)"
if [ -z "$disk" ]; then
  disk="/dev/pve/${vol#*:}"
fi

read_guest() {
  guestfish --ro -a "$disk" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot -m /dev/sda3:/var:subvol=@/var cat "$1"
}

base="$(read_guest /var/lib/elemental/base-ignition-marker)"
provider="$(read_guest /var/lib/elemental/provider-ignition-marker)"
userdata="$(read_guest /var/lib/elemental/k8s-dynamic/userdata.yaml)"
status="$(read_guest /var/lib/elemental/k8s-dynamic/status.yaml)"
init="$(read_guest /var/lib/elemental/kubernetes/init.yaml)"
staged="$(guestfish --ro -a "$disk" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot cat /usr/lib/ignition/base.d/10-elemental-base.ign)"

printf 'base=%%s\nprovider=%%s\n' "$base" "$provider"
printf '%%s\n' "$userdata" | grep -F 'hostname: merge-node.example.com'
printf '%%s\n' "$status" | grep -Ei 'applied|success|complete'
printf '%%s\n' "$init" | grep -F 'token: merge-token'
printf '%%s\n' "$staged" | grep -F '/var/lib/elemental/base-ignition-marker'
[ "$base" = "base-ok" ]
[ "$provider" = "provider-ok" ]
`, env.Storage, env.VMID)

	out := ssh(t, env.Host, check)
	for _, expected := range []string{"base=base-ok", "provider=provider-ok", "hostname: merge-node.example.com", "token: merge-token"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected merge-mode output to contain %q, got:\n%s", expected, out)
		}
	}
}
