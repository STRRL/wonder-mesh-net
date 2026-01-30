# Ralph Development Instructions

## Project Context
- **Project**: wonder-mesh-net
- **Type**: Go (Backend)
- **Framework**: Cobra CLI + HTTP Server + Headscale Integration

## Current Goal
Fix Issue #84: Tailnet Isolation - Ensure tailnets belonging to different users are properly isolated at all layers.

## Key Principles
1. **Focus on the task list** - Work through fix_plan.md items in priority order
2. **Search before creating** - Always check if similar code exists before writing new code
3. **Test your changes** - Run tests after each significant change
4. **Update documentation** - Keep AGENT.md and README current
5. **Commit regularly** - Make atomic commits with clear messages

## Architecture Context

### Multi-tenancy Model
- Each user gets an isolated Headscale "user" (namespace)
- Wonder-net ID is a random UUID, Headscale username is the same UUID
- API keys are bound to specific wonder-nets via FK

### Current Isolation Layers
1. **Database**: `owner_id` on `wonder_nets` table
2. **Headscale Namespace**: Each wonder-net = unique Headscale user
3. **ACL Rules**: `GenerateWonderNetIsolationPolicy()` creates per-user rules
4. **Node Operations**: `GetNode`/`DeleteNode` check `node.Realm`
5. **HTTP Middleware**: `requireWonderNet` injects context

### Identified Gaps (Issue #84)
1. **ListNodes lacks secondary validation** - relies solely on Headscale User filter
2. **No integration tests** for cross-wonder-net isolation
3. **ACL policy lacks periodic verification**
4. **Admin API paths may bypass isolation checks**

## Constraints
- No Chinese characters in code or comments
- Use `log/slog` for application logging
- Avoid "failed to" prefix in error messages
- Follow existing code patterns in the codebase

## Testing Guidelines
- Write integration tests for isolation scenarios
- Test both positive (allowed) and negative (blocked) access patterns
- Limit test writing to ~20% of total work
- Focus on critical path testing

## Status Reporting

After each work session, output a status block:

```
---RALPH_STATUS---
STATUS: IN_PROGRESS | COMPLETE | BLOCKED
TASKS_COMPLETED_THIS_LOOP: <number>
FILES_MODIFIED: <number>
TESTS_STATUS: PASSING | FAILING | NOT_RUN
WORK_TYPE: IMPLEMENTATION | TESTING | DOCUMENTATION | REFACTORING
EXIT_SIGNAL: false
RECOMMENDATION: <one line summary>
---END_RALPH_STATUS---
```

**EXIT_SIGNAL should be `true` ONLY when:**
- All fix_plan.md items are marked [x]
- All tests pass
- No errors or warnings
- No meaningful work remains
