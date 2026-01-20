# CLAUDE.md - Helm Chart

This file provides guidance for working with the Wonder Mesh Net Helm chart.

## Chart Structure

```
charts/wonder-mesh-net/
├── Chart.yaml              # Chart metadata (version, appVersion)
├── values.yaml             # Default configuration values
├── README.md               # User documentation
└── templates/
    ├── _helpers.tpl        # Template helper functions
    ├── deployment.yaml     # Coordinator + Headscale sidecar
    ├── deployment-keycloak.yaml  # Optional Keycloak for dev/test
    ├── service.yaml        # Coordinator service
    ├── service-keycloak.yaml
    ├── configmap-headscale.yaml  # Headscale config
    ├── configmap-keycloak.yaml   # Keycloak realm import
    ├── secret.yaml         # JWT secret, Headscale API key
    ├── secret-db.yaml      # Database DSN
    ├── serviceaccount.yaml
    ├── ingress.yaml
    ├── pvc.yaml            # Coordinator persistence
    └── pvc-keycloak.yaml
```

## Key Architecture Decisions

**Coordinator + Headscale as sidecar**: The main deployment runs both containers in a single pod, sharing:
- Unix socket for Headscale communication (`/var/run/headscale/headscale.sock`)
- Localhost network for direct access

**No bundled PostgreSQL**: Users must provide their own database if not using SQLite. Configure via:
- `coordinator.database.driver`: `sqlite` or `postgres`
- `coordinator.database.dsn`: Connection string for external database

**Keycloak for dev/test only**: The bundled Keycloak uses dev mode with H2 database. For production, use an external OIDC provider.

## Common Commands

```bash
# Lint the chart
helm lint charts/wonder-mesh-net

# Template locally (dry-run)
helm template my-release charts/wonder-mesh-net

# Template with specific values
helm template my-release charts/wonder-mesh-net \
  --set keycloak.enabled=true \
  --set coordinator.publicUrl=https://wonder.example.com

# Install
helm install my-release charts/wonder-mesh-net -n wonder --create-namespace

# Upgrade
helm upgrade my-release charts/wonder-mesh-net -n wonder

# Uninstall
helm uninstall my-release -n wonder
```

## Testing Changes

```bash
# Run chart tests (requires chart-testing tool)
ct lint --charts charts/wonder-mesh-net

# Validate templates render correctly
helm template test charts/wonder-mesh-net --debug
```

## Naming Conventions

All resources use the `wonder-mesh-net.fullname` helper which produces:
- `{release-name}-wonder-mesh-net` (if release name differs from chart name)
- `{release-name}` (if release name contains "wonder-mesh-net")

Resource suffixes:
- `-secret`: Main secrets (JWT, Headscale API key)
- `-db`: Database secret
- `-headscale`: Headscale configmap
- `-keycloak`: Keycloak resources

## Values Structure

Top-level sections in `values.yaml`:
- `headscale`: Headscale sidecar configuration
- `keycloak`: Optional Keycloak deployment
- `coordinator`: Main application settings
- `serviceAccount`: Service account settings
- `service`: Coordinator service settings
- `ingress`: Ingress configuration
- `persistence`: Coordinator data persistence

## Version Management

- `Chart.yaml` `version`: Chart version (semver), bump for chart changes
- `Chart.yaml` `appVersion`: Application version, updated by CI from git tags
- `coordinator.image.tag`: Defaults to `appVersion` if empty
