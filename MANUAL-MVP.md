# Wonder Mesh Net - Manual MVP Setup

This guide walks through setting up the MVP mesh network using Lima VMs.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    CLOUD PROVIDER                           │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │  Headscale  │  │   nginx     │  │   deploy-manager    │ │
│  │  (control)  │  │  (gateway)  │  │ (SOCKS5 + CLI)      │ │
│  └──────┬──────┘  └──────┬──────┘  └──────────┬──────────┘ │
└─────────┼────────────────┼───────────────────┼─────────────┘
          │                │                    │
          │         ┌──────┴────────────────────┘
          │         │      tailnet (WireGuard)
          │         │
┌─────────┼─────────┼────────────────────────────────────────┐
│         │         │           USER SIDE                    │
│  ┌──────┴─────┐  ┌┴────────────┐  ┌─────────────────────┐ │
│  │ worker-1   │  │  worker-2   │  │      (more...)      │ │
│  │ Cockpit    │  │  Cockpit    │  │                     │ │
│  └────────────┘  └─────────────┘  └─────────────────────┘ │
└────────────────────────────────────────────────────────────┘
```

## Prerequisites

- Lima installed (`brew install lima`)
- Go installed (for building deploy-cli)

## Phase 1: Bootstrap All VMs

### 1.1 Create shared directory for keys

```bash
mkdir -p /tmp/lima/keys
```

### 1.2 Start all VMs

```bash
cd /path/to/wonder-mesh-net

# Start network-coordinator (port 8080 -> Headscale)
limactl start --name=network-coordinator ./lima/network-coordinator-vm.yaml --tty=false

# Start traffic-gateway (port 8081 -> nginx)
limactl start --name=traffic-gateway ./lima/traffic-gateway-vm.yaml --tty=false

# Start deploy-manager
limactl start --name=deploy-manager ./lima/base-vm.yaml --tty=false

# Start worker nodes
limactl start --name=worker-node-1 ./lima/base-vm.yaml --tty=false
limactl start --name=worker-node-2 ./lima/base-vm.yaml --tty=false
```

### 1.3 Verify all VMs are running

```bash
limactl list
```

Expected output: All 5 VMs should show `Running` status.

## Phase 2: Setup Mesh Network

### 2.1 Setup Network Coordinator (Headscale)

```bash
limactl shell network-coordinator < ./scripts/setup-network-coordinator.sh
```

This will:
- Install Headscale
- Configure it to listen on port 8080
- Create a `mesh` user
- Generate pre-auth keys for all nodes
- Save keys to `/tmp/lima/keys/`

Verify keys were created:

```bash
ls -la /tmp/lima/keys/
cat /tmp/lima/keys/headscale-url.txt
```

### 2.2 Setup Traffic Gateway (nginx + Tailscale)

```bash
limactl shell traffic-gateway < ./scripts/setup-traffic-gateway.sh
```

This will:
- Install Tailscale and nginx
- Join the tailnet using the pre-auth key
- Configure nginx as an HTTP reverse proxy

Verify connection:

```bash
limactl shell traffic-gateway -- tailscale status
```

### 2.3 Setup Deploy Manager (tailscaled userspace + SOCKS)

```bash
limactl shell deploy-manager < ./scripts/setup-deploy-manager.sh
```

This will:
- Install Tailscale in userspace mode (no TUN device)
- Enable SOCKS5 proxy on localhost:1080
- Join the tailnet

Verify connection:

```bash
limactl shell deploy-manager -- sudo tailscale --socket=/var/run/tailscale-userspace/tailscaled.sock status
```

### 2.4 Setup Worker Nodes

```bash
limactl shell worker-node-1 < ./scripts/setup-worker-node.sh
limactl shell worker-node-2 < ./scripts/setup-worker-node.sh
```

This will:
- Install Tailscale and join the tailnet
- Install Cockpit web console
- Configure Cockpit with UrlRoot for reverse proxy
- Enable SSH password authentication
- Set password for the current user (same as your macOS username)

Verify all nodes are connected:

```bash
limactl shell network-coordinator -- sudo headscale nodes list
```

### 2.5 Update traffic gateway nginx config

```bash
limactl shell traffic-gateway -- sudo /usr/local/bin/update-gateway-config.sh
```

This updates nginx to route to the actual worker node tailnet IPs.

## Access Cockpit

Once everything is set up:

- **worker-node-1**: http://localhost:8081/worker-node-1/
- **worker-node-2**: http://localhost:8081/worker-node-2/

**Login credentials**: Use your macOS username for both username and password (e.g., `strrl` / `strrl`)

## Optional: Deploy-CLI for Remote Management

If you want to use deploy-cli for remote SSH management over the tailnet:

### Build and install deploy-cli

```bash
# Build for Linux ARM64 (Apple Silicon) or AMD64 (Intel)
GOOS=linux GOARCH=arm64 go build -o /tmp/deploy-cli ./cmd/deploy-cli/

# Copy to deploy-manager
limactl copy /tmp/deploy-cli deploy-manager:/tmp/deploy-cli
limactl shell deploy-manager -- sudo mv /tmp/deploy-cli /usr/local/bin/deploy-cli
limactl shell deploy-manager -- sudo chmod +x /usr/local/bin/deploy-cli
```

### Get worker tailnet IPs

```bash
WORKER1_IP=$(limactl shell worker-node-1 -- tailscale ip -4 | tr -d '\r\n')
WORKER2_IP=$(limactl shell worker-node-2 -- tailscale ip -4 | tr -d '\r\n')
echo "Worker 1: $WORKER1_IP"
echo "Worker 2: $WORKER2_IP"
```

### Check connectivity

```bash
# Replace YOUR_USERNAME with your macOS username
limactl shell deploy-manager -- deploy-cli -user YOUR_USERNAME -pass YOUR_USERNAME status "$WORKER1_IP" "$WORKER2_IP"
```

## Useful Commands

```bash
# SSH into VMs
limactl shell network-coordinator
limactl shell traffic-gateway
limactl shell deploy-manager
limactl shell worker-node-1
limactl shell worker-node-2

# Check Headscale status
limactl shell network-coordinator -- sudo headscale nodes list
limactl shell network-coordinator -- sudo headscale users list

# Check Tailscale status on any node
limactl shell traffic-gateway -- tailscale status
limactl shell worker-node-1 -- tailscale status

# Check deploy-manager tailscale (uses custom socket)
limactl shell deploy-manager -- sudo tailscale --socket=/var/run/tailscale-userspace/tailscaled.sock status

# List all VMs
limactl list

# Stop all VMs
limactl stop --all

# Delete all VMs
limactl delete --all
```

## Cleanup

Run the cleanup script:

```bash
./cleanup.sh
```

Or manually:

```bash
limactl stop network-coordinator traffic-gateway deploy-manager worker-node-1 worker-node-2
limactl delete network-coordinator traffic-gateway deploy-manager worker-node-1 worker-node-2
rm -rf /tmp/lima/keys
```

## Troubleshooting

### Tailscale can't connect to Headscale

Check that the headscale URL is correct:

```bash
cat /tmp/lima/keys/headscale-url.txt
# Should be: http://192.168.5.2:8080
```

Lima VMs reach the host at `192.168.5.2`. The host forwards port 8080 to network-coordinator.

If the URL is wrong, fix it:

```bash
echo "http://192.168.5.2:8080" > /tmp/lima/keys/headscale-url.txt
limactl shell network-coordinator -- sudo sed -i 's|server_url: .*|server_url: http://192.168.5.2:8080|' /etc/headscale/config.yaml
limactl shell network-coordinator -- sudo systemctl restart headscale
```

### Read-only filesystem error

Scripts must run in `/tmp`, not in the home directory (which is mounted read-only from host).

### Check Headscale logs

```bash
limactl shell network-coordinator -- sudo journalctl -u headscale -f
```

### Check Tailscale logs

```bash
limactl shell traffic-gateway -- sudo journalctl -u tailscaled -f
```
