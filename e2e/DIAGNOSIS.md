# E2E Test Diagnosis Report

## Current Status

### ✅ Successfully Working
1. **E2E Infrastructure**
   - Keycloak startup and configuration
   - Coordinator startup with embedded Headscale (after restart)
   - Worker containers startup
   - Docker Compose orchestration

2. **OIDC Authentication Flow**
   - Login redirect to Keycloak: ✅
   - Form submission and authentication: ✅
   - Callback handling: ✅
   - Session token generation: ✅
   - Join token creation: ✅
   - Worker authkey exchange: ✅

3. **Network Configuration**
   - Coordinator using `network_mode: host` to access Keycloak at localhost:9090
   - Worker containers with `extra_hosts: coordinator:host-gateway`
   - Headscale proxy at `/hs/` path

### ❌ Current Issue

**Tailscale client registration fails - state remains "NeedsLogin"**

```
tailscale up --login-server="http://coordinator:9080/hs" --authkey="..." --accept-routes
```

Times out after 30 seconds with:
```
context canceled
no current Tailscale IPs; state: NeedsLogin
```

## Root Cause Analysis

### Potential Issues

1. **Worker → Coordinator connectivity**
   - Workers use `extra_hosts: coordinator:host-gateway`
   - Coordinator listens on host network port 9080
   - Need to verify: Can worker containers actually reach coordinator:9080?

2. **Headscale server_url configuration**
   - Current config: `server_url: http://coordinator:9080/hs`
   - This URL is what Tailscale clients use to connect
   - Issue: This URL must be reachable from within worker containers

3. **Tailscale client → Headscale protocol**
   - Tailscale 1.92.1 (latest)
   - Headscale 0.27.1
   - Possible protocol compatibility issue?
   - Need to check Headscale logs for incoming connection attempts

## Next Steps for Debugging

1. **Verify Worker Network Connectivity**
   ```bash
   docker exec worker-1 cat /etc/hosts | grep coordinator
   docker exec worker-1 curl http://coordinator:9080/hs/health
   docker exec worker-1 nslookup coordinator
   ```

2. **Check Headscale Logs During Registration**
   ```bash
   docker logs coordinator 2>&1 | grep -E "(machine|node|register)"
   ```

3. **Test Tailscale with Verbose Logging**
   ```bash
   docker exec worker-1 tailscaled --verbose=true ...
   docker exec worker-1 tailscale --socket=/tmp/tailscaled.sock up \
     --login-server="http://coordinator:9080/hs" \
     --authkey="..." \
     --verbose
   ```

4. **Alternative server_url values to test**
   - `http://host.docker.internal:9080/hs` (if on Docker Desktop)
   - `http://<host-ip>:9080/hs` (explicit host IP)
   - Direct access without DNS resolution

## Configuration Files

### docker-compose.yml (Current)
```yaml
coordinator:
  network_mode: "host"
  environment:
    OIDC_ISSUER: http://localhost:9090/realms/wonder

worker-1/worker-2:
  extra_hosts:
    - "coordinator:host-gateway"
```

### headscale-config.yaml (Current)
```yaml
server_url: http://coordinator:9080/hs
listen_addr: 0.0.0.0:8080
```

## Tested Configurations

| Attempt | OIDC_ISSUER | Coordinator Network | Result |
|---------|------------|-------------------|--------|
| 1 | keycloak:8080 | bridge | OIDC failed (URL rewrite needed) |
| 2 | keycloak:8080 + URL rewrite | bridge | OIDC callback failed (issuer mismatch) |
| 3 | host.docker.internal:9090 | bridge | OIDC callback failed (issuer mismatch) |
| 4 | localhost:9090 | host | OIDC ✅ / Tailscale ❌ |

## Recommendation

The fundamental issue is that we need a URL that works for:
1. Coordinator (to access Keycloak)
2. Test script / browser (to access Keycloak)
3. Worker containers (to access Coordinator/Headscale)

Current architecture with `network_mode: host` for coordinator solves #1 and #2 but may have broken #3.

**Option A**: Fix worker → coordinator connectivity
- Verify `extra_hosts: coordinator:host-gateway` works correctly
- May need to use explicit IP or alternative DNS resolution

**Option B**: Use Tailscale's local test mode
- Run Headscale on host network port (not behind coordinator proxy)
- Workers access `http://host.docker.internal:8080` directly
- Bypasses coordinator proxy complexity

**Option C**: Simplify network architecture
- Run ALL services in host network mode
- Simpler connectivity, but less container isolation
