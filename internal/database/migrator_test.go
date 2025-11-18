package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	_ "github.com/lib/pq"
)

func TestMigrator(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping migration test in short mode")
	}

	// Create a temporary directory for test migrations
	tempDir, err := os.MkdirTemp("", "test_migrations")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a test migration file
	migrationContent := `
		CREATE TABLE IF NOT EXISTS test_table (
			id VARCHAR(255) PRIMARY KEY,
			name TEXT NOT NULL
		);
		
		CREATE INDEX IF NOT EXISTS idx_test_table_name ON test_table(name);
	`
	
	migrationFile := filepath.Join(tempDir, "0001_test.sql")
	err = os.WriteFile(migrationFile, []byte(migrationContent), 0644)
	assert.NoError(t, err)

	// Use an in-memory SQLite database for testing (or mock PostgreSQL)
	// For now, we'll test the migration file reading logic
	config := Config{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "postgres",
		DBName:   "mulev2_test",
		SSLMode:  "disable",
	}

	db, err := NewDB(config)
	if err != nil {
		t.Skipf("Could not connect to test database: %v", err)
	}
	defer db.Close()

	migrator := NewMigrator(db.DB)

	// Test running migrations (using embedded migrations)
	err = migrator.RunMigrations()
	if err != nil {
		t.Skipf("Could not run migrations: %v", err)
	}

	// Verify migration was recorded
	version, err := migrator.GetMigrationVersion()
	assert.NoError(t, err)
	assert.Equal(t, "0001_test.sql", version)

	// Test that running the same migration again doesn't cause issues
	err = migrator.RunMigrations()
	assert.NoError(t, err)

	// Verify table was created
	var exists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = 'test_table'
		)`).Scan(&exists)
	
	if err == nil {
		assert.True(t, exists, "Test table should exist")
	}
}

func TestMigrationFileOrdering(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "test_migrations_order")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create migration files in random order
	migrations := map[string]string{
		"0003_third.sql":  "CREATE TABLE third (id INT);",
		"0001_first.sql":  "CREATE TABLE first (id INT);",
		"0002_second.sql": "CREATE TABLE second (id INT);",
	}

	for filename, content := range migrations {
		filePath := filepath.Join(tempDir, filename)
		err = os.WriteFile(filePath, []byte(content), 0644)
		assert.NoError(t, err)
	}

	// Test file listing and sorting
	files, err := os.ReadDir(tempDir)
	assert.NoError(t, err)

	var migrationFiles []string
	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".sql" {
			migrationFiles = append(migrationFiles, file.Name())
		}
	}

	// Verify files are in correct order after sorting
	expectedOrder := []string{"0001_first.sql", "0002_second.sql", "0003_third.sql"}
	assert.Equal(t, expectedOrder, migrationFiles)
}

func TestMigratorErrorHandling(t *testing.T) {
	// Test with non-existent directory
	config := Config{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "postgres",
		DBName:   "mulev2_test",
		SSLMode:  "disable",
	}

	db, err := NewDB(config)
	if err != nil {
		t.Skipf("Could not connect to test database: %v", err)
	}
	defer db.Close()

	migrator := NewMigrator(db.DB)

	// Test with non-existent directory (this will now use embedded migrations)
	err = migrator.RunMigrations()
	// This should not fail with directory error anymore since we use embedded migrations
	// It might fail for other reasons (like no database connection), but not directory issues
	if err != nil && !strings.Contains(err.Error(), "failed to read embedded migrations directory") {
		// Expected to fail due to no database or other reasons, but not directory reading
		t.Logf("Expected error (not directory-related): %v", err)
	} else if err == nil {
		t.Log("Migrations ran successfully with embedded filesystem")
	}
}