package database

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

// DB represents the database connection
type DB struct {
	*sql.DB
}

// Config holds database configuration
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// NewDB creates a new database connection
func NewDB(config Config) (*DB, error) {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.User, config.Password, config.DBName, config.SSLMode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{db}, nil
}

// InitSchema initializes the database schema by running migrations
func (db *DB) InitSchema() error {
	// Use the migrator to run migrations from embedded files
	migrator := NewMigrator(db.DB)
	if err := migrator.RunMigrations(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	return nil
}
