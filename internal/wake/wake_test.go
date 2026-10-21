package wake

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mule-ai/mule/internal/agent"
	internaldb "github.com/mule-ai/mule/internal/database"
	"github.com/mule-ai/mule/internal/engine"
	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/pkg/job"
)

// ------------------------------------------------------------------
// scratch Postgres (same contract as cmd/api/tenancy_test.go and
// internal/database/db_test.go: postgres:postgres@localhost:5432,
// skip when unavailable)
// ------------------------------------------------------------------

func wakeDB(t *testing.T) *sql.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping wake test that requires PostgreSQL")
	}
	admin, err := sql.Open("postgres", "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable")
	if err != nil {
		t.Skipf("could not open admin connection: %v", err)
	}
	defer admin.Close()
	if err := admin.Ping(); err != nil {
		t.Skipf("could not connect to test PostgreSQL: %v", err)
	}
	if _, err := admin.Exec(`DROP DATABASE IF EXISTS mule_wakes WITH (FORCE)`); err != nil {
		t.Skipf("could not drop mule_wakes: %v", err)
	}
	if _, err := admin.Exec(`CREATE DATABASE mule_wakes`); err != nil {
		t.Skipf("could not create mule_wakes: %v", err)
	}
	db, err := internaldb.NewDB(internaldb.Config{
		Host: "localhost", Port: 5432, User: "postgres",
		Password: "postgres", DBName: "mule_wakes", SSLMode: "disable",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.InitSchema())
	return db.DB
}

// ------------------------------------------------------------------
// minimal fake forge (POST /messages + addressing; the engine
// package's fakeForge is test-local, so the wake package carries a
// smaller copy with only the endpoints ForgeSubmission touches)
// ------------------------------------------------------------------

type fakeForge struct {
	mu     sync.Mutex
	agent  string
	conv   string
	posts  []string
	apiKey string
}

func newFakeForge(agentID string) *fakeForge {
	return &fakeForge{agent: agentID, conv: "conv-1"}
}

func (f *fakeForge) posted() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.posts))
	copy(out, f.posts)
	return out
}

func (f *fakeForge) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/active"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"busy": false, "current_conversation": f.conv,
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/conversations"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"session":     map[string]interface{}{"id": f.conv},
				"working_dir": "/forge/sessions/" + f.conv,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/messages":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"messages": []interface{}{}})
		case r.Method == http.MethodPost && r.URL.Path == "/messages":
			var body struct {
				SessionID string `json:"session_id"`
				Content   string `json:"content"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.apiKey = r.Header.Get("Authorization")
			f.posts = append(f.posts, body.Content)
			f.mu.Unlock()
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"message": map[string]interface{}{}})
		default:
			http.NotFound(w, r)
		}
	})
}

func forgeTarget(srvURL, agentID string) map[string]interface{} {
	return map[string]interface{}{
		"forge_conversation": map[string]interface{}{
			"forge_url": srvURL,
			"api_key":   "sk_forge_test",
			"agent_id":  agentID,
		},
	}
}

func mustCreate(t *testing.T, store *Store, w *Wake) {
	t.Helper()
	if w.OwnerID == "" {
		w.OwnerID = "test-owner"
	}
	require.NoError(t, store.Create(context.Background(), w))
}

// ------------------------------------------------------------------
// cron semantics (unit — mirrors the Rust reference test vectors)
// ------------------------------------------------------------------

func TestCronMatchesVectors(t *testing.T) {
	// 2026-09-13T03:00:00Z is a Sunday (same vector as the Rust
	// reference, triggers.rs tests).
	ts := time.Unix(1789268400, 0).UTC()
	assert.True(t, CronMatches("0 3 * * *", ts))
	assert.True(t, CronMatches("0 3 * * 0", ts), "dow 0 = Sunday")
	assert.False(t, CronMatches("0 3 * * 1", ts))
	assert.False(t, CronMatches("5 3 * * *", ts))
	assert.True(t, CronMatches("* * * * *", ts))
	assert.False(t, CronMatches("15 * * * *", ts))
	assert.True(t, CronMatches("15 * * * *", ts.Add(15*time.Minute)))
}

func TestCronNextStrictlyAfter(t *testing.T) {
	ts := time.Unix(1789268400, 0).UTC() // 03:00:00 Sunday
	// Daily at 03:00 from exactly 03:00 -> tomorrow 03:00.
	nd, err := CronNext("0 3 * * *", ts)
	require.NoError(t, err)
	assert.Equal(t, ts.Add(24*time.Hour), nd)
	// Every 5 minutes from 03:00:30 -> 03:05 (aligned to the minute).
	nd, err = CronNext("*/5 * * * *", ts.Add(30*time.Second))
	require.NoError(t, err)
	assert.Equal(t, ts.Add(5*time.Minute), nd)
	// Every minute -> next full minute.
	nd, err = CronNext("* * * * *", ts)
	require.NoError(t, err)
	assert.Equal(t, ts.Add(time.Minute), nd)
}

func TestValidateCronExprSubset(t *testing.T) {
	valid := []string{
		"* * * * *", "*/15 * * * *", "0 3 * * *", "0 3 1,15 * *",
		"0-59 0-23 1-31 1-12 0-6", "5 4 * * 0", "0 0 1 1 *",
	}
	for _, e := range valid {
		assert.NoError(t, ValidateCronExpr(e), "%q must be valid", e)
	}
	invalid := []string{
		"* * * *",     // 4 fields
		"* * * * * *", // 6 fields
		"60 * * * *",  // minute out of range
		"* 24 * * *",  // hour out of range
		"0 0 0 * *",   // dom out of range
		"0 0 * 13 *",  // month out of range
		"0 0 * * 7",   // dow 7 NOT in the supported subset
		"a b c d e",   // unparseable
		"0 3 * * *x",  // trailing garbage
		"*/0 * * * *", // step must be >= 1
		"5-1 * * * *", // inverted range
		"",            // empty
	}
	for _, e := range invalid {
		assert.Error(t, ValidateCronExpr(e), "%q must be invalid", e)
	}
}

// ------------------------------------------------------------------
// fire path tests
// ------------------------------------------------------------------

func TestCronWakeFiresOnce(t *testing.T) {
	db := wakeDB(t)
	ctx := context.Background()
	store := NewStore(db)
	forg := newFakeForge("agent-1")
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()

	w := &Wake{
		Name:             "every-minute ping",
		Kind:             KindCron,
		Spec:             map[string]interface{}{"expr": "* * * * *"},
		Target:           forgeTarget(srv.URL, "agent-1"),
		PromptTemplate:   "ping from {{.wake_name}} at {{.fired_at}}",
		Enabled:          true,
		RateLimitPerHour: 24,
	}
	mustCreate(t, store, w)

	// Force-due: push next_due_at into the past (the handler computes
	// the real value at creation; the test injects it, per the plan).
	_, err := db.Exec(`UPDATE wakes SET next_due_at = NOW() - INTERVAL '60 seconds' WHERE id = $1`, w.ID)
	require.NoError(t, err)

	sched := NewScheduler(store, nil)
	require.Equal(t, 1, sched.ScanDue(ctx), "first scan must fire the due cron wake")

	// wake_log row: ok=true.
	logs, err := store.Log(ctx, w.ID, 100)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.NotNil(t, logs[0].OK)
	assert.True(t, *logs[0].OK)
	assert.Nil(t, logs[0].Error)

	// The forge got exactly one message: the mule-wake-tagged render.
	posts := forg.posted()
	require.Len(t, posts, 1, "exactly one forge message, got %v", posts)
	assert.Contains(t, posts[0], "[mule wake "+w.ID+"] ")
	assert.Contains(t, posts[0], "ping from every-minute ping at ")

	// Schedule advanced past now.
	refreshed, err := store.Get(ctx, w.ID)
	require.NoError(t, err)
	require.NotNil(t, refreshed.LastFiredAt)
	require.NotNil(t, refreshed.NextDueAt)
	assert.True(t, refreshed.NextDueAt.After(time.Now().UTC().Add(-time.Minute)),
		"next_due_at must advance to a future fire time")

	// Second scan in the same window: no double fire.
	require.Equal(t, 0, sched.ScanDue(ctx), "same-window rescan must not double-fire")
	assert.Len(t, forg.posted(), 1)
	logs, err = store.Log(ctx, w.ID, 100)
	require.NoError(t, err)
	assert.Len(t, logs, 1, "no second wake_log row")
}

func TestWebhookFireIngress(t *testing.T) {
	db := wakeDB(t)
	ctx := context.Background()
	store := NewStore(db)
	forg := newFakeForge("agent-1")
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()

	w := &Wake{
		Name:           "ranch push",
		Kind:           KindWebhook,
		Spec:           map[string]interface{}{"source": "ranch", "tag": "push"},
		Target:         forgeTarget(srv.URL, "agent-1"),
		PromptTemplate: "push on {{.payload.repo}}",
		Enabled:        true,
	}
	mustCreate(t, store, w)

	sched := NewScheduler(store, nil)

	// Matching source + tag fires; payload reaches the template.
	results, err := sched.FireExternal(ctx, "webhook", "ranch", "push",
		map[string]interface{}{"repo": "mule"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].OK, "fire result: %s", results[0].Error)
	posts := forg.posted()
	require.Len(t, posts, 1)
	assert.Equal(t, "[mule wake "+w.ID+"] push on mule", posts[0])

	// No-match: wrong source, wrong tag, disabled wake.
	empty, err := sched.FireExternal(ctx, "webhook", "github", "push", nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
	empty, err = sched.FireExternal(ctx, "webhook", "ranch", "issues", nil)
	require.NoError(t, err)
	assert.Empty(t, empty)

	// tag-less webhook wake matches the same source with any tag.
	w2 := &Wake{
		Name:           "ranch any",
		Kind:           KindWebhook,
		Spec:           map[string]interface{}{"source": "ranch"},
		Target:         forgeTarget(srv.URL, "agent-1"),
		PromptTemplate: "ranch event",
		Enabled:        true,
	}
	mustCreate(t, store, w2)
	empty, err = sched.FireExternal(ctx, "webhook", "ranch", "issues", nil)
	require.NoError(t, err)
	require.Len(t, empty, 1, "tag-less wake matches any event_tag")

	// kind "event": source carries the event type.
	w3 := &Wake{
		Name:           "deploy finished",
		Kind:           KindEvent,
		Spec:           map[string]interface{}{"type": "deploys.finished"},
		Target:         forgeTarget(srv.URL, "agent-1"),
		PromptTemplate: "deploy {{.payload.deploy}}",
		Enabled:        true,
	}
	mustCreate(t, store, w3)
	results, err = sched.FireExternal(ctx, "event", "deploys.finished", "",
		map[string]interface{}{"deploy": "42"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].OK)

	// Unsupported fire kind is an error.
	_, err = sched.FireExternal(ctx, "bogus", "x", "", nil)
	assert.Error(t, err)
}

// TestJobCompletedEventEndToEnd submits a real trivial workflow through
// a real engine (PG stores), lets the engine's terminal-state event
// flow through the scheduler's event lane, and asserts the event-kind
// wake fired with a fully-rendered prompt (wake name + payload job id).
func TestJobCompletedEventEndToEnd(t *testing.T) {
	db := wakeDB(t)
	ctx := context.Background()

	forg := newFakeForge("agent-1")
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()

	pstore := primitive.NewPGStore(db)
	jobStore := job.NewPGStore(db)
	store := NewStore(db)

	// A trivial workflow: one forge_conversation step (no wait).
	wf := &primitive.Workflow{Name: "trivial wake workflow"}
	require.NoError(t, pstore.CreateWorkflow(ctx, wf))
	require.NoError(t, pstore.CreateWorkflowStep(ctx, &primitive.WorkflowStep{
		WorkflowID: wf.ID,
		StepOrder:  0,
		StepType:   "forge_conversation",
		Config: map[string]interface{}{
			"forge_url": srv.URL, "api_key": "sk_forge_test",
			"agent_id": "agent-1", "content": "trivial step",
		},
	}))

	runtime := agent.NewRuntime(pstore, jobStore)
	wasmExec := engine.NewWASMExecutor(nil, pstore, runtime, nil)
	eng := engine.NewEngine(pstore, jobStore, runtime, wasmExec, engine.Config{Workers: 2})
	require.NoError(t, eng.Start(ctx))
	defer eng.Stop()

	sched := NewScheduler(store, eng)
	sched.Start()
	defer sched.Stop()

	w := &Wake{
		Name:           "reporter",
		Kind:           KindEvent,
		Spec:           map[string]interface{}{"type": "job.completed"},
		Target:         forgeTarget(srv.URL, "agent-1"),
		PromptTemplate: "wake {{.wake_name}}: job {{.payload.job_id}} status {{.payload.status}}",
		Enabled:        true,
	}
	mustCreate(t, store, w)

	submitted, err := eng.SubmitJob(ctx, wf.ID, map[string]interface{}{"prompt": "hi"})
	require.NoError(t, err)

	// Wait for the event lane to fire the wake (engine worker ->
	// job.completed sink -> bus -> matching wake -> forge post).
	deadline := time.Now().Add(20 * time.Second)
	var logs []LogEntry
	for {
		logs, err = store.Log(ctx, w.ID, 100)
		if err == nil && len(logs) > 0 && logs[0].OK != nil && *logs[0].OK {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	require.NoError(t, err)
	require.NotEmpty(t, logs, "the job.completed wake must have fired (no wake_log rows)")
	assert.True(t, *logs[0].OK, "wake_log error: %v", logs[0].Error)
	require.NotNil(t, logs[0].Payload)
	assert.Equal(t, submitted.ID, logs[0].Payload["job_id"],
		"the fire payload must carry the completed job's id")

	// The forge got the rendered prompt: template vars populated.
	found := false
	for _, p := range forg.posted() {
		if strings.Contains(p, "wake reporter: job "+submitted.ID+" status completed") {
			found = true
		}
	}
	assert.True(t, found, "forge must receive the rendered wake prompt, posts: %v", forg.posted())
}

// TestJobKindWake matches a job-kind wake on workflow_id + status.
func TestJobKindWake(t *testing.T) {
	db := wakeDB(t)
	ctx := context.Background()
	store := NewStore(db)
	forg := newFakeForge("agent-1")
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()

	wf := &primitive.Workflow{Name: "the workflow"}
	pstore := primitive.NewPGStore(db)
	require.NoError(t, pstore.CreateWorkflow(ctx, wf))

	w := &Wake{
		Name:           "workflow watcher",
		Kind:           KindJob,
		Spec:           map[string]interface{}{"workflow_id": wf.ID, "status": "completed"},
		Target:         forgeTarget(srv.URL, "agent-1"),
		PromptTemplate: "wf {{.payload.workflow_id}} done",
		Enabled:        true,
	}
	mustCreate(t, store, w)

	sched := NewScheduler(store, nil)
	// The completed event for THIS workflow fires the job-kind wake...
	sched.HandleEvent(ctx, Event{
		Type: "job.completed", Source: "mule",
		Payload: map[string]interface{}{"job_id": "j1", "workflow_id": wf.ID, "status": "completed"},
	})
	// ...and the failed one does not (status mismatch).
	sched.HandleEvent(ctx, Event{
		Type: "job.failed", Source: "mule",
		Payload: map[string]interface{}{"job_id": "j2", "workflow_id": wf.ID, "status": "failed"},
	})
	posts := forg.posted()
	require.Len(t, posts, 1, "only the completed event may fire, posts: %v", posts)
	assert.Contains(t, posts[0], "wf "+wf.ID+" done")
}

func TestRateLimitDropsThirdFire(t *testing.T) {
	db := wakeDB(t)
	ctx := context.Background()
	store := NewStore(db)
	forg := newFakeForge("agent-1")
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()

	w := &Wake{
		Name:             "flood",
		Kind:             KindCron,
		Spec:             map[string]interface{}{"expr": "* * * * *"},
		Target:           forgeTarget(srv.URL, "agent-1"),
		PromptTemplate:   "flood {{.fired_at}}",
		Enabled:          true,
		RateLimitPerHour: 2,
	}
	mustCreate(t, store, w)
	_, err := db.Exec(`UPDATE wakes SET next_due_at = NOW() - INTERVAL '60 seconds' WHERE id = $1`, w.ID)
	require.NoError(t, err)

	sched := NewScheduler(store, nil)

	// Fire 1 (scan lane) and fire 2 (direct): both go through.
	require.Equal(t, 1, sched.ScanDue(ctx))
	res2, err := sched.FireWake(ctx, w, "cron", nil)
	require.NoError(t, err)
	assert.True(t, res2.OK, "second fire within the limit must submit: %s", res2.Error)
	assert.Len(t, forg.posted(), 2)

	// Fire 3: dropped, logged ok=false error='rate_limited', no submit.
	res3, err := sched.FireWake(ctx, w, "cron", nil)
	require.NoError(t, err)
	assert.True(t, res3.RateLimited)
	assert.Equal(t, "rate_limited", res3.Error)
	assert.Len(t, forg.posted(), 2, "rate-limited fire must NOT submit")

	logs, err := store.Log(ctx, w.ID, 100)
	require.NoError(t, err)
	require.Len(t, logs, 3, "all three attempts (2 ok + 1 dropped) must be in wake_log")
	// Newest first.
	require.NotNil(t, logs[0].OK)
	assert.False(t, *logs[0].OK)
	require.NotNil(t, logs[0].Error)
	assert.Equal(t, "rate_limited", *logs[0].Error)
	assert.True(t, *logs[1].OK)
	assert.True(t, *logs[2].OK)
}

func TestWorkflowTargetSubmitsJob(t *testing.T) {
	db := wakeDB(t)
	ctx := context.Background()
	store := NewStore(db)
	jobStore := job.NewPGStore(db)

	pstore := primitive.NewPGStore(db)
	wf := &primitive.Workflow{Name: "wake target workflow"}
	require.NoError(t, pstore.CreateWorkflow(ctx, wf))

	w := &Wake{
		Name:             "submitter",
		Kind:             KindEvent,
		Spec:             map[string]interface{}{"type": "x.y"},
		Target:           map[string]interface{}{"workflow_id": wf.ID},
		PromptTemplate:   "run {{.payload.thing}}",
		Enabled:          true,
		RateLimitPerHour: 24,
	}
	mustCreate(t, store, w)

	// Engine deliberately NOT started: SubmitJob must record a job row
	// (and only that — no workers drain it during this test).
	eng := engine.NewEngine(pstore, jobStore,
		agent.NewRuntime(pstore, jobStore), nil, engine.Config{Workers: 1})
	sched := NewScheduler(store, eng)

	res, err := sched.FireWake(ctx, w, "mule", map[string]interface{}{"thing": "42"})
	require.NoError(t, err)
	assert.True(t, res.OK)
	assert.NotEmpty(t, res.JobID, "workflow target must return the submitted job id")

	j, err := jobStore.GetJob(res.JobID)
	require.NoError(t, err, "a job row must exist for the wake submission")
	assert.Equal(t, wf.ID, j.WorkflowID)
	assert.Equal(t, "run 42", j.InputData["prompt"], "the rendered prompt is the job's prompt input")

	logs, err := store.Log(ctx, w.ID, 100)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.NotNil(t, logs[0].JobID)
	assert.Equal(t, res.JobID, *logs[0].JobID, "wake_log.job_id must be set")
}

func TestInertKindsNeverFire(t *testing.T) {
	db := wakeDB(t)
	ctx := context.Background()
	store := NewStore(db)
	forg := newFakeForge("agent-1")
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()

	// memory + agent_signal wakes: accepted (validated + stored)...
	mem := &Wake{
		Name:             "memory nudge",
		Kind:             KindMemory,
		Spec:             map[string]interface{}{"agent_id": "agent-1", "predicate": "recall(\"x\")"},
		Target:           forgeTarget(srv.URL, "agent-1"),
		PromptTemplate:   "remembered",
		Enabled:          true,
		RateLimitPerHour: 24,
	}
	sig := &Wake{
		Name:             "signal nudge",
		Kind:             KindAgentSignal,
		Spec:             map[string]interface{}{"agent_id": "agent-1", "kinds": []interface{}{"handoff"}},
		Target:           forgeTarget(srv.URL, "agent-1"),
		PromptTemplate:   "signaled",
		Enabled:          true,
		RateLimitPerHour: 24,
	}
	require.NoError(t, ValidateSpec(KindMemory, mem.Spec))
	require.NoError(t, ValidateSpec(KindAgentSignal, sig.Spec))
	mustCreate(t, store, mem)
	mustCreate(t, store, sig)

	// ...but never fire from the cron/event lanes: even when due, the
	// cron lane ignores them (kind != 'cron'), and the event lane has
	// no event type to match. (The H4.5 forwarder is the ONLY path
	// that fires a memory wake, and it calls FireWake directly.)
	_, err := db.Exec(`UPDATE wakes SET next_due_at = NOW() - INTERVAL '1 hour' WHERE kind IN ('memory','agent_signal')`)
	require.NoError(t, err)
	sched := NewScheduler(store, nil)
	assert.Equal(t, 0, sched.ScanDue(ctx), "inert kinds must never be due-cron-fired")
	for _, ev := range []Event{
		{Type: "job.completed", Source: "mule", Payload: map[string]interface{}{"job_id": "j", "workflow_id": "", "status": "completed"}},
		{Type: "memory.match", Source: "mule", Payload: map[string]interface{}{}},
		{Type: "agent.signal", Source: "mule", Payload: map[string]interface{}{"kinds": []interface{}{"handoff"}}},
	} {
		sched.HandleEvent(ctx, ev)
	}
	assert.Empty(t, forg.posted(), "inert kinds must never fire, posts: %v", forg.posted())
	logs, err := store.Log(ctx, mem.ID, 100)
	require.NoError(t, err)
	assert.Empty(t, logs)

	// Invalid specs of the inert kinds are rejected.
	assert.Error(t, ValidateSpec(KindMemory, map[string]interface{}{"agent_id": "a"}))
	assert.Error(t, ValidateSpec(KindAgentSignal, map[string]interface{}{"agent_id": "a", "kinds": []interface{}{}}))
}

func TestFireFailureRecorded(t *testing.T) {
	db := wakeDB(t)
	ctx := context.Background()
	store := NewStore(db)

	// Target pointing at a dead forge: the fire fails, wake_log gets
	// ok=false with the error, and no panic.
	w := &Wake{
		Name:             "broken forge",
		Kind:             KindEvent,
		Spec:             map[string]interface{}{"type": "x"},
		Target:           forgeTarget("http://127.0.0.1:1", "agent-1"),
		PromptTemplate:   "hi",
		Enabled:          true,
		RateLimitPerHour: 24,
	}
	mustCreate(t, store, w)

	sched := NewScheduler(store, nil)
	res, err := sched.FireWake(ctx, w, "mule", nil)
	require.Error(t, err)
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "forge")

	logs, lerr := store.Log(ctx, w.ID, 100)
	require.NoError(t, lerr)
	require.Len(t, logs, 1)
	assert.False(t, *logs[0].OK)
	require.NotNil(t, logs[0].Error)
	assert.NotEmpty(t, *logs[0].Error)
}

// TestFireTemplateRenderError checks a broken prompt_template fails the
// fire (missing key) and records it, not <no value>.
func TestFireTemplateRenderError(t *testing.T) {
	db := wakeDB(t)
	ctx := context.Background()
	store := NewStore(db)
	forg := newFakeForge("agent-1")
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()

	w := &Wake{
		Name:             "bad template",
		Kind:             KindEvent,
		Spec:             map[string]interface{}{"type": "x"},
		Target:           forgeTarget(srv.URL, "agent-1"),
		PromptTemplate:   "{{.nonexistent_key}}",
		Enabled:          true,
		RateLimitPerHour: 24,
	}
	mustCreate(t, store, w)

	sched := NewScheduler(store, nil)
	res, err := sched.FireWake(ctx, w, "mule", nil)
	require.Error(t, err)
	assert.Contains(t, res.Error, "nonexistent_key")
	assert.Empty(t, forg.posted())

	logs, _ := store.Log(ctx, w.ID, 100)
	require.Len(t, logs, 1)
	assert.False(t, *logs[0].OK)
}

// ------------------------------------------------------------------
// F-002: cron cadence (next_due_at) must advance on EVERY fire —
// success, rate-limited drop, or failed dispatch — and consecutive
// failed dispatches must back off, never hot-loop the scan lane.
// ------------------------------------------------------------------

// TestCronAdvancePureLogic exercises the pure next_due_at math (no DB):
// rate-limited/successful fires land on the exact next cron slot,
// failed dispatches apply the 60 s floor then exponential doubling
// capped at FailBackoffMax, and a success resets the counter.
func TestCronAdvancePureLogic(t *testing.T) {
	sched := NewScheduler(nil, nil)
	firedAt := time.Unix(1789268400, 0).UTC().Add(13 * time.Second) // 03:00:13 Sunday

	// Daily-at-06:00 cron slot is far ahead of the backoff floor:
	// both a success (failed=false) and a first failure keep the slot.
	daily := &Wake{ID: "w-daily", Kind: KindCron, Spec: map[string]interface{}{"expr": "0 6 * * *"}}
	want, err := CronNext("0 6 * * *", firedAt)
	require.NoError(t, err)
	nd := sched.cronAdvance(daily, firedAt, false)
	require.NotNil(t, nd, "cron fires must always produce a next_due_at")
	assert.Equal(t, want.UTC(), *nd, "successful fire lands on the next cron slot")
	sched.bumpFailures(daily.ID) // recordFailure's order: advance, then bump
	nd = sched.cronAdvance(daily, firedAt, true)
	require.NotNil(t, nd)
	assert.Equal(t, want.UTC(), *nd, "failure with a far cron slot keeps the slot")
	sched.resetFailures(daily.ID)

	// Every-minute cron: the backoff dominates the next slot.
	everyMin := &Wake{ID: "w-min", Kind: KindCron, Spec: map[string]interface{}{"expr": "* * * * *"}}
	nd = sched.cronAdvance(everyMin, firedAt, false)
	require.NotNil(t, nd)
	assert.Equal(t, firedAt.Truncate(time.Minute).Add(time.Minute), *nd,
		"success on '* * * * *' is the next full minute")

	for i, wantBackoff := range []time.Duration{
		60 * time.Second,  // 1st consecutive failure: floor
		120 * time.Second, // 2nd: doubled
		240 * time.Second, // 3rd
	} {
		nd = sched.cronAdvance(everyMin, firedAt, true)
		require.NotNil(t, nd)
		assert.Equal(t, firedAt.Add(wantBackoff), *nd,
			"failure #%d must use the %s backoff floor", i+1, wantBackoff)
		sched.bumpFailures(everyMin.ID)
	}
	// Cap: after many consecutive failures, backoff is FailBackoffMax.
	for i := 0; i < 20; i++ {
		sched.bumpFailures(everyMin.ID)
	}
	nd = sched.cronAdvance(everyMin, firedAt, true)
	require.NotNil(t, nd)
	assert.Equal(t, firedAt.Add(FailBackoffMax), *nd, "backoff is capped")
	// A success resets the counter: back off from the floor again.
	sched.resetFailures(everyMin.ID)
	nd = sched.cronAdvance(everyMin, firedAt, true)
	require.NotNil(t, nd)
	assert.Equal(t, firedAt.Add(FailBackoffFloor), *nd, "success resets the backoff")

	// Missing/unparseable expr on a cron wake must NOT leave it due:
	// advance by the floor instead of returning nil.
	broken := &Wake{ID: "w-broken", Kind: KindCron, Spec: map[string]interface{}{"expr": "bogus"}}
	nd = sched.cronAdvance(broken, firedAt, false)
	require.NotNil(t, nd, "cron wake with a broken expr must still advance")
	assert.Equal(t, firedAt.Add(FailBackoffFloor), *nd)

	// Non-cron kinds have no schedule to advance.
	ev := &Wake{ID: "w-ev", Kind: KindEvent, Spec: map[string]interface{}{"type": "x"}}
	assert.Nil(t, sched.cronAdvance(ev, firedAt, false))
	assert.Nil(t, sched.cronAdvance(ev, firedAt, true))
}

// TestRateLimitedCronFireAdvancesNextDue reproduces F-002 end to end:
// a cron wake whose fire is dropped by the per-hour rate limiter must
// still advance next_due_at to the next cron slot (and keep its
// wake_log row), so the scan lane cannot re-fire it every tick.
func TestRateLimitedCronFireAdvancesNextDue(t *testing.T) {
	db := wakeDB(t)
	ctx := context.Background()
	store := NewStore(db)
	forg := newFakeForge("agent-1")
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()

	w := &Wake{
		Name:             "dependabot",
		Kind:             KindCron,
		Spec:             map[string]interface{}{"expr": "0 6 * * *"},
		Target:           forgeTarget(srv.URL, "agent-1"),
		PromptTemplate:   "check dependabot",
		Enabled:          true,
		RateLimitPerHour: 1,
	}
	mustCreate(t, store, w)
	forceDue := func() {
		_, err := db.Exec(`UPDATE wakes SET next_due_at = NOW() - INTERVAL '60 seconds' WHERE id = $1`, w.ID)
		require.NoError(t, err)
	}

	sched := NewScheduler(store, nil)

	// Fire 1: within the limit, submits, advances.
	forceDue()
	require.Equal(t, 1, sched.ScanDue(ctx))

	// Fire 2: force-due again; now the limiter drops it — but the
	// schedule must STILL advance to the next 06:00 slot.
	forceDue()
	res, err := sched.FireWake(ctx, w, "cron", nil)
	require.NoError(t, err)
	require.True(t, res.RateLimited, "second fire must be rate limited: %+v", res)

	refreshed, err := store.Get(ctx, w.ID)
	require.NoError(t, err)
	require.NotNil(t, refreshed.NextDueAt, "rate-limited fire must still advance next_due_at")
	assert.True(t, refreshed.NextDueAt.After(time.Now().UTC()),
		"next_due_at must be in the future, got %v", *refreshed.NextDueAt)
	assert.True(t, CronMatches("0 6 * * *", *refreshed.NextDueAt),
		"next_due_at must be a cron slot, got %v", *refreshed.NextDueAt)

	// The scan lane must NOT re-fire it (the F-002 hot loop).
	assert.Equal(t, 0, sched.ScanDue(ctx), "rate-limited wake must not stay due")
	assert.Len(t, forg.posted(), 1, "rate-limited fire must NOT submit")

	// Both attempts are in wake_log; the rate-limited one is ok=false.
	logs, err := store.Log(ctx, w.ID, 100)
	require.NoError(t, err)
	require.Len(t, logs, 2)
	require.NotNil(t, logs[0].OK)
	assert.False(t, *logs[0].OK)
	require.NotNil(t, logs[0].Error)
	assert.Equal(t, "rate_limited", *logs[0].Error)
	assert.True(t, *logs[1].OK)
}

// TestFailedCronDispatchBackoff proves a persistently failing cron
// dispatch cannot hot-loop the scan lane: each consecutive failure
// pushes next_due_at out by the exponential backoff, rescans stay
// silent, and a success resets the counter.
func TestFailedCronDispatchBackoff(t *testing.T) {
	db := wakeDB(t)
	ctx := context.Background()
	store := NewStore(db)
	forg := newFakeForge("agent-1")
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()

	deadTarget := forgeTarget("http://127.0.0.1:1", "agent-1")
	w := &Wake{
		Name:             "broken cron",
		Kind:             KindCron,
		// every minute: the cron slot is always <= 60 s out, so the
		// backoff floor always dominates it and the assertions below
		// measure the backoff itself
		Spec:             map[string]interface{}{"expr": "* * * * *"},
		Target:           deadTarget,
		PromptTemplate:   "ping",
		Enabled:          true,
		RateLimitPerHour: 24,
	}
	mustCreate(t, store, w)
	forceDue := func() {
		_, err := db.Exec(`UPDATE wakes SET next_due_at = NOW() - INTERVAL '60 seconds' WHERE id = $1`, w.ID)
		require.NoError(t, err)
	}
	setTarget := func(target map[string]interface{}) {
		b, err := json.Marshal(target)
		require.NoError(t, err)
		_, err = db.Exec(`UPDATE wakes SET target = $1::jsonb WHERE id = $2`, string(b), w.ID)
		require.NoError(t, err)
	}
	nextDue := func() *time.Time {
		refreshed, err := store.Get(ctx, w.ID)
		require.NoError(t, err)
		require.NotNil(t, refreshed.NextDueAt, "failed cron fire must still advance next_due_at")
		return refreshed.NextDueAt
	}

	sched := NewScheduler(store, nil)

	// Failure #1 via the scan lane: recorded, schedule pushed out by
	// at least the backoff floor, and rescans do not re-fire.
	forceDue()
	sched.ScanDue(ctx)
	require.GreaterOrEqual(t, nextDue().Sub(time.Now().UTC()), 55*time.Second,
		"first failure must respect the 60 s backoff floor")
	assert.Equal(t, 0, sched.ScanDue(ctx), "failing wake must not re-fire every tick")

	// Failure #2: backoff doubles (>= ~120 s).
	forceDue()
	before := time.Now().UTC()
	_, err := sched.FireWake(ctx, w, "cron", nil)
	require.Error(t, err, "dispatch to the dead forge must fail")
	assert.GreaterOrEqual(t, nextDue().Sub(before), 115*time.Second,
		"second consecutive failure must double the backoff")

	// Failure #3: doubles again (>= ~240 s).
	forceDue()
	before = time.Now().UTC()
	_, err = sched.FireWake(ctx, w, "cron", nil)
	require.Error(t, err)
	assert.GreaterOrEqual(t, nextDue().Sub(before), 235*time.Second,
		"third consecutive failure must double the backoff again")

	// Every failed dispatch kept a wake_log row (ok=false, error set).
	logs, err := store.Log(ctx, w.ID, 100)
	require.NoError(t, err)
	require.Len(t, logs, 3)
	for i, l := range logs {
		require.NotNil(t, l.OK, "log %d", i)
		assert.False(t, *l.OK)
		require.NotNil(t, l.Error, "log %d", i)
		assert.NotEmpty(t, *l.Error)
	}

	// Success resets the counter: fix the target, fire ok, break it
	// again — the next failure backs off from the floor, not 240 s.
	setTarget(forgeTarget(srv.URL, "agent-1"))
	forceDue()
	require.Equal(t, 1, sched.ScanDue(ctx))
	setTarget(deadTarget)
	forceDue()
	before = time.Now().UTC()
	_, err = sched.FireWake(ctx, w, "cron", nil)
	require.Error(t, err)
	since := nextDue().Sub(before)
	assert.GreaterOrEqual(t, since, 55*time.Second, "post-success failure uses the floor again")
	assert.Less(t, since, 115*time.Second, "counter must have reset on success")
}
