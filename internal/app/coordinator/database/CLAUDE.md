# Database Package

## Overview

This package manages database connections, migrations, and query generation for the coordinator service. It supports both SQLite and PostgreSQL backends.

## Directory Structure

```
database/
├── manager.go          # Database connection and migration management
├── goose/              # Migration files (goose format)
│   └── 001_init.sql    # Initial schema (all tables)
└── sqlc/               # Query definitions and generated code
    ├── *.sql           # Query definitions
    └── *.generated.go  # Generated Go code (do not edit)
```

## Tools

- **Migrations**: [goose](https://github.com/pressly/goose) - Embedded migrations run automatically on startup
- **Query Generation**: [sqlc](https://sqlc.dev/) - Type-safe Go code from SQL queries

## Development Workflow

### Adding or Modifying Schema

During development (pre-1.0.0), modify the schema directly in `goose/001_init.sql` rather than creating incremental migration files. This keeps the schema definition in one place and avoids migration complexity during rapid iteration.

After modifying the schema, regenerate sqlc code:

```bash
sqlc generate
```

### Adding Queries

1. Create or edit a `.sql` file in `sqlc/` (e.g., `users.sql`)
2. Write queries using sqlc annotations:
   ```sql
   -- name: GetUserByID :one
   SELECT * FROM users WHERE id = ?;
   ```
3. Regenerate: `sqlc generate`

## Configuration

The sqlc configuration is in the project root `sqlc.yaml`. Generated files use the `.generated` suffix and should not be manually edited.

## Supported Drivers

- `sqlite3` - SQLite (default, uses single connection to avoid locking)
- `pgx` - PostgreSQL (connection pooling enabled)

