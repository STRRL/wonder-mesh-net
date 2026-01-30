#!/bin/bash
set -e

log_info() {
    echo -e "\033[0;32m[INFO]\033[0m $1"
}

log_warn() {
    echo -e "\033[1;33m[WARN]\033[0m $1"
}

log_error() {
    echo -e "\033[0;31m[ERROR]\033[0m $1"
}

log_success() {
    echo -e "\033[0;36m[PASS]\033[0m $1"
}

log_fail() {
    echo -e "\033[0;31m[FAIL]\033[0m $1"
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

ISOLATION_FAILED=0

cleanup() {
    if [ "${NO_CLEANUP:-0}" = "1" ]; then
        log_info "NO_CLEANUP=1, skipping cleanup. Containers are still running."
        return
    fi
    log_info "Cleaning up..."
    docker compose -f docker-compose.yaml down -v --remove-orphans 2>/dev/null || true
}

trap cleanup EXIT

echo "=== Wonder Mesh Net E2E Isolation Test ==="
echo "=== Testing tailnet isolation between different users ==="
echo ""

log_info "Starting all services..."
docker compose -f docker-compose.yaml up -d --build --force-recreate
sleep 10

log_info "Waiting for Keycloak to be ready..."
for i in {1..60}; do
    if curl -sf http://localhost:8080/realms/wonder/.well-known/openid-configuration >/dev/null 2>&1; then
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

log_info "Waiting for Coordinator to be ready..."
for i in {1..30}; do
    if curl -sf http://localhost:8080/coordinator/health >/dev/null 2>&1; then
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

log_info "Building wonder binary for Linux..."
(cd .. && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/wonder-linux ./cmd/wonder)

log_info "Copying wonder binary to workers..."
for worker in worker-1 worker-2 worker-3 worker-4 worker-5; do
    docker cp ../bin/wonder-linux $worker:/usr/local/bin/wonder
done

COORDINATOR_URL="http://nginx"

echo ""
log_info "=== Phase 1: Setting up User 1 (testuser) WonderNet ==="

log_info "Getting access token for testuser..."
TOKEN_RESPONSE_USER1=$(docker exec deployer curl -s -X POST \
    "http://nginx/realms/wonder/protocol/openid-connect/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=password" \
    -d "client_id=wonder-mesh-net" \
    -d "client_secret=wonder-secret" \
    -d "username=testuser" \
    -d "password=testpass")

ACCESS_TOKEN_USER1=$(echo "$TOKEN_RESPONSE_USER1" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
if [ -z "$ACCESS_TOKEN_USER1" ]; then
    log_error "Failed to get access token for testuser"
    echo "$TOKEN_RESPONSE_USER1"
    exit 1
fi
log_info "User 1 access token obtained: ${ACCESS_TOKEN_USER1:0:50}..."

log_info "Creating join token for User 1..."
JOIN_TOKEN_RESPONSE_USER1=$(docker exec deployer curl -s \
    -H "Authorization: Bearer $ACCESS_TOKEN_USER1" \
    "http://nginx/coordinator/api/v1/join-token")

JOIN_TOKEN_USER1=$(echo "$JOIN_TOKEN_RESPONSE_USER1" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
if [ -z "$JOIN_TOKEN_USER1" ]; then
    log_error "Failed to create join token for User 1"
    echo "$JOIN_TOKEN_RESPONSE_USER1"
    exit 1
fi
log_info "User 1 join token created"

log_info "Joining workers 1-3 to User 1's WonderNet..."
docker exec worker-1 wonder worker join --coordinator-url="$COORDINATOR_URL" "$JOIN_TOKEN_USER1" \
    2>&1 || log_warn "wonder worker join returned non-zero for worker-1"
docker exec worker-2 wonder worker join --coordinator-url="$COORDINATOR_URL" "$JOIN_TOKEN_USER1" \
    2>&1 || log_warn "wonder worker join returned non-zero for worker-2"
docker exec worker-3 wonder worker join --coordinator-url="$COORDINATOR_URL" "$JOIN_TOKEN_USER1" \
    2>&1 || log_warn "wonder worker join returned non-zero for worker-3"

sleep 5

USER1_WORKER1_IP=$(docker exec worker-1 tailscale ip -4 2>/dev/null || echo "")
USER1_WORKER2_IP=$(docker exec worker-2 tailscale ip -4 2>/dev/null || echo "")
USER1_WORKER3_IP=$(docker exec worker-3 tailscale ip -4 2>/dev/null || echo "")

log_info "User 1 Worker IPs: $USER1_WORKER1_IP, $USER1_WORKER2_IP, $USER1_WORKER3_IP"

if [ -z "$USER1_WORKER1_IP" ] || [ -z "$USER1_WORKER2_IP" ] || [ -z "$USER1_WORKER3_IP" ]; then
    log_error "Failed to get User 1 worker IPs"
    exit 1
fi

echo ""
log_info "=== Phase 2: Setting up User 2 (testuser2) WonderNet ==="

log_info "Getting access token for testuser2..."
TOKEN_RESPONSE_USER2=$(docker exec deployer curl -s -X POST \
    "http://nginx/realms/wonder/protocol/openid-connect/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=password" \
    -d "client_id=wonder-mesh-net" \
    -d "client_secret=wonder-secret" \
    -d "username=testuser2" \
    -d "password=testpass2")

ACCESS_TOKEN_USER2=$(echo "$TOKEN_RESPONSE_USER2" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
if [ -z "$ACCESS_TOKEN_USER2" ]; then
    log_error "Failed to get access token for testuser2"
    echo "$TOKEN_RESPONSE_USER2"
    exit 1
fi
log_info "User 2 access token obtained: ${ACCESS_TOKEN_USER2:0:50}..."

log_info "Creating join token for User 2..."
JOIN_TOKEN_RESPONSE_USER2=$(docker exec deployer curl -s \
    -H "Authorization: Bearer $ACCESS_TOKEN_USER2" \
    "http://nginx/coordinator/api/v1/join-token")

JOIN_TOKEN_USER2=$(echo "$JOIN_TOKEN_RESPONSE_USER2" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
if [ -z "$JOIN_TOKEN_USER2" ]; then
    log_error "Failed to create join token for User 2"
    echo "$JOIN_TOKEN_RESPONSE_USER2"
    exit 1
fi
log_info "User 2 join token created"

log_info "Joining workers 4-5 to User 2's WonderNet..."
docker exec worker-4 wonder worker join --coordinator-url="$COORDINATOR_URL" "$JOIN_TOKEN_USER2" \
    2>&1 || log_warn "wonder worker join returned non-zero for worker-4"
docker exec worker-5 wonder worker join --coordinator-url="$COORDINATOR_URL" "$JOIN_TOKEN_USER2" \
    2>&1 || log_warn "wonder worker join returned non-zero for worker-5"

sleep 5

USER2_WORKER4_IP=$(docker exec worker-4 tailscale ip -4 2>/dev/null || echo "")
USER2_WORKER5_IP=$(docker exec worker-5 tailscale ip -4 2>/dev/null || echo "")

log_info "User 2 Worker IPs: $USER2_WORKER4_IP, $USER2_WORKER5_IP"

if [ -z "$USER2_WORKER4_IP" ] || [ -z "$USER2_WORKER5_IP" ]; then
    log_error "Failed to get User 2 worker IPs"
    exit 1
fi

echo ""
log_info "=== Phase 3: Testing API Isolation ==="

log_info "Checking User 1's nodes via API..."
NODES_USER1=$(docker exec deployer curl -s \
    -H "Authorization: Bearer $ACCESS_TOKEN_USER1" \
    "http://nginx/coordinator/api/v1/nodes")

NODE_COUNT_USER1=$(echo "$NODES_USER1" | sed -n 's/.*"count":\([0-9]*\).*/\1/p')
log_info "User 1 sees $NODE_COUNT_USER1 nodes"

if [ "$NODE_COUNT_USER1" -ne 3 ]; then
    log_fail "API Isolation: User 1 should see exactly 3 nodes, got $NODE_COUNT_USER1"
    echo "$NODES_USER1"
    ISOLATION_FAILED=1
else
    log_success "API Isolation: User 1 sees exactly 3 nodes"
fi

if echo "$NODES_USER1" | grep -q "$USER1_WORKER1_IP"; then
    log_success "API Isolation: User 1 sees worker-1 IP ($USER1_WORKER1_IP)"
else
    log_fail "API Isolation: User 1 does not see worker-1 IP"
    ISOLATION_FAILED=1
fi

if echo "$NODES_USER1" | grep -q "$USER2_WORKER4_IP"; then
    log_fail "API Isolation BREACH: User 1 can see User 2's worker-4 IP ($USER2_WORKER4_IP)"
    ISOLATION_FAILED=1
else
    log_success "API Isolation: User 1 cannot see User 2's worker-4 IP"
fi

if echo "$NODES_USER1" | grep -q "$USER2_WORKER5_IP"; then
    log_fail "API Isolation BREACH: User 1 can see User 2's worker-5 IP ($USER2_WORKER5_IP)"
    ISOLATION_FAILED=1
else
    log_success "API Isolation: User 1 cannot see User 2's worker-5 IP"
fi

log_info "Checking User 2's nodes via API..."
NODES_USER2=$(docker exec deployer curl -s \
    -H "Authorization: Bearer $ACCESS_TOKEN_USER2" \
    "http://nginx/coordinator/api/v1/nodes")

NODE_COUNT_USER2=$(echo "$NODES_USER2" | sed -n 's/.*"count":\([0-9]*\).*/\1/p')
log_info "User 2 sees $NODE_COUNT_USER2 nodes"

if [ "$NODE_COUNT_USER2" -ne 2 ]; then
    log_fail "API Isolation: User 2 should see exactly 2 nodes, got $NODE_COUNT_USER2"
    echo "$NODES_USER2"
    ISOLATION_FAILED=1
else
    log_success "API Isolation: User 2 sees exactly 2 nodes"
fi

if echo "$NODES_USER2" | grep -q "$USER2_WORKER4_IP"; then
    log_success "API Isolation: User 2 sees worker-4 IP ($USER2_WORKER4_IP)"
else
    log_fail "API Isolation: User 2 does not see worker-4 IP"
    ISOLATION_FAILED=1
fi

if echo "$NODES_USER2" | grep -q "$USER1_WORKER1_IP"; then
    log_fail "API Isolation BREACH: User 2 can see User 1's worker-1 IP ($USER1_WORKER1_IP)"
    ISOLATION_FAILED=1
else
    log_success "API Isolation: User 2 cannot see User 1's worker-1 IP"
fi

echo ""
log_info "=== Phase 4: Testing Network Isolation ==="

log_info "Testing intra-WonderNet connectivity (User 1: worker-1 -> worker-2)..."
if docker exec worker-1 ping -c 2 -W 3 "$USER1_WORKER2_IP" >/dev/null 2>&1; then
    log_success "Intra-WonderNet: User 1's worker-1 can ping worker-2"
else
    log_fail "Intra-WonderNet: User 1's worker-1 cannot ping worker-2 (unexpected)"
    ISOLATION_FAILED=1
fi

log_info "Testing intra-WonderNet connectivity (User 2: worker-4 -> worker-5)..."
if docker exec worker-4 ping -c 2 -W 3 "$USER2_WORKER5_IP" >/dev/null 2>&1; then
    log_success "Intra-WonderNet: User 2's worker-4 can ping worker-5"
else
    log_fail "Intra-WonderNet: User 2's worker-4 cannot ping worker-5 (unexpected)"
    ISOLATION_FAILED=1
fi

log_info "Testing cross-WonderNet isolation (User 1's worker-1 -> User 2's worker-4)..."
if docker exec worker-1 ping -c 2 -W 3 "$USER2_WORKER4_IP" >/dev/null 2>&1; then
    log_fail "Network Isolation BREACH: User 1's worker-1 can ping User 2's worker-4"
    ISOLATION_FAILED=1
else
    log_success "Network Isolation: User 1's worker-1 cannot ping User 2's worker-4"
fi

log_info "Testing cross-WonderNet isolation (User 1's worker-1 -> User 2's worker-5)..."
if docker exec worker-1 ping -c 2 -W 3 "$USER2_WORKER5_IP" >/dev/null 2>&1; then
    log_fail "Network Isolation BREACH: User 1's worker-1 can ping User 2's worker-5"
    ISOLATION_FAILED=1
else
    log_success "Network Isolation: User 1's worker-1 cannot ping User 2's worker-5"
fi

log_info "Testing cross-WonderNet isolation (User 2's worker-4 -> User 1's worker-1)..."
if docker exec worker-4 ping -c 2 -W 3 "$USER1_WORKER1_IP" >/dev/null 2>&1; then
    log_fail "Network Isolation BREACH: User 2's worker-4 can ping User 1's worker-1"
    ISOLATION_FAILED=1
else
    log_success "Network Isolation: User 2's worker-4 cannot ping User 1's worker-1"
fi

log_info "Testing cross-WonderNet isolation (User 2's worker-4 -> User 1's worker-2)..."
if docker exec worker-4 ping -c 2 -W 3 "$USER1_WORKER2_IP" >/dev/null 2>&1; then
    log_fail "Network Isolation BREACH: User 2's worker-4 can ping User 1's worker-2"
    ISOLATION_FAILED=1
else
    log_success "Network Isolation: User 2's worker-4 cannot ping User 1's worker-2"
fi

echo ""
log_info "=== Isolation Test Summary ==="

if [ "$ISOLATION_FAILED" -eq 0 ]; then
    log_info "All isolation tests PASSED"
    echo ""
    echo "=== Wonder Mesh Net Isolation E2E Test Complete ==="
    echo "- API Isolation: PASSED (users see only their own nodes)"
    echo "- Network Isolation: PASSED (cross-WonderNet ping blocked)"
    exit 0
else
    log_error "Some isolation tests FAILED"
    echo ""
    echo "=== Wonder Mesh Net Isolation E2E Test FAILED ==="
    echo ""
    echo "Note: If network isolation tests failed but API isolation passed,"
    echo "this indicates the ACL policy is too permissive (*:* allows all)."
    echo "See pkg/headscale/acl.go and internal/app/coordinator/service/wondernet.go"
    exit 1
fi
