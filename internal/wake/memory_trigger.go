package wake

// Herd H4.5: the memory trigger forwarder lane.
//
// Forge's episode capture (H4.2) scans each agent's active watch
// beliefs at episode insert; a hit past the watch's cooldown writes a
// row to forge's `memory_trigger_queue` (migration 024). This lane
// runs on the MULE host (separate process, separate database — forge
// and mule do NOT share a cluster in the lab, so the plan's
// "mule polls a forge-written DB table" becomes an HTTP pull with the
// same 30 s cadence):
//
//  1. every 30 s, for each agent id in `FORGE_TRIGGER_AGENTS`:
//     GET {base}/api/v1/agents/{id}/memory/triggers/pending?limit=50
//     (Bearer `FORGE_TRIGGER_KEY`, 10 s timeout);
//  2. for each pending row:
//     - stale (> MemoryTriggerStaleAfter old) → warn + ACK (a wake for
//       a 7-hour-old episode is not useful; the queue must advance);
//     - no `watch.wake_id` in the payload → warn + ACK (a
//       misconfigured belief must not wedge the queue; there is no
//       wake to log a skip against, so the note is the log line);
//     - wake id unknown to this mule's `wakes` table → warn + ACK;
//     - otherwise: look the wake up locally (by id, fleet-level
//       background context — no owner scoping, like the cron lane)
//       and fire it IN-PROCESS via Scheduler.FireWake (NOT the
//       POST /api/v1/wakes/fire ingress — this is the same binary),
//       source "memory_trigger";
//  3. after a successful (or rate-limited) fire: ACK with
//     POST {base}/api/v1/agents/{id}/memory/triggers/{tid}/consumed.
//
// Delivery is AT-LEAST-ONCE, not exactly-once: if the fire submitted
// but the ACK failed (or mule died between the two), the next poll
// re-fires. FireWake is fire-and-forget per H3.3, so the retry cost
// is a duplicate message to the target, bounded by the wake's
// rate_limit_per_hour guardrail. A fire that FAILED to submit (error
// from FireWake) is NOT acked — it retries next poll. A fire that the
// wake's own rate limiter DROPPED IS acked: the guardrail
// deliberately throttled it, and re-polling every 30 s would just
// re-trigger the limiter.
//
// Config: `FORGE_TRIGGER_BASE` (forge API base), `FORGE_TRIGGER_KEY`
// (Bearer key with owner-or-admin access to every listed agent),
// `FORGE_TRIGGER_AGENTS` (comma-separated forge agent ids; empty ⇒
// the lane is disabled — no loop, no HTTP).

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// MemoryTriggerPollInterval is the forwarder's poll period (30 s,
// same cadence as the cron lane).
const MemoryTriggerPollInterval = 30 * time.Second

// MemoryTriggerRequestTimeout bounds each forge HTTP call.
const MemoryTriggerRequestTimeout = 10 * time.Second

// MemoryTriggerStaleAfter: pending rows older than this are skipped
// (acked without firing) — a wake for a much older episode is not
// useful, and the queue must still advance.
const MemoryTriggerStaleAfter = 6 * time.Hour

// MemoryTriggerFireSource is the fire source recorded for every
// memory-trigger fire (wake_log + prompt template `.source`).
const MemoryTriggerFireSource = "memory_trigger"

// MemoryTriggerConfig is the lane's configuration (from the
// FORGE_TRIGGER_* environment variables, read at boot — the same
// pattern the server uses for MULE_ENCRYPTION_SECRET).
type MemoryTriggerConfig struct {
	// BaseURL is forge's API base (FORGE_TRIGGER_BASE).
	BaseURL string
	// APIKey is the Bearer key (FORGE_TRIGGER_KEY).
	APIKey string
	// Agents are the forge agent ids to poll (FORGE_TRIGGER_AGENTS,
	// comma-separated). Empty ⇒ the lane is disabled.
	Agents []string
}

// NewMemoryTriggerForwarderConfigFromEnv reads the FORGE_TRIGGER_*
// variables.
// An empty FORGE_TRIGGER_AGENTS leaves Agents nil (lane disabled).
func NewMemoryTriggerForwarderConfigFromEnv() MemoryTriggerConfig {
	cfg := MemoryTriggerConfig{
		BaseURL: strings.TrimSpace(os.Getenv("FORGE_TRIGGER_BASE")),
		APIKey:  os.Getenv("FORGE_TRIGGER_KEY"),
	}
	if list := strings.TrimSpace(os.Getenv("FORGE_TRIGGER_AGENTS")); list != "" {
		for _, id := range strings.Split(list, ",") {
			if id = strings.TrimSpace(id); id != "" {
				cfg.Agents = append(cfg.Agents, id)
			}
		}
	}
	return cfg
}

// forgeTriggerRow is one pending row of forge's memory_trigger_queue
// as returned by GET /agents/:id/memory/triggers/pending.
type forgeTriggerRow struct {
	ID        string                 `json:"id"`
	AgentID   string                 `json:"agent_id"`
	CreatedAt time.Time              `json:"created_at"`
	Payload   map[string]interface{} `json:"payload"`
}

// MemoryTriggerForwarder is the H4.5 lane: it polls the forge
// trigger queue and fires the corresponding mule wakes in-process.
type MemoryTriggerForwarder struct {
	cfg          MemoryTriggerConfig
	store        *Store
	sched        *Scheduler
	pollInterval time.Duration
	httpClient   *http.Client

	mu      sync.Mutex
	started bool
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

// NewMemoryTriggerForwarder creates the forwarder. `sched` must be
// the live scheduler (its FireWake is the in-process fire path).
func NewMemoryTriggerForwarder(store *Store, sched *Scheduler, cfg MemoryTriggerConfig) *MemoryTriggerForwarder {
	return &MemoryTriggerForwarder{
		cfg:          cfg,
		store:        store,
		sched:        sched,
		pollInterval: MemoryTriggerPollInterval,
		httpClient: &http.Client{
			Timeout: MemoryTriggerRequestTimeout,
		},
	}
}

// Enabled reports whether the lane will run (base URL + at least one
// agent configured).
func (f *MemoryTriggerForwarder) Enabled() bool {
	return f.cfg.BaseURL != "" && len(f.cfg.Agents) > 0
}

// Start launches the poll loop. Disabled configs (empty
// FORGE_TRIGGER_AGENTS) log and no-op — no loop, no HTTP. Idempotent.
func (f *MemoryTriggerForwarder) Start() {
	f.mu.Lock()
	if f.started {
		f.mu.Unlock()
		return
	}
	f.started = true
	if !f.Enabled() {
		f.mu.Unlock()
		log.Printf("memory trigger lane disabled (set FORGE_TRIGGER_BASE + FORGE_TRIGGER_AGENTS to enable)")
		return
	}
	f.stopCh = make(chan struct{})
	f.mu.Unlock()

	f.wg.Add(1)
	go f.pollLoop()
	log.Printf("memory trigger lane started (%d agent(s), poll every %s)", len(f.cfg.Agents), f.pollInterval)
}

// Stop halts the poll loop and waits for it to drain.
func (f *MemoryTriggerForwarder) Stop() {
	f.mu.Lock()
	if !f.started {
		f.mu.Unlock()
		return
	}
	f.started = false
	var stop chan struct{}
	if f.stopCh != nil {
		close(f.stopCh)
		stop = f.stopCh
		f.stopCh = nil
	}
	f.mu.Unlock()
	f.wg.Wait()
	if stop != nil {
		log.Printf("memory trigger lane stopped")
	}
}

func (f *MemoryTriggerForwarder) pollLoop() {
	defer f.wg.Done()
	f.PollOnce(context.Background())
	ticker := time.NewTicker(f.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-f.stopCh:
			return
		case <-ticker.C:
			f.PollOnce(context.Background())
		}
	}
}

// PollOnce runs one poll cycle over every configured agent (exposed
// for the tests; the loop is just a timer around this).
func (f *MemoryTriggerForwarder) PollOnce(ctx context.Context) {
	for _, agentID := range f.cfg.Agents {
		rows, err := f.fetchPending(ctx, agentID)
		if err != nil {
			log.Printf("memory trigger lane: agent %s: fetch pending: %v", agentID, err)
			continue
		}
		for _, row := range rows {
			f.processRow(ctx, agentID, row)
		}
	}
}

// fetchPending pulls the agent's unconsumed trigger rows.
func (f *MemoryTriggerForwarder) fetchPending(ctx context.Context, agentID string) ([]forgeTriggerRow, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		f.cfg.BaseURL+"/api/v1/agents/"+agentID+"/memory/triggers/pending?limit=50", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+f.cfg.APIKey)
	req.Header.Set("Accept", "application/json")
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("forge returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		Triggers []forgeTriggerRow `json:"triggers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode pending triggers: %w", err)
	}
	return out.Triggers, nil
}

// processRow handles one pending trigger row (see the package doc for
// the full decision table).
func (f *MemoryTriggerForwarder) processRow(ctx context.Context, agentID string, row forgeTriggerRow) {
	if time.Since(row.CreatedAt) > MemoryTriggerStaleAfter {
		log.Printf("memory trigger %s (agent %s): row is %s old (> %s) — skipping, acking to advance the queue",
			row.ID, agentID, time.Since(row.CreatedAt).Round(time.Second), MemoryTriggerStaleAfter)
		f.markConsumed(ctx, agentID, row.ID)
		return
	}

	// The queue payload carries the belief's `watch` verbatim,
	// including its `wake_id` (the mule wake to fire).
	wakeID, _ := nestedString(row.Payload, "watch", "wake_id")
	if wakeID == "" {
		log.Printf("memory trigger %s (agent %s): watch has no wake_id — misconfigured belief; acking so it cannot block the queue (skipped: no wake_id)",
			row.ID, agentID)
		f.markConsumed(ctx, agentID, row.ID)
		return
	}

	// Fleet-level lookup (no owner scoping — the lane's context has no
	// principal, like the cron lane's).
	w, err := f.store.Get(ctx, wakeID)
	if err != nil {
		log.Printf("memory trigger %s (agent %s): wake %s not found in this mule's wakes table — acking (skipped: wake not found)",
			row.ID, agentID, wakeID)
		f.markConsumed(ctx, agentID, row.ID)
		return
	}

	res, err := f.sched.FireWake(ctx, w, MemoryTriggerFireSource, row.Payload)
	if err != nil {
		// Submission FAILED: leave the row unconsumed so the next
		// poll retries (at-least-once; a dead forge target must not
		// silently eat the trigger).
		log.Printf("memory trigger %s (agent %s): wake %s fire failed: %v (row stays pending, retried next poll)",
			row.ID, agentID, wakeID, err)
		return
	}
	if res.RateLimited {
		// The wake's own guardrail dropped the fire: ack it.
		// Re-polling every 30 s would just re-trigger the limiter.
		log.Printf("memory trigger %s (agent %s): wake %s rate-limited — dropped by the wake's guardrail, acking",
			row.ID, agentID, wakeID)
		f.markConsumed(ctx, agentID, row.ID)
		return
	}
	log.Printf("memory trigger %s (agent %s): fired wake %s (kind %s)", row.ID, agentID, wakeID, w.Kind)
	if !f.markConsumed(ctx, agentID, row.ID) {
		// Fire submitted, ACK failed: the row stays pending and the
		// next poll RE-FIRES it (at-least-once, documented above).
		log.Printf("memory trigger %s (agent %s): ACK failed — row stays pending; the wake will be re-fired on the next poll (at-least-once)",
			row.ID, agentID)
	}
}

// markConsumed posts the ACK. Returns true on a 2xx.
func (f *MemoryTriggerForwarder) markConsumed(ctx context.Context, agentID, triggerID string) bool {
	url := f.cfg.BaseURL + "/api/v1/agents/" + agentID + "/memory/triggers/" + triggerID + "/consumed"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader("{}"))
	if err != nil {
		log.Printf("memory trigger %s: build ACK request: %v", triggerID, err)
		return false
	}
	req.Header.Set("Authorization", "Bearer "+f.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.httpClient.Do(req)
	if err != nil {
		log.Printf("memory trigger %s: ACK: %v", triggerID, err)
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("memory trigger %s: ACK returned %d", triggerID, resp.StatusCode)
		return false
	}
	return true
}

// nestedString reads payload[key][sub] as a trimmed string.
func nestedString(payload map[string]interface{}, key, sub string) (string, bool) {
	inner, ok := payload[key].(map[string]interface{})
	if !ok {
		return "", false
	}
	s, ok := inner[sub].(string)
	return strings.TrimSpace(s), ok && strings.TrimSpace(s) != ""
}
