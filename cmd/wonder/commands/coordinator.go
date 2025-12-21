package commands

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/strrl/wonder-mesh-net/app/coordinator"
)

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
	_ = viper.BindEnv("coordinator.github_client_id", "GITHUB_CLIENT_ID")
	_ = viper.BindEnv("coordinator.github_client_secret", "GITHUB_CLIENT_SECRET")
	_ = viper.BindEnv("coordinator.google_client_id", "GOOGLE_CLIENT_ID")
	_ = viper.BindEnv("coordinator.google_client_secret", "GOOGLE_CLIENT_SECRET")
	_ = viper.BindEnv("coordinator.oidc_issuer", "OIDC_ISSUER")
	_ = viper.BindEnv("coordinator.oidc_client_id", "OIDC_CLIENT_ID")
	_ = viper.BindEnv("coordinator.oidc_client_secret", "OIDC_CLIENT_SECRET")

	return cmd
}

func runCoordinator(cmd *cobra.Command, args []string) {
	var cfg coordinator.Config
	cfg.Listen = viper.GetString("coordinator.listen")
	cfg.PublicURL = viper.GetString("coordinator.public_url")
	cfg.JWTSecret = viper.GetString("coordinator.jwt_secret")
	cfg.GithubClientID = viper.GetString("coordinator.github_client_id")
	cfg.GithubClientSecret = viper.GetString("coordinator.github_client_secret")
	cfg.GoogleClientID = viper.GetString("coordinator.google_client_id")
	cfg.GoogleClientSecret = viper.GetString("coordinator.google_client_secret")
	cfg.OIDCIssuer = viper.GetString("coordinator.oidc_issuer")
	cfg.OIDCClientID = viper.GetString("coordinator.oidc_client_id")
	cfg.OIDCClientSecret = viper.GetString("coordinator.oidc_client_secret")

	if cfg.JWTSecret == "" {
		slog.Error("JWT_SECRET environment variable is required")
		slog.Info("generate one with: openssl rand -hex 32")
		os.Exit(1)
	}

	server, err := coordinator.NewServer(&cfg)
	if err != nil {
		slog.Error("failed to create server", "error", err)
		os.Exit(1)
	}

	if err := server.Run(); err != nil {
		slog.Error("shutdown error", "error", err)
	}
}
