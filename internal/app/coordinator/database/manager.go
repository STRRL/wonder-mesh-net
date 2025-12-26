package database

import (
	"database/sql"
	"embed"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pressly/goose/v3"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/database/sqlc"
)

//go:embed goose/*.sql
var embedMigrations embed.FS

// Driver represents a supported database driver
type Driver string

const (
	DriverSQLite   Driver = "sqlite3"
	DriverPostgres Driver = "pgx"
)

// Config holds database connection configuration
type Config struct {
	Driver Driver
	DSN    string
}

// Manager handles database connections and migrations
type Manager struct {
	db      *sql.DB
	queries *sqlc.Queries
}

// NewManager creates a new database manager and runs migrations.
func NewManager(cfg Config) (*Manager, error) {
	db, err := sql.Open(string(cfg.Driver), cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	configureConnectionPool(db, cfg.Driver)

	if err := runMigrations(db, cfg.Driver); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &Manager{
		db:      db,
		queries: sqlc.New(db),
	}, nil
}

func configureConnectionPool(db *sql.DB, driver Driver) {
	switch driver {
	case DriverSQLite:
		// SQLite does not handle multiple concurrent writers well.
		// Setting MaxOpenConns to 1 prevents "database is locked" errors.
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		db.SetConnMaxLifetime(time.Hour)
	case DriverPostgres:
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(5 * time.Minute)
	}
}

func runMigrations(db *sql.DB, driver Driver) error {
	goose.SetBaseFS(embedMigrations)

	dialect := gooseDialect(driver)
	if err := goose.SetDialect(dialect); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}

	if err := goose.Up(db, "goose"); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	return nil
}

func gooseDialect(driver Driver) string {
	switch driver {
	case DriverPostgres:
		return "postgres"
	default:
		return "sqlite3"
	}
}

// Queries returns the sqlc queries instance
func (m *Manager) Queries() *sqlc.Queries {
	return m.queries
}

// DB returns the underlying database connection
func (m *Manager) DB() *sql.DB {
	return m.db
}

// Close closes the database connection
func (m *Manager) Close() error {
	return m.db.Close()
}
