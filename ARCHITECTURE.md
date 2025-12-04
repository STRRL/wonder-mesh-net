# Wonder Mesh Net Architecture

## Roles

- **End User**: Owns compute resources (homelab, mini-PC, etc.), wants to deploy workloads on them.
- **Network Coordinator**: Facilitates ICE/NAT traversal and tunnel establishment between nodes.
- **Workload Manager**: Manages workload deployment and orchestration (e.g., Kubernetes, Zeabur, Railway).

The framework is designed to be extensible for each role. Current implementations:

- **Network Coordinator**: Tailscale / Headscale
- **Workload Manager**: Kubernetes, Zeabur (planned)

## Components

- **Coordinator** (`wonder coordinator`): OIDC authentication, tenant management, join token generation.
- **Worker** (`wonder worker`): CLI for device enrollment.
- **Wonder SDK** (`pkg/wondersdk`): Go SDK for Workload Manager integration with Coordinator.
- **Traffic Gateway** (planned): Ingress proxy for Workload Managers to route external traffic into the mesh.
