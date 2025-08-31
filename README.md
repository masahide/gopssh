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


## Windows

Windows での OpenSSH agent 対応:

- 既定のnamed pipe: `\\.\pipe\openssh-ssh-agent`
- `SSH_AUTH_SOCK` が未設定でも上記の既定パスを自動的に使用します。
- カスタムのnamed pipeを使いたい場合は `SSH_AUTH_SOCK` に以下のいずれかの形式で指定してください。
  - `\\.\pipe\openssh-ssh-agent`
  - `//./pipe/openssh-ssh-agent`
  - `np:////./pipe/openssh-ssh-agent`
- OpenSSH Authentication Agent サービスを起動してください（Windows 標準の OpenSSH）。

実行例（PowerShell）:

```powershell
# 既定のnamed pipeを使用（SSH_AUTH_SOCK 未設定でも可）
gopssh.exe -h hosts.txt whoami

# 特定のnamed pipeを明示指定
$env:SSH_AUTH_SOCK = "\\\.\pipe\openssh-ssh-agent"
gopssh.exe -h hosts.txt hostname
```

ビルド例（クロスコンパイル）:

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -v -ldflags "-X main.version=0.0.0 -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date --iso-8601=seconds)" \
  -o .bin/gopssh.exe cmd/gopssh/main.go
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
