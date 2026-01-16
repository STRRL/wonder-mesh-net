package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/strrl/wonder-mesh-net/examples/kubeadm-deployer/deployer"
)

var (
	coordinatorURL string
	apiKey         string
	verbose        bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "kubeadm-deployer",
		Short: "Bootstrap a Kubernetes cluster using kubeadm across Wonder Mesh Net nodes",
		Long: `kubeadm-deployer discovers nodes connected to Wonder Mesh Net and
bootstraps a Kubernetes cluster using kubeadm. It demonstrates how a
deployer/workload manager can use the Wonder Mesh Net SDK to orchestrate
distributed compute.

The deployer will:
1. Discover online nodes via Wonder Mesh Net API
2. Install containerd and kubeadm on all nodes
3. Initialize the control plane on the first node
4. Install Flannel CNI (v0.26.7)
5. Join remaining nodes as workers

Prerequisites:
- Wonder Mesh Net coordinator running with workers joined
- API key created for the deployer
- Tailscale SOCKS5 proxy running (userspace networking)`,
		RunE: runDeploy,
	}

	rootCmd.Flags().StringVar(&coordinatorURL, "coordinator-url", "", "Wonder Mesh Net coordinator URL (required)")
	rootCmd.Flags().StringVar(&apiKey, "api-key", "", "API key for authentication (required)")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")

	rootCmd.MarkFlagRequired("coordinator-url")
	rootCmd.MarkFlagRequired("api-key")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runDeploy(cmd *cobra.Command, args []string) error {
	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nReceived interrupt, canceling...")
		cancel()
	}()

	d, err := deployer.NewDeployer(deployer.Config{
		CoordinatorURL: coordinatorURL,
		APIKey:         apiKey,
	})
	if err != nil {
		return fmt.Errorf("create deployer: %w", err)
	}

	if err := d.Run(ctx); err != nil {
		return fmt.Errorf("deployment: %w", err)
	}

	if err := d.SaveKubeconfig("/tmp/kubeconfig"); err != nil {
		slog.Warn("save kubeconfig", "error", err)
	}

	return nil
}
