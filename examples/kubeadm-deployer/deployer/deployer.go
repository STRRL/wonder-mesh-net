package deployer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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
	installer      *Installer
	kubeadmMgr     *KubeadmManager
	ciliumMgr      *CiliumManager
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
		config.PodNetworkCIDR = "10.244.0.0/16"
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
		config:     config,
		sdkClient:  sdkClient,
		executor:   executor,
		installer:  NewInstaller(executor),
		kubeadmMgr: NewKubeadmManager(executor),
		ciliumMgr:  NewCiliumManager(executor),
	}, nil
}

// DiscoverNodes discovers online nodes from the coordinator
func (d *Deployer) DiscoverNodes(ctx context.Context) ([]wondersdk.Node, error) {
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

// SelectNodes selects control plane and worker nodes from discovered nodes
func (d *Deployer) SelectNodes(nodes []wondersdk.Node) error {
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

// WaitForSSH waits until all nodes are reachable via SSH
func (d *Deployer) WaitForSSH(ctx context.Context, timeout time.Duration) error {
	slog.Info("waiting for SSH connectivity")

	allIPs := append([]string{d.controlPlaneIP}, d.workerIPs...)
	return d.executor.WaitForAllNodes(ctx, allIPs, timeout)
}

// InstallPrerequisites installs containerd and kubeadm on all nodes
func (d *Deployer) InstallPrerequisites(ctx context.Context) error {
	slog.Info("installing prerequisites on all nodes")

	allIPs := append([]string{d.controlPlaneIP}, d.workerIPs...)
	return d.installer.InstallAllNodes(ctx, allIPs, d.config.KubeVersion)
}

// InitControlPlane initializes the Kubernetes control plane
func (d *Deployer) InitControlPlane(ctx context.Context) (*InitResult, error) {
	slog.Info("initializing control plane")

	initConfig := InitConfig{
		PodNetworkCIDR:     d.config.PodNetworkCIDR,
		ControlPlaneHost:   d.controlPlaneIP,
		IgnorePreflightErr: true,
	}

	result, err := d.kubeadmMgr.Init(ctx, d.controlPlaneIP, initConfig)
	if err != nil {
		return nil, err
	}

	d.kubeconfig = result.KubeconfigAdmin
	return result, nil
}

// InstallCNI installs Cilium CNI
func (d *Deployer) InstallCNI(ctx context.Context) error {
	slog.Info("installing Cilium CNI")

	config := DefaultCiliumConfig()
	config.WaitForRollout = true
	config.RolloutTimeout = 10 * time.Minute

	return d.ciliumMgr.Install(ctx, d.controlPlaneIP, config)
}

// JoinWorkers joins all worker nodes to the cluster
func (d *Deployer) JoinWorkers(ctx context.Context, joinCommand string) error {
	if len(d.workerIPs) == 0 {
		slog.Info("no worker nodes to join")
		return nil
	}

	slog.Info("joining worker nodes", "count", len(d.workerIPs))
	return d.kubeadmMgr.JoinAllWorkers(ctx, d.workerIPs, joinCommand, true)
}

// WaitForCluster waits until all nodes are Ready
func (d *Deployer) WaitForCluster(ctx context.Context, timeout time.Duration) error {
	expectedNodes := 1 + len(d.workerIPs)
	return d.kubeadmMgr.WaitForNodes(ctx, d.controlPlaneIP, expectedNodes, timeout)
}

// VerifyCluster verifies the cluster is working
func (d *Deployer) VerifyCluster(ctx context.Context) error {
	slog.Info("verifying cluster")

	nodes, err := d.kubeadmMgr.GetNodes(ctx, d.controlPlaneIP)
	if err != nil {
		return fmt.Errorf("get nodes: %w", err)
	}
	fmt.Println("\n=== Kubernetes Nodes ===")
	fmt.Println(nodes)

	pods, err := d.kubeadmMgr.GetPods(ctx, d.controlPlaneIP, "kube-system")
	if err != nil {
		return fmt.Errorf("get pods: %w", err)
	}
	fmt.Println("\n=== kube-system Pods ===")
	fmt.Println(pods)

	ciliumPods, err := d.ciliumMgr.GetPods(ctx, d.controlPlaneIP)
	if err != nil {
		slog.Warn("get cilium pods", "error", err)
	} else {
		fmt.Println("\n=== Cilium Pods ===")
		fmt.Println(ciliumPods)
	}

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

// Run executes the full deployment flow
func (d *Deployer) Run(ctx context.Context) error {
	slog.Info("starting Kubernetes cluster deployment")

	if err := d.sdkClient.Health(ctx); err != nil {
		return fmt.Errorf("coordinator health check: %w", err)
	}
	slog.Info("coordinator is healthy")

	nodes, err := d.DiscoverNodes(ctx)
	if err != nil {
		return err
	}

	if err := d.SelectNodes(nodes); err != nil {
		return err
	}

	if err := d.WaitForSSH(ctx, 2*time.Minute); err != nil {
		return fmt.Errorf("wait for SSH: %w", err)
	}

	if err := d.InstallPrerequisites(ctx); err != nil {
		return fmt.Errorf("install prerequisites: %w", err)
	}

	initResult, err := d.InitControlPlane(ctx)
	if err != nil {
		return fmt.Errorf("init control plane: %w", err)
	}

	if err := d.InstallCNI(ctx); err != nil {
		return fmt.Errorf("install CNI: %w", err)
	}

	if err := d.JoinWorkers(ctx, initResult.JoinCommand); err != nil {
		return fmt.Errorf("join workers: %w", err)
	}

	if err := d.WaitForCluster(ctx, 5*time.Minute); err != nil {
		return fmt.Errorf("wait for cluster: %w", err)
	}

	if err := d.VerifyCluster(ctx); err != nil {
		slog.Warn("cluster verification", "error", err)
	}

	fmt.Println("\n=== Deployment Complete ===")
	fmt.Printf("Control Plane: %s\n", d.controlPlaneIP)
	fmt.Printf("Workers: %v\n", d.workerIPs)
	fmt.Println("\nTo access the cluster from the deployer:")
	fmt.Printf("  kubectl --kubeconfig /tmp/kubeconfig get nodes\n")

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

// Reset resets all nodes in the cluster
func (d *Deployer) Reset(ctx context.Context) error {
	allIPs := append([]string{d.controlPlaneIP}, d.workerIPs...)
	return d.kubeadmMgr.ResetAllNodes(ctx, allIPs)
}
