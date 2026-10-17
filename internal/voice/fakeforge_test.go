package voice

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeForge is an in-memory forge for voice-loop tests: it implements
// exactly the surface the voice loop uses (agent addressing, message
// post/list, SSE turn events, and the two audio proxies). The "agent"
// is synchronous: each POST /messages immediately produces its canned
// assistant reply (that is what lets the SSE oneshot stream close with
// turn_ended on every read, like a turn that already ended).
//
// The addressable conversation is always "conv-1" (the agent's active
// conversation).
const fakeConv = "conv-1"

type forgeMsg struct {
	Seq     int
	Role    string
	Content string
}

type fakeForge struct {
	reqLog   []string
	mu       sync.Mutex
	msgs     []forgeMsg
	turn     int // index into replies the next POST /messages consumes
	replies  []string
	ttsCalls int
	sttCalls int
	// H6.4 test delays: the agent hop and the TTS hop hold their
	// responses this long (the barge-in / overlap tests need the
	// turns to be in flight long enough to stream in-frames).
	agentDelay time.Duration
	ttsDelay   time.Duration
	sttDelay   time.Duration
	// voice-model selection (PATCH /sessions/{id}) capture
	lastPatch string
	patches   int
}

func newFakeForge(t *testing.T, activeConv string, replies []string) (*fakeForge, *httptest.Server) {
	fg := &fakeForge{replies: replies}
	mux := http.NewServeMux()
	logReq := func(op string) {
		fg.mu.Lock()
		fg.reqLog = append(fg.reqLog, op)
		fg.mu.Unlock()
	}

	// GET /agents/{id}/active → the pinned active conversation.
	mux.HandleFunc("/agents/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/active") {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"busy":false,"current_conversation":%q}`, activeConv)
	})

	// GET /profiles — the voice-model picker source (voice model
	// selection): two profiles mirroring the lab (Spark/SGLang, Laguna/
	// vram llama.cpp).
	mux.HandleFunc("/profiles", func(w http.ResponseWriter, r *http.Request) {
		logReq("GET /profiles")
		fmt.Fprint(w, `{"profiles":[
			{"id":"p-spark","name":"Spark (Qwen3.8-27B)","provider":"spark","model":"spark/qwen3.8-sglang","base_url":"http://spark:8001/v1"},
			{"id":"p-laguna","name":"Laguna (dev)","provider":"proxy","model":"llamacpp/laguna-xs-2.1","base_url":"http://vram:8080/v1"}
		]}`)
	})

	// PATCH /sessions/{id} — forge's model switcher (records the last
	// override body for assertions); handled inside the /sessions/
	// handler below (mux allows one pattern per path prefix).

	// POST /messages — 202; append the user row + canned assistant
	// reply (the synchronous fake "agent").
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			// GET /messages?session_id=...
			fg.mu.Lock()
			defer fg.mu.Unlock()
			var out strings.Builder
			out.WriteString(`{"messages":[`)
			for i, m := range fg.msgs {
				if i > 0 {
					out.WriteString(",")
				}
				fmt.Fprintf(&out, `{"sequence":%d,"role":%q,"content":%q}`, m.Seq, m.Role, m.Content)
			}
			out.WriteString("]}")
			fmt.Fprint(w, out.String())
			return
		}
		if fg.agentDelay > 0 {
			time.Sleep(fg.agentDelay)
		}
		logReq("POST /messages")
		body, _ := io.ReadAll(r.Body)
		var posted struct {
			Content string `json:"content"`
		}
		_ = json.Unmarshal(body, &posted)
		fg.mu.Lock()
		defer fg.mu.Unlock()
		fg.append("user", posted.Content)
		if fg.turn < len(fg.replies) {
			fg.append("assistant", fg.replies[fg.turn])
			fg.turn++
		}
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, `{"message":{"id":"m"}}`)
	})

	// GET /sessions/{id}/events?since=N&oneshot=true — SSE: message
	// rows after since, then turn_ended, then the connection closes.
	mux.HandleFunc("/sessions/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			// forge's model switcher (voice model selection)
			logReq("PATCH /sessions")
			body, _ := io.ReadAll(r.Body)
			fg.mu.Lock()
			fg.lastPatch = string(body)
			fg.patches++
			fg.mu.Unlock()
			fmt.Fprint(w, `{"session":{"id":"s"},"profile":{}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		since, _ := strconv.Atoi(r.URL.Query().Get("since"))
		fg.mu.Lock()
		for _, m := range fg.msgs {
			if m.Seq > since {
				fmt.Fprintf(w, "event: message\ndata: {\"sequence\":%d,\"role\":%q,\"content\":%q}\n\n", m.Seq, m.Role, m.Content)
			}
		}
		fg.mu.Unlock()
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		fmt.Fprint(w, "event: turn_ended\ndata: {\"session_id\":\"conv-1\"}\n\n")
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
	})

	// POST /v1/audio/transcriptions — STT proxy shape (H6.2 shape).
	mux.HandleFunc("/v1/audio/transcriptions", func(w http.ResponseWriter, r *http.Request) {
		logReq("POST /transcriptions")
		if fg.sttDelay > 0 {
			time.Sleep(fg.sttDelay)
		}
		fg.mu.Lock()
		fg.sttCalls++
		n := fg.sttCalls
		fg.mu.Unlock()
		fmt.Fprintf(w, `{"text":%q}`, fmt.Sprintf("transcript-%d", n))
	})

	// POST /v1/audio/speech — TTS proxy shape: JSON in, audio out.
	// The tag (first audio byte = the Nth TTS call) is assigned at
	// request ARRIVAL, not completion: H6.4 barge-in abandons a call
	// while its response is in flight, so calls overlap and completion
	// order is not request order (the tags used to race under load).
	mux.HandleFunc("/v1/audio/speech", func(w http.ResponseWriter, r *http.Request) {
		logReq("POST /speech START")
		fg.mu.Lock()
		fg.ttsCalls++
		n := fg.ttsCalls
		fg.mu.Unlock()
		if fg.ttsDelay > 0 {
			time.Sleep(fg.ttsDelay)
		}
		w.Header().Set("Content-Type", "audio/pcm")
		buf := make([]byte, 200)
		for i := range buf {
			buf[i] = 0x11
		}
		buf[0] = byte(n) // tag which TTS call produced this chunk
		_, _ = w.Write(buf)
		logReq(fmt.Sprintf("POST /speech DONE call=%d", n))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return fg, srv
}

func (fg *fakeForge) append(role, content string) {
	fg.msgs = append(fg.msgs, forgeMsg{Seq: len(fg.msgs) + 1, Role: role, Content: content})
}

// TTSCount returns how many TTS proxy calls the fake has served.
func (fg *fakeForge) TTSCount() int {
	fg.mu.Lock()
	defer fg.mu.Unlock()
	return fg.ttsCalls
}

// SttCount returns how many STT proxy calls the fake has served.
func (fg *fakeForge) SttCount() int {
	fg.mu.Lock()
	defer fg.mu.Unlock()
	return fg.sttCalls
}

// ReqLog returns the fake's HTTP request log (for failure diagnostics).
func (fg *fakeForge) ReqLog() []string {
	fg.mu.Lock()
	defer fg.mu.Unlock()
	return append([]string(nil), fg.reqLog...)
}

// postedContents returns the user-role message contents in order.
func (fg *fakeForge) postedContents() []string {
	fg.mu.Lock()
	defer fg.mu.Unlock()
	var out []string
	for _, m := range fg.msgs {
		if m.Role == "user" {
			out = append(out, m.Content)
		}
	}
	return out
}
