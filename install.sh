#!/bin/sh

set -eu

repository="masahide/gopssh"
version="${GOPSSH_VERSION:-latest}"
install_dir="${GOPSSH_INSTALL_DIR:-${HOME}/.local/bin}"

case "$(uname -s)" in
	Darwin) os="darwin" ;;
	Linux) os="linux" ;;
	*)
		echo "gopssh: unsupported operating system: $(uname -s)" >&2
		exit 1
		;;
esac

case "$(uname -m)" in
	x86_64 | amd64) arch="amd64" ;;
	arm64 | aarch64) arch="arm64" ;;
	*)
		echo "gopssh: unsupported architecture: $(uname -m)" >&2
		exit 1
		;;
esac

if ! command -v curl >/dev/null 2>&1; then
	echo "gopssh: curl is required" >&2
	exit 1
fi
if ! command -v tar >/dev/null 2>&1; then
	echo "gopssh: tar is required" >&2
	exit 1
fi

asset="${os}-${arch}.tar.gz"
if [ "$version" = "latest" ]; then
	download_base="https://github.com/${repository}/releases/latest/download"
else
	case "$version" in
		v*) release_tag="$version" ;;
		*) release_tag="v${version}" ;;
	esac
	download_base="https://github.com/${repository}/releases/download/${release_tag}"
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM

echo "Downloading gopssh ${version} for ${os}/${arch}..."
curl -fsSL "${download_base}/${asset}" -o "${tmp_dir}/${asset}"
curl -fsSL "${download_base}/checksums.txt" -o "${tmp_dir}/checksums.txt"

expected="$(awk -v asset="$asset" '$2 == asset { print $1 }' "${tmp_dir}/checksums.txt")"
if [ -z "$expected" ]; then
	echo "gopssh: checksum for ${asset} was not found" >&2
	exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
	actual="$(sha256sum "${tmp_dir}/${asset}" | awk '{ print $1 }')"
elif command -v shasum >/dev/null 2>&1; then
	actual="$(shasum -a 256 "${tmp_dir}/${asset}" | awk '{ print $1 }')"
else
	echo "gopssh: sha256sum or shasum is required" >&2
	exit 1
fi
if [ "$actual" != "$expected" ]; then
	echo "gopssh: checksum verification failed for ${asset}" >&2
	exit 1
fi

tar -xzf "${tmp_dir}/${asset}" -C "$tmp_dir"
mkdir -p "$install_dir"
install -m 0755 "${tmp_dir}/gopssh" "${install_dir}/gopssh"

echo "gopssh was installed to ${install_dir}/gopssh"
case ":${PATH}:" in
	*":${install_dir}:"*) ;;
	*) echo "Add ${install_dir} to PATH before running gopssh." ;;
esac
