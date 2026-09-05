package database

import (
	"database/sql"
	"log"
)

// CloseRows closes sql.Rows, logging any close error. Close errors are
// typically non-critical (e.g. connection already closed).
func CloseRows(rows *sql.Rows) {
	if rows != nil {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Error closing rows: %v", closeErr)
		}
	}
}

// CloseDB closes sql.DB, logging any close error.
func CloseDB(db *sql.DB) {
	if db != nil {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("Error closing database: %v", closeErr)
		}
	}
}

// CloseStmt closes sql.Stmt, logging any close error.
func CloseStmt(stmt *sql.Stmt) {
	if stmt != nil {
		if closeErr := stmt.Close(); closeErr != nil {
			log.Printf("Error closing statement: %v", closeErr)
		}
	}
}
