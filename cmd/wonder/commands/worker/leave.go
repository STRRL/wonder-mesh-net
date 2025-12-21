package worker

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

func newLeaveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "leave",
		Short: "Leave the mesh",
		Long:  `Remove locally stored credentials and leave the mesh.`,
		RunE:  runLeave,
	}
}

func runLeave(cmd *cobra.Command, args []string) error {
	path := getCredentialsPath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	slog.Info("Left the mesh")
	slog.Info("Note: To fully disconnect, you may also want to run: sudo tailscale down")

	return nil
}
