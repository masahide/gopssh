# gopssh
[![Go Report Card](https://goreportcard.com/badge/github.com/masahide/gopssh)](https://goreportcard.com/report/github.com/masahide/gopssh)
[![Build status](https://github.com/masahide/gopssh/actions/workflows/buildpkg.yml/badge.svg)](https://github.com/masahide/gopssh/actions/workflows/buildpkg.yml)

parallel ssh client


# Usage

```bash
Usage of ./gopssh:
  -a int
    	Max ssh agent unix socket connections (default 50)
  -c	colorized outputs (default true)
  -ciphers string
    	comma-separated cipher algorithms (default: secure SSH defaults)
  -d	show hostname
  -debug
    	debug outputs
  -h string
    	host file
  -i string
    	identity files (default "~/.ssh/id_dsa,~/.ssh/id_ecdsa,~/.ssh/id_ed25519,~/.ssh/id_rsa")
  -identities-only
    	use identity files only and disable SSH Agent authentication
  -k	Do not check the host key
  -kex string
    	comma-separated key exchange algorithms (default: secure SSH defaults)
  -legacy-crypto
    	use the legacy SSH algorithms and priority order
  -macs string
    	comma-separated MAC algorithms (default: secure SSH defaults)
  -max-output-bytes int
    	maximum buffered bytes per stdout/stderr stream and host (default 10485760)
  -p int
    	maximum concurrent SSH connections (default 32)
  -s	sort the results and output (default true)
  -timeout duration
    	maximum amount of time for the TCP connection to establish. (default 15s)
  -u string
    	username (default "$USER")
  -version
    	Show version
```

example:
```bash
./gopssh -h <(echo host1 host2) ls -la /etc/
```

## Installation

macOS / Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/masahide/gopssh/main/install.sh | sh
```

The binary is installed to `~/.local/bin/gopssh`. To install a specific version
or change the destination:

```bash
curl -fsSL https://raw.githubusercontent.com/masahide/gopssh/main/install.sh |
  GOPSSH_VERSION=v0.5.6 GOPSSH_INSTALL_DIR="$HOME/bin" sh
```

See the [releases page](https://github.com/masahide/gopssh/releases) for packages
and release notes.

To uninstall:

```bash
curl -fsSL https://raw.githubusercontent.com/masahide/gopssh/main/uninstall.sh | sh
```

If `GOPSSH_INSTALL_DIR` was used during installation, specify the same directory:

```bash
curl -fsSL https://raw.githubusercontent.com/masahide/gopssh/main/uninstall.sh |
  GOPSSH_INSTALL_DIR="$HOME/bin" sh
```

## build

```
go build -v -ldflags "-X main.version=0.5.6
  -X main.commit=$(git rev-parse --short HEAD)
  -X main.date=$(date --iso-8601=seconds)" \
  -o .bin/gopssh \
  cmd/gopssh/main.go
```

### build rpm

```
ver=$(.bin/gopssh -version)
export VERSION=$(echo "$ver"|awk '/^version/{print $2}')
export HASH=$(echo "$ver"|awk '/^commit/{print $2}')
export ARCH=$(uname -m)
export RELEASE=1
export NAME=gopssh
export BINPATH=.bin/$NAME
go run pack/rpmpack/main.go
```


### build deb

```
ver=$(.bin/gopssh -version)
export VERSION=$(echo "$ver"|awk '/^version/{print $2}')
export ARCH=amd64
export MAINTAINER=$USER
export NAME=gopssh
export BINPATH=.bin/$NAME
go run pack/debpack/main.go
```
