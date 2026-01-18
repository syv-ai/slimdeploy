package db

import (
	"fmt"
)

// Migration represents a database migration
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// migrations is the list of all database migrations
var migrations = []Migration{
	{
		Version: 1,
		Name:    "create_projects_table",
		SQL: `
			CREATE TABLE IF NOT EXISTS projects (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL UNIQUE,
				git_url TEXT NOT NULL DEFAULT '',
				branch TEXT NOT NULL DEFAULT 'main',
				deploy_type TEXT NOT NULL DEFAULT 'image',
				image TEXT NOT NULL DEFAULT '',
				domain TEXT NOT NULL DEFAULT '',
				use_subdomain INTEGER NOT NULL DEFAULT 0,
				port INTEGER NOT NULL DEFAULT 80,
				env_vars TEXT NOT NULL DEFAULT '{}',
				auto_deploy INTEGER NOT NULL DEFAULT 0,
				last_commit TEXT NOT NULL DEFAULT '',
				status TEXT NOT NULL DEFAULT 'pending',
				status_msg TEXT NOT NULL DEFAULT '',
				container_ids TEXT NOT NULL DEFAULT '[]',
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
			);

			CREATE INDEX IF NOT EXISTS idx_projects_name ON projects(name);
			CREATE INDEX IF NOT EXISTS idx_projects_status ON projects(status);
		`,
	},
	{
		Version: 2,
		Name:    "create_sessions_table",
		SQL: `
			CREATE TABLE IF NOT EXISTS sessions (
				token TEXT PRIMARY KEY,
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				expires_at DATETIME NOT NULL
			);

			CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
		`,
	},
}

// Migrate runs all pending migrations
func (db *DB) Migrate() error {
	// Create migrations table if not exists
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get current version
	var currentVersion int
	err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("failed to get current schema version: %w", err)
	}

	// Apply pending migrations
	for _, m := range migrations {
		if m.Version <= currentVersion {
			continue
		}

		// Start transaction
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %d: %w", m.Version, err)
		}

		// Execute migration
		if _, err := tx.Exec(m.SQL); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to execute migration %d (%s): %w", m.Version, m.Name, err)
		}

		// Record migration
		if _, err := tx.Exec("INSERT INTO schema_migrations (version, name) VALUES (?, ?)", m.Version, m.Name); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %d: %w", m.Version, err)
		}

		// Commit transaction
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", m.Version, err)
		}

		fmt.Printf("Applied migration %d: %s\n", m.Version, m.Name)
	}

	return nil
}
