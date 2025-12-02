#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[OK]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo ""
    echo -e "${BOLD}${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BOLD}${CYAN}  Step $1: $2${NC}"
    echo -e "${BOLD}${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

echo ""
echo -e "${BOLD}================================================${NC}"
echo -e "${BOLD}    Wonder Mesh Net - MVP Demo Script (Lima)${NC}"
echo -e "${BOLD}================================================${NC}"
echo ""
echo "This script will guide you through setting up the MVP using Lima VMs:"
echo ""
echo "  Phase 1: Bootstrap all VMs"
echo "    1. Check prerequisites (Lima, Go)"
echo "    2. Create shared directory for keys"
echo "    3. Start all 5 VMs (base OS only)"
echo ""
echo "  Phase 2: Setup mesh network"
echo "    4. Setup network-coordinator (Headscale)"
echo "    5. Setup traffic-gateway (nginx + Tailscale)"
echo "    6. Setup deploy-manager (tailscaled userspace + SOCKS)"
echo "    7. Setup worker nodes (Tailscale + Cockpit)"
echo "    8. Build and install deploy-server"
echo "    9. Update nginx config with worker IPs"
echo ""

# ============================================================================
# PHASE 1: Bootstrap all VMs
# ============================================================================

log_step "1" "Check Prerequisites"

if ! command -v limactl &> /dev/null; then
    log_error "Lima is not installed. Please install Lima first."
    echo "  Install: brew install lima"
    exit 1
fi
log_success "Lima is installed ($(limactl --version 2>/dev/null | head -1))"

if ! command -v go &> /dev/null; then
    log_warn "Go is not installed locally. deploy-cli will be built inside the VM."
else
    log_success "Go is installed ($(go version | awk '{print $3}'))"
fi

log_step "2" "Create Shared Directory for Keys"

mkdir -p /tmp/lima/keys
log_success "Shared directory created at /tmp/lima/keys"

log_step "3" "Bootstrap All VMs"
echo "Starting all 5 VMs with base Ubuntu OS..."
echo ""

log_info "Starting network-coordinator VM..."
limactl start --name=network-coordinator "${SCRIPT_DIR}/lima/network-coordinator-vm.yaml" --tty=false || true

log_info "Starting traffic-gateway VM..."
limactl start --name=traffic-gateway "${SCRIPT_DIR}/lima/traffic-gateway-vm.yaml" --tty=false || true

log_info "Starting deploy-manager VM..."
limactl start --name=deploy-manager "${SCRIPT_DIR}/lima/base-vm.yaml" --tty=false || true

log_info "Starting worker-node-1 VM..."
limactl start --name=worker-node-1 "${SCRIPT_DIR}/lima/base-vm.yaml" --tty=false || true

log_info "Starting worker-node-2 VM..."
limactl start --name=worker-node-2 "${SCRIPT_DIR}/lima/base-vm.yaml" --tty=false || true

sleep 5
log_info "Current VM status:"
limactl list

log_success "All VMs bootstrapped!"

# ============================================================================
# PHASE 2: Setup mesh network
# ============================================================================

log_step "4" "Setup Network Coordinator (Headscale)"

log_info "Running setup script on network-coordinator..."
limactl shell network-coordinator < "${SCRIPT_DIR}/scripts/setup-network-coordinator.sh"

HEADSCALE_URL=$(cat /tmp/lima/keys/headscale-url.txt)
log_success "Headscale URL: $HEADSCALE_URL"
ls -la /tmp/lima/keys/

log_step "5" "Setup Traffic Gateway (nginx + Tailscale)"

log_info "Running setup script on traffic-gateway..."
limactl shell traffic-gateway < "${SCRIPT_DIR}/scripts/setup-traffic-gateway.sh"

log_success "Traffic Gateway setup complete"

log_step "6" "Setup Deploy Manager (tailscaled userspace + SOCKS)"

log_info "Running setup script on deploy-manager..."
limactl shell deploy-manager < "${SCRIPT_DIR}/scripts/setup-deploy-manager.sh"

log_success "Deploy Manager setup complete"

log_step "7" "Setup Worker Nodes (Tailscale + Cockpit)"

log_info "Running setup script on worker-node-1..."
limactl shell worker-node-1 < "${SCRIPT_DIR}/scripts/setup-worker-node.sh"

log_info "Running setup script on worker-node-2..."
limactl shell worker-node-2 < "${SCRIPT_DIR}/scripts/setup-worker-node.sh"

log_success "Worker nodes setup complete (Tailscale connected, Cockpit installed)"

sleep 3
log_info "Headscale node list:"
limactl shell network-coordinator -- sudo headscale nodes list

log_step "8" "Build and Install Deploy Server"

ARCH=$(uname -m)
if [ "$ARCH" = "arm64" ]; then
    GOARCH="arm64"
else
    GOARCH="amd64"
fi

log_info "Building deploy-server for linux/$GOARCH..."
cd "${SCRIPT_DIR}"
GOOS=linux GOARCH=$GOARCH go build -o /tmp/deploy-server ./cmd/deploy-server/

log_info "Copying deploy-server to deploy-manager..."
limactl copy /tmp/deploy-server deploy-manager:/tmp/deploy-server

log_info "Installing deploy-server on deploy-manager..."
limactl shell deploy-manager < "${SCRIPT_DIR}/scripts/install-deploy-server.sh"

log_success "Deploy server installed and running on port 8082"

log_step "9" "Update nginx Config with Worker IPs"

log_info "Updating traffic gateway nginx config..."
limactl shell traffic-gateway -- sudo /usr/local/bin/update-gateway-config.sh

log_success "nginx config updated"

# Get IPs for summary
WORKER1_IP=$(limactl shell worker-node-1 -- tailscale ip -4 2>/dev/null | tr -d '\r\n')
WORKER2_IP=$(limactl shell worker-node-2 -- tailscale ip -4 2>/dev/null | tr -d '\r\n')
DEPLOY_MANAGER_IP=$(limactl shell deploy-manager -- sudo tailscale --socket=/var/run/tailscale-userspace/tailscaled.sock ip -4 2>/dev/null | tr -d '\r\n')

# Final summary
log_step "10" "Demo Complete!"
echo ""
log_success "All components are set up and running!"
echo ""
echo "Access Cockpit:"
echo "  http://localhost:8081/worker-node-1/"
echo "  http://localhost:8081/worker-node-2/"
echo ""
echo "Login: Use your macOS username for both username and password"
echo "       (e.g., $(whoami) / $(whoami))"
echo ""
echo "Headscale (control plane):"
echo "  http://localhost:8080/health"
echo ""
echo "Deploy Server API:"
echo "  List nodes:    limactl shell deploy-manager -- curl -s http://localhost:8082/nodes"
echo "  Health check:  limactl shell deploy-manager -- curl -s http://localhost:8082/health"
echo "  Execute command:"
echo "    limactl shell deploy-manager -- curl -s -X POST http://localhost:8082/exec \\"
echo "      -H 'Content-Type: application/json' \\"
echo "      -d '{\"host\":\"$WORKER1_IP\",\"user\":\"$(whoami)\",\"password\":\"$(whoami)\",\"command\":\"hostname\"}'"
echo ""
echo "Tailnet IPs:"
echo "  traffic-gateway: 100.64.0.1"
echo "  deploy-manager:  $DEPLOY_MANAGER_IP"
echo "  worker-node-1:   $WORKER1_IP"
echo "  worker-node-2:   $WORKER2_IP"
echo ""
echo "Useful commands:"
echo "  limactl shell network-coordinator -- sudo headscale nodes list"
echo "  limactl shell traffic-gateway -- tailscale status"
echo ""
