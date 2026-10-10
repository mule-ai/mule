package voice

// call.go — the voice-loop call session (Herd H6.3).
//
// A call is one mule job: the voice_call step blocks in Call.Run until
// the call is hung up (clean end → the job completes) or killed
// (crash/kill path → Run returns an error → the engine marks the job
// failed, which is how both surfaces see "call dropped").
//
// Goroutine tree (owned by the job's step execution):
//
//	Run() (the step's goroutine)  the ONLY reader of inCh, the whole
//	                              audio state machine (listening →
//	                              utterance → thinking → speaking)
//	FeedIn() (in-WS handler)     pushes in-socket PCM frames to inCh
//	out WS handler               drains Out() and writes PCM chunks to
//	                              the /out socket
//
// The single-loop design is deliberate: STT/agent/TTS are blocking
// calls inside the loop, and incoming audio during the agent turn or
// TTS playback simply queues in inCh; barge-in is evaluated from that
// queue right after each TTS chunk is written out (barge-in v1 — see
// bargein.go). One loop = no shared mutable audio state.
//
// Hang-up: a `{"hangup":true}` control frame on the in socket, or the
// in socket closing — both map to HangUp() and end the job COMPLETED.

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mule-ai/mule/internal/engine"
)

// Phase is the call's audio pipeline state.
type Phase string

const (
	PhaseListening Phase = "listening" // waiting for the user to speak
	PhaseUtterance Phase = "utterance" // buffering the user's speech
	PhaseThinking  Phase = "thinking"  // STT + agent turn in flight
	PhaseSpeaking  Phase = "speaking"  // TTS streaming to the out socket
)

// Call is one voice-loop call, bound to one mule job.
type Call struct {
	mu        sync.Mutex
	cfg       CallConfig
	client    *engine.ForgeClient
	jobCtx    context.Context // the engine step's context (job timeout/cancel)
	ctx       context.Context // derived; cancelled by HangUp/Kill
	cancel    context.CancelFunc
	killErr   error                     // non-nil when Kill() was called (crash/kill path)
	hungUp    bool                      // set by HangUp (clean end → job COMPLETED)
	done      chan struct{}             // closed when Run returns
	inCh      chan []byte               // in-socket PCM frames (30 ms @ 16 kHz)
	textCh    chan string               // in-socket typed-utterance frames (text fallback)
	outCh     chan engine.VoiceOutChunk // TTS audio + assistant-text events
	vad       *VAD
	barge     *BargeIn
	buf       []byte // current utterance PCM (Run-goroutine only)
	phase     Phase  // Run-goroutine only
	sessionID string
	turns     int
	bargeIns  int
	dropped   []string // degraded hops (STT/TTS/agent errors), call kept alive
	outOpen   int32    // 1 while the out-socket handler is draining
}

// NewCall builds a call from a parsed config, rooted at ctx (the
// job's context: a job timeout cancels it, which ends the call as a
// dropped call → job FAILED). HangUp/Kill cancel the derived context.
func NewCall(ctx context.Context, cfg CallConfig) (*Call, error) {
	dctx, cancel := context.WithCancel(ctx)
	return &Call{
		cfg:     cfg,
		client:  engine.NewForgeClient(cfg.ForgeURL, cfg.APIKey),
		jobCtx:  ctx,
		ctx:     dctx,
		cancel:  cancel,
		done:    make(chan struct{}),
		inCh:    make(chan []byte, 128), // ~4 s of 30 ms frames
		textCh:  make(chan string, 16),
		outCh:   make(chan engine.VoiceOutChunk, 64),
		vad:     NewVAD(cfg.FrameDur, cfg.SilenceDur, cfg.VADThreshold),
		barge:   NewBargeIn(cfg.BargeInThreshold, 3),
		phase:   PhaseListening,
		outOpen: 1,
	}, nil
}

// MarkOutClosed is called by the out-socket handler when its client
// goes away: TTS synthesis stops (no point streaming audio nobody is
// listening to) while the call itself continues listening to the user.
func (c *Call) MarkOutClosed() { atomic.StoreInt32(&c.outOpen, 0) }

// Phase reports the current audio pipeline phase (surfaces' status
// line).
func (c *Call) Phase() Phase {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.phase
}

// FeedIn pushes one in-socket PCM frame. It blocks when the frame
// buffer is full (backpressure on the WS reader) and gives up when the
// call has ended (the WS handler detects that and closes).
func (c *Call) FeedIn(frame []byte) {
	select {
	case c.inCh <- frame:
	case <-c.done:
	}
}

// FeedText pushes one in-socket typed utterance (the CLI's
// key-typed-text fallback, H6.3: no STT — the text goes straight to
// the agent turn, which still gets a TTS answer).
func (c *Call) FeedText(text string) {
	select {
	case c.textCh <- text:
	case <-c.done:
	}
}

// Out is the read-only out-socket event stream (TTS audio chunks and
// assistant-text metadata events) for the out-socket writer.
func (c *Call) Out() <-chan engine.VoiceOutChunk { return c.outCh }

// HangUp ends the call cleanly (job COMPLETED). Idempotent.
func (c *Call) HangUp() {
	c.mu.Lock()
	c.hungUp = true
	c.mu.Unlock()
	c.cancel()
}

// Kill ends the call as a failure (the job is marked FAILED — the
// "call dropped" path). Idempotent; only the first error wins.
func (c *Call) Kill(err error) {
	c.mu.Lock()
	if c.killErr == nil {
		c.killErr = err
	}
	c.mu.Unlock()
	c.cancel()
}

// Run blocks until the call ends. It returns nil on a clean hang-up,
// the kill error when the call was killed, and a dropped-call error
// when the job's context ended first (job timeout/cancel). It closes
// outCh on the way out so the out-socket writer can finish.
func (c *Call) Run() error {
	defer close(c.outCh)
	defer close(c.done)

	for {
		select {
		case <-c.ctx.Done():
			// Our context was cancelled: either a clean HangUp or a
			// Kill (flagged below) — or the job's context ended, which
			// cancels us without either flag and means the call was
			// dropped (job timeout/cancel → the job fails).
			c.mu.Lock()
			killed, hung := c.killErr, c.hungUp
			c.mu.Unlock()
			if killed != nil || hung {
				return killed
			}
			return fmt.Errorf("voice call dropped: job context ended")
		case frame, ok := <-c.inCh:
			if !ok {
				// Defensive: inCh is never closed by design; treat a
				// close as a hang-up.
				return nil
			}
			c.handleFrame(frame)
		case text := <-c.textCh:
			c.runTurnFrom(strings.TrimSpace(text))
		}
	}
}

func (c *Call) handleFrame(frame []byte) {
	switch c.phase {
	case PhaseListening:
		ev := c.vad.Feed(frame)
		if ev.SpeechStarted {
			c.phase = PhaseUtterance
			c.buf = append([]byte(nil), frame...)
		}
	case PhaseUtterance:
		c.buf = append(c.buf, frame...)
		ev := c.vad.Feed(frame)
		if ev.UtteranceEnded {
			utterance := c.buf
			c.buf = nil
			c.vad.Reset()
			c.runTurn(utterance)
		}
	case PhaseThinking, PhaseSpeaking:
		// Unreachable from the loop: runTurn owns both phases and
		// drains inCh itself (barge-in). Frames queued during a turn
		// are handled by runTurn's drains.
	}
}

// runTurn executes one voiced user utterance: STT → runTurnFrom.
func (c *Call) runTurn(utterance []byte) {
	text, err := c.transcribe(utterance)
	if err != nil {
		c.noteDropped("stt", err)
		return
	}
	c.runTurnFrom(strings.TrimSpace(text))
}

// runTurnFrom executes one utterance's agent + TTS half. It is called
// from the Run loop, so it blocks that loop; audio arriving meanwhile
// sits in inCh and is drained by the barge-in check. An empty text is
// a no-op (silence-only utterances, blank typed lines).
func (c *Call) runTurnFrom(text string) {
	if text == "" {
		return // nothing said: stay where the phase machinery left us
	}
	c.phase = PhaseThinking
	defer func() {
		// A non-barge-in exit goes back to listening. Barge-in already
		// set PhaseUtterance + primed the VAD and must survive.
		if c.phase != PhaseUtterance {
			c.phase = PhaseListening
			c.buf = nil
		}
	}()

	assistant, err := c.agentTurn(text)
	if err != nil {
		c.noteDropped("agent", err)
		return
	}
	assistant = strings.TrimSpace(assistant)
	if assistant == "" {
		return
	}
	// The out-socket assistant-text event (non-audio surfaces print it;
	// audio surfaces may also show it — it is metadata, not audio).
	if atomic.LoadInt32(&c.outOpen) != 0 {
		c.outCh <- engine.VoiceOutChunk{Text: assistant}
	}

	c.phase = PhaseSpeaking
	c.barge.Reset()
	for _, sentence := range splitSentences(assistant) {
		if atomic.LoadInt32(&c.outOpen) == 0 {
			return // out socket gone: stop TTS, keep listening
		}
		audio, err := c.synthesize(sentence)
		if err != nil {
			c.noteDropped("tts", err)
			return
		}
		c.outCh <- engine.VoiceOutChunk{Audio: audio} // out handler drains until the call ends
		if c.drainBargeIn() {
			return // barge-in: phase is PhaseUtterance, VAD primed
		}
	}
}

// drainBargeIn checks queued in-frames for a barge-in trigger while
// TTS audio has just been written. Returns true (with the call in
// PhaseUtterance and the VAD primed on the triggering frame) when the
// user spoke over the TTS.
func (c *Call) drainBargeIn() bool {
	for {
		select {
		case f := <-c.inCh:
			if c.barge.Feed(FrameRMS(f)) {
				c.mu.Lock()
				c.bargeIns++
				c.mu.Unlock()
				c.barge.Reset()
				c.vad.Reset()
				c.vad.Feed(f) // prime: the triggering frame is speech
				c.phase = PhaseUtterance
				c.buf = append([]byte(nil), f...)
				return true
			}
		default:
			return false
		}
	}
}

// transcribe wraps the utterance PCM in a WAV and calls the forge STT
// proxy (POST /v1/audio/transcriptions).
func (c *Call) transcribe(pcm []byte) (string, error) {
	return c.client.Transcribe(c.ctx, EncodeWAV(pcm, InSampleRate), "utterance.wav")
}

// agentTurn steers the agent mid-conversation with the transcript.
// v1 steer-mode note: the H3.2 forge client implements "steer" as a
// plain POST /messages to the target conversation (the forge API has
// no separate mid-turn delivery surface yet); the mode label is kept
// so the upgrade to a real steer endpoint is config-only.
func (c *Call) agentTurn(text string) (string, error) {
	if c.sessionID == "" {
		id, err := c.client.ResolveConversation(c.ctx, c.cfg.AgentID, c.cfg.ConversationID)
		if err != nil {
			return "", err
		}
		c.sessionID = id
	}
	c.mu.Lock()
	c.turns++
	c.mu.Unlock()

	tctx, cancel := context.WithTimeout(c.ctx, c.cfg.TurnTimeout)
	defer cancel()
	content := fmt.Sprintf("[voice call] %s", text)
	assistant, err := c.client.PostAndWait(tctx, c.sessionID, content, c.cfg.TurnTimeout)
	if err != nil {
		return "", err
	}
	return assistant, nil
}

// synthesize speaks one sentence via the forge TTS proxy (the response
// content-type is whatever the backend returns; v1 requests PCM).
func (c *Call) synthesize(sentence string) ([]byte, error) {
	tctx, cancel := context.WithTimeout(c.ctx, 60*time.Second)
	defer cancel()
	_, audio, err := c.client.Speech(tctx, sentence, c.cfg.TTSVoice, c.cfg.TTSResponseFormat)
	return audio, err
}

// noteDropped records a degraded hop. v1 keeps the call alive when a
// hop fails (the agent stays usable over the remaining hops); the
// errors surface in Output() at hang-up.
func (c *Call) noteDropped(hop string, err error) {
	c.mu.Lock()
	c.dropped = append(c.dropped, fmt.Sprintf("%s: %s", hop, err.Error()))
	c.mu.Unlock()
	log.Printf("voice loop: call degraded (%s): %v", hop, err)
}

// Output is the voice_call step's terminal output (job output).
func (c *Call) Output() map[string]interface{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	return map[string]interface{}{
		"status":             "hung_up",
		"turns":              c.turns,
		"barge_ins":          c.bargeIns,
		"forge_conversation": c.sessionID,
		"in_sample_rate":     InSampleRate,
		"out_sample_rate":    c.cfg.TTSRate,
		"out_format":         c.cfg.TTSResponseFormat,
		"dropped":            append([]string(nil), c.dropped...),
	}
}

// splitSentences chunks text into TTS sentences. Split on sentence
// terminators (keeping the terminator with its sentence); long
// fragments fall through. Deliberately dumb — Kokoro's per-sentence
// calls are the streaming unit in v1 (H6.4's real streaming TTS is
// the upgrade).
func splitSentences(text string) []string {
	var out []string
	var cur strings.Builder
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		cur.WriteRune(runes[i])
		if runes[i] == '.' || runes[i] == '!' || runes[i] == '?' {
			// Consume trailing closing quote/paren if present.
			if i+1 < len(runes) && (runes[i+1] == '"' || runes[i+1] == '”' || runes[i+1] == ')' || runes[i+1] == ']') {
				cur.WriteRune(runes[i+1])
				i++
			}
			s := strings.TrimSpace(cur.String())
			if s != "" {
				out = append(out, s)
			}
			cur.Reset()
		}
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		out = append(out, s)
	}
	if len(out) == 0 {
		out = append(out, text)
	}
	return out
}

// ---------------------------------------------------------------- Hub

// Hub maps job IDs to live calls. It is the seam between the engine
// (which starts/forgets calls for voice_call steps) and the API layer
// (which routes the /voice/{job_id}/in|out sockets to a call).
type Hub struct {
	mu    sync.Mutex
	calls map[string]*Call
}

// NewHub creates an empty call hub.
func NewHub() *Hub {
	return &Hub{calls: make(map[string]*Call)}
}

// StartCall implements the engine's voice-executor seam: it builds the
// call for a voice_call step and registers it under the job id (the
// job id is the call's address on the voice sockets).
func (h *Hub) StartCall(jobCtx context.Context, jobID string, stepConfig map[string]interface{}, _ map[string]interface{}) (engine.VoiceCall, error) {
	cfg, err := ParseCallConfig(stepConfig)
	if err != nil {
		return nil, err
	}
	call, err := NewCall(jobCtx, cfg)
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, exists := h.calls[jobID]; exists {
		call.HangUp()
		return nil, fmt.Errorf("voice call for job %s already exists", jobID)
	}
	h.calls[jobID] = call
	return call, nil
}

// GetCall looks up a live call by job id.
func (h *Hub) GetCall(jobID string) (*Call, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	c, ok := h.calls[jobID]
	return c, ok
}

// WaitCall polls for a call to appear (the voice sockets can attach
// right after job creation, before the engine worker starts the
// step). It gives up at the timeout.
func (h *Hub) WaitCall(ctx context.Context, jobID string, timeout time.Duration) (*Call, bool) {
	deadline := time.Now().Add(timeout)
	for {
		if c, ok := h.GetCall(jobID); ok {
			return c, true
		}
		if time.Now().After(deadline) {
			return nil, false
		}
		select {
		case <-ctx.Done():
			return nil, false
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// ForgetCall removes a call from the hub (the engine calls it when
// the step finishes, whatever the outcome).
func (h *Hub) ForgetCall(jobID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.calls, jobID)
}
