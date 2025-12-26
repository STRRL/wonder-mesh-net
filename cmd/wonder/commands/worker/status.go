package worker

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// newStatusCmd creates the status subcommand that displays the current
// worker node connection status and mesh network information.
func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show worker status",
		Long:  `Show current worker status and connection information.`,
		RunE:  runStatus,
	}
}

// runStatus loads and displays the locally stored credentials including
// user, coordinator URL, and join timestamp.
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
	fmt.Printf("  Coordinator: %s\n", creds.CoordinatorURL)
	fmt.Printf("  Joined: %s\n", creds.JoinedAt.Format(time.RFC3339))

	return nil
}
