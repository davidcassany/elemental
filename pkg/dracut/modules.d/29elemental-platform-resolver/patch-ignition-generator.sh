#!/usr/bin/env bash
set -euo pipefail

generator="${1:-}"

if [ -z "$generator" ]; then
	printf 'usage: %s /path/to/ignition-generator\n' "$(basename "$0")" >&2
	exit 2
fi

if [ ! -f "$generator" ]; then
	printf 'ignition-generator not found: %s\n' "$generator" >&2
	exit 1
fi

if grep -Fq "elemental-platform-resolver preserve resolved PLATFORM_ID" "$generator"; then
	exit 0
fi

tmp="${generator}.tmp.$$"
trap 'rm -f "$tmp"' EXIT

awk '
	$0 == "echo \"PLATFORM_ID=$(cmdline_arg ignition.platform.id)\" > /run/ignition.env" {
		print "if [ -f /run/ignition.env ]; then"
		print "\t. /run/ignition.env"
		print "fi"
		print "if [ -z \"${PLATFORM_ID:-}\" ]; then"
		print "\techo \"PLATFORM_ID=$(cmdline_arg ignition.platform.id)\" > /run/ignition.env"
		patching = 1
		next
	}
	patching && $0 == ". /run/ignition.env" {
		print "\t. /run/ignition.env"
		print "fi"
		print "# elemental-platform-resolver preserve resolved PLATFORM_ID across generator reruns"
		patching = 0
		next
	}
	{ print }
	END {
		if (patching) {
			exit 3
		}
	}
' "$generator" >"$tmp"

if ! grep -Fq "elemental-platform-resolver preserve resolved PLATFORM_ID" "$tmp"; then
	printf 'failed patching ignition-generator: expected platform write block not found in %s\n' "$generator" >&2
	exit 1
fi

cat "$tmp" >"$generator"
