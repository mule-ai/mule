package voice

// vad.go — the voice-loop voice-activity detector + endpointer (Herd
// H6.3/H6.4).
//
// Energy-based VAD over 16 kHz mono s16le PCM frames (~30 ms each):
// a frame is "speech" when its RMS is at or above a threshold. The
// VAD tracks whether the caller is mid-utterance and reports events:
//
//   - SpeechStarted: first speech frame of an utterance;
//   - Partial: the first trailing-silence window has elapsed (the H6.4
//     early-partial point — the call submits the audio it has so far,
//     for in-turn steers; fires once per utterance);
//   - UtteranceEnded: the H6.4 endpointing rule closed the utterance.
//
// Endpointing v2 (H6.4): RNN-free "silence + energy ratio". An
// utterance ends when the trailing silence is at least the silence
// window AND the energy ratio mean(last 400 ms of speech frames) /
// mean(last 300 ms of below-threshold frames) is at least the ratio
// threshold (~3.0) — i.e. the silence is actually quiet relative to
// the speech. Digital silence (mean ≈ 0) passes as an infinite ratio.
// A hard cap (default 15 s of open utterance) force-ends even
// mid-speech. The decision itself is the pure DecideEndpoint, so the
// rule is unit-testable without a VAD instance.
//
// No dependency: a small pure state machine fed frame-by-frame. The
// VAD holds no timestamps — frame duration is assumed constant (the
// WS audio-in protocol's 30 ms contract).

import (
	"fmt"
	"math"
	"time"
)

// VAD reports speech-start / early-partial / utterance-end events as
// frames are fed.
type VAD struct {
	frameDur      time.Duration // nominal duration of one frame
	silenceDur    time.Duration // trailing-silence window (min + partial point)
	threshold     float64       // RMS threshold (0..32767 scale)
	silenceFrames int           // consecutive silence frames needed

	// H6.4 endpointing knobs (defaults set by NewVAD; the call
	// session applies its config before feeding starts).
	RatioThreshold  float64       // speech:silence RMS ratio that closes an utterance
	MaxUtteranceDur time.Duration // hard cap on an open utterance (0 = no cap)

	speechActive    bool
	seenSpeech      bool // any speech observed since Reset (for the start event)
	silenceCount    int
	utteranceFrames int // frames since the utterance opened (duration cap)
	partialFired    bool

	speechWin  *rmsRing // RMS of the last ~400 ms of SPEECH frames
	silenceWin *rmsRing // RMS of the last ~300 ms of below-threshold frames
}

// NewVAD builds a VAD. frameDur is the assumed duration of each fed
// frame; silenceDur is the trailing-silence window that gates the
// early-partial event and the endpoint (H6.4 default: 500 ms);
// threshold is the RMS energy floor (typical voice is thousands on
// the 0..32767 scale; room noise is usually under a few hundred).
func NewVAD(frameDur, silenceDur time.Duration, threshold float64) *VAD {
	if frameDur <= 0 {
		frameDur = 30 * time.Millisecond
	}
	if silenceDur <= 0 {
		silenceDur = 500 * time.Millisecond
	}
	if threshold <= 0 {
		threshold = 300
	}
	silenceFrames := int(math.Ceil(float64(silenceDur) / float64(frameDur)))
	if silenceFrames < 1 {
		silenceFrames = 1
	}
	return &VAD{
		frameDur:        frameDur,
		silenceDur:      silenceDur,
		threshold:       threshold,
		silenceFrames:   silenceFrames,
		RatioThreshold:  3.0,
		MaxUtteranceDur: 15 * time.Second,
		speechWin:       newRMSRing(winFrames(frameDur, 400*time.Millisecond)),
		silenceWin:      newRMSRing(winFrames(frameDur, 300*time.Millisecond)),
	}
}

// VADFrame are the events one Feed call can produce.
type VADFrame struct {
	SpeechStarted  bool // first speech frame of an utterance
	Partial        bool // first trailing-silence window elapsed (early partial, once)
	UtteranceEnded bool // endpointing rule closed the utterance
}

// EndpointDecision is the outcome of one pure endpoint evaluation.
type EndpointDecision struct {
	End    bool
	Reason string // "ratio" (silence + energy ratio) | "max" (hard cap)
}

// DecideEndpoint is the H6.4 endpointing rule, as a pure function of
// the windows and durations (no VAD state):
//
//   - force-end when the utterance has been open for at least
//     maxUtterance (hard cap, applies even mid-speech);
//   - otherwise end when trailingSilence ≥ minSilence AND the energy
//     ratio mean(speech window) / mean(silence window) is ≥ ratio:
//     the silence must actually be quiet relative to the speech.
//     Digital silence (meanSilence ≤ 0) counts as an infinite ratio
//     and ends; a missing speech reference (meanSpeech ≤ 0) cannot be
//     ratio-tested and does not end on the minimum silence alone.
func DecideEndpoint(meanSpeechRMS, meanSilenceRMS, ratio float64,
	trailingSilence, minSilence, utteranceDur, maxUtterance time.Duration) EndpointDecision {
	if maxUtterance > 0 && utteranceDur >= maxUtterance {
		return EndpointDecision{End: true, Reason: "max"}
	}
	if trailingSilence < minSilence {
		return EndpointDecision{}
	}
	if ratio <= 0 {
		ratio = 3.0
	}
	switch {
	case meanSilenceRMS <= 0:
		return EndpointDecision{End: true, Reason: "ratio"} // digital silence
	case meanSpeechRMS <= 0:
		return EndpointDecision{} // no speech reference: cannot ratio-test
	case meanSpeechRMS/meanSilenceRMS >= ratio:
		return EndpointDecision{End: true, Reason: "ratio"}
	default:
		return EndpointDecision{}
	}
}

// FrameRMS is the root-mean-square energy of a s16le PCM frame.
// An empty frame has RMS 0.
func FrameRMS(frame []byte) float64 {
	n := len(frame) / 2
	if n == 0 {
		return 0
	}
	var sum float64
	for i := 0; i < 2*n; i += 2 {
		sample := int16(frame[i]) | int16(frame[i+1])<<8
		sum += float64(sample) * float64(sample)
	}
	return math.Sqrt(sum / float64(n))
}

// Feed advances the VAD with one PCM frame and returns the events the
// frame produced (usually none).
func (v *VAD) Feed(frame []byte) VADFrame {
	ev := VADFrame{}
	energy := FrameRMS(frame)
	if energy < v.threshold {
		if !v.speechActive {
			return ev // silence before any speech: nothing to track
		}
		v.utteranceFrames++
		v.silenceCount++
		v.silenceWin.push(energy)
		if !v.partialFired && v.silenceCount == v.silenceFrames {
			// First trailing-silence window: the H6.4 early-partial
			// point. (If the same frame also ends the utterance the
			// caller ignores the partial — see the call session.)
			ev.Partial = true
			v.partialFired = true
		}
		if v.silenceCount >= v.silenceFrames &&
			DecideEndpoint(v.speechWin.mean(), v.silenceWin.mean(),
				v.RatioThreshold,
				time.Duration(v.silenceCount)*v.frameDur,
				v.silenceDur, v.utteranceDur(), v.MaxUtteranceDur).End {
			ev.UtteranceEnded = true
			v.speechActive = false
			v.silenceCount = 0
		}
		return ev
	}
	// Speech frame.
	v.utteranceFrames++
	v.speechWin.push(energy)
	if !v.speechActive {
		v.speechActive = true
		v.silenceCount = 0
		if !v.seenSpeech {
			v.seenSpeech = true
			ev.SpeechStarted = true
		}
	}
	// Hard cap applies mid-speech too.
	if v.MaxUtteranceDur > 0 && v.utteranceDur() >= v.MaxUtteranceDur {
		ev.UtteranceEnded = true
		v.speechActive = false
		v.silenceCount = 0
	}
	return ev
}

// utteranceDur is the open-utterance duration on the frame grid.
func (v *VAD) utteranceDur() time.Duration {
	return time.Duration(v.utteranceFrames) * v.frameDur
}

// Reset returns the VAD to its initial state (fresh utterance
// tracking). The call session resets it after each endpointing
// decision and on barge-in priming.
func (v *VAD) Reset() {
	v.speechActive = false
	v.seenSpeech = false
	v.silenceCount = 0
	v.utteranceFrames = 0
	v.partialFired = false
	v.speechWin = newRMSRing(len(v.speechWin.vals))
	v.silenceWin = newRMSRing(len(v.silenceWin.vals))
}

// String implements a human-readable view for logs/debugging.
func (v *VAD) String() string {
	state := "idle"
	if v.speechActive {
		state = "speaking"
	}
	return fmt.Sprintf("vad{state:%s, silence:%dms/%s, speechRMS:%.0f, silenceRMS:%.0f, ratio:%.2f, dur:%s}",
		state, time.Duration(v.silenceCount)*v.frameDur, v.silenceDur,
		v.speechWin.mean(), v.silenceWin.mean(), v.RatioThreshold, v.utteranceDur())
}

// winFrames converts a window duration into frame count (≥1).
func winFrames(frameDur, window time.Duration) int {
	n := int(math.Ceil(float64(window) / float64(frameDur)))
	if n < 1 {
		n = 1
	}
	return n
}

// rmsRing is a fixed-capacity ring of frame RMS values; mean is the
// average of the values seen so far (the window slides as new values
// arrive).
type rmsRing struct {
	vals []float64
	i    int
	n    int
}

func newRMSRing(capacity int) *rmsRing {
	if capacity < 1 {
		capacity = 1
	}
	return &rmsRing{vals: make([]float64, capacity)}
}

func (r *rmsRing) push(v float64) {
	r.vals[r.i] = v
	r.i = (r.i + 1) % len(r.vals)
	if r.n < len(r.vals) {
		r.n++
	}
}

func (r *rmsRing) mean() float64 {
	if r.n == 0 {
		return 0
	}
	var s float64
	for i := 0; i < r.n; i++ {
		s += r.vals[i]
	}
	return s / float64(r.n)
}
