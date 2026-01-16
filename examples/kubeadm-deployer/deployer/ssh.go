package deployer

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

// SSHConfig holds SSH connection configuration
type SSHConfig struct {
	User       string
	Password   string
	Timeout    time.Duration
	SOCKS5Addr string
}

// SSHClient wraps SSH functionality with SOCKS5 proxy support
type SSHClient struct {
	config     SSHConfig
	sshConfig  *ssh.ClientConfig
	dialer     proxy.Dialer
}

// NewSSHClient creates a new SSH client configured for mesh access
func NewSSHClient(config SSHConfig) (*SSHClient, error) {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.SOCKS5Addr == "" {
		config.SOCKS5Addr = "localhost:1080"
	}

	sshConfig := &ssh.ClientConfig{
		User: config.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(config.Password),
		},
		// SECURITY CONSIDERATION:
		// InsecureIgnoreHostKey disables SSH host key verification entirely.
		// This makes the connection vulnerable to man-in-the-middle (MITM)
		// attacks and MUST NOT be used in production or untrusted environments.
		//
		// This example uses InsecureIgnoreHostKey only to keep the sample
		// code simple and focused on demonstrating connectivity. When using
		// this code as a basis for real deployments, you MUST implement proper
		// host key verification (for example, by using a known_hosts file or a
		// custom ssh.HostKeyCallback) and document this choice as part of your
		// deployment's security considerations.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         config.Timeout,
	}

	dialer, err := proxy.SOCKS5("tcp", config.SOCKS5Addr, nil, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("create SOCKS5 dialer: %w", err)
	}

	return &SSHClient{
		config:    config,
		sshConfig: sshConfig,
		dialer:    dialer,
	}, nil
}

// CommandResult holds the result of a command execution
type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Connect establishes an SSH connection to the target host through SOCKS5 proxy
func (c *SSHClient) Connect(ctx context.Context, host string, port int) (*ssh.Client, error) {
	addr := fmt.Sprintf("%s:%d", host, port)

	conn, err := c.dialer.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial through SOCKS5: %w", err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, c.sshConfig)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("SSH handshake: %w", err)
	}

	return ssh.NewClient(sshConn, chans, reqs), nil
}

// RunCommand executes a command on the remote host and returns the result
func (c *SSHClient) RunCommand(ctx context.Context, host string, command string) (*CommandResult, error) {
	return c.RunCommandWithPort(ctx, host, 22, command)
}

// RunCommandWithPort executes a command on the remote host with custom port
func (c *SSHClient) RunCommandWithPort(ctx context.Context, host string, port int, command string) (*CommandResult, error) {
	client, err := c.Connect(ctx, host, port)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	defer session.Close()

	var stdout, stderr strings.Builder
	session.Stdout = &stdout
	session.Stderr = &stderr

	err = session.Run(command)
	result := &CommandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}

	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			result.ExitCode = exitErr.ExitStatus()
		} else {
			return nil, fmt.Errorf("run command: %w", err)
		}
	}

	return result, nil
}

// RunCommandWithRetry executes a command with retry logic for mesh convergence
func (c *SSHClient) RunCommandWithRetry(ctx context.Context, host string, command string, maxRetries int, retryDelay time.Duration) (*CommandResult, error) {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		result, err := c.RunCommand(ctx, host, command)
		if err == nil {
			return result, nil
		}

		lastErr = err
		slog.Debug("SSH command attempt",
			"host", host,
			"attempt", attempt,
			"max_retries", maxRetries,
			"error", err,
		)

		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryDelay):
			}
		}
	}

	return nil, fmt.Errorf("command failed after %d attempts: %w", maxRetries, lastErr)
}

// CopyFile copies content to a remote file via SSH
func (c *SSHClient) CopyFile(ctx context.Context, host string, content []byte, remotePath string, mode string) error {
	client, err := c.Connect(ctx, host, 22)
	if err != nil {
		return err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("create stdin pipe: %w", err)
	}

	cmd := fmt.Sprintf("cat > %s && chmod %s %s", remotePath, mode, remotePath)

	go func() {
		defer stdin.Close()
		_, _ = stdin.Write(content)
	}()

	return session.Run(cmd)
}

// WaitForSSH waits until SSH is available on the target host
func (c *SSHClient) WaitForSSH(ctx context.Context, host string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	retryDelay := 2 * time.Second

	for time.Now().Before(deadline) {
		client, err := c.Connect(ctx, host, 22)
		if err == nil {
			client.Close()
			return nil
		}

		slog.Debug("waiting for SSH", "host", host, "error", err)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryDelay):
		}
	}

	return fmt.Errorf("SSH not available on %s within %v", host, timeout)
}

// Executor provides high-level SSH operations for multiple nodes
type Executor struct {
	client *SSHClient
}

// NewExecutor creates a new SSH executor
func NewExecutor(sshConfig SSHConfig) (*Executor, error) {
	client, err := NewSSHClient(sshConfig)
	if err != nil {
		return nil, err
	}
	return &Executor{client: client}, nil
}

// RunOnNode executes a command on a single node
func (e *Executor) RunOnNode(ctx context.Context, nodeIP string, command string) (*CommandResult, error) {
	slog.Info("executing on node", "node", nodeIP, "command", truncateCommand(command))
	result, err := e.client.RunCommandWithRetry(ctx, nodeIP, command, 3, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("node %s: %w", nodeIP, err)
	}
	if result.ExitCode != 0 {
		slog.Warn("command returned non-zero exit code",
			"node", nodeIP,
			"exit_code", result.ExitCode,
			"stderr", result.Stderr,
		)
	}
	return result, nil
}

// RunOnAllNodes executes a command on multiple nodes sequentially
func (e *Executor) RunOnAllNodes(ctx context.Context, nodeIPs []string, command string) error {
	for _, ip := range nodeIPs {
		result, err := e.RunOnNode(ctx, ip, command)
		if err != nil {
			return err
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("command failed on node %s: exit code %d, stderr: %s",
				ip, result.ExitCode, result.Stderr)
		}
	}
	return nil
}

// WaitForAllNodes waits until SSH is available on all nodes
func (e *Executor) WaitForAllNodes(ctx context.Context, nodeIPs []string, timeout time.Duration) error {
	for _, ip := range nodeIPs {
		if err := e.client.WaitForSSH(ctx, ip, timeout); err != nil {
			return fmt.Errorf("node %s: %w", ip, err)
		}
		slog.Info("SSH available", "node", ip)
	}
	return nil
}

// GetClient returns the underlying SSH client
func (e *Executor) GetClient() *SSHClient {
	return e.client
}

func truncateCommand(cmd string) string {
	if len(cmd) > 80 {
		return cmd[:77] + "..."
	}
	return cmd
}

// CheckConnectivity verifies SSH connectivity to the host and returns roundtrip info
func (c *SSHClient) CheckConnectivity(ctx context.Context, host string) error {
	start := time.Now()
	client, err := c.Connect(ctx, host, 22)
	if err != nil {
		return err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	defer session.Close()

	err = session.Run("true")
	if err != nil {
		return fmt.Errorf("run test command: %w", err)
	}

	slog.Debug("SSH connectivity check",
		"host", host,
		"latency", time.Since(start),
	)
	return nil
}

// DialContext is a helper to create a connection with context support
func (c *SSHClient) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	type dialResult struct {
		conn net.Conn
		err  error
	}

	done := make(chan dialResult, 1)
	go func() {
		conn, err := c.dialer.Dial(network, addr)
		done <- dialResult{conn, err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-done:
		return result.conn, result.err
	}
}
