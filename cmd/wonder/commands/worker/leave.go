package worker

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// newLeaveCmd creates the leave subcommand that removes locally stored
// credentials and disconnects this device from the mesh network.
func newLeaveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "leave",
		Short: "Leave the mesh",
		Long:  `Remove locally stored credentials and leave the mesh.`,
		RunE:  runLeave,
	}
}

// runLeave removes the local credentials file and prints instructions
// for fully disconnecting from the mesh network.
func runLeave(cmd *cobra.Command, args []string) error {
	credentialPath, err := getCredentialsPath()
	if err != nil {
		return fmt.Errorf("leave wonder mesh: %w", err)
	}
	if err := os.Remove(credentialPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	fmt.Println("Left the mesh")
	fmt.Println("\nNote: To fully disconnect, you may also want to run:")
	fmt.Println("  sudo tailscale down")

	return nil
}
