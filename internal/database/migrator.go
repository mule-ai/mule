package database

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strings"

	_ "github.com/lib/pq"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migrator handles database migrations
type Migrator struct {
	db *sql.DB
}

// NewMigrator creates a new migrator instance
func NewMigrator(db *sql.DB) *Migrator {
	return &Migrator{db: db}
}

// RunMigrations runs all pending migrations from embedded filesystem
func (m *Migrator) RunMigrations() error {
	// Create migrations table if it doesn't exist
	if err := m.createMigrationsTable(); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get list of migration files from embedded filesystem
	files, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return fmt.Errorf("failed to read embedded migrations directory: %w", err)
	}

	// Sort files by name to ensure correct order
	var migrationFiles []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".sql") {
			migrationFiles = append(migrationFiles, file.Name())
		}
	}
	sort.Strings(migrationFiles)

	// Run each migration that hasn't been run yet
	for _, fileName := range migrationFiles {
		if err := m.runEmbeddedMigration(fileName); err != nil {
			return fmt.Errorf("failed to run migration %s: %w", fileName, err)
		}
	}

	log.Println("All migrations completed successfully")
	return nil
}

// RunMigrationsFromRoot runs migrations from the root directory
// Deprecated: Use RunMigrations() instead which uses embedded migrations
func (m *Migrator) RunMigrationsFromRoot() error {
	return m.RunMigrations()
}

// createMigrationsTable creates the table to track migrations
func (m *Migrator) createMigrationsTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`
	_, err := m.db.Exec(query)
	return err
}

// runEmbeddedMigration runs a single migration file from embedded filesystem if it hasn't been run yet
func (m *Migrator) runEmbeddedMigration(fileName string) error {
	// Check if migration has already been run
	var count int
	err := m.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = $1", fileName).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check migration status: %w", err)
	}

	if count > 0 {
		log.Printf("Migration %s already applied, skipping", fileName)
		return nil
	}

	// Read migration file from embedded filesystem
	filePath := "migrations/" + fileName
	content, err := fs.ReadFile(migrationFS, filePath)
	if err != nil {
		return fmt.Errorf("failed to read embedded migration file %s: %w", fileName, err)
	}

	// Start transaction
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute migration
	log.Printf("Running migration: %s", fileName)
	_, err = tx.Exec(string(content))
	if err != nil {
		return fmt.Errorf("failed to execute migration %s: %w", fileName, err)
	}

	// Record migration as applied
	_, err = tx.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", fileName)
	if err != nil {
		return fmt.Errorf("failed to record migration %s: %w", fileName, err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration %s: %w", fileName, err)
	}

	log.Printf("Migration %s applied successfully", fileName)
	return nil
}

// runMigration runs a single migration file if it hasn't been run yet
// Deprecated: Use runEmbeddedMigration instead
func (m *Migrator) runMigration(migrationsDir, fileName string) error {
	return m.runEmbeddedMigration(fileName)
}

// GetMigrationVersion returns the current migration version
func (m *Migrator) GetMigrationVersion() (string, error) {
	var version string
	err := m.db.QueryRow("SELECT version FROM schema_migrations ORDER BY applied_at DESC LIMIT 1").Scan(&version)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return version, nil
}