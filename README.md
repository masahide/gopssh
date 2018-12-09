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
    	Max ssh agent unix socket connections (default 100)
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
  -timeout duration
    	maximum amount of time for the TCP connection to establish. (default 5s)
  -u string
    	username (default "username")
  -version
    	Show version
```

example:
```bash
./gopssh -h <(echo host1 host2) ls -la /etc/
```

## Installation

### Linux

For RHEL/CentOS:

```bash
sudo yum install https://github.com/masahide/gopssh/releases/download/v0.3.0/gopssh_amd64.rpm
```

For Ubuntu/Debian:

```bash
wget -qO /tmp/gopssh_amd64.deb https://github.com/masahide/gopssh/releases/download/v0.3.0/gopssh_amd64.deb
sudo dpkg -i /tmp/gopssh_amd64.deb
```

### macOS


install via [brew](https://brew.sh):

```bash
brew tap masahide/gopssh https://github.com/masahide/gopssh
brew install gopssh
```
