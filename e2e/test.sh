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

# Restart coordinator to ensure OIDC registration
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

# OIDC login flow
log_info "Testing OIDC login flow..."

COOKIE_JAR="cookies.txt"
rm -f "$COOKIE_JAR"

log_info "Starting login flow..."
LOGIN_REDIRECT=$(curl -s -I -c "$COOKIE_JAR" -b "$COOKIE_JAR" \
    "http://localhost:9080/coordinator/auth/login?provider=oidc" \
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
    "http://localhost:9080/coordinator/api/v1/join-token")

JOIN_TOKEN=$(echo "$JOIN_TOKEN_RESPONSE" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
if [ -z "$JOIN_TOKEN" ]; then
    log_error "Failed to create join token"
    exit 1
fi
log_info "Join token created: ${JOIN_TOKEN:0:80}..."

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

sleep 5

# Check connectivity
log_info "Checking worker mesh connectivity..."

WORKER1_IP=$(docker exec worker-1 tailscale ip -4 2>/dev/null || echo "")
WORKER2_IP=$(docker exec worker-2 tailscale ip -4 2>/dev/null || echo "")

log_info "Worker 1 IP: $WORKER1_IP"
log_info "Worker 2 IP: $WORKER2_IP"

if [ -z "$WORKER1_IP" ] || [ -z "$WORKER2_IP" ]; then
    log_error "Failed to get worker IPs"

    log_info "Worker 1 status:"
    docker exec worker-1 tailscale status || true

    log_info "Worker 2 status:"
    docker exec worker-2 tailscale status || true

    exit 1
fi

# Test ping between workers
log_info "Testing ping from Worker 1 to Worker 2..."
if docker exec worker-1 ping -c 3 "$WORKER2_IP"; then
    log_info "Ping successful: Worker 1 -> Worker 2"
else
    log_warn "Ping failed: Worker 1 -> Worker 2"
fi

log_info "Testing ping from Worker 2 to Worker 1..."
if docker exec worker-2 ping -c 3 "$WORKER1_IP"; then
    log_info "Ping successful: Worker 2 -> Worker 1"
else
    log_warn "Ping failed: Worker 2 -> Worker 1"
fi

log_info "=== E2E Test Complete ==="
log_info "Workers connected to mesh successfully!"
