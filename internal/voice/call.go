package voice

// call.go — the voice-loop call session (Herd H6.3, full-duplex v2 in
// H6.4).
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
// One loop still owns the audio state — v2 keeps that invariant, but
// the blocking hops (STT / agent / TTS) run in sub-goroutines and the
// loop polls their result with a select over inCh/textCh
// (pollHop). That is the H6.4 full-duplex core:
//
//   - the in-VAD bookkeeping runs in EVERY state. While the agent is
//     thinking, overlapping user speech is captured by the VAD and,
//     when utterance-complete, submitted as a steer (fire-and-forget
//     POST to the agent's conversation, tagged, no TTS — the agent is
//     busy). While TTS plays, the barge-in machine feeds the same
//     stream in real time instead of only from post-chunk drains.
//   - the out-queue never stalls the loop: emitOut drops the OLDEST
//     event when the client is slower than TTS (counted; a late chunk
//     is worse than a gap), so out-queue pressure cannot pause
//     in-VAD bookkeeping.
//   - barge-in flush (H6.4, mule-boundary target < 150 ms): on
//     trigger the loop abandons the in-flight hop, DRAINS the out
//     queue (no further audio is enqueued) and emits a flush control
//     event so the client drops its own playback buffer (the wire +
//     phone tail is the client's budget — see spec.md "Voice loop
//     v2").
//
// Hang-up: a `{"hangup":true}` control frame on the in socket, or the
// in socket closing — both map to HangUp() and end the job COMPLETED.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mule-ai/mule/internal/engine"
)

// Phase is the call's audio pipeline state (plus PhaseGreeting, a
// client-facing label for the answer-the-call process).
type Phase string

const (
	PhaseListening Phase = "listening" // waiting for the user to speak
	PhaseUtterance Phase = "utterance" // buffering the user's speech
	PhaseThinking  Phase = "thinking"  // STT + agent turn in flight
	PhaseSpeaking  Phase = "speaking"  // TTS streaming to the out socket
	// PhaseGreeting: the answer-the-call turn is running — the caller
	// may talk over it (that cancels it; see startGreeting).
	PhaseGreeting Phase = "greeting"
)

// Sentinel hop outcomes (pollHop): they are not real failures.
var (
	errBargeIn   = errors.New("barge-in: user speech interrupted the turn")
	errCallEnded = errors.New("call ended")
)

// Call is one voice-loop call, bound to one mule job.
type Call struct {
	mu      sync.Mutex
	cfg     CallConfig
	client  *engine.ForgeClient
	jobCtx  context.Context // the engine step's context (job timeout/cancel)
	ctx     context.Context // derived; cancelled by HangUp/Kill
	cancel  context.CancelFunc
	killErr error                     // non-nil when Kill() was called (crash/kill path)
	hungUp  bool                      // set by HangUp (clean end → job COMPLETED)
	done    chan struct{}             // closed when Run returns
	inCh    chan []byte               // in-socket PCM frames (30 ms @ 16 kHz)
	textCh  chan string               // in-socket typed-utterance frames (text fallback)
	outCh   chan engine.VoiceOutChunk // TTS audio + assistant-text + flush events
	vad     *VAD
	barge   *BargeIn
	buf     []byte // current utterance PCM (Run-goroutine only)
	phase   Phase  // Run-goroutine only
	// turnBufActive (Run-goroutine only): the in-turn overlap utterance
	// is mid-buffer (SpeechStarted seen, endpoint not yet) while the
	// agent turn is in flight.
	turnBufActive bool
	// pendingTexts: typed in-frames that arrived while a turn was in
	// flight; run after the turn (Run-goroutine only).
	pendingTexts []string
	// lastState dedupes the out-socket {"state": …} events (mu-guarded:
	// the greeting goroutine and the Run loop both emit).
	lastState Phase
	// Answer-the-call greeting (2026-10-17): a simple async, cancellable
	// process started when the in socket attaches. It never gates the
	// call — the caller can speak immediately (first prompts can run
	// 5 min+ on the model side) — and "cancellable" cancels its SPEAKING
	// (a greeting nobody waited for is stale news), while its agent turn
	// keeps warming the prompt cache server-side either way. mu-guarded:
	// greetCancel non-nil while the process is alive; greetMuted closed
	// when caller activity cancelled it.
	greetCancel   context.CancelFunc
	greetMuted    chan struct{}
	greetMuteOnce *sync.Once
	greetState    string // none|pending|answered|cancelled|failed (Output)
	// modelApplied: the per-call voice-model switch has been attempted
	// (mu-guarded; one shot — see ensureModel).
	modelApplied bool
	// turnMu serializes AGENT-TURN WAITS (greeting vs user turns; hold
	// around agentTurn only, never around TTS). Two concurrent
	// PostAndWait calls on one conversation race illegitimately: turn B's
	// waiter can settle on turn A's late-landing assistant row (messages
	// carry no request id — forge cannot say which reply answers what).
	// Queueing the wait behind the greeting also keeps the user's POST
	// from landing mid-greeting-turn server-side, which is the same
	// crossing. The caller never notices: pollHop keeps in-frames
	// flowing while the wait blocks on the lock (status lane: thinking).
	turnMu      sync.Mutex
	inReady     chan struct{}
	inReadyOnce sync.Once
	sessionID   string // mu-guarded (steer goroutines read it)
	turns       int    // mu-guarded
	bargeIns    int    // mu-guarded
	steers      int    // mu-guarded: overlap utterances submitted mid-turn
	outDropped  int64  // atomic: out events dropped (slow client / flush)
	dropped     []string
	outOpen     int32 // 1 while the out-socket handler is draining
}

// NewCall builds a call from a parsed config, rooted at ctx (the
// job's context: a job timeout cancels it, which ends the call as a
// dropped call → job FAILED). HangUp/Kill cancel the derived context.
func NewCall(ctx context.Context, cfg CallConfig) (*Call, error) {
	dctx, cancel := context.WithCancel(ctx)
	vad := NewVAD(cfg.FrameDur, cfg.SilenceDur, cfg.VADThreshold)
	vad.RatioThreshold = cfg.EndpointRatio
	vad.MaxUtteranceDur = cfg.MaxUtterance
	return &Call{
		cfg:       cfg,
		client:    engine.NewForgeClient(cfg.ForgeURL, cfg.APIKey),
		jobCtx:    ctx,
		ctx:       dctx,
		cancel:    cancel,
		done:      make(chan struct{}),
		inCh:      make(chan []byte, 128), // ~4 s of 30 ms frames
		textCh:    make(chan string, 16),
		outCh:     make(chan engine.VoiceOutChunk, 64),
		vad:       vad,
		barge:     NewBargeIn(cfg.BargeInThreshold, 3),
		phase:     PhaseListening,
		lastState: PhaseListening,
		inReady:   make(chan struct{}),
		outOpen:   1,
	}, nil
}

// MarkOutClosed is called by the out-socket handler when its client
// goes away: TTS synthesis stops (no point streaming audio nobody is
// listening to) while the call itself continues listening to the user.
func (c *Call) MarkOutClosed() { atomic.StoreInt32(&c.outOpen, 0) }

// OnInAttached is called by the in-socket handler once the caller's
// WebSocket is up: it fires the answer-the-call greeting exactly once
// (Run's greetCh case).
func (c *Call) OnInAttached() { c.inReadyOnce.Do(func() { close(c.inReady) }) }

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

// Out is the read-only out-socket event stream (TTS audio chunks,
// assistant-text metadata events, and the H6.4 flush control event)
// for the out-socket writer.
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

	// answer-the-call (2026-10-17): once the caller's in socket is up,
	// start the greeting PROCESS (async: a 5-minute first prompt must
	// never gate the call — see startGreeting). A call nobody dials
	// never greets.
	var greetCh <-chan struct{} = c.inReady

	for {
		select {
		case <-greetCh:
			greetCh = nil
			c.startGreeting()
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
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			// Real caller news beats the greeting.
			c.cancelGreeting()
			if c.phase == PhaseListening {
				c.runTurnFrom(text)
			} else {
				// Arrived while a turn was in flight (or the user is
				// mid-utterance after a barge-in): run it afterwards.
				c.pendingTexts = append(c.pendingTexts, text)
			}
		}
	}
}

func (c *Call) handleFrame(frame []byte) {
	switch c.phase {
	case PhaseListening:
		ev := c.vad.Feed(frame)
		if ev.SpeechStarted {
			// The caller has started talking: the greeting (if still
			// coming) is cancelled — it will not speak.
			c.cancelGreeting()
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
		// ev.Partial on the plain path is deliberately ignored: there
		// is no in-flight agent turn to steer, and starting the agent
		// on partial text would leave nothing to say when the full
		// utterance lands (the agent has no partial-prompt surface —
		// documented in spec.md "Voice loop v2"). Early partials apply
		// to overlap steers only (handleActiveFrame).
	case PhaseThinking, PhaseSpeaking:
		// Unreachable from the loop: runTurn owns both phases and
		// polls inCh itself (handleActiveFrame).
	}
}

// runTurn executes one voiced user utterance: STT → runTurnFrom. The
// STT hop is polled too, so in-frames keep flowing while it runs.
func (c *Call) runTurn(utterance []byte) {
	c.phase = PhaseThinking
	c.emitState(PhaseThinking) // "…" the moment the endpoint fires
	text, err := pollHop(c, func() (string, error) { return c.transcribe(utterance) })
	if err != nil {
		if err != errCallEnded {
			c.noteDropped("stt", err)
		}
		if c.phase != PhaseUtterance {
			c.phase = PhaseListening
			c.buf = nil
			c.emitState(PhaseListening)
		}
		c.drainPendingTexts()
		return
	}
	c.runTurnFrom(strings.TrimSpace(text))
}

// runTurnFrom executes one utterance's agent + TTS half. It blocks the
// Run loop via pollHop, but in-frames are handled in real time
// (handleActiveFrame) while each hop is in flight — that is the v2
// full-duplex behavior. An empty text is a no-op (silence-only
// utterances, blank typed lines).
func (c *Call) runTurnFrom(text string) {
	if text == "" {
		// runTurn set PhaseThinking; nothing said (silence-only
		// utterance, blank typed line) — back to listening.
		c.phase = PhaseListening
		c.buf = nil
		c.emitState(PhaseListening)
		c.drainPendingTexts()
		return
	}
	c.phase = PhaseThinking
	c.emitState(PhaseThinking)
	// Live transcript: the caller sees their own words (final STT or
	// typed line) while the agent is still composing.
	if atomic.LoadInt32(&c.outOpen) != 0 {
		c.emitOut(engine.VoiceOutChunk{User: text})
	}
	c.vad.Reset() // fresh overlap tracking for the duration of the turn
	c.turnBufActive = false
	c.buf = nil
	defer func() {
		// A barge-in already set PhaseUtterance + primed the VAD and
		// must survive.
		if c.phase != PhaseUtterance {
			c.phase = PhaseListening
			c.buf = nil
			c.emitState(PhaseListening)
		}
		c.drainPendingTexts()
	}()

	assistant, err := pollHop(c, func() (string, error) {
		// Serialize agent-turn waits (see Call.turnMu): a greeting or a
		// previous user turn still being waited on owns the conversation.
		c.turnMu.Lock()
		defer c.turnMu.Unlock()
		return c.agentTurn(c.ctx, text)
	})
	if err == errBargeIn {
		return // phase is PhaseUtterance, VAD primed, buf intact
	}
	if err != nil {
		if err != errCallEnded {
			c.noteDropped("agent", err)
		}
		return
	}
	assistant = strings.TrimSpace(assistant)
	if assistant == "" {
		return
	}
	// The out-socket assistant-text event (non-audio surfaces print it;
	// audio surfaces may also show it — it is metadata, not audio).
	if atomic.LoadInt32(&c.outOpen) != 0 {
		c.emitOut(engine.VoiceOutChunk{Text: assistant})
	}

	c.phase = PhaseSpeaking
	c.emitState(PhaseSpeaking)
	c.barge.Reset()
	for _, sentence := range splitSentences(assistant) {
		if atomic.LoadInt32(&c.outOpen) == 0 {
			return // out socket gone: stop TTS, keep listening
		}
		audio, err := pollHop(c, func() ([]byte, error) { return c.synthesize(c.ctx, sentence) })
		if err == errBargeIn {
			return // phase is PhaseUtterance, VAD primed, buf intact
		}
		if err != nil {
			if err != errCallEnded {
				c.noteDropped("tts", err)
			}
			return
		}
		c.emitOut(engine.VoiceOutChunk{Audio: audio})
	}
}

// handleActiveFrame processes one in-frame while an agent turn is in
// flight (PhaseThinking/PhaseSpeaking) — the H6.4 full-duplex core,
// called from pollHop's select so it runs in real time:
//
//   - Speaking: the frame feeds the barge-in machine. On trigger the
//     out queue is drained and a flush control event is emitted
//     (mule-boundary stop-to-drain target < 150 ms), the VAD is
//     re-primed on the triggering frame, and the utterance buffer
//     CONTINUES from where it already was — any audio accumulated
//     during PhaseThinking is kept, so the in-utterance buffer is
//     continuous across the state change.
//   - Thinking: the frame feeds the VAD itself (the in-VAD runs in
//     every state). A completed overlap utterance is submitted as a
//     steer (fire-and-forget, no TTS); at the early-partial point the
//     audio-so-far is submitted with the partial tag so the agent can
//     begin on partial text.
//
// It returns true when the in-flight hop must be abandoned (barge-in).
func (c *Call) handleActiveFrame(f []byte) bool {
	switch c.phase {
	case PhaseSpeaking:
		if !c.barge.Feed(FrameRMS(f)) {
			return false
		}
		c.mu.Lock()
		c.bargeIns++
		c.mu.Unlock()
		c.cancelGreeting() // a barge-in buries the greeting too
		c.barge.Reset()
		c.flushOut() // drain the out queue + {"flush":true} control
		c.vad.Reset()
		c.vad.Feed(f) // prime: the triggering frame is speech
		c.phase = PhaseUtterance
		c.turnBufActive = false
		// Continuity: keep the thinking-phase audio if any.
		c.buf = append(c.buf, f...)
		return true
	case PhaseThinking:
		ev := c.vad.Feed(f)
		if ev.SpeechStarted {
			c.cancelGreeting() // caller speech beats the greeting
			c.turnBufActive = true
			c.buf = append([]byte(nil), f...)
		} else if c.turnBufActive {
			c.buf = append(c.buf, f...)
		}
		if c.turnBufActive {
			if ev.Partial && !ev.UtteranceEnded {
				c.submitSteer(copyBytes(c.buf), true)
			}
			if ev.UtteranceEnded {
				c.submitSteer(c.buf, false)
				c.buf = nil
				c.turnBufActive = false
				c.vad.Reset()
			}
		}
		return false
	default:
		return false
	}
}

// drainPendingTexts runs one typed utterance that queued while a turn
// was in flight (the Run loop's textCh case does the rest: each call
// drains one more, so several queue in order).
func (c *Call) drainPendingTexts() {
	if c.phase != PhaseListening || len(c.pendingTexts) == 0 {
		return
	}
	next := c.pendingTexts[0]
	c.pendingTexts = c.pendingTexts[1:]
	c.runTurnFrom(next)
}

// drainPendingUtterances is gone (2026-10-17 redesign): the async
// greeting never captures caller speech — utterances during it run as
// normal turns (their agent-turn WAIT queues behind the greeting's via
// turnMu, which is both honest and crossing-proof).

// greetingWaitTimeout bounds the greeting's AGENT-TURN WAIT only (the
// TTS that follows is bounded per sentence). First prompts can run
// minutes (cold harness, big context); past this the greeting is no
// longer worth answering a call with — HangUp cancels it sooner anyway.
const greetingWaitTimeout = 10 * time.Minute

// greetingPrompt asks the agent to answer the call. The [voice call]
// tag is prepended by agentTurn (prefix continuity with real turns).
const greetingPrompt = "(system) The voice call just connected; the caller is listening. Answer the call with a brief, warm greeting — one short sentence at most. The caller's words will arrive as [voice call] messages."

// startGreeting launches the answer-the-call process (async, never
// blocks the Run loop): one agent turn whose reply is captioned +
// spoken as "the agent answers your call", warming the conversation's
// prompt cache (system prompt, tools, history) so the first real
// utterance is a cache hit. Cancellation is SPEAKING-level and
// one-way: the first VAD speech or typed line from the caller mutes
// the greeting (stale news; queued audio is flushed), while the agent
// turn itself keeps running server-side — the cache warm-up stands
// even when nobody waits for the greeting. Hang-up (c.ctx) kills it
// outright.
func (c *Call) startGreeting() {
	c.mu.Lock()
	if c.greetCancel != nil || c.hungUp || c.killErr != nil {
		c.mu.Unlock()
		return
	}
	gctx, cancel := context.WithTimeout(c.ctx, greetingWaitTimeout)
	muted := make(chan struct{})
	once := &sync.Once{}
	c.greetCancel = cancel
	c.greetMuted = muted
	c.greetMuteOnce = once
	c.greetState = "pending"
	c.mu.Unlock()

	c.emitState(PhaseGreeting)

	go func() {
		defer cancel() // release gctx only once the whole process has finished
		isMuted := func() bool {
			select {
			case <-muted:
				return true
			default:
				return false
			}
		}
		settle := func(state string) {
			c.mu.Lock()
			if c.greetMuted == muted { // still OURS (CancelFuncs are uncomparable)
				c.greetCancel = nil
				c.greetMuted = nil
				c.greetMuteOnce = nil
				if c.greetState == "pending" {
					c.greetState = state
				}
			}
			c.mu.Unlock()
		}

		// The agent-turn wait holds the conversation (turnMu) — user
		// turns queue behind it (see the Call.turnMu comment). The
		// voice-model switch (if chosen) lands FIRST, inside the same
		// lock hold: no agent turn can post before the conversation's
		// model is re-pointed.
		reply, err := func() (string, error) {
			c.turnMu.Lock()
			defer c.turnMu.Unlock()
			c.ensureModel(gctx)
			return c.agentTurn(gctx, greetingPrompt)
		}()
		if gctx.Err() != nil {
			settle("cancelled") // call ended / wait timed out
			return
		}
		if err != nil {
			if !isMuted() {
				c.noteDropped("greeting", err)
				c.emitState(PhaseListening)
			}
			settle("failed")
			return
		}
		reply = strings.TrimSpace(reply)
		if isMuted() || reply == "" {
			settle("cancelled")
			return
		}
		settle("answered")
		if atomic.LoadInt32(&c.outOpen) != 0 {
			c.emitOut(engine.VoiceOutChunk{Text: reply})
		}
		c.emitState(PhaseSpeaking)
		for _, sentence := range splitSentences(reply) {
			if isMuted() || gctx.Err() != nil || atomic.LoadInt32(&c.outOpen) == 0 {
				break
			}
			audio, err := c.synthesize(gctx, sentence)
			if err != nil {
				if gctx.Err() == nil && !isMuted() {
					c.noteDropped("tts", err)
					c.emitState(PhaseListening)
				}
				break
			}
			// Re-check after the (slow) synth: speech during it must not
			// be followed by greeting audio.
			if isMuted() || gctx.Err() != nil {
				break
			}
			c.emitOut(engine.VoiceOutChunk{Audio: audio})
		}
		if !isMuted() && gctx.Err() == nil {
			c.emitState(PhaseListening)
		}
	}()
}

// cancelGreeting mutes the greeting (idempotent; any goroutine may
// call it — the Run loop on caller speech, now and forever). The
// agent-turn WAIT is deliberately NOT cancelled: it holds turnMu and
// its completion is what keeps later user turns from posting mid-
// greeting-turn (and its reply warms the cache). Queued greeting audio
// IS flushed so a half-spoken greeting stops at the mule boundary.
func (c *Call) cancelGreeting() {
	c.mu.Lock()
	once := c.greetMuteOnce
	muted := c.greetMuted
	alive := c.greetCancel != nil
	if alive {
		c.greetState = "cancelled"
	}
	c.mu.Unlock()
	if !alive || once == nil {
		return
	}
	once.Do(func() {
		close(muted)
		c.flushOut()
	})
}

// emitState pushes a deduped phase-change event ({"state": …}) on the
// out stream — the client's live "is it working?" lane (listening /
// thinking / speaking / greeting).
func (c *Call) emitState(p Phase) {
	c.mu.Lock()
	if c.lastState == p {
		c.mu.Unlock()
		return
	}
	c.lastState = p
	c.mu.Unlock()
	if atomic.LoadInt32(&c.outOpen) != 0 {
		c.emitOut(engine.VoiceOutChunk{State: string(p)})
	}
}

// pollHop runs f in a sub-goroutine and pumps the in-frame/text
// stream until the hop completes or the call ends (H6.4: barge-in and
// in-turn steers are detected in real time, even while STT/agent/TTS
// are in flight). Returns the hop's result, or a sentinel error
// (errBargeIn / errCallEnded) when the loop must move on.
func pollHop[T any](c *Call, f func() (T, error)) (T, error) {
	var zero T
	ch := make(chan hopResult[T], 1)
	go func() {
		v, err := f()
		ch <- hopResult[T]{value: v, err: err}
	}()
	for {
		select {
		case <-c.ctx.Done():
			return zero, errCallEnded
		case hr := <-ch:
			return hr.value, hr.err
		case f := <-c.inCh:
			if c.handleActiveFrame(f) {
				return zero, errBargeIn
			}
		case t := <-c.textCh:
			c.pendingTexts = append(c.pendingTexts, strings.TrimSpace(t))
		}
	}
}

type hopResult[T any] struct {
	value T
	err   error
}

// flushOut stops TTS at the mule boundary (H6.4 fast barge-in): it
// drains the out queue — every queued audio chunk is dropped, not
// sent, so nothing after the flush is stale — and emits the flush
// control event so the client drops ITS playback buffer (an
// AudioTrack holds queued audio beyond mule's knowledge; the wire +
// phone tail is the client's budget, documented in spec.md).
func (c *Call) flushOut() {
	// NB: `break` inside a select exits the select, not this loop —
	// the drain must break the LOOP (labelled) or it busy-spins on
	// the default clause forever once the queue is empty.
drain:
	for {
		select {
		case <-c.outCh:
			atomic.AddInt64(&c.outDropped, 1)
			continue
		default:
			break drain
		}
	}
	c.emitOut(engine.VoiceOutChunk{Flush: true})
}

// emitOut pushes one out event, dropping the OLDEST event when the
// queue is full (the client is slower than TTS: audio is
// latency-sensitive, a late chunk is worse than a gap; drops are
// counted and reported in Output). It never blocks, so out-queue
// pressure can never pause in-VAD bookkeeping.
func (c *Call) emitOut(chunk engine.VoiceOutChunk) {
	select {
	case c.outCh <- chunk:
		return
	default:
	}
	for {
		select {
		case <-c.outCh:
			atomic.AddInt64(&c.outDropped, 1)
		default:
		}
		select {
		case c.outCh <- chunk:
			return
		case <-c.done:
			return
		}
	}
}

// submitSteer transcribes an overlap utterance (spoken while the
// agent's own turn was in flight) and posts it to the agent's
// conversation without waiting — the agent is busy; the steer lands
// as its next message. No TTS: the in-flight turn owns the speaker.
// Runs in its own goroutine so the STT POST cannot stall the Run
// loop (and therefore barge-in detection). partial=true tags the
// early partial (first ~silence_ms trailing pause); the full
// utterance follows at endpoint.
func (c *Call) submitSteer(utterance []byte, partial bool) {
	go func() {
		text, err := c.transcribe(utterance)
		if err != nil {
			hop := "steer-stt"
			if partial {
				hop = "steer-stt(partial)"
			}
			c.noteDropped(hop, err)
			return
		}
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		c.mu.Lock()
		c.steers++
		c.mu.Unlock()
		tag := "[voice call] "
		if partial {
			tag += "(partial; the complete utterance follows) "
		}
		if err := c.steerPost(tag + text); err != nil {
			c.noteDropped("steer", err)
		}
	}()
}

// steerPost posts content to the agent's conversation fire-and-forget
// (no turn wait — the agent is mid-turn).
func (c *Call) steerPost(content string) error {
	c.mu.Lock()
	session := c.sessionID
	c.mu.Unlock()
	if session == "" {
		id, err := c.client.ResolveConversation(c.ctx, c.cfg.AgentID, c.cfg.ConversationID)
		if err != nil {
			return err
		}
		c.mu.Lock()
		if c.sessionID == "" {
			c.sessionID = id
		}
		c.mu.Unlock()
		session = c.sessionID
	}
	sctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()
	return c.client.PostMessage(sctx, session, content)
}

// ensureModel applies the per-call "voice model" choice once: resolve
// the conversation, match the label against forge's profiles, and
// PATCH the session (forge rebinds the live harness conversation via
// pi-durable setModel). Failures are DEGRADED-NOT-DEAD: a ⚠ notice
// rides the out socket and the call proceeds on the agent's default
// profile — a wrong model is better than no call.
func (c *Call) ensureModel(ctx context.Context) {
	c.mu.Lock()
	model, applied := c.cfg.Model, c.modelApplied
	c.mu.Unlock()
	if model == "" || applied {
		return
	}
	c.mu.Lock()
	c.modelApplied = true // one shot per call (no per-turn retry spam)
	session := c.sessionID
	c.mu.Unlock()
	if session == "" {
		id, err := c.client.ResolveConversation(ctx, c.cfg.AgentID, c.cfg.ConversationID)
		if err != nil {
			c.noteDropped("model", err)
			return
		}
		c.mu.Lock()
		if c.sessionID == "" {
			c.sessionID = id
		}
		session = c.sessionID
		c.mu.Unlock()
	}
	profile, err := c.client.MatchProfile(ctx, model)
	if err != nil {
		c.noteDropped("model", err)
		return
	}
	if err := c.client.SetModel(ctx, session, profile); err != nil {
		c.noteDropped("model", err)
		return
	}
	log.Printf("voice loop: call model set to %s (%s @ %s)", profile.Name, profile.Model, profile.BaseURL)
}

// transcribe wraps the utterance PCM in a WAV and calls the forge STT
// proxy (POST /v1/audio/transcriptions).
func (c *Call) transcribe(pcm []byte) (string, error) {
	return c.client.Transcribe(c.ctx, EncodeWAV(pcm, InSampleRate), "utterance.wav")
}

// agentTurn steers the agent mid-conversation with the transcript.
// v1 steer-mode note (kept): the H3.2 forge client implements "steer"
// as a plain POST /messages to the target conversation (the forge API
// has no separate mid-turn delivery surface yet); the mode label is
// kept so the upgrade to a real steer endpoint is config-only.
func (c *Call) agentTurn(ctx context.Context, text string) (string, error) {
	c.mu.Lock()
	session := c.sessionID
	c.mu.Unlock()
	if session == "" {
		id, err := c.client.ResolveConversation(ctx, c.cfg.AgentID, c.cfg.ConversationID)
		if err != nil {
			return "", err
		}
		c.mu.Lock()
		if c.sessionID == "" {
			c.sessionID = id
		}
		c.mu.Unlock()
		session = c.sessionID
	}
	c.mu.Lock()
	c.turns++
	c.mu.Unlock()

	tctx, cancel := context.WithTimeout(ctx, c.cfg.TurnTimeout)
	defer cancel()
	content := fmt.Sprintf("[voice call] %s", text)
	assistant, err := c.client.PostAndWait(tctx, session, content, c.cfg.TurnTimeout)
	if err != nil {
		return "", err
	}
	return assistant, nil
}

// synthesize speaks one sentence via the forge TTS proxy (the response
// content-type is whatever the backend returns; v1 requests PCM).
func (c *Call) synthesize(ctx context.Context, sentence string) ([]byte, error) {
	tctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	_, audio, err := c.client.Speech(tctx, sentence, c.cfg.TTSVoice, c.cfg.TTSResponseFormat)
	return audio, err
}

// noteDropped records a degraded hop. The call stays alive when a hop
// fails (the agent stays usable over the remaining hops); the errors
// surface in Output() at hang-up AND as an out-socket {"notice": …}
// text frame, so the caller is told "something broke" instead of
// sitting in silence (2026-10-16: every mic call died here silently
// when the WAV magic bytes broke STT — the phone showed a live call
// and heard nothing).
func (c *Call) noteDropped(hop string, err error) {
	c.mu.Lock()
	c.dropped = append(c.dropped, fmt.Sprintf("%s: %s", hop, err.Error()))
	c.mu.Unlock()
	log.Printf("voice loop: call degraded (%s): %v", hop, err)
	msg := err.Error()
	if len(msg) > 160 {
		msg = msg[:160] + "…"
	}
	c.emitOut(engine.VoiceOutChunk{Notice: fmt.Sprintf("%s: %s", hop, msg)})
}

// Output is the voice_call step's terminal output (job output).
func (c *Call) Output() map[string]interface{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	dropped := append([]string(nil), c.dropped...)
	return map[string]interface{}{
		"status":             "hung_up",
		"turns":              c.turns,
		"greeting":           c.greetState,
		"barge_ins":          c.bargeIns,
		"steers":             c.steers,
		"out_dropped":        atomic.LoadInt64(&c.outDropped),
		"forge_conversation": c.sessionID,
		"in_sample_rate":     InSampleRate,
		"out_sample_rate":    c.cfg.TTSRate,
		"out_format":         c.cfg.TTSResponseFormat,
		"dropped":            dropped,
	}
}

// copyBytes returns a copy of b (steer partials must snapshot the
// buffer; it keeps growing).
func copyBytes(b []byte) []byte {
	return append([]byte(nil), b...)
}

// splitSentences chunks text into TTS sentences. Split on sentence
// terminators (keeping the terminator with its sentence); long
// fragments fall through. Deliberately dumb — Kokoro's per-sentence
// calls are the streaming unit; sentence-chunked streaming is the
// v2 transport (each sentence is one out-socket binary frame, the
// barge-in flush is what makes it interruptible).
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
func (h *Hub) StartCall(jobCtx context.Context, jobID string, stepConfig map[string]interface{}, jobInput map[string]interface{}) (engine.VoiceCall, error) {
	cfg, err := ParseCallConfig(stepConfig)
	if err != nil {
		return nil, err
	}
	// Per-job overrides (POST /api/v1/jobs input_data): the caller's
	// chosen "voice model" wins over the step config (the phone/CLI
	// picker). Other knobs stay step-owned.
	if m, ok := jobInput["model"].(string); ok && strings.TrimSpace(m) != "" {
		cfg.Model = strings.TrimSpace(m)
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
