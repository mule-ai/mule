//go:build ignore

// pull — reflection workflow step 1 (Herd H4.4).
//
// Pulls the agent's recent episodes + current active beliefs from the
// forge memory API and emits them (with the forge credentials carried
// forward, v1 creds flow) as the next step's input.
//
// Input (merged module config + job input; input wins):
//
//	forge_url       string   required  e.g. http://10.10.199.29:8110
//	forge_api_key   string   required  sk_forge_... (Bearer)
//	agent_id        string   required  the forge agent to reflect on
//	recall_queries  []string optional  recall queries for episode search
//	k               int      optional  per-query episode/belief limit
//	                                (default 50; forge clamps 1..50)
//	since           string   optional  RFC3339; keep only episodes
//	                                created at/after this instant
//
// Output (JSON on stdout): reflmem.PullOutput
// {forge_url, forge_api_key, agent_id, since?, recall_queries, k,
//  episodes: [{id, summary, source, created_at, score}],
//  beliefs:  [{id, content, kind, confidence, rationale,
//              source_episodes, status, created_at, updated_at}],
//  generated_at}
//
// Failure mode: emits {"success": false, "error": ...} — the engine
// turns that into a failed step (and a failed job), which is what we
// want: a reflection run that could not read memory should not
// silently reflect on nothing.
//
// Why recall queries instead of "list recent episodes": forge has no
// plain episode-list endpoint — GET /agents/:id/memory/search requires
// a query and ranks by embedding cosine over that query (k capped at
// 50). Reflection is recall-based: a small set of generic queries
// recovers the episodes that mention the agent's ongoing work. The
// episode-count trigger (H4.5) will replace this with exact counts.
//
// Build (same toolchain as examples/wasm/*):
//	GOOS=wasip1 GOARCH=wasm go build -o pull.wasm pull/main.go

package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/mule-ai/mule/internal/reflection/reflmem"
	"github.com/mule-ai/mule/internal/reflection/wasmhttp"
)

func nowTime(in map[string]interface{}) time.Time {
	// WASI preview1 has no wall clock (time.Now() is pinned to the WASI
	// epoch); the engine injects the host time as mule_time.
	if s, ok := in["mule_time"].(string); ok && s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t
		}
	}
	return time.Now()
}

func fail(format string, args ...interface{}) {
	out, _ := json.Marshal(map[string]interface{}{
		"success": false,
		"error":   fmt.Sprintf(format, args...),
	})
	fmt.Fprintln(os.Stdout, string(out))
}

func main() {
	var in map[string]interface{}
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		fail("failed to decode input: %v", err)
		return
	}
	base, key, agentID, ok := reflmem.CredsFromInput(in)
	if !ok {
		fail("forge_url, forge_api_key and agent_id are required (module config or job input)")
		return
	}

	k := 50
	if n, ok := in["k"].(float64); ok && n >= 1 {
		k = int(n)
		if k > 50 {
			k = 50
		}
	}
	queries := []string{"work session", "what happened today", "preferences and feedback"}
	if raw, ok := in["recall_queries"].([]interface{}); ok && len(raw) > 0 {
		queries = nil
		for _, q := range raw {
			if s, ok := q.(string); ok && s != "" {
				queries = append(queries, s)
			}
		}
	}
	if len(queries) == 0 {
		fail("no usable recall_queries")
		return
	}
	var since string
	if s, ok := in["since"].(string); ok {
		since = s
	}

	seen := map[string]bool{}
	var episodes []reflmem.Episode
	for _, q := range queries {
		u := fmt.Sprintf("%s/agents/%s/memory/search?q=%s&k=%d",
			base, agentID, url.QueryEscape(q), k)
		var resp struct {
			Episodes []reflmem.Episode `json:"episodes"`
		}
		status, err := wasmhttp.JSON("GET", u, "", key, &resp)
		if err != nil && status == 0 {
			fail("search %q: %v", q, err)
			return
		}
		switch {
		case status == 404:
			fail("agent %s not found on %s (404) — check agent_id", agentID, base)
			return
		case status == 503:
			fail("forge embedding endpoint unavailable (503) — memory search is recall-based and needs it; retry when the embedding endpoint answers")
			return
		case status < 200 || status >= 300:
			fail("search %q returned HTTP %d", q, status)
			return
		}
		if err != nil {
			fail("search %q: %v", q, err)
			return
		}
		for _, e := range resp.Episodes {
			if seen[e.ID] || (since != "" && e.CreatedAt.String() < since) {
				// RFC3339 same-format strings compare lexically.
				continue
			}
			seen[e.ID] = true
			episodes = append(episodes, e)
		}
	}

	// Current active beliefs (the context the reflection agent sees).
	bURL := fmt.Sprintf("%s/agents/%s/memory/beliefs?status=active", base, agentID)
	var beliefsResp struct {
		Beliefs []reflmem.Belief `json:"beliefs"`
	}
	status, err := wasmhttp.JSON("GET", bURL, "", key, &beliefsResp)
	if err != nil {
		fail("beliefs: %v", err)
		return
	}
	if status < 200 || status >= 300 {
		fail("beliefs returned HTTP %d", status)
		return
	}

	out := reflmem.PullOutput{
		ForgeURL:      base,
		ForgeAPIKey:   key,
		AgentID:       agentID,
		RecallQueries: queries,
		K:             k,
		Episodes:      episodes,
		Beliefs:       beliefsResp.Beliefs,
		GeneratedAt:   nowTime(in),
	}
	if since != "" {
		if t, perr := time.Parse(time.RFC3339, since); perr == nil {
			out.Since = &t
		}
	}
	b, _ := json.Marshal(out)
	fmt.Fprintln(os.Stdout, string(b))
}
