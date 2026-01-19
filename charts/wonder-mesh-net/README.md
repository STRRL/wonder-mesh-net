# Wonder Mesh Net Helm Chart

[Wonder Mesh Net](https://github.com/strrl/wonder-mesh-net) is a PaaS bootstrapper that turns homelab/edge machines into bring-your-own compute.

## Introduction

This chart bootstraps a Wonder Mesh Net deployment on a Kubernetes cluster using the Helm package manager. It includes:
- Coordinator service
- Embedded Headscale instance
- PostgreSQL (optional, default uses SQLite)
- Keycloak (optional, for development/testing)

## Prerequisites

- Kubernetes 1.19+
- Helm 3.2.0+

## Installing the Chart

To install the chart with the release name `my-release`:

```console
$ helm install my-release ./charts/wonder-mesh-net
```

### Enabling Keycloak (Minimal Setup)

To automatically deploy a minimal Keycloak instance for testing:

```console
$ helm install my-release ./charts/wonder-mesh-net --set keycloak.enabled=true
```

When enabled:
- A Keycloak pod is deployed with a pre-configured "wonder" realm.
- Coordinator is automatically configured to point to this internal Keycloak instance.
- **Note**: For the OIDC browser login flow to work, the browser must be able to reach Keycloak. The chart configures the internal URL by default. For local testing (Minikube), you may need to use `kubectl port-forward` or configure Ingress to ensure the Keycloak URL is accessible from your browser.

## Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `headscale.enabled` | Enable embedded Headscale sidecar | `true` |
| `keycloak.enabled` | Enable embedded Keycloak for dev/test | `false` |
| `keycloak.adminUser` | Keycloak admin username | `admin` |
| `keycloak.adminPassword` | Keycloak admin password | `admin` |
| `coordinator.publicUrl` | Public URL for the coordinator (must match Ingress) | `http://localhost:9080` |
| `coordinator.database.driver` | Coordinator database driver (`sqlite` or `postgres`) | `sqlite` |
| `coordinator.database.dsn` | Coordinator database DSN (required for postgres) | `""` |
| ... (see values.yaml for full list) ...

When `postgres.enabled=true`, the chart configures the coordinator to use the internal PostgreSQL service by default. To override, set `coordinator.database.dsn` explicitly.

## Headscale Configuration

The embedded Headscale instance is enabled by default. It shares the pod network with the Coordinator, communicating over a Unix socket and localhost.

## OIDC Configuration

If `keycloak.enabled` is `false` (default), you must provide external OIDC details in `coordinator.oidc.*`.
If `keycloak.enabled` is `true`, these values are automatically set to the internal Keycloak instance, but can still be overridden.

### Enabling Production Mode with PostgreSQL

For production deployments, use production mode with persistent PostgreSQL:

```console
helm install my-release ./charts/wonder-mesh-net \
  --set keycloak.enabled=true \
  --set keycloak.production=true \
  --set postgres.enabled=true \
  --set postgres.persistence.enabled=true \
  --set postgres.auth.password=your-secure-password
```

#### Keycloak Configuration Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `keycloak.production` | Use production mode (`start`) instead of dev mode (`start-dev`) | `false` |
| `postgres.enabled` | Enable integrated PostgreSQL for Keycloak | `false` |
| `postgres.auth.database` | PostgreSQL database name | `keycloak` |
| `postgres.auth.username` | PostgreSQL username | `keycloak` |
| `postgres.auth.password` | PostgreSQL password | `keycloak` |
| `postgres.persistence.enabled` | Enable PostgreSQL persistence | `false` |
| `postgres.persistence.size` | PVC size | `1Gi` |

**Security note:** For production, always:
- Set strong passwords via `postgres.auth.password`
- Enable persistence via `postgres.persistence.enabled=true`
- Enable persistence for Keycloak data as well
