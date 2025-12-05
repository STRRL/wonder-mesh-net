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
	Long:  `Run the Wonder Mesh Net coordinator server that wraps Headscale API and provides OIDC authentication.`,
	Run:   runCoordinator,
}

func init() {
	coordinatorCmd.Flags().String("listen", ":8080", "Listen address")
	coordinatorCmd.Flags().String("headscale-url", "http://localhost:8080", "Headscale API URL (internal)")
	coordinatorCmd.Flags().String("headscale-public-url", "", "Headscale URL for workers (defaults to headscale-url)")
	coordinatorCmd.Flags().String("headscale-api-key", "", "Headscale API key")
	coordinatorCmd.Flags().String("public-url", "http://localhost:8080", "Public URL for callbacks")

	_ = viper.BindPFlag("coordinator.listen", coordinatorCmd.Flags().Lookup("listen"))
	_ = viper.BindPFlag("coordinator.headscale_url", coordinatorCmd.Flags().Lookup("headscale-url"))
	_ = viper.BindPFlag("coordinator.headscale_public_url", coordinatorCmd.Flags().Lookup("headscale-public-url"))
	_ = viper.BindPFlag("coordinator.headscale_api_key", coordinatorCmd.Flags().Lookup("headscale-api-key"))
	_ = viper.BindPFlag("coordinator.public_url", coordinatorCmd.Flags().Lookup("public-url"))

	_ = viper.BindEnv("coordinator.headscale_api_key", "HEADSCALE_API_KEY")
	_ = viper.BindEnv("coordinator.jwt_secret", "JWT_SECRET")
	_ = viper.BindEnv("coordinator.github_client_id", "GITHUB_CLIENT_ID")
	_ = viper.BindEnv("coordinator.github_client_secret", "GITHUB_CLIENT_SECRET")
	_ = viper.BindEnv("coordinator.google_client_id", "GOOGLE_CLIENT_ID")
	_ = viper.BindEnv("coordinator.google_client_secret", "GOOGLE_CLIENT_SECRET")
}

func runCoordinator(cmd *cobra.Command, args []string) {
	listenAddr := viper.GetString("coordinator.listen")
	headscaleURL := viper.GetString("coordinator.headscale_url")
	headscalePublicURL := viper.GetString("coordinator.headscale_public_url")
	headscaleAPIKey := viper.GetString("coordinator.headscale_api_key")
	publicURL := viper.GetString("coordinator.public_url")

	if headscaleAPIKey == "" {
		log.Fatal("headscale-api-key is required (flag, config, or HEADSCALE_API_KEY env)")
	}

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
		ListenAddr:         listenAddr,
		HeadscaleURL:       headscaleURL,
		HeadscalePublicURL: headscalePublicURL,
		HeadscaleAPIKey:    headscaleAPIKey,
		PublicURL:          publicURL,
		JWTSecret:          jwtSecret,
		OIDCProviders:      []oidc.ProviderConfig{},
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

	server, err := coordinator.NewServer(config)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	if err := server.Run(); err != nil {
		log.Printf("Shutdown error: %v", err)
	}
}
