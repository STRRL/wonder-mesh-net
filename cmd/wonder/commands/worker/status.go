package worker

import (
	"log/slog"
	"time"

	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show worker status",
		Long:  `Show current worker status and connection information.`,
		RunE:  runStatus,
	}
}

func runStatus(cmd *cobra.Command, args []string) error {
	creds, err := loadCredentials()
	if err != nil {
		slog.Info("Not joined to any mesh")
		slog.Info("To join, run: wonder worker join --coordinator https://your-coordinator.example.com")
		return nil
	}

	slog.Info("Worker Status",
		"user", creds.User,
		"coordinator", creds.Coordinator,
		"headscale", creds.HeadscaleURL,
		"joined", creds.JoinedAt.Format(time.RFC3339),
	)

	return nil
}
