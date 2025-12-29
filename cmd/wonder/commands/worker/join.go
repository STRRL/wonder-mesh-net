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

// newJoinCmd creates the join subcommand that connects this device
// to the Wonder Mesh Net using a join token.
func newJoinCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "join <token>",
		Short: "Join the mesh network",
		Long: `Join the Wonder Mesh Net using a join token.

Get a join token from your coordinator dashboard, then run:
  wonder worker join <token>`,
		Args: cobra.ExactArgs(1),
		RunE: runJoin,
	}

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

	reqBody, _ := json.Marshal(map[string]string{"token": token})
	resp, err := http.Post(
		info.CoordinatorURL+"/coordinator/api/v1/worker/join",
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

	var result struct {
		Authkey      string `json:"authkey"`
		HeadscaleURL string `json:"headscale_url"`
		User         string `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return completeJoin(result.Authkey, result.HeadscaleURL, result.User, info.CoordinatorURL)
}

// completeJoin saves credentials locally and executes tailscale up
// to complete mesh network registration.
func completeJoin(authkey, headscaleURL, user, coordinator string) error {
	creds := &credentials{
		User:           user,
		CoordinatorURL: coordinator,
		JoinedAt:       time.Now(),
	}
	if err := saveCredentials(creds); err != nil {
		fmt.Printf("Warning: save credentials: %v\n", err)
	}

	fmt.Println()
	fmt.Println("Connecting to Wonder Mesh Net...")

	return runTailscaleUp(headscaleURL, authkey)
}

// runTailscaleUp executes the tailscale up command with the provided
// login server and auth key to connect this device to the mesh network.
func runTailscaleUp(headscaleURL, authkey string) error {
	var tailscaleCmd *exec.Cmd
	if runtime.GOOS == "windows" {
		tailscaleCmd = exec.Command("tailscale", "up", "--login-server="+headscaleURL, "--authkey="+authkey)
	} else {
		tailscaleCmd = exec.Command("sudo", "tailscale", "up", "--login-server="+headscaleURL, "--authkey="+authkey)
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
