package manager

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/mule-ai/mule/internal/database"
	dbmodels "github.com/mule-ai/mule/pkg/database"
)

// ProviderManager handles provider operations
type ProviderManager struct {
	db     *database.DB
	secret []byte
}

// NewProviderManager creates a new provider manager
func NewProviderManager(db *database.DB, secret []byte) *ProviderManager {
	return &ProviderManager{
		db:     db,
		secret: secret,
	}
}

// CreateProvider creates a new provider
func (pm *ProviderManager) CreateProvider(ctx context.Context, name, apiBaseURL, apiKey string) (*dbmodels.Provider, error) {
	id := uuid.New().String()
	encryptedKey, err := pm.encryptAPIKey(apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt API key: %w", err)
	}

	now := time.Now()
	provider := &dbmodels.Provider{
		ID:              id,
		Name:            name,
		APIBaseURL:      apiBaseURL,
		APIKeyEncrypted: encryptedKey,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	query := `INSERT INTO providers (id, name, api_base_url, api_key_encrypted, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)`
	_, err = pm.db.ExecContext(ctx, query, provider.ID, provider.Name, provider.APIBaseURL, provider.APIKeyEncrypted, provider.CreatedAt, provider.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert provider: %w", err)
	}

	return provider, nil
}

// GetProvider retrieves a provider by ID
func (pm *ProviderManager) GetProvider(ctx context.Context, id string) (*dbmodels.Provider, error) {
	query := `SELECT id, name, api_base_url, api_key_encrypted, created_at, updated_at FROM providers WHERE id = $1`
	provider := &dbmodels.Provider{}
	err := pm.db.QueryRowContext(ctx, query, id).Scan(
		&provider.ID,
		&provider.Name,
		&provider.APIBaseURL,
		&provider.APIKeyEncrypted,
		&provider.CreatedAt,
		&provider.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("provider not found: %s", id)
		}
		return nil, fmt.Errorf("failed to query provider: %w", err)
	}

	return provider, nil
}

// ListProviders lists all providers
func (pm *ProviderManager) ListProviders(ctx context.Context) ([]*dbmodels.Provider, error) {
	query := `SELECT id, name, api_base_url, api_key_encrypted, created_at, updated_at FROM providers ORDER BY created_at DESC`
	rows, err := pm.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query providers: %w", err)
	}
	defer rows.Close()

	var providers []*dbmodels.Provider
	for rows.Next() {
		provider := &dbmodels.Provider{}
		err := rows.Scan(
			&provider.ID,
			&provider.Name,
			&provider.APIBaseURL,
			&provider.APIKeyEncrypted,
			&provider.CreatedAt,
			&provider.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan provider: %w", err)
		}
		providers = append(providers, provider)
	}

	return providers, nil
}

// UpdateProvider updates a provider
func (pm *ProviderManager) UpdateProvider(ctx context.Context, id, name, apiBaseURL, apiKey string) (*dbmodels.Provider, error) {
	provider, err := pm.GetProvider(ctx, id)
	if err != nil {
		return nil, err
	}

	provider.Name = name
	provider.APIBaseURL = apiBaseURL
	if apiKey != "" {
		encryptedKey, err := pm.encryptAPIKey(apiKey)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt API key: %w", err)
		}
		provider.APIKeyEncrypted = encryptedKey
	}
	provider.UpdatedAt = time.Now()

	query := `UPDATE providers SET name = $1, api_base_url = $2, api_key_encrypted = $3, updated_at = $4 WHERE id = $5`
	_, err = pm.db.ExecContext(ctx, query, provider.Name, provider.APIBaseURL, provider.APIKeyEncrypted, provider.UpdatedAt, provider.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update provider: %w", err)
	}

	return provider, nil
}

// DeleteProvider deletes a provider
func (pm *ProviderManager) DeleteProvider(ctx context.Context, id string) error {
	query := `DELETE FROM providers WHERE id = $1`
	result, err := pm.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete provider: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("provider not found: %s", id)
	}

	return nil
}

// GetDecryptedAPIKey gets the decrypted API key for a provider
func (pm *ProviderManager) GetDecryptedAPIKey(ctx context.Context, id string) (string, error) {
	provider, err := pm.GetProvider(ctx, id)
	if err != nil {
		return "", err
	}

	return pm.decryptAPIKey(provider.APIKeyEncrypted)
}

// encryptAPIKey encrypts an API key using AES-GCM
func (pm *ProviderManager) encryptAPIKey(apiKey string) (string, error) {
	block, err := aes.NewCipher(pm.secret)
	if err != nil {
		return "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// GCM standard nonce size is 12 bytes
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// Use GCM for encryption - this returns ciphertext with authentication tag
	ciphertext := aesgcm.Seal(nil, nonce, []byte(apiKey), nil)

	// Prepend nonce to ciphertext for storage
	result := append(nonce, ciphertext...)

	return hex.EncodeToString(result), nil
}

// decryptAPIKey decrypts an API key using AES-GCM
func (pm *ProviderManager) decryptAPIKey(encryptedKey string) (string, error) {
	ciphertext, err := hex.DecodeString(encryptedKey)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(pm.secret)
	if err != nil {
		return "", err
	}

	// GCM nonce size is 12 bytes
	if len(ciphertext) < 12 {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce := ciphertext[:12]
	ciphertext = ciphertext[12:]

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
