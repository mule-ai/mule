package database

import (
	"database/sql"
	"log"
)

// CloseRows safely closes sql.Rows and logs any error.
// This is a helper function to consolidate the repeated pattern:
//
//	defer func() {
//		if closeErr := rows.Close(); closeErr != nil {
//			log.Printf("Error closing rows: %v", closeErr)
//		}
//	}()
//
// Usage:
//
//	rows, err := db.QueryContext(ctx, query)
//	if err != nil {
//		return err
//	}
//	defer CloseRows(rows)
func CloseRows(rows *sql.Rows) {
	if rows != nil {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Error closing rows: %v", closeErr)
		}
	}
}

// CloseDB safely closes sql.DB and logs any error.
// Similar helper for database connection cleanup.
func CloseDB(db *sql.DB) {
	if db != nil {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("Error closing database: %v", closeErr)
		}
	}
}

// CloseStmt safely closes sql.Stmt and logs any error.
// Similar helper for statement cleanup.
func CloseStmt(stmt *sql.Stmt) {
	if stmt != nil {
		if closeErr := stmt.Close(); closeErr != nil {
			log.Printf("Error closing statement: %v", closeErr)
		}
	}
}
