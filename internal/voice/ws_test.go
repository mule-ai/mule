package voice

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"

	"github.com/mule-ai/mule/internal/engine"
)

// newTestCall builds a live call registered in a hub under jobID
// "j-voice" (no forge traffic is expected to succeed; the tests only
// exercise the transport).
func newTestCall(t *testing.T, hub *Hub, jobID string) *Call {
	t.Helper()
	call, err := hub.StartCall(context.Background(), jobID, map[string]interface{}{
		"forge_url": "http://127.0.0.1:1", "api_key": "k", "agent_id": "a",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := call.(*Call)
	if !ok {
		t.Fatal("StartCall did not return a *Call")
	}
	return c
}

func serveVoiceRoutes(t *testing.T, hub *Hub) *httptest.Server {
	t.Helper()
	r := mux.NewRouter()
	r.Handle("/voice/{job_id}/in", InHandler(hub))
	r.Handle("/voice/{job_id}/out", OutHandler(hub))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func wsURL(base, path string) string {
	return "ws" + base[len("http"):] + path
}

// TestInSocketDeliversFramesAndHangUp runs the real in-socket loop:
// binary frames land in the call, the hang-up control frame ends the
// call, and Run returns nil.
func TestInSocketDeliversFramesAndHangUp(t *testing.T) {
	hub := NewHub()
	srv := serveVoiceRoutes(t, hub)
	call := newTestCall(t, hub, "j-in")
	// No Run() in this test: frame consumption is covered by the
	// session tests; here the in-socket reader loop must deliver raw
	// frames into the call and translate control frames.

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(srv.URL, "/voice/j-in/in"), nil)
	if err != nil {
		t.Fatalf("in-socket dial: %v", err)
	}
	defer conn.Close()

	frame := makeFrame(480, 8000)
	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-call.inCh:
		if len(got) != len(frame) {
			t.Errorf("frame size = %d, want %d", len(got), len(frame))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("in-frame never reached the call")
	}

	// Unknown text frames are ignored (no hang-up).
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"hello":true}`)); err != nil {
		t.Fatal(err)
	}
	if call.ctx.Err() != nil {
		t.Fatal("unknown text frame hung up the call")
	}

	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"hangup":true}`)); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for call.ctx.Err() == nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if call.ctx.Err() == nil {
		t.Fatal("call did not hang up on the control frame")
	}
}

// TestInSocketCloseHangsUp: dropping the in connection (without a
// control frame) must also end the call — the spec's "connection close
// → job done" path.
func TestInSocketCloseHangsUp(t *testing.T) {
	hub := NewHub()
	srv := serveVoiceRoutes(t, hub)
	call := newTestCall(t, hub, "j-close")

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(srv.URL, "/voice/j-close/in"), nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.WriteMessage(websocket.BinaryMessage, makeFrame(480, 1))
	_ = conn.Close() // no hang-up frame: the close IS the hang-up

	deadline := time.Now().Add(5 * time.Second)
	for call.ctx.Err() == nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if call.ctx.Err() == nil {
		t.Fatal("call did not hang up on in-socket close")
	}
}

// TestOutSocketStreamsTTSAndDoneMarker: out chunks arrive as binary
// frames, and the call ending yields the {"done":true} marker.
func TestOutSocketStreamsTTSAndDoneMarker(t *testing.T) {
	hub := NewHub()
	srv := serveVoiceRoutes(t, hub)
	call := newTestCall(t, hub, "j-out")
	done := make(chan error, 1)
	go func() { done <- call.Run() }()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(srv.URL, "/voice/j-out/out"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	call.outCh <- engine.VoiceOutChunk{Audio: []byte{0x01, 0x02}}
	_, chunk, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("first out read: %v", err)
	}
	if len(chunk) != 2 || chunk[0] != 0x01 {
		t.Errorf("out chunk = %x, want 0102", chunk)
	}

	call.HangUp()
	if err := <-done; err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	_, marker, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("done marker read: %v", err)
	}
	if string(marker) != `{"done":true}` {
		t.Errorf("marker = %s, want {\"done\":true}", marker)
	}
}

// TestOutSocketClientGoneKeepsCallAlive: the out socket is a fire-
// forget playback path — when its client disappears the call must
// keep listening and still hang up cleanly. (TTS stopping on out-
// socket loss is covered white-box by TestTTSStopsWhenOutMarkedClosed;
// a write to a just-closed localhost socket may be TCP-buffered, so
// the flag flip itself is not asserted here.)
func TestOutSocketClientGoneKeepsCallAlive(t *testing.T) {
	hub := NewHub()
	srv := serveVoiceRoutes(t, hub)
	call := newTestCall(t, hub, "j-outgone")
	done := make(chan error, 1)
	go func() { done <- call.Run() }()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(srv.URL, "/voice/j-outgone/out"), nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()                                        // the out client goes away immediately
	call.outCh <- engine.VoiceOutChunk{Audio: []byte{0xAB}} // must not wedge the call

	call.HangUp()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("call did not end after hang-up with a dead out socket")
	}
}

// TestTTSStopsWhenOutMarkedClosed: once the out handler has observed
// the client leaving (MarkOutClosed), the session must skip TTS
// synthesis while still running STT + the agent turn, and must end
// cleanly on hang-up.
func TestTTSStopsWhenOutMarkedClosed(t *testing.T) {
	fg, srv := newFakeForge(t, fakeConv, []string{"Sure thing. Here you go."})
	cfg := testCfg(srv.URL)
	call, err := NewCall(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- call.Run() }()

	call.MarkOutClosed() // simulate the out handler noticing the client
	feedUtterance(call)
	// The agent turn must have happened (the fake agent answered) but
	// no TTS synthesis may follow.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if fg.TTSCount() == 0 && len(fg.postedContents()) >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := fg.TTSCount(); got != 0 {
		t.Errorf("ttsCalls = %d, want 0 with the out socket closed", got)
	}
	if got := len(fg.postedContents()); got < 1 {
		t.Errorf("agent turns posted = %d, want >= 1", got)
	}

	call.HangUp()
	if err := <-done; err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
}

// TestInSocketTextFrameTypedUtterance: a {"text": …} frame on the in
// socket feeds the call's typed-utterance channel (the CLI fallback).
func TestInSocketTextFrameTypedUtterance(t *testing.T) {
	hub := NewHub()
	srv := serveVoiceRoutes(t, hub)
	call := newTestCall(t, hub, "j-text")

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(srv.URL, "/voice/j-text/in"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"text":"check the logs"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-call.textCh:
		if got != "check the logs" {
			t.Errorf("text frame delivered as %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("text frame never reached the call")
	}
}

// TestOutSocketAssistantTextFrame: an assistant-text out event arrives
// as a {"assistant": …} text frame on the wire.
func TestOutSocketAssistantTextFrame(t *testing.T) {
	hub := NewHub()
	srv := serveVoiceRoutes(t, hub)
	call := newTestCall(t, hub, "j-asst")
	done := make(chan error, 1)
	go func() { done <- call.Run() }()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(srv.URL, "/voice/j-asst/out"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	call.outCh <- engine.VoiceOutChunk{Text: "all good"}
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	// WriteJSON appends a newline.
	if strings.TrimRight(string(raw), "\n") != `{"assistant":"all good"}` {
		t.Errorf("assistant frame = %s", raw)
	}

	call.HangUp()
	if err := <-done; err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
}

// TestVoiceSocketUnknownJob: a lookup for a job with no call reports
// not-found (WaitCall's timeout behavior is covered by
// TestHubWaitCall; the handler applies it with a 5-minute bound).
func TestVoiceSocketUnknownJob(t *testing.T) {
	hub := NewHub()
	if _, ok := hub.GetCall("ghost"); ok {
		t.Fatal("hub has a call for an unknown job")
	}
	if _, ok := hub.WaitCall(context.Background(), "ghost", 500*time.Millisecond); ok {
		t.Fatal("WaitCall found a call for a ghost job")
	}
}
