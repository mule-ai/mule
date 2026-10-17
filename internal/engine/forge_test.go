package engine

// forge_test.go: H3.2 forge_conversation step tests.
//
// A fake forge server (httptest) implements the exact endpoint shapes
// mule consumes (read from forge's axum router + api/events.rs):
//
//	GET  /agents/:id/active         {"busy", "current_conversation"}
//	POST /agents/:id/conversations  201 {"session":{"id":...},"working_dir"}
//	POST /messages                  202 {"message":{...}}
//	GET  /messages?session_id=      {"messages":[{sequence,role,content}]}
//	GET  /sessions/:id/events       SSE message/turn_ended/heartbeat
//
// The 3-step acceptance workflow runs through the real engine
// processJob: wasm echo -> forge_conversation (wait:true) ->
// forge_conversation (fork, wait:false), with the fake forge standing
// in for the forge machine. The wasm module is a hand-assembled
// minimal module whose _start writes a fixed JSON to fd 1 — enough to
// exercise the real WASM executor path without a tinygo toolchain in
// the test suite.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mule-ai/mule/internal/agent"
	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/pkg/job"
)

// ------------------------------------------------------------------
// fake forge
// ------------------------------------------------------------------

// fakeForge is an in-memory forge with the endpoint shapes mule's
// forge_conversation step consumes. Single agent (the one under test).
type fakeForge struct {
	mu             sync.Mutex
	agentID        string
	agentActive    string // conversation id /active reports, "" = none
	sessions       map[string]*fakeSession
	forkSource     map[string]string // created session -> fork_from
	created        []string          // session ids in creation order
	reqLog         []string
	apiKeySeen     string
	whenEnd        time.Duration // SSE turn_ended timing; 0 = never ends
	assistantReply map[string]string
	ended          map[string]bool // sessions that already emitted turn_ended
}

type fakeSession struct {
	rows        []fakeForgeRow
	pendingTurn bool // a turn is in flight (assistant row not yet written)
}

type fakeForgeRow struct {
	Sequence int    `json:"sequence"`
	Role     string `json:"role"`
	Content  string `json:"content"`
}

func newFakeForge(agentID, activeConversation string) *fakeForge {
	f := &fakeForge{
		agentID:        agentID,
		agentActive:    activeConversation,
		sessions:       map[string]*fakeSession{},
		forkSource:     map[string]string{},
		assistantReply: map[string]string{},
		ended:          map[string]bool{},
	}
	if activeConversation != "" {
		f.sessions[activeConversation] = &fakeSession{}
	}
	return f
}

// setReply sets the canned turn output for a session and arms its
// "turn" (assistant row lands whenEnd after each user post).
func (f *fakeForge) setReply(sessionID string, reply string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.assistantReply[sessionID] = reply
}

func (f *fakeForge) record(method, path string) {
	f.reqLog = append(f.reqLog, method+" "+path)
}

// userPosts returns the user-row contents of a session (test-side).
func (f *fakeForge) userPosts(sessionID string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	if sess, ok := f.sessions[sessionID]; ok {
		for _, row := range sess.rows {
			if row.Role == "user" {
				out = append(out, row.Content)
			}
		}
	}
	return out
}

func (f *fakeForge) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reqLog)
}

func writeJSONResponse(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeForge) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /agents/{id}/active", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.record(http.MethodGet, r.URL.String())
		active := f.agentActive
		id := r.PathValue("id")
		f.mu.Unlock()
		if id != f.agentID {
			http.NotFound(w, r)
			return
		}
		if active == "" {
			writeJSONResponse(w, map[string]interface{}{"busy": false, "current_conversation": nil})
			return
		}
		writeJSONResponse(w, map[string]interface{}{"busy": false, "current_conversation": active})
	})

	mux.HandleFunc("POST /agents/{id}/conversations", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.record(http.MethodPost, r.URL.String())
		id := fmt.Sprintf("conv-new-%d", len(f.created)+1)
		f.created = append(f.created, id)
		f.sessions[id] = &fakeSession{}
		if ff, _ := body["fork_from"].(string); ff != "" {
			f.forkSource[id] = ff
		}
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"session":     map[string]interface{}{"id": id},
			"working_dir": "/forge/sessions/" + id,
		})
	})

	mux.HandleFunc("POST /messages", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SessionID string `json:"session_id"`
			Content   string `json:"content"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.record(http.MethodPost, "/messages")
		f.apiKeySeen = r.Header.Get("Authorization")
		sess, ok := f.sessions[body.SessionID]
		whenEnd := f.whenEnd
		if ok {
			seq := len(sess.rows) + 1
			sess.rows = append(sess.rows, fakeForgeRow{Sequence: seq, Role: "user", Content: body.Content})
			if whenEnd > 0 {
				sess.pendingTurn = true
			}
		}
		f.mu.Unlock()
		if !ok {
			http.Error(w, `{"error":"Session not found"}`, http.StatusNotFound)
			return
		}
		// Run the "turn": after whenEnd append the assistant row so
		// SSE watchers + GET /messages both see it.
		if whenEnd > 0 {
			go func() {
				time.Sleep(whenEnd)
				f.mu.Lock()
				reply := f.assistantReply[body.SessionID]
				if reply == "" {
					reply = "the-answer"
				}
				sess.rows = append(sess.rows, fakeForgeRow{Sequence: len(sess.rows) + 1, Role: "assistant", Content: reply})
				sess.pendingTurn = false
				f.mu.Unlock()
			}()
		}
		w.WriteHeader(http.StatusAccepted)
		writeJSONResponse(w, map[string]interface{}{"message": map[string]interface{}{"role": "user"}})
	})

	mux.HandleFunc("GET /messages", func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.URL.Query().Get("session_id")
		f.mu.Lock()
		f.record(http.MethodGet, r.URL.String())
		sess, ok := f.sessions[sessionID]
		rows := []fakeForgeRow{}
		if ok {
			rows = append(rows, sess.rows...)
		}
		f.mu.Unlock()
		if !ok {
			http.Error(w, `{"error":"Session not found"}`, http.StatusNotFound)
			return
		}
		writeJSONResponse(w, map[string]interface{}{"messages": rows})
	})

	// SSE: replay rows with sequence > since, then go live. Closes
	// after emitting turn_ended when an assistant row beyond `since`
	// has landed and no turn is pending (oneshot semantics).
	mux.HandleFunc("GET /sessions/{id}/events", func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("id")
		since := 0
		if q := r.URL.Query().Get("since"); q != "" {
			fmt.Sscanf(q, "%d", &since)
		}
		f.mu.Lock()
		f.record(http.MethodGet, r.URL.String())
		sess, ok := f.sessions[sessionID]
		f.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "no flusher", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		ctx := r.Context()
		sent := map[int]bool{}
		for {
			f.mu.Lock()
			newAssistant := false
			for _, row := range sess.rows {
				if row.Sequence > since && !sent[row.Sequence] {
					_, _ = fmt.Fprintf(w, "event: message\ndata: {\"sequence\":%d,\"role\":%q,\"content\":%q}\n\n", row.Sequence, row.Role, row.Content)
					sent[row.Sequence] = true
					if row.Role == "assistant" {
						newAssistant = true
					}
				}
			}
			settle := !sess.pendingTurn && newAssistant && !f.ended[sessionID]
			f.mu.Unlock()
			flusher.Flush()

			if settle {
				f.mu.Lock()
				f.ended[sessionID] = true
				f.mu.Unlock()
				_, _ = fmt.Fprintf(w, "event: turn_ended\ndata: {\"session_id\":%q}\n\n", sessionID)
				flusher.Flush()
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
	})

	return mux
}

// ------------------------------------------------------------------
// hand-assembled minimal wasm module (echo of a fixed JSON string)
// ------------------------------------------------------------------

// wasmEchoModule builds the smallest valid wasm module that exports
// _start, which writes the fixed string `out` to fd 1 via the WASI
// fd_write import. Layout: iov struct at offset 8, string at offset 40.
func wasmEchoModule(out string) []byte {
	str := []byte(out)
	iov := make([]byte, 8)
	// base = 40 (little-endian)
	iov[0], iov[1], iov[2], iov[3] = 40, 0, 0, 0
	// len = len(str)
	ll := uint32(len(str))
	iov[4] = byte(ll)
	iov[5] = byte(ll >> 8)
	iov[6] = byte(ll >> 16)
	iov[7] = byte(ll >> 24)

	var b []byte
	b = append(b, 0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00) // magic+version
	// type section: type 0 = (i32,i32,i32,i32)->i32 (fd_write),
	// type 1 = ()->() (_start)
	b = append(b, 0x01, 0x0C, 0x02, 0x60, 0x04, 0x7F, 0x7F, 0x7F, 0x7F, 0x01, 0x7F, 0x60, 0x00, 0x00)
	// import section: wasi_snapshot_preview1.fd_write (type 0)
	modName := []byte("wasi_snapshot_preview1")
	fnName := []byte("fd_write")
	b = append(b, 0x02, 0x23, 0x01, byte(len(modName)))
	b = append(b, modName...)
	b = append(b, byte(len(fnName)))
	b = append(b, fnName...)
	b = append(b, 0x00, 0x00)
	// function section: one local function, type 1
	b = append(b, 0x03, 0x02, 0x01, 0x01)
	// memory section: one memory, min 1
	b = append(b, 0x05, 0x03, 0x01, 0x00, 0x01)
	// export section: _start (func 1), memory (mem 0)
	b = append(b, 0x07, 0x13, 0x02, 0x06)
	b = append(b, []byte("_start")...)
	b = append(b, 0x00, 0x01, 0x06)
	b = append(b, []byte("memory")...)
	b = append(b, 0x02, 0x00)
	// code section: _start = fd_write(1, 8, 1, 0); drop; end
	b = append(b, 0x0A, 0x0F, 0x01, 0x0D, 0x00,
		0x41, 0x01, // i32.const 1 (fd)
		0x41, 0x08, // i32.const 8 (iovs ptr)
		0x41, 0x01, // i32.const 1 (iovs len)
		0x41, 0x00, // i32.const 0 (nwritten ptr)
		0x10, 0x00, // call fd_write
		0x1A, // drop
		0x0B) // end
	// data section: two active segments (kind 0 = active, memory 0)
	// -- iov struct at 8, string at 40. Segment form:
	// kind, offset expr, size LEB, bytes.
	data := []byte{0x02} // segment count
	data = append(data, 0x00, 0x41, 0x08, 0x0B)
	data = append(data, lebu(len(iov))...)
	data = append(data, iov...)
	data = append(data, 0x00, 0x41, 0x28, 0x0B)
	data = append(data, lebu(len(str))...)
	data = append(data, str...)
	b = append(b, 0x0B)
	b = append(b, lebu(len(data))...)
	b = append(b, data...)
	return b
}

// lebu encodes n as unsigned LEB128 (wasm section/segment sizes are
// LEB128, not single bytes — the single-byte shortcut only worked for
// payloads under 128 bytes).
func lebu(n int) []byte {
	var out []byte
	for {
		b := byte(n & 0x7f)
		n >>= 7
		if n > 0 {
			b |= 0x80
		}
		out = append(out, b)
		if n == 0 {
			break
		}
	}
	return out
}

// ------------------------------------------------------------------
// engine plumbing
// ------------------------------------------------------------------

// forgeEngine builds a real engine over in-memory mock stores.
func forgeEngine(t *testing.T, store *MockPrimitiveStore) (*Engine, *MockJobStore) {
	t.Helper()
	mockJobStore := &MockJobStore{Jobs: make(map[string]*job.Job)}
	agentRuntime := agent.NewRuntime(store, mockJobStore)
	wasmExecutor := NewWASMExecutor(nil, store, agentRuntime, nil)
	eng := NewEngine(store, mockJobStore, agentRuntime, wasmExecutor, Config{Workers: 1})
	return eng, mockJobStore
}

func TestForgeConversationStepConfig(t *testing.T) {
	// Defaults: mode message, wait false, timeout 300.
	cfg, err := parseForgeStepConfig(map[string]interface{}{
		"forge_url": "http://forge",
		"api_key":   "k",
		"agent_id":  "a",
		"content":   "hi",
	})
	require.NoError(t, err)
	assert.Equal(t, "message", cfg.Mode)
	assert.False(t, cfg.Wait)
	assert.Equal(t, 300, cfg.TimeoutSecs)
	assert.Equal(t, "http://forge", cfg.ForgeURL)

	// Explicit values.
	cfg, err = parseForgeStepConfig(map[string]interface{}{
		"forge_url":    "http://forge/",
		"api_key":      "k",
		"agent_id":     "a",
		"content":      "hi",
		"mode":         "fork",
		"wait":         true,
		"timeout_secs": 45,
	})
	require.NoError(t, err)
	assert.Equal(t, "fork", cfg.Mode)
	assert.True(t, cfg.Wait)
	assert.Equal(t, 45, cfg.TimeoutSecs)
	assert.Equal(t, "http://forge", cfg.ForgeURL, "trailing slash must be trimmed")

	// Invalid config fails with the validation errors.
	_, err = parseForgeStepConfig(map[string]interface{}{"content": "x"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "forge_url")

	// Template rendering over the job input map.
	got, err := renderTemplate("about {{.prompt}}", map[string]interface{}{"prompt": "wasm-says-hi"})
	require.NoError(t, err)
	assert.Equal(t, "about wasm-says-hi", got)

	// Missing key is an error, not silent empty.
	_, err = renderTemplate("{{.nope}}", map[string]interface{}{})
	assert.Error(t, err)

	// Malformed template is an error.
	_, err = renderTemplate("{{.oops", map[string]interface{}{})
	assert.Error(t, err)
}

// TestForgeConversationWorkflow is the H3.2 acceptance workflow:
// wasm echo -> forge_conversation (wait:true, templated content) ->
// forge_conversation (fork, wait:false, content derived from the
// previous step's output), run through the real engine processJob
// against a fake forge.
func TestForgeConversationWorkflow(t *testing.T) {
	const jobID = "job-1"
	const workflowID = "workflow-forge"

	forg := newFakeForge("agent-x", "conv-1")
	forg.setReply("conv-1", "the-answer")
	forg.whenEnd = 300 * time.Millisecond
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()

	forgeStep := &primitive.WorkflowStep{
		ID: "step-2", WorkflowID: workflowID, StepOrder: 2,
		StepType: "forge_conversation",
		Config: map[string]interface{}{
			"forge_url":    srv.URL,
			"api_key":      "sk_forge_testkey",
			"agent_id":     "agent-x",
			"mode":         "message",
			"content":      "{{.prompt}}",
			"wait":         true,
			"timeout_secs": 30,
		},
	}
	forkStep := &primitive.WorkflowStep{
		ID: "step-3", WorkflowID: workflowID, StepOrder: 3,
		StepType: "forge_conversation",
		Config: map[string]interface{}{
			"forge_url":    srv.URL,
			"api_key":      "sk_forge_testkey",
			"agent_id":     "agent-x",
			"mode":         "fork",
			"content":      "follow-up about: {{.output}}",
			"timeout_secs": 30,
		},
	}
	wasmStep := &primitive.WorkflowStep{
		ID: "step-1", WorkflowID: workflowID, StepOrder: 1,
		StepType:     "wasm_module",
		WasmModuleID: &[]string{"wasm-echo"}[0],
	}

	store := &MockPrimitiveStore{
		Workflows: []*primitive.Workflow{{ID: workflowID, Name: "forge workflow"}},
		WorkflowSteps: []*primitive.WorkflowStep{
			wasmStep, forgeStep, forkStep,
		},
		WasmModules: []*primitive.WasmModuleListItem{{ID: "wasm-echo", Name: "echo"}},
		WasmModuleData: map[string][]byte{
			// The executor's stdout protocol: JSON with a "message" field
			// yields that value as the step output.
			"wasm-echo": wasmEchoModule(`{"message":"hello-from-wasm","success":true}`),
		},
	}
	eng, jobStore := forgeEngine(t, store)

	created := &job.Job{
		ID:         jobID,
		WorkflowID: workflowID,
		Status:     job.StatusQueued,
		InputData:  map[string]interface{}{"note": "job input"},
		OutputData: map[string]interface{}{},
		CreatedAt:  time.Now(),
	}
	require.NoError(t, jobStore.CreateJob(created))

	require.NoError(t, eng.processJob(context.Background(), jobID), "3-step workflow should complete")

	// The wasm echo output flowed into the forge step's template and
	// arrived at the fake forge with the mule-job tag.
	posts := forg.userPosts("conv-1")
	require.Len(t, posts, 1)
	assert.Equal(t, "[mule job "+jobID+" "+workflowID+"] hello-from-wasm", posts[0],
		"forge must see the mule-job-tagged, templated content")

	// wait:true returned the turn's assistant text, which flowed into
	// step 3's template (asserted via step 3's posted content).
	forkPosts := forg.userPosts("conv-new-1")
	require.Len(t, forkPosts, 1, "fork must have created a new conversation and been messaged")
	assert.Equal(t, "[mule job "+jobID+" "+workflowID+"] follow-up about: the-answer", forkPosts[0],
		"the waited-for assistant output must flow into the next step")

	// The fork was cut from the agent's active conversation.
	forg.mu.Lock()
	forkFrom := forg.forkSource["conv-new-1"]
	forg.mu.Unlock()
	assert.Equal(t, "conv-1", forkFrom, "fork_from must be the agent's active conversation")

	// Forge saw the mule key as a Bearer token.
	forg.mu.Lock()
	apiKey := forg.apiKeySeen
	forg.mu.Unlock()
	assert.Equal(t, "Bearer sk_forge_testkey", apiKey)

	// The job completed with the last step's output.
	done, err := jobStore.GetJob(jobID)
	require.NoError(t, err)
	assert.Equal(t, job.StatusCompleted, done.Status)
	assert.Equal(t, "conv-new-1", done.OutputData["forge_conversation"])
	assert.Equal(t, "[mule job "+jobID+" "+workflowID+"] follow-up about: the-answer", done.OutputData["output"])
}

// TestForgeConversationStepNewConversation covers addressing when the
// agent has no active conversation yet: the step must create one.
func TestForgeConversationStepNewConversation(t *testing.T) {
	forg := newFakeForge("agent-y", "") // agent with no conversations
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()

	eng, _ := forgeEngine(t, &MockPrimitiveStore{})
	step := &primitive.WorkflowStep{
		ID: "step-new", WorkflowID: "workflow-new", StepOrder: 1,
		StepType: "forge_conversation",
		Config: map[string]interface{}{
			"forge_url":    srv.URL,
			"api_key":      "sk_forge_testkey",
			"agent_id":     "agent-y",
			"content":      "ping",
			"timeout_secs": 10,
		},
	}

	out, err := eng.processForgeConversationStep(context.Background(), step, "job-new", map[string]interface{}{"prompt": "x"}, "")
	require.NoError(t, err)
	assert.Equal(t, "conv-new-1", out["forge_conversation"], "step must create the agent's conversation")
	assert.Equal(t, "[mule job job-new workflow-new] ping", out["output"], "wait:false returns the posted content")
}

// TestForgeConversationWaitTimeout verifies wait:true fails with the
// documented error when the forge turn never ends.
func TestForgeConversationWaitTimeout(t *testing.T) {
	forg := newFakeForge("agent-t", "conv-t")
	forg.whenEnd = 0 // the turn never ends
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()

	eng, _ := forgeEngine(t, &MockPrimitiveStore{})
	step := &primitive.WorkflowStep{
		ID: "step-timeout", WorkflowID: "workflow-to", StepOrder: 1,
		StepType: "forge_conversation",
		Config: map[string]interface{}{
			"forge_url":    srv.URL,
			"api_key":      "sk_forge_testkey",
			"agent_id":     "agent-t",
			"content":      "will you ever finish",
			"wait":         true,
			"timeout_secs": 2,
		},
	}

	start := time.Now()
	_, err := eng.processForgeConversationStep(context.Background(), step, "job-to", map[string]interface{}{}, "")
	elapsed := time.Since(start)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out waiting for forge turn")
	assert.Less(t, elapsed, 20*time.Second, "must give up at ~timeout_secs, not hang: %v", elapsed)
}

// TestForgeConversationStepCancellation verifies that cancelling the
// job context (DELETE /api/v1/jobs/:id -> engine ctx cancel) stops the
// wait prompt promptly and leaves no poll/SSE goroutine behind: the
// fake forge SSE handler returns and no further requests arrive.
func TestForgeConversationStepCancellation(t *testing.T) {
	forg := newFakeForge("agent-c", "conv-c")
	forg.whenEnd = 0 // the turn never ends; the SSE stream stays open
	inner := forg.handler()

	// Record when each handler returns; the SSE handler only returns
	// when the client closes the stream (ctx cancel) or the test
	// server tears down.
	handlerReturn := make(chan time.Time, 16)
	observed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inner.ServeHTTP(w, r)
		select {
		case handlerReturn <- time.Now():
		default:
		}
	})
	srv := httptest.NewServer(observed)
	defer srv.Close()

	eng, _ := forgeEngine(t, &MockPrimitiveStore{})
	step := &primitive.WorkflowStep{
		ID: "step-cancel", WorkflowID: "workflow-cx", StepOrder: 1,
		StepType: "forge_conversation",
		Config: map[string]interface{}{
			"forge_url":    srv.URL,
			"api_key":      "sk_forge_testkey",
			"agent_id":     "agent-c",
			"content":      "cancel me",
			"wait":         true,
			"timeout_secs": 600, // big: only our cancel should end it
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := eng.processForgeConversationStep(ctx, step, "job-cx", map[string]interface{}{}, "")
		done <- err
	}()

	// Let the step reach the SSE wait, then cancel (as
	// DELETE /api/v1/jobs/:id would do via the engine context).
	time.Sleep(500 * time.Millisecond)
	cancelTime := time.Now()
	cancel()

	select {
	case err := <-done:
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cancelled")
	case <-time.After(2 * time.Second):
		t.Fatal("step did not return within ~2s of context cancel")
	}
	assert.Less(t, time.Since(cancelTime), 2*time.Second)

	// The SSE handler must unwind promptly: if the client-side read
	// were leaked, the handler would block on the socket indefinitely
	// and only return when the httptest server tears down at test end.
	select {
	case <-handlerReturn:
	case <-time.After(3 * time.Second):
		t.Fatal("forge SSE handler did not return after cancel: the poll loop leaked")
	}

	// No further requests may arrive after the cancel: the poll loop
	// is gone and the deadline path is not running.
	afterCancel := forg.requestCount()
	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, afterCancel, forg.requestCount(),
		"no requests may follow a cancelled wait (poll loop must be gone)")
}

// TestStripThinking: reasoning models (Qwen3.8 et al.) emit
// chain-of-thought INLINE in the content text — as <think>…</think>,
// as a template-opened bare segment with only the closing tag, or as
// a dangling <think> (truncated turn). A voice caller must never
// hear any of it; plain content must pass through untouched.
func TestStripThinking(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"paired", "<think>plan it</think>Here is the answer.", "Here is the answer."},
		{"multiline paired", "<think>line one\nline two\n</think>\n\nShort answer.", "Short answer."},
		// The live Bob leak: Qwen3.8's template auto-opens the block —
		// reasoning text with NO opening tag, then </think>.
		{
			"no opening tag",
			"The user sent \"Last cookie.\" over the voice call. This is a short message to acknowledge. I should respond briefly.\n\nI'll give a short, friendly response acknowledging their message.</think>I hear you — last cookie noted. What for?",
			"I hear you — last cookie noted. What for?",
		},
		{"dangling opener", "<think>The user asked something and I", ""},
		{"clean", "Just the answer, nothing else.", "Just the answer, nothing else."},
		{"clean with think word", "I think the answer is 4.", "I think the answer is 4."},
		{"whitespace after strip", "  <think>x</think>  Answer.  ", "Answer."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, StripThinking(tc.in))
		})
	}
}

// TestLatestAssistantStripsThinking: the messages-table fetch the
// voice loop's PostAndWait relies on strips thinking before it
// returns (the fake forge serves an inline-<think> assistant row).
func TestLatestAssistantStripsThinking(t *testing.T) {
	forg := newFakeForge("agent-x", "conv-1")
	forg.setReply("conv-1", "<think>ponder</think>Visible answer.")
	srv := httptest.NewServer(forg.handler())
	defer srv.Close()

	forg.mu.Lock()
	forg.sessions["conv-1"].rows = append(forg.sessions["conv-1"].rows,
		fakeForgeRow{Sequence: 1, Role: "user", Content: "hi"},
		fakeForgeRow{Sequence: 2, Role: "assistant", Content: "<think>ponder</think>Visible answer."})
	forg.mu.Unlock()

	c := &forgeStepClient{baseURL: srv.URL, apiKey: "sk_forge_testkey", http: &http.Client{Timeout: 10 * time.Second}}
	got, err := c.latestAssistantAfter(context.Background(), "conv-1", 0)
	assert.NoError(t, err)
	assert.Equal(t, "Visible answer.", got)
}
