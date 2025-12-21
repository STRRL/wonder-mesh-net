package worker

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/strrl/wonder-mesh-net/pkg/jointoken"
)

var coordinatorURL string

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

func runJoin(cmd *cobra.Command, args []string) error {
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

	slog.Info("Joining Wonder Mesh Net...",
		"coordinator", info.CoordinatorURL,
		"user", info.HeadscaleUser,
		"token_expires", info.ExpiresAt.Format(time.RFC3339),
	)

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
		creds, err := loadCredentials()
		if err == nil && creds.Coordinator != "" {
			coordinatorURL = creds.Coordinator
		} else {
			return fmt.Errorf("--coordinator flag is required for device flow")
		}
	}

	slog.Info("Starting device authorization...")

	deviceCode, userCode, verifyURL, interval, err := requestDeviceCode(coordinatorURL)
	if err != nil {
		return fmt.Errorf("failed to start device authorization: %w", err)
	}

	slog.Info("To authorize this device",
		"url", verifyURL+"?code="+userCode,
		"code", userCode,
	)
	slog.Info("Waiting for authorization...")

	authkey, headscaleURL, user, err := pollForToken(coordinatorURL, deviceCode, interval)
	if err != nil {
		return err
	}

	slog.Info("Device authorized!")

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
	defaultPollInterval  = 5
	deviceFlowTimeout    = 15 * time.Minute
	maxConsecutiveErrors = 5
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
		slog.Debug("Polling for authorization...")
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
	creds := &credentials{
		User:         user,
		Coordinator:  coordinator,
		HeadscaleURL: headscaleURL,
		JoinedAt:     time.Now(),
	}
	if err := saveCredentials(creds); err != nil {
		slog.Warn("Failed to save credentials", "error", err)
	}

	slog.Info("Successfully obtained auth key!")
	slog.Info("To complete the setup, run on this device",
		"command", "sudo tailscale up --login-server="+headscaleURL+" --authkey="+authkey,
	)

	if askRunTailscale() {
		return runTailscaleUp(headscaleURL, authkey)
	}

	return nil
}

func askRunTailscale() bool {
	slog.Info("Would you like to run this command now? [y/N]")
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(answer)
	return answer == "y" || answer == "Y"
}

func runTailscaleUp(headscaleURL, authkey string) error {
	slog.Info("Running tailscale up...")

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

	slog.Info("Successfully joined the mesh!")
	return nil
}
