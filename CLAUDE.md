# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Wonder Mesh Net is a PaaS bootstrapper that turns homelab/edge machines (behind NAT, dynamic IPs, firewalls) into bring-your-own compute for PaaS platforms and orchestration tools.

**Core Purpose**: Make scattered compute usable by PaaS/cluster orchestrators as if it were cloud VMs.

**We solve bootstrapping for BYO compute: identity + join tokens + secure connectivity.** App management is left to Kubernetes, PaaS platforms (Zeabur, Railway, Fly.io), or self-hosted PaaS (Coolify, Dokploy).

**Technology**: Tailscale/Headscale for WireGuard-based mesh networking with DERP relay fallback.

## Build Commands

```bash
make build          # Build wonder binary to bin/
make build-all      # Cross-compile for linux/darwin, amd64/arm64
make test           # Run tests with race detector
make clean          # Remove build artifacts
```

Build artifacts go to `bin/` (gitignored).

## Architecture

```
cmd/wonder/
├── main.go           # CLI entry point (cobra/viper)
├── coordinator.go    # Multi-tenant coordinator server
└── worker.go         # Worker node join/status/leave commands

pkg/
├── headscale/        # Headscale API client
│   ├── client.go     # HTTP client for Headscale REST API
│   ├── tenant.go     # Tenant management (user = tenant isolation)
│   └── acl.go        # ACL policy generation per tenant
├── jointoken/        # JWT-based join tokens for workers
├── oidc/             # Multi-provider OIDC authentication
└── wondersdk/        # Client SDK for external integrations
```

### Key Concepts

**Multi-tenancy**: Each OIDC user gets an isolated Headscale "user" (namespace). Tenant ID is derived from `hash(issuer + subject)`, username is `tenant-{id[:12]}`.

**Auth flow**: User logs in via OIDC -> coordinator creates Headscale user -> generates session token -> user creates join token -> worker exchanges token for PreAuthKey -> runs `tailscale up` with authkey.

**Coordinator endpoints**:
- `/auth/login?provider=github` - Start OIDC flow
- `/auth/callback` - OIDC callback, creates tenant
- `/api/v1/join-token` - Generate JWT for worker join (needs `X-Session-Token` header)
- `/api/v1/worker/join` - Worker exchanges JWT for Headscale PreAuthKey

## Running Locally

Requires a Headscale instance. Environment variables:
```bash
HEADSCALE_API_KEY=xxx          # Required
GITHUB_CLIENT_ID=xxx           # For GitHub OIDC
GITHUB_CLIENT_SECRET=xxx
JWT_SECRET=xxx                 # Optional, random if not set
```

```bash
./bin/wonder coordinator \
  --listen :9080 \
  --headscale-url http://localhost:8080 \
  --public-url http://localhost:9080
```

## Code Style

- No end-of-line comments
- No Chinese in code comments
