#!/bin/bash

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[OK]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

echo ""
echo -e "${BOLD}================================================${NC}"
echo -e "${BOLD}    Wonder Mesh Net - Cleanup Script${NC}"
echo -e "${BOLD}================================================${NC}"
echo ""

# List of VMs to clean up
VMS=("network-coordinator" "traffic-gateway" "deploy-manager" "worker-node-1" "worker-node-2")

log_info "This will stop and delete the following Lima VMs:"
for vm in "${VMS[@]}"; do
    echo "  - $vm"
done
echo ""

read -p "Are you sure you want to proceed? (y/N) " -n 1 -r
echo ""

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    log_warn "Cleanup cancelled."
    exit 0
fi

echo ""

# Stop and delete each VM
for vm in "${VMS[@]}"; do
    if limactl list -q 2>/dev/null | grep -q "^${vm}$"; then
        log_info "Stopping $vm..."
        limactl stop "$vm" 2>/dev/null || true

        log_info "Deleting $vm..."
        limactl delete "$vm" --force 2>/dev/null || true

        log_success "$vm removed"
    else
        log_warn "$vm not found, skipping"
    fi
done

# Clean up shared keys directory
log_info "Cleaning up shared keys directory..."
rm -rf /tmp/lima/keys
log_success "Keys directory cleaned"

# Clean up temporary build artifacts
log_info "Cleaning up temporary files..."
rm -f /tmp/deploy-cli
log_success "Temporary files cleaned"

echo ""
log_success "Cleanup complete!"
echo ""

# Show remaining Lima VMs if any
REMAINING=$(limactl list -q 2>/dev/null)
if [ -n "$REMAINING" ]; then
    log_info "Remaining Lima VMs:"
    limactl list
fi
