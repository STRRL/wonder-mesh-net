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

var coordinatorURL string

// newJoinCmd creates the join subcommand that connects this device
// to the Wonder Mesh Net using either device authorization flow or a token.
func newJoinCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "join [token]",
		Short: "Join the mesh network",
		Long: `Join the Wonder Mesh Net.

Without arguments, starts device authorization flow:
  wonder worker join --coordinator https://your-coordinator.example.com

With a token, uses token-based join (legacy):
  wonder worker join <token>`,
		Args: cobra.MaximumNArgs(1),
		RunE: runJoin,
	}

	cmd.Flags().StringVar(&coordinatorURL, "coordinator", "", "Coordinator URL (required for device flow)")

	return cmd
}

// runJoin dispatches to token-based or device flow join based on arguments.
func runJoin(cmd *cobra.Command, args []string) error {
	if len(args) == 1 {
		return runTokenJoin(args[0])
	}
	return runDeviceFlowJoin()
}

// runTokenJoin performs legacy token-based join by exchanging the JWT token
// with the coordinator for a Headscale pre-auth key.
func runTokenJoin(token string) error {
	info, err := jointoken.GetJoinInfo(token)
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}

	fmt.Println("Joining Wonder Mesh Net...")
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
		return fmt.Errorf("contact coordinator: %w", err)
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
		return fmt.Errorf("decode response: %w", err)
	}

	return completeJoin(result.Authkey, result.HeadscaleURL, result.User, info.CoordinatorURL)
}

// runDeviceFlowJoin initiates OAuth 2.0 Device Authorization Flow,
// displaying a verification URL and code for user authentication.
func runDeviceFlowJoin() error {
	if coordinatorURL == "" {
		creds, err := loadCredentials()
		if err == nil && creds.CoordinatorURL != "" {
			coordinatorURL = creds.CoordinatorURL
		} else {
			return fmt.Errorf("--coordinator flag is required for device flow")
		}
	}

	fmt.Println("Starting device authorization...")
	fmt.Println()

	deviceCode, userCode, verifyURL, interval, err := requestDeviceCode(coordinatorURL)
	if err != nil {
		return fmt.Errorf("start device authorization: %w", err)
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

// deviceCodeResponse represents the coordinator's response when requesting
// a new device code for the OAuth 2.0 Device Authorization Flow.
type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// requestDeviceCode initiates the device authorization flow by requesting
// a device code and user code from the coordinator.
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
		return "", "", "", 0, fmt.Errorf("get device code: %s", string(body))
	}

	var result deviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", "", 0, err
	}

	return result.DeviceCode, result.UserCode, result.VerificationURL, result.Interval, nil
}

// deviceTokenResponse represents the coordinator's response when polling
// for token status during device authorization flow.
type deviceTokenResponse struct {
	Authkey      string `json:"authkey,omitempty"`
	HeadscaleURL string `json:"headscale_url,omitempty"`
	User         string `json:"user,omitempty"`
	Error        string `json:"error,omitempty"`
}

const (
	defaultPollInterval  = 5
	deviceFlowTimeout    = 15 * time.Minute
	maxConsecutiveErrors = 5
)

// pollOnce makes a single request to check if the user has authorized
// the device. Returns done=true when authorization completes or fails permanently.
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
		return "", "", "", false, fmt.Errorf("decode response: %w", err)
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

// pollForToken repeatedly polls the coordinator until the user authorizes
// the device, the request times out, or an unrecoverable error occurs.
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

// completeJoin saves credentials locally, displays the tailscale command,
// and optionally executes tailscale up to complete mesh network registration.
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
	fmt.Println("Successfully obtained auth key!")
	fmt.Println()
	fmt.Println("To complete the setup, run on this device:")
	fmt.Println()
	fmt.Printf("  sudo tailscale up --login-server=%s --authkey=%s\n", headscaleURL, authkey)
	fmt.Println()

	if askRunTailscale() {
		return runTailscaleUp(headscaleURL, authkey)
	}

	return nil
}

// askRunTailscale prompts the user to confirm whether to execute
// the tailscale up command automatically.
func askRunTailscale() bool {
	fmt.Print("Would you like to run this command now? [y/N]: ")
	var answer string
	_, _ = fmt.Scanln(&answer)
	return answer == "y" || answer == "Y"
}

// runTailscaleUp executes the tailscale up command with the provided
// login server and auth key to connect this device to the mesh network.
func runTailscaleUp(headscaleURL, authkey string) error {
	fmt.Println()
	fmt.Println("Running tailscale up...")

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
		return fmt.Errorf("tailscale up failed: %w", err)
	}

	fmt.Println()
	fmt.Println("Successfully joined the mesh!")
	return nil
}
