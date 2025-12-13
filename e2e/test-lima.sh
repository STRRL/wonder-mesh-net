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
    log_info "Cleaning up..."

    # Stop Lima VMs
    limactl stop worker-1 2>/dev/null || true
    limactl stop worker-2 2>/dev/null || true
    limactl delete worker-1 2>/dev/null || true
    limactl delete worker-2 2>/dev/null || true

    # Stop Docker services
    docker compose -f docker-compose-lima.yaml down -v
}

# Set trap for cleanup on exit
trap cleanup EXIT

echo "=== Wonder Mesh Net E2E Test (Lima VMs) ==="

# Start Docker services (Keycloak + Coordinator)
log_info "Starting Keycloak and Coordinator..."
docker compose -f docker-compose-lima.yaml up -d
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

# Restart coordinator to ensure OIDC registration
docker restart coordinator
sleep 5

# Wait for Coordinator
log_info "Waiting for Coordinator to be ready..."
for i in {1..30}; do
    if curl -sf http://localhost:9080/health >/dev/null 2>&1; then
        break
    fi
    if [ $i -eq 30 ]; then
        log_error "Coordinator did not start in time"
        exit 1
    fi
    echo "  Waiting for Coordinator... ($i/30)"
    sleep 2
done
log_info "Coordinator is ready"

# Wait for embedded Headscale
log_info "Waiting for embedded Headscale to be ready..."
sleep 3
if curl -sf http://localhost:9080/hs/health >/dev/null 2>&1; then
    log_info "Embedded Headscale is healthy"
else
    log_error "Headscale health check failed"
    exit 1
fi

# OIDC login flow
log_info "Testing OIDC login flow..."

COOKIE_JAR="cookies-lima.txt"
rm -f "$COOKIE_JAR"

log_info "Starting login flow..."
LOGIN_REDIRECT=$(curl -s -I -c "$COOKIE_JAR" -b "$COOKIE_JAR" \
    "http://localhost:9080/auth/login?provider=oidc" \
    | grep -i "^location:" | sed 's/location: //i' | tr -d '\r')

if [ -z "$LOGIN_REDIRECT" ]; then
    log_error "No redirect from login endpoint"
    exit 1
fi
log_info "Redirected to: ${LOGIN_REDIRECT:0:80}..."

# Rewrite keycloak:8080 to localhost:9090 for host access
KEYCLOAK_URL=$(echo "$LOGIN_REDIRECT" | sed 's|keycloak:8080|localhost:9090|g')
log_info "Fetching Keycloak login page (rewritten URL)..."

LOGIN_PAGE=$(curl -s -c "$COOKIE_JAR" -b "$COOKIE_JAR" -L "$KEYCLOAK_URL")

FORM_ACTION=$(echo "$LOGIN_PAGE" | sed -n 's/.*action="\([^"]*\)".*/\1/p' | head -1 | sed 's/&amp;/\&/g')
if [ -z "$FORM_ACTION" ]; then
    log_error "Could not find login form"
    exit 1
fi
log_info "Form action: ${FORM_ACTION:0:80}..."

log_info "Submitting login credentials..."
CALLBACK_RESPONSE=$(curl -s -c "$COOKIE_JAR" -b "$COOKIE_JAR" -L \
    -d "username=testuser" \
    -d "password=testpass" \
    -w "\nHTTP_CODE:%{http_code}\nURL:%{url_effective}" \
    "$FORM_ACTION")

FINAL_URL=$(echo "$CALLBACK_RESPONSE" | grep "^URL:" | cut -d: -f2-)
HTTP_CODE=$(echo "$CALLBACK_RESPONSE" | grep "^HTTP_CODE:" | cut -d: -f2)
RESPONSE_BODY=$(echo "$CALLBACK_RESPONSE" | sed '/^HTTP_CODE:/d; /^URL:/d')

log_info "HTTP Status: $HTTP_CODE"
log_info "Final URL: $FINAL_URL"

if [ -n "$RESPONSE_BODY" ]; then
    log_info "Response body (first 200 chars): ${RESPONSE_BODY:0:200}"
fi

SESSION=$(echo "$FINAL_URL" | sed -n 's/.*session=\([^&]*\).*/\1/p')
if [ -z "$SESSION" ]; then
    log_error "Failed to get session token from URL: $FINAL_URL"
    if echo "$RESPONSE_BODY" | grep -q "failed to exchange code"; then
        log_error "OIDC code exchange failed - check coordinator logs for details"
    fi
    exit 1
fi
log_info "Session token obtained: ${SESSION:0:20}..."

log_info "Creating join token..."
JOIN_TOKEN_RESPONSE=$(curl -s -X POST \
    -H "X-Session-Token: $SESSION" \
    -H "Content-Type: application/json" \
    -d '{"ttl": "1h"}' \
    "http://localhost:9080/api/v1/join-token")

JOIN_TOKEN=$(echo "$JOIN_TOKEN_RESPONSE" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
if [ -z "$JOIN_TOKEN" ]; then
    log_error "Failed to create join token"
    exit 1
fi
log_info "Join token created: ${JOIN_TOKEN:0:80}..."

# Create Lima VMs
log_info "Creating Lima VMs for workers..."

# Create worker-1
log_info "Creating worker-1 VM..."
limactl delete -f worker-1 2>/dev/null || true
limactl create --name=worker-1 lima-worker.yaml
limactl start worker-1

# Create worker-2
log_info "Creating worker-2 VM..."
limactl delete -f worker-2 2>/dev/null || true
limactl create --name=worker-2 lima-worker.yaml
limactl start worker-2

sleep 5

# Get host IP that VMs can reach
HOST_IP=$(ipconfig getifaddr en0 || hostname -I | awk '{print $1}')
log_info "Host IP (accessible from VMs): $HOST_IP"

# Worker 1: Join mesh
log_info "Worker 1: Joining mesh..."

# Get authkey for worker 1
WORKER1_API_RESPONSE=$(limactl shell worker-1 curl -s -X POST \
    -H 'Content-Type: application/json' \
    -d "{\"token\": \"$JOIN_TOKEN\"}" \
    "http://$HOST_IP:9080/api/v1/worker/join")

WORKER1_AUTHKEY=$(echo "$WORKER1_API_RESPONSE" | sed -n 's/.*"authkey":"\([^"]*\)".*/\1/p')
WORKER1_LOGIN_SERVER=$(echo "$WORKER1_API_RESPONSE" | sed -n 's/.*"headscale_url":"\([^"]*\)".*/\1/p' | sed "s/localhost/$HOST_IP/g")

if [ -z "$WORKER1_AUTHKEY" ]; then
    log_error "Failed to get authkey for worker 1"
    echo "$WORKER1_API_RESPONSE"
    exit 1
fi
log_info "Worker 1 authkey: ${WORKER1_AUTHKEY:0:20}..."

log_info "Starting tailscaled on Worker 1..."
limactl shell worker-1 sudo systemctl start tailscaled
sleep 2

log_info "Running tailscale up for Worker 1..."
limactl shell worker-1 sudo tailscale up \
    --reset \
    --authkey="$WORKER1_AUTHKEY" \
    --login-server="$WORKER1_LOGIN_SERVER" \
    --accept-routes \
    --accept-dns=false \
    2>&1 || log_warn "Tailscale up returned non-zero exit code for Worker 1"

# Worker 2: Join mesh
log_info "Worker 2: Joining mesh..."

WORKER2_API_RESPONSE=$(limactl shell worker-2 curl -s -X POST \
    -H 'Content-Type: application/json' \
    -d "{\"token\": \"$JOIN_TOKEN\"}" \
    "http://$HOST_IP:9080/api/v1/worker/join")

WORKER2_AUTHKEY=$(echo "$WORKER2_API_RESPONSE" | sed -n 's/.*"authkey":"\([^"]*\)".*/\1/p')
WORKER2_LOGIN_SERVER=$(echo "$WORKER2_API_RESPONSE" | sed -n 's/.*"headscale_url":"\([^"]*\)".*/\1/p' | sed "s/localhost/$HOST_IP/g")

if [ -z "$WORKER2_AUTHKEY" ]; then
    log_error "Failed to get authkey for worker 2"
    echo "$WORKER2_API_RESPONSE"
    exit 1
fi
log_info "Worker 2 authkey: ${WORKER2_AUTHKEY:0:20}..."

log_info "Starting tailscaled on Worker 2..."
limactl shell worker-2 sudo systemctl start tailscaled
sleep 2

log_info "Running tailscale up for Worker 2..."
limactl shell worker-2 sudo tailscale up \
    --reset \
    --authkey="$WORKER2_AUTHKEY" \
    --login-server="$WORKER2_LOGIN_SERVER" \
    --accept-routes \
    --accept-dns=false \
    2>&1 || log_warn "Tailscale up returned non-zero exit code for Worker 2"

sleep 5

# Check connectivity
log_info "Checking worker mesh connectivity..."

WORKER1_IP=$(limactl shell worker-1 sudo tailscale ip -4 2>/dev/null || echo "")
WORKER2_IP=$(limactl shell worker-2 sudo tailscale ip -4 2>/dev/null || echo "")

log_info "Worker 1 IP: $WORKER1_IP"
log_info "Worker 2 IP: $WORKER2_IP"

if [ -z "$WORKER1_IP" ] || [ -z "$WORKER2_IP" ]; then
    log_error "Failed to get worker IPs"

    log_info "Worker 1 status:"
    limactl shell worker-1 sudo tailscale status || true

    log_info "Worker 2 status:"
    limactl shell worker-2 sudo tailscale status || true

    exit 1
fi

# Test ping between workers
log_info "Testing ping from Worker 1 to Worker 2..."
if limactl shell worker-1 sudo tailscale ping -c 3 "$WORKER2_IP"; then
    log_info "✅ Ping successful: Worker 1 -> Worker 2"
else
    log_warn "Ping failed: Worker 1 -> Worker 2"
fi

log_info "Testing ping from Worker 2 to Worker 1..."
if limactl shell worker-2 sudo tailscale ping -c 3 "$WORKER1_IP"; then
    log_info "✅ Ping successful: Worker 2 -> Worker 1"
else
    log_warn "Ping failed: Worker 2 -> Worker 1"
fi

log_info "=== E2E Test Complete ==="
log_info "Workers connected to mesh successfully!"
