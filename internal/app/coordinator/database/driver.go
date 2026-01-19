package database

import (
	"fmt"
	"strings"
)

func ParseDriver(raw string) (Driver, error) {
	if raw == "" {
		return DriverSQLite, nil
	}

	switch strings.ToLower(raw) {
	case "sqlite", "sqlite3":
		return DriverSQLite, nil
	case "postgres", "postgresql", "pgsql", "pgx":
		return DriverPostgres, nil
	default:
		return "", fmt.Errorf("unsupported database driver: %s", raw)
	}
}
