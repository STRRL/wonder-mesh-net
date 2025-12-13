#!/bin/bash
set -e

cd "$(dirname "$0")"

echo "=== Wonder Mesh Net E2E Test (Embedded Headscale) ==="

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

cleanup() {
    log_info "Cleaning up..."
    docker compose down -v 2>/dev/null || true
    rm -f cookies.txt
}

trap cleanup EXIT

log_info "Starting Keycloak..."
docker compose up -d keycloak

log_info "Waiting for Keycloak to be ready..."
for i in {1..60}; do
    if curl -s http://localhost:9090/realms/wonder/.well-known/openid-configuration 2>/dev/null | grep -q "issuer"; then
        log_info "Keycloak is ready"
        break
    fi
    echo "  Waiting for Keycloak... ($i/60)"
    sleep 3
done

log_info "Starting Coordinator (with embedded Headscale)..."
docker compose up -d coordinator

log_info "Waiting for Coordinator to be ready..."
for i in {1..30}; do
    if curl -s http://localhost:9080/health 2>/dev/null | grep -q "ok"; then
        log_info "Coordinator is ready"
        break
    fi
    echo "  Waiting for Coordinator... ($i/30)"
    sleep 3
done

log_info "Waiting for embedded Headscale to be ready..."
for i in {1..30}; do
    if curl -s http://localhost:8080/health 2>/dev/null | grep -q "pass"; then
        log_info "Embedded Headscale is healthy"
        break
    fi
    echo "  Waiting for Headscale... ($i/30)"
    sleep 2
done

log_info "Testing OIDC login flow..."

COOKIE_JAR="cookies.txt"
rm -f "$COOKIE_JAR"

log_info "Starting login flow..."
LOGIN_REDIRECT=$(curl -s -I -c "$COOKIE_JAR" -b "$COOKIE_JAR" \
    "http://localhost:9080/auth/login?provider=oidc" \
    | grep -i "^location:" | sed 's/location: //i' | tr -d '\r')

if [ -z "$LOGIN_REDIRECT" ]; then
    log_error "No redirect from login endpoint"
    curl -s "http://localhost:9080/auth/login?provider=oidc"
    exit 1
fi
log_info "Redirected to: ${LOGIN_REDIRECT:0:80}..."

KEYCLOAK_URL="$LOGIN_REDIRECT"
log_info "Fetching Keycloak login page..."

LOGIN_PAGE=$(curl -s -c "$COOKIE_JAR" -b "$COOKIE_JAR" -L "$KEYCLOAK_URL")

FORM_ACTION=$(echo "$LOGIN_PAGE" | sed -n 's/.*action="\([^"]*\)".*/\1/p' | head -1 | sed 's/&amp;/\&/g')
if [ -z "$FORM_ACTION" ]; then
    log_error "Could not find login form"
    echo "$LOGIN_PAGE" | head -50
    exit 1
fi
log_info "Form action: ${FORM_ACTION:0:80}..."

log_info "Submitting login credentials..."
CALLBACK_RESPONSE=$(curl -s -c "$COOKIE_JAR" -b "$COOKIE_JAR" -L \
    -d "username=testuser" \
    -d "password=testpass" \
    -w "\n%{url_effective}" \
    "$FORM_ACTION")

FINAL_URL=$(echo "$CALLBACK_RESPONSE" | tail -1)
log_info "Final URL: $FINAL_URL"

SESSION=$(echo "$FINAL_URL" | sed -n 's/.*session=\([^&]*\).*/\1/p')
if [ -z "$SESSION" ]; then
    log_error "Failed to get session token"
    echo "$CALLBACK_RESPONSE"
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
    echo "$JOIN_TOKEN_RESPONSE"
    exit 1
fi
log_info "Join token created: ${JOIN_TOKEN:0:50}..."

log_info "Starting Worker containers..."
docker compose up -d worker-1 worker-2
sleep 3

log_info "Starting Tailscale daemons in workers..."
docker compose exec -T -d worker-1 tailscaled --state=/data/tailscaled.state --socket=/var/run/tailscale/tailscaled.sock
docker compose exec -T -d worker-2 tailscaled --state=/data/tailscaled.state --socket=/var/run/tailscale/tailscaled.sock
sleep 3

log_info "Worker 1: Joining mesh..."
WORKER1_API_RESPONSE=$(docker compose exec -T worker-1 sh -c "
    curl -s -X POST \
        -H 'Content-Type: application/json' \
        -d '{\"token\": \"$JOIN_TOKEN\"}' \
        'http://coordinator:9080/api/v1/worker/join'
")
WORKER1_AUTHKEY=$(echo "$WORKER1_API_RESPONSE" | sed -n 's/.*"authkey":"\([^"]*\)".*/\1/p')
WORKER1_LOGIN_SERVER=$(echo "$WORKER1_API_RESPONSE" | sed -n 's/.*"headscale_url":"\([^"]*\)".*/\1/p' | sed 's/localhost/coordinator/g')

if [ -z "$WORKER1_AUTHKEY" ]; then
    log_error "Failed to get authkey for worker 1"
    echo "$WORKER1_API_RESPONSE"
    exit 1
fi
log_info "Worker 1 authkey: ${WORKER1_AUTHKEY:0:20}..."

log_info "Running tailscale up for Worker 1..."
docker compose exec -T worker-1 sh -c "
    timeout 30 tailscale up \
        --reset \
        --authkey='$WORKER1_AUTHKEY' \
        --login-server='$WORKER1_LOGIN_SERVER' \
        --accept-routes \
        --accept-dns=false \
        2>&1
" || log_warn "Tailscale up timed out or failed for Worker 1"

log_info "Worker 2: Joining mesh..."
WORKER2_API_RESPONSE=$(docker compose exec -T worker-2 sh -c "
    curl -s -X POST \
        -H 'Content-Type: application/json' \
        -d '{\"token\": \"$JOIN_TOKEN\"}' \
        'http://coordinator:9080/api/v1/worker/join'
")
WORKER2_AUTHKEY=$(echo "$WORKER2_API_RESPONSE" | sed -n 's/.*"authkey":"\([^"]*\)".*/\1/p')
WORKER2_LOGIN_SERVER=$(echo "$WORKER2_API_RESPONSE" | sed -n 's/.*"headscale_url":"\([^"]*\)".*/\1/p' | sed 's/localhost/coordinator/g')

if [ -z "$WORKER2_AUTHKEY" ]; then
    log_error "Failed to get authkey for worker 2"
    echo "$WORKER2_API_RESPONSE"
    exit 1
fi
log_info "Worker 2 authkey: ${WORKER2_AUTHKEY:0:20}..."

log_info "Running tailscale up for Worker 2..."
docker compose exec -T worker-2 sh -c "
    timeout 30 tailscale up \
        --reset \
        --authkey='$WORKER2_AUTHKEY' \
        --login-server='$WORKER2_LOGIN_SERVER' \
        --accept-routes \
        --accept-dns=false \
        2>&1
" || log_warn "Tailscale up timed out or failed for Worker 2"

sleep 5

log_info "Checking worker mesh connectivity..."
WORKER1_IP=$(docker compose exec -T worker-1 tailscale ip -4 | tr -d '\r')
WORKER2_IP=$(docker compose exec -T worker-2 tailscale ip -4 | tr -d '\r')

log_info "Worker 1 IP: $WORKER1_IP"
log_info "Worker 2 IP: $WORKER2_IP"

if [ -z "$WORKER1_IP" ] || [ -z "$WORKER2_IP" ]; then
    log_error "Failed to get worker IPs"
    exit 1
fi

log_info "Testing Worker 1 -> Worker 2 connectivity..."
if docker compose exec -T worker-1 ping -c 3 "$WORKER2_IP"; then
    log_info "Worker 1 can reach Worker 2"
else
    log_error "Worker 1 cannot reach Worker 2"
    exit 1
fi

log_info "Testing Worker 2 -> Worker 1 connectivity..."
if docker compose exec -T worker-2 ping -c 3 "$WORKER1_IP"; then
    log_info "Worker 2 can reach Worker 1"
else
    log_error "Worker 2 cannot reach Worker 1"
    exit 1
fi

echo ""
echo "==================================="
log_info "E2E Test Passed!"
echo "==================================="
echo "Session: ${SESSION:0:32}..."
echo "Join Token: ${JOIN_TOKEN:0:50}..."
echo "Worker 1: $WORKER1_IP"
echo "Worker 2: $WORKER2_IP"
echo ""
