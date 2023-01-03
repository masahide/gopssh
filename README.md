# gopssh
[![Go Report Card](https://goreportcard.com/badge/github.com/masahide/gopssh)](https://goreportcard.com/report/github.com/masahide/gopssh)
[![Build Status](https://travis-ci.org/masahide/gopssh.svg?branch=master)](https://travis-ci.org/masahide/gopssh)
[![codecov](https://codecov.io/gh/masahide/gopssh/branch/master/graph/badge.svg)](https://codecov.io/gh/masahide/gopssh)
[![goreleaser](https://img.shields.io/badge/powered%20by-goreleaser-green.svg?style=flat-square)](https://github.com/goreleaser)

parallel ssh client


# Usage

```bash
Usage of ./gopssh:
  -a int
    	Max ssh agent unix socket connections (default 50)
  -c	colorized outputs (default true)
  -ciphers string
    	allowed cipher algorithms (default "arcfour256,aes128-gcm@openssh.com,chacha20-poly1305@openssh.com,aes128-ctr,aes192-ctr,aes256-ctr")
  -d	show hostname
  -debug
    	debug outputs
  -h string
    	host file
  -i string
    	identity files (default "~/.ssh/id_dsa,~/.ssh/id_ecdsa,~/.ssh/id_ed25519,~/.ssh/id_rsa")
  -k	Do not check the host key
  -kex string
    	allowed key exchanges algorithms (default "diffie-hellman-group1-sha1,diffie-hellman-group14-sha1,ecdh-sha2-nistp256,ecdh-sha2-nistp384,ecdh-sha2-nistp521,curve25519-sha256@libssh.org")
  -macs string
    	allowed MAC algorithms (default "hmac-sha1-96,hmac-sha1,hmac-sha2-256,hmac-sha2-256-etm@openssh.com")
  -p int
    	concurrency (defalut "0" is unlimit)
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

see [releases page](https://github.com/masahide/gopssh/releases).



## build

```
go build -v -ldflags "-X main.version=0.5.6 -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date --iso-8601=seconds)" -o .bin/gopssh cmd/gopssh/main.go
```

### build rpm

```
ver=$(.bin/gopssh -version)
version=$(echo "$ver"|awk '/^version/{print $2}')
commit=$(echo "$ver"|awk '/^commit/{print $2}')
arch=$(uname -m)
release=1
go run -ldflags "-X main.name=gopssh -X main.release=$release -X main.version=$version -X main.hash=$commit -X main.arch=$arch" cmd/rpmpack/main.go
```


### build deb

```
ver=$(.bin/gopssh -version)
version=$(echo "$ver"|awk '/^version/{print $2}')
arch=amd64
go run -ldflags "-X main.name=gopssh -X main.version=$version -X main.arch=$arch" cmd/debpack/main.go
```
