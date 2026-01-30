# Fix Plan - Issue #84: Tailnet Isolation

## High Priority

- [x] **Task 1: Write isolation integration test first (TDD)**
  - Created `internal/app/coordinator/service/nodes_test.go`
  - Test scenario: User A cannot see User B's nodes in ListNodes
  - Test scenario: User A cannot access User B's nodes via GetNode
  - Test scenario: User A cannot delete User B's nodes via DeleteNode
  - Test confirmed the gap existed before fix

- [x] **Task 2: Add secondary realm validation in ListNodes**
  - File: `internal/app/coordinator/service/nodes.go`
  - After Headscale returns nodes, verify each node's Realm matches expected headscale_user
  - Added defense-in-depth filtering with warning logs for mismatches

- [x] **Task 3: Populate Realm field in ListNodes backend**
  - File: `pkg/meshbackend/tailscale/tailscale_mesh.go`
  - Added `node.Realm = n.GetUser().GetName()` to match GetNode behavior
  - This enables service-layer validation

- [x] **Task 4: Verify Admin API isolation**
  - File: `internal/app/coordinator/controller/admin.go`
  - All admin node operations already use service layer methods
  - Service layer now enforces realm validation for all operations

## Medium Priority

- [x] **Task 5: Add ListNodes unit tests for cross-wonder-net isolation**
  - File: `internal/app/coordinator/service/nodes_test.go`
  - Tests verify filtering works correctly
  - Tests verify empty result when no matching realm

## Low Priority

- [ ] **Task 6: Add ACL policy verification (future)**
  - Consider periodic verification that Headscale ACL matches expected policy
  - Log warnings if drift detected
  - (Out of scope for initial fix)

- [ ] **Task 7: Document isolation architecture**
  - Update CLAUDE.md or create `.ralph/docs/isolation.md`
  - Document the multi-layer isolation approach
  - List all isolation checkpoints in the codebase

## Completed
- [x] Analyzed current isolation architecture
- [x] Identified gaps in ListNodes (missing Realm population)
- [x] Set up Ralph context for autonomous development
- [x] TDD: wrote failing tests first
- [x] Fixed ListNodes in tailscale_mesh.go to populate Realm
- [x] Added defense-in-depth validation in service layer
- [x] All tests pass

## Summary of Changes

1. `pkg/meshbackend/tailscale/tailscale_mesh.go` (lines 140-142):
   - Added `node.Realm = n.GetUser().GetName()` in ListNodes

2. `internal/app/coordinator/service/nodes.go` (lines 35-64):
   - Added realm validation loop that filters nodes by expected HeadscaleUser
   - Logs warnings for any mismatched nodes (defense-in-depth)

3. `internal/app/coordinator/service/nodes_test.go` (new file):
   - 4 test cases covering isolation scenarios
