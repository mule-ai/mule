package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mule-ai/mule/internal/api"
	internaldb "github.com/mule-ai/mule/internal/database"
)

// tenancyDB recreates a dedicated mule_tenancy database, runs all
// migrations, and returns a connected *internaldb.DB. Skips when
// PostgreSQL (postgres:postgres@localhost:5432) is unavailable, the
// same contract as internal/database/db_test.go.
func tenancyDB(t *testing.T) *internaldb.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping tenancy test that requires PostgreSQL")
	}

	admin, err := sql.Open("postgres", "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable")
	if err != nil {
		t.Skipf("could not open admin connection: %v", err)
	}
	defer admin.Close()
	if err := admin.Ping(); err != nil {
		t.Skipf("could not connect to test PostgreSQL: %v", err)
	}

	if _, err := admin.Exec(`DROP DATABASE IF EXISTS mule_tenancy WITH (FORCE)`); err != nil {
		t.Skipf("could not drop mule_tenancy: %v", err)
	}
	if _, err := admin.Exec(`CREATE DATABASE mule_tenancy`); err != nil {
		t.Skipf("could not create mule_tenancy: %v", err)
	}

	db, err := internaldb.NewDB(internaldb.Config{
		Host: "localhost", Port: 5432, User: "postgres",
		Password: "postgres", DBName: "mule_tenancy", SSLMode: "disable",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.InitSchema())
	return db
}

// tenancyKeyHash is the stored form of a presented sk_mule_ key.
func tenancyKeyHash(t *testing.T, key string) string {
	t.Helper()
	// Mirrors api.normalizeAPIKey: strip the prefix exactly once.
	if len(key) > 8 && key[:8] == "sk_mule_" {
		key = key[8:]
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// insertKeyForUser creates an API key directly in the DB for an
// existing user (there is no key-minting endpoint in scope) and
// returns the full key.
func insertKeyForUser(t *testing.T, db *internaldb.DB, keyID, userID string, restricted bool, expiresAt time.Time) string {
	t.Helper()
	key := "sk_mule_" + keyID
	var exp interface{}
	if !expiresAt.IsZero() {
		exp = expiresAt
	}
	if _, err := db.ExecContext(context.Background(),
		`INSERT INTO api_keys (id, user_id, key_hash, key_prefix, restricted, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		keyID, userID, tenancyKeyHash(t, key), key[:12], restricted, exp); err != nil {
		t.Fatalf("insert api key %s: %v", keyID, err)
	}
	return key
}

// insertTestUser creates a non-admin user plus an API key directly in
// the DB and returns the full key.
func insertTestUser(t *testing.T, db *internaldb.DB, id, email string, restricted bool, expiresAt time.Time) string {
	t.Helper()
	if _, err := db.ExecContext(context.Background(),
		`INSERT INTO users (id, email, name, password_hash, role) VALUES ($1, $2, $3, 'placeholder', 'user')`,
		id, email, email); err != nil {
		t.Fatalf("insert user %s: %v", id, err)
	}
	return insertKeyForUser(t, db, id, id, restricted, expiresAt)
}

// tenancyRouter builds an API router wired with the auth middleware,
// the same shape as main() in server.go.
func tenancyRouter(t *testing.T, db *internaldb.DB) http.Handler {
	t.Helper()
	handler := NewAPIHandler(db)
	router := mux.NewRouter()
	router.Use(api.AuthMiddleware(db.DB))
	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("OK"))
	})
	router.HandleFunc("/api/v1/workflows", handler.listWorkflowsHandler).Methods(http.MethodGet)
	router.HandleFunc("/api/v1/workflows", handler.createWorkflowHandler).Methods(http.MethodPost)
	router.HandleFunc("/api/v1/workflows/{id}", handler.getWorkflowHandler).Methods(http.MethodGet)
	router.HandleFunc("/api/v1/agents", handler.createAgentHandler).Methods(http.MethodPost)
	router.HandleFunc("/api/v1/jobs", handler.listJobsHandler).Methods(http.MethodGet)
	router.HandleFunc("/api/v1/jobs", handler.createJobHandler).Methods(http.MethodPost)
	router.HandleFunc("/api/v1/jobs/{id}", handler.getJobHandler).Methods(http.MethodGet)
	router.HandleFunc("/api/v1/jobs/{id}", handler.cancelJobHandler).Methods(http.MethodDelete)
	router.HandleFunc("/api/v1/providers", handler.createProviderHandler).Methods(http.MethodPost)
	// Wakes (H3.3): owner-scoped CRUD + external fire ingress.
	router.HandleFunc("/api/v1/wakes", handler.listWakesHandler).Methods(http.MethodGet)
	router.HandleFunc("/api/v1/wakes", handler.createWakesHandler).Methods(http.MethodPost)
	router.HandleFunc("/api/v1/wakes/fire", handler.fireWakesHandler).Methods(http.MethodPost)
	router.HandleFunc("/api/v1/wakes/{id}", handler.getWakesHandler).Methods(http.MethodGet)
	router.HandleFunc("/api/v1/wakes/{id}", handler.patchWakesHandler).Methods(http.MethodPatch)
	router.HandleFunc("/api/v1/wakes/{id}", handler.deleteWakesHandler).Methods(http.MethodDelete)
	router.HandleFunc("/api/v1/wakes/{id}/log", handler.wakeLogHandler).Methods(http.MethodGet)
	// Policy engine (H3.5): owner-scoped CRUD + evaluate/proposals.
	router.HandleFunc("/api/v1/policies", handler.listPoliciesHandler).Methods(http.MethodGet)
	router.HandleFunc("/api/v1/policies", handler.createPoliciesHandler).Methods(http.MethodPost)
	router.HandleFunc("/api/v1/policies/evaluate", handler.evaluatePolicyHandler).Methods(http.MethodPost)
	router.HandleFunc("/api/v1/policies/memo", handler.memoPolicyHandler).Methods(http.MethodPost)
	router.HandleFunc("/api/v1/policies/proposals", handler.createProposalHandler).Methods(http.MethodPost)
	router.HandleFunc("/api/v1/policies/proposals/{id}/approve", handler.approveProposalHandler).Methods(http.MethodPost)
	router.HandleFunc("/api/v1/policies/{id}", handler.getPolicyHandler).Methods(http.MethodGet)
	router.HandleFunc("/api/v1/policies/{id}", handler.patchPolicyHandler).Methods(http.MethodPatch)
	router.HandleFunc("/api/v1/policies/{id}", handler.deletePolicyHandler).Methods(http.MethodDelete)
	return router
}

func tenancyRequest(t *testing.T, router http.Handler, method, path, bearerKey string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var rd *bytes.Reader
	if body == nil {
		rd = bytes.NewReader(nil)
	} else {
		rd = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	if bearerKey != "" {
		req.Header.Set("Authorization", "Bearer "+bearerKey)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestTenancyBootstrap creates exactly one admin + one key, seeds the
// documented env vars, and is a no-op on the second call.
func TestTenancyBootstrap(t *testing.T) {
	db := tenancyDB(t)
	ctx := context.Background()

	t.Setenv("MULE_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("MULE_ADMIN_PASSWORD", "bootstrap-pass-1")
	t.Setenv("MULE_ADMIN_API_KEY", "sk_mule_adminkey0000000000000000000000")

	res, err := api.BootstrapAdmin(ctx, db.DB)
	require.NoError(t, err)
	assert.True(t, res.Created, "first boot must create the admin")
	assert.Equal(t, api.DefaultAdminID, res.AdminID, "default admin id must be the documented UUID")

	// Exactly one user, exactly one key.
	var users, keys int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&users))
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_keys`).Scan(&keys))
	assert.Equal(t, 1, users)
	assert.Equal(t, 1, keys)

	var email, role, passwordHash string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT email, role, password_hash FROM users WHERE id = $1`, api.DefaultAdminID,
	).Scan(&email, &role, &passwordHash))
	assert.Equal(t, "admin@example.com", email)
	assert.Equal(t, "admin", role)
	assert.Contains(t, passwordHash, "$argon2id$", "password must be stored as an argon2 PHC hash")

	var keyHash, keyPrefix string
	var restricted bool
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT key_hash, key_prefix, restricted FROM api_keys`).
		Scan(&keyHash, &keyPrefix, &restricted))
	assert.Equal(t, tenancyKeyHash(t, "sk_mule_adminkey0000000000000000000000"), keyHash)
	assert.Equal(t, "sk_mule_admi", keyPrefix)
	assert.False(t, restricted)

	// Idempotent on the second boot: no new rows.
	res2, err := api.BootstrapAdmin(ctx, db.DB)
	require.NoError(t, err)
	assert.False(t, res2.Created, "second boot must be a no-op")
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&users))
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_keys`).Scan(&keys))
	assert.Equal(t, 1, users)
	assert.Equal(t, 1, keys)
}

// TestTenancyBootstrapGeneratedCreds exercises the no-env path: a
// random 24-byte password and a generated sk_mule_ key (both logged
// once at boot by the real daemon).
func TestTenancyBootstrapGeneratedCreds(t *testing.T) {
	db := tenancyDB(t)
	ctx := context.Background()

	t.Setenv("MULE_ADMIN_EMAIL", "")
	t.Setenv("MULE_ADMIN_PASSWORD", "")
	t.Setenv("MULE_ADMIN_API_KEY", "")
	t.Setenv("MULE_ADMIN_ID", "")

	res, err := api.BootstrapAdmin(ctx, db.DB)
	require.NoError(t, err)
	assert.True(t, res.Created)
	assert.Equal(t, "admin@localhost", res.AdminEmail)
	require.NotEmpty(t, res.AdminKey)
	assert.True(t, len(res.AdminKey) == 8+64, "generated key must be sk_mule_ + 32 random bytes hex, got len %d", len(res.AdminKey))
	assert.Equal(t, api.APIKeyPrefix, res.AdminKey[:len(api.APIKeyPrefix)])

	// The generated key must verify through the stored hash.
	var count int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM api_keys WHERE key_hash = $1`, tenancyKeyHash(t, res.AdminKey)).Scan(&count))
	assert.Equal(t, 1, count, "generated key must match its stored hash")

	// Pre-tenancy owner backfill: the owner_id columns exist and the
	// users/api_keys tables are present with the documented shape.
	for _, table := range []string{"users", "api_keys", "agents", "workflows", "jobs", "skills", "providers"} {
		var hasOwner bool
		if table == "users" || table == "api_keys" {
			continue
		}
		err := db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = $1 AND column_name = 'owner_id')`,
			table).Scan(&hasOwner)
		require.NoError(t, err, "check owner_id on %s", table)
		assert.True(t, hasOwner, "%s must have owner_id after migration 0012", table)
	}
}

// requireAuthMode skips when the whole test process runs under
// MULE_NO_AUTH=1: in that mode the middleware is a pass-through with
// a synthetic admin, so auth-enforcement expectations do not hold.
// The pre-existing (non-tenancy) cmd/api suite is the MULE_NO_AUTH
// regression gate; these tests exercise the enforcement itself.
func requireAuthMode(t *testing.T) {
	t.Helper()
	if os.Getenv("MULE_NO_AUTH") == "1" {
		t.Skip("auth-enforcement test: skipped under MULE_NO_AUTH=1")
	}
}

// TestTenancyIsolation: two users cannot see each other's workflows
// or jobs; unauthenticated requests are rejected; health stays open.
func TestTenancyIsolation(t *testing.T) {
	requireAuthMode(t)
	db := tenancyDB(t)

	t.Setenv("MULE_ADMIN_EMAIL", "a@example.com")
	t.Setenv("MULE_ADMIN_PASSWORD", "a-pass")
	t.Setenv("MULE_ADMIN_API_KEY", "sk_mule_0000adminkey000000000000000000")
	t.Setenv("MULE_ADMIN_ID", "")
	_, err := api.BootstrapAdmin(context.Background(), db.DB)
	require.NoError(t, err)
	adminKey := "sk_mule_0000adminkey000000000000000000"

	userB := insertTestUser(t, db, "user-b", "b@example.com", false, time.Time{})
	router := tenancyRouter(t, db)

	// No key -> 401 on protected routes; health stays open.
	w := tenancyRequest(t, router, http.MethodGet, "/api/v1/workflows", "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "error")

	w = tenancyRequest(t, router, http.MethodGet, "/health", "", nil)
	assert.Equal(t, http.StatusOK, w.Code, "health endpoint must stay unauthenticated")

	// Unknown key -> 401.
	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/workflows", "sk_mule_nope", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Admin A creates a workflow.
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/workflows", adminKey,
		[]byte(`{"name":"alpha-workflow","description":"A's workflow"}`))
	require.Equal(t, http.StatusCreated, w.Code, "admin create workflow: %s", w.Body.String())
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.NotEmpty(t, created.ID)

	// The workflow row is owned by admin A.
	var ownerID string
	require.NoError(t, db.QueryRow("SELECT owner_id FROM workflows WHERE id = $1", created.ID).Scan(&ownerID))
	assert.Equal(t, api.DefaultAdminID, ownerID)

	// B cannot see A's workflow: empty list, 404 by id.
	var workflows []map[string]interface{}
	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/workflows", userB, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &workflows))
	assert.Empty(t, workflows, "B's workflow list must not contain A's workflows: %s", w.Body.String())

	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/workflows/"+created.ID, userB, nil)
	assert.Equal(t, http.StatusNotFound, w.Code, "B must not see A's workflow by id")

	// A still sees it.
	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/workflows/"+created.ID, adminKey, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// A submits a job on their workflow.
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/jobs", adminKey,
		[]byte(fmt.Sprintf(`{"workflow_id":%q,"input_data":{"prompt":"hi"}}`, created.ID)))
	require.Equal(t, http.StatusCreated, w.Code, "admin create job: %s", w.Body.String())
	var jobResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &jobResp))
	jobID := jobResp.Data.ID
	require.NotEmpty(t, jobID)

	// A's job is invisible to B: list + get by id.
	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/jobs", userB, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"jobs":[]`, "B's job list must be empty: %s", w.Body.String())

	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/jobs/"+jobID, userB, nil)
	assert.Equal(t, http.StatusNotFound, w.Code, "B must not see A's job by id")

	// A sees the job.
	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/jobs/"+jobID, adminKey, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// B cannot create a job against A's workflow (scoped lookup fails).
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/jobs", userB,
		[]byte(fmt.Sprintf(`{"workflow_id":%q,"input_data":{"prompt":"hi"}}`, created.ID)))
	assert.NotEqual(t, http.StatusCreated, w.Code, "B must not be able to run A's workflow")
}

// TestTenancyRestrictedKey: a restricted key can read but cannot
// write agents/workflows/jobs/policies; the admin role does not
// bypass the restricted flag (it is a property of the key).
func TestTenancyRestrictedKey(t *testing.T) {
	requireAuthMode(t)
	db := tenancyDB(t)

	t.Setenv("MULE_ADMIN_EMAIL", "a@example.com")
	t.Setenv("MULE_ADMIN_PASSWORD", "a-pass")
	t.Setenv("MULE_ADMIN_API_KEY", "sk_mule_0000adminkey000000000000000000")
	t.Setenv("MULE_ADMIN_ID", "")
	_, err := api.BootstrapAdmin(context.Background(), db.DB)
	require.NoError(t, err)

	restrictedB := insertTestUser(t, db, "user-r", "r@example.com", true, time.Time{})
	// A restricted key belonging to the ADMIN user: admin + restricted
	// must still be denied the write routes.
	restrictedAdmin := insertKeyForUser(t, db, "admin-rkey", api.DefaultAdminID, true, time.Time{})
	router := tenancyRouter(t, db)

	// Reads are allowed for restricted keys.
	w := tenancyRequest(t, router, http.MethodGet, "/api/v1/workflows", restrictedB, nil)
	assert.Equal(t, http.StatusOK, w.Code, "restricted key must be able to read: %s", w.Body.String())

	// Writes on restricted classes are denied.
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/workflows", restrictedB,
		[]byte(`{"name":"nope","description":"x"}`))
	assert.Equal(t, http.StatusForbidden, w.Code, "restricted key must not create workflows: %s", w.Body.String())
	assert.Contains(t, w.Body.String(), "restricted")

	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/agents", restrictedB,
		[]byte(`{"name":"nope","provider_id":"x"}`))
	assert.Equal(t, http.StatusForbidden, w.Code)

	w = tenancyRequest(t, router, http.MethodDelete, "/api/v1/jobs/some-job", restrictedB, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// Writes outside the restricted classes are still allowed.
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/providers", restrictedB,
		[]byte(`{"name":"r-provider","api_base_url":"https://x.example","api_key_encrypted":"k"}`))
	assert.Equal(t, http.StatusCreated, w.Code, "restricted key may create providers: %s", w.Body.String())

	// Admin role does NOT bypass the restricted flag.
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/workflows", restrictedAdmin,
		[]byte(`{"name":"nope","description":"x"}`))
	assert.Equal(t, http.StatusForbidden, w.Code, "admin+restricted must still be denied writes")

	// ...while the unrestricted admin key can write.
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/workflows", "sk_mule_0000adminkey000000000000000000",
		[]byte(`{"name":"ok-wf","description":"x"}`))
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}

// TestTenancyExpiredKey: an expired key is rejected with 401.
func TestTenancyExpiredKey(t *testing.T) {
	requireAuthMode(t)
	db := tenancyDB(t)

	t.Setenv("MULE_ADMIN_EMAIL", "a@example.com")
	t.Setenv("MULE_ADMIN_PASSWORD", "a-pass")
	t.Setenv("MULE_ADMIN_API_KEY", "sk_mule_0000adminkey000000000000000000")
	t.Setenv("MULE_ADMIN_ID", "")
	_, err := api.BootstrapAdmin(context.Background(), db.DB)
	require.NoError(t, err)

	expiresAt := time.Now().UTC().Add(-time.Hour)
	expiredKey := insertTestUser(t, db, "user-e", "e@example.com", false, expiresAt)
	router := tenancyRouter(t, db)

	w := tenancyRequest(t, router, http.MethodGet, "/api/v1/workflows", expiredKey, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code, "expired key must 401")
	assert.Contains(t, w.Body.String(), "expired")
}

// TestTenancyNoAuthMode: MULE_NO_AUTH=1 makes the middleware a
// pass-through with a synthetic admin context.
func TestTenancyNoAuthMode(t *testing.T) {
	db := tenancyDB(t)

	t.Setenv("MULE_NO_AUTH", "1")
	router := tenancyRouter(t, db)

	// No key needed; synthetic admin sees and creates.
	w := tenancyRequest(t, router, http.MethodGet, "/api/v1/workflows", "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/workflows", "",
		[]byte(`{"name":"no-auth-wf","description":"created under MULE_NO_AUTH"}`))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var ownerID string
	var id string
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT id, owner_id FROM workflows WHERE name = 'no-auth-wf'`).Scan(&id, &ownerID))
	assert.Equal(t, api.DefaultAdminID, ownerID, "synthetic admin must own created rows")

	// Health is still open.
	w = tenancyRequest(t, router, http.MethodGet, "/health", "", nil)
	assert.Equal(t, http.StatusOK, w.Code)
}
