package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authentication commands",
	Long:  `Authentication commands for Wonder Mesh Net (login, authkey, status, logout).`,
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login via OIDC provider",
	Long:  `Login to Wonder Mesh Net coordinator using an OIDC provider (e.g., Google).`,
	RunE:  runAuthLogin,
}

var authAuthkeyCmd = &cobra.Command{
	Use:   "authkey",
	Short: "Generate an auth key for device registration",
	Long:  `Generate a pre-authentication key that can be used to register devices with Tailscale.`,
	RunE:  runAuthAuthkey,
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show login status and nodes",
	Long:  `Show current login status and list all registered nodes.`,
	RunE:  runAuthStatus,
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove stored credentials",
	Long:  `Remove locally stored credentials and log out.`,
	RunE:  runAuthLogout,
}

func init() {
	authLoginCmd.Flags().String("coordinator", "http://localhost:8080", "Coordinator URL")
	authLoginCmd.Flags().String("provider", "google", "OIDC provider name")

	_ = viper.BindPFlag("auth.coordinator", authLoginCmd.Flags().Lookup("coordinator"))
	_ = viper.BindPFlag("auth.provider", authLoginCmd.Flags().Lookup("provider"))

	authAuthkeyCmd.Flags().String("ttl", "24h", "Key TTL (e.g., 1h, 24h, 168h)")
	authAuthkeyCmd.Flags().Bool("reusable", false, "Make key reusable")

	_ = viper.BindPFlag("auth.ttl", authAuthkeyCmd.Flags().Lookup("ttl"))
	_ = viper.BindPFlag("auth.reusable", authAuthkeyCmd.Flags().Lookup("reusable"))

	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authAuthkeyCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authLogoutCmd)
}

type credentials struct {
	Session     string    `json:"session"`
	User        string    `json:"user"`
	Coordinator string    `json:"coordinator"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

func getCredentialsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".wonder", "credentials.json")
}

func loadCredentials() (*credentials, error) {
	data, err := os.ReadFile(getCredentialsPath())
	if err != nil {
		return nil, err
	}

	var creds credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}

	return &creds, nil
}

func saveCredentials(creds *credentials) error {
	dir := filepath.Dir(getCredentialsPath())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(getCredentialsPath(), data, 0600)
}

func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		return fmt.Errorf("unsupported platform")
	}

	return exec.Command(cmd, args...).Start()
}

func runAuthLogin(cmd *cobra.Command, args []string) error {
	coordinatorURL := viper.GetString("auth.coordinator")
	provider := viper.GetString("auth.provider")

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to start local server: %w", err)
	}
	defer func() { _ = listener.Close() }()

	localAddr := listener.Addr().String()
	callbackURL := "http://" + localAddr + "/callback"

	resultCh := make(chan *credentials, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		session := r.URL.Query().Get("session")
		user := r.URL.Query().Get("user")

		if session == "" {
			errCh <- fmt.Errorf("no session in callback")
			_, _ = fmt.Fprintln(w, "Login failed: no session received")
			return
		}

		creds := &credentials{
			Session:     session,
			User:        user,
			Coordinator: coordinatorURL,
			ExpiresAt:   time.Now().Add(24 * time.Hour),
		}

		resultCh <- creds

		_, _ = fmt.Fprintln(w, "<html><body>")
		_, _ = fmt.Fprintln(w, "<h1>Login successful!</h1>")
		_, _ = fmt.Fprintln(w, "<p>You can close this window and return to the terminal.</p>")
		_, _ = fmt.Fprintln(w, "</body></html>")
	})

	server := &http.Server{Handler: mux}
	go func() {
		_ = server.Serve(listener)
	}()

	loginURL := fmt.Sprintf("%s/auth/login?provider=%s&redirect_uri=%s",
		coordinatorURL, provider, callbackURL)

	fmt.Printf("Opening browser for login...\n")
	fmt.Printf("If browser doesn't open, visit: %s\n\n", loginURL)

	if err := openBrowser(loginURL); err != nil {
		fmt.Printf("Failed to open browser: %v\n", err)
	}

	select {
	case creds := <-resultCh:
		if err := saveCredentials(creds); err != nil {
			return fmt.Errorf("failed to save credentials: %w", err)
		}
		fmt.Printf("Login successful!\n")
		fmt.Printf("Logged in as: %s\n", creds.User)
		return nil

	case err := <-errCh:
		return err

	case <-time.After(5 * time.Minute):
		return fmt.Errorf("login timeout")
	}
}

func runAuthAuthkey(cmd *cobra.Command, args []string) error {
	ttl := viper.GetString("auth.ttl")
	reusable := viper.GetBool("auth.reusable")

	creds, err := loadCredentials()
	if err != nil {
		return fmt.Errorf("not logged in, run 'wonder auth login' first: %w", err)
	}

	reqBody := fmt.Sprintf(`{"ttl": "%s", "reusable": %t}`, ttl, reusable)

	req, err := http.NewRequest(http.MethodPost, creds.Coordinator+"/api/v1/authkey",
		strings.NewReader(reqBody))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-Token", creds.Session)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create authkey: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create authkey: %s", string(body))
	}

	var result struct {
		Key        string    `json:"key"`
		Expiration time.Time `json:"expiration"`
		Reusable   bool      `json:"reusable"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Printf("Auth Key: %s\n", result.Key)
	fmt.Printf("Expires:  %s\n", result.Expiration.Format(time.RFC3339))
	fmt.Printf("Reusable: %t\n", result.Reusable)
	fmt.Println()
	fmt.Println("To join a device to your network:")
	fmt.Printf("  tailscale up --login-server=<HEADSCALE_URL> --authkey=%s\n", result.Key)

	return nil
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	creds, err := loadCredentials()
	if err != nil {
		fmt.Println("Not logged in")
		return nil
	}

	fmt.Printf("Logged in as: %s\n", creds.User)
	fmt.Printf("Coordinator:  %s\n", creds.Coordinator)
	fmt.Printf("Session:      %s...\n", creds.Session[:12])

	req, err := http.NewRequest(http.MethodGet, creds.Coordinator+"/api/v1/nodes", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Session-Token", creds.Session)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to list nodes: %s", string(body))
	}

	var result struct {
		Nodes []struct {
			ID        uint64   `json:"id"`
			Name      string   `json:"name"`
			IPAddress []string `json:"ipAddresses"`
			Online    bool     `json:"online"`
		} `json:"nodes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Println()
	fmt.Printf("Nodes (%d):\n", len(result.Nodes))
	for _, node := range result.Nodes {
		status := "offline"
		if node.Online {
			status = "online"
		}
		fmt.Printf("  - %s (%s) [%s]\n", node.Name, strings.Join(node.IPAddress, ", "), status)
	}

	return nil
}

func runAuthLogout(cmd *cobra.Command, args []string) error {
	path := getCredentialsPath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Println("Logged out")
	return nil
}
