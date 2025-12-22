package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	Version = "dev"
	GitSHA  = "unknown"
)

func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("wonder %s (%s)\n", Version, GitSHA)
		},
	}
}
