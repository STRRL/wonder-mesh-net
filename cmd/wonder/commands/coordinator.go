package commands

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator"
)

// NewCoordinatorCmd creates the coordinator subcommand that runs the
// Wonder Mesh Net coordinator server with embedded Headscale.
func NewCoordinatorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "coordinator",
		Short: "Run the coordinator server",
		Long:  `Run the Wonder Mesh Net coordinator server with embedded Headscale.`,
		Run:   runCoordinator,
	}

	cmd.Flags().String("listen", ":9080", "Coordinator listen address")
	cmd.Flags().String("public-url", "http://localhost:9080", "Public URL for callbacks")

	_ = viper.BindPFlag("coordinator.listen", cmd.Flags().Lookup("listen"))
	_ = viper.BindPFlag("coordinator.public_url", cmd.Flags().Lookup("public-url"))

	_ = viper.BindEnv("coordinator.listen", "LISTEN")
	_ = viper.BindEnv("coordinator.public_url", "PUBLIC_URL")
	_ = viper.BindEnv("coordinator.jwt_secret", "JWT_SECRET")
	_ = viper.BindEnv("coordinator.keycloak_url", "KEYCLOAK_URL")
	_ = viper.BindEnv("coordinator.keycloak_realm", "KEYCLOAK_REALM")
	_ = viper.BindEnv("coordinator.keycloak_client_id", "KEYCLOAK_CLIENT_ID")

	return cmd
}

// runCoordinator initializes and starts the coordinator server
// using configuration from flags and environment variables.
func runCoordinator(cmd *cobra.Command, args []string) {
	var cfg coordinator.Config
	cfg.Listen = viper.GetString("coordinator.listen")
	cfg.PublicURL = viper.GetString("coordinator.public_url")
	cfg.JWTSecret = viper.GetString("coordinator.jwt_secret")
	cfg.KeycloakURL = viper.GetString("coordinator.keycloak_url")
	cfg.KeycloakRealm = viper.GetString("coordinator.keycloak_realm")
	cfg.KeycloakClientID = viper.GetString("coordinator.keycloak_client_id")

	if cfg.JWTSecret == "" {
		slog.Error("JWT_SECRET environment variable is required")
		slog.Info("generate one with: openssl rand -hex 32")
		os.Exit(1)
	}

	if cfg.KeycloakURL == "" {
		slog.Error("KEYCLOAK_URL environment variable is required")
		os.Exit(1)
	}

	server, err := coordinator.BootstrapNewServer(&cfg)
	if err != nil {
		slog.Error("create server", "error", err)
		os.Exit(1)
	}

	if err := server.Run(); err != nil {
		slog.Error("shutdown error", "error", err)
	}
}
