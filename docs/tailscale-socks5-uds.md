# Tailscale SOCKS5 Unix Domain Socket (UDS) Support

## Background

When running Tailscale in userspace networking mode, it provides a SOCKS5 proxy for other processes to connect through the tailnet. By default, this proxy binds to a TCP port:

```bash
tailscaled --tun=userspace-networking --socks5-server=localhost:1080
```

In shared or containerized environments, binding to a TCP port has security implications - other users or processes on the same host can access the proxy. A Unix domain socket (UDS) would be preferable as it allows filesystem-based access control.

## Current Status

**Tailscale does NOT support UDS for SOCKS5 proxy.** The `--socks5-server` flag only accepts `[ip]:port` format.

Attempting to use a socket path results in:
```
SOCKS5 listener: listen tcp: address /var/run/tailscale/socks5.sock: missing port in address
```

## Feature Request

There is an open feature request to add UDS support:

- **Issue**: [#4657 - FR: Make `/var/run/tailscale/tailscaled.sock` also support SOCKS5](https://github.com/tailscale/tailscale/issues/4657)
- **Status**: Open (since May 2022)
- **Priority**: Labeled as "P1 Nuisance" / "L1 Very few" - low priority

### Proposed Solutions

1. **`file:` prefix syntax** (PR #4658, closed)
   ```bash
   tailscaled --socks5-server="file:/var/run/tailscale/socks5.sock"
   ```
   The PR author got this working with a small patch, but maintainers preferred a different approach.

2. **Protocol detection on existing socket** (preferred by maintainers)
   Add SOCKS5 support directly to `/var/run/tailscale/tailscaled.sock` with automatic protocol detection. Not yet implemented.

## Workarounds

### Option 1: Use `tailscale nc`

Tailscale provides a built-in netcat command that routes through the tailnet:

```bash
tailscale --socket /var/run/tailscale/tailscaled.sock nc <host> <port>
```

This uses the existing UDS control socket and doesn't require SOCKS5. In Go:

```go
cmd := exec.Command("tailscale", "--socket", socketPath, "nc", host, port)
stdin, _ := cmd.StdinPipe()
stdout, _ := cmd.StdoutPipe()
cmd.Start()
// stdin/stdout become the read/write ends of the connection
```

### Option 2: Local UDS-to-TCP Proxy

Run a small proxy that listens on a Unix socket and forwards to the TCP SOCKS5 port:

```go
// Listen on UDS
listener, _ := net.Listen("unix", "/var/run/tailscale/socks5.sock")

for {
    conn, _ := listener.Accept()
    go func(c net.Conn) {
        defer c.Close()
        // Connect to TCP SOCKS5
        proxy, _ := net.Dial("tcp", "localhost:1080")
        defer proxy.Close()
        // Bidirectional copy
        go io.Copy(proxy, c)
        io.Copy(c, proxy)
    }(conn)
}
```

### Option 3: Build Tailscale from Source

Apply a patch to support UDS in the SOCKS5 listener. The change is minimal - modify `net.Listen` to detect `file:` prefix and use `unix` network instead of `tcp`.

## Our Approach

In wonder-mesh-net, we use **Option 1 (`tailscale nc`)** in the deploy-server:

- Uses the existing UDS control socket (`/var/run/tailscale-userspace/tailscaled.sock`)
- No additional TCP ports exposed
- Works with standard tailscale installation

## References

- [Tailscale Userspace Networking Docs](https://tailscale.com/kb/1112/userspace-networking)
- [Issue #4657 - UDS SOCKS5 Feature Request](https://github.com/tailscale/tailscale/issues/4657)
- [PR #4658 - SOCKS5 Unix Socket Support (closed)](https://github.com/tailscale/tailscale/pull/4658)
- [Tailscale Docker Docs](https://tailscale.com/kb/1282/docker)
