package engine

// H4.4 reflection workflow acceptance (examples/reflection): the
// reflection loop is a real mule workflow —
//
//	1. wasm  pull   — forge memory API -> episodes + active beliefs
//	2. agent step   — cheap model proposes belief diffs (STRICT JSON)
//	3. wasm  check  — deterministic gate (dup / contradiction / decay)
//	4. wasm  submit — POST /agents/:id/memory/beliefs/proposals
//
// The agent step (2) cannot be faked in-process: it spawns
// `pi --mode rpc`. So the e2e job below substitutes a fixed-output
// stub (the hand-assembled wasmEchoModule from forge_test.go, and the
// shipped examples/reflection/stub module, tested separately via
// Execute) for step 2 — everything else runs the REAL wasm binaries
// through the REAL engine, against a fake forge (httptest). The
// real-model leg is the manual acceptance (see
// examples/reflection/README.md).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/internal/reflection/reflmem"
	"github.com/mule-ai/mule/pkg/job"
)

// ------------------------------------------------------------------
// fake forge: the memory API subset the reflection workflow speaks
// ------------------------------------------------------------------

type reflectionFakeForge struct {
	mu             sync.Mutex
	agentID        string
	episodes       []map[string]interface{}
	activeBeliefs  []map[string]interface{}
	pendingBeliefs []map[string]interface{}
	embeddingDown  bool
	posts          []map[string]interface{} // decoded proposals bodies
}

func newReflectionFakeForge(agentID string, episodes, active, pending []map[string]interface{}) *reflectionFakeForge {
	return &reflectionFakeForge{agentID: agentID, episodes: episodes, activeBeliefs: active, pendingBeliefs: pending}
}

func (f *reflectionFakeForge) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(r.URL.Path, "/")
		// /agents/:id/memory/...
		var id, rest string
		if len(parts) >= 5 && parts[1] == "agents" {
			id, rest = parts[2], strings.Join(parts[3:], "/")
		} else {
			w.WriteHeader(404)
			return
		}
		if id != f.agentID {
			w.WriteHeader(404)
			fmt.Fprint(w, `{"error":"agent not found"}`)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.Method == "GET" && rest == "memory/search":
			if f.embeddingDown {
				w.WriteHeader(503)
				fmt.Fprint(w, `{"error":"embedding unavailable"}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"query": r.URL.Query().Get("q"), "scope": "agent",
				"episodes": f.episodes, "beliefs": []interface{}{},
			})
		case r.Method == "GET" && rest == "memory/beliefs":
			var beliefs []map[string]interface{}
			switch r.URL.Query().Get("status") {
			case "pending":
				beliefs = f.pendingBeliefs
			case "active", "":
				beliefs = f.activeBeliefs
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"beliefs": beliefs})
		case r.Method == "POST" && rest == "memory/beliefs/proposals":
			var body struct {
				Actor   string            `json:"actor"`
				Beliefs []json.RawMessage `json:"beliefs"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(400)
				return
			}
			f.posts = append(f.posts, map[string]interface{}{
				"actor":   body.Actor,
				"beliefs": body.Beliefs,
			})
			results := make([]map[string]interface{}, len(body.Beliefs))
			for i := range body.Beliefs {
				results[i] = map[string]interface{}{
					"accepted": true, "belief_id": fmt.Sprintf("gen-%d", i), "card_pushed": true,
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": results})
		default:
			w.WriteHeader(404)
		}
	})
}

func (f *reflectionFakeForge) posted() []map[string]interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.posts
}

// ------------------------------------------------------------------
// wasm artifact build (dist/ is gitignored; built once, reused)
// ------------------------------------------------------------------

var reflBuildMu sync.Mutex

func buildReflectionWasm(t *testing.T, name string) []byte {
	t.Helper()
	distPath := filepath.Join("..", "..", "examples", "reflection", "dist", name+".wasm")
	if b, err := os.ReadFile(distPath); err == nil {
		return b
	}
	reflBuildMu.Lock()
	defer reflBuildMu.Unlock()
	if b, err := os.ReadFile(distPath); err == nil {
		return b
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(distPath), 0o755), "create dist dir")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", distPath,
		filepath.Join("examples", "reflection", name, "main.go"))
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "build %s.wasm:\n%s", name, out)
	b, err := os.ReadFile(distPath)
	require.NoError(t, err, "read built artifact")
	return b
}

func reflCreds(url string) map[string]interface{} {
	return map[string]interface{}{
		"forge_url": url, "forge_api_key": "sk_forge_test", "agent_id": "agent-x",
	}
}

func reflEpisode(id, summary, created string) map[string]interface{} {
	return map[string]interface{}{
		"id": id, "summary": summary,
		"source":     map[string]interface{}{"seq_range": []int{1, 5}},
		"created_at": created, "score": 0.8,
	}
}

func reflBelief(id, content string, conf float64, status, created string) map[string]interface{} {
	return map[string]interface{}{
		"id": id, "content": content, "kind": "preference",
		"confidence": conf, "rationale": "seeded",
		"source_episodes": []string{}, "status": status,
		"created_at": created, "updated_at": created,
	}
}

// ------------------------------------------------------------------
// e2e: pull -> (echo stand-in for the agent) -> check -> submit
// ------------------------------------------------------------------

// agentText is the fixed step-2 output: an agent-style final message
// with a fenced JSON diff array.
const agentText = "Reflection complete. Proposed belief diffs:\n" +
	"```json\n" +
	"[\n" +
	"  {\"op\": \"add\", \"belief\": \"the user wants brief replies\", \"confidence\": 0.9,\n" +
	"   \"rationale\": \"echoes the existing active belief (duplicate on purpose in this demo)\",\n" +
	"   \"source_episodes\": [\"00000000-0000-0000-0000-000000000001\"]},\n" +
	"  {\"op\": \"add\", \"belief\": \"the user deploys only after a staging pass\", \"confidence\": 0.8,\n" +
	"   \"rationale\": \"two episodes show staging-first deploys\",\n" +
	"   \"source_episodes\": [\"00000000-0000-0000-0000-000000000001\",\n" +
	"                        \"00000000-0000-0000-0000-000000000002\"]},\n" +
	"  {\"op\": \"forget\", \"belief\": \"the user wants long-form replies\", \"confidence\": 0.4,\n" +
	"   \"rationale\": \"superseded by the brief-replies pattern\"}\n" +
	"]\n" +
	"```\n" +
	"End of reflection."

func TestReflectionWorkflow_EndToEnd(t *testing.T) {
	created5dAgo := time.Now().Add(-5 * 24 * time.Hour).UTC().Format(time.RFC3339)
	createdNow := time.Now().UTC().Format(time.RFC3339)
	forge := newReflectionFakeForge("agent-x",
		[]map[string]interface{}{
			reflEpisode("00000000-0000-0000-0000-000000000001", "fixed the deploy script", created5dAgo),
			reflEpisode("00000000-0000-0000-0000-000000000002", "ran the release through staging", createdNow),
		},
		[]map[string]interface{}{
			reflBelief("aaaa0000-0000-0000-0000-000000000001", "the user wants brief replies", 0.9, "active", created5dAgo),
		},
		[]map[string]interface{}{
			reflBelief("bbbb0000-0000-0000-0000-000000000002", "a pending belief", 0.5, "pending", createdNow),
		},
	)
	srv := httptest.NewServer(forge.handler())
	defer srv.Close()

	credz := reflCreds(srv.URL)
	modPull, modCheck, modSubmit := "mod-pull", "mod-check", "mod-submit"
	credzCopy := func() map[string]interface{} {
		m := map[string]interface{}{}
		for k, v := range credz {
			m[k] = v
		}
		return m
	}
	echoID := "mod-echo"
	store := &MockPrimitiveStore{
		Workflows: []*primitive.Workflow{{ID: "wf-reflection", Name: "agent-reflection"}},
		WorkflowSteps: []*primitive.WorkflowStep{
			{ID: "s1", WorkflowID: "wf-reflection", StepOrder: 1, StepType: "wasm_module", WasmModuleID: &modPull},
			{ID: "s2", WorkflowID: "wf-reflection", StepOrder: 2, StepType: "wasm_module", WasmModuleID: &echoID},
			{ID: "s3", WorkflowID: "wf-reflection", StepOrder: 3, StepType: "wasm_module", WasmModuleID: &modCheck},
			{ID: "s4", WorkflowID: "wf-reflection", StepOrder: 4, StepType: "wasm_module", WasmModuleID: &modSubmit},
		},
		WasmModules: []*primitive.WasmModuleListItem{
			{ID: modPull, Name: "pull", Config: credzCopy()},
			{ID: echoID, Name: "echo"},
			{ID: modCheck, Name: "check", Config: credzCopy()},
			{ID: modSubmit, Name: "submit", Config: credzCopy()},
		},
		WasmModuleData: map[string][]byte{
			modPull:   buildReflectionWasm(t, "pull"),
			echoID:    wasmEchoModule(agentText),
			modCheck:  buildReflectionWasm(t, "check"),
			modSubmit: buildReflectionWasm(t, "submit"),
		},
	}
	eng, jobStore := forgeEngine(t, store)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	require.NoError(t, eng.Start(ctx))
	defer eng.Stop()

	jobRec, err := eng.SubmitJob(ctx, "wf-reflection", map[string]interface{}{"note": "e2e reflection run"})
	require.NoError(t, err)

	// Poll for terminal state (wasm steps each take a few seconds).
	deadline := time.Now().Add(60 * time.Second)
	for {
		j, gerr := jobStore.GetJob(jobRec.ID)
		require.NoError(t, gerr)
		if j.Status == job.StatusCompleted || j.Status == job.StatusFailed {
			break
		}
		require.False(t, time.Now().After(deadline), "job %s stuck in %s", j.ID, j.Status)
		time.Sleep(250 * time.Millisecond)
	}
	j, _ := jobStore.GetJob(jobRec.ID)
	require.Equal(t, job.StatusCompleted, j.Status, "job must complete; output: %v", j.OutputData)

	// Exactly one proposals POST; the gate must have:
	//  - submitted the clean add ("staging pass"),
	//  - submitted the forget with a "[needs human review]" prefix,
	//  - SKIPPED the exact duplicate of the active belief.
	posts := forge.posted()
	require.Len(t, posts, 1, "exactly one proposals call")
	var bodies []map[string]interface{}
	for _, p := range posts {
		for _, raw := range p["beliefs"].([]json.RawMessage) {
			var b map[string]interface{}
			require.NoError(t, json.Unmarshal(raw, &b))
			bodies = append(bodies, b)
		}
	}
	require.Len(t, bodies, 2, "safe add + needs-review forget, not the dup: %v", bodies)
	var sawSafe, sawForget bool
	for _, b := range bodies {
		content := b["content"].(string)
		switch {
		case strings.Contains(content, "staging pass"):
			assert.NotContains(t, b["rationale"].(string), "[needs human review]")
			sawSafe = true
		case strings.Contains(content, "long-form"):
			assert.Contains(t, b["rationale"].(string), "[needs human review]")
			sawForget = true
		default:
			t.Fatalf("unexpected submitted belief %q", content)
		}
	}
	assert.True(t, sawSafe, "clean add must be submitted")
	assert.True(t, sawForget, "forget (needs review) must be submitted with the prefix")
	for _, b := range bodies {
		assert.NotContains(t, b["content"].(string), "brief replies",
			"exact duplicate must NOT be resubmitted")
	}

	// Job output: submit's SubmitOutput with the decay report (the 5-day
	// active belief: 0.9 * exp(-0.1) ~ 0.815 -> reported). The engine
	// wraps the final step's output in the "prompt" envelope.
	od := j.OutputData
	if p, ok := od["prompt"].(map[string]interface{}); ok {
		od = p
	}
	var out struct {
		Submitted      int      `json:"submitted"`
		Accepted       int      `json:"accepted"`
		Rejected       int      `json:"rejected"`
		NeedsReview    int      `json:"needs_review_count"`
		Skipped        []string `json:"skipped"`
		DecayedBeliefs []struct {
			ID string `json:"belief_id"`
		} `json:"decayed_beliefs"`
	}
	require.NoError(t, json.Unmarshal(marshalJSON(od), &out))
	assert.Equal(t, 2, out.Submitted)
	assert.Equal(t, 2, out.Accepted)
	assert.Equal(t, 0, out.Rejected)
	assert.Equal(t, 2, out.NeedsReview) // dup + forget
	require.Len(t, out.Skipped, 1)
	assert.Contains(t, out.Skipped[0], "duplicate of existing belief")
	require.Len(t, out.DecayedBeliefs, 1)
	assert.Equal(t, "aaaa0000-0000-0000-0000-000000000001", out.DecayedBeliefs[0].ID)
}

func marshalJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("null")
	}
	return b
}

// ------------------------------------------------------------------
// step-level tests against the fake forge (real wasm via Execute)
// ------------------------------------------------------------------

func pullExecEnv(t *testing.T, forge *reflectionFakeForge) (*WASMExecutor, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(forge.handler())
	store := &MockPrimitiveStore{
		WasmModules: []*primitive.WasmModuleListItem{{ID: "mod-pull", Name: "pull"}},
		WasmModuleData: map[string][]byte{
			"mod-pull": buildReflectionWasm(t, "pull"),
		},
	}
	exec := NewWASMExecutor(nil, store, nil, nil)
	return exec, srv
}

func TestReflectionPull_ListsEpisodesAndBeliefs(t *testing.T) {
	createdNow := time.Now().UTC().Format(time.RFC3339)
	forge := newReflectionFakeForge("agent-x",
		[]map[string]interface{}{
			reflEpisode("00000000-0000-0000-0000-000000000001", "e1", createdNow),
			reflEpisode("00000000-0000-0000-0000-000000000002", "e2", createdNow),
		},
		[]map[string]interface{}{reflBelief("aaaa0000-0000-0000-0000-000000000001", "b1", 0.9, "active", createdNow)},
		nil,
	)
	exec, srv := pullExecEnv(t, forge)
	defer srv.Close()

	res, err := exec.Execute(context.Background(), "mod-pull", reflCreds(srv.URL), "")
	require.NoError(t, err)
	assert.Equal(t, true, res["success"], "stdout: %v", res["stdout"])
	out, ok := res["output"].(map[string]interface{})
	require.True(t, ok, "output is the PullOutput object: %v", res["output"])
	eps, ok := out["episodes"].([]interface{})
	require.True(t, ok && len(eps) == 2, "two episodes: %v", out["episodes"])
	blefs, ok := out["beliefs"].([]interface{})
	require.True(t, ok && len(blefs) == 1, "one active belief: %v", out["beliefs"])
	assert.Equal(t, srv.URL, out["forge_url"])
	assert.Equal(t, "sk_forge_test", out["forge_api_key"])
	assert.Equal(t, "agent-x", out["agent_id"])
}

func TestReflectionPull_404Agent(t *testing.T) {
	forge := newReflectionFakeForge("agent-x", nil, nil, nil)
	exec, srv := pullExecEnv(t, forge)
	defer srv.Close()

	in := reflCreds(srv.URL)
	in["agent_id"] = "missing-agent"
	res, err := exec.Execute(context.Background(), "mod-pull", in, "")
	require.NoError(t, err, "Execute returns the result map; failure is in-band")
	assert.Equal(t, false, res["success"])
	assert.Contains(t, res["stdout"].(string), "404")
}

func TestReflectionPull_EmbeddingDown(t *testing.T) {
	forge := newReflectionFakeForge("agent-x", nil, nil, nil)
	forge.embeddingDown = true
	exec, srv := pullExecEnv(t, forge)
	defer srv.Close()

	res, err := exec.Execute(context.Background(), "mod-pull", reflCreds(srv.URL), "")
	require.NoError(t, err)
	assert.Equal(t, false, res["success"])
	assert.Contains(t, res["stdout"].(string), "503")
}

func TestReflectionPull_SinceFilter(t *testing.T) {
	old := time.Now().Add(-30 * 24 * time.Hour).UTC().Format(time.RFC3339)
	recent := time.Now().UTC().Format(time.RFC3339)
	forge := newReflectionFakeForge("agent-x",
		[]map[string]interface{}{
			reflEpisode("00000000-0000-0000-0000-000000000001", "old episode", old),
			reflEpisode("00000000-0000-0000-0000-000000000002", "recent episode", recent),
		},
		nil, nil,
	)
	exec, srv := pullExecEnv(t, forge)
	defer srv.Close()

	in := reflCreds(srv.URL)
	in["since"] = time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	res, err := exec.Execute(context.Background(), "mod-pull", in, "")
	require.NoError(t, err)
	assert.Equal(t, true, res["success"])
	out := res["output"].(map[string]interface{})
	eps := out["episodes"].([]interface{})
	require.Len(t, eps, 1, "only the recent episode survives the since filter: %v", eps)
	assert.Contains(t, fmt.Sprint(eps[0]), "recent episode")
}

// ------------------------------------------------------------------
// check: the gate against the (fake) live belief set
// ------------------------------------------------------------------

func TestReflectionCheck_GatesProposals(t *testing.T) {
	created5dAgo := time.Now().Add(-5 * 24 * time.Hour).UTC().Format(time.RFC3339)
	createdNow := time.Now().UTC().Format(time.RFC3339)
	forge := newReflectionFakeForge("agent-x", nil,
		[]map[string]interface{}{
			reflBelief("aaaa0000-0000-0000-0000-000000000001", "the user wants brief replies", 0.9, "active", createdNow),
			reflBelief("cccc0000-0000-0000-0000-000000000003", "the agent sends the weekly report on friday", 0.8, "active", created5dAgo),
		},
		nil,
	)
	srv := httptest.NewServer(forge.handler())
	defer srv.Close()

	prompt := `[
  {"op": "add", "belief": "the user wants brief replies", "confidence": 0.9},
  {"op": "add", "belief": "the agent should stop sending the weekly report"},
  {"op": "add", "belief": "the user prefers tabs over spaces", "confidence": 0.7},
  {"op": "forget", "belief": "the user wants long-form replies"}
]`
	store := &MockPrimitiveStore{
		WasmModules: []*primitive.WasmModuleListItem{{ID: "mod-check", Name: "check"}},
		WasmModuleData: map[string][]byte{
			"mod-check": buildReflectionWasm(t, "check"),
		},
	}
	exec := NewWASMExecutor(nil, store, nil, nil)
	in := reflCreds(srv.URL)
	in["prompt"] = prompt
	res, err := exec.Execute(context.Background(), "mod-check", in, "")
	require.NoError(t, err)
	assert.Equal(t, true, res["success"], "stdout: %v", res["stdout"])
	out := res["output"].(map[string]interface{})
	safe, _ := out["safe"].([]interface{})
	rev, _ := out["needs_review"].([]interface{})
	decayed, _ := out["decayed_beliefs"].([]interface{})
	require.Len(t, safe, 1, "only the clean add is safe: %v", safe)
	require.Len(t, rev, 3, "dup + contradiction + forget op: %v", rev)
	require.Len(t, decayed, 1, "the 5-day-old belief decays: %v", decayed)
	revStr := fmt.Sprint(rev)
	assert.Contains(t, revStr, "duplicate of existing belief")
	assert.Contains(t, revStr, "contradiction")
	assert.Contains(t, revStr, "op forget")
}

// ------------------------------------------------------------------
// submit: exact POST body shape + per-belt results
// ------------------------------------------------------------------

func TestReflectionSubmit_PostsGatedDiffs(t *testing.T) {
	forge := newReflectionFakeForge("agent-x", nil,
		[]map[string]interface{}{reflBelief("aaaa0000-0000-0000-0000-000000000001", "x", 0.5, "active", time.Now().UTC().Format(time.RFC3339))},
		nil,
	)
	srv := httptest.NewServer(forge.handler())
	defer srv.Close()

	checkOut := map[string]interface{}{
		"safe": []interface{}{
			map[string]interface{}{
				"op": "add", "belief": "the user deploys only after a staging pass",
				"confidence": 0.8, "rationale": "two episodes",
				"source_episodes": []string{"00000000-0000-0000-0000-000000000001"},
			},
		},
		"needs_review": []interface{}{
			map[string]interface{}{
				"op": "forget", "belief": "the user wants long-form replies",
				"confidence": 0.4, "rationale": "superseded",
				"reason": "op forget needs human review (v1 proposals endpoint is add-only)",
			},
			map[string]interface{}{
				"op": "add", "belief": "the user wants brief replies",
				"confidence": 0.9, "reason": "duplicate of existing belief aaaa0000-0000-0000-0000-000000000001",
				"duplicate": true,
			},
		},
		"decayed_beliefs":  []interface{}{},
		"existing_beliefs": 1,
	}
	store := &MockPrimitiveStore{
		WasmModules: []*primitive.WasmModuleListItem{{ID: "mod-submit", Name: "submit"}},
		WasmModuleData: map[string][]byte{
			"mod-submit": buildReflectionWasm(t, "submit"),
		},
	}
	exec := NewWASMExecutor(nil, store, nil, nil)
	in := reflCreds(srv.URL)
	in["prompt"] = checkOut // the engine envelope: step output under "prompt"
	res, err := exec.Execute(context.Background(), "mod-submit", in, "")
	require.NoError(t, err)
	assert.Equal(t, true, res["success"], "stdout: %v", res["stdout"])

	posts := forge.posted()
	require.Len(t, posts, 1)
	assert.Equal(t, "reflection", posts[0]["actor"])
	beliefs := posts[0]["beliefs"].([]json.RawMessage)
	require.Len(t, beliefs, 2, "safe + needs-review forget; dup skipped: %v", beliefs)
	var got []map[string]interface{}
	for _, b := range beliefs {
		var m map[string]interface{}
		require.NoError(t, json.Unmarshal(b, &m))
		got = append(got, m)
	}
	assert.Equal(t, "the user deploys only after a staging pass", got[0]["content"])
	assert.NotContains(t, fmt.Sprint(got[0]["rationale"]), "[needs human review]")
	assert.Equal(t, "the user wants long-form replies", got[1]["content"])
	assert.Contains(t, fmt.Sprint(got[1]["rationale"]), "[needs human review]")
	assert.Equal(t, []interface{}{"00000000-0000-0000-0000-000000000001"}, got[0]["source_episodes"])

	out := res["output"].(map[string]interface{})
	assert.Equal(t, float64(2), out["submitted"])
	assert.Equal(t, float64(2), out["accepted"])
	assert.Equal(t, float64(0), out["rejected"])
	require.Len(t, out["skipped"].([]interface{}), 1)
}

// ------------------------------------------------------------------
// stub: the shipped step-2 stand-in behaves like an agent message
// ------------------------------------------------------------------

func TestReflectionStub_EmitsAgentStyleText(t *testing.T) {
	store := &MockPrimitiveStore{
		WasmModules: []*primitive.WasmModuleListItem{{ID: "mod-stub", Name: "stub"}},
		WasmModuleData: map[string][]byte{
			"mod-stub": buildReflectionWasm(t, "stub"),
		},
	}
	exec := NewWASMExecutor(nil, store, nil, nil)
	res, err := exec.Execute(context.Background(), "mod-stub", map[string]interface{}{}, "")
	require.NoError(t, err)
	assert.Equal(t, true, res["success"])
	text, ok := res["output"].(string)
	require.True(t, ok, "stub output is the raw agent-style text: %v", res["output"])
	assert.Contains(t, text, "Reflection complete")
	// The text must parse into three proposals (fenced-array path).
	proposals, err := reflmem.ParseProposals(text)
	require.NoError(t, err)
	assert.Len(t, proposals, 3)
}
