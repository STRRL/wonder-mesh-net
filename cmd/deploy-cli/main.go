package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/strrl/wonder-mesh-net/pkg/sshclient"
)

const (
	defaultSOCKSAddr = "localhost:1080"
	defaultSSHUser   = "vagrant"
	defaultSSHPass   = "vagrant"
	defaultSSHPort   = 22
)

func main() {
	var (
		socksAddr = flag.String("socks", defaultSOCKSAddr, "SOCKS5 proxy address")
		sshUser   = flag.String("user", defaultSSHUser, "SSH username")
		sshPass   = flag.String("pass", defaultSSHPass, "SSH password")
		sshPort   = flag.Int("port", defaultSSHPort, "SSH port")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <command> <worker1> [worker2] ...\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  install-cockpit    Install Cockpit on worker nodes\n")
		fmt.Fprintf(os.Stderr, "  exec <cmd>         Execute a command on worker nodes\n")
		fmt.Fprintf(os.Stderr, "  status             Check connectivity to worker nodes\n")
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s install-cockpit 100.64.0.3 100.64.0.4\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s exec \"uname -a\" worker-node-1 worker-node-2\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s status 100.64.0.3\n", os.Args[0])
	}

	flag.Parse()

	args := flag.Args()
	if len(args) < 2 {
		flag.Usage()
		os.Exit(1)
	}

	command := args[0]
	targets := args[1:]

	config := &deployConfig{
		SOCKSAddr: *socksAddr,
		SSHUser:   *sshUser,
		SSHPass:   *sshPass,
		SSHPort:   *sshPort,
	}

	switch command {
	case "install-cockpit":
		runInstallCockpit(config, targets)
	case "exec":
		if len(args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: exec requires a command and at least one target\n")
			flag.Usage()
			os.Exit(1)
		}
		execCmd := args[1]
		targets = args[2:]
		runExec(config, execCmd, targets)
	case "status":
		runStatus(config, targets)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		flag.Usage()
		os.Exit(1)
	}
}

type deployConfig struct {
	SOCKSAddr string
	SSHUser   string
	SSHPass   string
	SSHPort   int
}

type result struct {
	Target  string
	Success bool
	Output  string
	Error   error
}

func runInstallCockpit(config *deployConfig, targets []string) {
	fmt.Println("=== Installing Cockpit on worker nodes ===")
	fmt.Printf("Targets: %s\n", strings.Join(targets, ", "))
	fmt.Printf("SOCKS proxy: %s\n\n", config.SOCKSAddr)

	installCmd := "sudo apt-get update && sudo DEBIAN_FRONTEND=noninteractive apt-get install -y cockpit && sudo systemctl enable --now cockpit.socket"

	results := executeOnTargets(config, targets, installCmd)

	fmt.Println("\n=== Results ===")
	allSuccess := true
	for _, r := range results {
		if r.Success {
			fmt.Printf("[OK] %s: Cockpit installed successfully\n", r.Target)
		} else {
			fmt.Printf("[FAIL] %s: %v\n", r.Target, r.Error)
			allSuccess = false
		}
	}

	if allSuccess {
		fmt.Println("\nCockpit is now available on:")
		for _, target := range targets {
			fmt.Printf("  http://%s:9090\n", target)
		}
	} else {
		os.Exit(1)
	}
}

func runExec(config *deployConfig, cmd string, targets []string) {
	fmt.Printf("=== Executing command on %d targets ===\n", len(targets))
	fmt.Printf("Command: %s\n", cmd)
	fmt.Printf("SOCKS proxy: %s\n\n", config.SOCKSAddr)

	results := executeOnTargets(config, targets, cmd)

	for _, r := range results {
		fmt.Printf("--- %s ---\n", r.Target)
		if r.Success {
			fmt.Println(r.Output)
		} else {
			fmt.Printf("Error: %v\n", r.Error)
			if r.Output != "" {
				fmt.Println(r.Output)
			}
		}
		fmt.Println()
	}
}

func runStatus(config *deployConfig, targets []string) {
	fmt.Println("=== Checking connectivity ===")
	fmt.Printf("SOCKS proxy: %s\n\n", config.SOCKSAddr)

	results := executeOnTargets(config, targets, "hostname && tailscale ip -4")

	allSuccess := true
	for _, r := range results {
		if r.Success {
			lines := strings.Split(strings.TrimSpace(r.Output), "\n")
			hostname := ""
			tailscaleIP := ""
			if len(lines) >= 1 {
				hostname = lines[0]
			}
			if len(lines) >= 2 {
				tailscaleIP = lines[1]
			}
			fmt.Printf("[OK] %s -> hostname: %s, tailscale IP: %s\n", r.Target, hostname, tailscaleIP)
		} else {
			fmt.Printf("[FAIL] %s: %v\n", r.Target, r.Error)
			allSuccess = false
		}
	}

	if !allSuccess {
		os.Exit(1)
	}
}

func executeOnTargets(config *deployConfig, targets []string, cmd string) []result {
	var wg sync.WaitGroup
	results := make([]result, len(targets))

	for i, target := range targets {
		wg.Add(1)
		go func(idx int, host string) {
			defer wg.Done()

			r := result{Target: host}

			client := sshclient.NewClient(&sshclient.Config{
				Host:      host,
				Port:      config.SSHPort,
				User:      config.SSHUser,
				Password:  config.SSHPass,
				SOCKSAddr: config.SOCKSAddr,
			})

			fmt.Printf("[%s] Connecting...\n", host)

			if err := client.Connect(); err != nil {
				r.Error = fmt.Errorf("connection failed: %w", err)
				results[idx] = r
				return
			}
			defer client.Close()

			fmt.Printf("[%s] Running command...\n", host)

			output, err := client.RunCommand(cmd)
			r.Output = output
			if err != nil {
				r.Error = err
			} else {
				r.Success = true
			}

			results[idx] = r
		}(i, target)
	}

	wg.Wait()
	return results
}
