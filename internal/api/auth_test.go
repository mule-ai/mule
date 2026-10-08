package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeAPIKey(t *testing.T) {
	if got := normalizeAPIKey("sk_mule_abc123"); got != "abc123" {
		t.Fatalf("expected abc123, got %q", got)
	}
	// Bare body passes through unchanged.
	if got := normalizeAPIKey("abc123"); got != "abc123" {
		t.Fatalf("expected abc123, got %q", got)
	}
	// Strip exactly once.
	if got := normalizeAPIKey("sk_mule_sk_mule_abc"); got != "sk_mule_abc" {
		t.Fatalf("expected sk_mule_abc, got %q", got)
	}
}

func TestApiKeyHash(t *testing.T) {
	// Prefixed and bare forms hash to the same stored value.
	sum := sha256.Sum256([]byte("deadbeef"))
	want := hex.EncodeToString(sum[:])
	if got := apiKeyHash("sk_mule_deadbeef"); got != want {
		t.Fatalf("expected %s, got %q", want, got)
	}
	if got := apiKeyHash("deadbeef"); got != want {
		t.Fatalf("bare body must hash identically: expected %s, got %q", want, got)
	}
}

func TestBearerKey(t *testing.T) {
	cases := []struct {
		header string
		want   string
		ok     bool
	}{
		{"", "", false},
		{"Bearer sk_mule_abc", "sk_mule_abc", true},
		{"bearer sk_mule_abc", "sk_mule_abc", true}, // case-insensitive scheme
		{"Bearer   sk_mule_abc", "sk_mule_abc", true},
		{"Bearer", "", false},
		{"Bearer  ", "", false},
		{"Token sk_mule_abc", "", false},
		{"Basic YWJj", "", false},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
		if c.header != "" {
			req.Header.Set("Authorization", c.header)
		}
		got, ok := bearerKey(req)
		if got != c.want || ok != c.ok {
			t.Errorf("bearerKey(%q) = (%q, %v), want (%q, %v)", c.header, got, ok, c.want, c.ok)
		}
	}
}

func TestIsProtectedPath(t *testing.T) {
	// Protected: every /api/v1/* and /v1/* route.
	protected := []string{"/api/v1/agents", "/api/v1/jobs/123/cancel", "/v1/models", "/v1/chat/completions"}
	for _, p := range protected {
		if !isProtectedPath(p) {
			t.Errorf("isProtectedPath(%q) = false, want true", p)
		}
	}
	// Not protected: health, websocket, frontend, settings, other APIs.
	open := []string{"/health", "/ws", "/", "/index.html", "/favicon.ico", "/v2/whatever", "/api/v0/jobs"}
	for _, p := range open {
		if isProtectedPath(p) {
			t.Errorf("isProtectedPath(%q) = true, want false", p)
		}
	}
}

func TestIsRestrictedWrite(t *testing.T) {
	cases := []struct {
		method, path string
		want         bool
	}{
		// Read routes: allowed for restricted keys.
		{http.MethodGet, "/api/v1/agents", false},
		{http.MethodGet, "/api/v1/agents/1", false},
		{http.MethodGet, "/api/v1/workflows", false},
		{http.MethodGet, "/api/v1/jobs", false},
		// Write routes on restricted classes: denied.
		{http.MethodPost, "/api/v1/agents", true},
		{http.MethodPost, "/api/v1/agents/1/tools", true},
		{http.MethodPut, "/api/v1/workflows/1", true},
		{http.MethodDelete, "/api/v1/jobs/1", true},
		{http.MethodPost, "/api/v1/workflows/1/steps", true},
		{http.MethodPatch, "/api/v1/policies", true},
		{http.MethodDelete, "/api/v1/policies/1", true},
		// Write routes outside the restricted classes: allowed (a
		// restricted key may still create providers/tools/skills/wasm).
		{http.MethodPost, "/api/v1/providers", false},
		{http.MethodPost, "/api/v1/tools", false},
		{http.MethodPost, "/api/v1/skills", false},
		{http.MethodPost, "/api/v1/wasm-modules", false},
		// The OpenAI-compatible execution surface is NOT in the
		// restricted write classes: a restricted key may execute.
		{http.MethodPost, "/v1/chat/completions", false},
		{http.MethodGet, "/v1/models", false},
	}
	for _, c := range cases {
		if got := isRestrictedWrite(c.method, c.path); got != c.want {
			t.Errorf("isRestrictedWrite(%s, %q) = %v, want %v", c.method, c.path, got, c.want)
		}
	}
}

func TestKeyPrefix(t *testing.T) {
	if got := keyPrefix("sk_mule_deadbeefcafe"); got != "sk_mule_dead" {
		t.Fatalf("expected sk_mule_dead, got %q", got)
	}
	if got := keyPrefix("short"); got != "short" {
		t.Fatalf("expected short, got %q", got)
	}
}
