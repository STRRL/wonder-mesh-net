package worker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"
	"github.com/strrl/wonder-mesh-net/pkg/jointoken"
)

var joinFlags struct {
	coordinatorURL string
}

// newJoinCmd creates the join subcommand that connects this device
// to the Wonder Mesh Net using a join token.
func newJoinCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "join <token>",
		Short: "Join the mesh network",
		Long: `Join the Wonder Mesh Net using a join token.

Get a join token from your coordinator dashboard, then run:
  wonder worker join <token>

If the coordinator URL embedded in the token is not reachable (e.g., localhost
from inside a container), use --coordinator-url to override it.`,
		Args: cobra.ExactArgs(1),
		RunE: runJoin,
	}

	cmd.Flags().StringVar(&joinFlags.coordinatorURL, "coordinator-url", "", "Override the coordinator URL from the token")

	return cmd
}

// runJoin performs token-based join by exchanging the JWT token
// with the coordinator for mesh credentials.
func runJoin(cmd *cobra.Command, args []string) error {
	token := args[0]
	info, err := jointoken.GetJoinInfo(token)
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}

	fmt.Println("Joining Wonder Mesh Net...")

	if time.Now().After(info.ExpiresAt) {
		return fmt.Errorf("token has expired, please generate a new one")
	}

	coordinatorURL := info.CoordinatorURL
	if joinFlags.coordinatorURL != "" {
		coordinatorURL = joinFlags.coordinatorURL
	}

	reqBody, _ := json.Marshal(map[string]string{"token": token})
	resp, err := http.Post(
		coordinatorURL+"/coordinator/api/v1/worker/join",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return fmt.Errorf("contact coordinator: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("join: %s", string(body))
	}

	var result joinResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return completeJoin(&result, coordinatorURL)
}

// joinResponse represents the response from the coordinator's join endpoint.
type joinResponse struct {
	MeshType                string                   `json:"mesh_type"`
	TailscaleConnectionInfo *tailscaleConnectionInfo `json:"tailscale_connection_info,omitempty"`
}

// tailscaleConnectionInfo contains the credentials for joining a Tailscale/Headscale mesh.
type tailscaleConnectionInfo struct {
	LoginServer   string `json:"login_server"`
	Authkey       string `json:"authkey"`
	HeadscaleUser string `json:"headscale_user"`
}

// completeJoin saves credentials locally and executes the appropriate mesh client
// to complete network registration based on mesh_type.
func completeJoin(resp *joinResponse, coordinator string) error {
	meshType := resp.MeshType
	if meshType == "" {
		return fmt.Errorf("coordinator returned empty mesh_type; ensure coordinator and worker versions are compatible")
	}

	switch meshType {
	case "tailscale":
		info := resp.TailscaleConnectionInfo
		if info == nil || info.LoginServer == "" || info.Authkey == "" {
			return fmt.Errorf("missing tailscale connection info from coordinator")
		}

		creds := &credentials{
			User:           info.HeadscaleUser,
			CoordinatorURL: coordinator,
			JoinedAt:       time.Now(),
		}
		if err := saveCredentials(creds); err != nil {
			fmt.Printf("Warning: save credentials: %v\n", err)
		}

		fmt.Println()
		fmt.Println("Connecting to Wonder Mesh Net...")

		return runTailscaleUp(info.LoginServer, info.Authkey)

	default:
		return fmt.Errorf("unsupported mesh type: %s", meshType)
	}
}

// ensureTailscaledRunning starts tailscaled if it's not already running.
func ensureTailscaledRunning() error {
	socketPath := "/var/run/tailscale/tailscaled.sock"
	if _, err := os.Stat(socketPath); err == nil {
		return nil
	}

	fmt.Println("Starting tailscaled...")

	args := []string{
		"--state=/var/lib/tailscale/tailscaled.state",
		"--socket=" + socketPath,
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		cmd = exec.Command("tailscaled", args...)
	} else {
		cmd = exec.Command("sudo", append([]string{"tailscaled"}, args...)...)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start tailscaled: %w", err)
	}

	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		if _, err := os.Stat(socketPath); err == nil {
			return nil
		}
	}

	return fmt.Errorf("tailscaled socket not ready after 3 seconds")
}

// runTailscaleUp executes the tailscale up command with the provided
// login server and auth key to connect this device to the mesh network.
func runTailscaleUp(headscaleURL, authkey string) error {
	if err := ensureTailscaledRunning(); err != nil {
		return err
	}

	var tailscaleCmd *exec.Cmd
	args := []string{"up", "--login-server=" + headscaleURL, "--authkey=" + authkey}

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		tailscaleCmd = exec.Command("tailscale", args...)
	} else {
		tailscaleCmd = exec.Command("sudo", append([]string{"tailscale"}, args...)...)
	}

	tailscaleCmd.Stdout = os.Stdout
	tailscaleCmd.Stderr = os.Stderr
	tailscaleCmd.Stdin = os.Stdin

	if err := tailscaleCmd.Run(); err != nil {
		return fmt.Errorf("connect to mesh: %w", err)
	}

	fmt.Println()
	fmt.Println("Successfully joined Wonder Mesh Net!")
	return nil
}
