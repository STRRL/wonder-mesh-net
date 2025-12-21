package worker

import (
	"encoding/json"
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

func getCredentialsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".wonder", "worker.json")
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
