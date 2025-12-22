package worker

import (
	"fmt"
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
		fmt.Println("Not joined to any mesh")
		fmt.Println("\nTo join, run:")
		fmt.Println("  wonder worker join --coordinator https://your-coordinator.example.com")
		return nil
	}

	fmt.Println("Worker Status")
	fmt.Printf("  User: %s\n", creds.User)
	fmt.Printf("  Coordinator: %s\n", creds.Coordinator)
	fmt.Printf("  Headscale: %s\n", creds.HeadscaleURL)
	fmt.Printf("  Joined: %s\n", creds.JoinedAt.Format(time.RFC3339))

	return nil
}
