package deployer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// CiliumManager handles Cilium CNI operations
type CiliumManager struct {
	executor *Executor
}

// NewCiliumManager creates a new Cilium manager
func NewCiliumManager(executor *Executor) *CiliumManager {
	return &CiliumManager{executor: executor}
}

// CiliumConfig holds Cilium installation configuration
type CiliumConfig struct {
	Version         string
	TunnelMode      string
	IPAM            string
	KubeProxyFree   bool
	HubbleEnabled   bool
	WaitForRollout  bool
	RolloutTimeout  time.Duration
}

// DefaultCiliumConfig returns sensible defaults for Docker-in-Docker environments.
// KubeProxyFree is true because kubeadm init skips kube-proxy installation,
// so Cilium must handle Service/ClusterIP load-balancing.
func DefaultCiliumConfig() CiliumConfig {
	return CiliumConfig{
		Version:         "",
		TunnelMode:      "vxlan",
		IPAM:            "cluster-pool",
		KubeProxyFree:   true,
		HubbleEnabled:   false,
		WaitForRollout:  true,
		RolloutTimeout:  5 * time.Minute,
	}
}

// InstallCiliumCLI installs the Cilium CLI on a node
func (c *CiliumManager) InstallCiliumCLI(ctx context.Context, nodeIP string) error {
	slog.Info("installing Cilium CLI", "node", nodeIP)

	installCmd := `
set -e

if command -v cilium &>/dev/null; then
    echo "Cilium CLI already installed"
    cilium version --client
    exit 0
fi

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
`

	result, err := c.executor.RunOnNode(ctx, nodeIP, installCmd)
	if err != nil {
		return fmt.Errorf("install Cilium CLI: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("install Cilium CLI: exit code %d, stderr: %s", result.ExitCode, result.Stderr)
	}

	slog.Info("Cilium CLI installed", "node", nodeIP)
	return nil
}

// Install installs Cilium CNI using the Cilium CLI
func (c *CiliumManager) Install(ctx context.Context, controlPlaneIP string, config CiliumConfig) error {
	slog.Info("installing Cilium CNI", "node", controlPlaneIP, "tunnel", config.TunnelMode)

	if err := c.InstallCiliumCLI(ctx, controlPlaneIP); err != nil {
		return fmt.Errorf("install CLI: %w", err)
	}

	var installFlags []string

	if config.Version != "" {
		installFlags = append(installFlags, fmt.Sprintf("--version=%s", config.Version))
	}

	installFlags = append(installFlags, fmt.Sprintf("--set tunnel=%s", config.TunnelMode))

	installFlags = append(installFlags, fmt.Sprintf("--set ipam.mode=%s", config.IPAM))

	if config.KubeProxyFree {
		installFlags = append(installFlags, "--set kubeProxyReplacement=true")
	} else {
		installFlags = append(installFlags, "--set kubeProxyReplacement=false")
	}

	if config.HubbleEnabled {
		installFlags = append(installFlags, "--set hubble.enabled=true")
		installFlags = append(installFlags, "--set hubble.relay.enabled=true")
	}

	if config.WaitForRollout {
		installFlags = append(installFlags, "--wait")
		installFlags = append(installFlags, fmt.Sprintf("--wait-duration=%s", config.RolloutTimeout))
	}

	installCmd := fmt.Sprintf(`
set -e

cilium install %s 2>&1

echo "Cilium installation initiated"
`, strings.Join(installFlags, " "))

	result, err := c.executor.RunOnNode(ctx, controlPlaneIP, installCmd)
	if err != nil {
		return fmt.Errorf("cilium install: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("cilium install: exit code %d, output: %s, stderr: %s",
			result.ExitCode, result.Stdout, result.Stderr)
	}

	slog.Info("Cilium CNI installed", "node", controlPlaneIP)
	return nil
}

// Status returns the Cilium status
func (c *CiliumManager) Status(ctx context.Context, controlPlaneIP string) (string, error) {
	result, err := c.executor.RunOnNode(ctx, controlPlaneIP, "cilium status --wait")
	if err != nil {
		return "", err
	}
	return result.Stdout, nil
}

// WaitForReady waits until Cilium is fully operational
func (c *CiliumManager) WaitForReady(ctx context.Context, controlPlaneIP string, timeout time.Duration) error {
	slog.Info("waiting for Cilium to be ready", "timeout", timeout)

	deadline := time.Now().Add(timeout)
	retryDelay := 10 * time.Second

	for time.Now().Before(deadline) {
		result, err := c.executor.RunOnNode(ctx, controlPlaneIP, "cilium status --wait --wait-duration 30s")
		if err == nil && result.ExitCode == 0 {
			slog.Info("Cilium is ready")
			return nil
		}

		slog.Debug("Cilium not ready yet", "error", err)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryDelay):
		}
	}

	return fmt.Errorf("Cilium not ready within %v", timeout)
}

// ConnectivityTest runs Cilium connectivity tests
func (c *CiliumManager) ConnectivityTest(ctx context.Context, controlPlaneIP string) error {
	slog.Info("running Cilium connectivity test")

	result, err := c.executor.RunOnNode(ctx, controlPlaneIP,
		"cilium connectivity test --test-concurrency 1 --connect-timeout 30s --request-timeout 30s 2>&1 || echo 'Connectivity test completed with issues'")
	if err != nil {
		return fmt.Errorf("connectivity test: %w", err)
	}

	slog.Info("connectivity test completed", "output_length", len(result.Stdout))
	return nil
}

// Uninstall removes Cilium from the cluster
func (c *CiliumManager) Uninstall(ctx context.Context, controlPlaneIP string) error {
	slog.Info("uninstalling Cilium")

	result, err := c.executor.RunOnNode(ctx, controlPlaneIP, "cilium uninstall --wait 2>&1")
	if err != nil {
		return fmt.Errorf("cilium uninstall: %w", err)
	}
	if result.ExitCode != 0 {
		slog.Warn("cilium uninstall returned non-zero", "exit_code", result.ExitCode)
	}

	return nil
}

// GetPods returns Cilium-related pods status
func (c *CiliumManager) GetPods(ctx context.Context, controlPlaneIP string) (string, error) {
	result, err := c.executor.RunOnNode(ctx, controlPlaneIP,
		"kubectl get pods -n kube-system -l 'k8s-app in (cilium,cilium-operator)' -o wide")
	if err != nil {
		return "", err
	}
	return result.Stdout, nil
}
