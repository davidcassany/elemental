#!/usr/bin/env bash

check() {
	return 0
}

depends() {
	echo ignition
	return 0
}

install() {
	inst_simple "$moddir/elemental-ignition-merge.sh" "/usr/lib/elemental/elemental-ignition-merge.sh"
	inst_simple "$moddir/elemental-ignition-merge.service" "$systemdsystemunitdir/elemental-ignition-merge.service"
	mkdir -p "$initdir/$systemdsystemunitdir/initrd.target.wants"
	ln_r "$systemdsystemunitdir/elemental-ignition-merge.service" "$systemdsystemunitdir/initrd.target.wants/elemental-ignition-merge.service"
}
