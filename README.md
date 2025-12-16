# Wonder Mesh Net

A PaaS bootstrapper that turns homelab and edge machines into bring-your-own compute for PaaS platforms and orchestration tools.

## What is this?

Wonder Mesh Net bootstraps PaaS workloads onto scattered compute. It handles multi-tenant identity, issues join tokens, and wires up secure connectivity so your orchestrator can treat those machines like cloud VMs.

**Features:**
- Join-token based “bring your own compute” bootstrap
- NAT traversal (WireGuard + DERP relay fallback)
- Mesh networking (Headscale control plane)
- Multi-tenant isolation via OIDC authentication
- No TUN device required (userspace networking / tsnet)

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

## License

AGPL-3.0. See [LICENSE](LICENSE) for details.
