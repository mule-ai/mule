//go:build ignore

// submit — reflection workflow step 4 (Herd H4.4): POSTs the gated
// belief diffs to forge's proposals endpoint.
//
// What gets submitted (v1 governance path, documented in
// examples/reflection/README.md):
//
//   - SAFE adds: as-is.
//   - NEEDS_REVIEW diffs (non-exact-duplicate): ALSO submitted, with
//     their gate reason prefixed to the rationale as
//     "[needs human review] <reason>" — they become pending beliefs a
//     human sees on a memory_review card and can Keep/Forget. This is
//     the H4.4 governance bridge: the proposal endpoint is add-only,
//     so forget/supersede/contradiction verdicts surface as pending
//     beliefs whose review the human acts on by hand.
//   - EXACT duplicates: NOT resubmitted (forge's own near-duplicate
//     gate would reject them); they stay in the job output with their
//     "duplicate of belief <id>" reason, which is the actionable bit.
//   - DECAYED beliefs: report-only in the job output. A "weaken" of an
//     existing belief would be rejected as a duplicate, so v1 does not
//     submit them — the human applies the drop on the card.
//
// Input (merged module config + step input): the check module's
// output (engine envelope: {"prompt": <check output object>}), plus
//
//	forge_url / forge_api_key / agent_id  (required)
//	actor                                  optional, default
//	                                            "reflection"
//
// Output (JSON on stdout): reflmem.SubmitOutput — the per-belt
// accepted/rejected results from forge, the skipped exact duplicates,
// and the decay report. This map is the job's final output_data.
//
// Creds flow (v1): forge credentials live in the MODULE CONFIG of
// pull/check/submit (seed.sh sets them), mirrored in the pull output
// for transparency. A secrets table + api_key_ref is the documented
// upgrade path (same stance as the H3.2 forge_conversation step).
//
// Build: GOOS=wasip1 GOARCH=wasm go build -o submit.wasm submit/main.go

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/mule-ai/mule/internal/reflection/reflmem"
	"github.com/mule-ai/mule/internal/reflection/wasmhttp"
)

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func fail(format string, args ...interface{}) {
	out, _ := json.Marshal(map[string]interface{}{
		"success": false,
		"error":   fmt.Sprintf(format, args...),
	})
	fmt.Fprintln(os.Stdout, string(out))
}

// cleanEpisodes drops non-uuid source references: forge rejects the
// whole item on one malformed uuid, and a hallucinated episode id is
// less useful than none.
func cleanEpisodes(ep []string) []string {
	out := make([]string, 0, len(ep))
	for _, e := range ep {
		if uuidRe.MatchString(e) {
			out = append(out, e)
		}
	}
	return out
}

type belt struct {
	Content        string   `json:"content"`
	Confidence     float64  `json:"confidence"`
	Rationale      string   `json:"rationale"`
	SourceEpisodes []string `json:"source_episodes"`
}

func main() {
	var in map[string]interface{}
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		fail("failed to decode input: %v", err)
		return
	}
	// Unwrap the engine envelope: the check step's output arrives as
	// {"prompt": <check output object>}.
	in, _ = reflmem.UnwrapStepInput(in)
	base, key, agentID, ok := reflmem.CredsFromInput(in)
	if !ok {
		fail("forge_url, forge_api_key and agent_id are required (module config or job input)")
		return
	}
	var checkOut reflmem.CheckOutput
	if err := json.Unmarshal(marshal(in), &checkOut); err != nil {
		fail("check output not decodable: %v", err)
		return
	}

	var belts []belt
	skipped := []string{}
	for _, d := range checkOut.Safe {
		belts = append(belts, belt{
			Content:        d.Belief,
			Confidence:     d.Confidence,
			Rationale:      strings.TrimSpace(d.Rationale),
			SourceEpisodes: cleanEpisodes(d.SourceEpisodes),
		})
	}
	for _, d := range checkOut.NeedsReview {
		if d.Duplicate {
			skipped = append(skipped, fmt.Sprintf("%s: %s", d.Belief, d.Reason))
			continue
		}
		belts = append(belts, belt{
			Content:        d.Belief,
			Confidence:     d.Confidence,
			Rationale:      fmt.Sprintf("[needs human review] %s: %s", d.Reason, strings.TrimSpace(d.Rationale)),
			SourceEpisodes: cleanEpisodes(d.SourceEpisodes),
		})
	}

	out := reflmem.SubmitOutput{
		AgentID:        agentID,
		NeedsReview:    len(checkOut.NeedsReview),
		DecayedBeliefs: checkOut.DecayedBeliefs,
		Skipped:        skipped,
	}

	if len(belts) == 0 {
		// Nothing to submit is a SUCCESSFUL reflection (the agent may
		// have found nothing learnable), not a failure.
		b, _ := json.Marshal(out)
		fmt.Fprintln(os.Stdout, string(b))
		return
	}

	actor := "reflection"
	if a, ok := in["actor"].(string); ok && a != "" {
		actor = a
	}
	body, _ := json.Marshal(map[string]interface{}{
		"actor":   actor,
		"beliefs": belts,
	})
	u := fmt.Sprintf("%s/agents/%s/memory/beliefs/proposals", base, agentID)
	var resp struct {
		Results []json.RawMessage `json:"results"`
	}
	status, err := wasmhttp.JSON("POST", u, string(body), key, &resp)
	if err != nil {
		fail("proposals POST: %v", err)
		return
	}
	if status < 200 || status >= 300 {
		fail("proposals POST returned HTTP %d", status)
		return
	}
	for _, r := range resp.Results {
		out.Results = append(out.Results, r)
		var one struct {
			Accepted bool `json:"accepted"`
		}
		if jerr := json.Unmarshal(r, &one); jerr == nil {
			if one.Accepted {
				out.Accepted++
			} else {
				out.Rejected++
			}
		}
	}
	out.Submitted = len(belts)
	b, _ := json.Marshal(out)
	fmt.Fprintln(os.Stdout, string(b))
}

func marshal(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}
