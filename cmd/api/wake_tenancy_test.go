package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mule-ai/mule/internal/api"
)

// fakeForgeMinimal is the smallest forge the wake forge target needs
// (the engine package's fakeForge is test-local to that package).
type fakeForgeMinimal struct {
	mu    sync.Mutex
	agent string
	conv  string
	posts []string
}

func newFakeForgeMinimal(agentID string) *fakeForgeMinimal {
	return &fakeForgeMinimal{agent: agentID, conv: "conv-1"}
}

func (f *fakeForgeMinimal) posted() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.posts))
	copy(out, f.posts)
	return out
}

func (f *fakeForgeMinimal) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/active"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"busy": false, "current_conversation": f.conv,
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/conversations"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"session": map[string]interface{}{"id": f.conv},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/messages":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"messages": []interface{}{}})
		case r.Method == http.MethodPost && r.URL.Path == "/messages":
			var body struct {
				Content string `json:"content"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.posts = append(f.posts, body.Content)
			f.mu.Unlock()
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"message": map[string]interface{}{}})
		default:
			http.NotFound(w, r)
		}
	})
}

// wakesAdminBoot bootstraps the deterministic admin + key used by the
// wake tenancy tests.
func wakesAdminBoot(t *testing.T) (string, string) {
	t.Helper()
	t.Setenv("MULE_ADMIN_EMAIL", "wakeadmin@example.com")
	t.Setenv("MULE_ADMIN_PASSWORD", "wake-admin-pass")
	t.Setenv("MULE_ADMIN_API_KEY", "sk_mule_1111adminkey000000000000000000")
	t.Setenv("MULE_ADMIN_ID", "")
	return "sk_mule_1111adminkey000000000000000000", "wakeadmin@example.com"
}

// TestWakesOwnerIsolation: user A's wakes are invisible and
// untouchable by user B (H3.1 scoping on the new owner_id column).
func TestWakesOwnerIsolation(t *testing.T) {
	requireAuthMode(t)
	db := tenancyDB(t)
	adminKey, _ := wakesAdminBoot(t)
	_, err := api.BootstrapAdmin(context.Background(), db.DB)
	require.NoError(t, err)
	userB := insertTestUser(t, db, "wake-user-b", "wb@example.com", false, time.Time{})
	router := tenancyRouter(t, db)

	forg := newFakeForgeMinimal("agent-1")
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()

	body := fmt.Sprintf(`{
		"name": "A's nightly wake",
		"kind": "cron",
		"spec": {"expr": "0 3 * * *"},
		"target": {"forge_conversation": {"forge_url": %q, "api_key": "sk_forge_test", "agent_id": "agent-1"}},
		"prompt_template": "hello from {{.wake_name}}"
	}`, srv.URL)

	// A creates: 201, cron next_due_at computed (future).
	w := tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes", adminKey, []byte(body))
	require.Equal(t, http.StatusCreated, w.Code, "A create wake: %s", w.Body.String())
	var created struct {
		ID        string  `json:"id"`
		NextDueAt *string `json:"next_due_at"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.NotEmpty(t, created.ID)
	require.NotNil(t, created.NextDueAt, "cron wakes must have a computed next_due_at")
	require.NotEmpty(t, *created.NextDueAt)

	// B sees nothing: empty list, 404 by id, 404 on log.
	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/wakes", userB, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"wakes":[]`, "B's wake list must be empty: %s", w.Body.String())

	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/wakes/"+created.ID, userB, nil)
	assert.Equal(t, http.StatusNotFound, w.Code, "B must not see A's wake by id")
	w = tenancyRequest(t, router, http.MethodPatch, "/api/v1/wakes/"+created.ID, userB, []byte(`{"enabled":false}`))
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = tenancyRequest(t, router, http.MethodDelete, "/api/v1/wakes/"+created.ID, userB, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/wakes/"+created.ID+"/log", userB, nil)
	assert.Equal(t, http.StatusNotFound, w.Code, "B must not read A's wake log")

	// A still sees it, and can disable it.
	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/wakes/"+created.ID, adminKey, nil)
	require.Equal(t, http.StatusOK, w.Code)
	w = tenancyRequest(t, router, http.MethodPatch, "/api/v1/wakes/"+created.ID, adminKey, []byte(`{"enabled":false}`))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var patched struct {
		Enabled *bool `json:"enabled"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &patched))
	require.NotNil(t, patched.Enabled)
	assert.False(t, *patched.Enabled)

	// A can delete it; then the id 404s for A too.
	w = tenancyRequest(t, router, http.MethodDelete, "/api/v1/wakes/"+created.ID, adminKey, nil)
	require.Equal(t, http.StatusNoContent, w.Code)
	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/wakes/"+created.ID, adminKey, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestWakesValidation: bad kind / bad cron expr / bad target / missing
// prompt_template are all 400 at creation; inert H4 kinds are accepted
// (validated + stored, never fired).
func TestWakesValidation(t *testing.T) {
	requireAuthMode(t)
	db := tenancyDB(t)
	adminKey, _ := wakesAdminBoot(t)
	_, err := api.BootstrapAdmin(context.Background(), db.DB)
	require.NoError(t, err)
	router := tenancyRouter(t, db)

	forg := newFakeForgeMinimal("agent-1")
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()
	target := map[string]interface{}{"forge_conversation": map[string]interface{}{
		"forge_url": srv.URL, "api_key": "sk_forge_test", "agent_id": "agent-1"}}
	base := func(mut func(map[string]interface{})) []byte {
		m := map[string]interface{}{
			"name": "v", "kind": "cron",
			"spec":            map[string]interface{}{"expr": "0 3 * * *"},
			"target":          target,
			"prompt_template": "hi",
		}
		mut(m)
		b, _ := json.Marshal(m)
		return b
	}

	// Invalid cron expr -> 400 (the documented subset).
	w := tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes", adminKey, base(func(m map[string]interface{}) {
		m["spec"] = map[string]interface{}{"expr": "60 3 * * *"}
	}))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	// Unknown kind -> 400.
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes", adminKey, base(func(m map[string]interface{}) {
		m["kind"] = "shout"
	}))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	// Missing target -> 400.
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes", adminKey, base(func(m map[string]interface{}) {
		delete(m, "target")
	}))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	// Bare agent_id without forge creds is not addressable in v1 -> 400.
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes", adminKey, base(func(m map[string]interface{}) {
		m["target"] = map[string]interface{}{"agent_id": "agent-1"}
	}))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	// Missing prompt_template -> 400.
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes", adminKey, base(func(m map[string]interface{}) {
		delete(m, "prompt_template")
	}))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Inert H4 kinds: accepted (201), inert (no fire lane exists for them).
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes", adminKey, base(func(m map[string]interface{}) {
		m["kind"] = "memory"
		m["spec"] = map[string]interface{}{"agent_id": "agent-1", "predicate": "recall(\"x\")"}
	}))
	require.Equal(t, http.StatusCreated, w.Code, "memory wake accepted: %s", w.Body.String())
	var memWake struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &memWake))
	// Stored with a NULL next_due_at (no cron schedule) and no log.
	var nextDue interface{}
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT next_due_at FROM wakes WHERE id = $1`, memWake.ID).Scan(&nextDue))
	assert.Nil(t, nextDue, "inert wakes have no cron schedule")
	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/wakes/"+memWake.ID+"/log", adminKey, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"log":[]`)

	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes", adminKey, base(func(m map[string]interface{}) {
		m["kind"] = "agent_signal"
		m["spec"] = map[string]interface{}{"agent_id": "agent-1", "kinds": []string{"handoff"}}
	}))
	require.Equal(t, http.StatusCreated, w.Code, "agent_signal wake accepted: %s", w.Body.String())
}

// TestWakesFireIngressOverHTTP: POST /api/v1/wakes/fire (webhook shape,
// matching the ranch webhook {source, event_tag, payload} contract)
// fires the matching wake synchronously and the fire shows up in the
// wake log. Any valid key may call it (ingress, not mutation).
func TestWakesFireIngressOverHTTP(t *testing.T) {
	requireAuthMode(t)
	db := tenancyDB(t)
	adminKey, _ := wakesAdminBoot(t)
	_, err := api.BootstrapAdmin(context.Background(), db.DB)
	require.NoError(t, err)
	userB := insertTestUser(t, db, "wake-user-c", "wc@example.com", false, time.Time{})
	router := tenancyRouter(t, db)

	forg := newFakeForgeMinimal("agent-1")
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()

	create := fmt.Sprintf(`{
		"name": "ranch push reporter",
		"kind": "webhook",
		"spec": {"source": "ranch", "tag": "push"},
		"target": {"forge_conversation": {"forge_url": %q, "api_key": "sk_forge_test", "agent_id": "agent-1"}},
		"prompt_template": "repo {{.payload.repo}} pushed on {{.fired_at}}"
	}`, srv.URL)
	w := tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes", adminKey, []byte(create))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))

	// B's key fires the ingress (fleet-wide; owner-scoped CRUD only).
	fire := `{"kind":"webhook","source":"ranch","event_tag":"push","payload":{"repo":"mule"}}`
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes/fire", userB, []byte(fire))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var out struct {
		Fired int `json:"fired"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, 1, out.Fired, "the matching webhook wake must fire: %s", w.Body.String())

	posts := forg.posted()
	require.Len(t, posts, 1, "the forge agent must have been woken")
	assert.Contains(t, posts[0], "[mule wake "+created.ID+"] ")
	assert.Contains(t, posts[0], "repo mule pushed on ")

	// wake_log: ok=true, payload carries the fire payload.
	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/wakes/"+created.ID+"/log", adminKey, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var logResp struct {
		Log []struct {
			OK      *bool                  `json:"ok"`
			Error   *string                `json:"error"`
			Payload map[string]interface{} `json:"payload"`
		} `json:"log"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &logResp))
	require.Len(t, logResp.Log, 1)
	assert.True(t, *logResp.Log[0].OK)
	assert.Equal(t, "mule", logResp.Log[0].Payload["repo"])

	// Non-matching ingress: fired=0, no new log row.
	fire = `{"kind":"webhook","source":"github","event_tag":"push","payload":{}}`
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes/fire", adminKey, []byte(fire))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"fired":0`)
	// Bad fire kind: 400.
	fire = `{"kind":"shout","source":"x"}`
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes/fire", adminKey, []byte(fire))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestWakesNoAuthMode: MULE_NO_AUTH=1 — the synthetic admin owns the
// wakes (regression gate, same contract as the other primitive routes).
func TestWakesNoAuthMode(t *testing.T) {
	db := tenancyDB(t)
	t.Setenv("MULE_NO_AUTH", "1")
	router := tenancyRouter(t, db)

	w := tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes", "", []byte(`{
		"name": "no-auth wake",
		"kind": "event",
		"spec": {"type": "job.completed"},
		"target": {"workflow_id": "00000000-0000-4000-8000-0000000000aa"},
		"prompt_template": "done"
	}`))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct {
		OwnerID string `json:"owner_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, api.DefaultAdminID, created.OwnerID, "synthetic admin must own created wakes")

	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/wakes", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "no-auth wake")
}
