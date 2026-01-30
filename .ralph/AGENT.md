# Agent Build Instructions

## Project Setup
```bash
go mod download
```

## Run Tests
```bash
make test
# Or run specific tests:
go test -v -run TestFunctionName ./path/to/package
```

## Build
```bash
make build          # Build wonder binary with web UI
make build-go       # Build Go binary only (skip web UI)
```

## Lint & Format
```bash
make check          # Run gofmt, go vet, golangci-lint
```

## Quality Standards
- All tests must pass before committing
- Follow existing code style (no end-of-line comments)
- Use English for all code, comments, and identifiers

## Git Workflow
- Commit format: `type(scope): description`
- Types: feat, fix, docs, test, refactor, chore
- Make atomic commits
- Example: `fix(isolation): add realm validation to ListNodes`

## Key Files for Issue #84

### Isolation Logic
- `internal/app/coordinator/service/nodes.go` - Node operations with realm checks
- `internal/app/coordinator/service/wondernet.go` - Wonder-net resolution
- `pkg/meshbackend/tailscale/tailscale_mesh.go` - Headscale backend
- `pkg/headscale/acl.go` - ACL policy generation

### Middleware & Controllers
- `internal/app/coordinator/server.go` - Auth middleware
- `internal/app/coordinator/controller/nodes.go` - Node API handlers
- `internal/app/coordinator/controller/admin.go` - Admin API handlers

### Database
- `internal/app/coordinator/database/goose/001_init.sql` - Schema
- `internal/app/coordinator/database/sqlc/` - Generated queries

## Completion Checklist
- [ ] All tasks in fix_plan.md complete
- [ ] Tests passing (`make test`)
- [ ] Lint passing (`make check`)
- [ ] Code reviewed
