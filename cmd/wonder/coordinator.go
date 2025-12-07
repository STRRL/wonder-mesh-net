package main

import (
	"crypto/rand"
	"encoding/hex"
	"log"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/strrl/wonder-mesh-net/app/coordinator"
	"github.com/strrl/wonder-mesh-net/pkg/oidc"
)

var coordinatorCmd = &cobra.Command{
	Use:   "coordinator",
	Short: "Run the coordinator server",
	Long:  `Run the Wonder Mesh Net coordinator server with embedded Headscale.`,
	Run:   runCoordinator,
}

func init() {
	coordinatorCmd.Flags().String("listen", ":9080", "Coordinator listen address")
	coordinatorCmd.Flags().String("public-url", "http://localhost:9080", "Public URL for callbacks")

	_ = viper.BindPFlag("coordinator.listen", coordinatorCmd.Flags().Lookup("listen"))
	_ = viper.BindPFlag("coordinator.public_url", coordinatorCmd.Flags().Lookup("public-url"))

	_ = viper.BindEnv("coordinator.jwt_secret", "JWT_SECRET")
	_ = viper.BindEnv("coordinator.github_client_id", "GITHUB_CLIENT_ID")
	_ = viper.BindEnv("coordinator.github_client_secret", "GITHUB_CLIENT_SECRET")
	_ = viper.BindEnv("coordinator.google_client_id", "GOOGLE_CLIENT_ID")
	_ = viper.BindEnv("coordinator.google_client_secret", "GOOGLE_CLIENT_SECRET")
	_ = viper.BindEnv("coordinator.oidc_issuer", "OIDC_ISSUER")
	_ = viper.BindEnv("coordinator.oidc_client_id", "OIDC_CLIENT_ID")
	_ = viper.BindEnv("coordinator.oidc_client_secret", "OIDC_CLIENT_SECRET")
}

func runCoordinator(cmd *cobra.Command, args []string) {
	listenAddr := viper.GetString("coordinator.listen")
	publicURL := viper.GetString("coordinator.public_url")

	jwtSecret := viper.GetString("coordinator.jwt_secret")
	if jwtSecret == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			log.Fatalf("Failed to generate JWT secret: %v", err)
		}
		jwtSecret = hex.EncodeToString(b)
		log.Printf("Warning: JWT_SECRET not set, generated random secret (tokens won't survive restart)")
	}

	config := &coordinator.Config{
		ListenAddr:    listenAddr,
		PublicURL:     publicURL,
		JWTSecret:     jwtSecret,
		OIDCProviders: []oidc.ProviderConfig{},
	}

	if githubClientID := viper.GetString("coordinator.github_client_id"); githubClientID != "" {
		config.OIDCProviders = append(config.OIDCProviders, oidc.ProviderConfig{
			Type:         "github",
			Name:         "github",
			ClientID:     githubClientID,
			ClientSecret: viper.GetString("coordinator.github_client_secret"),
		})
	}

	if googleClientID := viper.GetString("coordinator.google_client_id"); googleClientID != "" {
		config.OIDCProviders = append(config.OIDCProviders, oidc.ProviderConfig{
			Type:         "google",
			Name:         "google",
			ClientID:     googleClientID,
			ClientSecret: viper.GetString("coordinator.google_client_secret"),
		})
	}

	if oidcIssuer := viper.GetString("coordinator.oidc_issuer"); oidcIssuer != "" {
		config.OIDCProviders = append(config.OIDCProviders, oidc.ProviderConfig{
			Type:         "oidc",
			Name:         "oidc",
			Issuer:       oidcIssuer,
			ClientID:     viper.GetString("coordinator.oidc_client_id"),
			ClientSecret: viper.GetString("coordinator.oidc_client_secret"),
		})
	}

	server, err := coordinator.NewServer(config)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	if err := server.Run(); err != nil {
		log.Printf("Shutdown error: %v", err)
	}
}
