package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type credentials struct {
	User         string    `json:"user"`
	Coordinator  string    `json:"coordinator"`
	HeadscaleURL string    `json:"headscale_url"`
	JoinedAt     time.Time `json:"joined_at"`
}

func getCredentialsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get credentials path: %w", err)
	}
	return filepath.Join(home, ".wonder", "worker.json"), nil
}

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
