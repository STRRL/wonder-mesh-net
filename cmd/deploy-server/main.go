package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

const (
	defaultListenAddr = ":8082"
	defaultSOCKSAddr  = "localhost:1080"
	defaultSSHPort    = 22
	defaultTimeout    = 60 * time.Second
)

type Config struct {
	ListenAddr string
	SOCKSAddr  string
	SSHPort    int
	Timeout    time.Duration
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

var config Config

type dialerFunc func(network, addr string) (net.Conn, error)

func (d dialerFunc) Dial(network, addr string) (net.Conn, error) {
	return d(network, addr)
}

func main() {
	flag.StringVar(&config.ListenAddr, "listen", defaultListenAddr, "HTTP server listen address")
	flag.StringVar(&config.SOCKSAddr, "socks", defaultSOCKSAddr, "SOCKS5 proxy address")
	flag.IntVar(&config.SSHPort, "ssh-port", defaultSSHPort, "Default SSH port")
	flag.DurationVar(&config.Timeout, "timeout", defaultTimeout, "SSH command timeout")
	flag.Parse()

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/exec", execHandler)

	log.Printf("Deploy Server starting on %s", config.ListenAddr)
	log.Printf("SOCKS5 proxy: %s", config.SOCKSAddr)
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

	var dialer proxy.Dialer
	var err error

	if strings.HasPrefix(config.SOCKSAddr, "/") {
		unixDialer := &net.Dialer{}
		baseDialer := proxy.FromEnvironmentUsing(
			dialerFunc(func(network, addr string) (net.Conn, error) {
				return unixDialer.Dial("unix", config.SOCKSAddr)
			}),
		)
		dialer, err = proxy.SOCKS5("tcp", "", nil, baseDialer)
	} else {
		dialer, err = proxy.SOCKS5("tcp", config.SOCKSAddr, nil, proxy.Direct)
	}
	if err != nil {
		resp.Error = fmt.Sprintf("Failed to create SOCKS5 dialer: %v", err)
		return resp
	}

	addr := fmt.Sprintf("%s:%d", req.Host, config.SSHPort)
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		resp.Error = fmt.Sprintf("Failed to dial through SOCKS5: %v", err)
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
