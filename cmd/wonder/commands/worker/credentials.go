package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// credentials stores the worker's mesh network connection information
// persisted to disk after a successful join. The data is stored as JSON
// in the user's home directory at ~/.wonder/worker.json.
type credentials struct {
	// User is the Headscale username assigned to this worker node.
	User string `json:"user"`
	// CoordinatorURL is the base URL of the Wonder Mesh Net coordinator server.
	CoordinatorURL string `json:"coordinatorURL"`
	// JoinedAt records the timestamp when this worker joined the mesh.
	JoinedAt time.Time `json:"joined_at"`
}

// getCredentialsPath returns the filesystem path where worker credentials
// are stored, typically ~/.wonder/worker.json.
func getCredentialsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get credentials path: %w", err)
	}
	return filepath.Join(home, ".wonder", "worker.json"), nil
}

// loadCredentials reads and parses the credentials file from disk.
// Returns an error if the file does not exist or cannot be parsed.
func loadCredentials() (*credentials, error) {
	credentialPath, err := getCredentialsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(credentialPath)
	if err != nil {
		return nil, err
	}

	var creds credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}

	return &creds, nil
}

// saveCredentials persists the credentials to disk, creating the parent
// directory if necessary with restricted permissions (0700 for dir, 0600 for file).
func saveCredentials(creds *credentials) error {
	credentialPath, err := getCredentialsPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(credentialPath), 0700); err != nil {
		return fmt.Errorf("create credentials directory: %w", err)
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(credentialPath, data, 0600)
}
