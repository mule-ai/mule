package wake

import (
	"fmt"
	"strings"
	"time"

	"github.com/mule-ai/mule/internal/engine"
)

// Wake mirrors the `wakes` table (migration 0014, Herd H3.3).
type Wake struct {
	ID               string
	OwnerID          string
	Name             string
	Kind             Kind
	Spec             map[string]interface{}
	Target           map[string]interface{}
	PromptTemplate   string
	Enabled          bool
	RateLimitPerHour int
	LastFiredAt      *time.Time
	NextDueAt        *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Kind is a wake's condition class.
//
// Cron / Webhook / Event / Job are live in H3. Memory and AgentSignal
// are registered and validated in H3 but INERT (never fire); H4 wires
// them to the memory store and the agent signal bus respectively.
type Kind string

const (
	KindCron        Kind = "cron"
	KindWebhook     Kind = "webhook"
	KindEvent       Kind = "event"
	KindJob         Kind = "job"
	KindMemory      Kind = "memory"
	KindAgentSignal Kind = "agent_signal"
)

// AllKinds lists every accepted kind value.
var AllKinds = []Kind{KindCron, KindWebhook, KindEvent, KindJob, KindMemory, KindAgentSignal}

// IsKind reports whether s is a known kind.
func IsKind(s string) (Kind, bool) {
	for _, k := range AllKinds {
		if k == Kind(s) {
			return k, true
		}
	}
	return "", false
}

// Inert reports whether the kind is accepted-but-not-wired in H3.
func (k Kind) Inert() bool { return k == KindMemory || k == KindAgentSignal }

// LogEntry mirrors one `wake_log` row (fire audit trail).
type LogEntry struct {
	WakeID  string
	FiredAt time.Time
	Payload map[string]interface{}
	JobID   *string
	OK      *bool
	Error   *string
}

// FireResult is the outcome of one fire-path invocation.
type FireResult struct {
	WakeID         string
	OK             bool
	RateLimited    bool
	JobID          string // set for workflow targets
	ConversationID string // set for forge-conversation targets
	Prompt         string // rendered prompt (empty when not rendered)
	Error          string
}

// Event is one wake event on the in-process bus.
//
//   - Type: "job.completed" / "job.failed" (mule engine) or any
//     external event type (POST /api/v1/wakes/fire, kind "event").
//   - Source: "mule" for engine events; the webhook source for
//     external webhook fires; the event type for external event fires.
//   - Tag: optional webhook event_tag.
//   - Payload: free-form event payload (template variable in prompts).
type Event struct {
	Type    string
	Source  string
	Tag     *string
	Payload map[string]interface{}
}

// specString reads a required string spec key (present, non-empty).
func specString(spec map[string]interface{}, key string) (string, bool) {
	s, ok := spec[key].(string)
	return strings.TrimSpace(s), ok && strings.TrimSpace(s) != ""
}

// ValidateSpec validates the kind-specific spec object.
func ValidateSpec(kind Kind, spec map[string]interface{}) error {
	if spec == nil {
		return fmt.Errorf("spec is required")
	}
	switch kind {
	case KindCron:
		expr, ok := specString(spec, "expr")
		if !ok {
			return fmt.Errorf("cron spec requires a non-empty \"expr\"")
		}
		if err := ValidateCronExpr(expr); err != nil {
			return fmt.Errorf("cron spec: %w", err)
		}
	case KindWebhook:
		source, ok := specString(spec, "source")
		if !ok {
			return fmt.Errorf("webhook spec requires a non-empty \"source\" (the ranch webhook source, e.g. \"github\")")
		}
		if tag, present := spec["tag"]; present && tag != nil {
			if ts, ok := tag.(string); !ok || strings.TrimSpace(ts) == "" {
				return fmt.Errorf("webhook spec \"tag\" must be a non-empty string")
			}
		}
		_ = source
	case KindEvent:
		if t, ok := specString(spec, "type"); !ok {
			return fmt.Errorf("event spec requires a non-empty \"type\" (e.g. \"job.completed\")")
		} else {
			_ = t
		}
	case KindJob:
		if wf, ok := specString(spec, "workflow_id"); !ok {
			return fmt.Errorf("job spec requires a non-empty \"workflow_id\"")
		} else {
			_ = wf
		}
		status, ok := specString(spec, "status")
		if !ok || (status != "completed" && status != "failed") {
			return fmt.Errorf("job spec \"status\" must be \"completed\" or \"failed\"")
		}
	case KindMemory:
		// H4.5 stub: validated + stored, never fires.
		if _, ok := specString(spec, "agent_id"); !ok {
			return fmt.Errorf("memory spec requires a non-empty \"agent_id\"")
		}
		if _, ok := specString(spec, "predicate"); !ok {
			return fmt.Errorf("memory spec requires a non-empty \"predicate\" (H4 stub)")
		}
	case KindAgentSignal:
		// H4.6 stub: validated + stored, never fires.
		if _, ok := specString(spec, "agent_id"); !ok {
			return fmt.Errorf("agent_signal spec requires a non-empty \"agent_id\"")
		}
		kinds, ok := spec["kinds"].([]interface{})
		if !ok || len(kinds) == 0 {
			return fmt.Errorf("agent_signal spec requires a non-empty \"kinds\" array (H4 stub)")
		}
		for _, k := range kinds {
			if ks, ok := k.(string); !ok || strings.TrimSpace(ks) == "" {
				return fmt.Errorf("agent_signal spec \"kinds\" entries must be non-empty strings")
			}
		}
	default:
		return fmt.Errorf("unknown wake kind %q", kind)
	}
	return nil
}

// ValidateTarget validates the target object: at least one of
//
//   - target.workflow_id: string — the wake submits a mule job on that
//     workflow (input: {"prompt": <rendered>, "payload": <fire payload>});
//   - target.forge_conversation: an H3.2 forge step config map
//     ({forge_url, api_key, agent_id|conversation_id}) — the wake posts
//     the rendered prompt as a message to the resolved conversation.
//
// An agent_id WITHOUT forge_url/api_key is not addressable in v1: the
// forge credentials ride in the target (same as the H3.2 step config;
// a shared fleet forge config is the documented upgrade path).
func ValidateTarget(target map[string]interface{}) error {
	if target == nil {
		return fmt.Errorf("target is required")
	}
	if wf, ok := target["workflow_id"]; ok {
		if ws, ok := wf.(string); !ok || strings.TrimSpace(ws) == "" {
			return fmt.Errorf("target.workflow_id must be a non-empty string")
		}
		return nil
	}
	if fc, ok := target["forge_conversation"]; ok {
		fm, ok := fc.(map[string]interface{})
		if !ok {
			return fmt.Errorf("target.forge_conversation must be an object")
		}
		if _, err := engine.ParseForgeSubmission(fm); err != nil {
			return fmt.Errorf("target.forge_conversation: %w", err)
		}
		return nil
	}
	return fmt.Errorf("target requires workflow_id or forge_conversation")
}
