# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Wonder Mesh Net is a **networking layer** that connects homelab machines (behind NAT, dynamic IPs, firewalls) to the internet, making them accessible to PaaS platforms and orchestration tools.

**Core Purpose**: Make homelab/mini-PC infrastructure reachable as if they were cloud VMs, enabling deployment platforms to use user-owned hardware.

**We solve one problem well: network connectivity.** App management is left to:
- Kubernetes (k3s, k8s)
- PaaS platforms: Zeabur, Railway, Fly.io, Supabase
- Self-hosted PaaS: Coolify, Dokploy

**Primary Technology Choice**: Tailscale/Headscale + tsnet for Go-native socket API with WireGuard-based L3 overlay network and DERP relay fallback.

**Alternative**: ZeroTier + libzt when multi-language bindings or smaller binary size is needed.

## Build Commands

```bash
# Build the wonder binary (outputs to bin/)
make build

# Build for all platforms (linux/darwin, amd64/arm64)
make build-all

# Run all checks (fmt, vet, lint, test)
make check

# Run tests
make test

# Clean build artifacts
make clean

# Show all available make targets
make help
```

Build artifacts are saved to `bin/` directory (gitignored).

## Code Style

- No end-of-line comments
- No Chinese in code comments
