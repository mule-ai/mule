package voice

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mule-ai/mule/internal/engine"
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
	// Drain to close: the out stream also carries metadata events
	// (state/user/assistant/notice), so a single receive could land on
	// a buffered event rather than the close itself. The range ends
	// only when the channel is closed and drained.
	for range call.Out() {
	}
	if _, open := <-call.Out(); open {
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
	// answered by reply 2 (TTS call #3). The TTS hop holds 100 ms so
	// the "Second sentence." chunk is still in flight when the hot
	// frames land — with zero hop delays the speaking window is a
	// scheduling coin-flip and the frames can arrive after the turn
	// settled (no barge at all).
	fg, srv := newFakeForge(t, fakeConv, []string{"First. Second sentence.", "Okay, adjusting."})
	fg.ttsDelay = 100 * time.Millisecond
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
	// The barged-in utterance's agent reply must produce the next
	// AUDIO chunk after the flush control: the flush drains the out
	// queue first, T1's loop is dead at flush time, so nothing from
	// the cut TTS run can follow it. The chunk's fake tag is
	// deliberately not asserted: how many TTS proxy calls ran is
	// timing-dependent (the abandoned call's POST can land early or
	// late), but the next-audio-after-flush invariant is not.
	seenFlush := false
	nextAudio := false
	deadline := time.After(10 * time.Second)
	for !nextAudio {
		select {
		case chunk, ok := <-call.Out():
			if !ok {
				t.Fatalf("out channel closed before the post-flush audio chunk")
			}
			if chunk.Flush {
				seenFlush = true
			} else if len(chunk.Audio) > 0 && seenFlush {
				nextAudio = true
			}
		case <-deadline:
			// Diagnostics: is the chunk somewhere the reader can't see?
			var rest []string
		drainLoop:
			for {
				select {
				case c, ok := <-call.Out():
					if !ok {
						rest = append(rest, "<closed>")
						break drainLoop
					}
					if c.Flush {
						rest = append(rest, "flush")
					} else if c.Text != "" {
						rest = append(rest, "text")
					} else {
						rest = append(rest, fmt.Sprintf("audio(tag=%d)", c.Audio[0]))
					}
				default:
					break drainLoop
				}
			}
			t.Fatalf("timed out waiting for the post-flush audio chunk; seenFlush=%v phase=%s tts=%d barge=%d inCh=%d outQ=%d dropped=%d leftover-out=%q reqlog=%v",
				seenFlush,
				call.Phase(), fg.TTSCount(), call.bargeIns, len(call.inCh),
				len(call.outCh), atomic.LoadInt64(&call.outDropped), rest, fg.ReqLog())
		}
	}

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
	// Drain to close (metadata events may be buffered ahead of the close).
	for range call.Out() {
	}
	if _, open := <-call.Out(); open {
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
	// Every degraded hop must also surface on the OUT channel as a
	// Notice (the {"notice": …} wire frame) — the caller is never left
	// in unexplained silence (2026-10-16 mic-call bug).
	var notices []string
	for ch := range call.Out() {
		if ch.Notice != "" {
			notices = append(notices, ch.Notice)
		}
	}
	if len(notices) == 0 {
		t.Fatal("expected at least one degraded-hop Notice on Out()")
	}
	hasStt := false
	for _, n := range notices {
		if strings.HasPrefix(n, "stt:") {
			hasStt = true
		}
	}
	if !hasStt {
		t.Errorf("expected an stt: notice, got %v", notices)
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
	if cfg.SilenceDur != 500*time.Millisecond || cfg.FrameDur != 30*time.Millisecond {
		t.Errorf("timings wrong: %+v", cfg)
	}
	if cfg.EndpointRatio != 3.0 || cfg.MaxUtterance != 15*time.Second {
		t.Errorf("H6.4 endpointing defaults wrong: %+v", cfg)
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

// ------------------------------------------------------------------
// H6.4: full-duplex barge-in v2
// ------------------------------------------------------------------

// outTap continuously drains a call's Out() stream and records, in
// arrival order, the first byte of each audio chunk, each assistant
// text event, and each flush control's arrival time. It stands in for
// the out-socket handler in the tests below.
type outTap struct {
	mu      sync.Mutex
	audio   []byte // first byte of each audio chunk
	texts   []string
	flushes []time.Time
}

func tapOut(c *Call) *outTap {
	tap := &outTap{}
	go func() {
		for chunk := range c.Out() {
			tap.mu.Lock()
			if chunk.Flush {
				tap.flushes = append(tap.flushes, time.Now())
			} else if len(chunk.Audio) > 0 {
				tap.audio = append(tap.audio, chunk.Audio[0])
			} else if chunk.Text != "" {
				tap.texts = append(tap.texts, chunk.Text)
			}
			tap.mu.Unlock()
		}
	}()
	return tap
}

// waitText blocks until n assistant-text events have arrived on the
// tap (or the deadline).
func (tap *outTap) waitTexts(t *testing.T, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		tap.mu.Lock()
		got := len(tap.texts)
		tap.mu.Unlock()
		if got >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d assistant-text events, got %d", n, got)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestCallTenConsecutiveInterruptions is the H6.4 acceptance test,
// mule side: the agent's TTS is interrupted ten times in a row. Each
// interruption must (a) stop the out-stream within the 150 ms mule
// budget (trigger frame → out queue drained + flush control on the
// out stream — the wire + phone tail is the client's budget,
// documented in spec.md) and (b) produce the next utterance boundary
// (the barged-in speech becomes the next full turn).
func TestCallTenConsecutiveInterruptions(t *testing.T) {
	replies := make([]string, 25) // 11 turns + overlap steers all consume one
	for i := range replies {
		replies[i] = "Okay. Adjusting."
	}
	fg, srv := newFakeForge(t, fakeConv, replies)
	fg.ttsDelay = 300 * time.Millisecond // long "playback" window to interrupt
	cfg := testCfg(srv.URL)
	call, err := NewCall(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- call.Run() }()

	hot := makeFrame(480, 9000)
	silence := makeFrame(480, 0)
	tap := tapOut(call)

	var latencies []time.Duration
	for i := 0; i < 10; i++ {
		// One utterance (the first from the listening state; from cycle
		// 1 on the previous cycle's barged-in remainder already ran its
		// own turn, so this utterance overlaps that turn and is
		// submitted as a steer — the barge-in below still lands on the
		// turn's TTS, which is what the test is about): ~1 s of
		// speech, then silence past the 600 ms window.
		feedUtterance(call)
		// Wait for this turn's assistant text: the TTS hop has just
		// begun (the fake holds the response 300 ms), so barge-in
		// frames land while "playback" is in flight.
		tap.waitTexts(t, i+1, 5*time.Second)
		t0 := time.Now()
		for j := 0; j < 3; j++ {
			call.FeedIn(hot) // barge-in hysteresis
		}
		// Complete the barged-in utterance: more speech, then
		// silence past the window.
		for j := 0; j < 20; j++ {
			call.FeedIn(hot)
		}
		for j := 0; j < 24; j++ {
			call.FeedIn(silence)
		}
		// The flush control must have landed on the out stream.
		deadline := time.Now().Add(500 * time.Millisecond)
		var flushAt time.Time
		for {
			tap.mu.Lock()
			if len(tap.flushes) > i {
				flushAt = tap.flushes[i]
				tap.mu.Unlock()
				break
			}
			tap.mu.Unlock()
			if time.Now().After(deadline) {
				t.Fatalf("interrupt %d: no flush control on the out stream", i+1)
			}
			time.Sleep(2 * time.Millisecond)
		}
		latency := flushAt.Sub(t0)
		latencies = append(latencies, latency)
		if latency >= 150*time.Millisecond {
			t.Errorf("interrupt %d: stop-to-flush %v exceeds the 150 ms mule budget", i+1, latency)
		}
	}

	call.HangUp()
	if err := <-done; err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	out := call.Output()
	if got := out["barge_ins"]; got != 10 {
		t.Errorf("barge_ins = %v, want 10", got)
	}
	// 11 turns: cycle 0's utterance + the 10 barge-primed remainders
	// (each interruption produced the next utterance boundary).
	if got := out["turns"]; got != 11 {
		t.Errorf("turns = %v, want 11", got)
	}
	tap.mu.Lock()
	if len(tap.flushes) != 10 {
		t.Errorf("flush controls = %d, want 10", len(tap.flushes))
	}
	tap.mu.Unlock()
	for i, l := range latencies {
		t.Logf("interrupt %02d: stop-to-flush = %v", i+1, l)
	}
}

// TestCallDualChannelLiveness: in-frames and out-frames flow
// SIMULTANEOUSLY for ~2 s against a deliberately slow out client. The
// out queue must stay bounded (drop-oldest with a counter — a late
// chunk is worse than a gap) and the in-VAD bookkeeping must keep
// running: a barge-in fires on time even while the out queue is
// dropping audio.
func TestCallDualChannelLiveness(t *testing.T) {
	// 120 sentences × 30 ms TTS synth: the out-queue (cap 64)
	// overflows against a 200 ms/audio-event consumer, so the call's
	// emitOut drops the oldest events (counted) for the whole window.
	var reply string
	for i := 0; i < 120; i++ {
		reply += fmt.Sprintf("Part %d done. ", i+1)
	}
	fg, srv := newFakeForge(t, fakeConv, []string{reply, "All done."})
	fg.ttsDelay = 30 * time.Millisecond
	cfg := testCfg(srv.URL)
	call, err := NewCall(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- call.Run() }()

	// Slow out consumer: 200 ms per AUDIO event; text + flush control
	// fast-path (a real client never paces control frames behind
	// audio).
	slowTap := &outTap{}
	go func() {
		for {
			select {
			case chunk, ok := <-call.Out():
				if !ok {
					return
				}
				slowTap.mu.Lock()
				if chunk.Flush {
					slowTap.flushes = append(slowTap.flushes, time.Now())
				} else if len(chunk.Audio) > 0 {
					slowTap.audio = append(slowTap.audio, chunk.Audio[0])
				} else if chunk.Text != "" {
					slowTap.texts = append(slowTap.texts, chunk.Text)
				}
				slowTap.mu.Unlock()
				if len(chunk.Audio) > 0 && !chunk.Flush {
					time.Sleep(200 * time.Millisecond)
				}
			case <-time.After(30 * time.Second):
				return
			}
		}
	}()

	// Utterance 1 → the long reply's TTS starts streaming.
	feedUtterance(call)
	slowTap.waitTexts(t, 1, 5*time.Second)

	// In-frames flow for ~2 s of real time (silence) while the out
	// frames out-produce the slow client: dual channels live at once.
	silence := makeFrame(480, 0)
	hot := makeFrame(480, 9000)
	start := time.Now()
	for time.Since(start) < 2*time.Second {
		call.FeedIn(silence)
		time.Sleep(30 * time.Millisecond)
	}

	// Barge into the still-streaming TTS (the out queue is full and
	// dropping at this point). The flush must land quickly — it is
	// emitted after the queue is drained, so it lands ahead of the
	// backlog — and the interruption must be counted.
	hotStream := make(chan struct{})
	go func() {
		for {
			select {
			case <-hotStream:
				return
			default:
				call.FeedIn(hot)
				time.Sleep(5 * time.Millisecond)
			}
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		slowTap.mu.Lock()
		got := len(slowTap.flushes)
		slowTap.mu.Unlock()
		if got >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("barge-in did not fire while dual channels were live")
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(hotStream)
	// Close the barged-in utterance → the next agent turn.
	for i := 0; i < 24; i++ {
		call.FeedIn(silence)
	}
	deadline = time.Now().Add(10 * time.Second)
	for {
		if len(fg.postedContents()) >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the barged-in utterance did not produce the next turn; posts = %q", fg.postedContents())
		}
		time.Sleep(25 * time.Millisecond)
	}

	call.HangUp()
	if err := <-done; err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	out := call.Output()
	if got, _ := out["out_dropped"].(int64); got <= 0 {
		t.Errorf("out_dropped = %v, want > 0 (the slow client must trigger drop-oldest, not a deadlock)", got)
	}
	if got := out["barge_ins"].(int); got < 1 {
		t.Errorf("barge_ins = %v, want >= 1 (in-VAD stayed live while out was dropping)", got)
	}
}

// TestCallOverlapSteerDuringThinking: while the agent's turn is in
// flight (PhaseThinking), overlapping user speech is captured by the
// always-on in-VAD and submitted as a steer — the EARLY PARTIAL at
// the first silence window (tagged), then the FULL utterance at
// endpoint — neither gets a TTS answer (the in-flight turn owns the
// speaker).
func TestCallOverlapSteerDuringThinking(t *testing.T) {
	fg, srv := newFakeForge(t, fakeConv, []string{"Working on that. Give me a moment."})
	fg.agentDelay = 800 * time.Millisecond // hold the agent turn open
	fg.sttDelay = 20 * time.Millisecond    // keep partial/full steer ordering deterministic
	cfg := testCfg(srv.URL)
	call, err := NewCall(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- call.Run() }()

	// Utterance 1: starts the agent turn (the fake holds it 800 ms).
	feedUtterance(call)

	// The overlap utterance, spoken during the agent hop. 20 speech
	// frames at RMS 600, then 21 frames of "noise" at RMS 250 (below
	// the 300 VAD floor, so silence — but 600/250 = 2.4 < 3.0, so the
	// ratio holds the utterance open past the first 600 ms window,
	// where the EARLY PARTIAL fires), then more speech, then digital
	// silence to the endpoint.
	speech := makeFrame(480, 600)
	noise := makeFrame(480, 250)
	silence := makeFrame(480, 0)
	for i := 0; i < 20; i++ {
		call.FeedIn(speech)
	}
	for i := 0; i < 21; i++ {
		call.FeedIn(noise)
	}
	for i := 0; i < 6; i++ {
		call.FeedIn(speech)
	}
	for i := 0; i < 24; i++ {
		call.FeedIn(silence)
	}

	// Both steer posts must land (the agent hop is 800 ms; the overlap
	// utterance is ~1.6 s of frames and is processed in real time
	// inside the polled hop). The three STT calls (turn + partial +
	// full) run concurrently against the fake, so neither transcript
	// numbering nor server-side post order is a mule property —
	// assert the shape instead: three distinct transcripts, exactly
	// ONE tagged with the early-partial marker.
	deadline := time.Now().Add(10 * time.Second)
	for {
		contents := fg.postedContents()
		if len(contents) >= 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("steer posts did not land; posted = %q", fg.postedContents())
		}
		time.Sleep(20 * time.Millisecond)
	}
	contents := fg.postedContents()
	partials, seen := 0, map[string]bool{}
	for _, p := range contents {
		if !strings.Contains(p, "[voice call] ") {
			t.Errorf("post %q missing the voice-call tag", p)
		}
		if strings.Contains(p, "(partial; the complete utterance follows)") {
			partials++
		}
		transcript := p[strings.LastIndex(p, "transcript-"):]
		if seen[transcript] {
			t.Errorf("duplicate transcript %q among posts %q", transcript, contents)
		}
		seen[transcript] = true
	}
	if partials != 1 {
		t.Errorf("early-partial-tagged posts = %d, want 1 (partial at the first window, full at endpoint, untagged turn)", partials)
	}

	// The steers are fire-and-forget: no TTS for them. Turn 1's reply
	// (two sentences) is the only TTS traffic.
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && fg.TTSCount() < 2 {
		time.Sleep(20 * time.Millisecond)
	}
	if got := fg.TTSCount(); got != 2 {
		t.Errorf("ttsCalls = %d, want 2 (steers get no TTS)", got)
	}

	call.HangUp()
	if err := <-done; err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if got := call.Output()["steers"]; got != 2 {
		t.Errorf("steers = %v, want 2 (partial + full)", got)
	}
}

// ---------------------------------------------------------------- Feedback

// nextOut waits for one out event of any kind (fails on timeout/close).
func nextOut(t *testing.T, c *Call, timeout time.Duration) engine.VoiceOutChunk {
	t.Helper()
	select {
	case chunk, ok := <-c.Out():
		if !ok {
			t.Fatal("out channel closed while waiting for an event")
		}
		return chunk
	case <-time.After(timeout):
		t.Fatal("timed out waiting for an out event")
		return engine.VoiceOutChunk{}
	}
}

// waitForOut skips events until one satisfies match (bounded).
func waitForOut(t *testing.T, c *Call, timeout time.Duration, match func(engine.VoiceOutChunk) bool) engine.VoiceOutChunk {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case chunk, ok := <-c.Out():
			if !ok {
				t.Fatal("out channel closed before the expected event")
			}
			if match(chunk) {
				return chunk
			}
		case <-deadline:
			t.Fatal("timed out waiting for the expected out event")
			return engine.VoiceOutChunk{}
		}
	}
}

// TestCallGreetingAndFeedbackLanes covers the 2026-10-17 feedback work:
// the in socket firing the answer-the-call greeting (agent "picks up"
// before the caller says a word, warming the prompt cache) and the
// per-turn {"state": …} / {"user": …} lanes.
func TestCallGreetingAndFeedbackLanes(t *testing.T) {
	fg, srv := newFakeForge(t, fakeConv, []string{
		"Hi, it's Bob.", // greeting turn
		"2 plus 2 is 4.",
	})
	cfg := testCfg(srv.URL)
	call, err := NewCall(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- call.Run() }()

	// Before the in socket attaches there is NO greeting traffic: a
	// job nobody dialed must not spend an agent turn.
	select {
	case chunk := <-call.Out():
		t.Fatalf("out event before attach: %+v", chunk)
	case <-time.After(150 * time.Millisecond):
	}

	call.OnInAttached()

	// The greeting: a thinking state, the spoken reply, then back to
	// listening.
	if got := nextOut(t, call, 5*time.Second).State; got != string(PhaseThinking) {
		t.Errorf("first event state = %q, want %q", got, PhaseThinking)
	}
	greet := waitForOut(t, call, 5*time.Second, func(c engine.VoiceOutChunk) bool { return c.Text != "" })
	if greet.Text != "Hi, it's Bob." {
		t.Errorf("greeting text = %q", greet.Text)
	}
	waitForOut(t, call, 5*time.Second, func(c engine.VoiceOutChunk) bool { return c.State == string(PhaseListening) })

	// The greeting consumed one agent turn (prefix-cache warm-up).
	if out := call.Output(); out["turns"] != 1 {
		t.Errorf("turns after greeting = %v, want 1", out["turns"])
	}

	// A voiced utterance: state thinking → the caller's own words →
	// assistant text → speaking → listening.
	feedUtterance(call)
	waitForOut(t, call, 5*time.Second, func(c engine.VoiceOutChunk) bool { return c.User == "transcript-1" })
	answered := waitForOut(t, call, 5*time.Second, func(c engine.VoiceOutChunk) bool { return c.Text != "" })
	if answered.Text != "2 plus 2 is 4." {
		t.Errorf("answer text = %q", answered.Text)
	}
	waitForOut(t, call, 5*time.Second, func(c engine.VoiceOutChunk) bool { return c.State == string(PhaseListening) })

	if got := fg.TTSCount(); got != 2 {
		t.Errorf("ttsCalls = %d, want 2 (greeting + answer)", got)
	}

	call.HangUp()
	if err := <-done; err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if out := call.Output(); out["turns"] != 2 {
		t.Errorf("turns = %v, want 2 (greeting + one utterance)", out["turns"])
	}
}
