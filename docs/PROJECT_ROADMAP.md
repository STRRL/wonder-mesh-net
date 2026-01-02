# Wonder Mesh Net - Project Roadmap

## Executive Summary

**Vision**: Wonder Mesh Net is a PaaS bootstrapper that turns homelab/edge machines into bring-your-own compute for PaaS platforms. We solve bootstrapping for BYO compute: identity + join tokens + secure connectivity.

**Strategic Position**:
- Commercial-grade infrastructure with open self-hosting capability
- First commercial partner: Zeabur (running their own coordinator)
- Long-term goal: Enable any organization or individual to become a coordinator operator

**Timeline**:
- Small-scale trial: 1 month (end of January 2026)
- Full production launch: 6 months (end of June 2026)

---

## Current State Assessment

### Completed Features

| Component | Status | Notes |
|-----------|--------|-------|
| Multi-tenant Architecture | ✅ Complete | 1 OIDC identity = 1 wonder net |
| Keycloak OIDC Integration | ✅ Complete | JWT validation, auto user creation |
| API Key Authentication | ✅ Complete | SHA-256 hashing, expiration support |
| Worker Join Flow | ✅ Complete | JWT token → PreAuthKey → tailscale up |
| Headscale Integration | ✅ Complete | gRPC client, ACL policies |
| E2E Testing | ✅ Complete | Docker Compose full workflow |
| Cross-platform Builds | ✅ Complete | linux/darwin × amd64/arm64 |

### Known Gaps

| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| Unit Test Coverage | Critical | M | Security-critical paths untested |
| Worker Leave Implementation | Critical | S | Currently a stub |
| OAuth 2.0 Authorization | High | L | Current API Key flow is clunky |
| Org/Team Hierarchy | High | L | Currently 1:1 user-to-realm |
| Prometheus Metrics | Medium | M | Zero observability |
| Production Deployment Docs | Medium | M | No self-hosting guide |
| MeshBackend Abstraction | Low | XL | Future multi-backend support |

### Current Pain Point

Users on Zeabur-hosted coordinator must manually:
1. Generate an API Key
2. Copy-paste it to Zeabur

This should be a single "Authorize Zeabur" click via OAuth 2.0.

---

## Milestone Definitions

### Timeline Overview

| Milestone | Objective | Target Date | Status |
|-----------|-----------|-------------|--------|
| **M0: Stable Foundation** | Test coverage, observability, worker leave | Jan 14, 2026 | Not Started |
| **M1: Small-Scale Trial** | OAuth 2.0, Zeabur integration optimization | Jan 31, 2026 | Not Started |
| **M2: Production Ready** | Org/Team, security hardening | Mar 31, 2026 | Not Started |
| **M3: General Availability** | High availability, disaster recovery | May 31, 2026 | Not Started |
| **M4: Open Ecosystem** | Self-hosting docs, multi-backend abstraction | Jun 30, 2026 | Not Started |

---

## M0: Stable Foundation (2 weeks)

**Objective**: Bring existing features to production-grade reliability.

**Target Date**: January 14, 2026

### Work Items

| Task | Size | Priority | Description |
|------|------|----------|-------------|
| Unit tests: `pkg/jointoken` | S | P0 | Security-critical token generation/validation |
| Unit tests: `pkg/jwtauth` | S | P0 | Authentication middleware edge cases |
| Unit tests: `pkg/apikey` | S | P0 | API Key validation logic |
| Implement `worker leave` | M | P0 | Currently a stub, need full implementation |
| Prometheus metrics: basics | M | P1 | Request count, latency, error rate |
| Structured error handling | S | P1 | Unified error codes, user-friendly messages |

### Acceptance Criteria

- [ ] `make test` passes with >70% coverage on `pkg/` packages
- [ ] Grafana dashboard shows basic request metrics
- [ ] Worker can successfully join and leave mesh
- [ ] All E2E tests continue to pass

### Effort Estimate

- **Total**: 20-30 hours
- **At 10 hrs/week**: 2-3 weeks

---

## M1: Small-Scale Trial (4 weeks)

**Objective**: Solve Zeabur integration pain point, enable 5-10 user trial.

**Target Date**: January 31, 2026

### Work Items

| Task | Size | Priority | Description |
|------|------|----------|-------------|
| OAuth 2.0 Authorization Server | L | P0 | Authorization code flow, token issuance |
| Third-party app registration | M | P0 | Zeabur registers as OAuth client |
| Authorization consent page | M | P0 | User clicks to authorize |
| Token scope design | S | P0 | Define permission scopes |
| Refactor `/deployer/join` | M | P1 | Use OAuth token instead of API Key |
| Audit logging: basics | M | P1 | Log authorization and access events |

### OAuth 2.0 Flow Design

```
┌─────────┐     ┌──────────┐     ┌─────────────┐
│  User   │     │  Zeabur  │     │ Coordinator │
└────┬────┘     └────┬─────┘     └──────┬──────┘
     │               │                   │
     │ Click "Connect Wonder Net"        │
     │──────────────>│                   │
     │               │                   │
     │               │ Redirect to /oauth/authorize
     │               │──────────────────>│
     │               │                   │
     │<──────────────────────────────────│
     │         Show consent page         │
     │                                   │
     │ Click "Authorize"                 │
     │──────────────────────────────────>│
     │                                   │
     │               │<──────────────────│
     │               │ Redirect with auth code
     │               │                   │
     │               │ Exchange code for token
     │               │──────────────────>│
     │               │                   │
     │               │<──────────────────│
     │               │   Access Token    │
     │               │                   │
     │               │ Call API with token
     │               │──────────────────>│
```

### New API Endpoints

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/oauth/authorize` | GET | Session | Authorization consent page |
| `/oauth/token` | POST | None | Exchange code for token |
| `/oauth/clients` | POST | Session | Register OAuth client |
| `/oauth/clients` | GET | Session | List registered clients |
| `/oauth/clients/{id}` | DELETE | Session | Revoke client |

### Token Scopes

| Scope | Permissions |
|-------|-------------|
| `nodes:read` | List nodes in wonder net |
| `nodes:write` | Add/remove nodes (deployer join) |
| `keys:read` | List API keys |
| `keys:write` | Create/delete API keys |
| `profile:read` | Read user profile |

### Acceptance Criteria

- [ ] Zeabur can complete OAuth flow and obtain access token
- [ ] Users do not need to manually copy API keys
- [ ] Token refresh works correctly
- [ ] 5-10 users can successfully use the integration
- [ ] Audit logs capture authorization events

### Effort Estimate

- **Total**: 40-60 hours
- **At 10 hrs/week**: 4-6 weeks

---

## M2: Production Ready (8 weeks)

**Objective**: Support multi-user organizations, production-grade security.

**Target Date**: March 31, 2026

### Work Items

| Task | Size | Priority | Description |
|------|------|----------|-------------|
| Org/Team data model | L | P0 | realms, orgs, memberships tables |
| Role-based permissions | M | P0 | owner, admin, member roles |
| Invite/join workflow | M | P0 | Organization member management |
| API Key security hardening | M | P1 | Encrypted storage, rotation policies |
| Rate limiting | M | P1 | Prevent abuse |
| Security audit logging | M | P1 | Sensitive operation logs |

### Data Model Changes

```sql
-- New tables
CREATE TABLE realms (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE organizations (
    id UUID PRIMARY KEY,
    realm_id UUID NOT NULL REFERENCES realms(id),
    name TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE memberships (
    id UUID PRIMARY KEY,
    org_id UUID NOT NULL REFERENCES organizations(id),
    user_id TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(org_id, user_id)
);

-- Migrate existing data
-- wonder_nets.owner_id becomes memberships with role='owner'
```

### Role Permissions

| Permission | Owner | Admin | Member |
|------------|-------|-------|--------|
| View nodes | ✅ | ✅ | ✅ |
| Generate join tokens | ✅ | ✅ | ❌ |
| Manage API keys | ✅ | ✅ | ❌ |
| Manage OAuth clients | ✅ | ✅ | ❌ |
| Invite members | ✅ | ✅ | ❌ |
| Remove members | ✅ | ✅ | ❌ |
| Delete organization | ✅ | ❌ | ❌ |
| Transfer ownership | ✅ | ❌ | ❌ |

### Acceptance Criteria

- [ ] Multiple users can share a wonder net via organization
- [ ] Role-based permissions enforced correctly
- [ ] Invitation flow works (email or link)
- [ ] API keys stored with encryption at rest
- [ ] Rate limiting prevents abuse
- [ ] Security audit passes basic review

### Effort Estimate

- **Total**: 60-80 hours
- **At 10 hrs/week**: 6-8 weeks

---

## M3: General Availability (8 weeks)

**Objective**: Scale to 1000+ nodes, achieve 99.9% availability.

**Target Date**: May 31, 2026

### Work Items

| Task | Size | Priority | Description |
|------|------|----------|-------------|
| PostgreSQL production deployment | M | P0 | Replace SQLite for scale |
| Coordinator high availability | L | P0 | Multi-instance, load balancing |
| Headscale high availability | L | P0 | Multi-instance or hot standby |
| Alerting system | M | P1 | Anomaly detection, notifications |
| Performance optimization | M | P1 | Connection pooling, caching |
| Disaster recovery plan | L | P1 | Backup, restore procedures |

### High Availability Architecture

```
                    ┌─────────────┐
                    │   HAProxy   │
                    │   / Nginx   │
                    └──────┬──────┘
                           │
           ┌───────────────┼───────────────┐
           │               │               │
    ┌──────▼──────┐ ┌──────▼──────┐ ┌──────▼──────┐
    │ Coordinator │ │ Coordinator │ │ Coordinator │
    │   Node 1    │ │   Node 2    │ │   Node 3    │
    └──────┬──────┘ └──────┬──────┘ └──────┬──────┘
           │               │               │
           └───────────────┼───────────────┘
                           │
                    ┌──────▼──────┐
                    │ PostgreSQL  │
                    │  (Primary)  │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │ PostgreSQL  │
                    │  (Replica)  │
                    └─────────────┘
```

### SLA Targets

| Metric | Target |
|--------|--------|
| Availability | 99.9% (8.76 hrs/year downtime) |
| API Latency (p99) | < 500ms |
| Node Join Time | < 30 seconds |
| Recovery Time Objective (RTO) | < 1 hour |
| Recovery Point Objective (RPO) | < 5 minutes |

### Acceptance Criteria

- [ ] Support 1000+ concurrent nodes
- [ ] 99.9% uptime over 30-day period
- [ ] Automated failover works correctly
- [ ] Disaster recovery tested and documented
- [ ] Alerting catches critical issues within 5 minutes

### Effort Estimate

- **Total**: 80-120 hours
- **At 10 hrs/week**: 8-12 weeks

---

## M4: Open Ecosystem (4 weeks)

**Objective**: Enable anyone to run their own coordinator.

**Target Date**: June 30, 2026

### Work Items

| Task | Size | Priority | Description |
|------|------|----------|-------------|
| Self-hosting documentation | M | P0 | Docker, K8s, systemd guides |
| One-click deploy scripts | M | P0 | Lower barrier to entry |
| MeshBackend abstraction layer | XL | P1 | Support multiple network backends |
| Netbird adapter | L | P2 | First alternative backend |
| Plugin system design | L | P2 | Extensibility framework |

### Self-Hosting Requirements

**Minimal Setup** (single node):
- 1 VM with 2 vCPU, 4GB RAM
- Docker or systemd
- Domain with TLS certificate
- Keycloak instance (can be shared)

**Production Setup** (HA):
- 3+ coordinator nodes
- PostgreSQL cluster
- Load balancer
- Monitoring stack

### MeshBackend Interface

The interface is already implemented in `pkg/meshbackend/backend.go`:

```go
type MeshBackend interface {
    // MeshType returns the mesh network type.
    // Clients use this to determine how to process the metadata from CreateJoinCredentials.
    MeshType() MeshType

    // CreateRealm creates an isolated network namespace.
    CreateRealm(ctx context.Context, name string) error

    // GetRealm checks if a realm exists.
    GetRealm(ctx context.Context, name string) (exists bool, err error)

    // CreateJoinCredentials generates credentials for a node to join the mesh.
    // Returns backend-specific metadata serialized directly to the API response.
    CreateJoinCredentials(ctx context.Context, realmName string, opts JoinOptions) (map[string]any, error)

    // ListNodes returns all nodes in a realm.
    ListNodes(ctx context.Context, realmName string) ([]*Node, error)

    // Healthy performs a health check on the backend.
    Healthy(ctx context.Context) error
}

type JoinOptions struct {
    TTL       time.Duration
    Reusable  bool
    Ephemeral bool
}

type MeshType string

const (
    MeshTypeTailscale MeshType = "tailscale"
    MeshTypeNetbird   MeshType = "netbird"
    MeshTypeZeroTier  MeshType = "zerotier"
)
```

**Design Notes:**
- `CreateJoinCredentials` combines auth key creation and metadata assembly into one operation
- Backend-specific metadata (authkey, login_server, etc.) is returned as `map[string]any` to avoid meaningless abstraction
- `DeleteRealm` and `DeleteNode` are deferred to future milestones as they require careful handling of connected nodes

### Acceptance Criteria

- [ ] New user can deploy coordinator in < 30 minutes
- [ ] Documentation covers common scenarios
- [ ] At least 2 mesh backends supported
- [ ] Community contributions enabled

### Effort Estimate

- **Total**: 80-120 hours
- **At 10 hrs/week**: 8-12 weeks

---

## Total Effort Summary

| Milestone | Hours | At 10 hrs/week | At 20 hrs/week |
|-----------|-------|----------------|----------------|
| M0: Stable Foundation | 20-30 | 2-3 weeks | 1-2 weeks |
| M1: Small-Scale Trial | 40-60 | 4-6 weeks | 2-3 weeks |
| M2: Production Ready | 60-80 | 6-8 weeks | 3-4 weeks |
| M3: General Availability | 80-120 | 8-12 weeks | 4-6 weeks |
| M4: Open Ecosystem | 80-120 | 8-12 weeks | 4-6 weeks |
| **Total** | **280-410** | **28-41 weeks** | **14-21 weeks** |

---

## Key Decision Points

Decisions that must be made before implementation:

| Decision | When | Options | Recommendation |
|----------|------|---------|----------------|
| OAuth 2.0 library | Before M1 | Build vs ory/fosite vs go-oauth2 | Use ory/fosite (battle-tested) |
| Token scope granularity | Before M1 | Fine-grained vs coarse | Start coarse, refine later |
| Org hierarchy depth | Before M2 | 2-level (org→user) vs 3-level (org→team→user) | 2-level initially |
| Database for production | Before M3 | PostgreSQL vs CockroachDB | PostgreSQL (simpler) |
| First alternative backend | Before M4 | Netbird vs ZeroTier | Netbird (more similar to Tailscale) |

---

## Risks and Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| OAuth 2.0 implementation complexity | M1 delay | Medium | Use mature library (ory/fosite) |
| Headscale stability issues | All milestones | Low | Monitoring, quick rollback |
| Zeabur requirements change | M1 rework | Medium | Lock down API contract early |
| Self-hosting support burden | Post-M4 | High | Invest in documentation, community |
| Keycloak dependency concerns | Long-term | Low | Document alternatives |

---

## Next Steps

1. **Confirm this roadmap** aligns with expectations
2. **Determine weekly time investment** to calibrate timeline
3. **Begin M0** with unit tests for `pkg/jointoken`
4. **Schedule sync with Zeabur** to confirm OAuth requirements

---

## Appendix: File References

Key files for each area:

| Area | Files |
|------|-------|
| Coordinator server | `internal/app/coordinator/server.go` |
| Authentication | `pkg/jwtauth/`, `pkg/apikey/` |
| Join tokens | `pkg/jointoken/token.go` |
| Headscale client | `pkg/headscale/` |
| Database | `internal/app/coordinator/database/` |
| Services | `internal/app/coordinator/service/` |
| Controllers | `internal/app/coordinator/controller/` |
| Worker CLI | `cmd/wonder/commands/worker/` |
| E2E tests | `e2e/test.sh` |

---
