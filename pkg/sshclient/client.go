package sshclient

import (
	"fmt"
	"io"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

// Config holds SSH connection configuration
type Config struct {
	Host         string
	Port         int
	User         string
	Password     string
	SOCKSAddr    string
	Timeout      time.Duration
}

// Client represents an SSH client that connects through a SOCKS5 proxy
type Client struct {
	config *Config
	conn   *ssh.Client
}

// NewClient creates a new SSH client with the given configuration
func NewClient(config *Config) *Client {
	if config.Port == 0 {
		config.Port = 22
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	return &Client{config: config}
}

// Connect establishes an SSH connection through the SOCKS5 proxy
func (c *Client) Connect() error {
	// Create SOCKS5 dialer
	dialer, err := proxy.SOCKS5("tcp", c.config.SOCKSAddr, nil, proxy.Direct)
	if err != nil {
		return fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
	}

	// Dial through SOCKS5 proxy
	addr := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to dial through SOCKS5: %w", err)
	}

	// Set connection deadline
	if deadline, ok := conn.(interface{ SetDeadline(time.Time) error }); ok {
		deadline.SetDeadline(time.Now().Add(c.config.Timeout))
	}

	// SSH client configuration
	sshConfig := &ssh.ClientConfig{
		User: c.config.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(c.config.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         c.config.Timeout,
	}

	// Establish SSH connection
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, sshConfig)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to establish SSH connection: %w", err)
	}

	c.conn = ssh.NewClient(sshConn, chans, reqs)
	return nil
}

// Close closes the SSH connection
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// RunCommand executes a command on the remote host and returns the output
func (c *Client) RunCommand(cmd string) (string, error) {
	if c.conn == nil {
		return "", fmt.Errorf("not connected")
	}

	session, err := c.conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return string(output), fmt.Errorf("command failed: %w, output: %s", err, output)
	}

	return string(output), nil
}

// RunCommandWithOutput executes a command and streams output to the provided writers
func (c *Client) RunCommandWithOutput(cmd string, stdout, stderr io.Writer) error {
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	session, err := c.conn.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	session.Stdout = stdout
	session.Stderr = stderr

	if err := session.Run(cmd); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}

	return nil
}

// DialThroughSOCKS5 is a helper function to dial a connection through SOCKS5
func DialThroughSOCKS5(socksAddr, targetAddr string) (net.Conn, error) {
	dialer, err := proxy.SOCKS5("tcp", socksAddr, nil, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
	}

	conn, err := dialer.Dial("tcp", targetAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial through SOCKS5: %w", err)
	}

	return conn, nil
}
