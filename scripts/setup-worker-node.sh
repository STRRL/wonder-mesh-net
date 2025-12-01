#!/bin/bash
set -eux -o pipefail

# Change to writable directory
cd /tmp

# Get the Lima instance name (strip 'lima-' prefix if present)
RAW_HOSTNAME=$(hostname)
HOSTNAME=${RAW_HOSTNAME#lima-}
echo "=== Installing Tailscale on Worker Node: $HOSTNAME ==="

# Install dependencies
sudo apt-get update
sudo apt-get install -y curl

# Install Tailscale
curl -fsSL https://tailscale.com/install.sh | sh

# Read pre-auth key and Headscale URL
KEY_FILE="/tmp/lima/keys/${HOSTNAME}.key"
URL_FILE="/tmp/lima/keys/headscale-url.txt"

if [ ! -f "$KEY_FILE" ] || [ ! -f "$URL_FILE" ]; then
    echo "ERROR: Keys not found for $HOSTNAME"
    echo "Expected key file: $KEY_FILE"
    ls -la /tmp/lima/keys/ || true
    exit 1
fi

AUTH_KEY=$(cat "$KEY_FILE")
HEADSCALE_URL=$(cat "$URL_FILE")

# Configure Tailscale to use Headscale
echo "Connecting to Headscale at $HEADSCALE_URL..."
echo "Using auth key: $AUTH_KEY"

for attempt in {1..5}; do
    if sudo tailscale up \
        --login-server="$HEADSCALE_URL" \
        --authkey="$AUTH_KEY" \
        --hostname="$HOSTNAME" \
        --accept-routes \
        --timeout=60s; then
        echo "Connected successfully!"
        break
    fi
    echo "Connection attempt $attempt failed, retrying in 10s..."
    sleep 10
done

# Enable SSH password authentication for deploy-cli
sudo sed -i 's/^#*PasswordAuthentication.*/PasswordAuthentication yes/' /etc/ssh/sshd_config
sudo sed -i 's/^#*ChallengeResponseAuthentication.*/ChallengeResponseAuthentication no/' /etc/ssh/sshd_config
# Override cloud-init settings that may disable password auth
echo 'PasswordAuthentication yes' | sudo tee /etc/ssh/sshd_config.d/60-cloudimg-settings.conf
sudo systemctl restart sshd

# Set password for the current user (Lima uses host username, not ubuntu)
CURRENT_USER=$(whoami)
echo "${CURRENT_USER}:${CURRENT_USER}" | sudo chpasswd
echo "Password set for user: ${CURRENT_USER}"

# Install Cockpit for web-based management
sudo apt-get install -y cockpit
sudo systemctl enable cockpit.socket

# Configure Cockpit for reverse proxy with path prefix
# The UrlRoot will be set by deploy-cli based on the worker name
sudo mkdir -p /etc/cockpit
sudo tee /etc/cockpit/cockpit.conf > /dev/null << EOF
[WebService]
Origins = http://localhost:8081 ws://localhost:8081
ProtocolHeader = X-Forwarded-Proto
UrlRoot = /${HOSTNAME}
AllowUnencrypted = true
EOF

sudo systemctl restart cockpit.socket

echo "=== Tailscale setup complete ==="
tailscale status
tailscale ip -4
echo "Cockpit configured with UrlRoot=/${HOSTNAME}"
