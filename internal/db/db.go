package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/beacon-media/pulse/internal/config"
)

// DB wraps the underlying sql.DB and tracks which driver is in use.
type DB struct {
	SQL    *sql.DB
	Driver string
}

// Open opens a database connection based on the provided configuration.
func Open(cfg config.DatabaseConfig) (*DB, error) {
	switch cfg.Driver {
	case "sqlite", "":
		return openSQLite(cfg.Path)
	default:
		return nil, fmt.Errorf("unsupported database driver: %q (must be sqlite)", cfg.Driver)
	}
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.SQL.Close()
}

func openSQLite(path string) (*DB, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite path must not be empty")
	}

	if err := ensureDir(path); err != nil {
		return nil, fmt.Errorf("creating database directory: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_synchronous=NORMAL&_foreign_keys=ON&_busy_timeout=5000", path)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}

	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("pinging sqlite database: %w", err)
	}

	return &DB{SQL: sqlDB, Driver: "sqlite"}, nil
}

func ensureDir(filePath string) error {
	dir := filepath.Dir(filePath)
	return os.MkdirAll(dir, 0o755)
}
