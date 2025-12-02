package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	defaultListenAddr      = ":8082"
	defaultTailscaleSocket = "/var/run/tailscale-userspace/tailscaled.sock"
	defaultSSHPort         = 22
	defaultTimeout         = 60 * time.Second
)

type Config struct {
	ListenAddr      string
	TailscaleSocket string
	SSHPort         int
	Timeout         time.Duration
}

type ExecRequest struct {
	Host     string `json:"host"`
	User     string `json:"user"`
	Password string `json:"password"`
	Command  string `json:"command"`
}

type ExecResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

type Node struct {
	Hostname  string   `json:"hostname"`
	IPs       []string `json:"ips"`
	Online    bool     `json:"online"`
	ExitNode  bool     `json:"exit_node"`
	OS        string   `json:"os,omitempty"`
	TailnetIP string   `json:"tailnet_ip,omitempty"`
}

type NodesResponse struct {
	Nodes []Node `json:"nodes"`
	Self  *Node  `json:"self,omitempty"`
	Error string `json:"error,omitempty"`
}

type TailscaleStatus struct {
	Self  TailscalePeer            `json:"Self"`
	Peer  map[string]TailscalePeer `json:"Peer"`
	Peers []TailscalePeer          `json:"-"`
}

type TailscalePeer struct {
	ID             string   `json:"ID"`
	PublicKey      string   `json:"PublicKey"`
	HostName       string   `json:"HostName"`
	DNSName        string   `json:"DNSName"`
	OS             string   `json:"OS"`
	TailscaleIPs   []string `json:"TailscaleIPs"`
	Online         bool     `json:"Online"`
	ExitNode       bool     `json:"ExitNode"`
	ExitNodeOption bool     `json:"ExitNodeOption"`
}

var config Config

func main() {
	flag.StringVar(&config.ListenAddr, "listen", defaultListenAddr, "HTTP server listen address")
	flag.StringVar(&config.TailscaleSocket, "tailscale-socket", defaultTailscaleSocket, "Tailscale socket path")
	flag.IntVar(&config.SSHPort, "ssh-port", defaultSSHPort, "Default SSH port")
	flag.DurationVar(&config.Timeout, "timeout", defaultTimeout, "SSH command timeout")
	flag.Parse()

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/nodes", nodesHandler)
	http.HandleFunc("/exec", execHandler)

	log.Printf("Deploy Server starting on %s", config.ListenAddr)
	log.Printf("Tailscale socket: %s", config.TailscaleSocket)
	log.Printf("Default SSH port: %d", config.SSHPort)

	if err := http.ListenAndServe(config.ListenAddr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Server failed: %v\n", err)
		os.Exit(1)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func nodesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	status, err := getTailscaleStatus()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(NodesResponse{
			Error: fmt.Sprintf("Failed to get tailscale status: %v", err),
		})
		return
	}

	resp := NodesResponse{
		Nodes: make([]Node, 0, len(status.Peer)),
	}

	selfNode := peerToNode(status.Self)
	resp.Self = &selfNode

	for _, peer := range status.Peer {
		node := peerToNode(peer)
		resp.Nodes = append(resp.Nodes, node)
	}

	json.NewEncoder(w).Encode(resp)
}

func peerToNode(peer TailscalePeer) Node {
	node := Node{
		Hostname: peer.HostName,
		IPs:      peer.TailscaleIPs,
		Online:   peer.Online,
		ExitNode: peer.ExitNode,
		OS:       peer.OS,
	}
	if len(peer.TailscaleIPs) > 0 {
		node.TailnetIP = peer.TailscaleIPs[0]
	}
	return node
}

func getTailscaleStatus() (*TailscaleStatus, error) {
	cmd := exec.Command("tailscale", "--socket", config.TailscaleSocket, "status", "--json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tailscale status failed: %w", err)
	}

	var status TailscaleStatus
	if err := json.Unmarshal(output, &status); err != nil {
		return nil, fmt.Errorf("failed to parse tailscale status: %w", err)
	}

	return &status, nil
}

func dialViaTailscale(addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	cmd := exec.Command("tailscale", "--socket", config.TailscaleSocket, "nc", host, port)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("failed to start tailscale nc: %w", err)
	}

	return &pipeConn{
		cmd:    cmd,
		reader: stdout,
		writer: stdin,
	}, nil
}

type pipeConn struct {
	cmd    *exec.Cmd
	reader io.Reader
	writer io.WriteCloser
}

func (c *pipeConn) Read(b []byte) (int, error) {
	return c.reader.Read(b)
}

func (c *pipeConn) Write(b []byte) (int, error) {
	return c.writer.Write(b)
}

func (c *pipeConn) Close() error {
	c.writer.Close()
	return c.cmd.Wait()
}

func (c *pipeConn) LocalAddr() net.Addr {
	return &net.TCPAddr{}
}

func (c *pipeConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{}
}

func (c *pipeConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *pipeConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *pipeConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func execHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	if req.Host == "" {
		sendError(w, http.StatusBadRequest, "host is required")
		return
	}
	if req.User == "" {
		sendError(w, http.StatusBadRequest, "user is required")
		return
	}
	if req.Password == "" {
		sendError(w, http.StatusBadRequest, "password is required")
		return
	}
	if req.Command == "" {
		sendError(w, http.StatusBadRequest, "command is required")
		return
	}

	log.Printf("Executing command on %s@%s: %s", req.User, req.Host, req.Command)

	resp := executeSSHCommand(req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func sendError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ExecResponse{
		ExitCode: -1,
		Error:    message,
	})
}

func executeSSHCommand(req ExecRequest) ExecResponse {
	resp := ExecResponse{ExitCode: -1}

	addr := fmt.Sprintf("%s:%d", req.Host, config.SSHPort)
	conn, err := dialViaTailscale(addr)
	if err != nil {
		resp.Error = fmt.Sprintf("Failed to dial through tailscale: %v", err)
		return resp
	}
	defer conn.Close()

	sshConfig := &ssh.ClientConfig{
		User: req.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(req.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         config.Timeout,
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, sshConfig)
	if err != nil {
		resp.Error = fmt.Sprintf("Failed to establish SSH connection: %v", err)
		return resp
	}

	client := ssh.NewClient(sshConn, chans, reqs)
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		resp.Error = fmt.Sprintf("Failed to create session: %v", err)
		return resp
	}
	defer session.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	err = session.Run(req.Command)
	resp.Stdout = stdoutBuf.String()
	resp.Stderr = stderrBuf.String()

	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			resp.ExitCode = exitErr.ExitStatus()
		} else {
			resp.Error = fmt.Sprintf("Command execution failed: %v", err)
		}
	} else {
		resp.ExitCode = 0
	}

	return resp
}
