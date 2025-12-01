#!/bin/bash
set -eux -o pipefail

# Change to writable directory
cd /tmp

echo "=== Installing Traffic Gateway (Tailscale + nginx) ==="

# Install dependencies
sudo apt-get update
sudo apt-get install -y curl nginx jq

# Install Tailscale
curl -fsSL https://tailscale.com/install.sh | sh

# Read pre-auth key and Headscale URL
KEY_FILE="/tmp/lima/keys/traffic-gateway.key"
URL_FILE="/tmp/lima/keys/headscale-url.txt"

if [ ! -f "$KEY_FILE" ] || [ ! -f "$URL_FILE" ]; then
    echo "ERROR: Keys not found"
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
        --hostname="traffic-gateway" \
        --accept-routes \
        --timeout=60s; then
        echo "Connected successfully!"
        break
    fi
    echo "Connection attempt $attempt failed, retrying in 10s..."
    sleep 10
done

# Configure nginx as HTTP reverse proxy with WebSocket support
sudo tee /etc/nginx/sites-available/gateway > /dev/null << 'NGINX_EOF'
# Traffic Gateway - HTTP Reverse Proxy
# Routes requests to worker nodes over tailnet

# WebSocket upgrade mapping
map $http_upgrade $connection_upgrade {
    default upgrade;
    '' close;
}

server {
    listen 80;
    server_name _;

    # Health check endpoint
    location /health {
        return 200 'OK';
        add_header Content-Type text/plain;
    }

    # Route to worker-node-1 Cockpit
    # Placeholder IP will be updated by update-gateway-config.sh
    location /worker-node-1/ {
        proxy_pass http://127.0.0.1:19091/worker-node-1/;
        proxy_http_version 1.1;
        proxy_buffering off;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        gzip off;
    }

    # Route to worker-node-2 Cockpit
    # Placeholder IP will be updated by update-gateway-config.sh
    location /worker-node-2/ {
        proxy_pass http://127.0.0.1:19092/worker-node-2/;
        proxy_http_version 1.1;
        proxy_buffering off;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        gzip off;
    }

    # Default - show gateway status
    location / {
        return 200 'Traffic Gateway is running. Use /worker-node-1/ or /worker-node-2/ to access Cockpit.';
        add_header Content-Type text/plain;
    }
}
NGINX_EOF

# Enable the gateway site
sudo ln -sf /etc/nginx/sites-available/gateway /etc/nginx/sites-enabled/gateway
sudo rm -f /etc/nginx/sites-enabled/default

# Create script to update nginx config with actual worker IPs
sudo tee /usr/local/bin/update-gateway-config.sh > /dev/null << 'SCRIPT'
#!/bin/bash
# Update nginx config with worker tailnet IPs

WORKER1_IP=$(tailscale status --json | jq -r '.Peer[] | select(.HostName == "worker-node-1") | .TailscaleIPs[0]' 2>/dev/null)
WORKER2_IP=$(tailscale status --json | jq -r '.Peer[] | select(.HostName == "worker-node-2") | .TailscaleIPs[0]' 2>/dev/null)

if [ -n "$WORKER1_IP" ] && [ "$WORKER1_IP" != "null" ]; then
    echo "Worker 1 IP: $WORKER1_IP"
    sed -i "s|proxy_pass http://127.0.0.1:19091/worker-node-1/;|proxy_pass http://${WORKER1_IP}:9090/worker-node-1/;|g" /etc/nginx/sites-available/gateway
fi

if [ -n "$WORKER2_IP" ] && [ "$WORKER2_IP" != "null" ]; then
    echo "Worker 2 IP: $WORKER2_IP"
    sed -i "s|proxy_pass http://127.0.0.1:19092/worker-node-2/;|proxy_pass http://${WORKER2_IP}:9090/worker-node-2/;|g" /etc/nginx/sites-available/gateway
fi

nginx -t && systemctl reload nginx
echo "Gateway config updated"
SCRIPT
sudo chmod +x /usr/local/bin/update-gateway-config.sh

# Test and start nginx
sudo nginx -t
sudo systemctl enable nginx
sudo systemctl restart nginx

echo "=== Traffic Gateway setup complete ==="
tailscale status
tailscale ip -4
