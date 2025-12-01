# Wonder Mesh Net

A networking layer that connects homelab machines to the internet, making them accessible to PaaS platforms and orchestration tools.

## What is this?

Wonder Mesh Net solves **one problem well**: network connectivity for homelab infrastructure.

Your homelab machines are behind NAT, have dynamic IPs, or sit behind firewalls. Wonder Mesh Net creates an overlay network that makes them reachable as if they were cloud VMs.

```
┌─────────────────────────────────────────────────────────────────────┐
│                    PaaS / Orchestration Layer                       │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  Kubernetes / Zeabur / Railway / Fly.io / Coolify / Dokploy │   │
│  │                  (app management - not our scope)            │   │
│  └──────────────────────────────┬──────────────────────────────┘   │
└─────────────────────────────────┼───────────────────────────────────┘
                                  │ needs network access
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      WONDER-MESH-NET (this project)                 │
│                                                                     │
│   "Make homelab machines reachable as if they were cloud VMs"       │
│                                                                     │
│   - NAT traversal (WireGuard + DERP relay fallback)                 │
│   - Mesh networking (Headscale control plane)                       │
│   - HTTP gateway for inbound traffic                                │
│   - No TUN device required (userspace networking / tsnet)           │
│                                                                     │
└─────────────────────────────────┬───────────────────────────────────┘
                                  │ overlay network
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    HOMELAB MACHINES (user-owned)                    │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐            │
│  │ mini-pc  │  │ mini-pc  │  │   old    │  │   NAS    │   ...      │
│  │ (ARM64)  │  │  (x86)   │  │  laptop  │  │          │            │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘            │
└─────────────────────────────────────────────────────────────────────┘
```

## Why?

- **For users**: Use your own hardware for deployments. Pay less. Keep data local.
- **For PaaS providers**: Offer "bring your own compute" without building NAT traversal infrastructure.

We're working with partners like Zeabur and welcome integrations from Railway, Fly.io, Supabase, Coolify, Dokploy, and others.

## What we don't do

App management. That's handled by:
- **Kubernetes** (k3s, k8s)
- **PaaS platforms**: Zeabur, Railway, Fly.io, Supabase
- **Self-hosted PaaS**: Coolify, Dokploy

We provide the network. They manage the apps.

## Technology

- **Tailscale/Headscale**: WireGuard-based mesh networking with DERP relay fallback
- **tsnet**: Go-native socket API (no TUN device permissions required)
- **Alternative**: ZeroTier + libzt for multi-language bindings

## Quick Start (MVP Demo)

The MVP uses Lima VMs to demonstrate the networking layer on macOS.

```bash
# Prerequisites
brew install lima
brew install go

# Run the demo
./demo.sh
```

See [MANUAL-MVP.md](./MANUAL-MVP.md) for detailed setup instructions.

## License

MIT
