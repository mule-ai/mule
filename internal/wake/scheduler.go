package wake

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"text/template"
	"time"

	"github.com/mule-ai/mule/internal/engine"
)

// CronScanInterval is the cron-lane scan period (30 s, like
// engine.jobPoller's 1 s ticker but minute-resolution is enough: next
// fires are minute-granularity).
const CronScanInterval = 30 * time.Second

// fireTimeout bounds a single fire's submission (rate check, render,
// submit). Wakes are cheap fire-and-forget calls; nothing longer.
const fireTimeout = 30 * time.Second

// Failed-dispatch backoff (F-002): consecutive failed dispatches of
// the SAME wake are spaced by at least FailBackoffFloor, doubling per
// consecutive failure up to FailBackoffMax, so a persistently broken
// target cannot hot-loop the scan lane.
const (
	FailBackoffFloor = 60 * time.Second
	FailBackoffMax   = time.Hour
)

// EventBus is an in-process pub/sub bus: a bounded FIFO channel that
// drops the OLDEST event with a warning when full (a slow consumer
// must never block publishers; a full bus means the consumer is stuck
// and dropping the oldest is least harmful for proactivity).
type EventBus struct {
	ch chan Event
}

// NewEventBus creates a bus with the given buffer size.
func NewEventBus(size int) *EventBus {
	if size <= 0 {
		size = 1024
	}
	return &EventBus{ch: make(chan Event, size)}
}

// Publish enqueues ev without blocking. When the bus is full the
// oldest pending event is dropped with a warning.
func (b *EventBus) Publish(ev Event) {
	select {
	case b.ch <- ev:
		return
	default:
		select {
		case old := <-b.ch:
			log.Printf("wake event bus full: dropping oldest event %q to accept %q", old.Type, ev.Type)
		default:
		}
		select {
		case b.ch <- ev:
		default:
			log.Printf("wake event bus full: dropping new event %q", ev.Type)
		}
	}
}

// C exposes the bus channel to the scheduler's event loop.
func (b *EventBus) C() <-chan Event { return b.ch }

// Scheduler runs the two wake lanes:
//
//   - cron lane: every 30 s, scan enabled cron wakes whose next_due_at
//     has passed and fire them (advancing last_fired_at/next_due_at in
//     the same transaction as the wake_log row);
//   - event lane: consume the EventBus. The engine publishes
//     job.completed / job.failed terminal states (SetJobEventSink,
//     wired in Start); external pushes arrive via FireExternal (the
//     POST /api/v1/wakes/fire ingress for webhooks and event pushes).
//
// memory / agent_signal wakes are inert in H3 (registered, validated,
// stored; never fired — H4 wires them).
// wakeStore is the Scheduler's dependency surface on the PostgreSQL
// store (*Store implements it). Tests inject a fake to exercise the
// scheduling logic (cadence advance, backoff, rate limiting) without
// a database.
type wakeStore interface {
	DueCronWakes(ctx context.Context, now time.Time) ([]*Wake, error)
	MatchingWebhookWakes(ctx context.Context, source, tag string) ([]*Wake, error)
	MatchingEventWakes(ctx context.Context, eventType, workflowID, status string) ([]*Wake, error)
	CountRecentFires(ctx context.Context, wakeID string, now time.Time) (int, error)
	RecordFire(ctx context.Context, wakeID string, payload map[string]interface{}, jobID *string, ok bool, errMsg *string, nextDue *time.Time) error
}

type Scheduler struct {
	store        wakeStore
	engine       *engine.Engine // may be nil (tests; workflow targets then fail)
	bus          *EventBus
	scanInterval time.Duration
	now          func() time.Time // injectable clock (tests)

	mu      sync.Mutex
	started bool
	stopCh  chan struct{}
	wg      sync.WaitGroup

	failMu   sync.Mutex
	failures map[string]int // wake_id -> consecutive failed dispatches
}

// NewScheduler creates a scheduler. The engine is required for
// workflow targets and engine event publication.
func NewScheduler(store *Store, eng *engine.Engine) *Scheduler {
	return &Scheduler{
		store:        store,
		engine:       eng,
		bus:          NewEventBus(1024),
		scanInterval: CronScanInterval,
		now:          time.Now,
	}
}

// newSchedulerWithStore builds a scheduler over any wakeStore
// implementation (tests exercise the scheduling logic with a fake
// store and no database).
func newSchedulerWithStore(store wakeStore, eng *engine.Engine) *Scheduler {
	s := NewScheduler(nil, eng)
	s.store = store
	return s
}

// Store exposes the scheduler's store (handlers CRUD wakes through it).
func (s *Scheduler) Store() *Store {
	st, _ := s.store.(*Store)
	return st
}

// EventBus exposes the event bus (tests publish into it directly).
func (s *Scheduler) EventBus() *EventBus { return s.bus }

// Start registers the engine job-event sink and launches both lanes.
// Idempotent; call Stop once for shutdown.
func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	if s.engine != nil {
		s.engine.SetJobEventSink(s.onEngineJobEvent)
	}

	s.wg.Add(2)
	go s.scanLoop()
	go s.eventLoop()
	log.Printf("wake scheduler started (cron scan every %s)", s.scanInterval)
}

// Stop halts both lanes and waits for them to drain.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return
	}
	s.started = false
	close(s.stopCh)
	s.mu.Unlock()
	s.wg.Wait()
	log.Printf("wake scheduler stopped")
}

// onEngineJobEvent is the engine's JobEvent sink: terminal job states
// become bus events the event lane matches against event/job wakes.
func (s *Scheduler) onEngineJobEvent(ev engine.JobEvent) {
	status := "failed"
	if ev.Event == "job.completed" {
		status = "completed"
	}
	s.bus.Publish(Event{
		Type:   ev.Event,
		Source: "mule",
		Payload: map[string]interface{}{
			"job_id":      ev.JobID,
			"workflow_id": ev.WorkflowID,
			"status":      status,
		},
	})
}

func (s *Scheduler) scanLoop() {
	defer s.wg.Done()
	// Scan once at start: wakes that came due while mule was down
	// fire once on boot (catch-up), at most once per wake per due.
	s.ScanDue(context.Background())
	ticker := time.NewTicker(s.scanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.ScanDue(context.Background())
		}
	}
}

// ScanDue fires every enabled cron wake whose next_due_at <= now and
// returns the number it fired (ok or rate-limited-dropped).
func (s *Scheduler) ScanDue(ctx context.Context) int {
	wakes, err := s.store.DueCronWakes(ctx, s.now().UTC())
	if err != nil {
		log.Printf("wake cron scan: %v", err)
		return 0
	}
	n := 0
	for _, w := range wakes {
		res, err := s.FireWake(ctx, w, "cron", nil)
		if err != nil {
			log.Printf("wake %s (%s): fire failed: %v", w.ID, w.Name, err)
			continue
		}
		if res.RateLimited {
			log.Printf("wake %s (%s): fire dropped (rate limited)", w.ID, w.Name)
		}
		n++
	}
	if n > 0 {
		log.Printf("wake cron scan: fired %d due wake(s)", n)
	}
	return n
}

func (s *Scheduler) eventLoop() {
	defer s.wg.Done()
	for {
		select {
		case <-s.stopCh:
			return
		case ev := <-s.bus.C():
			s.HandleEvent(context.Background(), ev)
		}
	}
}

// HandleEvent runs the event lane for one bus event: matches enabled
// event/job wakes and fires each. Inert kinds are skipped.
func (s *Scheduler) HandleEvent(ctx context.Context, ev Event) {
	workflowID, _ := ev.Payload["workflow_id"].(string)
	status, _ := ev.Payload["status"].(string)
	wakes, err := s.store.MatchingEventWakes(ctx, ev.Type, workflowID, status)
	if err != nil {
		log.Printf("wake event lane: match failed for %q: %v", ev.Type, err)
		return
	}
	for _, w := range wakes {
		if w.Kind.Inert() {
			continue
		}
		s.fireMatched(ctx, w, ev.Source, ev.Payload)
	}
}

// FireExternal is the POST /api/v1/wakes/fire ingress. It matches
// enabled wakes against an EXTERNAL event and fires each:
//
//   - kind "webhook": source + optional tag match spec {source, tag?}
//     (the ranch webhook shape {source, event_tag, payload});
//   - kind "event": source carries the event TYPE and matches
//     spec {type} of event-kind wakes.
//
// It returns one FireResult per matched wake. Any valid API key may
// call it: it is an ingress, not a mutation.
func (s *Scheduler) FireExternal(ctx context.Context, kind, source, tag string, payload map[string]interface{}) ([]FireResult, error) {
	var wakes []*Wake
	var err error
	switch kind {
	case "webhook":
		if source == "" {
			return nil, errors.New("webhook fire requires a non-empty \"source\"")
		}
		wakes, err = s.store.MatchingWebhookWakes(ctx, source, tag)
	case "event":
		if source == "" {
			return nil, errors.New("event fire requires a non-empty \"source\" (the event type)")
		}
		wakes, err = s.store.MatchingEventWakes(ctx, source, "", "")
	default:
		return nil, fmt.Errorf("unsupported fire kind %q (want \"webhook\" or \"event\")", kind)
	}
	if err != nil {
		return nil, fmt.Errorf("wake match failed: %w", err)
	}
	results := []FireResult{}
	for _, w := range wakes {
		if w.Kind.Inert() {
			continue
		}
		res, err := s.FireWake(ctx, w, source, payload)
		if err != nil {
			res = FireResult{WakeID: w.ID, Error: err.Error()}
			log.Printf("wake %s (%s): fire failed: %v", w.ID, w.Name, err)
		}
		results = append(results, res)
	}
	return results, nil
}

// fireMatched is the event-lane fire with logging.
func (s *Scheduler) fireMatched(ctx context.Context, w *Wake, source string, payload map[string]interface{}) {
	res, err := s.FireWake(ctx, w, source, payload)
	if err != nil {
		log.Printf("wake %s (%s): fire failed: %v", w.ID, w.Name, err)
		return
	}
	if res.RateLimited {
		log.Printf("wake %s (%s): fire dropped (rate limited)", w.ID, w.Name)
		return
	}
	if res.JobID != "" {
		log.Printf("wake %s (%s): fired -> mule job %s", w.ID, w.Name, res.JobID)
	}
	if res.ConversationID != "" {
		log.Printf("wake %s (%s): fired -> forge conversation %s", w.ID, w.Name, res.ConversationID)
	}
}

// FireWake runs the full fire path for one wake:
//
//  1. rate-limit guardrail: if this wake's wake_log already has
//     rate_limit_per_hour rows in the trailing hour, the fire is
//     DROPPED and logged (ok=false, error='rate_limited') — no
//     submission. This is the "runaway proactivity" guardrail
//     (research §10.3), enforced in the control plane.
//  2. render prompt_template (Go text/template, missingkey=error) over
//     the fire payload map {wake_id, wake_name, kind, source, payload,
//     fired_at}.
//  3. submit: target.workflow_id -> engine.SubmitJob (input
//     {prompt, payload}); else target.forge_conversation ->
//     engine.ForgeSubmission.PostMessage (fire-and-forget, no turn
//     wait — wakes are cheap).
//  4. record wake_log {payload, job_id (workflow targets), ok, error}
//     and, for cron wakes, advance last_fired_at/next_due_at — in the
//     same transaction (RecordFire).
//
// Cron wakes advance next_due_at unconditionally — on success, on a
// rate-limited drop, and on submission failure (F-002): a fire that
// leaves the wake due re-fires on every scan tick. Failed dispatches
// additionally honor an exponential backoff floor (see cronAdvance),
// and the rate limit bounds a broken target to rate_limit_per_hour
// error rows per hour.
func (s *Scheduler) FireWake(ctx context.Context, w *Wake, source string, payload map[string]interface{}) (FireResult, error) {
	if payload == nil {
		payload = map[string]interface{}{}
	}
	firedAt := s.now().UTC()

	recent, err := s.store.CountRecentFires(ctx, w.ID, firedAt)
	if err != nil {
		return FireResult{WakeID: w.ID, Error: err.Error()}, err
	}
	if recent >= w.RateLimitPerHour {
		errMsg := "rate_limited"
		// A rate-limited cron fire STILL advances next_due_at (F-002):
		// otherwise the scan lane re-fires the wake on every tick while
		// the limiter drops it, flooding wake_log with rate_limited rows.
		if recErr := s.store.RecordFire(ctx, w.ID, payload, nil, false, &errMsg,
			s.cronAdvance(w, firedAt, false)); recErr != nil {
			log.Printf("wake %s: failed to record rate-limited drop: %v", w.ID, recErr)
		}
		return FireResult{WakeID: w.ID, RateLimited: true, Error: "rate_limited"}, nil
	}

	data := map[string]interface{}{
		"wake_id":   w.ID,
		"wake_name": w.Name,
		"kind":      string(w.Kind),
		"source":    source,
		"payload":   payload,
		"fired_at":  firedAt.Format(time.RFC3339),
	}
	prompt, err := renderFireTemplate(w.PromptTemplate, data)
	if err != nil {
		errMsg := err.Error()
		_ = s.store.RecordFire(ctx, w.ID, payload, nil, false, &errMsg,
			s.cronAdvance(w, firedAt, true))
		s.bumpFailures(w.ID)
		return FireResult{WakeID: w.ID, Error: err.Error()}, err
	}

	submitCtx, cancel := context.WithTimeout(ctx, fireTimeout)
	defer cancel()

	var jobID *string
	var convID string
	if wfID, _ := w.Target["workflow_id"].(string); wfID != "" {
		if s.engine == nil {
			return s.recordFailure(ctx, w, payload, firedAt, "no engine configured for workflow target")
		}
		j, err := s.engine.SubmitJob(submitCtx, wfID, map[string]interface{}{
			"prompt":  prompt,
			"payload": payload,
		})
		if err != nil {
			return s.recordFailure(ctx, w, payload, firedAt, fmt.Sprintf("failed to submit workflow job: %v", err))
		}
		jobID = &j.ID
	} else {
		fc, ok := w.Target["forge_conversation"].(map[string]interface{})
		if !ok {
			return s.recordFailure(ctx, w, payload, firedAt, "wake target has neither workflow_id nor forge_conversation")
		}
		sub, err := engine.ParseForgeSubmission(fc)
		if err != nil {
			return s.recordFailure(ctx, w, payload, firedAt, err.Error())
		}
		content := "[mule wake " + w.ID + "] " + prompt
		convID, err = sub.PostMessage(submitCtx, content)
		if err != nil {
			return s.recordFailure(ctx, w, payload, firedAt, err.Error())
		}
	}

	s.resetFailures(w.ID)
	nextDue := s.cronAdvance(w, firedAt, false)
	if err := s.store.RecordFire(ctx, w.ID, payload, jobID, true, nil, nextDue); err != nil {
		return FireResult{WakeID: w.ID, OK: true, JobID: deref(jobID), ConversationID: convID, Prompt: prompt,
			Error: "submission succeeded but wake_log record failed: " + err.Error()}, err
	}
	return FireResult{WakeID: w.ID, OK: true, JobID: deref(jobID), ConversationID: convID, Prompt: prompt}, nil
}

// recordFailure logs a failed submission (ok=false, error message).
// Cron wakes STILL advance next_due_at (F-002), pushed out by the
// failed-dispatch backoff so a persistently failing target cannot
// hot-loop the scan lane.
func (s *Scheduler) recordFailure(ctx context.Context, w *Wake, payload map[string]interface{}, firedAt time.Time, msg string) (FireResult, error) {
	err := errors.New(msg)
	errMsg := msg
	nextDue := s.cronAdvance(w, firedAt, true)
	s.bumpFailures(w.ID)
	if recErr := s.store.RecordFire(ctx, w.ID, payload, nil, false, &errMsg, nextDue); recErr != nil {
		log.Printf("wake %s: failed to record fire failure: %v", w.ID, recErr)
	}
	return FireResult{WakeID: w.ID, Error: msg}, err
}

// cronAdvance returns the next_due_at to persist with a fire of a
// cron wake (nil for non-cron kinds). It NEVER returns nil for a cron
// wake with a usable expr — successful, rate-limited-dropped, and
// failed fires all advance the cadence (F-002). On a failed dispatch
// the advance is pushed to at least firedAt + backoff (exponential
// from FailBackoffFloor, capped at FailBackoffMax); on success or a
// rate-limited drop it is exactly the next cron slot. A cron wake
// whose expr is missing/unparseable still advances by the backoff
// floor so it can never sit permanently due.
func (s *Scheduler) cronAdvance(w *Wake, firedAt time.Time, failed bool) *time.Time {
	if w.Kind != KindCron {
		return nil
	}
	var next time.Time
	if expr, _ := w.Spec["expr"].(string); expr != "" {
		if nd, err := CronNext(expr, firedAt); err == nil {
			next = nd.UTC()
		}
	}
	if failed {
		if floor := firedAt.Add(s.failBackoff(w.ID)); next.IsZero() || next.Before(floor) {
			next = floor
		}
	} else if next.IsZero() {
		next = firedAt.Add(FailBackoffFloor)
	}
	if next.IsZero() {
		return nil
	}
	return &next
}

// failBackoff returns the backoff for the NEXT consecutive failure of
// wakeID (60 s, 120 s, 240 s, ... capped at FailBackoffMax) without
// mutating the counter (bumpFailures drives the state).
func (s *Scheduler) failBackoff(wakeID string) time.Duration {
	s.failMu.Lock()
	defer s.failMu.Unlock()
	n := s.failures[wakeID] // count of failures already recorded
	backoff := FailBackoffFloor
	for i := 0; i < n && backoff < FailBackoffMax; i++ {
		backoff *= 2
	}
	if backoff > FailBackoffMax {
		backoff = FailBackoffMax
	}
	return backoff
}

// bumpFailures records one more consecutive failed dispatch of wakeID.
func (s *Scheduler) bumpFailures(wakeID string) {
	s.failMu.Lock()
	defer s.failMu.Unlock()
	if s.failures == nil {
		s.failures = map[string]int{}
	}
	s.failures[wakeID]++
}

// resetFailures clears the failed-dispatch counter on success.
func (s *Scheduler) resetFailures(wakeID string) {
	s.failMu.Lock()
	defer s.failMu.Unlock()
	delete(s.failures, wakeID)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// renderFireTemplate renders a Go text/template over the fire payload
// map. Missing keys are an error (a typo in the wake's prompt_template
// should fail the fire and show up in wake_log, not silently render
// <no value>).
func renderFireTemplate(tmplText string, data map[string]interface{}) (string, error) {
	tmpl, err := template.New("wake").Option("missingkey=error").Parse(tmplText)
	if err != nil {
		return "", fmt.Errorf("invalid prompt_template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render prompt_template: %w", err)
	}
	return buf.String(), nil
}
