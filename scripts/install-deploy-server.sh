#!/bin/bash
set -eux -o pipefail

echo "=== Installing Deploy Server ==="

# Check if binary exists
if [ ! -f /tmp/deploy-server ]; then
    echo "ERROR: /tmp/deploy-server not found"
    echo "Please copy the deploy-server binary to /tmp/deploy-server first"
    exit 1
fi

# Move binary to /usr/local/bin
sudo mv /tmp/deploy-server /usr/local/bin/deploy-server
sudo chmod +x /usr/local/bin/deploy-server

# Create systemd service for deploy-server
sudo tee /etc/systemd/system/deploy-server.service > /dev/null << EOF
[Unit]
Description=Deploy Server (SSH execution over SOCKS5)
After=network.target tailscaled-userspace.service

[Service]
Type=simple
ExecStart=/usr/local/bin/deploy-server -listen :8082 -tailscale-socket /var/run/tailscale-userspace/tailscaled.sock
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Start deploy-server
sudo systemctl daemon-reload
sudo systemctl enable deploy-server
sudo systemctl start deploy-server

# Wait for service to start
sleep 2

# Check status
sudo systemctl status deploy-server --no-pager

echo ""
echo "=== Deploy Server installed ==="
echo "HTTP API listening on port 8082"
echo ""
echo "Endpoints:"
echo "  GET  /health  - Health check"
echo "  GET  /nodes   - List all nodes in the mesh network"
echo "  POST /exec    - Execute SSH command on a node"
echo ""
echo "Usage examples:"
echo "  curl http://localhost:8082/nodes"
echo ""
echo '  curl -X POST http://localhost:8082/exec \'
echo '    -H "Content-Type: application/json" \'
echo '    -d '"'"'{"host":"100.64.0.3","user":"strrl","password":"strrl","command":"hostname"}'"'"
