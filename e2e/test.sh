#!/bin/bash
set -e

# Color output functions
log_info() {
    echo -e "\033[0;32m[INFO]\033[0m $1"
}

log_warn() {
    echo -e "\033[1;33m[WARN]\033[0m $1"
}

log_error() {
    echo -e "\033[0;31m[ERROR]\033[0m $1"
}

# Cleanup function
cleanup() {
    if [ "${NO_CLEANUP:-0}" = "1" ]; then
        log_info "NO_CLEANUP=1, skipping cleanup. Containers are still running."
        return
    fi
    log_info "Cleaning up..."
    docker compose -f docker-compose.yaml down -v --remove-orphans 2>/dev/null || true
}

# Set trap for cleanup on exit
trap cleanup EXIT

echo "=== Wonder Mesh Net E2E Test ==="

# Start all Docker services
log_info "Starting all services..."
docker compose -f docker-compose.yaml up -d --build
sleep 10

# Wait for Keycloak
log_info "Waiting for Keycloak to be ready..."
for i in {1..60}; do
    if curl -sf http://localhost:9090/health >/dev/null 2>&1 || curl -sf http://localhost:9090/ >/dev/null 2>&1; then
        break
    fi
    if [ $i -eq 60 ]; then
        log_error "Keycloak did not start in time"
        exit 1
    fi
    echo "  Waiting for Keycloak... ($i/60)"
    sleep 2
done
log_info "Keycloak is ready"

# Restart coordinator to ensure Keycloak connection
docker restart coordinator
sleep 5

# Wait for Coordinator
log_info "Waiting for Coordinator to be ready..."
for i in {1..30}; do
    if curl -sf http://localhost:9080/coordinator/health >/dev/null 2>&1; then
        break
    fi
    if [ $i -eq 30 ]; then
        log_error "Coordinator did not start in time"
        docker logs coordinator 2>&1 | tail -50
        exit 1
    fi
    echo "  Waiting for Coordinator... ($i/30)"
    sleep 2
done
log_info "Coordinator is ready"

# Wait for embedded Headscale (now at root path)
log_info "Waiting for embedded Headscale to be ready..."
sleep 3
if curl -sf http://localhost:9080/health >/dev/null 2>&1; then
    log_info "Embedded Headscale is healthy"
else
    log_error "Headscale health check failed"
    exit 1
fi

# ============================================
# Get access token from Keycloak using ROPC
# ============================================
log_info "Getting access token from Keycloak..."

TOKEN_RESPONSE=$(curl -s -X POST \
    "http://localhost:9090/realms/wonder/protocol/openid-connect/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=password" \
    -d "client_id=wonder-mesh-net" \
    -d "client_secret=wonder-secret" \
    -d "username=testuser" \
    -d "password=testpass")

ACCESS_TOKEN=$(echo "$TOKEN_RESPONSE" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
if [ -z "$ACCESS_TOKEN" ]; then
    log_error "Failed to get access token from Keycloak"
    echo "$TOKEN_RESPONSE"
    exit 1
fi
log_info "Access token obtained: ${ACCESS_TOKEN:0:50}..."

# ============================================
# Test protected endpoints with JWT
# ============================================
log_info "Creating join token..."
JOIN_TOKEN_RESPONSE=$(curl -s -X POST \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    -H "Content-Type: application/json" \
    "http://localhost:9080/coordinator/api/v1/join-token")

JOIN_TOKEN=$(echo "$JOIN_TOKEN_RESPONSE" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
if [ -z "$JOIN_TOKEN" ]; then
    log_error "Failed to create join token"
    echo "$JOIN_TOKEN_RESPONSE"
    exit 1
fi
log_info "Join token created: ${JOIN_TOKEN:0:80}..."

log_info "Testing nodes endpoint..."
NODES_BY_TOKEN=$(curl -s \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    "http://localhost:9080/coordinator/api/v1/nodes")

if ! echo "$NODES_BY_TOKEN" | grep -q '"nodes"'; then
    log_error "Failed to get nodes with access token"
    echo "$NODES_BY_TOKEN"
    exit 1
fi
log_info "Nodes endpoint works with access token"

# Get host IP that containers can reach (coordinator uses host network)
HOST_IP="host.docker.internal"
# On Linux, host.docker.internal may not work, use docker bridge gateway
if ! docker exec worker-1 ping -c 1 -W 1 host.docker.internal >/dev/null 2>&1; then
    HOST_IP=$(docker network inspect bridge -f '{{range .IPAM.Config}}{{.Gateway}}{{end}}' 2>/dev/null || echo "172.17.0.1")
fi
log_info "Host IP (accessible from containers): $HOST_IP"

# Worker 1: Join mesh
log_info "Worker 1: Joining mesh..."

# Start tailscaled in worker-1
docker exec worker-1 tailscaled --state=/var/lib/tailscale/tailscaled.state --socket=/var/run/tailscale/tailscaled.sock &
sleep 3

# Get authkey for worker 1
WORKER1_API_RESPONSE=$(docker exec worker-1 curl -s -X POST \
    -H 'Content-Type: application/json' \
    -d "{\"token\": \"$JOIN_TOKEN\"}" \
    "http://$HOST_IP:9080/coordinator/api/v1/worker/join")

WORKER1_AUTHKEY=$(echo "$WORKER1_API_RESPONSE" | sed -n 's/.*"authkey":"\([^"]*\)".*/\1/p')
WORKER1_LOGIN_SERVER=$(echo "$WORKER1_API_RESPONSE" | sed -n 's/.*"headscale_url":"\([^"]*\)".*/\1/p' | sed "s/localhost/$HOST_IP/g")

if [ -z "$WORKER1_AUTHKEY" ]; then
    log_error "Failed to get authkey for worker 1"
    echo "$WORKER1_API_RESPONSE"
    exit 1
fi
log_info "Worker 1 authkey: ${WORKER1_AUTHKEY:0:20}..."
log_info "Worker 1 login server: $WORKER1_LOGIN_SERVER"

log_info "Running tailscale up for Worker 1..."
docker exec worker-1 tailscale up \
    --reset \
    --authkey="$WORKER1_AUTHKEY" \
    --login-server="$WORKER1_LOGIN_SERVER" \
    --accept-routes \
    --accept-dns=false \
    2>&1 || log_warn "Tailscale up returned non-zero exit code for Worker 1"

# Worker 2: Join mesh
log_info "Worker 2: Joining mesh..."

# Start tailscaled in worker-2
docker exec worker-2 tailscaled --state=/var/lib/tailscale/tailscaled.state --socket=/var/run/tailscale/tailscaled.sock &
sleep 3

WORKER2_API_RESPONSE=$(docker exec worker-2 curl -s -X POST \
    -H 'Content-Type: application/json' \
    -d "{\"token\": \"$JOIN_TOKEN\"}" \
    "http://$HOST_IP:9080/coordinator/api/v1/worker/join")

WORKER2_AUTHKEY=$(echo "$WORKER2_API_RESPONSE" | sed -n 's/.*"authkey":"\([^"]*\)".*/\1/p')
WORKER2_LOGIN_SERVER=$(echo "$WORKER2_API_RESPONSE" | sed -n 's/.*"headscale_url":"\([^"]*\)".*/\1/p' | sed "s/localhost/$HOST_IP/g")

if [ -z "$WORKER2_AUTHKEY" ]; then
    log_error "Failed to get authkey for worker 2"
    echo "$WORKER2_API_RESPONSE"
    exit 1
fi
log_info "Worker 2 authkey: ${WORKER2_AUTHKEY:0:20}..."
log_info "Worker 2 login server: $WORKER2_LOGIN_SERVER"

log_info "Running tailscale up for Worker 2..."
docker exec worker-2 tailscale up \
    --reset \
    --authkey="$WORKER2_AUTHKEY" \
    --login-server="$WORKER2_LOGIN_SERVER" \
    --accept-routes \
    --accept-dns=false \
    2>&1 || log_warn "Tailscale up returned non-zero exit code for Worker 2"

# Worker 3: Join mesh
log_info "Worker 3: Joining mesh..."

# Start tailscaled in worker-3
docker exec worker-3 tailscaled --state=/var/lib/tailscale/tailscaled.state --socket=/var/run/tailscale/tailscaled.sock &
sleep 3

WORKER3_API_RESPONSE=$(docker exec worker-3 curl -s -X POST \
    -H 'Content-Type: application/json' \
    -d "{\"token\": \"$JOIN_TOKEN\"}" \
    "http://$HOST_IP:9080/coordinator/api/v1/worker/join")

WORKER3_AUTHKEY=$(echo "$WORKER3_API_RESPONSE" | sed -n 's/.*"authkey":"\([^"]*\)".*/\1/p')
WORKER3_LOGIN_SERVER=$(echo "$WORKER3_API_RESPONSE" | sed -n 's/.*"headscale_url":"\([^"]*\)".*/\1/p' | sed "s/localhost/$HOST_IP/g")

if [ -z "$WORKER3_AUTHKEY" ]; then
    log_error "Failed to get authkey for worker 3"
    echo "$WORKER3_API_RESPONSE"
    exit 1
fi
log_info "Worker 3 authkey: ${WORKER3_AUTHKEY:0:20}..."
log_info "Worker 3 login server: $WORKER3_LOGIN_SERVER"

log_info "Running tailscale up for Worker 3..."
docker exec worker-3 tailscale up \
    --reset \
    --authkey="$WORKER3_AUTHKEY" \
    --login-server="$WORKER3_LOGIN_SERVER" \
    --accept-routes \
    --accept-dns=false \
    2>&1 || log_warn "Tailscale up returned non-zero exit code for Worker 3"

sleep 5

# Check connectivity
log_info "Checking worker mesh connectivity..."

WORKER1_IP=$(docker exec worker-1 tailscale ip -4 2>/dev/null || echo "")
WORKER2_IP=$(docker exec worker-2 tailscale ip -4 2>/dev/null || echo "")
WORKER3_IP=$(docker exec worker-3 tailscale ip -4 2>/dev/null || echo "")

log_info "Worker 1 IP: $WORKER1_IP"
log_info "Worker 2 IP: $WORKER2_IP"
log_info "Worker 3 IP: $WORKER3_IP"

if [ -z "$WORKER1_IP" ] || [ -z "$WORKER2_IP" ] || [ -z "$WORKER3_IP" ]; then
    log_error "Failed to get worker IPs"

    log_info "Worker 1 status:"
    docker exec worker-1 tailscale status || true

    log_info "Worker 2 status:"
    docker exec worker-2 tailscale status || true

    log_info "Worker 3 status:"
    docker exec worker-3 tailscale status || true

    exit 1
fi

# Test ping between workers (full mesh)
log_info "Testing ping from Worker 1 to Worker 2..."
if docker exec worker-1 ping -c 3 "$WORKER2_IP"; then
    log_info "Ping successful: Worker 1 -> Worker 2"
else
    log_warn "Ping failed: Worker 1 -> Worker 2"
fi

log_info "Testing ping from Worker 1 to Worker 3..."
if docker exec worker-1 ping -c 3 "$WORKER3_IP"; then
    log_info "Ping successful: Worker 1 -> Worker 3"
else
    log_warn "Ping failed: Worker 1 -> Worker 3"
fi

log_info "Testing ping from Worker 2 to Worker 1..."
if docker exec worker-2 ping -c 3 "$WORKER1_IP"; then
    log_info "Ping successful: Worker 2 -> Worker 1"
else
    log_warn "Ping failed: Worker 2 -> Worker 1"
fi

log_info "Testing ping from Worker 2 to Worker 3..."
if docker exec worker-2 ping -c 3 "$WORKER3_IP"; then
    log_info "Ping successful: Worker 2 -> Worker 3"
else
    log_warn "Ping failed: Worker 2 -> Worker 3"
fi

log_info "Testing ping from Worker 3 to Worker 1..."
if docker exec worker-3 ping -c 3 "$WORKER1_IP"; then
    log_info "Ping successful: Worker 3 -> Worker 1"
else
    log_warn "Ping failed: Worker 3 -> Worker 1"
fi

log_info "Testing ping from Worker 3 to Worker 2..."
if docker exec worker-3 ping -c 3 "$WORKER2_IP"; then
    log_info "Ping successful: Worker 3 -> Worker 2"
else
    log_warn "Ping failed: Worker 3 -> Worker 2"
fi

# ============================================
# Verify nodes endpoint after workers joined
# ============================================
log_info "=== Verifying nodes visible after workers joined ==="

NODES_FINAL=$(curl -s \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    "http://localhost:9080/coordinator/api/v1/nodes")

NODE_COUNT=$(echo "$NODES_FINAL" | sed -n 's/.*"count":\([0-9]*\).*/\1/p')
log_info "Nodes visible: $NODE_COUNT"

if [ "$NODE_COUNT" -lt 3 ]; then
    log_warn "Expected 3 nodes, got $NODE_COUNT"
    echo "$NODES_FINAL"
else
    log_info "All 3 workers visible"
fi

if echo "$NODES_FINAL" | grep -q "$WORKER1_IP"; then
    log_info "Worker 1 IP ($WORKER1_IP) found in API response"
else
    log_warn "Worker 1 IP not found in API response"
fi

if echo "$NODES_FINAL" | grep -q "$WORKER2_IP"; then
    log_info "Worker 2 IP ($WORKER2_IP) found in API response"
else
    log_warn "Worker 2 IP not found in API response"
fi

if echo "$NODES_FINAL" | grep -q "$WORKER3_IP"; then
    log_info "Worker 3 IP ($WORKER3_IP) found in API response"
else
    log_warn "Worker 3 IP not found in API response"
fi

# ============================================
# Deployer Test (using API Key)
# ============================================
log_info "=== Testing Deployer with API Key ==="

# Step 1: Create an API key for the deployer
log_info "Creating API key for deployer..."
API_KEY_RESPONSE=$(curl -s -X POST \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"name": "deployer-key", "expires_in": "24h"}' \
    "http://localhost:9080/coordinator/api/v1/api-keys")

API_KEY=$(echo "$API_KEY_RESPONSE" | sed -n 's/.*"key":"\([^"]*\)".*/\1/p')
API_KEY_ID=$(echo "$API_KEY_RESPONSE" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')

if [ -z "$API_KEY" ]; then
    log_error "Failed to create API key"
    echo "$API_KEY_RESPONSE"
    exit 1
fi
log_info "API key created: ${API_KEY:0:20}..."

# Step 2: List API keys to verify
log_info "Listing API keys..."
API_KEYS_LIST=$(curl -s \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    "http://localhost:9080/coordinator/api/v1/api-keys")

if ! echo "$API_KEYS_LIST" | grep -q "$API_KEY_ID"; then
    log_warn "API key not found in list"
    echo "$API_KEYS_LIST"
fi
log_info "API key listing works"

# Step 3: Test API key can access nodes endpoint (read-only)
log_info "Testing API key access to nodes endpoint..."
NODES_WITH_API_KEY=$(curl -s \
    -H "Authorization: Bearer $API_KEY" \
    "http://localhost:9080/coordinator/api/v1/nodes")

if echo "$NODES_WITH_API_KEY" | grep -q "nodes"; then
    log_info "API key can access nodes endpoint (read-only access works)"
else
    log_error "API key cannot access nodes endpoint"
    echo "$NODES_WITH_API_KEY"
    exit 1
fi

# Step 4: Deployer joins mesh using API key
log_info "Deployer joining mesh with API key..."
DEPLOYER_JOIN_RESPONSE=$(docker exec deployer curl -s -X POST \
    -H "Authorization: Bearer $API_KEY" \
    -H "Content-Type: application/json" \
    "http://$HOST_IP:9080/coordinator/api/v1/deployer/join")

DEPLOYER_AUTHKEY=$(echo "$DEPLOYER_JOIN_RESPONSE" | sed -n 's/.*"authkey":"\([^"]*\)".*/\1/p')
DEPLOYER_LOGIN_SERVER=$(echo "$DEPLOYER_JOIN_RESPONSE" | sed -n 's/.*"headscale_url":"\([^"]*\)".*/\1/p' | sed "s/localhost/$HOST_IP/g")

if [ -z "$DEPLOYER_AUTHKEY" ]; then
    log_error "Failed to get authkey for deployer"
    echo "$DEPLOYER_JOIN_RESPONSE"
    exit 1
fi
log_info "Deployer authkey: ${DEPLOYER_AUTHKEY:0:20}..."
log_info "Deployer login server: $DEPLOYER_LOGIN_SERVER"

# Start userspace tailscaled in deployer
log_info "Starting userspace tailscaled in deployer..."
docker exec -d deployer tailscaled \
    --tun=userspace-networking \
    --socks5-server=:1080 \
    --state=/tmp/tailscale.state \
    --socket=/tmp/tailscaled.sock

sleep 3

# Join mesh
log_info "Deployer joining mesh..."
docker exec deployer tailscale --socket=/tmp/tailscaled.sock up \
    --authkey="$DEPLOYER_AUTHKEY" \
    --login-server="$DEPLOYER_LOGIN_SERVER" \
    2>&1 || log_warn "Tailscale up returned non-zero exit code for Deployer"

sleep 3

# Check deployer status
log_info "Deployer tailscale status:"
docker exec deployer tailscale --socket=/tmp/tailscaled.sock status || true

# SSH to worker-1 via SOCKS5 and deploy app
log_info "Deploying app to Worker 1 via SSH over SOCKS5..."
docker exec deployer sshpass -p 'worker' ssh -T \
    -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    -o ConnectTimeout=10 \
    -o ProxyCommand="nc -x localhost:1080 %h %p" \
    root@$WORKER1_IP \
    'sh -c "echo \"Hello from deployed app\" > /tmp/index.html && nohup python3 -m http.server 8080 -d /tmp > /tmp/httpd.log 2>&1 &"'

SSH_EXIT=$?
if [ $SSH_EXIT -ne 0 ]; then
    log_error "SSH command failed with exit code $SSH_EXIT"
    exit 1
fi
log_info "SSH deploy command completed"

# Wait for HTTP server to start and retry
log_info "Accessing deployed app via mesh..."
for i in 1 2 3 4 5; do
    sleep 2
    APP_RESPONSE=$(docker exec deployer curl -s --connect-timeout 10 --socks5-hostname localhost:1080 \
        "http://$WORKER1_IP:8080/index.html" 2>/dev/null || true)
    if echo "$APP_RESPONSE" | grep -q "Hello from deployed app"; then
        log_info "Deployer test PASSED: App accessible via mesh"
        break
    fi
    if [ $i -lt 5 ]; then
        log_info "Retry $i: HTTP server not ready yet, waiting..."
    fi
done

if ! echo "$APP_RESPONSE" | grep -q "Hello from deployed app"; then
    log_error "Deployer test FAILED"
    echo "Response: $APP_RESPONSE"
    exit 1
fi

log_info "=== Deployer Test Complete ==="

log_info "=== E2E Test Complete ==="
log_info "3 workers connected to mesh successfully!"
log_info "Keycloak JWT authentication verified!"
log_info "API key created and used for deployer authentication!"
log_info "Deployer test passed - apps can be deployed and accessed via mesh!"
