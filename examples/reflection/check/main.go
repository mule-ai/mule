//go:build ignore

// check — reflection workflow step 3 (Herd H4.4): the deterministic
// gate. Takes the reflection agent's proposed belief diffs (step 2's
// output, under the engine's "prompt" key), re-fetches the agent's
// current pending+active beliefs from forge, and runs the gate:
//
//   - duplicate: normalized-string equality against existing beliefs +
//     batch-local dedupe;
//   - contradiction: v1 opposite-signal heuristic (negation-marker flip
//     + Jaccard token overlap >= 0.4);
//   - routing: ops the proposals endpoint cannot express in v1
//     (strengthen/weaken/forget/supersede) always need a human;
//   - decay: active beliefs decay as confidence * exp(-lambda * age);
//     lambda_per_day defaults to 0.02.
//
// The gate logic itself is host-tested in
// internal/reflection/gate; this module is a thin
// stdin/stdout + host-HTTP shell.
//
// Input (merged module config + step input):
//
//	prompt         string  (required) the agent's final message
//	forge_url      string  (required)
//	forge_api_key  string  (required)
//	agent_id       string  (required)
//	lambda_per_day float   optional  default 0.02
//
// Output (JSON on stdout): reflmem.CheckOutput
// {safe: [Decision], needs_review: [Decision],
//  decayed_beliefs: [Decayed], existing_beliefs: N}
//
// Build: GOOS=wasip1 GOARCH=wasm go build -o check.wasm check/main.go

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/mule-ai/mule/internal/reflection/gate"
	"github.com/mule-ai/mule/internal/reflection/reflmem"
	"github.com/mule-ai/mule/internal/reflection/wasmhttp"
)

func fail(format string, args ...interface{}) {
	out, _ := json.Marshal(map[string]interface{}{
		"success": false,
		"error":   fmt.Sprintf(format, args...),
	})
	fmt.Fprintln(os.Stdout, string(out))
}

// fetchBeliefs returns the agent's beliefs for one status filter.
func fetchBeliefs(base, key, agentID, status string) ([]reflmem.Belief, error) {
	u := fmt.Sprintf("%s/agents/%s/memory/beliefs?status=%s", base, agentID, status)
	var resp struct {
		Beliefs []reflmem.Belief `json:"beliefs"`
	}
	st, err := wasmhttp.JSON("GET", u, "", key, &resp)
	switch {
	case st == 404:
		return nil, fmt.Errorf("agent %s not found (404)", agentID)
	case st < 200 || st >= 300:
		return nil, fmt.Errorf("beliefs?status=%s returned HTTP %d", status, st)
	case err != nil:
		return nil, err
	}
	return resp.Beliefs, nil
}

func main() {
	var in map[string]interface{}
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		fail("failed to decode input: %v", err)
		return
	}
	// The engine passes step 2's output as {"prompt": ...}. pull's
	// output is a MAP (kept for the agent's context); the agent step
	// overwrites it with its own TEXT output — so here "prompt" is the
	// agent's message. Unwrap is a no-op for that shape.
	if m, uerr := reflmem.UnwrapStepInput(in); uerr == nil {
		in = m
	}
	promptText, _ := in["prompt"].(string)
	if promptText == "" {
		fail("no agent prompt in input (step 2 must run first)")
		return
	}
	base, key, agentID, ok := reflmem.CredsFromInput(in)
	if !ok {
		fail("forge_url, forge_api_key and agent_id are required (module config or job input)")
		return
	}

	proposals, err := reflmem.ParseProposals(promptText)
	if err != nil {
		fail("could not parse proposals from the agent output: %v", err)
		return
	}

	// The gate runs against BOTH pending and active beliefs: a
	// proposal duplicating a pending belief would be rejected by forge
	// anyway, and the human should see WHY it was held.
	var existing []gate.Existing
	for _, status := range []string{"active", "pending"} {
		beliefs, err := fetchBeliefs(base, key, agentID, status)
		if err != nil {
			fail("fetching %s beliefs: %v", status, err)
			return
		}
		for _, b := range beliefs {
			existing = append(existing, gate.Existing{
				ID: b.ID, Content: b.Content,
				Confidence: b.Confidence, Status: b.Status,
				CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
			})
		}
	}

	lambda := float64(gate.DefaultLambdaPerDay)
	if l, ok := in["lambda_per_day"].(float64); ok && l >= 0 {
		lambda = l
	}

	gp := make([]gate.Proposed, len(proposals))
	for i, p := range proposals {
		gp[i] = gate.Proposed{
			Op: p.Op, Belief: p.Belief, Confidence: p.Confidence,
			Rationale: p.Rationale, SourceEpisodes: p.SourceEpisodes,
		}
	}
	// WASI preview1 has no wall clock: inside a module time.Now() is
	// pinned to the WASI epoch (2022-01-01). The engine exposes the
	// host time as mule_time in the merged input; module config or
	// job input may pin it (deterministic tests).
	now := time.Now()
	if s, ok := in["mule_time"].(string); ok && s != "" {
		if t, perr := time.Parse(time.RFC3339, s); perr == nil {
			now = t
		}
	}
	rep := gate.Gate(gp, existing, now, lambda)

	out := reflmem.CheckOutput{
		ExistingCount: len(existing),
	}
	for _, v := range rep.Safe {
		out.Safe = append(out.Safe, reflmem.Decision{
			Proposal: reflmem.Proposal{
				Op: v.Op, Belief: v.Belief, Confidence: v.Confidence,
				Rationale: v.Rationale, SourceEpisodes: v.SourceEpisodes,
			},
			Reason: v.Reason, Duplicate: v.Duplicate,
		})
	}
	for _, v := range rep.NeedsReview {
		out.NeedsReview = append(out.NeedsReview, reflmem.Decision{
			Proposal: reflmem.Proposal{
				Op: v.Op, Belief: v.Belief, Confidence: v.Confidence,
				Rationale: v.Rationale, SourceEpisodes: v.SourceEpisodes,
			},
			Reason: v.Reason, Duplicate: v.Duplicate,
		})
	}
	for _, d := range rep.Decayed {
		out.DecayedBeliefs = append(out.DecayedBeliefs, reflmem.Decayed{
			ID: d.ID, Content: d.Content, OldConf: d.OldConf,
			NewConf: d.NewConf, AgeDays: d.AgeDays,
			LastChanged: d.AgeAnchor.Format(time.RFC3339),
		})
	}
	b, _ := json.Marshal(out)
	fmt.Fprintln(os.Stdout, string(b))
}
