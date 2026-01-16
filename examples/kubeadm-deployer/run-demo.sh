#!/bin/bash
# Kubeadm Deployer Demo Runner
# Bootstraps a 3-node Kubernetes cluster using Wonder Mesh Net
# This demo uses the Admin API for simplified authentication flow
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Admin API token - must match docker-compose.yaml (see ADMIN_API_AUTH_TOKEN around line 65).
# If you change this value, update docker-compose.yaml accordingly, or vice versa.
ADMIN_TOKEN="kubeadm-demo-admin-token-at-least-32-chars"

log_info() {
    echo -e "\033[0;32m[INFO]\033[0m $1"
}

log_warn() {
    echo -e "\033[1;33m[WARN]\033[0m $1"
}

log_error() {
    echo -e "\033[0;31m[ERROR]\033[0m $1"
}

cleanup() {
    if [ "${NO_CLEAN:-0}" = "1" ]; then
        log_info "NO_CLEAN=1, skipping cleanup. Containers are still running."
        log_info "To clean up manually: docker compose down -v"
        return
    fi
    log_info "Cleaning up..."
    docker compose down -v --remove-orphans 2>/dev/null || true
}

trap cleanup EXIT

echo "==========================================="
echo "  Kubeadm Deployer Demo (Admin API)"
echo "  Wonder Mesh Net + Kubernetes"
echo "==========================================="

log_info "Starting all services..."
docker compose up -d --build --force-recreate
sleep 15

log_info "Waiting for Coordinator to be ready..."
for i in {1..30}; do
    if curl -sf http://localhost:8080/coordinator/health >/dev/null 2>&1; then
        break
    fi
    if [ $i -eq 30 ]; then
        log_error "Coordinator did not start in time"
        docker logs kubeadm-coordinator 2>&1 | tail -30
        exit 1
    fi
    echo "  Waiting for Coordinator... ($i/30)"
    sleep 2
done
log_info "Coordinator is ready"

log_info "Creating wonder net via Admin API..."
WONDER_NET_RESPONSE=$(docker exec kubeadm-deployer curl -s -X POST \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"owner_id": "kubeadm-demo", "display_name": "Kubeadm Demo Wonder Net"}' \
    "http://nginx/coordinator/admin/api/v1/wonder-nets")

WONDER_NET_ID=$(echo "$WONDER_NET_RESPONSE" | jq -r '.id // empty')
if [ -z "$WONDER_NET_ID" ]; then
    log_error "Failed to create wonder net"
    echo "$WONDER_NET_RESPONSE"
    exit 1
fi
log_info "Wonder net created: $WONDER_NET_ID"

log_info "Creating join token via Admin API..."
JOIN_TOKEN_RESPONSE=$(docker exec kubeadm-deployer curl -s -X POST \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    "http://nginx/coordinator/admin/api/v1/wonder-nets/$WONDER_NET_ID/join-token")

JOIN_TOKEN=$(echo "$JOIN_TOKEN_RESPONSE" | jq -r '.token // empty')
if [ -z "$JOIN_TOKEN" ]; then
    log_error "Failed to create join token"
    echo "$JOIN_TOKEN_RESPONSE"
    exit 1
fi
log_info "Join token created"

log_info "Building wonder binary for Linux..."
ARCH=$(uname -m)
case "$ARCH" in
    x86_64) GOARCH=amd64 ;;
    aarch64|arm64) GOARCH=arm64 ;;
    *) log_error "Unsupported architecture: $ARCH"; exit 1 ;;
esac
log_info "  Detected architecture: $ARCH -> GOARCH=$GOARCH"
(cd ../.. && GOOS=linux GOARCH=$GOARCH CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/wonder-linux ./cmd/wonder)

log_info "Copying wonder binary to workers..."
docker cp ../../bin/wonder-linux k8s-node-1:/usr/local/bin/wonder
docker cp ../../bin/wonder-linux k8s-node-2:/usr/local/bin/wonder
docker cp ../../bin/wonder-linux k8s-node-3:/usr/local/bin/wonder

COORDINATOR_URL="http://nginx"

log_info "Worker nodes joining mesh..."

log_info "  k8s-node-1 joining mesh..."
docker exec k8s-node-1 wonder worker join --coordinator-url="$COORDINATOR_URL" "$JOIN_TOKEN" \
    2>&1 || log_warn "wonder worker join returned non-zero for k8s-node-1"

log_info "  k8s-node-2 joining mesh..."
docker exec k8s-node-2 wonder worker join --coordinator-url="$COORDINATOR_URL" "$JOIN_TOKEN" \
    2>&1 || log_warn "wonder worker join returned non-zero for k8s-node-2"

log_info "  k8s-node-3 joining mesh..."
docker exec k8s-node-3 wonder worker join --coordinator-url="$COORDINATOR_URL" "$JOIN_TOKEN" \
    2>&1 || log_warn "wonder worker join returned non-zero for k8s-node-3"

sleep 5

log_info "Verifying mesh connectivity..."
NODE1_IP=$(docker exec k8s-node-1 tailscale ip -4 2>/dev/null || echo "")
NODE2_IP=$(docker exec k8s-node-2 tailscale ip -4 2>/dev/null || echo "")
NODE3_IP=$(docker exec k8s-node-3 tailscale ip -4 2>/dev/null || echo "")

log_info "  k8s-node-1 IP: $NODE1_IP"
log_info "  k8s-node-2 IP: $NODE2_IP"
log_info "  k8s-node-3 IP: $NODE3_IP"

if [ -z "$NODE1_IP" ] || [ -z "$NODE2_IP" ] || [ -z "$NODE3_IP" ]; then
    log_error "Failed to get worker mesh IPs"
    exit 1
fi

log_info "Testing mesh connectivity..."
docker exec k8s-node-1 ping -c 2 "$NODE2_IP" >/dev/null && log_info "  k8s-node-1 -> k8s-node-2: OK"
docker exec k8s-node-1 ping -c 2 "$NODE3_IP" >/dev/null && log_info "  k8s-node-1 -> k8s-node-3: OK"

log_info "Creating API key for deployer via Admin API..."
API_KEY_RESPONSE=$(docker exec kubeadm-deployer curl -s -X POST \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"name": "kubeadm-deployer", "expires_in": "24h"}' \
    "http://nginx/coordinator/admin/api/v1/wonder-nets/$WONDER_NET_ID/api-keys")

API_KEY=$(echo "$API_KEY_RESPONSE" | jq -r '.key // empty')
if [ -z "$API_KEY" ]; then
    log_error "Failed to create API key"
    echo "$API_KEY_RESPONSE"
    exit 1
fi
log_info "API key created"

log_info "Deployer joining mesh via Admin API..."
DEPLOYER_JOIN_RESPONSE=$(docker exec kubeadm-deployer curl -s -X POST \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    "http://nginx/coordinator/admin/api/v1/wonder-nets/$WONDER_NET_ID/deployer/join")

DEPLOYER_AUTHKEY=$(echo "$DEPLOYER_JOIN_RESPONSE" | jq -r '.tailscale_connection_info.authkey // empty')
DEPLOYER_LOGIN_SERVER=$(echo "$DEPLOYER_JOIN_RESPONSE" | jq -r '.tailscale_connection_info.login_server // empty')

if [ -z "$DEPLOYER_AUTHKEY" ]; then
    log_error "Failed to get authkey for deployer"
    echo "$DEPLOYER_JOIN_RESPONSE"
    exit 1
fi

log_info "Starting userspace tailscaled in deployer..."
docker exec -d kubeadm-deployer tailscaled \
    --tun=userspace-networking \
    --socks5-server=:1080 \
    --state=/tmp/tailscale.state \
    --socket=/tmp/tailscaled.sock

sleep 3

log_info "Deployer connecting to mesh..."
docker exec kubeadm-deployer tailscale --socket=/tmp/tailscaled.sock up \
    --authkey="$DEPLOYER_AUTHKEY" \
    --login-server="$DEPLOYER_LOGIN_SERVER" \
    2>&1 || log_warn "Tailscale up returned non-zero for deployer"

sleep 3

log_info "Deployer tailscale status:"
docker exec kubeadm-deployer tailscale --socket=/tmp/tailscaled.sock status || true

log_info "Verifying nodes visible from coordinator (Admin API)..."
NODES_RESPONSE=$(docker exec kubeadm-deployer curl -s \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    "http://nginx/coordinator/admin/api/v1/wonder-nets/$WONDER_NET_ID/nodes")

NODE_COUNT=$(echo "$NODES_RESPONSE" | jq -r '.count // 0' 2>/dev/null || echo 0)
log_info "Nodes visible: $NODE_COUNT"

if [ "$NODE_COUNT" -lt 3 ]; then
    log_warn "Expected 3 nodes, got $NODE_COUNT"
    echo "$NODES_RESPONSE" | jq . 2>/dev/null || echo "$NODES_RESPONSE"
fi

echo ""
echo "==========================================="
echo "  Running Kubeadm Deployer"
echo "==========================================="

log_info "Starting kubeadm-deployer..."
docker exec kubeadm-deployer kubeadm-deployer \
    --coordinator-url="http://nginx/coordinator" \
    --api-key="$API_KEY" \
    --verbose

DEPLOY_EXIT=$?

if [ $DEPLOY_EXIT -eq 0 ]; then
    echo ""
    echo "==========================================="
    echo "  Deployment Complete!"
    echo "==========================================="
    log_info "Kubernetes cluster bootstrapped successfully"

    echo ""
    log_info "Cluster status:"
    docker exec kubeadm-deployer kubectl --kubeconfig /tmp/kubeconfig get nodes -o wide || true

    echo ""
    log_info "To interact with the cluster:"
    echo "  docker exec kubeadm-deployer kubectl --kubeconfig /tmp/kubeconfig get nodes"
    echo "  docker exec kubeadm-deployer kubectl --kubeconfig /tmp/kubeconfig get pods -A"

    echo ""
    log_info "To keep containers running for exploration:"
    echo "  NO_CLEAN=1 ./run-demo.sh"
else
    log_error "Deployment failed with exit code $DEPLOY_EXIT"
    exit 1
fi
