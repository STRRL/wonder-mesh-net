# Getting Started

This guide walks through the full Wonder Mesh Net setup using Lima VMs.

## Prerequisites

- Lima (`brew install lima`)
- Go 1.21+
- GitHub OAuth App (for authentication)

## Part 1: Operator Setup

### 1.1 Create GitHub OAuth App

1. Go to GitHub Settings > Developer settings > OAuth Apps > New OAuth App
2. Set:
   - Application name: `Wonder Mesh Net`
   - Homepage URL: `http://localhost:9080`
   - Authorization callback URL: `http://localhost:9080/auth/callback`
3. Save Client ID and Client Secret

### 1.2 Start Headscale VM

```bash
# Create headscale VM
limactl start --name=headscale template://debian --tty=false

# Install headscale
limactl shell headscale <<'EOF'
curl -fsSL https://github.com/juanfont/headscale/releases/download/v0.23.0/headscale_0.23.0_linux_amd64.deb -o /tmp/headscale.deb
sudo dpkg -i /tmp/headscale.deb

# Configure
sudo mkdir -p /etc/headscale
sudo tee /etc/headscale/config.yaml > /dev/null <<'CONFIG'
server_url: http://localhost:8080
listen_addr: 0.0.0.0:8080
private_key_path: /var/lib/headscale/private.key
noise:
  private_key_path: /var/lib/headscale/noise_private.key
prefixes:
  v4: 100.64.0.0/10
  v6: fd7a:115c:a1e0::/48
database:
  type: sqlite
  sqlite:
    path: /var/lib/headscale/db.sqlite
CONFIG

sudo mkdir -p /var/lib/headscale
sudo systemctl enable --now headscale
EOF

# Create API key
limactl shell headscale -- sudo headscale apikeys create
# Save this key as HEADSCALE_API_KEY
```

### 1.3 Build and Run Coordinator

```bash
# Build wonder CLI
go build -o wonder ./cmd/wonder/

# Set environment variables
export HEADSCALE_API_KEY="your-api-key-from-above"
export GITHUB_CLIENT_ID="your-github-client-id"
export GITHUB_CLIENT_SECRET="your-github-client-secret"

# Get headscale VM IP
HEADSCALE_IP=$(limactl shell headscale -- hostname -I | awk '{print $1}')

# Run coordinator
./wonder coordinator \
  --listen :9080 \
  --headscale-url "http://${HEADSCALE_IP}:8080" \
  --public-url "http://localhost:9080"
```

Coordinator is now running on `http://localhost:9080`.

## Part 2: End User Flow

### 2.1 Login via GitHub

Open browser:

```
http://localhost:9080/auth/login?provider=github
```

After GitHub OAuth, you'll be redirected to `/auth/complete` with JSON:

```json
{
  "session": "a1b2c3d4e5f6...",
  "user": "tenant-a1b2c3d4e5f6"
}
```

Save the `session` value.

### 2.2 Generate Join Token

```bash
SESSION="your-session-from-above"

curl -X POST http://localhost:9080/api/v1/join-token \
  -H "X-Session-Token: ${SESSION}" \
  -H "Content-Type: application/json" \
  -d '{"ttl": "1h"}'
```

Response:

```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "command": "wonder worker join eyJhbGciOiJIUzI1NiIs..."
}
```

### 2.3 Create Worker VM

```bash
# Create worker VM
limactl start --name=worker-1 template://debian --tty=false

# Install tailscale
limactl shell worker-1 <<'EOF'
curl -fsSL https://tailscale.com/install.sh | sh
EOF
```

### 2.4 Join Worker to Mesh

Copy the `wonder` binary and join token to the worker:

```bash
# Copy wonder binary
limactl copy ./wonder worker-1:/tmp/wonder
limactl shell worker-1 -- sudo mv /tmp/wonder /usr/local/bin/wonder
limactl shell worker-1 -- sudo chmod +x /usr/local/bin/wonder

# Join the mesh
TOKEN="eyJhbGciOiJIUzI1NiIs..."
limactl shell worker-1 -- wonder worker join "${TOKEN}"
```

Output:

```
Joining Wonder Mesh Net...
  Coordinator: http://localhost:9080
  User: tenant-a1b2c3d4e5f6
  Token expires: 2024-12-04T10:00:00Z

Successfully obtained auth key!

To complete the setup, run on this device:
  sudo tailscale up --login-server=http://192.168.x.x:8080 --authkey=hskey-xxx

Would you like to run this command now? [y/N]:
```

Press `y` to connect.

### 2.5 Verify Connection

```bash
# Check tailscale status on worker
limactl shell worker-1 -- tailscale status

# Check nodes via API
curl http://localhost:9080/api/v1/nodes \
  -H "X-Session-Token: ${SESSION}"
```

## Part 3: Workload Manager Integration

### 3.1 Using Wonder SDK

Example Go program simulating a Workload Manager:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/strrl/wonder-mesh-net/pkg/wondersdk"
)

func main() {
    client := wondersdk.NewClient("http://localhost:9080", "")

    session := "your-session-token"

    nodes, err := client.ListNodes(context.Background(), session)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Available nodes (%d):\n", len(nodes))
    for _, node := range nodes {
        status := "offline"
        if node.Online {
            status = "online"
        }
        fmt.Printf("  - %s (%v) [%s]\n", node.Name, node.Addresses, status)
    }

    // Deploy to online nodes
    for _, node := range nodes {
        if node.Online && len(node.Addresses) > 0 {
            fmt.Printf("\nDeploying to %s at %s...\n", node.Name, node.Addresses[0])
            // SSH/exec to node.Addresses[0] via mesh network
        }
    }
}
```

### 3.2 Deploy Demo Workload

```bash
# SSH to worker via mesh IP (from another node in the mesh)
WORKER_IP=$(curl -s http://localhost:9080/api/v1/nodes \
  -H "X-Session-Token: ${SESSION}" | jq -r '.nodes[0].ipAddresses[0]')

# From a machine in the mesh, deploy a container
ssh user@${WORKER_IP} "docker run -d -p 80:80 nginx"
```

## Cleanup

```bash
# Stop coordinator
Ctrl+C

# Delete VMs
limactl delete headscale worker-1 --force
```

## API Reference

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/health` | GET | - | Health check |
| `/auth/providers` | GET | - | List OIDC providers |
| `/auth/login?provider=xxx` | GET | - | Start OAuth flow |
| `/auth/complete` | GET | - | OAuth completion (returns session) |
| `/api/v1/join-token` | POST | Session | Generate join token |
| `/api/v1/worker/join` | POST | JWT | Exchange token for authkey |
| `/api/v1/nodes` | GET | Session | List user's nodes |
