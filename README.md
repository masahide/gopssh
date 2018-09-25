# gopssh
[![Build Status](https://travis-ci.org/masahide/gopssh.svg?branch=master)](https://travis-ci.org/masahide/gopssh)
[![goreleaser](https://img.shields.io/badge/powered%20by-goreleaser-green.svg?style=flat-square)](https://github.com/goreleaser)

parallel ssh ssh client


# Usage

```
Usage of gopssh:
  -c	colorized outputs (default true)
  -ciphers string
    	allowed cipher algorithms (default "arcfour256,aes128-gcm@openssh.com,chacha20-poly1305@openssh.com,aes128-ctr,aes192-ctr,aes256-ctr")
  -debug
    	debug outputs
  -h string
    	host file
  -i	read stdin
  -kex string
    	allowed key exchanges algorithms (default "diffie-hellman-group1-sha1,diffie-hellman-group14-sha1,ecdh-sha2-nistp256,ecdh-sha2-nistp384,ecdh-sha2-nistp521,curve25519-sha256@libssh.org")
  -macs string
    	allowed MAC algorithms (default "hmac-sha1-96,hmac-sha1,hmac-sha2-256,hmac-sha2-256-etm@openssh.com")
  -n	show hostname
  -p int
    	concurrency(0 is unlimit)
  -timeout duration
    	maximum amount of time for the TCP connection to establish. (default 5s)
  -u string
    	user (default "yamasaki_masahide")
```
