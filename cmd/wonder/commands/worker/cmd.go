package worker

import (
	"github.com/spf13/cobra"
)

// NewCmd creates the worker subcommand group containing commands
// for managing this device as a worker node in Wonder Mesh Net.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Worker node commands",
		Long:  `Commands for managing this device as a worker node in Wonder Mesh Net.`,
	}

	cmd.AddCommand(newJoinCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newLeaveCmd())

	return cmd
}
