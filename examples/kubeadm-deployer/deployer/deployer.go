package deployer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/strrl/wonder-mesh-net/pkg/wondersdk"
)

// Config holds the deployer configuration
type Config struct {
	CoordinatorURL string
	APIKey         string
	SSHUser        string
	SSHPassword    string
	SOCKS5Addr     string
	KubeVersion    string
	PodNetworkCIDR string
}

// Deployer orchestrates Kubernetes cluster bootstrap
type Deployer struct {
	config         Config
	sdkClient      *wondersdk.Client
	executor       *Executor
	controlPlaneIP string
	workerIPs      []string
	kubeconfig     string
}

// NewDeployer creates a new Deployer instance
func NewDeployer(config Config) (*Deployer, error) {
	if config.SSHUser == "" {
		config.SSHUser = "root"
	}
	if config.SSHPassword == "" {
		config.SSHPassword = "worker"
	}
	if config.SOCKS5Addr == "" {
		config.SOCKS5Addr = "localhost:1080"
	}
	if config.KubeVersion == "" {
		config.KubeVersion = "1.31"
	}
	if config.PodNetworkCIDR == "" {
		config.PodNetworkCIDR = "10.233.233.0/24"
	}

	sdkClient := wondersdk.NewClient(config.CoordinatorURL, config.APIKey)

	sshConfig := SSHConfig{
		User:       config.SSHUser,
		Password:   config.SSHPassword,
		SOCKS5Addr: config.SOCKS5Addr,
		Timeout:    30 * time.Second,
	}

	executor, err := NewExecutor(sshConfig)
	if err != nil {
		return nil, fmt.Errorf("create SSH executor: %w", err)
	}

	return &Deployer{
		config:    config,
		sdkClient: sdkClient,
		executor:  executor,
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

	if err := d.installPrerequisites(ctx); err != nil {
		return fmt.Errorf("install prerequisites: %w", err)
	}

	joinCommand, err := d.initControlPlane(ctx)
	if err != nil {
		return fmt.Errorf("init control plane: %w", err)
	}

	if err := d.installCilium(ctx); err != nil {
		return fmt.Errorf("install CNI: %w", err)
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
	fmt.Printf("Control Plane: %s\n", d.controlPlaneIP)
	fmt.Printf("Workers: %v\n", d.workerIPs)
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

// GetControlPlaneIP returns the control plane IP
func (d *Deployer) GetControlPlaneIP() string {
	return d.controlPlaneIP
}

// GetWorkerIPs returns the worker IPs
func (d *Deployer) GetWorkerIPs() []string {
	return d.workerIPs
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
	allIPs := append([]string{d.controlPlaneIP}, d.workerIPs...)
	for _, ip := range allIPs {
		if err := d.resetNode(ctx, ip); err != nil {
			slog.Warn("reset node", "node", ip, "error", err)
		}
	}
	return nil
}

func (d *Deployer) discoverNodes(ctx context.Context) ([]wondersdk.Node, error) {
	slog.Info("discovering nodes from coordinator")

	nodes, err := d.sdkClient.GetOnlineNodes(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("get online nodes: %w", err)
	}

	slog.Info("discovered nodes", "count", len(nodes))
	for _, node := range nodes {
		slog.Debug("node", "name", node.Name, "addresses", node.Addresses, "online", node.Online)
	}

	return nodes, nil
}

func (d *Deployer) selectNodes(nodes []wondersdk.Node) error {
	if len(nodes) < 1 {
		return fmt.Errorf("at least 1 node required, found %d", len(nodes))
	}

	d.controlPlaneIP = ""
	if len(nodes[0].Addresses) > 0 {
		d.controlPlaneIP = nodes[0].Addresses[0]
	}
	if d.controlPlaneIP == "" {
		return fmt.Errorf("control plane node has no IP address")
	}

	d.workerIPs = make([]string, 0, len(nodes)-1)
	for i := 1; i < len(nodes); i++ {
		if len(nodes[i].Addresses) > 0 {
			d.workerIPs = append(d.workerIPs, nodes[i].Addresses[0])
		}
	}

	slog.Info("node selection",
		"control_plane", d.controlPlaneIP,
		"workers", d.workerIPs,
	)

	return nil
}

func (d *Deployer) waitForSSH(ctx context.Context, timeout time.Duration) error {
	slog.Info("waiting for SSH connectivity")

	allIPs := append([]string{d.controlPlaneIP}, d.workerIPs...)
	return d.executor.WaitForAllNodes(ctx, allIPs, timeout)
}

func (d *Deployer) installPrerequisites(ctx context.Context) error {
	slog.Info("installing prerequisites on all nodes")

	allIPs := append([]string{d.controlPlaneIP}, d.workerIPs...)
	for idx, ip := range allIPs {
		slog.Info("installing on node", "node", ip, "index", idx+1, "total", len(allIPs))
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
		result, err := d.executor.RunOnNode(ctx, nodeIP, c.cmd)
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

	result, err := d.executor.RunOnNode(ctx, nodeIP, installCmd)
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
	slog.Info("installing kubeadm", "node", nodeIP, "version", d.config.KubeVersion)

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
`, d.config.KubeVersion, d.config.KubeVersion)

	result, err := d.executor.RunOnNode(ctx, nodeIP, installCmd)
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
	slog.Info("initializing control plane", "node", d.controlPlaneIP)

	initCmd := fmt.Sprintf(`
set -e

kubeadm init \
    --apiserver-advertise-address=%s \
    --pod-network-cidr=%s \
    --skip-phases=addon/kube-proxy \
    --ignore-preflight-errors=all \
    2>&1

mkdir -p /root/.kube
cp /etc/kubernetes/admin.conf /root/.kube/config
chown root:root /root/.kube/config

echo "=== KUBECONFIG ==="
cat /root/.kube/config
echo "=== END KUBECONFIG ==="

echo "=== JOIN COMMAND ==="
kubeadm token create --print-join-command
echo "=== END JOIN COMMAND ==="
`, d.controlPlaneIP, d.config.PodNetworkCIDR)

	result, err := d.executor.RunOnNode(ctx, d.controlPlaneIP, initCmd)
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
		"node", d.controlPlaneIP,
		"has_kubeconfig", d.kubeconfig != "",
		"has_join_command", joinCommand != "",
	)

	return joinCommand, nil
}

// installCilium installs Cilium CNI.
// KubeProxyFree is true because kubeadm init skips kube-proxy installation,
// so Cilium must handle Service/ClusterIP load-balancing.
func (d *Deployer) installCilium(ctx context.Context) error {
	slog.Info("installing Cilium CNI", "node", d.controlPlaneIP)

	installCmd := `
set -e

if command -v cilium &>/dev/null; then
    echo "Cilium CLI already installed"
    cilium version --client
else
    CILIUM_CLI_VERSION=$(curl -s https://raw.githubusercontent.com/cilium/cilium-cli/main/stable.txt)
    ARCH=$(dpkg --print-architecture 2>/dev/null || echo "amd64")

    curl -L --fail --remote-name-all \
        "https://github.com/cilium/cilium-cli/releases/download/${CILIUM_CLI_VERSION}/cilium-linux-${ARCH}.tar.gz" \
        "https://github.com/cilium/cilium-cli/releases/download/${CILIUM_CLI_VERSION}/cilium-linux-${ARCH}.tar.gz.sha256sum"

    sha256sum -c "cilium-linux-${ARCH}.tar.gz.sha256sum"

    tar xzvf cilium-linux-${ARCH}.tar.gz -C /usr/local/bin
    rm cilium-linux-${ARCH}.tar.gz cilium-linux-${ARCH}.tar.gz.sha256sum

    echo "Cilium CLI installed"
    cilium version --client
fi

cilium install \
    --set tunnel=vxlan \
    --set ipam.mode=cluster-pool \
    --set kubeProxyReplacement=true \
    --wait \
    --wait-duration=10m0s \
    2>&1

echo "Cilium installation complete"
`

	result, err := d.executor.RunOnNode(ctx, d.controlPlaneIP, installCmd)
	if err != nil {
		return fmt.Errorf("cilium install: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("cilium install: exit code %d, output: %s, stderr: %s",
			result.ExitCode, result.Stdout, result.Stderr)
	}

	slog.Info("Cilium CNI installed", "node", d.controlPlaneIP)
	return nil
}

func (d *Deployer) joinWorkers(ctx context.Context, joinCommand string) error {
	if len(d.workerIPs) == 0 {
		slog.Info("no worker nodes to join")
		return nil
	}

	slog.Info("joining worker nodes", "count", len(d.workerIPs))

	for idx, ip := range d.workerIPs {
		slog.Info("joining worker", "node", ip, "index", idx+1, "total", len(d.workerIPs))

		joinCmd := fmt.Sprintf(`
set -e
%s --ignore-preflight-errors=all 2>&1
`, joinCommand)

		result, err := d.executor.RunOnNode(ctx, ip, joinCmd)
		if err != nil {
			return fmt.Errorf("worker %s: %w", ip, err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("worker %s: exit code %d, output: %s, stderr: %s",
				ip, result.ExitCode, result.Stdout, result.Stderr)
		}

		slog.Info("worker joined", "node", ip)
	}

	return nil
}

func (d *Deployer) waitForCluster(ctx context.Context, timeout time.Duration) error {
	expectedNodes := 1 + len(d.workerIPs)
	slog.Info("waiting for nodes", "expected", expectedNodes, "timeout", timeout)

	deadline := time.Now().Add(timeout)
	retryDelay := 10 * time.Second

	for time.Now().Before(deadline) {
		checkCmd := `kubectl get nodes --no-headers 2>/dev/null | grep -c " Ready " || echo 0`

		result, err := d.executor.RunOnNode(ctx, d.controlPlaneIP, checkCmd)
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

	nodesResult, err := d.executor.RunOnNode(ctx, d.controlPlaneIP, "kubectl get nodes -o wide")
	if err != nil {
		return fmt.Errorf("get nodes: %w", err)
	}
	fmt.Println("\n=== Kubernetes Nodes ===")
	fmt.Println(nodesResult.Stdout)

	podsResult, err := d.executor.RunOnNode(ctx, d.controlPlaneIP, "kubectl get pods -n kube-system -o wide")
	if err != nil {
		return fmt.Errorf("get pods: %w", err)
	}
	fmt.Println("\n=== kube-system Pods ===")
	fmt.Println(podsResult.Stdout)

	ciliumResult, err := d.executor.RunOnNode(ctx, d.controlPlaneIP,
		"kubectl get pods -n kube-system -l 'k8s-app in (cilium,cilium-operator)' -o wide")
	if err != nil {
		slog.Warn("get cilium pods", "error", err)
	} else {
		fmt.Println("\n=== Cilium Pods ===")
		fmt.Println(ciliumResult.Stdout)
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
iptables -F && iptables -t nat -F && iptables -t mangle -F && iptables -X || true
`

	result, err := d.executor.RunOnNode(ctx, nodeIP, resetCmd)
	if err != nil {
		return fmt.Errorf("kubeadm reset: %w", err)
	}
	if result.ExitCode != 0 {
		slog.Warn("kubeadm reset returned non-zero", "exit_code", result.ExitCode, "stderr", result.Stderr)
	}

	return nil
}
