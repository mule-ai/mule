package main

// Herd H5.3 (mule half): the agent bulk-pause on wakes + the wake
// activity feed the ranch Activity View polls.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mule-ai/mule/internal/api"
)

// makeWake posts a wake and returns its id. specKey is "source" for
// webhook wakes and "type" for event wakes (the kind-specific spec
// shape, see ValidateSpec).
func makeWake(t *testing.T, router http.Handler, key, name, kind, specKey, specValue, targetJSON string) string {
	t.Helper()
	body := fmt.Sprintf(`{
		"name": %q,
		"kind": %q,
		"spec": {"%s": %q},
		"target": %s,
		"prompt_template": "activity probe"
	}`, name, kind, specKey, specValue, targetJSON)
	w := tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes", key, []byte(body))
	require.Equal(t, http.StatusCreated, w.Code, "create %s: %s", name, w.Body.String())
	var out struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out.ID
}

func forgeTargetJSON(srvURL, agentID string) string {
	if agentID != "" {
		return fmt.Sprintf(`{"forge_conversation": {"forge_url": %q, "api_key": "sk_forge_test", "agent_id": %q}}`, srvURL, agentID)
	}
	// A conversation target: no agent_id, so the agent pause must not
	// touch it.
	return fmt.Sprintf(`{"forge_conversation": {"forge_url": %q, "api_key": "sk_forge_test", "conversation_id": "conv-fixed"}}`, srvURL)
}

// TestWakesAgentPauseBulk: POST /api/v1/wakes/pause bulk-enables/
// disables the caller's agent-targeted wakes, owner-scoped, without
// touching conversation targets or other users' wakes.
func TestWakesAgentPauseBulk(t *testing.T) {
	requireAuthMode(t)
	db := tenancyDB(t)
	adminKey, _ := wakesAdminBoot(t)
	_, err := api.BootstrapAdmin(context.Background(), db.DB)
	require.NoError(t, err)
	userA := insertTestUser(t, db, "h53-a", "h53a@example.com", false, time.Time{})
	userB := insertTestUser(t, db, "h53-b", "h53b@example.com", false, time.Time{})
	router := tenancyRouter(t, db)

	forg := newFakeForgeMinimal("agent-1")
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()

	// A: two agent-1 wakes (webhook + event), one agent-2 wake, one
	// conversation-targeted wake.
	_ = makeWake(t, router, userA, "A agent-1 webhook", "webhook", "source", "src-a1", forgeTargetJSON(srv.URL, "agent-1"))
	_ = makeWake(t, router, userA, "A agent-1 event", "event", "type", "agent.turn_ended", forgeTargetJSON(srv.URL, "agent-1"))
	_ = makeWake(t, router, userA, "A agent-2 event", "event", "type", "agent.turn_ended", forgeTargetJSON(srv.URL, "agent-2"))
	_ = makeWake(t, router, userA, "A conversation wake", "event", "type", "agent.turn_ended", forgeTargetJSON(srv.URL, ""))
	// B: one agent-1 wake.
	_ = makeWake(t, router, userB, "B agent-1 event", "event", "type", "agent.turn_ended", forgeTargetJSON(srv.URL, "agent-1"))

	// A pauses agent-1: exactly A's two agent-1 wakes flip.
	w := tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes/pause", userA,
		[]byte(`{"agent_id": "agent-1", "paused": true}`))
	require.Equal(t, http.StatusOK, w.Code, "pause: %s", w.Body.String())
	var pauseOut struct {
		AgentID string `json:"agent_id"`
		Paused  bool   `json:"paused"`
		Updated int    `json:"updated"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &pauseOut))
	assert.Equal(t, "agent-1", pauseOut.AgentID)
	assert.True(t, pauseOut.Paused)
	assert.Equal(t, 2, pauseOut.Updated, "A's two agent-1 wakes only")

	// Verify the flip (and that agent-2 / conversation / B's wakes are
	// untouched).
	listStates := func(key string) map[string]bool {
		out := tenancyRequest(t, router, http.MethodGet, "/api/v1/wakes", key, nil)
		require.Equal(t, http.StatusOK, out.Code)
		var v struct {
			Wakes []struct {
				Name    string `json:"name"`
				Enabled *bool  `json:"enabled"`
			} `json:"wakes"`
		}
		require.NoError(t, json.Unmarshal(out.Body.Bytes(), &v))
		m := map[string]bool{}
		for _, x := range v.Wakes {
			m[x.Name] = *x.Enabled
		}
		return m
	}
	states := listStates(userA)
	assert.False(t, states["A agent-1 webhook"], "agent-1 wake paused")
	assert.False(t, states["A agent-1 event"], "agent-1 wake paused")
	assert.True(t, states["A agent-2 event"], "agent-2 wake untouched")
	assert.True(t, states["A conversation wake"], "conversation target untouched")
	bStates := listStates(userB)
	assert.True(t, bStates["B agent-1 event"], "B's wake is owner-scoped out")

	// B pauses the SAME agent: only B's own wake flips.
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes/pause", userB,
		[]byte(`{"agent_id": "agent-1", "paused": true}`))
	require.Equal(t, http.StatusOK, w.Code, "B pause: %s", w.Body.String())
	require.Contains(t, w.Body.String(), `"updated":1`, "B updates only B's wake")

	// Resume (paused:false) re-enables.
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes/pause", userA,
		[]byte(`{"agent_id": "agent-1", "paused": false}`))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"updated":2`)

	// Idempotent no-op for an agent nobody has wakes for.
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes/pause", userA,
		[]byte(`{"agent_id": "agent-nobody", "paused": true}`))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"updated":0`)

	// admin (no owner scope): pauses BOTH users' agent-1 wakes.
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes/pause", adminKey,
		[]byte(`{"agent_id": "agent-1", "paused": true}`))
	require.Equal(t, http.StatusOK, w.Code, "admin pause: %s", w.Body.String())
	require.Contains(t, w.Body.String(), `"updated":3`, "admin is not owner-scoped: %s", w.Body.String())

	// Validation: missing agent_id → 400.
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes/pause", userA,
		[]byte(`{"paused": true}`))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestWakesActivityFeed: GET /api/v1/wakes/activity returns the
// caller's recent wake_log rows (ok AND failed fires), owner-scoped,
// newest first, with since/limit.
func TestWakesActivityFeed(t *testing.T) {
	requireAuthMode(t)
	db := tenancyDB(t)
	adminKey, _ := wakesAdminBoot(t)
	_, err := api.BootstrapAdmin(context.Background(), db.DB)
	require.NoError(t, err)
	userA := insertTestUser(t, db, "h53-act-a", "h53acta@example.com", false, time.Time{})
	userB := insertTestUser(t, db, "h53-act-b", "h53actb@example.com", false, time.Time{})
	router := tenancyRouter(t, db)

	forg := newFakeForgeMinimal("agent-1")
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()

	// A: one healthy webhook wake (fires ok) + one dead-forge wake
	// (fires failed → still logged). B: one webhook wake.
	wa := makeWake(t, router, userA, "A healthy", "webhook", "source", "h53-act-a", forgeTargetJSON(srv.URL, "agent-1"))
	_ = makeWake(t, router, userA, "A dead forge", "webhook", "source", "h53-act-a",
		forgeTargetJSON("http://127.0.0.1:1", "agent-1"))
	_ = makeWake(t, router, userB, "B wake", "webhook", "source", "h53-act-b", forgeTargetJSON(srv.URL, "agent-1"))

	// Fire A's source (matches BOTH of A's wakes — healthy + dead).
	w := tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes/fire", adminKey,
		[]byte(`{"kind": "webhook", "source": "h53-act-a"}`))
	require.Equal(t, http.StatusOK, w.Code, "fire A: %s", w.Body.String())
	require.Contains(t, w.Body.String(), `"fired":1`, "exactly one ok fire: %s", w.Body.String())
	// Fire B's source.
	w = tenancyRequest(t, router, http.MethodPost, "/api/v1/wakes/fire", adminKey,
		[]byte(`{"kind": "webhook", "source": "h53-act-b"}`))
	require.Equal(t, http.StatusOK, w.Code, "fire B: %s", w.Body.String())

	// A's feed: both rows (ok AND failed), owner-scoped, B absent.
	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/wakes/activity", userA, nil)
	require.Equal(t, http.StatusOK, w.Code, "activity A: %s", w.Body.String())
	var feed struct {
		Activity []struct {
			WakeID string  `json:"wake_id"`
			Name   string  `json:"name"`
			Kind   string  `json:"kind"`
			OK     *bool   `json:"ok"`
			Error  *string `json:"error"`
		} `json:"activity"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &feed))
	require.Len(t, feed.Activity, 2, "A has one ok + one failed fire: %s", w.Body.String())
	byName := map[string]*struct {
		WakeID string  `json:"wake_id"`
		Name   string  `json:"name"`
		Kind   string  `json:"kind"`
		OK     *bool   `json:"ok"`
		Error  *string `json:"error"`
	}{}
	for i := range feed.Activity {
		byName[feed.Activity[i].Name] = &feed.Activity[i]
	}
	healthy := byName["A healthy"]
	require.NotNil(t, healthy, "the healthy wake's fire is in the feed")
	assert.Equal(t, wa, healthy.WakeID)
	assert.True(t, *healthy.OK)
	assert.Equal(t, "webhook", healthy.Kind)
	assert.Nil(t, healthy.Error)
	dead := byName["A dead forge"]
	require.NotNil(t, dead, "the failed fire is in the feed too (audit trail)")
	assert.False(t, *dead.OK)
	require.NotNil(t, dead.Error, "the failed fire carries its error")
	assert.NotContains(t, w.Body.String(), "B wake", "owner scoping hides B's rows")

	// B's feed: exactly B's row.
	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/wakes/activity", userB, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &feed))
	require.Len(t, feed.Activity, 1)
	assert.Equal(t, "B wake", feed.Activity[0].Name)
	assert.True(t, *feed.Activity[0].OK)

	// since in the future → empty; bad since → 400; limit caps; bad
	// limit → 400.
	future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/wakes/activity?since="+future, userA, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &feed))
	assert.Len(t, feed.Activity, 0, "nothing fired since the future")

	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/wakes/activity?since=not-a-time", userA, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/wakes/activity?limit=1", userA, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &feed))
	assert.Len(t, feed.Activity, 1, "limit=1 caps the feed")

	w = tenancyRequest(t, router, http.MethodGet, "/api/v1/wakes/activity?limit=zero", userA, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
