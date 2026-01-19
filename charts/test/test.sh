#!/bin/bash
log_info() {
    echo -e "\033[0;32m[INFO]\033[0m $1"
}

log_warn() {
    echo -e "\033[1;33m[WARN]\033[0m $1"
}

log_error() {
    echo -e "\033[0;31m[ERROR]\033[0m $1"
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
NAMESPACE="wonder"
KEYCLOAK_SVC="wonder-mesh-net-keycloak"

log_info "Cleaning up previous installation..."
helm uninstall wonder-mesh-net -n ${NAMESPACE} 2>/dev/null || true
kubectl delete ns ${NAMESPACE} 2>/dev/null || true
kubectl create ns ${NAMESPACE}

log_info "Building Wonder Linux binary..."
cd "${PROJECT_ROOT}"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" -o charts/test/wonder-linux ./cmd/wonder

log_info "Building images..."
cd "${PROJECT_ROOT}/charts/test"
docker build --network=host -t wonder-worker:test -f Dockerfile.worker .
docker build --network=host -t wonder-deployer:test -f Dockerfile.deployer .

log_info "Checking external images in Minikube..."
docker pull headscale/headscale:0.27.1
minikube image load headscale/headscale:0.27.1
docker pull quay.io/keycloak/keycloak:26.0
minikube image load quay.io/keycloak/keycloak:26.0
log_info "Images already available in Minikube"

log_info "Loading images into Minikube..."
minikube image load wonder-worker:test
minikube image load wonder-deployer:test

TEST_IMAGE_TAG="test"
cd "${PROJECT_ROOT}"
docker build --network=host -t wonder-mesh-net:${TEST_IMAGE_TAG} .
minikube image load wonder-mesh-net:${TEST_IMAGE_TAG}

log_info "Installing Helm chart..."
helm install wonder-mesh-net ./charts/wonder-mesh-net \
    --namespace ${NAMESPACE} \
    --set coordinator.image.repository=wonder-mesh-net \
    --set coordinator.image.tag=${TEST_IMAGE_TAG} \
    --set coordinator.image.pullPolicy=Never \
    --set headscale.image.pullPolicy=IfNotPresent \
    --set keycloak.image.pullPolicy=IfNotPresent \
    --set keycloak.enabled=true \
    --set keycloak.production=true \
    --set postgres.enabled=true \
    --set coordinator.publicUrl="http://wonder-mesh-net"

log_info "Waiting for pods to be ready..."
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=wonder-mesh-net -n ${NAMESPACE} --timeout=300s

wait_for_keycloak() {
    log_info "Waiting for Keycloak to be ready..."

    kubectl wait --for=condition=ready pod -l app.kubernetes.io/component=keycloak -n ${NAMESPACE} --timeout=120s

    for i in $(seq 1 30); do
        if kubectl run healthcheck -n ${NAMESPACE} --rm -i --restart=Never --image=alpine:3.19 -- wget -q -O- "http://${KEYCLOAK_SVC}:8080/realms/wonder/.well-known/openid-configuration" >/dev/null 2>&1; then
            log_info "Keycloak is ready!"
            return 0
        fi
        echo "Waiting for Keycloak health... ($i/30)"
        sleep 2
    done

    log_error "Keycloak did not become ready in 60 seconds"
    return 1
}

log_info "Deploying Workers..."
cat <<EOF | kubectl apply -n ${NAMESPACE} -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: worker
spec:
  replicas: 3
  selector:
    matchLabels:
      app: worker
  template:
    metadata:
      labels:
        app: worker
    spec:
      terminationGracePeriodSeconds: 0
      containers:
        - name: worker
          image: wonder-worker:test
          imagePullPolicy: Never
          securityContext:
            privileged: true
          command: ["/bin/sh", "-c", "sleep infinity"]
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: deployer
spec:
  replicas: 1
  selector:
    matchLabels:
      app: deployer
  template:
    metadata:
      labels:
        app: deployer
    spec:
      containers:
      - name: deployer
        image: wonder-deployer:test
        imagePullPolicy: Never
        securityContext:
          privileged: true
EOF

log_info "Waiting for worker/deployer pods..."
kubectl wait --for=condition=ready pod -l app=worker -n ${NAMESPACE} --timeout=120s
kubectl wait --for=condition=ready pod -l app=deployer -n ${NAMESPACE} --timeout=120s

wait_for_keycloak

DEPLOYER_POD=$(kubectl get pod -n ${NAMESPACE} -l app=deployer -o jsonpath='{.items[0].metadata.name}')

get_access_token_with_retry() {
    local max_attempts=15
    local attempt=0
    
    while [ ${attempt} -lt ${max_attempts} ]; do
        attempt=$((attempt + 1))
        
        TOKEN_RESPONSE=$(kubectl exec -n ${NAMESPACE} ${DEPLOYER_POD} -- curl -s -X POST \
            "http://${KEYCLOAK_SVC}:8080/realms/wonder/protocol/openid-connect/token" \
            -H "Content-Type: application/x-www-form-urlencoded" \
            -d "grant_type=password" \
            -d "client_id=wonder-mesh-net" \
            -d "client_secret=wonder-secret" \
            -d "username=testuser" \
            -d "password=testpass" 2>/dev/null)
        
        ACCESS_TOKEN=$(echo "${TOKEN_RESPONSE}" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
        
        if [ -n "${ACCESS_TOKEN}" ]; then
            echo "${ACCESS_TOKEN}"
            return 0
        fi
        
        sleep 3
    done
    
    log_error "Failed to get access token after ${max_attempts} attempts"
    return 1
}

ACCESS_TOKEN=$(get_access_token_with_retry)

if [ -z "${ACCESS_TOKEN}" ]; then
    log_error "Failed to get access token"
    exit 1
fi

COORDINATOR_SVC="wonder-mesh-net"

log_info "Creating join token..."
JOIN_TOKEN_RESPONSE=$(kubectl exec -n ${NAMESPACE} ${DEPLOYER_POD} -- curl -s \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    "http://${COORDINATOR_SVC}/coordinator/api/v1/join-token")

JOIN_TOKEN=$(echo "${JOIN_TOKEN_RESPONSE}" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
log_info "Join Token: ${JOIN_TOKEN}"

if [ -z "${JOIN_TOKEN}" ]; then
    log_error "Failed to create join token: ${JOIN_TOKEN_RESPONSE}"
    exit 1
fi
log_info "Join token created."

WORKER_PODS=$(kubectl get pod -n ${NAMESPACE} -l app=worker -o jsonpath='{.items[*].metadata.name}')
WORKER1_POD=""
WORKER1_IP=""

for pod in ${WORKER_PODS}; do
    log_info "Joining worker ${pod} to mesh..."
    kubectl exec -n ${NAMESPACE} ${pod} -- wonder worker join --coordinator-url="http://${COORDINATOR_SVC}" "${JOIN_TOKEN}"
    
    sleep 5
    IP=$(kubectl exec -n ${NAMESPACE} ${pod} -- tailscale ip -4)
    log_info "Worker ${pod} IP: ${IP}"
    if [ -z "${WORKER1_POD}" ]; then
        WORKER1_POD=${pod}
        WORKER1_IP=${IP}
    fi
done

sleep 10

log_info "Checking nodes visibility..."
NODES_JSON=$(kubectl exec -n ${NAMESPACE} ${DEPLOYER_POD} -- curl -s \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    "http://${COORDINATOR_SVC}/coordinator/api/v1/nodes")

NODE_COUNT=$(echo "${NODES_JSON}" | grep -o '"id":' | wc -l)
log_info "Nodes found: ${NODE_COUNT}"

if [ "${NODE_COUNT}" -lt 3 ]; then
    log_error "Expected at least 3 nodes, got ${NODE_COUNT}"
    exit 1
fi

if [ -z "${WORKER1_POD}" ] || [ -z "${WORKER1_IP}" ]; then
    log_error "Missing worker pod or IP for deployer test"
    exit 1
fi

log_info "Starting sshd on worker ${WORKER1_POD}..."
kubectl exec -n ${NAMESPACE} ${WORKER1_POD} -- /usr/sbin/sshd || log_warn "sshd may already be running"

log_info "Creating API key for deployer..."
API_KEY_RESPONSE=$(kubectl exec -n ${NAMESPACE} ${DEPLOYER_POD} -- curl -s -X POST \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{"name": "deployer-key", "expires_in": "24h"}' \
    "http://${COORDINATOR_SVC}/coordinator/api/v1/api-keys")

API_KEY=$(echo "${API_KEY_RESPONSE}" | sed -n 's/.*"key":"\([^"]*\)".*/\1/p')

if [ -z "${API_KEY}" ]; then
    log_error "Failed to create API key: ${API_KEY_RESPONSE}"
    exit 1
fi
log_info "API key created."

log_info "Deployer joining mesh with API key..."
DEPLOYER_JOIN_RESPONSE=$(kubectl exec -n ${NAMESPACE} ${DEPLOYER_POD} -- curl -s -X POST \
    -H "Authorization: Bearer ${API_KEY}" \
    -H "Content-Type: application/json" \
    "http://${COORDINATOR_SVC}/coordinator/api/v1/deployer/join")

DEPLOYER_AUTHKEY=$(echo "${DEPLOYER_JOIN_RESPONSE}" | grep -o '"authkey":"[^"]*"' | sed 's/"authkey":"//;s/"$//')
DEPLOYER_LOGIN_SERVER=$(echo "${DEPLOYER_JOIN_RESPONSE}" | grep -o '"login_server":"[^"]*"' | sed 's/"login_server":"//;s/"$//')

if [ -z "${DEPLOYER_AUTHKEY}" ] || [ -z "${DEPLOYER_LOGIN_SERVER}" ]; then
    log_error "Failed to get authkey or login server: ${DEPLOYER_JOIN_RESPONSE}"
    exit 1
fi

log_info "Starting userspace tailscaled in deployer..."
kubectl exec -n ${NAMESPACE} ${DEPLOYER_POD} -- sh -c \
    "tailscaled --tun=userspace-networking --socks5-server=localhost:1055 --state=/tmp/tailscale.state --socket=/tmp/tailscaled.sock >/tmp/tailscaled.log 2>&1 &"

sleep 3

log_info "Deployer joining mesh..."
kubectl exec -n ${NAMESPACE} ${DEPLOYER_POD} -- tailscale --socket=/tmp/tailscaled.sock up \
    --authkey="${DEPLOYER_AUTHKEY}" \
    --login-server="${DEPLOYER_LOGIN_SERVER}"

sleep 3

log_info "Deployer tailscale status:"
kubectl exec -n ${NAMESPACE} ${DEPLOYER_POD} -- tailscale --socket=/tmp/tailscaled.sock status || true

log_info "Deploying app to Worker 1 via SSH over SOCKS5..."
kubectl exec -n ${NAMESPACE} ${DEPLOYER_POD} -- \
    sshpass -p worker ssh -T \
    -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    -o ConnectTimeout=10 \
    -o "ProxyCommand=nc -x localhost:1055 %h %p" \
    root@${WORKER1_IP} \
    'echo "Hello from deployed app" > /tmp/index.html && nohup python3 -m http.server 8080 -d /tmp > /tmp/httpd.log 2>&1 &'

SSH_EXIT=$?
if [ ${SSH_EXIT} -ne 0 ]; then
    log_error "SSH command failed with exit code ${SSH_EXIT}"
    exit 1
fi
log_info "SSH deploy command completed"

log_info "Accessing deployed app via mesh..."
for i in 1 2 3 4 5; do
    sleep 2
    APP_RESPONSE=$(kubectl exec -n ${NAMESPACE} ${DEPLOYER_POD} -- curl -s --connect-timeout 10 --socks5-hostname localhost:1055 \
        "http://${WORKER1_IP}:8080/index.html" 2>/dev/null || true)
    if echo "${APP_RESPONSE}" | grep -q "Hello from deployed app"; then
        log_info "Deployer test PASSED: App accessible via mesh"
        break
    fi
    if [ ${i} -lt 5 ]; then
        log_info "Retry ${i}: HTTP server not ready yet, waiting..."
    fi
done

if ! echo "${APP_RESPONSE}" | grep -q "Hello from deployed app"; then
    log_error "Deployer test FAILED"
    echo "Response: ${APP_RESPONSE}"
    exit 1
fi

log_info "Test Passed!"
