package headscale

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ProcessManager manages the Headscale subprocess
type ProcessManager struct {
	binaryPath string
	configPath string
	dataDir    string

	cmd     *exec.Cmd
	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
}

// ProcessConfig contains configuration for the Headscale process
type ProcessConfig struct {
	BinaryPath string
	ConfigPath string
	DataDir    string
}

// NewProcessManager creates a new Headscale process manager
func NewProcessManager(cfg ProcessConfig) *ProcessManager {
	binaryPath := cfg.BinaryPath
	if binaryPath == "" {
		binaryPath = "headscale"
	}

	return &ProcessManager{
		binaryPath: binaryPath,
		configPath: cfg.ConfigPath,
		dataDir:    cfg.DataDir,
		stopCh:     make(chan struct{}),
	}
}

// Start starts the Headscale process
func (pm *ProcessManager) Start(ctx context.Context) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.running {
		return fmt.Errorf("headscale is already running")
	}

	args := []string{"serve"}
	if pm.configPath != "" {
		args = append(args, "--config", pm.configPath)
	}

	pm.cmd = exec.Command(pm.binaryPath, args...)
	pm.cmd.Env = os.Environ()

	stdout, err := pm.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := pm.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := pm.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start headscale: %w", err)
	}

	pm.running = true

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fmt.Printf("[headscale] %s\n", scanner.Text())
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			fmt.Printf("[headscale] %s\n", scanner.Text())
		}
	}()

	go func() {
		err := pm.cmd.Wait()
		pm.mu.Lock()
		pm.running = false
		pm.mu.Unlock()
		if err != nil {
			fmt.Printf("[headscale] process exited with error: %v\n", err)
		} else {
			fmt.Printf("[headscale] process exited normally\n")
		}
	}()

	return nil
}

// Stop stops the Headscale process
func (pm *ProcessManager) Stop() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if !pm.running || pm.cmd == nil || pm.cmd.Process == nil {
		return nil
	}

	if err := pm.cmd.Process.Signal(os.Interrupt); err != nil {
		return pm.cmd.Process.Kill()
	}

	done := make(chan struct{})
	go func() {
		_ = pm.cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(10 * time.Second):
		return pm.cmd.Process.Kill()
	}
}

// IsRunning returns true if Headscale is running
func (pm *ProcessManager) IsRunning() bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.running
}

// WaitReady waits for Headscale to be ready
func (pm *ProcessManager) WaitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		cmd := exec.CommandContext(ctx, pm.binaryPath, "users", "list")
		if pm.configPath != "" {
			cmd.Args = append(cmd.Args, "--config", pm.configPath)
		}

		if err := cmd.Run(); err == nil {
			return nil
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("headscale not ready after %v", timeout)
}

// CreateAPIKey creates a new API key for gRPC authentication
func (pm *ProcessManager) CreateAPIKey(ctx context.Context) (string, error) {
	args := []string{"apikeys", "create", "--expiration", "87600h"}
	if pm.configPath != "" {
		args = append(args, "--config", pm.configPath)
	}

	cmd := exec.CommandContext(ctx, pm.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create API key: %w, output: %s", err, output)
	}

	apiKey := parseAPIKeyOutput(string(output))
	if apiKey == "" {
		return "", fmt.Errorf("failed to parse API key from output: %s", output)
	}

	return apiKey, nil
}

// parseAPIKeyOutput extracts the API key from headscale output.
// The output contains log lines (starting with timestamps or "time=")
// and the API key on the last non-empty line.
func parseAPIKeyOutput(output string) string {
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "time=") || strings.HasPrefix(line, "20") {
			continue
		}
		return line
	}
	return ""
}
