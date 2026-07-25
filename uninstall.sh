#!/bin/sh

set -eu

install_dir="${GOPSSH_INSTALL_DIR:-${HOME}/.local/bin}"
binary="${install_dir}/gopssh"

if [ ! -e "$binary" ] && [ ! -L "$binary" ]; then
	echo "gopssh is not installed at ${binary}"
	exit 0
fi

rm -f -- "$binary"
echo "gopssh was removed from ${binary}"
