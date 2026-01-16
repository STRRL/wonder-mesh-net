package deployer

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/strrl/wonder-mesh-net/pkg/wondersdk"
)

const (
	kubeVersion    = "1.31"
	podNetworkCIDR = "10.244.0.0/16"
)

// Config holds the deployer configuration
type Config struct {
	CoordinatorURL string
	APIKey         string
	SSHUser        string
	SSHPassword    string
	SOCKS5Addr     string
}

// Deployer orchestrates Kubernetes cluster bootstrap
type Deployer struct {
	config      Config
	sdkClient   *wondersdk.Client
	sshExecutor *SSHExecutor

	// Tailscale IPs - used for SSH connectivity via SOCKS5 proxy
	controlPlaneTailscaleIP string
	workerTailscaleIPs      []string

	// Internal IPs (Docker network) - used for kubeadm control plane
	controlPlaneInternalIP string
	workerInternalIPs      []string

	kubeconfig string
}

// NewDeployer creates a new Deployer instance
func NewDeployer(config Config) (*Deployer, error) {
	if config.SSHUser == "" {
		config.SSHUser = "root"
	}
	if config.SSHPassword == "" {
		config.SSHPassword = "KubeadmWorkerDemoOnly!123"
	}
	if config.SOCKS5Addr == "" {
		config.SOCKS5Addr = "localhost:1080"
	}

	sdkClient := wondersdk.NewClient(config.CoordinatorURL, config.APIKey)

	sshConfig := SSHConfig{
		User:       config.SSHUser,
		Password:   config.SSHPassword,
		SOCKS5Addr: config.SOCKS5Addr,
		Timeout:    30 * time.Second,
	}

	executor, err := NewSSHExecutor(sshConfig)
	if err != nil {
		return nil, fmt.Errorf("create SSH executor: %w", err)
	}

	return &Deployer{
		config:      config,
		sdkClient:   sdkClient,
		sshExecutor: executor,
	}, nil
}

// Run executes the full deployment flow
func (d *Deployer) Run(ctx context.Context) error {
	slog.Info("starting Kubernetes cluster deployment")

	if err := d.sdkClient.Health(ctx); err != nil {
		return fmt.Errorf("coordinator health check: %w", err)
	}
	slog.Info("coordinator is healthy")

	nodes, err := d.discoverNodes(ctx)
	if err != nil {
		return err
	}

	if err := d.selectNodes(nodes); err != nil {
		return err
	}

	if err := d.waitForSSH(ctx, 2*time.Minute); err != nil {
		return fmt.Errorf("wait for SSH: %w", err)
	}

	if err := d.discoverInternalIPs(ctx); err != nil {
		return fmt.Errorf("discover internal IPs: %w", err)
	}

	if err := d.installPrerequisites(ctx); err != nil {
		return fmt.Errorf("install prerequisites: %w", err)
	}

	joinCommand, err := d.initControlPlane(ctx)
	if err != nil {
		return fmt.Errorf("init control plane: %w", err)
	}

	if err := d.installFlannel(ctx); err != nil {
		return fmt.Errorf("install CNI: %w", err)
	}

	if err := d.patchCoreDNS(ctx); err != nil {
		return fmt.Errorf("patch CoreDNS: %w", err)
	}

	if err := d.joinWorkers(ctx, joinCommand); err != nil {
		return fmt.Errorf("join workers: %w", err)
	}

	if err := d.waitForCluster(ctx, 5*time.Minute); err != nil {
		return fmt.Errorf("wait for cluster: %w", err)
	}

	if err := d.verifyCluster(ctx); err != nil {
		slog.Warn("cluster verification", "error", err)
	}

	fmt.Println("\n=== Deployment Complete ===")
	fmt.Printf("Control Plane: %s (internal), %s (tailscale)\n", d.controlPlaneInternalIP, d.controlPlaneTailscaleIP)
	fmt.Printf("Workers: %v (internal)\n", d.workerInternalIPs)
	fmt.Println("\nTo access the cluster from the deployer:")
	fmt.Printf("  kubectl --kubeconfig /tmp/kubeconfig get nodes\n")

	return nil
}

// SaveKubeconfig saves the admin kubeconfig to a file
func (d *Deployer) SaveKubeconfig(path string) error {
	if d.kubeconfig == "" {
		return fmt.Errorf("no kubeconfig available")
	}

	if err := os.WriteFile(path, []byte(d.kubeconfig), 0600); err != nil {
		return fmt.Errorf("write kubeconfig: %w", err)
	}

	slog.Info("kubeconfig saved", "path", path)
	return nil
}

// GetControlPlaneIP returns the control plane internal IP
func (d *Deployer) GetControlPlaneIP() string {
	return d.controlPlaneInternalIP
}

// GetWorkerIPs returns the worker internal IPs
func (d *Deployer) GetWorkerIPs() []string {
	return d.workerInternalIPs
}

// GetKubeconfig returns the admin kubeconfig
func (d *Deployer) GetKubeconfig() string {
	return d.kubeconfig
}

// Reset resets all nodes in the cluster.
// WARNING: This is a destructive operation. The iptables flush will disrupt
// active network connections. Only use in demo/test environments or when
// intentionally tearing down a cluster.
func (d *Deployer) Reset(ctx context.Context) error {
	// Use Tailscale IPs for SSH access
	allTailscaleIPs := append([]string{d.controlPlaneTailscaleIP}, d.workerTailscaleIPs...)
	for _, ip := range allTailscaleIPs {
		if err := d.resetNode(ctx, ip); err != nil {
			slog.Warn("reset node", "node", ip, "error", err)
		}
	}
	return nil
}

func (d *Deployer) discoverNodes(ctx context.Context) ([]wondersdk.Node, error) {
	slog.Info("discovering nodes from coordinator")

	allNodes, err := d.sdkClient.GetOnlineNodes(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("get online nodes: %w", err)
	}

	hostname, _ := os.Hostname()

	nodes := make([]wondersdk.Node, 0, len(allNodes))
	for _, node := range allNodes {
		if node.Name == hostname {
			slog.Debug("skipping self", "name", node.Name)
			continue
		}
		nodes = append(nodes, node)
	}

	slog.Info("discovered nodes", "count", len(nodes), "excluded_self", hostname)
	for _, node := range nodes {
		slog.Debug("node", "name", node.Name, "addresses", node.Addresses, "online", node.Online)
	}

	return nodes, nil
}

// selectIPv4 returns the first IPv4 address from the list.
// Headscale/Tailscale may return both IPv4 and IPv6, and the ordering is not
// guaranteed. IPv6 literals without brackets break SSH address formatting, so
// we prefer IPv4. Falls back to the first address if no IPv4 is found.
func selectIPv4(addresses []string) string {
	for _, addr := range addresses {
		ip := net.ParseIP(addr)
		if ip != nil && ip.To4() != nil {
			return addr
		}
	}
	if len(addresses) > 0 {
		return addresses[0]
	}
	return ""
}

// selectNodes assigns roles to mesh nodes for Kubernetes cluster deployment.
// The first node in the slice becomes the control plane; remaining nodes become workers.
// It extracts IPv4 addresses from each node's Tailscale addresses for SSH connectivity.
//
// The function populates:
//   - d.controlPlaneTailscaleIP: the control plane node's IPv4 address
//   - d.workerTailscaleIPs: slice of worker nodes' IPv4 addresses
//
// Returns an error if fewer than 1 node is provided or if the control plane node
// has no valid IPv4 address. Worker nodes without IPv4 addresses are silently skipped.
func (d *Deployer) selectNodes(nodes []wondersdk.Node) error {
	if len(nodes) < 1 {
		return fmt.Errorf("at least 1 node required, found %d", len(nodes))
	}

	d.controlPlaneTailscaleIP = selectIPv4(nodes[0].Addresses)
	if d.controlPlaneTailscaleIP == "" {
		return fmt.Errorf("control plane node has no IP address")
	}

	d.workerTailscaleIPs = make([]string, 0, len(nodes)-1)
	for i := 1; i < len(nodes); i++ {
		addr := selectIPv4(nodes[i].Addresses)
		if addr != "" {
			d.workerTailscaleIPs = append(d.workerTailscaleIPs, addr)
		}
	}

	slog.Info("node selection (Tailscale IPs for SSH)",
		"control_plane", d.controlPlaneTailscaleIP,
		"workers", d.workerTailscaleIPs,
	)

	return nil
}

func (d *Deployer) waitForSSH(ctx context.Context, timeout time.Duration) error {
	slog.Info("waiting for SSH connectivity")

	allIPs := append([]string{d.controlPlaneTailscaleIP}, d.workerTailscaleIPs...)
	return d.sshExecutor.WaitForAllNodes(ctx, allIPs, timeout)
}

// discoverInternalIPs queries each node for its Docker network IP (eth0)
func (d *Deployer) discoverInternalIPs(ctx context.Context) error {
	slog.Info("discovering internal IPs for kubeadm")

	// Get control plane internal IP
	result, err := d.sshExecutor.RunOnNode(ctx, d.controlPlaneTailscaleIP,
		"ip -4 addr show eth0 | grep -oP '(?<=inet\\s)\\d+(\\.\\d+){3}'")
	if err != nil {
		return fmt.Errorf("get control plane internal IP: %w", err)
	}
	d.controlPlaneInternalIP = strings.TrimSpace(result.Stdout)
	if d.controlPlaneInternalIP == "" {
		return fmt.Errorf("control plane has no eth0 IP")
	}

	// Get worker internal IPs
	d.workerInternalIPs = make([]string, 0, len(d.workerTailscaleIPs))
	for _, tailscaleIP := range d.workerTailscaleIPs {
		result, err := d.sshExecutor.RunOnNode(ctx, tailscaleIP,
			"ip -4 addr show eth0 | grep -oP '(?<=inet\\s)\\d+(\\.\\d+){3}'")
		if err != nil {
			return fmt.Errorf("get worker internal IP for %s: %w", tailscaleIP, err)
		}
		internalIP := strings.TrimSpace(result.Stdout)
		if internalIP == "" {
			return fmt.Errorf("worker %s has no eth0 IP", tailscaleIP)
		}
		d.workerInternalIPs = append(d.workerInternalIPs, internalIP)
	}

	slog.Info("discovered internal IPs (Docker network for kubeadm)",
		"control_plane", d.controlPlaneInternalIP,
		"workers", d.workerInternalIPs,
	)

	return nil
}

func (d *Deployer) installPrerequisites(ctx context.Context) error {
	slog.Info("installing prerequisites on all nodes")

	// Use Tailscale IPs for SSH access
	allTailscaleIPs := append([]string{d.controlPlaneTailscaleIP}, d.workerTailscaleIPs...)
	for idx, ip := range allTailscaleIPs {
		slog.Info("installing on node", "node", ip, "index", idx+1, "total", len(allTailscaleIPs))
		if err := d.installOnNode(ctx, ip); err != nil {
			return fmt.Errorf("node %s: %w", ip, err)
		}
	}
	return nil
}

func (d *Deployer) installOnNode(ctx context.Context, nodeIP string) error {
	if err := d.configurePrerequisites(ctx, nodeIP); err != nil {
		return fmt.Errorf("configure prerequisites: %w", err)
	}

	if err := d.installContainerd(ctx, nodeIP); err != nil {
		return fmt.Errorf("install containerd: %w", err)
	}

	if err := d.installKubeadm(ctx, nodeIP); err != nil {
		return fmt.Errorf("install kubeadm: %w", err)
	}

	return nil
}

func (d *Deployer) configurePrerequisites(ctx context.Context, nodeIP string) error {
	slog.Info("configuring prerequisites", "node", nodeIP)

	commands := []struct {
		name string
		cmd  string
	}{
		{
			name: "load kernel modules",
			cmd: `
modprobe br_netfilter || true
modprobe overlay || true

cat > /etc/modules-load.d/k8s.conf << 'EOF'
br_netfilter
overlay
EOF
`,
		},
		{
			name: "configure sysctl",
			cmd: `
cat > /etc/sysctl.d/k8s.conf << 'EOF'
net.bridge.bridge-nf-call-iptables = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward = 1
EOF

sysctl --system
`,
		},
		{
			name: "disable swap",
			cmd: `
swapoff -a || true
sed -i '/swap/d' /etc/fstab || true
`,
		},
	}

	for _, c := range commands {
		slog.Debug("running", "step", c.name, "node", nodeIP)
		result, err := d.sshExecutor.RunOnNode(ctx, nodeIP, c.cmd)
		if err != nil {
			return fmt.Errorf("%s: %w", c.name, err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("%s: exit code %d, stderr: %s", c.name, result.ExitCode, result.Stderr)
		}
	}

	return nil
}

// installContainerd installs containerd runtime.
// NOTE: This downloads packages from Docker's official repository with GPG verification.
// For production, ensure you're connecting to legitimate sources and consider
// additional verification or using pre-built images.
func (d *Deployer) installContainerd(ctx context.Context, nodeIP string) error {
	slog.Info("installing containerd", "node", nodeIP)

	installCmd := `
set -e

if command -v containerd &>/dev/null; then
    echo "containerd already installed"
    exit 0
fi

apt-get update
apt-get install -y apt-transport-https ca-certificates curl gnupg

install -m 0755 -d /etc/apt/keyrings

curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
chmod a+r /etc/apt/keyrings/docker.gpg

echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
  tee /etc/apt/sources.list.d/docker.list > /dev/null

apt-get update
apt-get install -y containerd.io

mkdir -p /etc/containerd
containerd config default > /etc/containerd/config.toml

sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml

systemctl restart containerd
systemctl enable containerd

echo "containerd installed successfully"
`

	result, err := d.sshExecutor.RunOnNode(ctx, nodeIP, installCmd)
	if err != nil {
		return fmt.Errorf("install containerd: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("install containerd: exit code %d, stderr: %s", result.ExitCode, result.Stderr)
	}

	slog.Info("containerd installed", "node", nodeIP)
	return nil
}

func (d *Deployer) installKubeadm(ctx context.Context, nodeIP string) error {
	slog.Info("installing kubeadm", "node", nodeIP, "version", kubeVersion)

	installCmd := fmt.Sprintf(`
set -e

if command -v kubeadm &>/dev/null; then
    echo "kubeadm already installed"
    exit 0
fi

apt-get update
apt-get install -y apt-transport-https ca-certificates curl gpg

mkdir -p /etc/apt/keyrings

curl -fsSL https://pkgs.k8s.io/core:/stable:/v%s/deb/Release.key | gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg

echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v%s/deb/ /" | \
  tee /etc/apt/sources.list.d/kubernetes.list

apt-get update
apt-get install -y kubelet kubeadm kubectl
apt-mark hold kubelet kubeadm kubectl

systemctl enable kubelet

echo "kubeadm installed successfully"
`, kubeVersion, kubeVersion)

	result, err := d.sshExecutor.RunOnNode(ctx, nodeIP, installCmd)
	if err != nil {
		return fmt.Errorf("install kubeadm: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("install kubeadm: exit code %d, stderr: %s", result.ExitCode, result.Stderr)
	}

	slog.Info("kubeadm installed", "node", nodeIP)
	return nil
}

func (d *Deployer) initControlPlane(ctx context.Context) (string, error) {
	slog.Info("initializing control plane",
		"ssh", d.controlPlaneTailscaleIP,
		"apiserver", d.controlPlaneInternalIP)

	// Use internal IP for kubeadm (Docker network), SSH via Tailscale
	// Use kubeadm config file to configure kube-proxy to skip conntrack settings
	// (required for Docker Desktop where /proc/sys is read-only)
	initCmd := fmt.Sprintf(`
set -e

cat > /tmp/kubeadm-config.yaml << 'EOF'
apiVersion: kubeadm.k8s.io/v1beta4
kind: InitConfiguration
localAPIEndpoint:
  advertiseAddress: %s
nodeRegistration:
  ignorePreflightErrors:
    - all
---
apiVersion: kubeadm.k8s.io/v1beta4
kind: ClusterConfiguration
networking:
  podSubnet: %s
---
apiVersion: kubeproxy.config.k8s.io/v1alpha1
kind: KubeProxyConfiguration
conntrack:
  maxPerCore: 0
EOF

kubeadm init --config /tmp/kubeadm-config.yaml 2>&1

mkdir -p /root/.kube
cp /etc/kubernetes/admin.conf /root/.kube/config
chown root:root /root/.kube/config

echo "=== KUBECONFIG ==="
cat /root/.kube/config
echo "=== END KUBECONFIG ==="

echo "=== JOIN COMMAND ==="
kubeadm token create --print-join-command
echo "=== END JOIN COMMAND ==="
`, d.controlPlaneInternalIP, podNetworkCIDR)

	// SSH via Tailscale IP
	result, err := d.sshExecutor.RunOnNode(ctx, d.controlPlaneTailscaleIP, initCmd)
	if err != nil {
		return "", fmt.Errorf("kubeadm init: %w", err)
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("kubeadm init: exit code %d, stderr: %s", result.ExitCode, result.Stderr)
	}

	kubeconfigRe := regexp.MustCompile(`=== KUBECONFIG ===\n([\s\S]*?)=== END KUBECONFIG ===`)
	if match := kubeconfigRe.FindStringSubmatch(result.Stdout); len(match) > 1 {
		d.kubeconfig = strings.TrimSpace(match[1])
	}

	var joinCommand string
	joinCmdRe := regexp.MustCompile(`=== JOIN COMMAND ===\n([\s\S]*?)=== END JOIN COMMAND ===`)
	if match := joinCmdRe.FindStringSubmatch(result.Stdout); len(match) > 1 {
		joinCommand = strings.TrimSpace(match[1])
	}

	slog.Info("control plane initialized",
		"node", d.controlPlaneInternalIP,
		"has_kubeconfig", d.kubeconfig != "",
		"has_join_command", joinCommand != "",
	)

	return joinCommand, nil
}

// installFlannel installs Flannel CNI.
// Flannel is used because it works well in containerized environments
// without requiring BPF filesystem access.
func (d *Deployer) installFlannel(ctx context.Context) error {
	slog.Info("installing Flannel CNI", "node", d.controlPlaneInternalIP)

	// With internal Docker IPs, standard Flannel setup works without hacks
	installCmd := `
set -e

# Flannel manifest URL is pinned to a specific version for reproducibility.
# Update the version tag consciously after testing compatibility.
kubectl apply -f https://github.com/flannel-io/flannel/releases/download/v0.26.7/kube-flannel.yml

echo "Flannel installation initiated"

echo "Waiting for Flannel to be ready..."
for i in $(seq 1 30); do
    READY=$(kubectl -n kube-flannel get pods -l app=flannel --no-headers 2>/dev/null | grep -c Running || true)
    if [ "$READY" -ge 1 ]; then
        echo "Flannel pod is running"
        break
    fi
    echo "  Waiting for Flannel pod... ($i/30)"
    sleep 10
done

echo "Flannel CNI installed"
`

	// SSH via Tailscale IP
	result, err := d.sshExecutor.RunOnNode(ctx, d.controlPlaneTailscaleIP, installCmd)
	if err != nil {
		return fmt.Errorf("flannel install: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("flannel install: exit code %d, output: %s, stderr: %s",
			result.ExitCode, result.Stdout, result.Stderr)
	}

	slog.Info("Flannel CNI installed", "node", d.controlPlaneInternalIP)
	return nil
}

// patchCoreDNS patches the CoreDNS ConfigMap to use external DNS servers.
// This fixes a loop detection issue in containerized environments where
// /etc/resolv.conf points to localhost or creates a DNS loop.
func (d *Deployer) patchCoreDNS(ctx context.Context) error {
	slog.Info("patching CoreDNS to use external DNS", "node", d.controlPlaneInternalIP)

	patchCmd := `
set -e

# Wait for CoreDNS ConfigMap to exist
for i in $(seq 1 30); do
    if kubectl get configmap coredns -n kube-system &>/dev/null; then
        break
    fi
    echo "Waiting for CoreDNS ConfigMap... ($i/30)"
    sleep 2
done

# Patch CoreDNS to use external DNS servers instead of /etc/resolv.conf
# This prevents the "Loop detected" issue in containerized environments
kubectl patch configmap coredns -n kube-system --type merge -p '{"data":{"Corefile":".:53 {\n    errors\n    health {\n       lameduck 5s\n    }\n    ready\n    kubernetes cluster.local in-addr.arpa ip6.arpa {\n       pods insecure\n       fallthrough in-addr.arpa ip6.arpa\n       ttl 30\n    }\n    prometheus :9153\n    forward . 8.8.8.8 8.8.4.4 {\n       max_concurrent 1000\n    }\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n"}}'

# Restart CoreDNS to pick up the new config
kubectl rollout restart deployment coredns -n kube-system

echo "CoreDNS patched successfully"
`

	result, err := d.sshExecutor.RunOnNode(ctx, d.controlPlaneTailscaleIP, patchCmd)
	if err != nil {
		return fmt.Errorf("patch CoreDNS: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("patch CoreDNS: exit code %d, output: %s, stderr: %s",
			result.ExitCode, result.Stdout, result.Stderr)
	}

	slog.Info("CoreDNS patched", "node", d.controlPlaneInternalIP)
	return nil
}

func (d *Deployer) joinWorkers(ctx context.Context, joinCommand string) error {
	if len(d.workerTailscaleIPs) == 0 {
		slog.Info("no worker nodes to join")
		return nil
	}

	slog.Info("joining worker nodes", "count", len(d.workerTailscaleIPs))

	// SSH via Tailscale IPs
	for idx, tailscaleIP := range d.workerTailscaleIPs {
		slog.Info("joining worker", "ssh", tailscaleIP, "index", idx+1, "total", len(d.workerTailscaleIPs))

		joinCmd := fmt.Sprintf(`
set -e
%s --ignore-preflight-errors=all 2>&1
`, joinCommand)

		result, err := d.sshExecutor.RunOnNode(ctx, tailscaleIP, joinCmd)
		if err != nil {
			return fmt.Errorf("worker %s: %w", tailscaleIP, err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("worker %s: exit code %d, output: %s, stderr: %s",
				tailscaleIP, result.ExitCode, result.Stdout, result.Stderr)
		}

		slog.Info("worker joined", "node", tailscaleIP)
	}

	return nil
}

func (d *Deployer) waitForCluster(ctx context.Context, timeout time.Duration) error {
	expectedNodes := 1 + len(d.workerTailscaleIPs)
	slog.Info("waiting for nodes", "expected", expectedNodes, "timeout", timeout)

	deadline := time.Now().Add(timeout)
	retryDelay := 10 * time.Second

	for time.Now().Before(deadline) {
		checkCmd := `kubectl get nodes --no-headers 2>/dev/null | grep -c " Ready " || echo 0`

		// SSH via Tailscale IP
		result, err := d.sshExecutor.RunOnNode(ctx, d.controlPlaneTailscaleIP, checkCmd)
		if err == nil && result.ExitCode == 0 {
			count := 0
			fmt.Sscanf(strings.TrimSpace(result.Stdout), "%d", &count)

			slog.Debug("node status", "ready", count, "expected", expectedNodes)

			if count >= expectedNodes {
				slog.Info("all nodes ready", "count", count)
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryDelay):
		}
	}

	return fmt.Errorf("timed out waiting for %d nodes to be Ready", expectedNodes)
}

func (d *Deployer) verifyCluster(ctx context.Context) error {
	slog.Info("verifying cluster")

	// SSH via Tailscale IP
	nodesResult, err := d.sshExecutor.RunOnNode(ctx, d.controlPlaneTailscaleIP, "kubectl get nodes -o wide")
	if err != nil {
		return fmt.Errorf("get nodes: %w", err)
	}
	fmt.Println("\n=== Kubernetes Nodes ===")
	fmt.Println(nodesResult.Stdout)

	podsResult, err := d.sshExecutor.RunOnNode(ctx, d.controlPlaneTailscaleIP, "kubectl get pods -n kube-system -o wide")
	if err != nil {
		return fmt.Errorf("get pods: %w", err)
	}
	fmt.Println("\n=== kube-system Pods ===")
	fmt.Println(podsResult.Stdout)

	flannelResult, err := d.sshExecutor.RunOnNode(ctx, d.controlPlaneTailscaleIP,
		"kubectl get pods -n kube-flannel -o wide")
	if err != nil {
		slog.Warn("get flannel pods", "error", err)
	} else {
		fmt.Println("\n=== Flannel Pods ===")
		fmt.Println(flannelResult.Stdout)
	}

	return nil
}

func (d *Deployer) resetNode(ctx context.Context, nodeIP string) error {
	slog.Info("resetting node", "node", nodeIP)

	resetCmd := `
kubeadm reset -f || true
rm -rf /etc/cni/net.d/* || true
rm -rf /var/lib/etcd/* || true
rm -rf /var/lib/kubelet/* || true
rm -rf /root/.kube || true
# WARNING: Flushing iptables will disrupt existing network connections on this node.
iptables -F && iptables -t nat -F && iptables -t mangle -F && iptables -X || true
`

	result, err := d.sshExecutor.RunOnNode(ctx, nodeIP, resetCmd)
	if err != nil {
		return fmt.Errorf("kubeadm reset: %w", err)
	}
	if result.ExitCode != 0 {
		slog.Warn("kubeadm reset returned non-zero", "exit_code", result.ExitCode, "stderr", result.Stderr)
	}

	return nil
}
