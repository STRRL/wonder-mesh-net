# Kubeadm Deployer Demo

This demo shows how to use Wonder Mesh Net to bootstrap a Kubernetes cluster across distributed nodes using kubeadm.

## Overview

The kubeadm-deployer demonstrates the deployer integration pattern using the **Admin API** for simplified authentication:

1. **Admin** creates a wonder net via Admin API using `ADMIN_API_AUTH_TOKEN`
2. **Admin** creates join tokens for worker nodes
3. **Workers** join the mesh network using join tokens
4. **Admin** creates an API key and deployer credentials for the deployer
5. **Deployer** joins the mesh and discovers online worker nodes
6. **Deployer** SSHs to each node over the mesh to install containerd and kubeadm
7. **Deployer** runs `kubeadm init` on the first node (control plane)
8. **Deployer** installs Flannel CNI
9. **Deployer** runs `kubeadm join` on remaining nodes (workers)

Result: A working 3-node Kubernetes cluster accessible over the mesh network.

## Prerequisites

- Docker with Docker Compose v2
- Go (for building; use the version specified in go.mod)
- ~8GB RAM available for containers
- Linux host with kernel modules: `br_netfilter`, `overlay`

### Host Kernel Modules

Ensure these modules are loaded on your host:

```bash
sudo modprobe br_netfilter
sudo modprobe overlay
```

## Quick Start

```bash
cd examples/kubeadm-deployer

# Run the full demo (builds, starts, and deploys)
./run-demo.sh

# Keep containers running after demo completes
NO_CLEAN=1 ./run-demo.sh
```

## What the Demo Does

1. **Starts infrastructure**: nginx, Keycloak, Headscale, Coordinator (with Admin API enabled)
2. **Starts 3 systemd-enabled workers**: k8s-node-1, k8s-node-2, k8s-node-3
3. **Creates wonder net**: Via Admin API using `ADMIN_API_AUTH_TOKEN`
4. **Creates join token**: Via Admin API for worker authentication
5. **Workers join mesh**: Each worker runs `wonder worker join`
6. **Creates API key**: Via Admin API for deployer authentication
7. **Deployer joins mesh**: Using userspace Tailscale with SOCKS5 proxy (credentials via Admin API)
8. **Runs kubeadm-deployer**: Bootstraps the Kubernetes cluster

## Manual Execution

If you want to run steps manually:

```bash
# Admin API token (must match docker-compose.yaml)
ADMIN_TOKEN="kubeadm-demo-admin-token-at-least-32-chars"

# 1. Start services
docker compose up -d --build

# 2. Wait for services to be ready
curl http://localhost:8080/coordinator/health

# 3. Create wonder net via Admin API
WONDER_NET_RESPONSE=$(docker exec kubeadm-deployer curl -s -X POST \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"owner_id": "kubeadm-demo", "display_name": "Kubeadm Demo Wonder Net"}' \
    "http://nginx/coordinator/admin/api/v1/wonder-nets")
WONDER_NET_ID=$(echo "$WONDER_NET_RESPONSE" | jq -r '.id')

# 4. Create join token via Admin API
JOIN_TOKEN=$(docker exec kubeadm-deployer curl -s -X POST \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    "http://nginx/coordinator/admin/api/v1/wonder-nets/$WONDER_NET_ID/join-token" | jq -r '.token')

# 5. Join workers (see run-demo.sh for full flow)
# ...

# 6. Create API key via Admin API
API_KEY=$(docker exec kubeadm-deployer curl -s -X POST \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"name": "kubeadm-deployer", "expires_in": "24h"}' \
    "http://nginx/coordinator/admin/api/v1/wonder-nets/$WONDER_NET_ID/api-keys" | jq -r '.key')

# 7. Run deployer
docker exec kubeadm-deployer kubeadm-deployer \
    --coordinator-url="http://nginx/coordinator" \
    --api-key="$API_KEY" \
    --verbose
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Docker Compose Stack                      │
├──────────────┬──────────────┬──────────────┬────────────────┤
│   nginx      │   Keycloak   │  Headscale   │  Coordinator   │
│   (proxy)    │   (OIDC)     │  (mesh ctrl) │  (WMN API)     │
└──────┬───────┴──────────────┴──────┬───────┴───────┬────────┘
       │                              │               │
       │         Mesh Network (Tailscale/Headscale)   │
       │    ┌─────────────────────────────────────────┤
       │    │                                         │
┌──────┴────┴─────┐  ┌───────────────┐  ┌─────────────┴─────┐
│   k8s-node-1    │  │  k8s-node-2   │  │    k8s-node-3     │
│ (control plane) │  │   (worker)    │  │     (worker)      │
│   100.64.x.x    │  │  100.64.x.x   │  │    100.64.x.x     │
└─────────────────┘  └───────────────┘  └───────────────────┘
         ▲                   ▲                    ▲
         │                   │                    │
         └───────────────────┼────────────────────┘
                             │
                    ┌────────┴────────┐
                    │    deployer     │
                    │ (kubeadm-deployer)
                    │  SOCKS5 proxy   │
                    │  100.64.x.x     │
                    └─────────────────┘
```

## CLI Usage

```
kubeadm-deployer --help

Usage:
  kubeadm-deployer [flags]

Flags:
      --api-key string           API key for authentication (required)
      --coordinator-url string   Wonder Mesh Net coordinator URL (required)
  -h, --help                     help for kubeadm-deployer
  -v, --verbose                  Enable verbose logging
```

Default values are hardcoded for demo simplicity:
- Kubernetes version: 1.31
- Pod network CIDR: 10.244.0.0/16
- SSH user/password: root/worker
- SOCKS5 proxy: localhost:1080

## SDK Usage Example

The deployer demonstrates how to use `wondersdk`:

```go
import "github.com/strrl/wonder-mesh-net/pkg/wondersdk"

// Create client
client := wondersdk.NewClient(coordinatorURL, apiKey)

// Discover online nodes
nodes, err := client.GetOnlineNodes(ctx, "")
if err != nil {
    log.Fatal(err)
}

for _, node := range nodes {
    fmt.Printf("Node: %s, IPs: %v\n", node.Name, node.Addresses)
}
```

## Troubleshooting

### Workers fail to join mesh

Check Tailscale status:
```bash
docker exec k8s-node-1 tailscale status
docker logs k8s-node-1
```

### kubeadm init fails

Check systemd and containerd:
```bash
docker exec k8s-node-1 systemctl status containerd
docker exec k8s-node-1 journalctl -u kubelet
```

### Flannel pods not starting

Check Flannel status:
```bash
docker exec kubeadm-deployer kubectl --kubeconfig /tmp/kubeconfig get pods -n kube-flannel
docker exec k8s-node-1 kubectl logs -n kube-flannel -l app=flannel
```

### SSH connection fails

Verify SOCKS5 proxy is running:
```bash
docker exec kubeadm-deployer tailscale --socket=/tmp/tailscaled.sock status
docker exec kubeadm-deployer nc -zv localhost 1080
```

## Cleanup

```bash
docker compose down -v
```

## Related Issues

- [#39](https://github.com/strrl/wonder-mesh-net/issues/39) - Example deployer (kubeadm)
- [#68](https://github.com/strrl/wonder-mesh-net/issues/68) - Local deployer with kubeadm/Cluster API demo
