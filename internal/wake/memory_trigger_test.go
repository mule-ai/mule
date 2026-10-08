package wake

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ------------------------------------------------------------------
// fake forge for the H4.5 trigger queue API
// (GET …/memory/triggers/pending + POST …/memory/triggers/:tid/consumed)
// ------------------------------------------------------------------

type triggerTestRow struct {
	id        string
	agentID   string
	createdAt time.Time
	payload   map[string]interface{}
	consumed  bool
}

type consumedCall struct {
	agentID   string
	triggerID string
}

type fakeTriggerForge struct {
	mu          sync.Mutex
	rows        map[string][]*triggerTestRow
	consumed    []consumedCall
	failConsume bool
	pendingHits int
	lastAuth    string
}

func newFakeTriggerForge() *fakeTriggerForge {
	return &fakeTriggerForge{rows: map[string][]*triggerTestRow{}}
}

func (f *fakeTriggerForge) addRow(r *triggerTestRow) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[r.agentID] = append(f.rows[r.agentID], r)
}

func (f *fakeTriggerForge) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		f.mu.Lock()
		f.lastAuth = r.Header.Get("Authorization")
		defer f.mu.Unlock()

		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/")
		parts := strings.Split(rest, "/")
		// parts: [agentID ...]
		agentID := parts[0]

		switch {
		case r.Method == http.MethodGet &&
			len(parts) == 4 && parts[1] == "memory" && parts[2] == "triggers" && parts[3] == "pending":
			f.pendingHits++
			out := []map[string]interface{}{}
			for _, row := range f.rows[agentID] {
				if row.consumed {
					continue
				}
				out = append(out, map[string]interface{}{
					"id":         row.id,
					"agent_id":   row.agentID,
					"created_at": row.createdAt.UTC().Format(time.RFC3339Nano),
					"payload":    row.payload,
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"triggers": out})
		case r.Method == http.MethodPost &&
			len(parts) == 5 && parts[1] == "memory" && parts[2] == "triggers" && parts[4] == "consumed":
			tid := parts[3]
			if f.failConsume {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"boom"}`))
				return
			}
			for _, row := range f.rows[agentID] {
				if row.id == tid {
					row.consumed = true
					f.consumed = append(f.consumed, consumedCall{agentID, tid})
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"trigger": map[string]interface{}{"id": tid}})
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"Trigger not found"}`))
		default:
			http.NotFound(w, r)
		}
	})
}

func (f *fakeTriggerForge) snapshot() (int, []consumedCall, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]consumedCall, len(f.consumed))
	copy(out, f.consumed)
	return f.pendingHits, out, f.lastAuth
}

func watchPayload(wakeID, episodeSummary string) map[string]interface{} {
	watch := map[string]interface{}{
		"match":          "invoice deadline",
		"cooldown_hours": 24,
	}
	if wakeID != "" {
		watch["wake_id"] = wakeID
	}
	return map[string]interface{}{
		"belief_id":       "belief-1",
		"belief_content":  "nudge when an invoice deadline is mentioned",
		"watch_match":     "invoice deadline",
		"watch":           watch,
		"episode_id":      "episode-1",
		"episode_summary": episodeSummary,
		"match_score":     1.0,
	}
}

// ------------------------------------------------------------------
// H4.5 forwarder tests (fake forge + real mule store on scratch PG)
// ------------------------------------------------------------------

// (a) a pending row whose watch.wake_id names a live wake → FireWake
// fires it (wake_log row + the rendered prompt reaches the wake's
// forge target) and the row is acked to the fake forge.
func TestMemoryTriggerFiresWakeAndAcks(t *testing.T) {
	db := wakeDB(t)
	ctx := context.Background()
	store := NewStore(db)
	sched := NewScheduler(store, nil)

	forg := newFakeForge("agent-1")
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()

	w := &Wake{
		Name:           "invoice nudge",
		Kind:           KindMemory,
		Spec:           map[string]interface{}{"agent_id": "agent-1", "predicate": "watch"},
		Target:         forgeTarget(srv.URL, "agent-1"),
		PromptTemplate: "watch fired: {{.payload.episode_summary}}",
		Enabled:        true,
	}
	mustCreate(t, store, w)

	fake := newFakeTriggerForge()
	fake.addRow(&triggerTestRow{
		id: "trigger-1", agentID: "agent-1", createdAt: time.Now().UTC(),
		payload: watchPayload(w.ID, "the Invoice Deadline moved to Friday"),
	})
	tf := httptest.NewServer(fake.handler())
	defer tf.Close()

	fwd := NewMemoryTriggerForwarder(store, sched, MemoryTriggerConfig{
		BaseURL: tf.URL,
		APIKey:  "sk_forgemem",
		Agents:  []string{"agent-1"},
	})
	require.True(t, fwd.Enabled())
	fwd.PollOnce(ctx)

	// The wake fired: wake_log row with ok=true and the trigger payload.
	logs, err := store.Log(ctx, w.ID, 100)
	require.NoError(t, err)
	require.Len(t, logs, 1, "exactly one wake_log row")
	require.NotNil(t, logs[0].OK)
	assert.True(t, *logs[0].OK, "fire ok: %v", logs[0].Error)
	require.NotNil(t, logs[0].Payload)
	assert.Equal(t, "episode-1", logs[0].Payload["episode_id"])

	// The wake's target got the rendered prompt (fire-and-forget post).
	posts := forg.posted()
	require.Len(t, posts, 1)
	assert.Contains(t, posts[0], "[mule wake "+w.ID+"] ")
	assert.Contains(t, posts[0], "watch fired: the Invoice Deadline moved to Friday")

	// The row was acked to forge, with the Bearer key.
	hits, consumed, auth := fake.snapshot()
	assert.Equal(t, 1, hits, "one pending poll")
	require.Len(t, consumed, 1)
	assert.Equal(t, consumedCall{"agent-1", "trigger-1"}, consumed[0])
	assert.Equal(t, "Bearer sk_forgemem", auth)
}

// (b) rows with no wake_id (and rows whose wake_id is unknown to this
// mule) are acked without firing — a misconfigured belief cannot block
// the queue.
func TestMemoryTriggerNoWakeIdIsAcked(t *testing.T) {
	db := wakeDB(t)
	ctx := context.Background()
	store := NewStore(db)
	sched := NewScheduler(store, nil)

	fake := newFakeTriggerForge()
	fake.addRow(&triggerTestRow{
		id: "no-wake-id", agentID: "agent-1", createdAt: time.Now().UTC(),
		payload: watchPayload("", "the invoice deadline arrived"),
	})
	fake.addRow(&triggerTestRow{
		id: "unknown-wake", agentID: "agent-1", createdAt: time.Now().UTC(),
		payload: watchPayload("wake-that-does-not-exist", "another mention"),
	})
	tf := httptest.NewServer(fake.handler())
	defer tf.Close()

	fwd := NewMemoryTriggerForwarder(store, sched, MemoryTriggerConfig{
		BaseURL: tf.URL,
		APIKey:  "k",
		Agents:  []string{"agent-1"},
	})
	fwd.PollOnce(ctx)

	_, consumed, _ := fake.snapshot()
	require.Len(t, consumed, 2, "both rows acked")
	assert.ElementsMatch(t,
		[]consumedCall{{"agent-1", "no-wake-id"}, {"agent-1", "unknown-wake"}},
		consumed)

	// No wake was fired: nothing in any wake_log.
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM wake_log`).Scan(&count))
	assert.Zero(t, count, "no fires for unresolvable watches")
}

// (c) fire-then-ACK failure: the fire submits (wake_log + target
// message) but the ACK 500s, so the row stays pending and the NEXT
// poll re-fires it — at-least-once delivery.
func TestMemoryTriggerAckFailureRetries(t *testing.T) {
	db := wakeDB(t)
	ctx := context.Background()
	store := NewStore(db)
	sched := NewScheduler(store, nil)

	forg := newFakeForge("agent-1")
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()

	w := &Wake{
		Name:             "invoice nudge",
		Kind:             KindMemory,
		Spec:             map[string]interface{}{"agent_id": "agent-1", "predicate": "watch"},
		Target:           forgeTarget(srv.URL, "agent-1"),
		PromptTemplate:   "nudge {{.fired_at}}",
		Enabled:          true,
		RateLimitPerHour: 24,
	}
	mustCreate(t, store, w)

	fake := newFakeTriggerForge()
	fake.addRow(&triggerTestRow{
		id: "retry-me", agentID: "agent-1", createdAt: time.Now().UTC(),
		payload: watchPayload(w.ID, "invoice deadline slipped"),
	})
	tf := httptest.NewServer(fake.handler())
	defer tf.Close()

	fwd := NewMemoryTriggerForwarder(store, sched, MemoryTriggerConfig{
		BaseURL: tf.URL, APIKey: "k", Agents: []string{"agent-1"},
	})

	// Poll 1: fire succeeds, ACK fails → row stays pending.
	fake.mu.Lock()
	fake.failConsume = true
	fake.mu.Unlock()
	fwd.PollOnce(ctx)

	_, consumed, _ := fake.snapshot()
	assert.Empty(t, consumed, "ACK failed → nothing consumed")
	logs, err := store.Log(ctx, w.ID, 100)
	require.NoError(t, err)
	require.Len(t, logs, 1, "the fire itself was recorded")
	require.NotNil(t, logs[0].OK)
	assert.True(t, *logs[0].OK)
	assert.Len(t, forg.posted(), 1, "the first fire reached the target")

	// Poll 2: ACK now works → the row re-fires once and is consumed.
	fake.mu.Lock()
	fake.failConsume = false
	fake.mu.Unlock()
	fwd.PollOnce(ctx)

	_, consumed, _ = fake.snapshot()
	require.Len(t, consumed, 1)
	assert.Equal(t, "retry-me", consumed[0].triggerID)
	logs, err = store.Log(ctx, w.ID, 100)
	require.NoError(t, err)
	assert.Len(t, logs, 2, "at-least-once: the second poll re-fired the wake")
	assert.Len(t, forg.posted(), 2, "the target got both fires")

	// Poll 3: nothing pending → no third fire.
	fwd.PollOnce(ctx)
	logs, err = store.Log(ctx, w.ID, 100)
	require.NoError(t, err)
	assert.Len(t, logs, 2, "no fire after the row is consumed")
}

// (d) a pending row older than MemoryTriggerStaleAfter (6 h) is
// skipped (not fired) and acked, so the queue advances.
func TestMemoryTriggerStaleRowSkipped(t *testing.T) {
	db := wakeDB(t)
	ctx := context.Background()
	store := NewStore(db)
	sched := NewScheduler(store, nil)

	forg := newFakeForge("agent-1")
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()

	w := &Wake{
		Name:             "invoice nudge",
		Kind:             KindMemory,
		Spec:             map[string]interface{}{"agent_id": "agent-1", "predicate": "watch"},
		Target:           forgeTarget(srv.URL, "agent-1"),
		PromptTemplate:   "stale?",
		Enabled:          true,
		RateLimitPerHour: 24,
	}
	mustCreate(t, store, w)

	fake := newFakeTriggerForge()
	fake.addRow(&triggerTestRow{
		id: "ancient", agentID: "agent-1", createdAt: time.Now().UTC().Add(-7 * time.Hour),
		payload: watchPayload(w.ID, "invoice deadline from long ago"),
	})
	tf := httptest.NewServer(fake.handler())
	defer tf.Close()

	fwd := NewMemoryTriggerForwarder(store, sched, MemoryTriggerConfig{
		BaseURL: tf.URL, APIKey: "k", Agents: []string{"agent-1"},
	})
	fwd.PollOnce(ctx)

	// Acked, but never fired.
	_, consumed, _ := fake.snapshot()
	require.Len(t, consumed, 1, "stale row acked to advance the queue")
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM wake_log`).Scan(&count))
	assert.Zero(t, count, "stale row must not fire the wake")
	assert.Empty(t, forg.posted())
}

// (e) FORGE_TRIGGER_AGENTS empty (or no base URL) → the lane is
// disabled: Start/Stop are no-ops and the forge is never polled.
func TestMemoryTriggerDisabledWithoutAgents(t *testing.T) {
	store := NewStore(nil)
	sched := NewScheduler(store, nil)

	fake := newFakeTriggerForge()
	tf := httptest.NewServer(fake.handler())
	defer tf.Close()

	// No agents → disabled.
	fwd := NewMemoryTriggerForwarder(store, sched, MemoryTriggerConfig{
		BaseURL: tf.URL, APIKey: "k",
	})
	assert.False(t, fwd.Enabled())
	fwd.Start()
	assert.NotPanics(t, fwd.Stop, "Stop on a never-started loop must not hang or panic")

	// Agents but no base URL → disabled too.
	fwd2 := NewMemoryTriggerForwarder(store, sched, MemoryTriggerConfig{
		APIKey: "k", Agents: []string{"agent-1"},
	})
	assert.False(t, fwd2.Enabled())
	fwd2.Start()
	fwd2.Stop()

	hits, _, _ := fake.snapshot()
	assert.Zero(t, hits, "a disabled lane must never poll the forge")
}
