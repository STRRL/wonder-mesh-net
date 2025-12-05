# Wonder Mesh Net – Purpose

## Statement of Purpose

Enable PaaS platforms and orchestrators to treat dispersed homelab or edge devices like cloud VMs by quickly bootstrapping identity, trust, and secure connectivity—turning bring-your-own compute into cloud-grade reachability.

## Background & Problem

- Most personal or edge machines lack public IPs, so services cannot be exposed directly.
- Advances in libp2p, WebRTC, blockchain networking, and IPv6 make “no public IP” far less of a blocker.
- PaaS providers (e.g., Zeabur) are beginning to explore Bring Your Own Compute (BYOC); the tech is feasible and the go-to-market experiments have started. Now is the right time to productize it.

## How Wonder Mesh Net Solves It

- Standardizes the onboarding flow so developers and homelab users can, Tailscale-style, turn small clusters into secure mesh nodes with a simple token.
- Provides a consistent SDK surface for PaaS platforms (Zeabur, Railway, Fly.io, Coolify, Dokploy, etc.) to integrate BYOC nodes without rebuilding networking or identity plumbing.
- Establishes the mesh via Headscale/Tailscale (WireGuard + DERP fallback) and uses tsnet to avoid TUN permissions.

## Scope Boundaries

- We own bootstrapping and connectivity: identity, tenant isolation, join tokens, and mesh wiring.
- We do **not** manage applications or workloads; Kubernetes or the PaaS layer handles that.

## Target Users

- PaaS/platform teams adding BYOC.
- Homelab/edge operators who want local hardware to behave like cloud nodes.
- Integrators building workload managers atop the Wonder SDK.

## Success Criteria (Definition of Done)

- A node owner signs in via OIDC, receives a join token, and adds a machine with a single command.
- Orchestrators reach the node over the mesh with no extra firewall/NAT work.
- Tenants are isolated per OIDC identity namespace.
- Partners integrate through the SDK without reimplementing networking or auth.
