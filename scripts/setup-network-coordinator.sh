#!/bin/bash
set -eux -o pipefail

# Change to writable directory
cd /tmp

echo "=== Installing Headscale on Network Coordinator ==="

# Install dependencies
sudo apt-get update
sudo apt-get install -y curl wget

# Download and install Headscale (detect architecture)
HEADSCALE_VERSION="0.23.0"
ARCH=$(dpkg --print-architecture)

echo "Downloading Headscale v${HEADSCALE_VERSION} for ${ARCH}..."
wget --timeout=60 --tries=3 "https://github.com/juanfont/headscale/releases/download/v${HEADSCALE_VERSION}/headscale_${HEADSCALE_VERSION}_linux_${ARCH}.deb"

sudo dpkg -i "headscale_${HEADSCALE_VERSION}_linux_${ARCH}.deb"
rm "headscale_${HEADSCALE_VERSION}_linux_${ARCH}.deb"

# Create configuration directory
sudo mkdir -p /etc/headscale

# Get the Lima host gateway IP (192.168.5.2 is the default Lima host)
# This allows other VMs to connect via the host's port forwarding
HOST_IP="192.168.5.2"

# Create Headscale configuration
sudo tee /etc/headscale/config.yaml > /dev/null << EOF
server_url: http://${HOST_IP}:8080
listen_addr: 0.0.0.0:8080
metrics_listen_addr: 127.0.0.1:9090
grpc_listen_addr: 127.0.0.1:50443
grpc_allow_insecure: false

private_key_path: /var/lib/headscale/private.key
noise:
  private_key_path: /var/lib/headscale/noise_private.key

prefixes:
  v4: 100.64.0.0/10
  v6: fd7a:115c:a1e0::/48

derp:
  server:
    enabled: false
  urls:
    - https://controlplane.tailscale.com/derpmap/default
  auto_update_enabled: true
  update_frequency: 24h

disable_check_updates: false
ephemeral_node_inactivity_timeout: 30m

database:
  type: sqlite
  sqlite:
    path: /var/lib/headscale/db.sqlite

log:
  format: text
  level: info

dns:
  magic_dns: true
  base_domain: mesh.local
  nameservers:
    global:
      - 1.1.1.1
      - 8.8.8.8
EOF

# Create data directory
sudo mkdir -p /var/lib/headscale

# Enable and start Headscale service
sudo systemctl enable headscale
sudo systemctl start headscale

# Wait for Headscale to start
echo "Waiting for Headscale to start..."
sleep 5

# Create user for the mesh network
sudo headscale users create mesh

# Generate pre-auth keys for all nodes and save to shared folder
mkdir -p /tmp/lima/keys

# Generate reusable keys
echo "Generating pre-auth keys..."
sudo headscale preauthkeys create --user mesh --reusable --expiration 24h | tail -1 > /tmp/lima/keys/traffic-gateway.key
sudo headscale preauthkeys create --user mesh --reusable --expiration 24h | tail -1 > /tmp/lima/keys/deploy-manager.key
sudo headscale preauthkeys create --user mesh --reusable --expiration 24h | tail -1 > /tmp/lima/keys/worker-node-1.key
sudo headscale preauthkeys create --user mesh --reusable --expiration 24h | tail -1 > /tmp/lima/keys/worker-node-2.key

# Save headscale URL for other VMs
echo "http://${HOST_IP}:8080" > /tmp/lima/keys/headscale-url.txt

# Verify keys were created
echo "=== Generated pre-auth keys ==="
ls -la /tmp/lima/keys/
echo "Keys content:"
head -1 /tmp/lima/keys/*.key

echo "=== Headscale setup complete ==="
sudo headscale users list
sudo headscale preauthkeys list --user mesh || true
sudo headscale nodes list || true
