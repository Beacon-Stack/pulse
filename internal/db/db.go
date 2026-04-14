package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/beacon-stack/pulse/internal/config"
)

// DB wraps the underlying sql.DB and tracks which driver is in use.
type DB struct {
	SQL    *sql.DB
	Driver string
}

// Open opens a database connection based on the provided configuration.
func Open(cfg config.DatabaseConfig) (*DB, error) {
	switch cfg.Driver {
	case "postgres", "":
		return openPostgres(cfg.DSN.Value())
	default:
		return nil, fmt.Errorf("unsupported database driver: %q", cfg.Driver)
	}
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.SQL.Close()
}

func openPostgres(dsn string) (*DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("postgres DSN must not be empty (set PULSE_DATABASE_DSN)")
	}

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening postgres database: %w", err)
	}

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("pinging postgres database: %w", err)
	}

	return &DB{SQL: sqlDB, Driver: "postgres"}, nil
}
