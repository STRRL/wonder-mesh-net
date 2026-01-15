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
	cmd.Flags().Bool("enable-admin-api", false, "Enable admin API endpoints")

	_ = viper.BindPFlag("coordinator.listen", cmd.Flags().Lookup("listen"))
	_ = viper.BindPFlag("coordinator.public_url", cmd.Flags().Lookup("public-url"))
	_ = viper.BindPFlag("coordinator.enable_admin_api", cmd.Flags().Lookup("enable-admin-api"))

	_ = viper.BindEnv("coordinator.listen", "LISTEN")
	_ = viper.BindEnv("coordinator.public_url", "PUBLIC_URL")
	_ = viper.BindEnv("coordinator.jwt_secret", "JWT_SECRET")
	_ = viper.BindEnv("coordinator.headscale_url", "HEADSCALE_URL")
	_ = viper.BindEnv("coordinator.headscale_unix_socket", "HEADSCALE_UNIX_SOCKET")
	_ = viper.BindEnv("coordinator.keycloak_url", "KEYCLOAK_URL")
	_ = viper.BindEnv("coordinator.keycloak_realm", "KEYCLOAK_REALM")
	_ = viper.BindEnv("coordinator.keycloak_client_id", "KEYCLOAK_CLIENT_ID")
	_ = viper.BindEnv("coordinator.keycloak_client_secret", "KEYCLOAK_CLIENT_SECRET")
	_ = viper.BindEnv("coordinator.enable_admin_api", "ENABLE_ADMIN_API")
	_ = viper.BindEnv("coordinator.admin_api_auth_token", "ADMIN_API_AUTH_TOKEN")

	return cmd
}

// runCoordinator initializes and starts the coordinator server
// using configuration from flags and environment variables.
func runCoordinator(cmd *cobra.Command, args []string) {
	var cfg coordinator.Config
	cfg.Listen = viper.GetString("coordinator.listen")
	cfg.PublicURL = viper.GetString("coordinator.public_url")
	cfg.JWTSecret = viper.GetString("coordinator.jwt_secret")
	cfg.HeadscaleURL = viper.GetString("coordinator.headscale_url")
	cfg.HeadscaleUnixSocket = viper.GetString("coordinator.headscale_unix_socket")
	cfg.KeycloakURL = viper.GetString("coordinator.keycloak_url")
	cfg.KeycloakRealm = viper.GetString("coordinator.keycloak_realm")
	cfg.KeycloakClientID = viper.GetString("coordinator.keycloak_client_id")
	cfg.KeycloakClientSecret = viper.GetString("coordinator.keycloak_client_secret")
	cfg.EnableAdminAPI = viper.GetBool("coordinator.enable_admin_api")
	cfg.AdminAPIAuthToken = viper.GetString("coordinator.admin_api_auth_token")

	if cfg.HeadscaleURL == "" {
		cfg.HeadscaleURL = coordinator.DefaultHeadscaleURL
	}
	if cfg.HeadscaleUnixSocket == "" {
		cfg.HeadscaleUnixSocket = coordinator.DefaultHeadscaleUnixSocket
	}

	if cfg.JWTSecret == "" {
		slog.Error("JWT_SECRET environment variable is required")
		slog.Info("generate one with: openssl rand -hex 32")
		os.Exit(1)
	}

	if cfg.KeycloakURL == "" {
		slog.Error("KEYCLOAK_URL environment variable is required")
		os.Exit(1)
	}

	if cfg.KeycloakClientSecret == "" {
		slog.Error("KEYCLOAK_CLIENT_SECRET environment variable is required")
		os.Exit(1)
	}

	if cfg.EnableAdminAPI {
		if cfg.AdminAPIAuthToken == "" {
			slog.Error("ADMIN_API_AUTH_TOKEN environment variable is required when admin API is enabled")
			os.Exit(1)
		}
		if len(cfg.AdminAPIAuthToken) < 32 {
			slog.Error("ADMIN_API_AUTH_TOKEN must be at least 32 characters")
			os.Exit(1)
		}
		slog.Info("admin API enabled")
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
