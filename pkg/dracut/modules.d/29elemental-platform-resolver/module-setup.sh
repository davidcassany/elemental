#!/usr/bin/env bash

check() {
	return 0
}

depends() {
	echo ignition
	return 0
}

install() {
	inst_multiple -o \
		awk \
		bash \
		find \
		grep \
		mount \
		sleep \
		systemd-detect-virt \
		tr \
		umount
	inst_multiple -o grub2-editenv

	inst_simple "$moddir/elemental-platform-resolver.sh" "/usr/libexec/elemental-platform-resolver.sh"
	inst_simple "$moddir/elemental-platform-resolver.service" "$systemdsystemunitdir/elemental-platform-resolver.service"
	$SYSTEMCTL -q --root "$initdir" enable elemental-platform-resolver.service
}
