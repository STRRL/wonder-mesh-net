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

var coordinatorURL string

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Worker node commands",
	Long:  `Commands for managing this device as a worker node in Wonder Mesh Net.`,
}

var workerJoinCmd = &cobra.Command{
	Use:   "join [token]",
	Short: "Join the mesh network",
	Long: `Join the Wonder Mesh Net.

Without arguments, starts device authorization flow:
  wonder worker join --coordinator https://your-coordinator.example.com

With a token, uses token-based join (legacy):
  wonder worker join <token>`,
	Args: cobra.MaximumNArgs(1),
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
	workerJoinCmd.Flags().StringVar(&coordinatorURL, "coordinator", "", "Coordinator URL (required for device flow)")
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
	if len(args) == 1 {
		return runTokenJoin(args[0])
	}
	return runDeviceFlowJoin()
}

func runTokenJoin(token string) error {
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
		info.CoordinatorURL+"/coordinator/api/v1/worker/join",
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

	return completeJoin(result.Authkey, result.HeadscaleURL, result.User, info.CoordinatorURL)
}

func runDeviceFlowJoin() error {
	if coordinatorURL == "" {
		creds, err := loadWorkerCredentials()
		if err == nil && creds.Coordinator != "" {
			coordinatorURL = creds.Coordinator
		} else {
			return fmt.Errorf("--coordinator flag is required for device flow")
		}
	}

	fmt.Println("Starting device authorization...")
	fmt.Println()

	deviceCode, userCode, verifyURL, interval, err := requestDeviceCode(coordinatorURL)
	if err != nil {
		return fmt.Errorf("failed to start device authorization: %w", err)
	}

	fmt.Println("To authorize this device, visit:")
	fmt.Println()
	fmt.Printf("  %s?code=%s\n", verifyURL, userCode)
	fmt.Println()
	fmt.Printf("And enter the code: %s\n", userCode)
	fmt.Println()
	fmt.Println("Waiting for authorization...")

	authkey, headscaleURL, user, err := pollForToken(coordinatorURL, deviceCode, interval)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Device authorized!")

	return completeJoin(authkey, headscaleURL, user, coordinatorURL)
}

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

func requestDeviceCode(coordinator string) (deviceCode, userCode, verifyURL string, interval int, err error) {
	resp, err := http.Post(
		coordinator+"/coordinator/device/code",
		"application/json",
		bytes.NewReader([]byte("{}")),
	)
	if err != nil {
		return "", "", "", 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", "", 0, fmt.Errorf("failed to get device code: %s", string(body))
	}

	var result deviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", "", 0, err
	}

	return result.DeviceCode, result.UserCode, result.VerificationURL, result.Interval, nil
}

type deviceTokenResponse struct {
	Authkey      string `json:"authkey,omitempty"`
	HeadscaleURL string `json:"headscale_url,omitempty"`
	User         string `json:"user,omitempty"`
	Error        string `json:"error,omitempty"`
}

const (
	defaultPollInterval   = 5
	deviceFlowTimeout     = 15 * time.Minute
	maxConsecutiveErrors  = 5
)

func pollOnce(coordinator, deviceCode string) (authkey, headscaleURL, user string, done bool, err error) {
	reqBody, _ := json.Marshal(map[string]string{"device_code": deviceCode})
	resp, err := http.Post(
		coordinator+"/coordinator/device/token",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return "", "", "", false, fmt.Errorf("network error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result deviceTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", "", false, fmt.Errorf("failed to decode response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return result.Authkey, result.HeadscaleURL, result.User, true, nil
	case http.StatusAccepted:
		fmt.Print(".")
		return "", "", "", false, nil
	case http.StatusGone:
		return "", "", "", true, fmt.Errorf("device code expired, please try again")
	case http.StatusForbidden:
		return "", "", "", true, fmt.Errorf("authorization denied")
	default:
		return "", "", "", false, nil
	}
}

func pollForToken(coordinator, deviceCode string, interval int) (authkey, headscaleURL, user string, err error) {
	if interval < 1 {
		interval = defaultPollInterval
	}

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	timeout := time.After(deviceFlowTimeout)
	consecutiveErrors := 0

	for {
		select {
		case <-timeout:
			return "", "", "", fmt.Errorf("authorization timed out")
		case <-ticker.C:
			authkey, headscaleURL, user, done, err := pollOnce(coordinator, deviceCode)
			if err != nil {
				consecutiveErrors++
				if consecutiveErrors >= maxConsecutiveErrors {
					return "", "", "", err
				}
				continue
			}
			consecutiveErrors = 0
			if done {
				return authkey, headscaleURL, user, nil
			}
		}
	}
}

func completeJoin(authkey, headscaleURL, user, coordinator string) error {
	creds := &workerCredentials{
		User:         user,
		Coordinator:  coordinator,
		HeadscaleURL: headscaleURL,
		JoinedAt:     time.Now(),
	}
	if err := saveWorkerCredentials(creds); err != nil {
		fmt.Printf("Warning: failed to save credentials: %v\n", err)
	}

	fmt.Printf("\nSuccessfully obtained auth key!\n\n")
	fmt.Printf("To complete the setup, run on this device:\n\n")
	fmt.Printf("  sudo tailscale up --login-server=%s --authkey=%s\n\n", headscaleURL, authkey)

	if askRunTailscale() {
		return runTailscaleUp(headscaleURL, authkey)
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
		fmt.Println("\nTo join, run:")
		fmt.Println("  wonder worker join --coordinator https://your-coordinator.example.com")
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
