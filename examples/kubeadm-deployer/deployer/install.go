package deployer

import (
	"context"
	"fmt"
	"log/slog"
)

// Installer handles package installation on remote nodes
type Installer struct {
	executor *Executor
}

// NewInstaller creates a new installer
func NewInstaller(executor *Executor) *Installer {
	return &Installer{executor: executor}
}

// ConfigurePrerequisites sets up kernel modules and sysctl for Kubernetes
func (i *Installer) ConfigurePrerequisites(ctx context.Context, nodeIP string) error {
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
		result, err := i.executor.RunOnNode(ctx, nodeIP, c.cmd)
		if err != nil {
			return fmt.Errorf("%s: %w", c.name, err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("%s: exit code %d, stderr: %s", c.name, result.ExitCode, result.Stderr)
		}
	}

	return nil
}

// InstallContainerd installs containerd runtime.
// NOTE: This downloads packages from Docker's official repository with GPG verification.
// For production, ensure you're connecting to legitimate sources and consider
// additional verification or using pre-built images.
func (i *Installer) InstallContainerd(ctx context.Context, nodeIP string) error {
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

	result, err := i.executor.RunOnNode(ctx, nodeIP, installCmd)
	if err != nil {
		return fmt.Errorf("install containerd: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("install containerd: exit code %d, stderr: %s", result.ExitCode, result.Stderr)
	}

	slog.Info("containerd installed", "node", nodeIP)
	return nil
}

// InstallKubeadm installs kubeadm, kubelet, and kubectl
func (i *Installer) InstallKubeadm(ctx context.Context, nodeIP string, kubeVersion string) error {
	slog.Info("installing kubeadm", "node", nodeIP, "version", kubeVersion)

	if kubeVersion == "" {
		kubeVersion = "1.31"
	}

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

	result, err := i.executor.RunOnNode(ctx, nodeIP, installCmd)
	if err != nil {
		return fmt.Errorf("install kubeadm: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("install kubeadm: exit code %d, stderr: %s", result.ExitCode, result.Stderr)
	}

	slog.Info("kubeadm installed", "node", nodeIP)
	return nil
}

// InstallAll installs all prerequisites, containerd, and kubeadm on a node
func (i *Installer) InstallAll(ctx context.Context, nodeIP string, kubeVersion string) error {
	if err := i.ConfigurePrerequisites(ctx, nodeIP); err != nil {
		return fmt.Errorf("configure prerequisites: %w", err)
	}

	if err := i.InstallContainerd(ctx, nodeIP); err != nil {
		return fmt.Errorf("install containerd: %w", err)
	}

	if err := i.InstallKubeadm(ctx, nodeIP, kubeVersion); err != nil {
		return fmt.Errorf("install kubeadm: %w", err)
	}

	return nil
}

// InstallAllNodes installs prerequisites on all nodes
func (i *Installer) InstallAllNodes(ctx context.Context, nodeIPs []string, kubeVersion string) error {
	for idx, ip := range nodeIPs {
		slog.Info("installing on node", "node", ip, "index", idx+1, "total", len(nodeIPs))
		if err := i.InstallAll(ctx, ip, kubeVersion); err != nil {
			return fmt.Errorf("node %s: %w", ip, err)
		}
	}
	return nil
}

// VerifyInstallation checks if the installation was successful
func (i *Installer) VerifyInstallation(ctx context.Context, nodeIP string) error {
	slog.Info("verifying installation", "node", nodeIP)

	checks := []struct {
		name string
		cmd  string
	}{
		{
			name: "containerd running",
			cmd:  "systemctl is-active containerd",
		},
		{
			name: "kubelet installed",
			cmd:  "kubeadm version",
		},
		{
			name: "crictl working",
			cmd:  "crictl --runtime-endpoint unix:///run/containerd/containerd.sock info",
		},
	}

	for _, check := range checks {
		result, err := i.executor.RunOnNode(ctx, nodeIP, check.cmd)
		if err != nil {
			return fmt.Errorf("verify %s: %w", check.name, err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("verify %s: exit code %d, stderr: %s", check.name, result.ExitCode, result.Stderr)
		}
		slog.Debug("verification passed", "check", check.name, "node", nodeIP)
	}

	return nil
}
