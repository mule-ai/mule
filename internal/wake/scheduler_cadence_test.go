package wake

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ------------------------------------------------------------------
// F-002 regression (no DB): cadence advance + failed-dispatch
// backoff at the FireWake/ScanDue level. A fake wakeStore records
// every RecordFire and the scheduler clock is injectable, so these
// run without PostgreSQL (the wakeDB tests in wake_test.go cover the
// same ground end to end when Postgres is available).
// ------------------------------------------------------------------

type recordedFire struct {
	wakeID  string
	payload map[string]interface{}
	jobID   *string
	ok      bool
	errMsg  *string
	nextDue *time.Time
}

type fakeWakeStore struct {
	mu       sync.Mutex
	due      []*Wake
	dueErr   error
	recent   int // CountRecentFires return value
	recorded []recordedFire
}

func (f *fakeWakeStore) DueCronWakes(ctx context.Context, now time.Time) ([]*Wake, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dueErr != nil {
		return nil, f.dueErr
	}
	var out []*Wake
	for _, w := range f.due {
		if w.NextDueAt != nil && !w.NextDueAt.After(now) {
			out = append(out, w)
		}
	}
	return out, nil
}

func (f *fakeWakeStore) MatchingWebhookWakes(ctx context.Context, source, tag string) ([]*Wake, error) {
	return nil, nil
}

func (f *fakeWakeStore) MatchingEventWakes(ctx context.Context, eventType, workflowID, status string) ([]*Wake, error) {
	return nil, nil
}

func (f *fakeWakeStore) CountRecentFires(ctx context.Context, wakeID string, now time.Time) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.recent, nil
}

func (f *fakeWakeStore) RecordFire(ctx context.Context, wakeID string, payload map[string]interface{}, jobID *string, ok bool, errMsg *string, nextDue *time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recorded = append(f.recorded, recordedFire{
		wakeID: wakeID, payload: payload, jobID: jobID, ok: ok, errMsg: errMsg, nextDue: nextDue,
	})
	return nil
}

func (f *fakeWakeStore) fires() []recordedFire {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]recordedFire, len(f.recorded))
	copy(out, f.recorded)
	return out
}

// newNoDBScheduler builds a Scheduler over a fake store (same-package
// test seam; the Scheduler.store field accepts any wakeStore).
func newNoDBScheduler(store wakeStore) *Scheduler {
	s := NewScheduler(nil, nil)
	s.store = store
	return s
}

func cronWakeNoDB(id, expr string) *Wake {
	return &Wake{
		ID:               id,
		OwnerID:          "test-owner",
		Name:             "wake-" + id,
		Kind:             KindCron,
		Spec:             map[string]interface{}{"expr": expr},
		PromptTemplate:   "hello {{.fired_at}}",
		Enabled:          true,
		RateLimitPerHour: 24,
	}
}

// (a) A rate-limited cron fire STILL advances next_due_at to the next
// cron slot (F-002: previously it stayed due, so the scan lane
// re-fired the wake every tick and flooded wake_log with
// rate_limited rows — the wake-mule-dependabot production symptom).
func TestRateLimitedCronFireAdvancesNextDueNoDB(t *testing.T) {
	base := time.Date(2025, 10, 19, 21, 39, 10, 0, time.UTC)
	store := &fakeWakeStore{recent: 5} // already over the limit
	sched := newNoDBScheduler(store)
	sched.now = func() time.Time { return base }

	w := cronWakeNoDB("wake-mule-dependabot", "0 6 * * *")
	w.RateLimitPerHour = 2

	res, err := sched.FireWake(context.Background(), w, "cron", nil)
	require.NoError(t, err)
	assert.True(t, res.RateLimited)
	assert.Equal(t, "rate_limited", res.Error)

	fires := store.fires()
	require.Len(t, fires, 1, "rate-limited fire must still get its wake_log row")
	rec := fires[0]
	assert.Equal(t, w.ID, rec.wakeID)
	assert.False(t, rec.ok)
	require.NotNil(t, rec.errMsg)
	assert.Equal(t, "rate_limited", *rec.errMsg)

	require.NotNil(t, rec.nextDue, "rate-limited cron fire MUST advance next_due_at (F-002)")
	assert.True(t, rec.nextDue.Equal(time.Date(2025, 10, 20, 6, 0, 0, 0, time.UTC)),
		"next_due_at must advance to the next cron slot, got %s", rec.nextDue)
	assert.True(t, rec.nextDue.After(base))
}

// The scan lane must not re-fire a rate-limited wake on subsequent
// ticks; it becomes due again only when the advanced slot passes.
func TestScanDueRateLimitedWakeNotRefiredNoDB(t *testing.T) {
	base := time.Date(2025, 10, 19, 21, 30, 10, 0, time.UTC)
	w := cronWakeNoDB("wake-a", "*/5 * * * *")
	w.RateLimitPerHour = 1
	dueAt := base.Add(-time.Minute)
	w.NextDueAt = &dueAt
	store := &fakeWakeStore{recent: 1, due: []*Wake{w}} // over the limit
	sched := newNoDBScheduler(store)
	now := base
	sched.now = func() time.Time { return now }
	ctx := context.Background()

	// Tick 1: due -> dropped by the limiter, but the schedule advances
	// to the next */5 slot (21:35:00).
	assert.Equal(t, 1, sched.ScanDue(ctx))
	fires := store.fires()
	require.Len(t, fires, 1)
	require.NotNil(t, fires[0].nextDue)
	advanced := *fires[0].nextDue
	assert.True(t, advanced.Equal(time.Date(2025, 10, 19, 21, 35, 0, 0, time.UTC)),
		"rate-limited scan fire must advance to the next cron slot, got %s", advanced)
	w.NextDueAt = &advanced // mirror what RecordFire persisted

	// Following ticks before the advanced slot: no re-fire (the old
	// F-002 hot loop re-fired every 30 s tick).
	now = now.Add(30 * time.Second)
	assert.Equal(t, 0, sched.ScanDue(ctx))
	now = now.Add(30 * time.Second)
	assert.Equal(t, 0, sched.ScanDue(ctx))
	assert.Len(t, store.fires(), 1, "rate-limited wake must not re-fire every tick")

	// Once the advanced due passes, it fires (drops) once more.
	now = advanced.Add(time.Second)
	assert.Equal(t, 1, sched.ScanDue(ctx))
	assert.Len(t, store.fires(), 2)
}

// (b) A failing dispatch never retries faster than the backoff floor,
// and the backoff grows exponentially across consecutive failures.
func TestFailedDispatchBackoffFloorNoDB(t *testing.T) {
	// Dead forge server -> PostMessage always fails.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()

	base := time.Date(2025, 10, 19, 21, 39, 10, 0, time.UTC)
	store := &fakeWakeStore{}
	sched := newNoDBScheduler(store)
	now := base
	sched.now = func() time.Time { return now }

	w := cronWakeNoDB("broken", "* * * * *") // cron slot closer than the floor
	w.Target = forgeTarget(deadURL, "agent-1")
	ctx := context.Background()

	// Failure 1: next_due_at pushed to at least firedAt + floor (60 s),
	// even though the raw cron slot (21:40:00) is only 50 s out.
	_, err := sched.FireWake(ctx, w, "cron", nil)
	require.Error(t, err, "dispatch to a dead target must fail")
	fires := store.fires()
	require.Len(t, fires, 1)
	require.NotNil(t, fires[0].nextDue, "failed cron fire must still advance next_due_at (F-002)")
	assert.False(t, fires[0].ok)
	require.NotNil(t, fires[0].errMsg)
	assert.False(t, fires[0].nextDue.Before(base.Add(FailBackoffFloor)),
		"first failure must respect the %s floor, got %s", FailBackoffFloor, fires[0].nextDue)

	// Failure 2 (consecutive): floor doubles to 120 s.
	now = *fires[0].nextDue
	_, err = sched.FireWake(ctx, w, "cron", nil)
	require.Error(t, err)
	fires = store.fires()
	require.Len(t, fires, 2)
	require.NotNil(t, fires[1].nextDue)
	assert.False(t, fires[1].nextDue.Before(now.Add(2*FailBackoffFloor)),
		"second consecutive failure must wait >= 2x floor, got %s from %s", fires[1].nextDue, now)

	// Failure 3 (consecutive): floor 240 s.
	now = *fires[1].nextDue
	_, err = sched.FireWake(ctx, w, "cron", nil)
	require.Error(t, err)
	fires = store.fires()
	require.Len(t, fires, 3)
	require.NotNil(t, fires[2].nextDue)
	assert.False(t, fires[2].nextDue.Before(now.Add(4*FailBackoffFloor)),
		"third consecutive failure must wait >= 4x floor, got %s from %s", fires[2].nextDue, now)

	// The backoff is capped.
	for i := 0; i < 20; i++ {
		sched.bumpFailures(w.ID)
	}
	assert.Equal(t, FailBackoffMax, sched.failBackoff(w.ID))
}

// A successful fire advances to exactly the next cron slot and resets
// the consecutive-failure counter, so the next isolated failure is
// back at the floor.
func TestSuccessResetsBackoffAndAdvancesToCronSlotNoDB(t *testing.T) {
	forg := newFakeForge("agent-1")
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()

	base := time.Date(2025, 10, 19, 21, 39, 10, 0, time.UTC)
	store := &fakeWakeStore{}
	sched := newNoDBScheduler(store)
	now := base
	sched.now = func() time.Time { return now }

	w := cronWakeNoDB("healthy", "0 6 * * *")
	w.Target = forgeTarget(srv.URL, "agent-1")
	ctx := context.Background()

	res, err := sched.FireWake(ctx, w, "cron", nil)
	require.NoError(t, err)
	require.True(t, res.OK)
	fires := store.fires()
	require.Len(t, fires, 1)
	require.NotNil(t, fires[0].nextDue)
	assert.True(t, fires[0].nextDue.Equal(time.Date(2025, 10, 20, 6, 0, 0, 0, time.UTC)),
		"successful cron fire advances to the next cron slot, got %s", fires[0].nextDue)
	assert.Len(t, forg.posted(), 1)

	// Simulate prior consecutive failures; a success resets them.
	sched.bumpFailures(w.ID)
	sched.bumpFailures(w.ID)
	require.Equal(t, 2, sched.failures[w.ID])

	now = base.Add(9 * time.Hour)
	res, err = sched.FireWake(ctx, w, "cron", nil)
	require.NoError(t, err)
	require.True(t, res.OK)
	assert.Equal(t, 0, sched.failures[w.ID], "success must reset the backoff counter")
	fires = store.fires()
	require.Len(t, fires, 2)
	require.NotNil(t, fires[1].nextDue)
	assert.True(t, fires[1].nextDue.Equal(time.Date(2025, 10, 21, 6, 0, 0, 0, time.UTC)))

	// An isolated failure after a success is back at the floor (60 s).
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	w.Target = forgeTarget(deadURL, "agent-1")
	_, err = sched.FireWake(ctx, w, "cron", nil)
	require.Error(t, err)
	fires = store.fires()
	require.Len(t, fires, 3)
	require.NotNil(t, fires[2].nextDue)
	assert.False(t, fires[2].nextDue.Before(now.Add(FailBackoffFloor)),
		"isolated failure after success must honor the floor, got %s", fires[2].nextDue)
}

// A cron wake with an unusable expr still advances (it can never sit
// permanently due), and non-cron wakes get no cadence advance.
func TestCronAdvanceEdgeCasesNoDB(t *testing.T) {
	base := time.Date(2025, 10, 19, 21, 39, 10, 0, time.UTC)
	store := &fakeWakeStore{recent: 3}
	sched := newNoDBScheduler(store)
	sched.now = func() time.Time { return base }

	// Broken expr + rate limited -> advances by the floor, not stuck.
	w := cronWakeNoDB("broken-expr", "not a cron")
	w.RateLimitPerHour = 1
	res, err := sched.FireWake(context.Background(), w, "cron", nil)
	require.NoError(t, err)
	assert.True(t, res.RateLimited)
	fires := store.fires()
	require.Len(t, fires, 1)
	require.NotNil(t, fires[0].nextDue, "even a broken-expr cron fire must advance (no hot loop)")
	assert.Equal(t, base.Add(FailBackoffFloor), *fires[0].nextDue)

	// Non-cron wake: no next_due_at to advance (behavior unchanged).
	store2 := &fakeWakeStore{}
	sched2 := newNoDBScheduler(store2)
	sched2.now = func() time.Time { return base }
	we := &Wake{
		ID: "evt", OwnerID: "test-owner", Name: "evt", Kind: KindEvent,
		Spec: map[string]interface{}{"type": "x.y"}, PromptTemplate: "hi",
		Enabled: true, RateLimitPerHour: 24,
	}
	// No target -> dispatch fails; non-cron -> nil nextDue.
	_, err = sched2.FireWake(context.Background(), we, "x.y", nil)
	require.Error(t, err)
	fires2 := store2.fires()
	require.Len(t, fires2, 1)
	assert.Nil(t, fires2[0].nextDue, "non-cron wakes have no cadence to advance")
}
