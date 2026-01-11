# Kubeadm Deployer Demo

This demo shows how to use Wonder Mesh Net to bootstrap a Kubernetes cluster across distributed nodes using kubeadm.

## Overview

The kubeadm-deployer demonstrates the deployer integration pattern:

1. **Deployer** authenticates with Wonder Mesh Net coordinator using an API key
2. **Deployer** joins the mesh network and discovers online worker nodes
3. **Deployer** SSHs to each node over the mesh to install containerd and kubeadm
4. **Deployer** runs `kubeadm init` on the first node (control plane)
5. **Deployer** installs Cilium CNI
6. **Deployer** runs `kubeadm join` on remaining nodes (workers)

Result: A working 3-node Kubernetes cluster accessible over the mesh network.

## Prerequisites

- Docker with Docker Compose v2
- Go 1.23+ (for building)
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
NO_CLEANUP=1 ./run-demo.sh
```

## What the Demo Does

1. **Starts infrastructure**: nginx, Keycloak, Headscale, Coordinator
2. **Starts 3 systemd-enabled workers**: k8s-node-1, k8s-node-2, k8s-node-3
3. **Workers join mesh**: Each worker runs `wonder worker join`
4. **Creates API key**: For deployer authentication
5. **Deployer joins mesh**: Using userspace Tailscale with SOCKS5 proxy
6. **Runs kubeadm-deployer**: Bootstraps the Kubernetes cluster

## Manual Execution

If you want to run steps manually:

```bash
# 1. Start services
docker compose up -d --build

# 2. Wait for services to be ready
curl http://localhost:8080/coordinator/health

# 3. Get access token (from inside deployer container)
docker exec kubeadm-deployer curl -s -X POST \
    "http://nginx/realms/wonder/protocol/openid-connect/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=password&client_id=wonder-mesh-net&client_secret=wonder-secret&username=testuser&password=testpass"

# 4. Create join token and join workers
# ... (see run-demo.sh for full flow)

# 5. Run deployer
docker exec kubeadm-deployer kubeadm-deployer \
    --coordinator-url="http://nginx" \
    --api-key="<your-api-key>" \
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
      --kube-version string      Kubernetes version to install (default "1.31")
      --kubeconfig-output string Path to save admin kubeconfig (default "/tmp/kubeconfig")
      --pod-network-cidr string  Pod network CIDR (default "10.244.0.0/16")
      --socks5-addr string       SOCKS5 proxy address for mesh access (default "localhost:1080")
      --ssh-password string      SSH password for node access (default "worker")
      --ssh-user string          SSH username for node access (default "root")
  -v, --verbose                  Enable verbose logging
```

## SDK Usage Example

The deployer demonstrates how to use `wondersdk`:

```go
import "github.com/strrl/wonder-mesh-net/pkg/wondersdk"

// Create client
client := wondersdk.NewClient(coordinatorURL, apiKey)

// Check coordinator health
if err := client.Health(ctx); err != nil {
    log.Fatal(err)
}

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

### Cilium pods not starting

Check Cilium status:
```bash
docker exec kubeadm-deployer kubectl --kubeconfig /tmp/kubeconfig get pods -n kube-system
docker exec k8s-node-1 cilium status
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
