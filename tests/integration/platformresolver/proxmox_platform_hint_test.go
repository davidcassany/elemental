//go:build integration

package platformresolver

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestProxmoxPlatformHintSelectsProxmoxVE(t *testing.T) {
	env := requireProxmoxEnv(t)

	vmName := "elemental-platform-hint-" + env.VMID
	remoteDir := "/var/lib/vz/template/iso/elemental-platform-hint-" + env.VMID
	remoteModuleDir := remoteDir + "/29elemental-platform-resolver"
	workImage := remoteDir + "/source-with-resolver.qcow2"
	cidataISO := "/var/lib/vz/template/iso/vm-" + env.VMID + "-cidata.iso"
	platformISO := "/var/lib/vz/template/iso/elemental-platform-hint-" + env.VMID + "-platform.iso"
	markerPath := "/var/lib/elemental-platform-hint/provider.txt"
	localModuleDir := filepath.Join("..", "..", "..", "pkg", "dracut", "modules.d", "29elemental-platform-resolver")

	cleanup := fmt.Sprintf(`
set -euo pipefail
qm stop %[1]s >/dev/null 2>&1 || true
qm destroy %[1]s --purge >/dev/null 2>&1 || true
rm -rf %[2]q
rm -f %[3]q %[4]q
`, env.VMID, remoteDir, platformISO, cidataISO)
	defer ssh(t, env.Host, cleanup)

	ssh(t, env.Host, fmt.Sprintf("set -euo pipefail\nrm -rf %[1]q\nmkdir -p %[1]q\n", remoteDir))
	scpToHost(t, localModuleDir, env.Host, remoteDir+"/")

	setup := fmt.Sprintf(`
set -euo pipefail
remote_dir=%[1]q
marker_path=%[2]q
platform_iso=%[3]q
cidata_iso=%[4]q
vmid=%[5]q
vm_name=%[6]q
source_image=%[7]q
storage=%[8]q
work_image=%[9]q
module_dir=%[10]q

mkdir -p "$remote_dir"
cd "$remote_dir"
rm -rf platform cidata initrd-work "$work_image"
mkdir -p platform cidata initrd-work/extract initrd-work/out

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

cat > cidata/user-data <<IGN
{
  "ignition": { "version": "3.5.0" },
  "storage": {
    "files": [
      {
        "path": "$marker_path",
        "mode": 420,
        "contents": { "source": "data:,proxmoxve" }
      }
    ]
  }
}
IGN
cat > cidata/meta-data <<META
instance-id: vm-$vmid
local-hostname: $vm_name
META
make_iso cidata "$cidata_iso" cidata
make_iso PLATFORM_HINT "$platform_iso" platform

cp "$source_image" "$work_image"
guestfish --rw -a "$work_image" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot debug sh 'btrfs property set /sysroot ro false'
virt-copy-in -a "$work_image" "$module_dir" /usr/lib/dracut/modules.d
guestfish --rw -a "$work_image" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot rm-f /etc/systemd/system/default.target.wants/jeos-firstboot.service
guestfish --rw -a "$work_image" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot rm-f /etc/systemd/system/sysinit.target.wants/systemd-firstboot.service
guestfish --rw -a "$work_image" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot ln-sf /dev/null /etc/systemd/system/jeos-firstboot.service
guestfish --rw -a "$work_image" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot ln-sf /dev/null /etc/systemd/system/systemd-firstboot.service
virt-copy-out -a "$work_image" /boot/initrd-7.0.12-1-default initrd-work/out
(cd initrd-work/extract && zstd -dc ../out/initrd-7.0.12-1-default | cpio -id --quiet)
install -D -m 0644 "$module_dir/elemental-platform-resolver.service" initrd-work/extract/etc/systemd/system/elemental-platform-resolver.service
mkdir -p initrd-work/extract/etc/systemd/system/initrd.target.wants
ln -sf ../elemental-platform-resolver.service initrd-work/extract/etc/systemd/system/initrd.target.wants/elemental-platform-resolver.service
install -D -m 0755 "$module_dir/elemental-platform-resolver.sh" initrd-work/extract/usr/libexec/elemental-platform-resolver.sh
(cd initrd-work/extract && find . -print0 | cpio --null -o --format=newc --quiet | zstd -19 -T0 > ../out/initrd-7.0.12-1-default)
virt-copy-in -a "$work_image" initrd-work/out/initrd-7.0.12-1-default /boot
guestfish --rw -a "$work_image" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot debug sh 'btrfs property set /sysroot ro true'

qm destroy "$vmid" --purge >/dev/null 2>&1 || true
qm create "$vmid" --name "$vm_name" --memory 2048 --cores 2 --net0 virtio,bridge=vmbr0 --serial0 socket --vga serial0 --bios ovmf --scsihw virtio-scsi-pci
qm importdisk "$vmid" "$work_image" "$storage" >/dev/null
qm set "$vmid" --scsi0 "$storage:vm-$vmid-disk-0"
qm set "$vmid" --efidisk0 "$storage:1,efitype=4m,pre-enrolled-keys=0"
qm set "$vmid" --scsi1 "local:iso/$(basename "$cidata_iso"),media=cdrom"
qm set "$vmid" --scsi2 "local:iso/$(basename "$platform_iso"),media=cdrom"
qm set "$vmid" --boot order=scsi0
qm config "$vmid"
qm start "$vmid" >/dev/null
`, remoteDir, markerPath, platformISO, cidataISO, env.VMID, vmName, env.SourceImage, env.Storage, workImage, remoteModuleDir)
	ssh(t, env.Host, setup)

	check := fmt.Sprintf(`
set -euo pipefail
sleep 180
qm shutdown %[2]s --timeout 120 >/dev/null 2>&1 || qm stop %[2]s >/dev/null
qm status %[2]s
config="$(qm config %[2]s)"
printf '%%s\n' "$config"
printf '%%s\n' "$config" | grep -F "scsi1: local:iso/vm-%[2]s-cidata.iso,media=cdrom"
printf '%%s\n' "$config" | grep -F "scsi2: local:iso/elemental-platform-hint-%[2]s-platform.iso,media=cdrom"
vol="$(printf '%%s\n' "$config" | awk -F'[ ,]+' '/^scsi0:/{print $2; exit}')"
disk="$(pvesm path "$vol" 2>/dev/null || true)"
if [ -z "$disk" ]; then
  disk="/dev/pve/${vol#*:}"
fi
if ! guestfish --ro -a "$disk" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot -m /dev/sda3:/var:subvol=@/var cat %[3]q; then
	echo "--- journal ignition/platform diagnostics ---" >&2
	echo "--- grub platform args ---" >&2
	guestfish --ro -a "$disk" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot cat /etc/default/grub 2>/dev/null | grep 'ignition.platform.id' >&2 || true
	guestfish --ro -a "$disk" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot cat /boot/grub2/grub.cfg 2>/dev/null | grep 'ignition.platform.id' >&2 || true
	echo "--- ignition result ---" >&2
	guestfish --ro -a "$disk" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot cat /.ignition-result.json 2>/dev/null >&2 || true
	echo "--- ignition files ---" >&2
	guestfish --ro -a "$disk" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot find /etc 2>/dev/null | grep -Ei 'ignition|firstboot' >&2 || true
	guestfish --ro -a "$disk" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot find /usr/lib/dracut/modules.d/29elemental-platform-resolver >&2 || true
	guestfish --ro -a "$disk" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot -m /dev/sda3:/var:subvol=@/var find /var 2>/dev/null | grep -Ei 'ignition|journal|elemental-platform' >&2 || true
	guestfish --ro -a "$disk" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot -m /dev/sda3:/var:subvol=@/var find /var/log/journal 2>/dev/null | head -20 >&2 || true
	rm -rf /tmp/epr-journal
	mkdir -p /tmp/epr-journal
	guestfish --ro -a "$disk" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot -m /dev/sda3:/var:subvol=@/var copy-out /var/log/journal /tmp/epr-journal >/dev/null 2>&1 || true
	if [ -d /tmp/epr-journal/journal ]; then
		echo "--- journalctl ignition/platform excerpts ---" >&2
		journalctl --directory=/tmp/epr-journal/journal -b --no-pager -o short-monotonic 2>/dev/null | grep -Ei 'ignition|platform|elemental|cidata|proxmox|qemu|failed|error' >&2 || true
	fi
	guestfish --ro -a "$disk" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot cat /run/ignition.env 2>/dev/null >&2 || true
  rm -rf /tmp/epr-initrd-check
  mkdir -p /tmp/epr-initrd-check/extract
  guestfish --ro -a "$disk" -m /dev/sda3:/:subvol=@/.snapshots/1/snapshot copy-out /boot/initrd-7.0.12-1-default /tmp/epr-initrd-check >/dev/null 2>&1 || true
  if [ -f /tmp/epr-initrd-check/initrd-7.0.12-1-default ]; then
    (cd /tmp/epr-initrd-check/extract && zstd -dc ../initrd-7.0.12-1-default | cpio -id --quiet)
    find /tmp/epr-initrd-check/extract | grep elemental-platform >&2 || true
  fi
  exit 1
fi
`, env.Storage, env.VMID, markerPath)

	out := ssh(t, env.Host, check)
	if !strings.Contains(out, "proxmoxve") {
		t.Fatalf("expected provider marker to contain proxmoxve, got:\n%s", out)
	}
}

func TestProxmoxIntegrationDocumentsFutureKVMConversion(t *testing.T) {
	note := filepath.Join("..", "..", "..", "docs", "ignition-integration.md")
	data, err := os.ReadFile(note)
	if err != nil {
		t.Fatalf("reading docs: %v", err)
	}
	if !strings.Contains(string(data), "Linux/KVM") {
		t.Fatalf("docs should mention Proxmox validation should move to Linux/KVM when available")
	}
}
