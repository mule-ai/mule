package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mule-ai/mule/internal/primitive"
)

// API key and tenancy conventions (docs/TENANCY.md, migration 0012):
//
//   - Keys look like `sk_mule_<hex>`. The DB stores SHA-256 of the key
//     BODY (the hex after the `sk_mule_` prefix); a bare body without
//     the prefix verifies against the same row (mirrors forge's
//     dual-read).
//   - Authentication: `Authorization: Bearer sk_mule_<hex>` on every
//     /api/v1/* and /v1/* route.
//   - MULE_NO_AUTH=1: middleware becomes a pass-through that attaches
//     a synthetic admin principal, so existing local/ranch
//     integrations keep working without keys.

// DefaultAdminID is the well-known user id of the bootstrap admin
// created on first boot when MULE_ADMIN_ID is unset. Migration 0012
// backfills pre-tenancy rows to this id so the default admin key
// reaches existing single-operator data.
const DefaultAdminID = "00000000-0000-4000-8000-000000000001"

// APIKeyPrefix is minted onto every generated API key.
const APIKeyPrefix = "sk_mule_"

// AuthUser is the authenticated caller attached to the request
// context by AuthMiddleware.
type AuthUser struct {
	ID         string
	Email      string
	Role       string
	Restricted bool // this API key carries the restricted flag
	NoAuth     bool // synthetic user injected under MULE_NO_AUTH=1
}

type authCtxKey struct{}

// WithAuthUser attaches au to ctx (tenancy principal included).
func WithAuthUser(ctx context.Context, au AuthUser) context.Context {
	pr := primitive.Principal{UserID: au.ID, Admin: au.Role == "admin" || au.NoAuth}
	ctx = primitive.WithPrincipal(ctx, pr)
	return context.WithValue(ctx, authCtxKey{}, au)
}

// AuthUserFrom extracts the authenticated user carried by the request
// context (set by AuthMiddleware), if any.
func AuthUserFrom(r *http.Request) (AuthUser, bool) {
	au, ok := r.Context().Value(authCtxKey{}).(AuthUser)
	return au, ok
}

// noAuthEnabled reports whether MULE_NO_AUTH=1. Read per request (env
// reads are cheap and this keeps the flag testable via t.Setenv).
func noAuthEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("MULE_NO_AUTH")), "1")
}

// isProtectedPath reports whether the path must be authenticated:
// every /api/v1/* and /v1/* route. /health, /ws, and the frontend
// catch-all are intentionally NOT protected (documented in
// docs/TENANCY.md).
func isProtectedPath(path string) bool {
	return strings.HasPrefix(path, "/api/v1/") || strings.HasPrefix(path, "/v1/")
}

// restrictedWritePrefixes are the route classes a restricted key may
// NOT write to: POST/PUT/PATCH/DELETE on agents, workflows, jobs, and
// (when it lands) policies. A restricted key can still READ and can
// still execute workflows via the /v1/* surface (submission is
// restricted only under /api/v1/jobs* — see docs/TENANCY.md for the
// chosen semantics).
//
// Note: admin role does NOT bypass the restricted flag — restricted
// is a property of the KEY, and admin+restricted still cannot write
// these routes (documented deliberate choice, mirrors forge).
var restrictedWritePrefixes = []string{
	"/api/v1/agents",
	"/api/v1/workflows",
	"/api/v1/jobs",
	"/api/v1/policies",
}

// isRestrictedWrite reports whether (method, path) is a write on a
// restricted route class.
func isRestrictedWrite(method, path string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return false
	}
	for _, p := range restrictedWritePrefixes {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

// bearerKey extracts the Bearer token from the Authorization header.
func bearerKey(r *http.Request) (string, bool) {
	auth := r.Header.Get("Authorization")
	if len(auth) < len("Bearer ") {
		return "", false
	}
	if !strings.EqualFold(auth[:len("Bearer ")], "Bearer ") {
		return "", false
	}
	token := strings.TrimSpace(auth[len("Bearer "):])
	if token == "" {
		return "", false
	}
	return token, true
}

// normalizeAPIKey strips the sk_mule_ prefix exactly once, so keys are
// hashed on their secret material. Presentation of a bare body and a
// prefixed key verify against the same row.
func normalizeAPIKey(key string) string {
	if rest, ok := strings.CutPrefix(key, APIKeyPrefix); ok {
		return rest
	}
	return key
}

// apiKeyHash returns the SHA-256 hex of the normalized key body — the
// value stored in api_keys.key_hash.
func apiKeyHash(key string) string {
	sum := sha256.Sum256([]byte(normalizeAPIKey(key)))
	return hex.EncodeToString(sum[:])
}

// AuthMiddleware authenticates /api/v1/* and /v1/* requests and
// attaches the authenticated user (plus a tenancy principal) to the
// request context. All other paths (/health, /ws, frontend) pass
// through unauthenticated.
func AuthMiddleware(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isProtectedPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			if noAuthEnabled() {
				au := AuthUser{
					ID:     DefaultAdminID,
					Email:  "no-auth",
					Role:   "admin",
					NoAuth: true,
				}
				r = r.WithContext(WithAuthUser(r.Context(), au))
				next.ServeHTTP(w, r)
				return
			}

			key, ok := bearerKey(r)
			if !ok {
				authError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			var (
				au      AuthUser
				expires sql.NullTime
			)
			err := db.QueryRowContext(r.Context(), `
				SELECT ak.restricted, ak.expires_at, u.id, u.email, u.role
				FROM api_keys ak
				JOIN users u ON u.id = ak.user_id
				WHERE ak.key_hash = $1`, apiKeyHash(key),
			).Scan(&au.Restricted, &expires, &au.ID, &au.Email, &au.Role)
			if err == sql.ErrNoRows {
				authError(w, http.StatusUnauthorized, "unknown API key")
				return
			}
			if err != nil {
				log.Printf("auth: failed to look up API key: %v", err)
				authError(w, http.StatusInternalServerError, "authentication failed")
				return
			}

			if expires.Valid && !expires.Time.After(time.Now()) {
				authError(w, http.StatusUnauthorized, "API key expired")
				return
			}

			if au.Restricted && isRestrictedWrite(r.Method, r.URL.Path) {
				authError(w, http.StatusForbidden, "restricted API key: write access to this resource class is denied")
				return
			}

			r = r.WithContext(WithAuthUser(r.Context(), au))
			next.ServeHTTP(w, r)
		})
	}
}

// authError writes a 4xx/5xx JSON error in the standard mule shape.
func authError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error:   http.StatusText(status),
		Message: message,
	})
}
