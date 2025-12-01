#!/bin/bash
set -eux -o pipefail

# Change to writable directory
cd /tmp

echo "=== Installing Deploy Manager (tailscaled userspace + SOCKS) ==="

# Install dependencies
sudo apt-get update
sudo apt-get install -y curl wget

# Install Tailscale
curl -fsSL https://tailscale.com/install.sh | sh

# Stop the default tailscaled service (we'll run in userspace mode)
sudo systemctl stop tailscaled || true
sudo systemctl disable tailscaled || true

# Read pre-auth key and Headscale URL
KEY_FILE="/tmp/lima/keys/deploy-manager.key"
URL_FILE="/tmp/lima/keys/headscale-url.txt"

if [ ! -f "$KEY_FILE" ] || [ ! -f "$URL_FILE" ]; then
    echo "ERROR: Keys not found"
    exit 1
fi

AUTH_KEY=$(cat "$KEY_FILE")
HEADSCALE_URL=$(cat "$URL_FILE")

# Create state directory for userspace tailscaled
sudo mkdir -p /var/lib/tailscale-userspace
sudo mkdir -p /var/run/tailscale-userspace

# Create systemd service for tailscaled in userspace mode with SOCKS5 proxy
sudo tee /etc/systemd/system/tailscaled-userspace.service > /dev/null << EOF
[Unit]
Description=Tailscale daemon (userspace networking with SOCKS5)
After=network.target

[Service]
Type=simple
ExecStart=/usr/sbin/tailscaled --tun=userspace-networking --socks5-server=localhost:1080 --state=/var/lib/tailscale-userspace/tailscaled.state --socket=/var/run/tailscale-userspace/tailscaled.sock
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Start tailscaled in userspace mode
sudo systemctl daemon-reload
sudo systemctl enable tailscaled-userspace
sudo systemctl start tailscaled-userspace

# Wait for tailscaled to be ready
sleep 5

# Connect to Headscale using the userspace socket
echo "Connecting to Headscale at $HEADSCALE_URL..."
echo "Using auth key: $AUTH_KEY"

for attempt in {1..5}; do
    if sudo tailscale --socket=/var/run/tailscale-userspace/tailscaled.sock up \
        --login-server="$HEADSCALE_URL" \
        --authkey="$AUTH_KEY" \
        --hostname="deploy-manager" \
        --timeout=60s; then
        echo "Connected successfully!"
        break
    fi
    echo "Connection attempt $attempt failed, retrying in 10s..."
    sleep 10
done

# Create alias for easier tailscale commands
cat >> ~/.bashrc << 'BASH_EOF'
alias tailscale='sudo tailscale --socket=/var/run/tailscale-userspace/tailscaled.sock'
export PATH=$PATH:/usr/local/go/bin
BASH_EOF

# Install Go for building deploy-cli (detect architecture)
GO_VERSION="1.22.0"
ARCH=$(dpkg --print-architecture)
if [ "$ARCH" = "arm64" ]; then
    GO_ARCH="arm64"
else
    GO_ARCH="amd64"
fi
wget -q "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
sudo tar -C /usr/local -xzf "go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
rm "go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
export PATH=$PATH:/usr/local/go/bin

echo "=== Deploy Manager setup complete ==="
echo "SOCKS5 proxy listening on localhost:1080"
sudo tailscale --socket=/var/run/tailscale-userspace/tailscaled.sock status
sudo tailscale --socket=/var/run/tailscale-userspace/tailscaled.sock ip -4
