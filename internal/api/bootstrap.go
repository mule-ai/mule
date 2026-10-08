package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"

	"golang.org/x/crypto/argon2"
)

// BootstrapAdmin creates the operator's admin user + API key on first
// boot. Idempotent: when any user already exists it is a logged no-op.
//
// Operator flow (docs/TENANCY.md):
//   - Local dev: set MULE_ADMIN_EMAIL / MULE_ADMIN_PASSWORD /
//     MULE_ADMIN_API_KEY in the environment; the generated key is
//     logged once at boot.
//   - Fleet: set the same vars from your secrets store; the key is
//     then seeded verbatim (MULE_ADMIN_API_KEY) and is NOT re-logged.
//
// The admin's id is MULE_ADMIN_ID when set, otherwise DefaultAdminID
// (00000000-0000-4000-8000-000000000001) — the constant migration 0012
// backfills pre-tenancy rows with, so the default admin key reaches
// existing data. When MULE_ADMIN_ID is overridden, pre-tenancy rows
// keep the placeholder owner and the operator migrates them
// explicitly.
type BootstrapResult struct {
	AdminID    string
	AdminKey   string // empty when an admin already existed
	AdminEmail string
	Created    bool
}

// BootstrapAdmin runs the one-time admin bootstrap. It reads the
// MULE_ADMIN_* environment on every call (tests set them per-call).
func BootstrapAdmin(ctx context.Context, db *sql.DB) (*BootstrapResult, error) {
	// Idempotent guard: any existing user means the operator already
	// has an account (created here or out-of-band).
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return nil, fmt.Errorf("bootstrap: count users: %w", err)
	}
	if count > 0 {
		adminID, err := existingAdminID(ctx, db)
		if err != nil {
			return nil, err
		}
		log.Printf("tenancy bootstrap: skipped (users already exist); default admin id %s", DefaultAdminID)
		return &BootstrapResult{AdminID: adminID, Created: false}, nil
	}

	adminID := strings.TrimSpace(os.Getenv("MULE_ADMIN_ID"))
	if adminID == "" {
		adminID = DefaultAdminID
	}
	email := strings.TrimSpace(os.Getenv("MULE_ADMIN_EMAIL"))
	if email == "" {
		email = "admin@localhost"
	}

	password, generatedPassword := "", false
	if p := strings.TrimSpace(os.Getenv("MULE_ADMIN_PASSWORD")); p != "" {
		password = p
	} else {
		var buf [24]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return nil, fmt.Errorf("bootstrap: generate password: %w", err)
		}
		password = hex.EncodeToString(buf[:])
		generatedPassword = true
	}
	passwordHash, err := argon2PHC(password)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: hash password: %w", err)
	}

	apiKey, generatedKey := "", false
	if k := strings.TrimSpace(os.Getenv("MULE_ADMIN_API_KEY")); k != "" {
		apiKey = k
	} else {
		var buf [32]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return nil, fmt.Errorf("bootstrap: generate API key: %w", err)
		}
		apiKey = APIKeyPrefix + hex.EncodeToString(buf[:])
		generatedKey = true
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO users (id, email, name, password_hash, role) VALUES ($1, $2, $3, $4, 'admin')`,
		adminID, email, "Mule Admin", passwordHash,
	); err != nil {
		return nil, fmt.Errorf("bootstrap: insert user: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO api_keys (id, user_id, key_hash, key_prefix, restricted) VALUES ($1, $2, $3, $4, FALSE)`,
		newKeyID(), adminID, apiKeyHash(apiKey), keyPrefix(apiKey),
	); err != nil {
		return nil, fmt.Errorf("bootstrap: insert api key: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("bootstrap: commit: %w", err)
	}

	if generatedPassword {
		// Logged ONCE: this is the only time the generated password
		// appears anywhere. Operators should record it now.
		log.Printf("==============================================================")
		log.Printf("TENANCY BOOTSTRAP: created admin user %q (id %s)", email, adminID)
		log.Printf("TENANCY BOOTSTRAP: MULE_ADMIN_PASSWORD=%s", password)
		log.Printf("TENANCY BOOTSTRAP: record this password now; it is only shown once.")
		log.Printf("==============================================================")
	}
	if generatedKey {
		// Same one-time contract for the generated API key.
		log.Printf("TENANCY BOOTSTRAP: MULE_ADMIN_API_KEY=%s", apiKey)
		log.Printf("TENANCY BOOTSTRAP: record this key now; it is only shown once.")
	} else {
		log.Printf("tenancy bootstrap: seeded admin API key from MULE_ADMIN_API_KEY (prefix %s)", keyPrefix(apiKey))
	}

	return &BootstrapResult{
		AdminID:    adminID,
		AdminKey:   apiKey,
		AdminEmail: email,
		Created:    true,
	}, nil
}

// existingAdminID returns the id of an existing admin user (used when
// bootstrap is skipped) so downstream initialization can still carry a
// principal; falls back to DefaultAdminID.
func existingAdminID(ctx context.Context, db *sql.DB) (string, error) {
	var id string
	err := db.QueryRowContext(ctx, `SELECT id FROM users WHERE role = 'admin' ORDER BY created_at LIMIT 1`).Scan(&id)
	if err == sql.ErrNoRows {
		return DefaultAdminID, nil
	}
	if err != nil {
		return "", fmt.Errorf("bootstrap: look up existing admin: %w", err)
	}
	return id, nil
}

// argon2PHC hashes password with argon2id (default parameters) and
// returns the PHC-encoded string
// ($argon2id$v=19$m=65536,t=2,p=1$<b64 salt>$<b64 hash>), the same
// format forge stores.
func argon2PHC(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	params := struct {
		Time    uint32
		Memory  uint32
		Threads uint8
		KeyLen  uint32
	}{Time: 2, Memory: 64 * 1024, Threads: 1, KeyLen: 32} // argon2id defaults
	hash := argon2.IDKey([]byte(password), salt, params.Time, uint32(params.Memory), params.Threads, params.KeyLen)
	encSalt := base64.RawStdEncoding.EncodeToString(salt)
	encHash := base64.RawStdEncoding.EncodeToString(hash)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		params.Memory, params.Time, params.Threads, encSalt, encHash), nil
}

// newKeyID returns a random 16-byte hex id for api_keys.id.
func newKeyID() string {
	var buf [16]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}

// keyPrefix returns the first 12 characters of the key (forge
// convention: enough to identify, not enough to use).
func keyPrefix(key string) string {
	if len(key) <= 12 {
		return key
	}
	return key[:12]
}
