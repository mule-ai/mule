// Package reflmem holds the shared data types and pure parsing helpers
// for the Herd H4.4 reflection workflow (examples/reflection).
//
// The three shipped wasm modules (pull, check, submit) are single-file
// `//go:build ignore` main.go programs — each declares its own
// `//go:wasmimport` host-HTTP functions — but they all speak the same
// JSON shapes, and the agent-output parsing is subtle enough to deserve
// host-side unit tests. That is why the shapes live here: plain stdlib,
// no wasm imports, so `go build ./...` / `go vet ./...` on the host
// stay green and the parse logic is testable without wazero.
//
// Forge API reference (read-only grounding, forge crates/forge-api):
//
//	GET  /agents/:id/memory/search?q=&k=&scope=
//	     -> {"query", "scope", "episodes": [EpisodeHit], "beliefs": [BeliefHit]}
//	     (EpisodeHit = Episode + "score"; embedding-endpoint-down -> 503)
//	GET  /agents/:id/memory/beliefs?status=active|pending
//	     -> {"beliefs": [Belief]}
//	POST /agents/:id/memory/beliefs/proposals
//	     body {"actor"?, "beliefs": [{"content", "kind"?, "confidence"?,
//	          "rationale"?, "source_episodes": [uuid]}]}
//	     -> {"results": [{"accepted", "belief_id"?, "card_pushed"?,
//	          "rejected_reason"?}]}
//
// Auth on every call: `Authorization: Bearer sk_forge_...`.
package reflmem

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Episode is one forge `episodes` row as returned by memory search.
type Episode struct {
	ID        string          `json:"id"`
	Summary   string          `json:"summary"`
	Source    json.RawMessage `json:"source"`
	CreatedAt time.Time       `json:"created_at"`
	Score     *float64        `json:"score,omitempty"` // present on search hits
}

// Belief is one forge `beliefs` row.
type Belief struct {
	ID             string    `json:"id"`
	Content        string    `json:"content"`
	Kind           string    `json:"kind"`
	Confidence     float64   `json:"confidence"`
	Rationale      *string   `json:"rationale,omitempty"`
	SourceEpisodes []string  `json:"source_episodes"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Proposal is one belief diff proposed by the reflection agent (step 2).
// Op is add | strengthen | weaken | forget | supersede (H4.4 plan step 2
// shape). Only `add` is expressible end-to-end by the forge proposals
// endpoint in v1; the other ops are routed to needs_review by the
// deterministic gate (check) and surface as pending beliefs a human can
// apply manually.
type Proposal struct {
	Op             string   `json:"op"`
	Belief         string   `json:"belief"`
	Confidence     float64  `json:"confidence"`
	Rationale      string   `json:"rationale"`
	SourceEpisodes []string `json:"source_episodes"`
}

// PullOutput is step 1's (pull) output: the un-reflected episodes, the
// agent's current beliefs, plus the credentials carried forward so the
// later deterministic steps (check/submit) can reach forge without a
// secrets table (v1 creds flow — module config, documented in
// examples/reflection/README.md).
type PullOutput struct {
	ForgeURL      string     `json:"forge_url"`
	ForgeAPIKey   string     `json:"forge_api_key"`
	AgentID       string     `json:"agent_id"`
	Since         *time.Time `json:"since,omitempty"`
	RecallQueries []string   `json:"recall_queries"`
	K             int        `json:"k"`
	Episodes      []Episode  `json:"episodes"`
	Beliefs       []Belief   `json:"beliefs"`
	GeneratedAt   time.Time  `json:"generated_at"`
}

// CheckOutput is step 3's (check) output: the deterministic gate's
// verdicts plus the stale-belief decay report.
type CheckOutput struct {
	Safe           []Decision `json:"safe"`
	NeedsReview    []Decision `json:"needs_review"`
	DecayedBeliefs []Decayed  `json:"decayed_beliefs"`
	ExistingCount  int        `json:"existing_beliefs"`
}

// Decision is a proposed diff plus the gate's verdict reason.
type Decision struct {
	Proposal
	Reason string `json:"reason"`
	// Duplicate marks an exact (normalized) match against an existing
	// belief: such an item is NOT resubmitted by the submit step —
	// forge's own near-duplicate gate would reject it anyway.
	Duplicate bool `json:"duplicate,omitempty"`
}

// Decayed reports a stale belief whose confidence has decayed.
type Decayed struct {
	ID          string  `json:"belief_id"`
	Content     string  `json:"content"`
	OldConf     float64 `json:"old_confidence"`
	NewConf     float64 `json:"new_confidence"`
	AgeDays     float64 `json:"age_days"`
	LastChanged string  `json:"last_changed"`
}

// SubmitOutput is step 4's (submit) output = the job's final output.
type SubmitOutput struct {
	AgentID        string            `json:"agent_id"`
	Submitted      int               `json:"submitted"`
	Accepted       int               `json:"accepted"`
	Rejected       int               `json:"rejected"`
	Results        []json.RawMessage `json:"results"`
	Skipped        []string          `json:"skipped"`
	NeedsReview    int               `json:"needs_review_count"`
	DecayedBeliefs []Decayed         `json:"decayed_beliefs"`
}

// ParseProposals extracts the belief-diff JSON array from the reflection
// agent's final message (step 2 -> step 3 boundary). The agent is asked
// for STRICT JSON, but real models wrap it in prose or code fences, so
// this is tolerant:
//  1. if the text has a ``` fenced block, use its content;
//  2. for each candidate top-level '[' in the text, find the balanced
//     ']' (string-aware, so a `]` inside a JSON string cannot end the
//     scan early) and try to decode it — the first decodable array
//     wins, so leading prose that itself contains brackets
//     ("[not json] preamble ...") cannot poison the result;
//  3. decode the array into []Proposal.
//
// An empty array is a legitimate result (an agent that saw nothing
// learnable replies with `[]`), and bracket-free prose is treated as
// "no proposals". But if the text DID contain bracket candidates and
// none of them decoded, that is an error: a garbage-producing model
// should fail the job (visible in job output), not silently reflect
// nothing every night.
func ParseProposals(text string) ([]Proposal, error) {
	s := strings.TrimSpace(text)
	if i := strings.Index(s, "```"); i >= 0 {
		rest := s[i+len("```"):]
		// Drop an optional language marker line (```json).
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			rest = rest[nl+1:]
		}
		if j := strings.Index(rest, "```"); j >= 0 {
			s = strings.TrimSpace(rest[:j])
		}
	}
	// Try each top-level candidate '[' in order; the first candidate
	// whose balanced slice decodes as a []Proposal wins.
	candCount := 0
	i := strings.Index(s, "[")
	for i >= 0 {
		candCount++
		// Walk to the matching ']' (string-aware).
		depth, inString, escaped := 0, false, false
		end := -1
		for k := i; k < len(s); k++ {
			c := s[k]
			if inString {
				switch {
				case escaped:
					escaped = false
				case c == '\\':
					escaped = true
				case c == '"':
					inString = false
				}
				continue
			}
			switch c {
			case '"':
				inString = true
			case '[':
				depth++
			case ']':
				depth--
				if depth == 0 {
					end = k
				}
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			// Candidate '[' never balanced: record it and stop — a
			// truncated array means a truncated message.
			return nil, fmt.Errorf("proposals array is not closed: %q", s[i:])
		}
		var out []Proposal
		if err := json.Unmarshal([]byte(s[i:end+1]), &out); err == nil {
			return out, nil
		}
		// Not JSON — keep scanning after this candidate.
		j := strings.Index(s[i+1:], "[")
		if j < 0 {
			break
		}
		i = i + 1 + j
	}
	if candCount > 0 {
		return nil, fmt.Errorf("no decodable proposals JSON array found in agent output (%d candidates; %q…)", candCount, truncate(s, 120))
	}
	return []Proposal{}, nil
}

// truncate is a tiny display helper for error messages.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// UnwrapStepInput handles the engine's step envelope: each step's output
// is passed to the next step as {"prompt": <output>} while the module
// config keys (forge creds, lambda, …) sit as SIBLINGS of "prompt" in
// the same merged input. A map-valued "prompt" is therefore MERGED
// into the input (payload keys win on conflict) rather than replacing
// it — the creds ride along next to the payload. A string "prompt"
// (agent text) is left untouched so steps can read it. Every module
// accepts either the bare payload or the envelope, so a module can
// also be run standalone (job input) against the engine.
func UnwrapStepInput(in map[string]interface{}) (map[string]interface{}, error) {
	if in == nil {
		return map[string]interface{}{}, nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	if v, ok := in["prompt"]; ok {
		if m, ok := v.(map[string]interface{}); ok {
			for k2, v2 := range m {
				out[k2] = v2
			}
		}
	}
	return out, nil
}

// CredsFromInput resolves forge_url / forge_api_key / agent_id from a
// merged (module config + step input) map, with job input winning over
// module config (the engine merges config first, then input).
func CredsFromInput(in map[string]interface{}) (url, key, agent string, ok bool) {
	url, _ = in["forge_url"].(string)
	key, _ = in["forge_api_key"].(string)
	agent, _ = in["agent_id"].(string)
	ok = url != "" && key != "" && agent != ""
	return
}
