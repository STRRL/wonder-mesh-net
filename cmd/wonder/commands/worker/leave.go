package worker

import (
	"fmt"
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
	credentialPath, err := getCredentialsPath()
	if err != nil {
		return fmt.Errorf("leave wonder mesh %w", err)
	}
	if err := os.Remove(credentialPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	fmt.Println("Left the mesh")
	fmt.Println("\nNote: To fully disconnect, you may also want to run:")
	fmt.Println("  sudo tailscale down")

	return nil
}
