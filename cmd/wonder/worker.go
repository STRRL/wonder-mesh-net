package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"
	"github.com/strrl/wonder-mesh-net/pkg/jointoken"
)

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Worker node commands",
	Long:  `Commands for managing this device as a worker node in Wonder Mesh Net.`,
}

var workerJoinCmd = &cobra.Command{
	Use:   "join <token>",
	Short: "Join the mesh using a join token",
	Long: `Join the Wonder Mesh Net using a join token generated from the coordinator web UI.

The token contains all necessary information to connect this device to your mesh network.`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkerJoin,
}

var workerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show worker status",
	Long:  `Show current worker status and connection information.`,
	RunE:  runWorkerStatus,
}

var workerLeaveCmd = &cobra.Command{
	Use:   "leave",
	Short: "Leave the mesh",
	Long:  `Remove locally stored credentials and leave the mesh.`,
	RunE:  runWorkerLeave,
}

func init() {
	workerCmd.AddCommand(workerJoinCmd)
	workerCmd.AddCommand(workerStatusCmd)
	workerCmd.AddCommand(workerLeaveCmd)
}

type workerCredentials struct {
	User         string    `json:"user"`
	Coordinator  string    `json:"coordinator"`
	HeadscaleURL string    `json:"headscale_url"`
	JoinedAt     time.Time `json:"joined_at"`
}

func getWorkerCredentialsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".wonder", "worker.json")
}

func loadWorkerCredentials() (*workerCredentials, error) {
	data, err := os.ReadFile(getWorkerCredentialsPath())
	if err != nil {
		return nil, err
	}

	var creds workerCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}

	return &creds, nil
}

func saveWorkerCredentials(creds *workerCredentials) error {
	dir := filepath.Dir(getWorkerCredentialsPath())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(getWorkerCredentialsPath(), data, 0600)
}

func runWorkerJoin(cmd *cobra.Command, args []string) error {
	token := args[0]

	info, err := jointoken.GetJoinInfo(token)
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}

	fmt.Printf("Joining Wonder Mesh Net...\n")
	fmt.Printf("  Coordinator: %s\n", info.CoordinatorURL)
	fmt.Printf("  User: %s\n", info.HeadscaleUser)
	fmt.Printf("  Token expires: %s\n", info.ExpiresAt.Format(time.RFC3339))
	fmt.Println()

	if time.Now().After(info.ExpiresAt) {
		return fmt.Errorf("token has expired, please generate a new one from the coordinator")
	}

	reqBody, _ := json.Marshal(map[string]string{"token": token})
	resp, err := http.Post(
		info.CoordinatorURL+"/api/v1/worker/join",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return fmt.Errorf("failed to contact coordinator: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("join failed: %s", string(body))
	}

	var result struct {
		Authkey      string `json:"authkey"`
		HeadscaleURL string `json:"headscale_url"`
		User         string `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	creds := &workerCredentials{
		User:         result.User,
		Coordinator:  info.CoordinatorURL,
		HeadscaleURL: result.HeadscaleURL,
		JoinedAt:     time.Now(),
	}
	if err := saveWorkerCredentials(creds); err != nil {
		fmt.Printf("Warning: failed to save credentials: %v\n", err)
	}

	fmt.Printf("Successfully obtained auth key!\n\n")
	fmt.Printf("To complete the setup, run on this device:\n\n")
	fmt.Printf("  sudo tailscale up --login-server=%s --authkey=%s\n\n", result.HeadscaleURL, result.Authkey)

	if askRunTailscale() {
		return runTailscaleUp(result.HeadscaleURL, result.Authkey)
	}

	return nil
}

func askRunTailscale() bool {
	fmt.Print("Would you like to run this command now? [y/N]: ")
	var answer string
	_, _ = fmt.Scanln(&answer)
	return answer == "y" || answer == "Y"
}

func runTailscaleUp(headscaleURL, authkey string) error {
	fmt.Println("\nRunning tailscale up...")

	var tsCmd *exec.Cmd
	if runtime.GOOS == "windows" {
		tsCmd = exec.Command("tailscale", "up", "--login-server="+headscaleURL, "--authkey="+authkey)
	} else {
		tsCmd = exec.Command("sudo", "tailscale", "up", "--login-server="+headscaleURL, "--authkey="+authkey)
	}

	tsCmd.Stdout = os.Stdout
	tsCmd.Stderr = os.Stderr
	tsCmd.Stdin = os.Stdin

	if err := tsCmd.Run(); err != nil {
		return fmt.Errorf("tailscale up failed: %w", err)
	}

	fmt.Println("\nSuccessfully joined the mesh!")
	return nil
}

func runWorkerStatus(cmd *cobra.Command, args []string) error {
	creds, err := loadWorkerCredentials()
	if err != nil {
		fmt.Println("Not joined to any mesh")
		fmt.Println("\nTo join, get a token from the coordinator web UI and run:")
		fmt.Println("  wonder worker join <token>")
		return nil
	}

	fmt.Printf("Worker Status\n")
	fmt.Printf("  User: %s\n", creds.User)
	fmt.Printf("  Coordinator: %s\n", creds.Coordinator)
	fmt.Printf("  Headscale: %s\n", creds.HeadscaleURL)
	fmt.Printf("  Joined: %s\n", creds.JoinedAt.Format(time.RFC3339))

	return nil
}

func runWorkerLeave(cmd *cobra.Command, args []string) error {
	path := getWorkerCredentialsPath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	fmt.Println("Left the mesh")
	fmt.Println("\nNote: To fully disconnect, you may also want to run:")
	fmt.Println("  sudo tailscale down")

	return nil
}
