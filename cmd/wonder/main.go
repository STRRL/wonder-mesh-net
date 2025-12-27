package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/strrl/wonder-mesh-net/cmd/wonder/commands"
	"github.com/strrl/wonder-mesh-net/cmd/wonder/commands/worker"
)

// newRootCmd creates the root cobra command for the wonder CLI.
func newRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "wonder",
		Short: "Wonder Mesh Net CLI",
		Long: `Wonder Mesh Net - A networking layer that connects homelab machines
to the internet, making them accessible to PaaS platforms and orchestration tools.`,
	}
}

// initConfig returns a configuration initializer that sets up viper
// to read from config files and environment variables.
func initConfig(configFile *string) func() {
	return func() {
		if *configFile != "" {
			viper.SetConfigFile(*configFile)
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}

			viper.AddConfigPath(home + "/.wonder")
			viper.AddConfigPath(".")
			viper.SetConfigName("config")
			viper.SetConfigType("yaml")
		}

		viper.SetEnvPrefix("WONDER")
		viper.AutomaticEnv()

		if err := viper.ReadInConfig(); err == nil {
			fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
		}
	}
}

func main() {
	var configFile string

	rootCmd := newRootCmd()

	cobra.OnInitialize(initConfig(&configFile))

	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "config file (default is $HOME/.wonder/config.yaml)")

	rootCmd.AddCommand(commands.NewVersionCmd())
	rootCmd.AddCommand(commands.NewCoordinatorCmd())
	rootCmd.AddCommand(worker.NewWorkerCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
