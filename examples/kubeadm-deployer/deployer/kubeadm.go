package deployer

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
)

// KubeadmManager handles kubeadm operations
type KubeadmManager struct {
	executor *Executor
}

// NewKubeadmManager creates a new kubeadm manager
func NewKubeadmManager(executor *Executor) *KubeadmManager {
	return &KubeadmManager{executor: executor}
}

// InitConfig holds kubeadm init configuration
type InitConfig struct {
	PodNetworkCIDR     string
	ServiceCIDR        string
	ControlPlaneHost   string
	IgnorePreflightErr bool
}

// InitResult contains the output of kubeadm init
type InitResult struct {
	JoinCommand     string
	Token           string
	CACertHash      string
	KubeconfigAdmin string
}

// Init runs kubeadm init on the control plane node
func (k *KubeadmManager) Init(ctx context.Context, controlPlaneIP string, config InitConfig) (*InitResult, error) {
	slog.Info("initializing control plane", "node", controlPlaneIP)

	if config.PodNetworkCIDR == "" {
		config.PodNetworkCIDR = "10.244.0.0/16"
	}

	ignoreFlag := ""
	if config.IgnorePreflightErr {
		ignoreFlag = "--ignore-preflight-errors=all"
	}

	apiServerAddr := controlPlaneIP
	if config.ControlPlaneHost != "" {
		apiServerAddr = config.ControlPlaneHost
	}

	initCmd := fmt.Sprintf(`
set -e

kubeadm init \
    --apiserver-advertise-address=%s \
    --pod-network-cidr=%s \
    --skip-phases=addon/kube-proxy \
    %s \
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
`, apiServerAddr, config.PodNetworkCIDR, ignoreFlag)

	result, err := k.executor.RunOnNode(ctx, controlPlaneIP, initCmd)
	if err != nil {
		return nil, fmt.Errorf("kubeadm init: %w", err)
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("kubeadm init: exit code %d, stderr: %s", result.ExitCode, result.Stderr)
	}

	initResult := &InitResult{}

	kubeconfigRe := regexp.MustCompile(`=== KUBECONFIG ===\n([\s\S]*?)=== END KUBECONFIG ===`)
	if match := kubeconfigRe.FindStringSubmatch(result.Stdout); len(match) > 1 {
		initResult.KubeconfigAdmin = strings.TrimSpace(match[1])
	}

	joinCmdRe := regexp.MustCompile(`=== JOIN COMMAND ===\n([\s\S]*?)=== END JOIN COMMAND ===`)
	if match := joinCmdRe.FindStringSubmatch(result.Stdout); len(match) > 1 {
		initResult.JoinCommand = strings.TrimSpace(match[1])
	}

	if initResult.JoinCommand != "" {
		tokenRe := regexp.MustCompile(`--token\s+(\S+)`)
		if match := tokenRe.FindStringSubmatch(initResult.JoinCommand); len(match) > 1 {
			initResult.Token = match[1]
		}

		hashRe := regexp.MustCompile(`--discovery-token-ca-cert-hash\s+(\S+)`)
		if match := hashRe.FindStringSubmatch(initResult.JoinCommand); len(match) > 1 {
			initResult.CACertHash = match[1]
		}
	}

	slog.Info("control plane initialized",
		"node", controlPlaneIP,
		"token", initResult.Token,
		"has_kubeconfig", initResult.KubeconfigAdmin != "",
	)

	return initResult, nil
}

// JoinWorker joins a worker node to the cluster
func (k *KubeadmManager) JoinWorker(ctx context.Context, workerIP string, joinCommand string, ignorePreflightErr bool) error {
	slog.Info("joining worker node", "node", workerIP)

	ignoreFlag := ""
	if ignorePreflightErr {
		ignoreFlag = "--ignore-preflight-errors=all"
	}

	joinCmd := fmt.Sprintf(`
set -e

%s %s 2>&1
`, joinCommand, ignoreFlag)

	result, err := k.executor.RunOnNode(ctx, workerIP, joinCmd)
	if err != nil {
		return fmt.Errorf("kubeadm join: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("kubeadm join: exit code %d, output: %s, stderr: %s",
			result.ExitCode, result.Stdout, result.Stderr)
	}

	slog.Info("worker joined", "node", workerIP)
	return nil
}

// JoinAllWorkers joins multiple worker nodes to the cluster
func (k *KubeadmManager) JoinAllWorkers(ctx context.Context, workerIPs []string, joinCommand string, ignorePreflightErr bool) error {
	for idx, ip := range workerIPs {
		slog.Info("joining worker", "node", ip, "index", idx+1, "total", len(workerIPs))
		if err := k.JoinWorker(ctx, ip, joinCommand, ignorePreflightErr); err != nil {
			return fmt.Errorf("worker %s: %w", ip, err)
		}
	}
	return nil
}

// WaitForNodes waits until all nodes are Ready
func (k *KubeadmManager) WaitForNodes(ctx context.Context, controlPlaneIP string, expectedNodes int, timeout time.Duration) error {
	slog.Info("waiting for nodes", "expected", expectedNodes, "timeout", timeout)

	deadline := time.Now().Add(timeout)
	retryDelay := 10 * time.Second

	for time.Now().Before(deadline) {
		checkCmd := `kubectl get nodes --no-headers 2>/dev/null | grep -c " Ready " || echo 0`

		result, err := k.executor.RunOnNode(ctx, controlPlaneIP, checkCmd)
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

// GetNodes returns the current node status
func (k *KubeadmManager) GetNodes(ctx context.Context, controlPlaneIP string) (string, error) {
	result, err := k.executor.RunOnNode(ctx, controlPlaneIP, "kubectl get nodes -o wide")
	if err != nil {
		return "", err
	}
	return result.Stdout, nil
}

// GetPods returns pod status for a namespace
func (k *KubeadmManager) GetPods(ctx context.Context, controlPlaneIP string, namespace string) (string, error) {
	cmd := "kubectl get pods -A -o wide"
	if namespace != "" {
		cmd = fmt.Sprintf("kubectl get pods -n %s -o wide", namespace)
	}

	result, err := k.executor.RunOnNode(ctx, controlPlaneIP, cmd)
	if err != nil {
		return "", err
	}
	return result.Stdout, nil
}

// CopyKubeconfig copies the kubeconfig from control plane
func (k *KubeadmManager) CopyKubeconfig(ctx context.Context, controlPlaneIP string) (string, error) {
	result, err := k.executor.RunOnNode(ctx, controlPlaneIP, "cat /etc/kubernetes/admin.conf")
	if err != nil {
		return "", fmt.Errorf("read kubeconfig: %w", err)
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("read kubeconfig: exit code %d", result.ExitCode)
	}
	return result.Stdout, nil
}

// ResetNode runs kubeadm reset on a node
func (k *KubeadmManager) ResetNode(ctx context.Context, nodeIP string) error {
	slog.Info("resetting node", "node", nodeIP)

	resetCmd := `
kubeadm reset -f || true
rm -rf /etc/cni/net.d/* || true
rm -rf /var/lib/etcd/* || true
rm -rf /var/lib/kubelet/* || true
rm -rf /root/.kube || true
iptables -F && iptables -t nat -F && iptables -t mangle -F && iptables -X || true
`

	result, err := k.executor.RunOnNode(ctx, nodeIP, resetCmd)
	if err != nil {
		return fmt.Errorf("kubeadm reset: %w", err)
	}
	if result.ExitCode != 0 {
		slog.Warn("kubeadm reset returned non-zero", "exit_code", result.ExitCode, "stderr", result.Stderr)
	}

	return nil
}

// ResetAllNodes runs kubeadm reset on all nodes
func (k *KubeadmManager) ResetAllNodes(ctx context.Context, nodeIPs []string) error {
	for _, ip := range nodeIPs {
		if err := k.ResetNode(ctx, ip); err != nil {
			slog.Warn("reset node", "node", ip, "error", err)
		}
	}
	return nil
}
