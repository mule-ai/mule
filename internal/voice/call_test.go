package voice

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// testCfg builds a CallConfig pointed at a fake forge URL.
func testCfg(url string) CallConfig {
	return CallConfig{
		ForgeURL:          url,
		APIKey:            "sk_forge_test",
		AgentID:           "agent-1",
		Mode:              "steer",
		TTSVoice:          "af_heart",
		TTSResponseFormat: "pcm",
		TTSRate:           24000,
		SilenceDur:        600 * time.Millisecond,
		FrameDur:          30 * time.Millisecond,
		VADThreshold:      300,
		BargeInThreshold:  600,
		TurnTimeout:       30 * time.Second,
	}
}

// feedUtterance pushes enough 30 ms frames to constitute a full
// utterance: ~1 s of speech then 700 ms of silence (crosses the
// 600 ms boundary).
func feedUtterance(c *Call) {
	speech := makeFrame(480, 8000)
	silence := makeFrame(480, 0)
	for i := 0; i < 33; i++ {
		c.FeedIn(speech)
	}
	for i := 0; i < 24; i++ {
		c.FeedIn(silence)
	}
}

// drainOut reads out events until n AUDIO chunks or a timeout (text
// metadata events are skipped).
func drainOut(t *testing.T, c *Call, n int, timeout time.Duration) [][]byte {
	t.Helper()
	var chunks [][]byte
	deadline := time.After(timeout)
	for len(chunks) < n {
		select {
		case chunk, ok := <-c.Out():
			if !ok {
				return chunks
			}
			if len(chunk.Audio) > 0 {
				chunks = append(chunks, chunk.Audio)
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %d out chunks, got %d", n, len(chunks))
		}
	}
	return chunks
}

func TestCallFullLoopHangUp(t *testing.T) {
	_, srv := newFakeForge(t, fakeConv, []string{"On it. Checking now.", "All done. Let me verify."})
	cfg := testCfg(srv.URL)
	call, err := NewCall(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- call.Run() }()

	// Utterance 1.
	feedUtterance(call)
	chunks := drainOut(t, call, 2, 5*time.Second)
	// "On it." + "Checking now." → two TTS calls (200 B each, the 2nd
	// tagged with byte 2 in position 0).
	if len(chunks) != 2 {
		t.Fatalf("out chunks = %d, want 2", len(chunks))
	}
	if chunks[1][0] != 2 {
		t.Errorf("second chunk is not from TTS call #2: first byte 0x%02x", chunks[1][0])
	}

	// Utterance 2: the agent's second reply.
	feedUtterance(call)
	drainOut(t, call, 1, 5*time.Second)

	// Hang up: Run returns nil (clean end), out channel closes.
	call.HangUp()
	if err := <-done; err != nil {
		t.Fatalf("Run() = %v, want nil on clean hang-up", err)
	}
	_, open := <-call.Out()
	if open {
		t.Error("out channel should be closed after Run")
	}

	out := call.Output()
	if out["status"] != "hung_up" {
		t.Errorf("status = %v, want hung_up", out["status"])
	}
	if out["turns"] != 2 {
		t.Errorf("turns = %v, want 2", out["turns"])
	}
	if out["forge_conversation"] != fakeConv {
		t.Errorf("forge_conversation = %v, want %s", out["forge_conversation"], fakeConv)
	}
	if out["in_sample_rate"] != 16000 || out["out_sample_rate"] != 24000 {
		t.Errorf("sample rates = %v/%v, want 16000/24000", out["in_sample_rate"], out["out_sample_rate"])
	}
}

// waitForTagged reads out events until one AUDIO chunk whose first
// byte is the given TTS-call tag arrives (text events and stale
// chunks from an interrupted TTS run are skipped). Times out after
// `timeout`.
func waitForTagged(t *testing.T, c *Call, tag byte, timeout time.Duration) []byte {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case chunk, ok := <-c.Out():
			if !ok {
				t.Fatalf("out channel closed before TTS call #%d chunk", tag)
			}
			if len(chunk.Audio) > 0 && chunk.Audio[0] == tag {
				return chunk.Audio
			}
		case <-deadline:
			t.Fatalf("timed out waiting for TTS call #%d chunk", tag)
		}
	}
}

func TestCallBargeInStopsTTSAndCapturesSpeech(t *testing.T) {
	// Reply 1 is two sentences → two TTS chunks; the barge-in must land
	// while the agent is speaking and the barged-in utterance must be
	// answered by reply 2 (TTS call #3).
	_, srv := newFakeForge(t, fakeConv, []string{"First. Second sentence.", "Okay, adjusting."})
	cfg := testCfg(srv.URL)
	call, err := NewCall(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- call.Run() }()

	feedUtterance(call)
	first := drainOut(t, call, 1, 5*time.Second)
	if first[0][0] != 1 {
		t.Fatalf("first chunk not from TTS call #1")
	}
	// Barge-in: three hot in-frames while TTS is (still) speaking.
	// They queue in inCh; the loop's post-chunk drain fires the trigger,
	// the second TTS chunk is never synthesized.
	hot := makeFrame(480, 9000)
	for i := 0; i < 3; i++ {
		call.FeedIn(hot)
	}
	// Complete the barged-in utterance: 2 s more of speech, then
	// silence past the boundary.
	silence := makeFrame(480, 0)
	for i := 0; i < 60; i++ {
		call.FeedIn(hot)
	}
	for i := 0; i < 24; i++ {
		call.FeedIn(silence)
	}
	// The agent's reply to the barged-in utterance is TTS call #3.
	// (A stale "Second sentence." chunk from the cut TTS run may still
	// be in the out buffer; waitForTagged skips it.)
	waitForTagged(t, call, 3, 5*time.Second)

	call.HangUp()
	if err := <-done; err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if got := call.Output()["barge_ins"]; got != 1 {
		t.Errorf("barge_ins = %v, want 1", got)
	}
	if got := call.Output()["turns"]; got != 2 {
		t.Errorf("turns = %v, want 2", got)
	}
}

func TestCallKillMidCallFails(t *testing.T) {
	_, srv := newFakeForge(t, fakeConv, []string{"Working on it."})
	cfg := testCfg(srv.URL)
	call, err := NewCall(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- call.Run() }()

	feedUtterance(call)
	drainOut(t, call, 1, 5*time.Second) // mid-call: TTS done, listening

	killErr := errors.New("orchestrator crashed")
	call.Kill(killErr)
	select {
	case err := <-done:
		if !errors.Is(err, killErr) {
			t.Fatalf("Run() = %v, want the kill error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Kill")
	}
	// Second Kill is a no-op (first error wins).
	call.Kill(errors.New("second"))
	_, open := <-call.Out()
	if open {
		t.Error("out channel should be closed after killed Run")
	}
}

func TestCallJobCtxEndMeansDropped(t *testing.T) {
	_, srv := newFakeForge(t, fakeConv, []string{"Hello."})
	cfg := testCfg(srv.URL)
	jobCtx, cancel := context.WithCancel(context.Background())
	call, err := NewCall(jobCtx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- call.Run() }()

	cancel() // the job's context dies (job timeout/cancel) — no hang-up
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run() = nil, want a dropped-call error when the job ctx ends")
		}
		if !strings.Contains(err.Error(), "dropped") {
			t.Errorf("Run() error = %v, want it to mention dropped", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after job ctx cancel")
	}
}

func TestCallSTTFailureKeepsCallAlive(t *testing.T) {
	// No fake forge at all: STT/agent/TTS all fail; the call must stay
	// listening (degraded, not dead) and record the dropped hops.
	call, err := NewCall(context.Background(), testCfg("http://127.0.0.1:1")) // closed port
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- call.Run() }()

	feedUtterance(call)
	// Give the degraded pipeline a moment, then a second utterance
	// must still be accepted (the call is alive).
	time.Sleep(300 * time.Millisecond)
	feedUtterance(call)

	call.HangUp()
	if err := <-done; err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	dropped, _ := call.Output()["dropped"].([]string)
	if len(dropped) == 0 {
		t.Fatal("expected at least one recorded dropped hop")
	}
	for _, d := range dropped {
		if !strings.Contains(d, "stt") && !strings.Contains(d, "agent") && !strings.Contains(d, "tts") {
			t.Errorf("unexpected dropped entry %q", d)
		}
	}
}

func TestParseCallConfig(t *testing.T) {
	good := map[string]interface{}{
		"forge_url": "http://forge:8080/",
		"api_key":   "sk_forge_x",
		"agent_id":  "a1",
	}
	cfg, err := ParseCallConfig(good)
	if err != nil {
		t.Fatalf("ParseCallConfig(good) = %v", err)
	}
	if cfg.Mode != "steer" || cfg.TTSVoice != "af_heart" || cfg.TTSRate != 24000 {
		t.Errorf("defaults not applied: %+v", cfg)
	}
	if cfg.SilenceDur != 600*time.Millisecond || cfg.FrameDur != 30*time.Millisecond {
		t.Errorf("timings wrong: %+v", cfg)
	}

	if _, err := ParseCallConfig(map[string]interface{}{}); err == nil {
		t.Error("empty config should fail")
	}
	_, err = ParseCallConfig(map[string]interface{}{"forge_url": "not-a-url", "api_key": "k", "agent_id": "a"})
	if err == nil || !strings.Contains(err.Error(), "forge_url") {
		t.Errorf("bad forge_url not reported: %v", err)
	}
	_, err = ParseCallConfig(map[string]interface{}{"forge_url": "http://f", "api_key": "k"})
	if err == nil || !strings.Contains(err.Error(), "agent_id") {
		t.Errorf("missing addressing not reported: %v", err)
	}
	_, err = ParseCallConfig(map[string]interface{}{"forge_url": "http://f", "api_key": "k", "agent_id": "a", "mode": "fork"})
	if err == nil {
		t.Error("mode fork should be rejected for a call")
	}
}

func TestCallTypedTextUtterance(t *testing.T) {
	// The CLI's key-typed-text fallback: no audio in, no STT — the
	// typed line goes straight to the agent turn, and the answer still
	// gets TTS + an assistant-text out event.
	fg, srv := newFakeForge(t, fakeConv, []string{"Sure thing. On my way."})
	cfg := testCfg(srv.URL)
	call, err := NewCall(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- call.Run() }()

	call.FeedText("  check the deploy logs  ")

	// Assistant-text event first, then the two TTS chunks.
	deadline := time.After(5 * time.Second)
	gotText := ""
	var audio int
	for gotText == "" || audio < 2 {
		select {
		case chunk, ok := <-call.Out():
			if !ok {
				t.Fatalf("out closed; gotText=%q audio=%d", gotText, audio)
			}
			if chunk.Text != "" {
				gotText = chunk.Text
			} else {
				audio++
			}
		case <-deadline:
			t.Fatalf("timed out; gotText=%q audio=%d", gotText, audio)
		}
	}
	if gotText != "Sure thing. On my way." {
		t.Errorf("assistant text event = %q", gotText)
	}

	// No STT call may have happened for the typed path.
	call.HangUp()
	if err := <-done; err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if got := call.Output()["turns"]; got != 1 {
		t.Errorf("turns = %v, want 1", got)
	}
	if fg.SttCount() != 0 {
		t.Errorf("sttCalls = %d, want 0 for a typed utterance", fg.SttCount())
	}
	if got := fg.postedContents(); len(got) != 1 || got[0] != "[voice call] check the deploy logs" {
		t.Errorf("posted = %q, want the trimmed typed line", got)
	}
}

func TestHubStartGetForget(t *testing.T) {
	hub := NewHub()
	cfg := map[string]interface{}{
		"forge_url": "http://f", "api_key": "k", "agent_id": "a",
	}
	_, err := hub.StartCall(context.Background(), "j1", cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := hub.GetCall("j1"); !ok {
		t.Fatal("call not registered")
	}
	if _, err := hub.StartCall(context.Background(), "j1", cfg, nil); err == nil {
		t.Error("double StartCall for one job should fail")
	}
	hub.ForgetCall("j1")
	if _, ok := hub.GetCall("j1"); ok {
		t.Error("ForgetCall did not remove the call")
	}
	// Bad config fails at StartCall with a clear message.
	if _, err := hub.StartCall(context.Background(), "j2", map[string]interface{}{}, nil); err == nil {
		t.Error("bad config should fail StartCall")
	}
	if _, ok := hub.GetCall("j2"); ok {
		t.Error("failed StartCall should not register")
	}
}

func TestHubWaitCall(t *testing.T) {
	hub := NewHub()
	cfg := map[string]interface{}{
		"forge_url": "http://f", "api_key": "k", "agent_id": "a",
	}
	// WaitCall on a not-yet-started job times out quickly.
	if _, ok := hub.WaitCall(context.Background(), "nope", 500*time.Millisecond); ok {
		t.Error("WaitCall returned a call that was never started")
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		_, _ = hub.StartCall(context.Background(), "late", cfg, nil)
	}()
	if _, ok := hub.WaitCall(context.Background(), "late", 5*time.Second); !ok {
		t.Error("WaitCall missed a call that started while waiting")
	}
}
