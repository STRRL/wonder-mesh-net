#!/bin/bash
# Kubeadm Deployer Demo Runner
# Bootstraps a 3-node Kubernetes cluster using Wonder Mesh Net
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

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
echo "  Kubeadm Deployer Demo"
echo "  Wonder Mesh Net + Kubernetes"
echo "==========================================="

log_info "Starting all services..."
docker compose up -d --build --force-recreate
sleep 15

log_info "Waiting for Keycloak to be ready..."
for i in {1..60}; do
    if curl -sf http://localhost:8080/realms/wonder/.well-known/openid-configuration >/dev/null 2>&1; then
        break
    fi
    if [ $i -eq 60 ]; then
        log_error "Keycloak did not start in time"
        docker logs kubeadm-keycloak 2>&1 | tail -30
        exit 1
    fi
    echo "  Waiting for Keycloak... ($i/60)"
    sleep 2
done
log_info "Keycloak is ready"

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

# NOTE: These credentials are for DEMO USE ONLY. Do not use in production.
# In production, use proper secret management and environment variables.
log_info "Getting access token from Keycloak..."
TOKEN_RESPONSE=$(docker exec kubeadm-deployer curl -s -X POST \
    "http://nginx/realms/wonder/protocol/openid-connect/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=password" \
    -d "client_id=wonder-mesh-net" \
    -d "client_secret=wonder-secret" \
    -d "username=testuser" \
    -d "password=testpass")

ACCESS_TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.access_token // empty')
if [ -z "$ACCESS_TOKEN" ]; then
    log_error "Failed to get access token (check Keycloak logs for details)"
    exit 1
fi
log_info "Access token obtained"

log_info "Creating join token..."
JOIN_TOKEN_RESPONSE=$(docker exec kubeadm-deployer curl -s \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    "http://nginx/coordinator/api/v1/join-token")

JOIN_TOKEN=$(echo "$JOIN_TOKEN_RESPONSE" | jq -r '.token // empty')
if [ -z "$JOIN_TOKEN" ]; then
    log_error "Failed to create join token (check coordinator logs for details)"
    exit 1
fi
log_info "Join token created"

log_info "Building wonder binary for Linux..."
(cd ../.. && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/wonder-linux ./cmd/wonder)

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

log_info "Creating API key for deployer..."
API_KEY_RESPONSE=$(docker exec kubeadm-deployer curl -s -X POST \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"name": "kubeadm-deployer", "expires_in": "24h"}' \
    "http://nginx/coordinator/api/v1/api-keys")

API_KEY=$(echo "$API_KEY_RESPONSE" | jq -r '.key // empty')
if [ -z "$API_KEY" ]; then
    log_error "Failed to create API key (check coordinator logs for details)"
    exit 1
fi
log_info "API key created"

log_info "Deployer joining mesh..."
DEPLOYER_JOIN_RESPONSE=$(docker exec kubeadm-deployer curl -s -X POST \
    -H "Authorization: Bearer $API_KEY" \
    -H "Content-Type: application/json" \
    "http://nginx/coordinator/api/v1/deployer/join")

DEPLOYER_AUTHKEY=$(echo "$DEPLOYER_JOIN_RESPONSE" | jq -r '.tailscale_connection_info.authkey // empty')
DEPLOYER_LOGIN_SERVER=$(echo "$DEPLOYER_JOIN_RESPONSE" | jq -r '.tailscale_connection_info.login_server // empty')

if [ -z "$DEPLOYER_AUTHKEY" ]; then
    log_error "Failed to get authkey for deployer (check coordinator logs for details)"
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

log_info "Verifying nodes visible from coordinator..."
NODES_RESPONSE=$(docker exec kubeadm-deployer curl -s \
    -H "Authorization: Bearer $API_KEY" \
    "http://nginx/coordinator/api/v1/nodes")

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
