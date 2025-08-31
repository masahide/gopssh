//go:build windows

package pssh

import (
    "net"
    "path/filepath"
    "strings"

    winio "github.com/Microsoft/go-winio"
)

// winDial dials Windows OpenSSH agent named pipe.
type winDial struct{}

func (w *winDial) Dial(network, address string) (net.Conn, error) {
    // `address` is expected to be a named pipe path, e.g.:
    //  - `\\.\pipe\openssh-ssh-agent`
    //  - `//./pipe/openssh-ssh-agent`
    //  - `np:////./pipe/openssh-ssh-agent` (some tools prepend `np:`)
    p := normalizePipePath(address)
    return winio.DialPipe(p, nil)
}

func init() {
    // Override the dialer factory for Windows to use named pipes.
    newNetDialFunc = func() dialIface { return &winDial{} }
}

// normalizePipePath converts various representations to `\\.\pipe\name`.
func normalizePipePath(p string) string {
    // Drop optional `np:` prefix used by some environments
    if strings.HasPrefix(p, "np:") {
        p = strings.TrimPrefix(p, "np:")
    }

    // Convert forward slashes to backslashes for reliability
    p = filepath.Clean(strings.ReplaceAll(p, "/", "\\"))

    // Ensure it starts with `\\.\pipe\`
    if strings.HasPrefix(p, `\\\\.\\pipe\\`) { // already normalized
        return p
    }
    if strings.HasPrefix(strings.ToLower(p), `\\\\.\\pipe\\`) {
        return p
    }
    // Handle `//./pipe/` style
    if strings.HasPrefix(p, `\\\\.\\pipe\`) { // after Clean it could be this
        return p
    }
    if strings.HasPrefix(p, `\\\\.\pipe\`) {
        return p
    }
    if strings.HasPrefix(strings.ToLower(p), `\\\\.\pipe\`) {
        return p
    }
    if strings.HasPrefix(p, `\\.\pipe\`) {
        return p
    }
    if strings.HasPrefix(p, `//./pipe/`) {
        return `\\.\pipe\` + strings.TrimPrefix(p, `//./pipe/`)
    }
    if strings.HasPrefix(p, `////./pipe/`) {
        return `\\.\pipe\` + strings.TrimPrefix(p, `////./pipe/`)
    }
    // As a last resort, assume it is the pipe name without prefix
    return `\\.\pipe\` + strings.TrimLeft(p, `\`)
}
