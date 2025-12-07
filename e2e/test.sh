#!/bin/bash
set -e

cd "$(dirname "$0")"

echo "=== Wonder Mesh Net E2E Test ==="

# Colors
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

# Step 1: Start Headscale first to generate API key
log_info "Starting Headscale..."
docker compose up -d headscale
sleep 5

# Wait for Headscale to be healthy
log_info "Waiting for Headscale to be ready..."
for i in {1..30}; do
    if curl -s http://localhost:8080/health 2>/dev/null | grep -q "pass"; then
        log_info "Headscale is healthy"
        break
    fi
    echo "  Attempt $i/30..."
    sleep 3
done

# Generate API key
log_info "Generating Headscale API key..."
API_KEY=""
for i in {1..5}; do
    API_KEY=$(docker compose exec -T headscale headscale apikeys create --expiration 24h 2>&1 | grep -v "^$" | tail -1)
    if [ -n "$API_KEY" ] && [[ ! "$API_KEY" =~ "error" ]]; then
        break
    fi
    log_warn "Retry $i/5..."
    sleep 2
done

if [ -z "$API_KEY" ]; then
    log_error "Failed to generate API key"
    docker compose logs headscale | tail -20
    exit 1
fi
log_info "API Key generated: ${API_KEY:0:20}..."

# Export for docker-compose
export HEADSCALE_API_KEY="$API_KEY"

# Step 2: Start Keycloak first (coordinator needs Keycloak's OIDC endpoint at startup)
log_info "Starting Keycloak..."
docker compose up -d keycloak

# Wait for Keycloak health
log_info "Waiting for Keycloak to be ready..."
for i in {1..60}; do
    if curl -s http://localhost:9090/health/ready 2>/dev/null | grep -q "UP"; then
        log_info "Keycloak is ready"
        break
    fi
    echo "  Waiting for Keycloak... ($i/60)"
    sleep 5
done

# Wait for Keycloak OIDC endpoint (required for coordinator to register provider)
log_info "Waiting for Keycloak OIDC endpoint..."
for i in {1..30}; do
    if curl -s http://localhost:9090/realms/wonder/.well-known/openid-configuration 2>/dev/null | grep -q "issuer"; then
        log_info "Keycloak OIDC endpoint is ready"
        break
    fi
    echo "  Waiting for OIDC endpoint... ($i/30)"
    sleep 2
done

# Step 3: Start Coordinator (now Keycloak is fully ready)
log_info "Starting Coordinator..."
docker compose up -d coordinator

# Wait for Coordinator
log_info "Waiting for Coordinator to be ready..."
for i in {1..30}; do
    if curl -s http://localhost:9080/health 2>/dev/null | grep -q "ok"; then
        log_info "Coordinator is ready"
        break
    fi
    echo "  Waiting for Coordinator... ($i/30)"
    sleep 3
done

# Step 4: Test OIDC login flow using curl
log_info "Testing OIDC login flow..."

# Get the login URL and follow redirects to Keycloak
COOKIE_JAR="cookies.txt"
rm -f "$COOKIE_JAR"

# Start login flow - get redirect Location header
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

# Keycloak should already use localhost:9090 URLs
KEYCLOAK_URL="$LOGIN_REDIRECT"
log_info "Fetching Keycloak login page..."

# Get Keycloak login form
LOGIN_PAGE=$(curl -s -c "$COOKIE_JAR" -b "$COOKIE_JAR" -L "$KEYCLOAK_URL")

# Extract form action URL (using sed for macOS compatibility)
FORM_ACTION=$(echo "$LOGIN_PAGE" | sed -n 's/.*action="\([^"]*\)".*/\1/p' | head -1 | sed 's/&amp;/\&/g')
if [ -z "$FORM_ACTION" ]; then
    log_error "Could not find login form"
    echo "$LOGIN_PAGE" | head -50
    exit 1
fi
log_info "Form action: ${FORM_ACTION:0:80}..."

# Submit login form
log_info "Submitting login credentials..."
CALLBACK_RESPONSE=$(curl -s -c "$COOKIE_JAR" -b "$COOKIE_JAR" -L \
    -d "username=testuser" \
    -d "password=testpass" \
    -w "\n%{url_effective}" \
    "$FORM_ACTION")

# Extract final URL and session
FINAL_URL=$(echo "$CALLBACK_RESPONSE" | tail -1)
log_info "Final URL: $FINAL_URL"

# Extract session from URL (using sed for macOS compatibility)
SESSION=$(echo "$FINAL_URL" | sed -n 's/.*session=\([^&]*\).*/\1/p')
if [ -z "$SESSION" ]; then
    log_error "Failed to get session token"
    echo "$CALLBACK_RESPONSE"
    exit 1
fi
log_info "Session token obtained: ${SESSION:0:20}..."

# Step 5: Test API with session
log_info "Testing API endpoints..."

# Create join token
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

# Test worker join endpoint
log_info "Testing worker join endpoint..."
WORKER_JOIN_RESPONSE=$(curl -s -X POST \
    -H "Content-Type: application/json" \
    -d "{\"token\": \"$JOIN_TOKEN\"}" \
    "http://localhost:9080/api/v1/worker/join")

AUTHKEY=$(echo "$WORKER_JOIN_RESPONSE" | sed -n 's/.*"authkey":"\([^"]*\)".*/\1/p')
if [ -z "$AUTHKEY" ]; then
    log_error "Failed to get authkey"
    echo "$WORKER_JOIN_RESPONSE"
    exit 1
fi
log_info "Authkey obtained: ${AUTHKEY:0:20}..."

# Step 6: Summary
echo ""
echo "==================================="
log_info "E2E Test Passed!"
echo "==================================="
echo "Session: ${SESSION:0:32}..."
echo "Join Token: ${JOIN_TOKEN:0:50}..."
echo "Authkey: ${AUTHKEY:0:32}..."
echo ""
